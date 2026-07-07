package webserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"bticino_bridge/pkg/bticino"
	"bticino_bridge/pkg/config"
	"bticino_bridge/pkg/messageparser"
	"bticino_bridge/pkg/version"
	"github.com/sirupsen/logrus"
)

// ==================== LOG BUFFER (RING BUFFER FOR WEB VIEWER) ====================

// LogEntry represents a single log entry for the web viewer
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Fields    string `json:"fields,omitempty"`
}

// LogBuffer is a thread-safe ring buffer that stores recent log entries
type LogBuffer struct {
	entries []LogEntry
	mu      sync.RWMutex
	maxSize int
	pos     int
	full    bool
}

// NewLogBuffer creates a new log ring buffer with specified capacity
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		entries: make([]LogEntry, maxSize),
		maxSize: maxSize,
	}
}

// Add appends a log entry to the ring buffer
func (lb *LogBuffer) Add(entry LogEntry) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.entries[lb.pos] = entry
	lb.pos = (lb.pos + 1) % lb.maxSize
	if lb.pos == 0 {
		lb.full = true
	}
}

// GetAll returns all log entries in chronological order
func (lb *LogBuffer) GetAll() []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	if !lb.full {
		result := make([]LogEntry, lb.pos)
		copy(result, lb.entries[:lb.pos])
		return result
	}
	result := make([]LogEntry, lb.maxSize)
	copy(result, lb.entries[lb.pos:])
	copy(result[lb.maxSize-lb.pos:], lb.entries[:lb.pos])
	return result
}

// GetLast returns the last n log entries in chronological order
func (lb *LogBuffer) GetLast(n int) []LogEntry {
	all := lb.GetAll()
	if n >= len(all) {
		return all
	}
	return all[len(all)-n:]
}

// LogBufferHook is a logrus hook that captures log entries into a LogBuffer
type LogBufferHook struct {
	buffer *LogBuffer
}

// NewLogBufferHook creates a logrus hook backed by the given LogBuffer
func NewLogBufferHook(buffer *LogBuffer) *LogBufferHook {
	return &LogBufferHook{buffer: buffer}
}

// Levels returns all log levels this hook should fire for
func (h *LogBufferHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire is called by logrus for each log entry
func (h *LogBufferHook) Fire(entry *logrus.Entry) error {
	fields := ""
	if len(entry.Data) > 0 {
		parts := make([]string, 0, len(entry.Data))
		for k, v := range entry.Data {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		fields = strings.Join(parts, " ")
	}
	h.buffer.Add(LogEntry{
		Timestamp: entry.Time.Format("2006-01-02 15:04:05"),
		Level:     entry.Level.String(),
		Message:   entry.Message,
		Fields:    fields,
	})
	return nil
}

// BTicinoBridge interface defines the methods we need from the MQTT bridge
type BTicinoBridge interface {
	GetMulticastStats() map[string]interface{}
	GetEventBusStats() map[string]interface{}
	GetVideoStats() map[string]interface{}
	SendOpenWebNetCommand(command string) error // NEW: For sending OpenWebNet commands
}

// MQTTPublisher defines a function type for MQTT message publishing
type MQTTPublisher func(topic, payload string, retain bool) error

// MQTTStatusProvider interface for getting MQTT bridge status without import cycle
type MQTTStatusProvider interface {
	GetMQTTStatus() map[string]interface{}
	IsConnected() bool
}

// LEDStatusFunc returns LED states as map of name -> on/off
type LEDStatusFunc func() map[string]bool

// SnapshotFunc captures a JPEG frame from the camera.
// timeout covers the whole capture (including stream activation);
// maxAge > 0 allows returning a cached snapshot newer than maxAge.
type SnapshotFunc func(timeout, maxAge time.Duration) ([]byte, error)

// CallController exposes SIP call control to the web API.
type CallController interface {
	GetCallStateString() string // Idle/Registered/IncomingCall/Connected/...
	IsRegistered() bool
	Hangup() error // send BYE to end the active call
}

// VideoProbeFunc runs a single cooperative video probe (one *7*300, no retry,
// no self-INVITE). Returns a diagnostic report and, if any, the best JPEG frame.
type VideoProbeFunc func() (report map[string]interface{}, jpeg []byte, err error)

// AudioProbeFunc runs a single cooperative audio probe (one *7*300 type=2) and
// measures the incoming Speex RTP flow. Returns a diagnostic report.
type AudioProbeFunc func() (report map[string]interface{}, err error)

// WebServer provides HTTP interface for BTicino bridge management
type WebServer struct {
	config             *config.Config
	bridge             BTicinoBridge
	logger             *logrus.Logger
	server             *http.Server
	staticDir          string
	messageParser      *messageparser.MessageParser
	mqttPublisher      MQTTPublisher        // Optional MQTT publishing function
	mqttStatusProvider MQTTStatusProvider   // Optional MQTT status provider
	ledStatusFunc      LEDStatusFunc        // Optional LED status reader
	snapshotFunc       SnapshotFunc         // Optional camera JPEG snapshot provider
	callController     CallController       // Optional SIP call control
	videoProbeFunc     VideoProbeFunc       // Optional one-shot cooperative video probe
	audioProbeFunc     AudioProbeFunc       // Optional one-shot cooperative audio probe
	logBuffer          *LogBuffer           // Ring buffer for web log viewer
	startTime          time.Time            // Track when server started for real uptime
	configManager      *ConfigManager       // Configuration manager for web-based config
	configPath         string               // Path to configuration file
	sseClients         map[chan []byte]bool // SSE clients for real-time updates
	sseMu              sync.Mutex
}

// NewWebServer creates a new web server instance
func NewWebServer(cfg *config.Config, bridge BTicinoBridge, logger *logrus.Logger, configPath ...string) *WebServer {
	logBuf := NewLogBuffer(5000)
	logger.AddHook(NewLogBufferHook(logBuf))

	// Determine config path: use provided path or default
	cfgPath := "configs/config.yaml"
	if len(configPath) > 0 && configPath[0] != "" {
		cfgPath = configPath[0]
	}

	// Initialize config manager
	configManager := NewConfigManager(cfgPath, logger)
	if _, err := configManager.LoadConfig(); err != nil {
		logger.WithError(err).Warnf("Failed to load config from %s, using provided config", cfgPath)
	}

	return &WebServer{
		config:        cfg,
		bridge:        bridge,
		logger:        logger,
		staticDir:     cfg.Web.StaticDir,
		messageParser: messageparser.NewMessageParser(),
		logBuffer:     logBuf,
		startTime:     time.Now(),
		configManager: configManager,
		configPath:    cfgPath,
		sseClients:    make(map[chan []byte]bool),
	}
}

// SetMQTTPublisher sets the MQTT publisher function for real-time updates
func (ws *WebServer) SetMQTTPublisher(publisher MQTTPublisher) {
	ws.mqttPublisher = publisher
}

// SetMQTTStatusProvider sets the MQTT status provider
func (ws *WebServer) SetMQTTStatusProvider(provider MQTTStatusProvider) {
	ws.mqttStatusProvider = provider
}

// SetLEDStatusFunc sets the function to read LED states
func (ws *WebServer) SetLEDStatusFunc(fn LEDStatusFunc) {
	ws.ledStatusFunc = fn
}

// SetSnapshotFunc sets the camera JPEG snapshot provider
func (ws *WebServer) SetSnapshotFunc(fn SnapshotFunc) {
	ws.snapshotFunc = fn
}

// SetCallController sets the SIP call controller
func (ws *WebServer) SetCallController(cc CallController) {
	ws.callController = cc
}

// SetVideoProbeFunc sets the one-shot cooperative video probe
func (ws *WebServer) SetVideoProbeFunc(fn VideoProbeFunc) {
	ws.videoProbeFunc = fn
}

// SetAudioProbeFunc sets the one-shot cooperative audio probe
func (ws *WebServer) SetAudioProbeFunc(fn AudioProbeFunc) {
	ws.audioProbeFunc = fn
}

// SetWebServer sets reference to self for callbacks (used for SSE broadcasting)
var globalWebServer *WebServer

func (ws *WebServer) SetGlobalWebServer() {
	globalWebServer = ws
}

// GetGlobalWebServer returns the global WebServer instance
func GetGlobalWebServer() *WebServer {
	return globalWebServer
}

// Start starts the web server
func (ws *WebServer) Start(ctx context.Context) error {
	if !ws.config.Web.Enabled {
		ws.logger.Info("Web server disabled in configuration")
		return nil
	}

	// Setup routes
	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/status", ws.handleAPIStatus)
	mux.HandleFunc("/api/snapshot", ws.handleAPISnapshot)
	mux.HandleFunc("/api/call", ws.handleAPICallState)
	mux.HandleFunc("/api/controls/call/hangup", ws.handleCallHangup)
	mux.HandleFunc("/api/video/probe", ws.handleVideoProbe)
	mux.HandleFunc("/api/audio/probe", ws.handleAudioProbe)
	mux.HandleFunc("/api/messages", ws.handleAPIMessages)
	mux.HandleFunc("/api/system", ws.handleAPISystem)
	mux.HandleFunc("/api/logs", ws.handleAPILogs)
	mux.HandleFunc("/api/logs/download", ws.handleAPILogsDownload)
	mux.HandleFunc("/api/controls/door/unlock", ws.handleDoorUnlock)
	mux.HandleFunc("/api/controls/answering-machine/toggle", ws.handleAnsweringMachineToggle)
	mux.HandleFunc("/api/controls/display/on", ws.handleDisplayOn)
	mux.HandleFunc("/api/controls/display/off", ws.handleDisplayOff)
	mux.HandleFunc("/api/controls/mute/on", ws.handleMuteOn)
	mux.HandleFunc("/api/controls/mute/off", ws.handleMuteOff)
	mux.HandleFunc("/api/controls/doorbell/on", ws.handleDoorbellSoundOn)
	mux.HandleFunc("/api/controls/doorbell/off", ws.handleDoorbellSoundOff)
	mux.HandleFunc("/api/controls/light/on", ws.handleLightOn)
	mux.HandleFunc("/api/controls/command", ws.handleArbitraryCommand)

	// Enhanced Message Management API Routes
	mux.HandleFunc("/api/messages/list", ws.handleAPIMessagesList)
	mux.HandleFunc("/api/messages/", ws.handleAPIMessageDetail)
	mux.HandleFunc("/api/messages/download/", ws.handleAPIMessageDownload)
	mux.HandleFunc("/api/messages/mark-all-read", ws.handleAPIMessageMarkAllRead)
	mux.HandleFunc("/api/messages/mark-read/", ws.handleAPIMessageMarkRead)
	mux.HandleFunc("/api/messages/delete/", ws.handleAPIMessageDelete)

	// Memos API (voice and text notes)
	mux.HandleFunc("/api/memos", ws.handleAPIMemos)
	mux.HandleFunc("/api/memos/", ws.handleAPIMemoDetail)
	mux.HandleFunc("/api/memos/audio/", ws.handleAPIMemoAudio)
	mux.HandleFunc("/api/memos/mark-read/", ws.handleAPIMemoMarkRead)
	mux.HandleFunc("/api/memos/delete/", ws.handleAPIMemoDelete)

	// Streaming API Routes (RTSP/WebRTC)
	mux.HandleFunc("/api/streaming", ws.handleAPIStreaming)
	mux.HandleFunc("/api/streaming/start", ws.handleAPIStreamingStart)
	mux.HandleFunc("/api/streaming/stop", ws.handleAPIStreamingStop)
	mux.HandleFunc("/api/streaming/sessions", ws.handleAPIStreamingSessions)
	mux.HandleFunc("/api/streaming/record", ws.handleAPIStreamingRecord)
	mux.HandleFunc("/api/streaming/config", ws.handleAPIStreamingConfig)
	mux.HandleFunc("/api/webrtc/start", ws.handleAPIWebRTCStart)
	mux.HandleFunc("/api/webrtc/stop", ws.handleAPIWebRTCStop)
	mux.HandleFunc("/api/webrtc/status", ws.handleAPIWebRTCStatus)

	// Serve Svelte static files from web/dist
	fs := http.FileServer(http.Dir("web"))
	mux.Handle("/", fs)

	// Configuration Management API Routes (NEW in v0.13.0)
	mux.HandleFunc("/api/config", ws.handleAPIConfig)
	mux.HandleFunc("/api/config/save", ws.handleAPIConfigSave)
	mux.HandleFunc("/api/config/validate", ws.handleAPIConfigValidate)
	mux.HandleFunc("/api/config/backup", ws.handleAPIConfigBackup)
	mux.HandleFunc("/api/config/backups", ws.handleAPIConfigBackup)
	mux.HandleFunc("/api/config/restore", ws.handleAPIConfigRestore)
	mux.HandleFunc("/api/config/history", ws.handleAPIConfigHistory)
	mux.HandleFunc("/api/config/reload", ws.handleAPIConfigReload)

	// Device Configuration API Routes (NEW in v0.14.2)
	mux.HandleFunc("/api/device/ntp", ws.handleAPIDeviceNTP)
	mux.HandleFunc("/api/device/timezone", ws.handleAPIDeviceTimezone)
	mux.HandleFunc("/api/device/language", ws.handleAPIDeviceLanguage)
	mux.HandleFunc("/api/device/ringtone", ws.handleAPIDeviceRingtone)
	mux.HandleFunc("/api/device/ringtones", ws.handleAPIDeviceRingtones)
	mux.HandleFunc("/api/device/languages", ws.handleAPIDeviceLanguages)
	mux.HandleFunc("/api/device/save", ws.handleAPIDeviceSave)

	// Device config (Fase 1: Sincronización QML → Bridge)
	mux.HandleFunc("/api/config/device", ws.handleAPIDeviceConfig)
	mux.HandleFunc("/api/config/language", ws.handleAPILanguage)
	mux.HandleFunc("/api/config/sip-accounts", ws.handleAPISIPAccounts)

	// Static assets
	mux.HandleFunc("/assets/config_ui.css", ws.handleConfigCSS)
	mux.HandleFunc("/assets/config_ui.js", ws.handleConfigJS)

	// Swagger UI
	mux.Handle("/api/docs/", http.StripPrefix("/api/docs/", http.FileServer(http.Dir("docs"))))

	// SSE for real-time updates (LEDs, GPIOs, events)
	mux.HandleFunc("/api/events", ws.handleSSEvents)

	ws.server = &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", ws.config.Web.Port),
		Handler: ws.corsMiddleware(mux),
	}

	go func() {
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ws.logger.WithError(err).Error("Web server error")
		}
	}()

	return nil
}

// Stop stops the web server
func (ws *WebServer) Stop() error {
	if ws.server != nil {
		return ws.server.Shutdown(context.Background())
	}
	return nil
}

// injectVersion replaces the placeholder version string in HTML/JS/CSS content
// with the actual version from the version package
func (ws *WebServer) injectVersion(content string) string {
	return strings.ReplaceAll(content, "{{VERSION}}", version.GetVersion())
}

// Handler functions
func (ws *WebServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(ws.injectVersion(ws.getDashboardHTML())))
}

func (ws *WebServer) handleMessagesPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(ws.injectVersion(ws.getMessagesHTML())))
}

func (ws *WebServer) handleControlsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(ws.injectVersion(ws.getControlsHTML())))
}

func (ws *WebServer) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(ws.injectVersion(ws.getSettingsHTML())))
}

func (ws *WebServer) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(ws.injectVersion(ws.getLogsHTML())))
}

// @Summary Get application logs
// @Description Returns recent application logs with optional filtering by level and count
// @Tags Logs
// @Accept json
// @Produce json
// @Param count query int false "Number of logs to return (max 500)"
// @Param level query string false "Filter by log level (debug, info, warn, error)"
// @Success 200 {object} map[string]interface{}
// @Router /api/logs [get]
func (ws *WebServer) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	countStr := r.URL.Query().Get("count")
	count := 200
	if countStr != "" {
		if n, err := strconv.Atoi(countStr); err == nil && n > 0 && n <= 500 {
			count = n
		}
	}
	levelFilter := r.URL.Query().Get("level")
	entries := ws.logBuffer.GetLast(count)
	if levelFilter != "" && levelFilter != "all" {
		filtered := make([]LogEntry, 0, len(entries))
		for _, e := range entries {
			if e.Level == levelFilter {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	ws.writeJSON(w, map[string]interface{}{
		"logs":       entries,
		"total":      len(entries),
		"max_buffer": 500,
	})
}

// @Summary Download application logs
// @Description Downloads application logs as a text file with optional count parameter
// @Tags Logs
// @Accept json
// @Produce text/plain
// @Param count query int false "Number of logs to download (max 2000)"
// @Success 200 {file} text/plain
// @Router /api/logs/download [get]
func (ws *WebServer) handleAPILogsDownload(w http.ResponseWriter, r *http.Request) {
	countStr := r.URL.Query().Get("count")
	count := 500
	if countStr != "" {
		if n, err := strconv.Atoi(countStr); err == nil && n > 0 && n <= 2000 {
			count = n
		}
	}

	entries := ws.logBuffer.GetAll()
	if len(entries) > count {
		entries = entries[len(entries)-count:]
	}

	// Generate text format
	filename := fmt.Sprintf("bticino_bridge_logs_%s.txt", time.Now().Format("2006-01-02_15-04-05"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	for _, e := range entries {
		fmt.Fprintf(w, "[%s] %s: %s %s\n", e.Timestamp, e.Level, e.Message, e.Fields)
	}
}

// @Summary Get system status
// @Description Returns the current system status including version, uptime, MQTT connection, and LED states
// @Tags Status
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/status [get]
func (ws *WebServer) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	// Get real message count from parser
	messageCount := 0
	newMessages := 0
	storageUsed := "0%"
	amStatus, err := ws.messageParser.GetAnsweringMachineStatus()
	if err == nil {
		messageCount = amStatus.TotalMessages
		newMessages = amStatus.NewMessages
		storageUsed = amStatus.StorageUsed
	}

	status := map[string]interface{}{
		"version":      version.GetVersion(),
		"timestamp":    time.Now(),
		"uptime":       ws.getUptime(),
		"boot_time":    ws.startTime.Format(time.RFC3339), // permite verificar reinicios aunque el restart haga timeout
		"storage_used": storageUsed,
		"components": map[string]interface{}{
			"message_parser": map[string]interface{}{
				"status":         "active",
				"messages_found": messageCount,
				"new_messages":   newMessages,
			},
			"openwebnet": map[string]interface{}{
				"status": "active",
				"port":   bticino.OpenWebNetLocalPort,
			},
			"web_dashboard": map[string]interface{}{
				"status": "active",
				"port":   ws.config.Web.Port,
			},
		},
	}

	// Add MQTT status if provider available
	if ws.mqttStatusProvider != nil {
		status["mqtt"] = map[string]interface{}{
			"connected":     ws.mqttStatusProvider.IsConnected(),
			"topics_prefix": ws.config.MQTT.TopicPrefix,
			"broker":        ws.config.MQTT.Host,
		}
	}

	// Add LED status if reader available
	if ws.ledStatusFunc != nil {
		ledStates := ws.ledStatusFunc()
		status["leds"] = ledStates
	}

	ws.writeJSON(w, status)
}

// @Summary Get camera snapshot
// @Description Captures a JPEG frame from the door camera, starting the video stream if needed. Query params: timeout (seconds, default 20) and max_age (seconds, default 2; 0 disables the snapshot cache).
// @Tags Streaming
// @Produce jpeg
// @Success 200 {string} binary "JPEG image"
// @Failure 503 {object} map[string]interface{}
// @Router /api/snapshot [get]
func (ws *WebServer) handleAPISnapshot(w http.ResponseWriter, r *http.Request) {
	if ws.snapshotFunc == nil {
		http.Error(w, "snapshot unavailable: video subsystem not active", http.StatusServiceUnavailable)
		return
	}

	timeout := 20 * time.Second
	if v := r.URL.Query().Get("timeout"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 && secs <= 120 {
			timeout = time.Duration(secs) * time.Second
		}
	}
	maxAge := 2 * time.Second
	if v := r.URL.Query().Get("max_age"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			maxAge = time.Duration(secs) * time.Second
		}
	}

	jpeg, err := ws.snapshotFunc(timeout, maxAge)
	if err != nil {
		ws.logger.WithError(err).Warn("Snapshot capture failed")
		http.Error(w, fmt.Sprintf("snapshot capture failed: %v", err), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(jpeg)))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(jpeg)
}

// @Summary Cooperative video probe (diagnostic)
// @Description Sends a SINGLE *7*300 (no retry, no self-INVITE) asking bt_av_media to duplicate its video RTP, then tries to decode a JPEG. Use while native video is active (eye/auto-on button). Requires POST + ?confirm=yes. On success returns image/jpeg with X-Probe-* headers; otherwise a JSON report.
// @Tags Streaming
// @Produce jpeg
// @Produce json
// @Router /api/video/probe [post]
func (ws *WebServer) handleVideoProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed (use POST)", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("confirm") != "yes" {
		http.Error(w, "confirm required: POST /api/video/probe?confirm=yes", http.StatusBadRequest)
		return
	}
	if ws.videoProbeFunc == nil {
		http.Error(w, "video probe unavailable (no OpenWebNet client)", http.StatusServiceUnavailable)
		return
	}

	report, jpeg, err := ws.videoProbeFunc()
	if err != nil {
		ws.logger.WithError(err).Warn("Video probe failed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		ws.writeJSON(w, map[string]interface{}{"error": err.Error(), "report": report})
		return
	}

	// Cabeceras de diagnóstico siempre presentes
	if report != nil {
		if ack, ok := report["ack"].(bool); ok {
			w.Header().Set("X-Probe-Ack", strconv.FormatBool(ack))
		}
		if frames, ok := report["frames"].(int); ok {
			w.Header().Set("X-Probe-Frames", strconv.Itoa(frames))
		}
		if note, ok := report["note"].(string); ok {
			w.Header().Set("X-Probe-Note", note)
		}
	}

	if len(jpeg) > 0 {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", strconv.Itoa(len(jpeg)))
		w.Header().Set("Cache-Control", "no-store")
		w.Write(jpeg)
		return
	}

	// Sin imagen: devolver el informe en JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	ws.writeJSON(w, map[string]interface{}{"report": report})
}

// @Summary Cooperative audio probe (diagnostic)
// @Description Sends a SINGLE *7*300 type=2 asking bt_av_media to duplicate its Speex audio RTP, then measures the incoming flow. Use with the native session active (eye + phone buttons). Requires POST + ?confirm=yes.
// @Tags Streaming
// @Produce json
// @Router /api/audio/probe [post]
func (ws *WebServer) handleAudioProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed (use POST)", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("confirm") != "yes" {
		http.Error(w, "confirm required: POST /api/audio/probe?confirm=yes", http.StatusBadRequest)
		return
	}
	if ws.audioProbeFunc == nil {
		http.Error(w, "audio probe unavailable (no OpenWebNet client)", http.StatusServiceUnavailable)
		return
	}

	report, err := ws.audioProbeFunc()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		ws.logger.WithError(err).Warn("Audio probe failed")
		w.WriteHeader(http.StatusServiceUnavailable)
		ws.writeJSON(w, map[string]interface{}{"error": err.Error(), "report": report})
		return
	}

	// 200 si llegó RTP, 503 si no
	packets := 0
	if report != nil {
		if p, ok := report["packets"].(int); ok {
			packets = p
		}
	}
	if packets == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	ws.writeJSON(w, map[string]interface{}{"report": report})
}

// @Summary Get SIP call state
// @Description Returns the current SIP registration and call state (Idle/Registered/IncomingCall/Connected/...).
// @Tags Streaming
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/call [get]
func (ws *WebServer) handleAPICallState(w http.ResponseWriter, r *http.Request) {
	if ws.callController == nil {
		ws.writeJSON(w, map[string]interface{}{"available": false, "state": "unavailable"})
		return
	}
	ws.writeJSON(w, map[string]interface{}{
		"available":  true,
		"state":      ws.callController.GetCallStateString(),
		"registered": ws.callController.IsRegistered(),
	})
}

// @Summary Hang up the active SIP call
// @Description Sends a SIP BYE to end the current call.
// @Tags Streaming
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /api/controls/call/hangup [post]
func (ws *WebServer) handleCallHangup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if ws.callController == nil {
		http.Error(w, "call control unavailable: SIP subsystem not active", http.StatusServiceUnavailable)
		return
	}
	if err := ws.callController.Hangup(); err != nil {
		ws.logger.WithError(err).Warn("Call hangup failed")
		http.Error(w, fmt.Sprintf("hangup failed: %v", err), http.StatusServiceUnavailable)
		return
	}
	ws.writeJSON(w, map[string]interface{}{"success": true, "action": "hangup"})
}

// @Summary Get system information
// @Description Returns detailed system information including device info, filesystem paths, and network configuration
// @Tags Status
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/system [get]
func (ws *WebServer) handleAPISystem(w http.ResponseWriter, r *http.Request) {
	system := map[string]interface{}{
		"device_info": map[string]interface{}{
			"model":        "BTicino Classe 300X",
			"ip":           "192.168.1.38",
			"architecture": "ARM v7l",
		},
		"filesystem": map[string]interface{}{
			"messages_dir":    "/home/bticino/cfg/extra/47/messages/",
			"config_files":    []string{"/var/tmp/conf.xml", "/home/bticino/sp/dbfiles_ws.xml"},
			"led_monitoring":  "/sys/class/leds",
			"gpio_monitoring": "/sys/class/gpio",
		},
		"network": map[string]interface{}{
			"openwebnet_ports": []int{20000, 30006, 30007},
			"web_port":         ws.config.Web.Port,
			"homekit_port":     8081,
		},
		"enhanced_features": []string{
			"Real BTicino filesystem integration",
			"Enhanced OpenWebNet with 50+ proven commands",
			"Home Assistant MQTT discovery",
			"Physical device monitoring (GPIO/LED)",
			"Message parsing with image/video support",
			"Cross-compiled ARM support",
		},
	}
	ws.writeJSON(w, system)
}

// @Summary Server-Sent Events for real-time updates
// @Description Provides real-time updates for LEDs, GPIOs, and system events via SSE
// @Tags Status
// @Accept json
// @Produce text/event-stream
// @Success 200 {object} stream
// @Router /api/events [get]
func (ws *WebServer) handleSSEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	clientChan := make(chan []byte, 100)
	ws.sseMu.Lock()
	ws.sseClients[clientChan] = true
	ws.sseMu.Unlock()

	defer func() {
		ws.sseMu.Lock()
		delete(ws.sseClients, clientChan)
		ws.sseMu.Unlock()
		close(clientChan)
	}()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case data := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// BroadcastEvent sends an event to all connected SSE clients
func (ws *WebServer) BroadcastEvent(eventType string, data map[string]interface{}) {
	data["type"] = eventType
	data["timestamp"] = time.Now().Unix()

	jsonData, err := json.Marshal(data)
	if err != nil {
		ws.logger.WithError(err).Error("Failed to marshal SSE event")
		return
	}

	ws.sseMu.Lock()
	defer ws.sseMu.Unlock()

	for client := range ws.sseClients {
		select {
		case client <- jsonData:
		default:
		}
	}
}

// @Summary Get messages summary
// @Description Returns a summary of answering machine messages including total count, new messages, and preview of recent messages
// @Tags Messages
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/messages [get]
func (ws *WebServer) handleAPIMessages(w http.ResponseWriter, r *http.Request) {
	// Get real answering machine status
	status, err := ws.messageParser.GetAnsweringMachineStatus()
	if err != nil {
		ws.logger.WithError(err).Error("Failed to get answering machine status")
		status = &messageparser.AnsweringMachineStatus{
			Enabled:       false,
			TotalMessages: 0,
			NewMessages:   0,
			StorageUsed:   "0%",
			LastChecked:   time.Now(),
		}
	}

	// Get recent messages for preview (limit to 10 most recent)
	allMessages, err := ws.messageParser.GetAllMessages()
	if err != nil {
		ws.logger.WithError(err).Error("Failed to get messages")
		allMessages = []*messageparser.Message{}
	}

	// Limit to 10 most recent messages for the basic endpoint
	previewMessages := allMessages
	if len(allMessages) > 10 {
		previewMessages = allMessages[:10]
	}

	messages := map[string]interface{}{
		"answering_machine": map[string]interface{}{
			"enabled":        status.Enabled,
			"new_messages":   status.NewMessages,
			"total_messages": status.TotalMessages,
			"storage_used":   status.StorageUsed,
			"last_checked":   status.LastChecked.Format(time.RFC3339),
		},
		"messages": previewMessages, // Real messages from BTicino device
		"enhanced_integration": map[string]interface{}{
			"real_filesystem_paths": []string{
				"/home/bticino/cfg/extra/47/messages/",
				"/var/tmp/conf.xml",
				"/sys/class/leds",
				"/sys/class/gpio",
			},
			"proven_commands": []string{
				"*8*91##",        // Enable answering machine
				"*8*92##",        // Disable answering machine
				"*#8**33*1##",    // Audio status query
				"*7*73#1#100*##", // Display activation
			},
			"version":                version.GetVersion(),
			"real_messages_detected": status.TotalMessages,
			"filesystem_integration": "active",
			"features": []string{
				"Real message parsing with metadata",
				"Image and video file access",
				"Message pagination and filtering",
				"Read/unread status management",
				"File download functionality",
				"Message deletion capabilities",
			},
		},
	}
	ws.writeJSON(w, messages)
}

// @Summary Unlock door
// @Description Sends door unlock command to open the door lock (press + release sequence)
// @Tags Controls
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/door/unlock [post]
func (ws *WebServer) handleDoorUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws.logger.Info("Door unlock requested via web dashboard")

	// Send press command
	pressCmd := bticino.CmdDoorOpenPress // *8*19*20##
	err := ws.bridge.SendOpenWebNetCommand(pressCmd)
	if err != nil {
		ws.logger.WithError(err).Error("Failed to send door unlock press command")
		ws.writeJSON(w, map[string]interface{}{
			"success":   false,
			"message":   "Failed to send door unlock command: " + err.Error(),
			"command":   pressCmd,
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Wait 1 second then send release command (like the working script)
	go func() {
		time.Sleep(1 * time.Second)
		releaseCmd := bticino.CmdDoorOpenRelease // *8*20*20##
		if err := ws.bridge.SendOpenWebNetCommand(releaseCmd); err != nil {
			ws.logger.WithError(err).Error("Failed to send door unlock release command")
		} else {
			ws.logger.Info("Door unlock release command sent successfully")
		}
	}()

	ws.logger.Info("Door unlock press command sent, release scheduled in 1s")
	ws.writeJSON(w, map[string]interface{}{
		"success":   true,
		"message":   "Door unlock command sent (press + release)",
		"command":   pressCmd,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// @Summary Toggle answering machine
// @Description Enable or disable the answering machine (voicemail) feature
// @Tags Controls
// @Accept json
// @Produce json
// @Param action query string false "Action: enable or disable"
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/answering-machine/toggle [post]
func (ws *WebServer) handleAnsweringMachineToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	action := r.URL.Query().Get("action") // "enable" or "disable"

	var command string
	var message string

	if action == "enable" {
		command = bticino.CmdVoicemailOn // *8*91##
		message = "Answering machine enabled"
	} else {
		command = bticino.CmdVoicemailOff // *8*92##
		message = "Answering machine disabled"
	}

	ws.logger.Infof("Answering machine %s requested via web dashboard", action)

	// Send real OpenWebNet command
	err := ws.bridge.SendOpenWebNetCommand(command)

	var result map[string]interface{}
	if err != nil {
		ws.logger.WithError(err).Errorf("Failed to send answering machine command: %s", command)
		result = map[string]interface{}{
			"success":   false,
			"message":   "Failed to " + message + ": " + err.Error(),
			"command":   command,
			"action":    action,
			"timestamp": time.Now().Format(time.RFC3339),
		}
	} else {
		ws.logger.Infof("Answering machine command sent successfully: %s", command)
		result = map[string]interface{}{
			"success":   true,
			"message":   message,
			"command":   command,
			"action":    action,
			"timestamp": time.Now().Format(time.RFC3339),
		}
	}

	ws.writeJSON(w, result)
}

// handleGenericOWNCommand is a helper that sends a single OWN command and returns JSON result
func (ws *WebServer) sendOWNCommand(w http.ResponseWriter, r *http.Request, command, description string) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws.logger.Infof("%s requested via web dashboard", description)
	err := ws.bridge.SendOpenWebNetCommand(command)

	result := map[string]interface{}{
		"command":   command,
		"action":    description,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	if err != nil {
		ws.logger.WithError(err).Errorf("Failed: %s", description)
		result["success"] = false
		result["message"] = "Failed: " + err.Error()
	} else {
		ws.logger.Infof("Success: %s", description)
		result["success"] = true
		result["message"] = description
	}
	ws.writeJSON(w, result)
}

// @Summary Turn display on
// @Description Turn on the device display
// @Tags Controls
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/display/on [post]
func (ws *WebServer) handleDisplayOn(w http.ResponseWriter, r *http.Request) {
	ws.sendOWNCommand(w, r, bticino.CmdDisplayOn, "Display ON")
}

// @Summary Turn display off
// @Description Turn off the device display
// @Tags Controls
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/display/off [post]
func (ws *WebServer) handleDisplayOff(w http.ResponseWriter, r *http.Request) {
	ws.sendOWNCommand(w, r, bticino.CmdDisplayOff, "Display OFF")
}

// @Summary Mute device
// @Description Mute the device (disable sounds)
// @Tags Controls
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/mute/on [post]
func (ws *WebServer) handleMuteOn(w http.ResponseWriter, r *http.Request) {
	ws.sendOWNCommand(w, r, bticino.CmdMuteOn, "Mute ON")
}

// @Summary Unmute device
// @Description Unmute the device (enable sounds)
// @Tags Controls
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/mute/off [post]
func (ws *WebServer) handleMuteOff(w http.ResponseWriter, r *http.Request) {
	ws.sendOWNCommand(w, r, bticino.CmdMuteOff, "Mute OFF")
}

// @Summary Enable doorbell sound
// @Description Enable the doorbell sound
// @Tags Controls
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/doorbell/on [post]
func (ws *WebServer) handleDoorbellSoundOn(w http.ResponseWriter, r *http.Request) {
	ws.sendOWNCommand(w, r, bticino.CmdBellOn, "Doorbell sound ON")
}

// @Summary Disable doorbell sound
// @Description Disable the doorbell sound
// @Tags Controls
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/doorbell/off [post]
func (ws *WebServer) handleDoorbellSoundOff(w http.ResponseWriter, r *http.Request) {
	ws.sendOWNCommand(w, r, bticino.CmdBellOff, "Doorbell sound OFF")
}

// @Summary Turn staircase light on
// @Description Turn on the staircase light (press + release sequence)
// @Tags Controls
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/light/on [post]
func (ws *WebServer) handleLightOn(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ws.logger.Info("Staircase light ON requested via web dashboard")
	err := ws.bridge.SendOpenWebNetCommand(bticino.CmdLightOnPress)
	if err != nil {
		ws.writeJSON(w, map[string]interface{}{"success": false, "message": err.Error()})
		return
	}
	// Send release after 1 second (same as door unlock pattern)
	go func() {
		time.Sleep(1 * time.Second)
		if err := ws.bridge.SendOpenWebNetCommand(bticino.CmdLightOnRelease); err != nil {
			ws.logger.WithError(err).Error("Failed to send light release command")
		}
	}()
	ws.writeJSON(w, map[string]interface{}{
		"success":   true,
		"message":   "Staircase light ON (press + release)",
		"command":   bticino.CmdLightOnPress,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// @Summary Send arbitrary OpenWebNet command
// @Description Send any OpenWebNet command (must start with * and end with ##)
// @Tags Controls
// @Accept json
// @Produce json
// @Param request body object true "Command payload"
// @Success 200 {object} map[string]interface{}
// @Router /api/controls/command [post]
// handleArbitraryCommand allows sending any OpenWebNet command (for advanced users / debugging)
func (ws *WebServer) handleArbitraryCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Command == "" {
		http.Error(w, "Invalid request: JSON body with 'command' field required", http.StatusBadRequest)
		return
	}

	// Basic validation: OWN commands start with * and end with ##
	if !strings.HasPrefix(body.Command, "*") || !strings.HasSuffix(body.Command, "##") {
		http.Error(w, "Invalid OpenWebNet command format (must start with * and end with ##)", http.StatusBadRequest)
		return
	}

	ws.logger.Infof("Arbitrary command requested: %s", body.Command)
	err := ws.bridge.SendOpenWebNetCommand(body.Command)

	result := map[string]interface{}{
		"command":   body.Command,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	if err != nil {
		result["success"] = false
		result["message"] = "Failed: " + err.Error()
	} else {
		result["success"] = true
		result["message"] = "Command sent: " + body.Command
	}
	ws.writeJSON(w, result)
}

func (ws *WebServer) handleCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Write([]byte(ws.injectVersion(ws.getCSS())))
}

func (ws *WebServer) handleJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write([]byte(ws.injectVersion(ws.getJS())))
}

func (ws *WebServer) handleConfigCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Write([]byte(ws.getConfigCSS()))
}

func (ws *WebServer) handleConfigJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write([]byte(ws.getConfigJS()))
}

func (ws *WebServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (ws *WebServer) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		ws.logger.WithError(err).Error("Failed to write JSON response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (ws *WebServer) getUptime() string {
	d := time.Since(ws.startTime)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func (ws *WebServer) getMockMessages() []interface{} {
	return []interface{}{
		map[string]interface{}{
			"id":        1,
			"timestamp": time.Now().Add(-2 * time.Hour).Format("2006-01-02 15:04:05"),
			"duration":  "15",
			"read":      false,
			"caller_id": "External",
			"message":   "Real message from BTicino device",
			"type":      "voice_message",
			"has_image": true,
			"has_video": true,
		},
		map[string]interface{}{
			"id":        2,
			"timestamp": time.Now().Add(-5 * time.Hour).Format("2006-01-02 15:04:05"),
			"duration":  "23",
			"read":      true,
			"caller_id": "Door Entry",
			"message":   "Doorbell activation with video",
			"type":      "door_event",
			"has_image": true,
			"has_video": true,
		},
	}
}

// HTML Templates
func (ws *WebServer) getDashboardHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>BTicino Bridge {{VERSION}} - Dashboard</title>
    <link rel="stylesheet" href="/assets/style.css">
</head>
<body>
    <nav class="navbar">
        <div class="nav-brand">🚀 BTicino Bridge {{VERSION}}</div>
        <div class="nav-links">
            <a href="/dashboard" class="active">Dashboard</a>
            <a href="/messages">Messages</a>
            <a href="/controls">Controls</a>
            <a href="/settings">Settings</a>
            <a href="/logs">Logs</a>
        </div>
    </nav>

    <div class="container">
        <h1>BTicino Bridge Dashboard</h1>
        
        <div class="status-grid">
            <div class="status-card">
                <div class="card-header">
                    <h3>🔌 OpenWebNet Connection</h3>
                    <span class="status-badge status-connected" id="openwebnet-status">Connected</span>
                </div>
                <div class="card-content">
                    <p>Primary Port: <strong>20000</strong></p>
                    <p>Device: <strong>192.168.1.38</strong></p>
                    <p>Commands Available: <strong>50+</strong></p>
                </div>
            </div>

            <div class="status-card">
                <div class="card-header">
                    <h3>📱 Answering Machine</h3>
                    <span class="status-badge status-enabled" id="answering-status">Enabled</span>
                </div>
                <div class="card-content">
                    <p>Total Messages: <strong id="total-messages">25</strong></p>
                    <p>New Messages: <strong id="new-messages">2</strong></p>
                    <p>Storage Used: <strong id="storage-used">18%</strong></p>
                </div>
            </div>

            <div class="status-card">
                <div class="card-header">
                    <h3>🏠 Home Assistant</h3>
                    <span class="status-badge status-ready" id="ha-status">Ready</span>
                </div>
                <div class="card-content">
                    <p>MQTT Entities: <strong id="mqtt-entities">-</strong></p>
                    <p>Connected Since: <strong id="mqtt-since">-</strong></p>
                    <p>Reconnects: <strong id="mqtt-reconnects">0</strong></p>
                    <div id="mqtt-events" class="mqtt-events-list"></div>
                </div>
            </div>

            <div class="status-card">
                <div class="card-header">
                    <h3>🔧 Physical Monitoring</h3>
                    <span class="status-badge status-active" id="monitoring-status">Active</span>
                </div>
                <div class="card-content">
                    <p>GPIO Monitoring: <strong>Available</strong></p>
                    <p>LED Status: <strong>Monitored</strong></p>
                    <p>Thermal: <strong>Normal</strong></p>
                </div>
            </div>
        </div>

        <div class="enhanced-features">
            <h2>✅ Enhanced Features Active</h2>
            <div class="features-grid">
                <div class="feature-item">
                    <span class="feature-icon">🗂️</span>
                    <div>
                        <strong>Real Filesystem Integration</strong>
                        <p>Connected to /home/bticino/cfg/extra/47/messages/</p>
                    </div>
                </div>
                <div class="feature-item">
                    <span class="feature-icon">⚡</span>
                    <div>
                        <strong>Enhanced OpenWebNet Commands</strong>
                        <p>50+ proven commands from device analysis</p>
                    </div>
                </div>
                <div class="feature-item">
                    <span class="feature-icon">📡</span>
                    <div>
                        <strong>MQTT Home Assistant Integration</strong>
                        <p>Device discovery and state publishing ready</p>
                    </div>
                </div>
                <div class="feature-item">
                    <span class="feature-icon">🎯</span>
                    <div>
                        <strong>Physical Device Monitoring</strong>
                        <p>GPIO, LED, and thermal monitoring active</p>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <script src="/assets/app.js"></script>
    <script>
        // Initialize dashboard
        document.addEventListener('DOMContentLoaded', function() {
            refreshStatus();
            setInterval(refreshStatus, 30000); // Refresh every 30 seconds
        });
        
        function downloadLogs(count) {
            window.location.href = '/api/logs/download?count=' + count;
        }
    </script>
</body>
</html>`
}

func (ws *WebServer) getMessagesHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>BTicino Bridge {{VERSION}} - Messages</title>
    <link rel="stylesheet" href="/assets/style.css">
    <style>
        /* Enhanced Messages Styles */
        .messages-controls {
            display: flex;
            gap: 1rem;
            margin-bottom: 1rem;
            flex-wrap: wrap;
            align-items: center;
        }
        
        .filter-controls {
            display: flex;
            gap: 0.5rem;
            align-items: center;
        }
        
        .filter-controls select, .filter-controls input {
            padding: 0.4rem;
            border: 1px solid #ddd;
            border-radius: 4px;
            font-size: 0.9rem;
        }
        
        .message-item {
            background: white;
            border: 1px solid #e1e5e9;
            border-radius: 8px;
            margin-bottom: 1rem;
            padding: 1rem;
            transition: all 0.2s ease;
            cursor: pointer;
        }
        
        .message-item:hover {
            border-color: #007bff;
            transform: translateY(-1px);
            box-shadow: 0 4px 8px rgba(0,123,255,0.1);
        }
        
        .message-item.unread {
            border-left: 4px solid #007bff;
            background: #f8f9ff;
        }
        
        .message-item.selected {
            background: #e3f2fd;
            border-color: #2196f3;
        }
        
        .message-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 0.5rem;
        }
        
        .message-info {
            display: flex;
            gap: 1rem;
            align-items: center;
            font-size: 0.9rem;
            color: #666;
        }
        
        .message-actions {
            display: flex;
            gap: 0.5rem;
            margin-top: 0.5rem;
        }
        
        .message-actions button {
            padding: 0.3rem 0.6rem;
            border: none;
            border-radius: 4px;
            font-size: 0.8rem;
            cursor: pointer;
            transition: background-color 0.2s;
        }
        
        .btn-read { background: #28a745; color: white; }
        .btn-read:hover { background: #218838; }
        .btn-delete { background: #dc3545; color: white; }
        .btn-delete:hover { background: #c82333; }
        .btn-download { background: #17a2b8; color: white; }
        .btn-download:hover { background: #138496; }
        .btn-view { background: #6c757d; color: white; }
        .btn-view:hover { background: #5a6268; }
        
        .pagination {
            display: flex;
            justify-content: center;
            align-items: center;
            gap: 1rem;
            margin: 2rem 0;
            padding: 1rem;
            background: white;
            border-radius: 8px;
            border: 1px solid #e1e5e9;
        }
        
        .pagination button {
            padding: 0.5rem 1rem;
            border: 1px solid #ddd;
            border-radius: 4px;
            background: white;
            cursor: pointer;
            transition: all 0.2s;
        }
        
        .pagination button:hover:not(:disabled) {
            background: #007bff;
            color: white;
        }
        
        .pagination button:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }
        
        .pagination .current-page {
            background: #007bff;
            color: white;
        }
        
        .page-info {
            font-size: 0.9rem;
            color: #666;
        }
        
        .message-detail-modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.5);
            z-index: 1000;
        }
        
        .modal-content {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            background: white;
            border-radius: 8px;
            padding: 2rem;
            max-width: 600px;
            width: 90%;
            max-height: 80vh;
            overflow-y: auto;
        }
        
        .modal-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 1rem;
            padding-bottom: 1rem;
            border-bottom: 1px solid #eee;
        }
        
        .modal-close {
            background: none;
            border: none;
            font-size: 1.5rem;
            cursor: pointer;
            color: #666;
        }
        
        .modal-close:hover {
            color: #333;
        }
        
        .message-media {
            margin: 1rem 0;
            text-align: center;
        }
        
        .message-media img {
            max-width: 100%;
            border-radius: 4px;
            border: 1px solid #ddd;
        }
        
        .loading-spinner {
            display: inline-block;
            width: 20px;
            height: 20px;
            border: 3px solid #f3f3f3;
            border-top: 3px solid #007bff;
            border-radius: 50%;
            animation: spin 1s linear infinite;
        }
        
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        
        .bulk-actions {
            display: flex;
            gap: 0.5rem;
            margin-bottom: 1rem;
            padding: 1rem;
            background: #f8f9fa;
            border-radius: 8px;
        }
        
        .bulk-actions button {
            padding: 0.5rem 1rem;
            border: 1px solid #ddd;
            border-radius: 4px;
            background: white;
            cursor: pointer;
            font-size: 0.9rem;
        }
        
        @media (max-width: 768px) {
            .messages-controls {
                flex-direction: column;
                align-items: stretch;
            }
            
            .filter-controls {
                justify-content: space-between;
            }
            
            .message-header {
                flex-direction: column;
                align-items: flex-start;
                gap: 0.5rem;
            }
            
            .message-actions {
                flex-wrap: wrap;
            }
            
            .pagination {
                flex-wrap: wrap;
                gap: 0.5rem;
            }
        }
    </style>
</head>
<body>
    <nav class="navbar">
        <div class="nav-brand">🚀 BTicino Bridge {{VERSION}}</div>
        <div class="nav-links">
            <a href="/dashboard">Dashboard</a>
            <a href="/messages" class="active">Messages</a>
            <a href="/controls">Controls</a>
            <a href="/settings">Settings</a>
            <a href="/logs">Logs</a>
        </div>
    </nav>

    <div class="container">
        <h1>📱 Answering Machine Messages</h1>
        
        <div class="messages-header">
            <div class="messages-stats">
                <div class="stat-item">
                    <span class="stat-number" id="msg-total">0</span>
                    <span class="stat-label">Total Messages</span>
                </div>
                <div class="stat-item">
                    <span class="stat-number" id="msg-unread">0</span>
                    <span class="stat-label">Unread Messages</span>
                </div>
                <div class="stat-item">
                    <span class="stat-number" id="msg-storage">0MB</span>
                    <span class="stat-label">Storage Used</span>
                </div>
            </div>
        </div>

        <div class="messages-controls">
            <button class="action-btn primary" onclick="refreshMessages()">
                🔄 <span id="refresh-text">Refresh</span>
            </button>
            
            <div class="filter-controls">
                <label for="message-filter">Filter:</label>
                <select id="message-filter" onchange="applyFilters()">
                    <option value="all">All Messages</option>
                    <option value="unread">Unread Only</option>
                    <option value="read">Read Only</option>
                </select>
                
                <label for="per-page">Per page:</label>
                <select id="per-page" onchange="applyFilters()">
                    <option value="5">5</option>
                    <option value="10" selected>10</option>
                    <option value="20">20</option>
                    <option value="50">50</option>
                </select>
            </div>
        </div>

        <div class="bulk-actions" id="bulk-actions" style="display: none;">
            <button onclick="markAllAsRead()">📧 Mark All as Read</button>
            <button onclick="deleteSelected()" style="background: #dc3545; color: white;">🗑️ Delete Selected</button>
            <button onclick="clearSelection()">❌ Clear Selection</button>
            <span id="selection-count" style="margin-left: auto; color: #666;"></span>
        </div>

        <div class="messages-container">
            <div id="messages-list">
                <div style="text-align: center; padding: 2rem; color: #666;">
                    <div class="loading-spinner"></div>
                    <p>Loading messages...</p>
                </div>
            </div>
        </div>

        <div class="pagination" id="pagination">
            <button onclick="goToPage(currentPage - 1)" id="prev-btn">← Previous</button>
            <div class="page-info" id="page-info">Page 1 of 1</div>
            <button onclick="goToPage(currentPage + 1)" id="next-btn">Next →</button>
        </div>

        <div class="filesystem-info">
            <h2>🗂️ Real Device Integration</h2>
            <div class="info-grid">
                <div class="info-item">
                    <strong>Messages Directory:</strong>
                    <code>/home/bticino/cfg/extra/47/messages/</code>
                </div>
                <div class="info-item">
                    <strong>File Structure:</strong>
                    <code>msg_info.ini, aswm.avi, aswm.jpg</code>
                </div>
                <div class="info-item">
                    <strong>API Endpoints:</strong>
                    <code>/api/messages/list, /api/messages/{id}</code>
                </div>
            </div>
        </div>
    </div>

    <!-- Message Detail Modal -->
    <div class="message-detail-modal" id="message-modal">
        <div class="modal-content">
            <div class="modal-header">
                <h3>📱 Message Details</h3>
                <button class="modal-close" onclick="closeMessageModal()">×</button>
            </div>
            <div id="modal-body">
                <!-- Message details will be loaded here -->
            </div>
        </div>
    </div>

    <script src="/assets/app.js"></script>
    <script>
        // Enhanced Messages Page JavaScript
        let currentPage = 1;
        let totalPages = 1;
        let selectedMessages = new Set();
        let allMessages = [];

        document.addEventListener('DOMContentLoaded', function() {
            refreshMessages();
        });

        async function refreshMessages() {
            const refreshBtn = document.getElementById('refresh-text');
            refreshBtn.textContent = 'Refreshing...';
            
            try {
                await loadMessages();
                refreshBtn.textContent = 'Refresh';
            } catch (error) {
                refreshBtn.textContent = 'Refresh';
                console.error('Failed to refresh messages:', error);
            }
        }

        async function loadMessages() {
            try {
                const filter = document.getElementById('message-filter').value;
                const perPage = document.getElementById('per-page').value;
                
                const url = '/api/messages/list?page=' + currentPage + 
                           '&limit=' + perPage + 
                           (filter !== 'all' ? '&unread_only=' + (filter === 'unread') : '');
                
                const response = await fetch(url);
                const data = await response.json();
                
                if (data.messages) {
                    allMessages = data.messages;
                    displayMessages(data.messages);
                    updatePagination(data.pagination);
                    updateStats(data.pagination.total, data.messages);
                } else {
                    throw new Error('Invalid response format');
                }
            } catch (error) {
                console.error('Failed to load messages:', error);
                document.getElementById('messages-list').innerHTML = 
                    '<div style="text-align: center; padding: 2rem; color: #dc3545;">❌ Failed to load messages</div>';
            }
        }

        function displayMessages(messages) {
            const container = document.getElementById('messages-list');
            container.innerHTML = '';

            if (messages.length === 0) {
                container.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666;">📭 No messages found</div>';
                return;
            }

            messages.forEach(message => {
                const messageDiv = document.createElement('div');
                messageDiv.className = 'message-item ' + (message.read ? '' : 'unread');
                messageDiv.dataset.messageId = message.id;
                
                messageDiv.innerHTML = 
                    '<div class="message-header">' +
                        '<div>' +
                            '<strong>' + (message.caller_id || 'Unknown Caller') + '</strong>' +
                            '<div class="message-info">' +
                                '<span>📅 ' + (message.message || 'No timestamp') + '</span>' +
                                '<span>' + (message.has_image ? '📸' : '') + (message.has_video ? '🎥' : '') + '</span>' +
                                '<span class="' + (message.read ? 'text-success' : 'text-primary') + '">' +
                                    (message.read ? '✅ Read' : '📬 Unread') +
                                '</span>' +
                            '</div>' +
                        '</div>' +
                        '<input type="checkbox" onchange="toggleMessageSelection(' + message.id + ')" style="margin-left: 1rem;">' +
                    '</div>' +
                    '<div class="message-actions">' +
                        '<button class="btn-view" onclick="viewMessage(' + message.id + ')">👁️ View</button>' +
                        (!message.read ? '<button class="btn-read" onclick="markAsRead(' + message.id + ')">✅ Mark Read</button>' : '') +
                        (message.has_image ? '<button class="btn-download" onclick="downloadFile(' + message.id + ', \'image\')">📸 Image</button>' : '') +
                        (message.has_video ? '<button class="btn-download" onclick="downloadFile(' + message.id + ', \'video\')">🎥 Video</button>' : '') +
                        '<button class="btn-delete" onclick="deleteMessage(' + message.id + ')">🗑️ Delete</button>' +
                    '</div>';
                
                container.appendChild(messageDiv);
            });
        }

        function updatePagination(pagination) {
            currentPage = pagination.page;
            totalPages = pagination.total_pages;
            
            document.getElementById('prev-btn').disabled = !pagination.has_previous;
            document.getElementById('next-btn').disabled = !pagination.has_next;
            document.getElementById('page-info').textContent = 
                'Page ' + pagination.page + ' of ' + pagination.total_pages + ' (' + pagination.total + ' total)';
        }

        function updateStats(total, messages) {
            const unreadCount = messages.filter(m => !m.read).length;
            const storageMB = Math.round(total * 5.2); // Rough estimate: 5MB per message
            
            document.getElementById('msg-total').textContent = total;
            document.getElementById('msg-unread').textContent = unreadCount;
            document.getElementById('msg-storage').textContent = storageMB + 'MB';
        }

        function goToPage(page) {
            if (page < 1 || page > totalPages) return;
            currentPage = page;
            loadMessages();
        }

        function applyFilters() {
            currentPage = 1;
            selectedMessages.clear();
            updateBulkActions();
            loadMessages();
        }

        function toggleMessageSelection(messageId) {
            if (selectedMessages.has(messageId)) {
                selectedMessages.delete(messageId);
            } else {
                selectedMessages.add(messageId);
            }
            updateBulkActions();
        }

        function updateBulkActions() {
            const bulkActions = document.getElementById('bulk-actions');
            const selectionCount = document.getElementById('selection-count');
            
            if (selectedMessages.size > 0) {
                bulkActions.style.display = 'flex';
                selectionCount.textContent = selectedMessages.size + ' selected';
            } else {
                bulkActions.style.display = 'none';
            }
        }

        function clearSelection() {
            selectedMessages.clear();
            document.querySelectorAll('.message-item input[type="checkbox"]').forEach(cb => cb.checked = false);
            updateBulkActions();
        }

        async function viewMessage(messageId) {
            try {
                const response = await fetch('/api/messages/' + messageId);
                const message = await response.json();
                
                showMessageModal(message);
            } catch (error) {
                console.error('Failed to load message details:', error);
                alert('Failed to load message details');
            }
        }

        function showMessageModal(message) {
            const modal = document.getElementById('message-modal');
            const modalBody = document.getElementById('modal-body');
            
            modalBody.innerHTML = 
                '<div class="message-detail">' +
                    '<h4>📞 ' + (message.caller_id || 'Unknown Caller') + '</h4>' +
                    '<p><strong>Timestamp:</strong> ' + (message.message || 'No timestamp') + '</p>' +
                    '<p><strong>Status:</strong> ' + (message.read ? '✅ Read' : '📬 Unread') + '</p>' +
                    '<p><strong>Files:</strong> ' + 
                        (message.has_image ? '📸 Image ' : '') + 
                        (message.has_video ? '🎥 Video' : '') + 
                    '</p>' +
                    (message.image_base64 ? 
                        '<div class="message-media"><img src="data:image/jpeg;base64,' + message.image_base64 + '" alt="Message Image"></div>' : 
                        '') +
                    '<div style="margin-top: 1rem;">' +
                        (!message.read ? '<button class="btn-read" onclick="markAsRead(' + message.id + '); closeMessageModal();">✅ Mark as Read</button> ' : '') +
                        (message.has_image ? '<button class="btn-download" onclick="downloadFile(' + message.id + ', \'image\')">📸 Download Image</button> ' : '') +
                        (message.has_video ? '<button class="btn-download" onclick="downloadFile(' + message.id + ', \'video\')">🎥 Download Video</button> ' : '') +
                        '<button class="btn-delete" onclick="deleteMessage(' + message.id + '); closeMessageModal();" style="margin-left: 1rem;">🗑️ Delete</button>' +
                    '</div>' +
                '</div>';
            
            modal.style.display = 'block';
        }

        function closeMessageModal() {
            document.getElementById('message-modal').style.display = 'none';
        }

        // Click outside modal to close
        document.getElementById('message-modal').addEventListener('click', function(e) {
            if (e.target === this) {
                closeMessageModal();
            }
        });

        async function markAsRead(messageId) {
            try {
                const response = await fetch('/api/messages/mark-read/' + messageId, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ read: true })
                });
                
                if (response.ok) {
                    await loadMessages();
                } else {
                    alert('Failed to mark message as read');
                }
            } catch (error) {
                console.error('Failed to mark message as read:', error);
                alert('Failed to mark message as read');
            }
        }

        async function deleteMessage(messageId) {
            if (!confirm('Are you sure you want to delete this message?')) return;
            
            try {
                const response = await fetch('/api/messages/delete/' + messageId, {
                    method: 'DELETE'
                });
                
                if (response.ok) {
                    await loadMessages();
                } else {
                    alert('Failed to delete message');
                }
            } catch (error) {
                console.error('Failed to delete message:', error);
                alert('Failed to delete message');
            }
        }

        async function markAllAsRead() {
            if (!confirm('Mark all messages as read?')) return;
            
            const unreadMessages = allMessages.filter(m => !m.read);
            
            for (const message of unreadMessages) {
                try {
                    await fetch('/api/messages/mark-read/' + message.id, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ read: true })
                    });
                } catch (error) {
                    console.error('Failed to mark message as read:', error);
                }
            }
            
            await loadMessages();
        }

        async function deleteSelected() {
            if (selectedMessages.size === 0) return;
            if (!confirm('Delete ' + selectedMessages.size + ' selected messages?')) return;
            
            for (const messageId of selectedMessages) {
                try {
                    await fetch('/api/messages/delete/' + messageId, {
                        method: 'DELETE'
                    });
                } catch (error) {
                    console.error('Failed to delete message:', error);
                }
            }
            
            selectedMessages.clear();
            updateBulkActions();
            await loadMessages();
        }

        async function downloadFile(messageId, type) {
            try {
                const response = await fetch('/api/messages/download/' + messageId + '/' + type);
                
                if (response.ok) {
                    const blob = await response.blob();
                    const url = window.URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = 'message_' + messageId + '_' + type + (type === 'image' ? '.jpg' : '.avi');
                    document.body.appendChild(a);
                    a.click();
                    window.URL.revokeObjectURL(url);
                    document.body.removeChild(a);
                } else {
                    alert('Failed to download file');
                }
            } catch (error) {
                console.error('Failed to download file:', error);
                alert('Failed to download file');
            }
        }
    </script>
</body>
</html>`
}

func (ws *WebServer) getControlsHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>BTicino Bridge {{VERSION}} - Controls</title>
    <link rel="stylesheet" href="/assets/style.css">
</head>
<body>
    <nav class="navbar">
        <div class="nav-brand">🚀 BTicino Bridge {{VERSION}}</div>
        <div class="nav-links">
            <a href="/dashboard">Dashboard</a>
            <a href="/messages">Messages</a>
            <a href="/controls" class="active">Controls</a>
            <a href="/settings">Settings</a>
            <a href="/logs">Logs</a>
        </div>
    </nav>

    <div class="container">
        <h1>Device Controls</h1>
        
        <div class="controls-grid">
            <div class="control-section">
                <h2>Door Controls</h2>
                <div class="control-buttons">
                    <button class="control-btn primary" onclick="unlockDoor()">
                        <span class="btn-icon">&#128275;</span>
                        <div>
                            <strong>Unlock Door</strong>
                            <small>Press + Release</small>
                        </div>
                    </button>
                    <button class="control-btn info" onclick="staircaseLight()">
                        <span class="btn-icon">&#128161;</span>
                        <div>
                            <strong>Staircase Light</strong>
                            <small>Command: *8*21*16##</small>
                        </div>
                    </button>
                </div>
            </div>

            <div class="control-section">
                <h2>Answering Machine</h2>
                <div class="control-buttons">
                    <button class="control-btn success" onclick="enableAnsweringMachine()">
                        <span class="btn-icon">&#9989;</span>
                        <div>
                            <strong>Enable</strong>
                            <small>Command: *8*91##</small>
                        </div>
                    </button>
                    <button class="control-btn warning" onclick="disableAnsweringMachine()">
                        <span class="btn-icon">&#10060;</span>
                        <div>
                            <strong>Disable</strong>
                            <small>Command: *8*92##</small>
                        </div>
                    </button>
                </div>
            </div>

            <div class="control-section">
                <h2>Display</h2>
                <div class="control-buttons">
                    <button class="control-btn success" onclick="activateDisplay()">
                        <span class="btn-icon">&#128250;</span>
                        <div>
                            <strong>Display ON</strong>
                            <small>Command: *7*73#1#100*##</small>
                        </div>
                    </button>
                    <button class="control-btn warning" onclick="deactivateDisplay()">
                        <span class="btn-icon">&#128260;</span>
                        <div>
                            <strong>Display OFF</strong>
                            <small>Command: *7*73#1#10*##</small>
                        </div>
                    </button>
                </div>
            </div>

            <div class="control-section">
                <h2>Audio Controls</h2>
                <div class="control-buttons">
                    <button class="control-btn warning" onclick="muteOn()">
                        <span class="btn-icon">&#128263;</span>
                        <div>
                            <strong>Mute ON</strong>
                            <small>Command: *8*30*20##</small>
                        </div>
                    </button>
                    <button class="control-btn success" onclick="muteOff()">
                        <span class="btn-icon">&#128266;</span>
                        <div>
                            <strong>Mute OFF</strong>
                            <small>Command: *8*31*20##</small>
                        </div>
                    </button>
                </div>
            </div>

            <div class="control-section">
                <h2>Doorbell Sound</h2>
                <div class="control-buttons">
                    <button class="control-btn success" onclick="doorbellSoundOn()">
                        <span class="btn-icon">&#128276;</span>
                        <div>
                            <strong>Doorbell ON</strong>
                            <small>Command: *#8**33*1##</small>
                        </div>
                    </button>
                    <button class="control-btn warning" onclick="doorbellSoundOff()">
                        <span class="btn-icon">&#128277;</span>
                        <div>
                            <strong>Doorbell OFF</strong>
                            <small>Command: *#8**33*0##</small>
                        </div>
                    </button>
                </div>
            </div>

            <div class="control-section">
                <h2>Send Command</h2>
                <div class="control-buttons" style="flex-direction: column;">
                    <input type="text" id="arbitrary-command-input" placeholder="e.g. *8*19*20##" style="padding: 10px; border: 1px solid #ccc; border-radius: 6px; font-family: monospace; font-size: 14px; width: 100%; box-sizing: border-box;">
                    <button class="control-btn info" onclick="sendArbitraryCommand()" style="width: 100%;">
                        <span class="btn-icon">&#9881;</span>
                        <div>
                            <strong>Send OWN Command</strong>
                            <small>Advanced: send any OpenWebNet command</small>
                        </div>
                    </button>
                </div>
            </div>
        </div>


        <div class="command-log">
            <h2>📜 Command Log</h2>
            <div id="command-log-list">
                <!-- Command history will be displayed here -->
            </div>
        </div>
    </div>

    <script src="/assets/app.js"></script>
</body>
</html>`
}

func (ws *WebServer) getSettingsHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>BTicino Bridge {{VERSION}} - Settings</title>
    <link rel="stylesheet" href="/assets/style.css">
    <link rel="stylesheet" href="/assets/config_ui.css">
</head>
<body>
    <nav class="navbar">
        <div class="nav-brand">🚀 BTicino Bridge {{VERSION}}</div>
        <div class="nav-links">
            <a href="/dashboard">Dashboard</a>
            <a href="/messages">Messages</a>
            <a href="/controls">Controls</a>
            <a href="/settings" class="active">Settings</a>
            <a href="/logs">Logs</a>
        </div>
    </nav>

    <div class="loading-overlay">
        <div class="loading-spinner"></div>
    </div>

    <div class="container">
        <h1>⚙️ Configuration Settings</h1>
        
        <div class="config-page">
            <div class="config-header">
                <h2>Settings - bticino_bridge {{VERSION}}</h2>
                <div class="config-actions">
                    <button class="btn btn-warning" onclick="validateConfig()">✓ Validate</button>
                    <button class="btn btn-secondary" onclick="createBackup()">📦 Backup</button>
                    <button class="btn btn-secondary" onclick="loadBackups()">📂 Backups</button>
                    <button class="btn btn-secondary" onclick="loadHistory()">📜 History</button>
                    <button class="btn btn-secondary" onclick="reloadConfig()">🔄 Reload</button>
                    <button class="btn btn-secondary" id="save-config-btn" onclick="saveConfig()" disabled>💾 Save</button>
                </div>
            </div>
            
            <div id="validation-result" class="validation-result"></div>
            
            <div class="tabs">
                <button class="tab active" data-tab="bridge" onclick="switchTab('bridge')">Bridge</button>
                <button class="tab" data-tab="device" onclick="switchTab('device')">Device</button>
                <button class="tab" data-tab="openwebnet" onclick="switchTab('openwebnet')">OpenWebNet</button>
                <button class="tab" data-tab="sip" onclick="switchTab('sip')">SIP</button>
                <button class="tab" data-tab="mqtt" onclick="switchTab('mqtt')">MQTT</button>
                <button class="tab" data-tab="homekit" onclick="switchTab('homekit')">HomeKit</button>
                <button class="tab" data-tab="hardware" onclick="switchTab('hardware')">Hardware</button>
                <button class="tab" data-tab="streaming" onclick="switchTab('streaming')">Streaming</button>
                <button class="tab" data-tab="audio" onclick="switchTab('audio')">Audio</button>
                <button class="tab" data-tab="display" onclick="switchTab('display')">Display</button>
                <button class="tab" data-tab="privacy" onclick="switchTab('privacy')">Privacy</button>
                <button class="tab" data-tab="security" onclick="switchTab('security')">Security</button>
            </div>
            
            <div id="tab-bridge" class="tab-content active">
                <div class="config-section">
                    <h3><span class="icon">🌉</span> Bridge Settings</h3>
                    <div class="config-grid">
                        <div class="config-field">
                            <label>Bridge Name</label>
                            <input type="text" id="bridge_name" placeholder="My BTicino Bridge">
                        </div>
                        <div class="config-field">
                            <label>Version</label>
                            <input type="text" id="bridge_version" readonly>
                        </div>
                        <div class="config-field">
                            <label>Log Level</label>
                            <select id="log_level">
                                <option value="debug">Debug</option>
                                <option value="info">Info</option>
                                <option value="warn">Warning</option>
                                <option value="error">Error</option>
                            </select>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Device System Tab (NEW - Fase 2) -->
            <div id="tab-device" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">📱</span> Device System <button class="btn btn-small" onclick="loadDeviceConfig()">↻ Refresh</button></h3>
                    <div class="config-grid" id="device-info-content">
                        <div class="config-grid">
                            <div class="config-field">
                                <label>Language</label>
                                <input type="text" id="device_language" readonly>
                            </div>
                            <div class="config-field">
                                <label>Timezone</label>
                                <input type="text" id="device_timezone" readonly>
                            </div>
                            <div class="config-field">
                                <label>NTP Server</label>
                                <input type="text" id="device_ntp_server" readonly>
                            </div>
                            <div class="config-field">
                                <label>NTP Mode</label>
                                <input type="text" id="device_ntp_algo" readonly>
                            </div>
                            <div class="config-field">
                                <label>Model</label>
                                <input type="text" id="device_model" readonly>
                            </div>
                            <div class="config-field">
                                <label>IP Address</label>
                                <input type="text" id="device_ip" readonly>
                            </div>
                            <div class="config-field">
                                <label>Firmware Version</label>
                                <input type="text" id="device_firmware" readonly>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="config-section">
                    <h3><span class="icon">📅</span> Date & Time</h3>
                    <div id="datetime-display" style="text-align: center; padding: 15px; font-size: 1.2rem;">
                        <span id="current-datetime">Loading...</span>
                    </div>
                </div>
                <div class="config-section">
                    <h3><span class="icon">📞</span> Answering Machine <button class="btn btn-small" onclick="loadDeviceConfig()">↻</button></h3>
                    <div class="config-grid" id="answering-info-content"></div>
                </div>
            </div>

            <!-- Audio & Ringtone Tab (NEW - Fase 2) -->
            <div id="tab-audio" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">🔔</span> Ringtones <button class="btn btn-small" onclick="loadRingtones(); loadVolumes();">↻</button></h3>
                    <div class="config-grid" id="ringtones-content"></div>
                </div>
                <div class="config-section">
                    <h3><span class="icon">🔊</span> Volumes</h3>
                    <div class="config-grid" id="volumes-content"></div>
                </div>
            </div>

            <!-- Display Tab (NEW - Fase 2) -->
            <div id="tab-display" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">💡</span> Display <button class="btn btn-small" onclick="loadDisplayConfig(); loadCameras();">↻</button></h3>
                    <div class="config-grid" id="display-content"></div>
                </div>
                <div class="config-section">
                    <h3><span class="icon">📹</span> Cameras</h3>
                    <div id="cameras-content"></div>
                </div>
            </div>
            
            <div id="tab-openwebnet" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">🔌</span> OpenWebNet Connection</h3>
                    <div class="config-grid">
                        <div class="config-field">
                            <label>Host</label>
                            <input type="text" id="own_host" placeholder="192.168.1.38">
                        </div>
                        <div class="config-field">
                            <label>Port</label>
                            <input type="number" id="own_port" min="1" max="65535" value="20000">
                        </div>
                        <div class="config-field">
                            <label>Timeout (ms)</label>
                            <input type="number" id="own_timeout" min="1000" max="30000" value="5000">
                        </div>
                        <div class="config-field">
                            <label>Retry Attempts</label>
                            <input type="number" id="own_retry_attempts" min="0" max="10" value="3">
                        </div>
                    </div>
                </div>
            </div>
            
            <div id="tab-sip" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">📞</span> SIP Configuration</h3>
                    <div class="config-grid">
                        <div class="config-field">
                            <label>SIP Enabled</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="sip_enabled" onclick="this.classList.toggle('active'); document.getElementById('sip_enabled_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="sip_enabled_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>Server Host</label>
                            <input type="text" id="sip_server_host" placeholder="sip.example.com">
                        </div>
                        <div class="config-field">
                            <label>Server Port</label>
                            <input type="number" id="sip_server_port" min="1" max="65535" value="5060">
                        </div>
                        <div class="config-field">
                            <label>Username</label>
                            <input type="text" id="sip_username" placeholder="username">
                        </div>
                        <div class="config-field">
                            <label>Password</label>
                            <input type="password" id="sip_password" placeholder="password">
                        </div>
                        <div class="config-field">
                            <label>Domain</label>
                            <input type="text" id="sip_domain" placeholder="example.com">
                        </div>
                        <div class="config-field">
                            <label>Transport</label>
                            <select id="sip_transport">
                                <option value="udp">UDP</option>
                                <option value="tcp">TCP</option>
                                <option value="tls">TLS</option>
                            </select>
                        </div>
                    </div>
                </div>
            </div>
            
            <div id="tab-mqtt" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">📡</span> MQTT Configuration</h3>
                    <div class="config-grid">
                        <div class="config-field">
                            <label>MQTT Enabled</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="mqtt_enabled" onclick="this.classList.toggle('active'); document.getElementById('mqtt_enabled_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="mqtt_enabled_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>Host</label>
                            <input type="text" id="mqtt_host" placeholder="localhost">
                        </div>
                        <div class="config-field">
                            <label>Port</label>
                            <input type="number" id="mqtt_port" min="1" max="65535" value="1883">
                        </div>
                        <div class="config-field">
                            <label>Client ID</label>
                            <input type="text" id="mqtt_client_id" placeholder="bticino_bridge">
                        </div>
                        <div class="config-field">
                            <label>Username</label>
                            <input type="text" id="mqtt_username" placeholder="username">
                        </div>
                        <div class="config-field">
                            <label>Password</label>
                            <input type="password" id="mqtt_password" placeholder="password">
                        </div>
                        <div class="config-field">
                            <label>Topic Prefix</label>
                            <input type="text" id="mqtt_topic_prefix" placeholder="homeassistant">
                        </div>
                    </div>
                </div>
            </div>
            
            <div id="tab-homekit" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">🏠</span> HomeKit Configuration</h3>
                    <div class="config-grid">
                        <div class="config-field">
                            <label>HomeKit Enabled</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="homekit_enabled" onclick="this.classList.toggle('active'); document.getElementById('homekit_enabled_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="homekit_enabled_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>Bridge Name</label>
                            <input type="text" id="homekit_name" placeholder="BTicino Bridge">
                        </div>
                        <div class="config-field">
                            <label>PIN (8 digits)</label>
                            <input type="text" id="homekit_pin" placeholder="12345678" maxlength="8">
                        </div>
                    </div>
                </div>
            </div>
            
            <div id="tab-hardware" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">🔧</span> Hardware Configuration</h3>
                    <div class="config-grid">
                        <div class="config-field">
                            <label>Hardware Monitoring</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="hardware_enabled" onclick="this.classList.toggle('active'); document.getElementById('hardware_enabled_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="hardware_enabled_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>GPIO Monitoring</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="hardware_gpio" onclick="this.classList.toggle('active'); document.getElementById('hardware_gpio_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="hardware_gpio_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>Input Device</label>
                            <input type="text" id="input_device" placeholder="/dev/input/event0">
                        </div>
                    </div>
                </div>
            </div>
            
            <div id="tab-streaming" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">📹</span> Streaming Configuration</h3>
                    <div class="config-grid">
                        <div class="config-field">
                            <label>Streaming Enabled</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="streaming_enabled" onclick="this.classList.toggle('active'); document.getElementById('streaming_enabled_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="streaming_enabled_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>RTSP Port</label>
                            <input type="number" id="rtsp_port" min="1" max="65535" value="8554">
                        </div>
                        <div class="config-field">
                            <label>WebRTC Port</label>
                            <input type="number" id="webrtc_port" min="1" max="65535" value="8888">
                        </div>
                    </div>
                </div>
            </div>
            
            <div id="tab-privacy" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">🔒</span> Privacy Settings</h3>
                    <div class="config-grid">
                        <div class="config-field">
                            <label>Block External Telemetry</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="privacy_block_telemetry" onclick="this.classList.toggle('active'); document.getElementById('privacy_block_telemetry_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="privacy_block_telemetry_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>Block Cloud</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="privacy_block_cloud" onclick="this.classList.toggle('active'); document.getElementById('privacy_block_cloud_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="privacy_block_cloud_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>Local Logging</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="privacy_local_logging" onclick="this.classList.toggle('active'); document.getElementById('privacy_local_logging_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="privacy_local_logging_hidden" value="true">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>Disable Auto Updates</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="privacy_disable_updates" onclick="this.classList.toggle('active'); document.getElementById('privacy_disable_updates_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="privacy_disable_updates_hidden" value="false">
                            </div>
                        </div>
                    </div>
                </div>
            </div>
            
            <div id="tab-security" class="tab-content">
                <div class="config-section">
                    <h3><span class="icon">🛡️</span> Security Settings</h3>
                    <div class="config-grid">
                        <div class="config-field">
                            <label>Web Authentication Required</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="security_web_auth" onclick="this.classList.toggle('active'); document.getElementById('security_web_auth_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="security_web_auth_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>HTTPS Enabled</label>
                            <div class="toggle-wrapper">
                                <div class="toggle" id="security_https" onclick="this.classList.toggle('active'); document.getElementById('security_https_hidden').value = this.classList.contains('active'); configChanged=true;"></div>
                                <input type="hidden" id="security_https_hidden" value="false">
                            </div>
                        </div>
                        <div class="config-field">
                            <label>API Rate Limit (req/min)</label>
                            <input type="number" id="api_rate_limit" min="1" max="1000" value="100">
                        </div>
                    </div>
                </div>
            </div>
            
            <div class="config-section" style="margin-top: 1.5rem;">
                <h3><span class="icon">📦</span> Backup Management</h3>
                <div id="backup-list" class="backup-list">
                    <p style="color: #6c757d;">Click "Backups" to load available backups</p>
                </div>
            </div>
            
            <div class="config-section" style="margin-top: 1.5rem;">
                <h3><span class="icon">📜</span> Configuration History</h3>
                <div id="history-list" class="history-list">
                    <p style="color: #6c757d;">Click "History" to load change history</p>
                </div>
            </div>
        </div>
    </div>

    <script src="/assets/config_ui.js"></script>
</body>
</html>`
}

// getLogsHTML returns the HTML for the log viewer page
func (ws *WebServer) getLogsHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>BTicino Bridge {{VERSION}} - Logs</title>
    <link rel="stylesheet" href="/assets/style.css">
    <style>
        .log-controls {
            display: flex;
            gap: 1rem;
            margin-bottom: 1rem;
            flex-wrap: wrap;
            align-items: center;
        }
        .log-controls select, .log-controls input {
            padding: 0.4rem;
            border: 1px solid #ddd;
            border-radius: 4px;
            font-size: 0.9rem;
        }
        .log-controls button {
            padding: 0.4rem 1rem;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 0.9rem;
            background: #667eea;
            color: white;
        }
        .log-controls button:hover {
            background: #5a6fd6;
        }
        .log-viewer {
            background: #1e1e2e;
            color: #cdd6f4;
            border-radius: 8px;
            padding: 1rem;
            font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
            font-size: 0.82rem;
            max-height: 70vh;
            overflow-y: auto;
            line-height: 1.5;
        }
        .log-entry {
            padding: 2px 0;
            border-bottom: 1px solid rgba(255,255,255,0.05);
            word-break: break-all;
        }
        .log-entry .log-time {
            color: #89b4fa;
        }
        .log-entry .log-level-info {
            color: #a6e3a1;
            font-weight: bold;
        }
        .log-entry .log-level-warning {
            color: #f9e2af;
            font-weight: bold;
        }
        .log-entry .log-level-error {
            color: #f38ba8;
            font-weight: bold;
        }
        .log-entry .log-level-debug {
            color: #9399b2;
            font-weight: bold;
        }
        .log-entry .log-msg {
            color: #cdd6f4;
        }
        .log-entry .log-fields {
            color: #9399b2;
            font-size: 0.78rem;
        }
        .log-stats {
            display: flex;
            gap: 1rem;
            margin-top: 0.5rem;
            font-size: 0.85rem;
            color: #666;
        }
        #auto-scroll-label {
            display: flex;
            align-items: center;
            gap: 0.3rem;
            font-size: 0.9rem;
        }
    </style>
</head>
<body>
    <nav class="navbar">
        <div class="nav-brand">&#128640; BTicino Bridge {{VERSION}}</div>
        <div class="nav-links">
            <a href="/dashboard">Dashboard</a>
            <a href="/messages">Messages</a>
            <a href="/controls">Controls</a>
            <a href="/settings">Settings</a>
            <a href="/logs" class="active">Logs</a>
        </div>
    </nav>

    <div class="container">
        <h1>&#128196; BTicino Bridge Logs</h1>

        <div class="log-controls">
            <label>Level:
                <select id="level-filter" onchange="refreshLogs()">
                    <option value="all">All</option>
                    <option value="error">Error</option>
                    <option value="warning">Warning</option>
                    <option value="info" selected>Info</option>
                    <option value="debug">Debug</option>
                </select>
            </label>
            <label>Count:
                <select id="count-select" onchange="refreshLogs()">
                    <option value="50">50</option>
                    <option value="100">100</option>
                    <option value="200" selected>200</option>
                    <option value="500">500</option>
                </select>
            </label>
            <label id="auto-scroll-label">
                <input type="checkbox" id="auto-scroll" checked> Auto-scroll
            </label>
            <button onclick="refreshLogs()">&#128259; Refresh</button>
            <button onclick="downloadLogs(5000)">&#128230; Download Logs</button>
            <label id="auto-scroll-label">
                <input type="checkbox" id="auto-refresh" checked onchange="toggleAutoRefresh()"> Auto-refresh (5s)
            </label>
        </div>

        <div class="log-viewer" id="log-viewer">
            <div id="log-entries">Loading logs...</div>
        </div>
        <div class="log-stats">
            <span id="log-count">-</span>
            <span id="log-updated">-</span>
        </div>
    </div>

    <script>
    var autoRefreshTimer = null;

    function getLevelClass(level) {
        if (level === 'info') return 'log-level-info';
        if (level === 'warning') return 'log-level-warning';
        if (level === 'error') return 'log-level-error';
        if (level === 'debug') return 'log-level-debug';
        return 'log-level-info';
    }

    function escapeHtml(text) {
        var d = document.createElement('div');
        d.textContent = text;
        return d.innerHTML;
    }

    function renderLogEntry(entry) {
        var time = entry.timestamp ? entry.timestamp.substring(11, 19) : '--:--:--';
        var lvl = (entry.level || 'info').toUpperCase();
        var cls = getLevelClass(entry.level || 'info');
        var msg = escapeHtml(entry.message || '');
        var fields = entry.fields ? ' <span class="log-fields">' + escapeHtml(entry.fields) + '</span>' : '';
        return '<div class="log-entry"><span class="log-time">' + time + '</span> <span class="' + cls + '">[' + lvl + ']</span> <span class="log-msg">' + msg + '</span>' + fields + '</div>';
    }

    function refreshLogs() {
        var level = document.getElementById('level-filter').value;
        var count = document.getElementById('count-select').value;
        var url = '/api/logs?count=' + count;
        if (level !== 'all') {
            url += '&level=' + level;
        }
        fetch(url)
            .then(function(r) { return r.json(); })
            .then(function(data) {
                var entries = data.logs || [];
                var html = '';
                for (var i = 0; i < entries.length; i++) {
                    html += renderLogEntry(entries[i]);
                }
                if (entries.length === 0) {
                    html = '<div class="log-entry">No log entries matching filter.</div>';
                }
                document.getElementById('log-entries').innerHTML = html;
                document.getElementById('log-count').textContent = 'Showing ' + entries.length + ' / ' + data.max_buffer + ' entries';
                document.getElementById('log-updated').textContent = 'Updated: ' + new Date().toLocaleTimeString();
                if (document.getElementById('auto-scroll').checked) {
                    var viewer = document.getElementById('log-viewer');
                    viewer.scrollTop = viewer.scrollHeight;
                }
            })
            .catch(function(err) {
                document.getElementById('log-entries').innerHTML = '<div class="log-entry" style="color:#f38ba8;">Error loading logs: ' + err + '</div>';
            });
    }

    function toggleAutoRefresh() {
        if (document.getElementById('auto-refresh').checked) {
            autoRefreshTimer = setInterval(refreshLogs, 5000);
        } else {
            if (autoRefreshTimer) clearInterval(autoRefreshTimer);
            autoRefreshTimer = null;
        }
    }

    function downloadLogs(count) {
        window.location.href = '/api/logs/download?count=' + count;
    }

    // Initial load
    refreshLogs();
    autoRefreshTimer = setInterval(refreshLogs, 5000);
    </script>
</body>
</html>`
}

// getConfigCSS returns the CSS styles for the configuration page
func (ws *WebServer) getConfigCSS() string {
	return `/* Configuration UI Styles */
.config-page {
    background: white;
    border-radius: 16px;
    padding: 2rem;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
    margin-bottom: 2rem;
}

.config-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 2rem;
    padding-bottom: 1rem;
    border-bottom: 2px solid #e1e5e9;
}

.config-header h2 {
    margin: 0;
    color: #333;
}

.config-actions {
    display: flex;
    gap: 0.75rem;
}

.config-section {
    margin-bottom: 2rem;
    padding: 1.5rem;
    background: #f8f9fa;
    border-radius: 12px;
    border: 1px solid #e1e5e9;
}

.config-section h3 {
    margin: 0 0 1.25rem 0;
    color: #495057;
    font-size: 1.1rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.config-section h3 .icon {
    font-size: 1.25rem;
}

.config-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: 1rem;
}

/* Range input styling for brightness slider */
input[type="range"] {
    -webkit-appearance: none;
    width: 100%;
    height: 8px;
    border-radius: 4px;
    background: linear-gradient(90deg, #667eea, #764ba2);
    outline: none;
    opacity: 0.9;
}

input[type="range"]::-webkit-slider-thumb {
    -webkit-appearance: none;
    appearance: none;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: #fff;
    cursor: pointer;
    box-shadow: 0 2px 6px rgba(0,0,0,0.3);
}

input[type="range"][readonly] {
    background: #e0e0e0;
}

input[type="range"][readonly]::-webkit-slider-thumb {
    background: #999;
}

.config-field {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
}

.config-field label {
    font-size: 0.85rem;
    font-weight: 600;
    color: #6c757d;
    text-transform: uppercase;
    letter-spacing: 0.5px;
}

.config-field input,
.config-field select,
.config-field textarea {
    padding: 0.65rem 0.85rem;
    border: 1px solid #ced4da;
    border-radius: 6px;
    font-size: 0.95rem;
    transition: all 0.2s;
    background: white;
}

.config-field input:focus,
.config-field select:focus,
.config-field textarea:focus {
    outline: none;
    border-color: #667eea;
    box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.15);
}

.config-field input:read-only {
    background: #e9ecef;
    color: #6c757d;
}

.config-field .toggle-wrapper {
    display: flex;
    align-items: center;
    gap: 0.75rem;
}

.config-field .toggle {
    position: relative;
    width: 48px;
    height: 26px;
    background: #ced4da;
    border-radius: 13px;
    cursor: pointer;
    transition: background 0.3s;
}

.config-field .toggle.active {
    background: #10b981;
}

.config-field .toggle::after {
    content: '';
    position: absolute;
    top: 3px;
    left: 3px;
    width: 20px;
    height: 20px;
    background: white;
    border-radius: 50%;
    transition: transform 0.3s;
    box-shadow: 0 2px 4px rgba(0,0,0,0.2);
}

.config-field .toggle.active::after {
    transform: translateX(22px);
}

.config-field .toggle-label {
    font-size: 0.9rem;
    color: #495057;
}

.btn {
    padding: 0.65rem 1.25rem;
    border: none;
    border-radius: 6px;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
}

.btn-primary {
    background: #667eea;
    color: white;
}

.btn-primary:hover {
    background: #5a6fd6;
    transform: translateY(-1px);
}

.btn-secondary {
    background: #6c757d;
    color: white;
}

.btn-secondary:hover {
    background: #5a6268;
}

.btn-success {
    background: #10b981;
    color: white;
}

.btn-success:hover {
    background: #059669;
}

.btn-danger {
    background: #dc3545;
    color: white;
}

.btn-danger:hover {
    background: #c82333;
}

.btn-warning {
    background: #f59e0b;
    color: white;
}

.btn-warning:hover {
    background: #d97706;
}

.btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
}

.validation-result {
    margin-top: 1rem;
    padding: 1rem;
    border-radius: 8px;
    display: none;
}

.validation-result.success {
    display: block;
    background: #d1fae5;
    border: 1px solid #10b981;
    color: #065f46;
}

.validation-result.error {
    display: block;
    background: #fee2e2;
    border: 1px solid #dc3545;
    color: #991b1b;
}

.validation-result.warning {
    display: block;
    background: #fef3c7;
    border: 1px solid #f59e0b;
    color: #92400e;
}

.validation-result h4 {
    margin: 0 0 0.5rem 0;
    font-size: 1rem;
}

.validation-result ul {
    margin: 0;
    padding-left: 1.25rem;
}

.validation-result li {
    margin: 0.25rem 0;
}

.backup-list {
    margin-top: 1rem;
    max-height: 300px;
    overflow-y: auto;
}

.backup-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem;
    background: white;
    border: 1px solid #e1e5e9;
    border-radius: 6px;
    margin-bottom: 0.5rem;
}

.backup-item .backup-info {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
}

.backup-item .backup-date {
    font-weight: 600;
    color: #333;
}

.backup-item .backup-size {
    font-size: 0.85rem;
    color: #6c757d;
}

.backup-item .backup-actions {
    display: flex;
    gap: 0.5rem;
}

.history-list {
    margin-top: 1rem;
}

.history-item {
    padding: 0.75rem;
    background: white;
    border: 1px solid #e1e5e9;
    border-radius: 6px;
    margin-bottom: 0.5rem;
    display: flex;
    justify-content: space-between;
    align-items: center;
}

.history-item .history-info {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
}

.history-item .history-date {
    font-weight: 600;
    color: #333;
    font-size: 0.9rem;
}

.history-item .history-user {
    font-size: 0.85rem;
    color: #6c757d;
}

.history-item .history-changes {
    font-size: 0.85rem;
    color: #495057;
}

.loading-overlay {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.5);
    display: none;
    justify-content: center;
    align-items: center;
    z-index: 9999;
}

.loading-overlay.active {
    display: flex;
}

.loading-spinner {
    width: 50px;
    height: 50px;
    border: 4px solid rgba(255, 255, 255, 0.3);
    border-top: 4px solid #667eea;
    border-radius: 50%;
    animation: spin 1s linear infinite;
}

@keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
}

.toast {
    position: fixed;
    bottom: 20px;
    right: 20px;
    padding: 1rem 1.5rem;
    border-radius: 8px;
    color: white;
    font-weight: 500;
    z-index: 10000;
    transform: translateY(100px);
    opacity: 0;
    transition: all 0.3s;
}

.toast.show {
    transform: translateY(0);
    opacity: 1;
}

.toast.success {
    background: #10b981;
}

.toast.error {
    background: #dc3545;
}

.toast.warning {
    background: #f59e0b;
}

.tabs {
    display: flex;
    gap: 0.25rem;
    margin-bottom: 1.5rem;
    border-bottom: 2px solid #e1e5e9;
    padding-bottom: 0;
}

.tab {
    padding: 0.75rem 1.25rem;
    background: none;
    border: none;
    font-size: 0.95rem;
    font-weight: 500;
    color: #6c757d;
    cursor: pointer;
    border-bottom: 2px solid transparent;
    margin-bottom: -2px;
    transition: all 0.2s;
}

.tab:hover {
    color: #495057;
}

.tab.active {
    color: #667eea;
    border-bottom-color: #667eea;
}

.tab-content {
    display: none;
}

.tab-content.active {
    display: block;
}

@media (max-width: 768px) {
    .config-grid {
        grid-template-columns: 1fr;
    }
    
    .config-header {
        flex-direction: column;
        gap: 1rem;
    }
    
    .config-actions {
        flex-wrap: wrap;
        width: 100%;
    }
    
    .btn {
        flex: 1;
        justify-content: center;
    }
}`
}

func (ws *WebServer) getConfigJS() string {
	return `// Configuration UI JavaScript
let currentConfig = null;
let configChanged = false;

document.addEventListener('DOMContentLoaded', function() {
    loadConfig();
    setupEventListeners();
});

function setupEventListeners() {
    const inputs = document.querySelectorAll('.config-field input, .config-field select, .config-field textarea');
    inputs.forEach(input => {
        input.addEventListener('change', function() {
            configChanged = true;
            updateSaveButtonState();
        });
    });
    
    const toggles = document.querySelectorAll('.config-field .toggle');
    toggles.forEach(toggle => {
        toggle.addEventListener('click', function() {
            this.classList.toggle('active');
            const hiddenInput = this.parentElement.querySelector('input[type="hidden"]');
            if (hiddenInput) {
                hiddenInput.value = this.classList.contains('active') ? 'true' : 'false';
            }
            configChanged = true;
            updateSaveButtonState();
        });
    });
}

function updateSaveButtonState() {
    const saveBtn = document.getElementById('save-config-btn');
    if (saveBtn) {
        saveBtn.disabled = !configChanged;
        saveBtn.classList.toggle('btn-primary', configChanged);
        saveBtn.classList.toggle('btn-secondary', !configChanged);
    }
}

async function loadConfig() {
    showLoading(true);
    try {
        const response = await fetch('/api/config');
        const data = await response.json();
        currentConfig = data;
        populateForm(data);
    } catch (error) {
        showToast('Failed to load configuration: ' + error.message, 'error');
    } finally {
        showLoading(false);
    }
}

function populateForm(config) {
    if (!config) return;
    
    // Bridge settings
    setValue('bridge_name', config.bridge?.name || '');
    setValue('bridge_version', config.bridge?.version || '', true);
    setValue('log_level', config.bridge?.log_level || 'info');
    
    // OpenWebNet
    setValue('own_host', config.openwebnet?.host || '');
    setValue('own_port', config.openwebnet?.port || 20000);
    setValue('own_timeout', config.openwebnet?.timeout || 5000);
    setValue('own_retry_attempts', config.openwebnet?.retry_attempts || 3);
    
    // SIP
    setToggle('sip_enabled', config.sip?.enabled || false);
    setValue('sip_server_host', config.sip?.server_host || '');
    setValue('sip_server_port', config.sip?.server_port || 5060);
    setValue('sip_username', config.sip?.username || '');
    setValue('sip_password', config.sip?.password || '');
    setValue('sip_domain', config.sip?.domain || '');
    setValue('sip_transport', config.sip?.transport || 'udp');
    
    // MQTT
    setToggle('mqtt_enabled', config.mqtt?.enabled || false);
    setValue('mqtt_host', config.mqtt?.host || '');
    setValue('mqtt_port', config.mqtt?.port || 1883);
    setValue('mqtt_client_id', config.mqtt?.client_id || 'bticino_bridge');
    setValue('mqtt_username', config.mqtt?.username || '');
    setValue('mqtt_password', config.mqtt?.password || '');
    setValue('mqtt_topic_prefix', config.mqtt?.topic_prefix || 'homeassistant');
    
    // HomeKit
    setToggle('homekit_enabled', config.homekit?.enabled || false);
    setValue('homekit_name', config.homekit?.name || 'BTicino Bridge');
    setValue('homekit_pin', config.homekit?.pin || '');
    
    // Web
    setToggle('web_enabled', config.web?.enabled || false);
    setValue('web_port', config.web?.port || 8080);
    
    // Hardware
    setToggle('hardware_enabled', config.hardware?.enabled || false);
    setToggle('hardware_gpio', config.hardware?.gpio_monitoring || false);
    setValue('input_device', config.hardware?.input_device || '/dev/input/event0');
    
    // Streaming
    setToggle('streaming_enabled', config.streaming?.enabled || false);
    setValue('rtsp_port', config.streaming?.rtsp_port || 8554);
    setValue('webrtc_port', config.streaming?.webrtc_port || 8888);
    
    // Privacy
    if (config.privacy) {
        setToggle('privacy_block_telemetry', config.privacy.block_external_telemetry || false);
        setToggle('privacy_block_cloud', config.privacy.block_cloud || false);
        setToggle('privacy_local_logging', config.privacy.local_logging || true);
        setToggle('privacy_disable_updates', config.privacy.disable_auto_updates || false);
    }
    
    // Security
    if (config.security) {
        setToggle('security_web_auth', config.security.web_auth_required || false);
        setToggle('security_https', config.security.web_https_enabled || false);
        setValue('api_rate_limit', config.security.api_rate_limit || 100);
    }
}

function setValue(id, value, readonly) {
    const el = document.getElementById(id);
    if (el) {
        el.value = value || '';
        if (readonly) {
            el.setAttribute('readonly', true);
        }
    }
}

function setToggle(id, value) {
    const toggle = document.getElementById(id);
    const hiddenInput = document.getElementById(id + '_hidden');
    if (toggle && hiddenInput) {
        if (value) {
            toggle.classList.add('active');
        } else {
            toggle.classList.remove('active');
        }
        hiddenInput.value = value ? 'true' : 'false';
    }
}

async function saveConfig() {
    if (!configChanged) return;
    
    showLoading(true);
    try {
        const config = gatherFormData();
        const response = await fetch('/api/config/save', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ config: config, user: 'admin' })
        });
        
        const result = await response.json();
        if (result.success) {
            showToast('Configuration saved successfully!', 'success');
            configChanged = false;
            updateSaveButtonState();
            
            if (result.restart_required) {
                showToast('Restart required for changes to take effect', 'warning');
            }
            
            if (result.warnings && result.warnings.length > 0) {
                showValidationWarnings(result.warnings);
            }
        } else {
            showToast('Failed to save: ' + (result.message || 'Unknown error'), 'error');
        }
    } catch (error) {
        showToast('Error saving configuration: ' + error.message, 'error');
    } finally {
        showLoading(false);
    }
}

async function validateConfig() {
    showLoading(true);
    try {
        const config = gatherFormData();
        const response = await fetch('/api/config/validate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ config: config })
        });
        
        const result = await response.json();
        showValidationResult(result);
    } catch (error) {
        showToast('Error validating configuration: ' + error.message, 'error');
    } finally {
        showLoading(false);
    }
}

function showValidationResult(result) {
    const container = document.getElementById('validation-result');
    if (!container) return;
    
    container.style.display = 'block';
    container.className = 'validation-result';
    
    if (result.valid) {
        container.classList.add('success');
        container.innerHTML = '<h4>✅ Configuration Valid</h4>';
        
        if (result.warnings && result.warnings.length > 0) {
            container.innerHTML += '<ul>';
            result.warnings.forEach(w => {
                container.innerHTML += '<li>⚠️ ' + w + '</li>';
            });
            container.innerHTML += '</ul>';
        } else {
            container.innerHTML += '<p>No issues found.</p>';
        }
    } else {
        container.classList.add('error');
        container.innerHTML = '<h4>❌ Configuration Invalid</h4><ul>';
        result.errors.forEach(e => {
            container.innerHTML += '<li>❌ ' + e + '</li>';
        });
        container.innerHTML += '</ul>';
    }
}

function showValidationWarnings(warnings) {
    const container = document.getElementById('validation-result');
    if (!container) return;
    
    container.style.display = 'block';
    container.className = 'validation-result warning';
    container.innerHTML = '<h4>⚠️ Warnings</h4><ul>';
    warnings.forEach(w => {
        container.innerHTML += '<li>' + w + '</li>';
    });
    container.innerHTML += '</ul>';
}

function gatherFormData() {
    return {
        bridge: {
            name: getValue('bridge_name'),
            version: getValue('bridge_version'),
            log_level: getValue('log_level')
        },
        openwebnet: {
            host: getValue('own_host'),
            port: parseInt(getValue('own_port')) || 20000,
            timeout: parseInt(getValue('own_timeout')) || 5000,
            retry_attempts: parseInt(getValue('own_retry_attempts')) || 3
        },
        sip: {
            enabled: getToggle('sip_enabled'),
            server_host: getValue('sip_server_host'),
            server_port: parseInt(getValue('sip_server_port')) || 5060,
            username: getValue('sip_username'),
            password: getValue('sip_password'),
            domain: getValue('sip_domain'),
            transport: getValue('sip_transport')
        },
        mqtt: {
            enabled: getToggle('mqtt_enabled'),
            host: getValue('mqtt_host'),
            port: parseInt(getValue('mqtt_port')) || 1883,
            client_id: getValue('mqtt_client_id'),
            username: getValue('mqtt_username'),
            password: getValue('mqtt_password'),
            topic_prefix: getValue('mqtt_topic_prefix')
        },
        homekit: {
            enabled: getToggle('homekit_enabled'),
            name: getValue('homekit_name'),
            pin: getValue('homekit_pin')
        },
        web: {
            enabled: getToggle('web_enabled'),
            port: parseInt(getValue('web_port')) || 8080
        },
        hardware: {
            enabled: getToggle('hardware_enabled'),
            gpio_monitoring: getToggle('hardware_gpio'),
            input_device: getValue('input_device')
        },
        streaming: {
            enabled: getToggle('streaming_enabled'),
            rtsp_port: parseInt(getValue('rtsp_port')) || 8554,
            webrtc_port: parseInt(getValue('webrtc_port')) || 8888
        },
        privacy: {
            block_external_telemetry: getToggle('privacy_block_telemetry'),
            block_cloud: getToggle('privacy_block_cloud'),
            local_logging: getToggle('privacy_local_logging'),
            disable_auto_updates: getToggle('privacy_disable_updates')
        },
        security: {
            web_auth_required: getToggle('security_web_auth'),
            web_https_enabled: getToggle('security_https'),
            api_rate_limit: parseInt(getValue('api_rate_limit')) || 100
        }
    };
}

function getValue(id) {
    const el = document.getElementById(id);
    return el ? el.value : '';
}

function getToggle(id) {
    const el = document.getElementById(id);
    return el ? el.classList.contains('active') : false;
}

async function createBackup() {
    showLoading(true);
    try {
        const response = await fetch('/api/config/backup', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ user: 'admin' })
        });
        
        const result = await response.json();
        if (result.success) {
            showToast('Backup created: ' + result.filename, 'success');
            loadBackups();
        } else {
            showToast('Failed to create backup: ' + result.message, 'error');
        }
    } catch (error) {
        showToast('Error creating backup: ' + error.message, 'error');
    } finally {
        showLoading(false);
    }
}

async function loadBackups() {
    try {
        const response = await fetch('/api/config/backups');
        const data = await response.json();
        displayBackups(data.backups || []);
    } catch (error) {
        console.error('Failed to load backups:', error);
    }
}

function displayBackups(backups) {
    const container = document.getElementById('backup-list');
    if (!container) return;
    
    if (backups.length === 0) {
        container.innerHTML = '<p style="color: #6c757d;">No backups available</p>';
        return;
    }
    
    container.innerHTML = '';
    backups.forEach(backup => {
        const item = document.createElement('div');
        item.className = 'backup-item';
        item.innerHTML = 
            '<div class="backup-info">' +
                '<span class="backup-date">' + backup.date + '</span>' +
                '<span class="backup-size">' + backup.size + '</span>' +
            '</div>' +
            '<div class="backup-actions">' +
                '<button class="btn btn-secondary" onclick="restoreBackup(\'' + backup.file + '\')">Restore</button>' +
            '</div>';
        container.appendChild(item);
    });
}

async function restoreBackup(filename) {
    if (!confirm('Are you sure you want to restore this backup? Current configuration will be replaced.')) {
        return;
    }
    
    showLoading(true);
    try {
        const response = await fetch('/api/config/restore', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ filename: filename, user: 'admin' })
        });
        
        const result = await response.json();
        if (result.success) {
            showToast('Backup restored successfully!', 'success');
            loadConfig();
        } else {
            showToast('Failed to restore: ' + result.message, 'error');
        }
    } catch (error) {
        showToast('Error restoring backup: ' + error.message, 'error');
    } finally {
        showLoading(false);
    }
}

async function loadHistory() {
    try {
        const response = await fetch('/api/config/history');
        const data = await response.json();
        displayHistory(data.history || []);
    } catch (error) {
        console.error('Failed to load history:', error);
    }
}

function displayHistory(history) {
    const container = document.getElementById('history-list');
    if (!container) return;
    
    if (history.length === 0) {
        container.innerHTML = '<p style="color: #6c757d;">No history available</p>';
        return;
    }
    
    container.innerHTML = '';
    history.forEach(item => {
        const div = document.createElement('div');
        div.className = 'history-item';
        div.innerHTML = 
            '<div class="history-info">' +
                '<span class="history-date">' + item.timestamp + '</span>' +
                '<span class="history-user">by ' + item.user + '</span>' +
            '</div>' +
            '<span class="history-changes">' + item.changes + ' changes</span>';
        container.appendChild(div);
    });
}

async function reloadConfig() {
    showLoading(true);
    try {
        const response = await fetch('/api/config/reload', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        
        const result = await response.json();
        if (result.success) {
            showToast('Configuration reloaded!', 'success');
        } else {
            showToast('Failed to reload: ' + result.message, 'error');
        }
    } catch (error) {
        showToast('Error reloading configuration: ' + error.message, 'error');
    } finally {
        showLoading(false);
    }
}

function showLoading(show) {
    const overlay = document.querySelector('.loading-overlay');
    if (overlay) {
        overlay.classList.toggle('active', show);
    }
}

function showToast(message, type) {
    let toast = document.querySelector('.toast');
    if (!toast) {
        toast = document.createElement('div');
        toast.className = 'toast';
        document.body.appendChild(toast);
    }
    
    toast.textContent = message;
    toast.className = 'toast ' + type + ' show';
    
    setTimeout(function() {
        toast.classList.remove('show');
    }, 4000);
}

function switchTab(tabId) {
    const tabs = document.querySelectorAll('.tab');
    tabs.forEach(t => t.classList.remove('active'));
    
    const contents = document.querySelectorAll('.tab-content');
    contents.forEach(c => c.classList.remove('active'));
    
    document.querySelector('.tab[data-tab="' + tabId + '"]').classList.add('active');
    document.getElementById('tab-' + tabId).classList.add('active');
}

// Expose functions globally for inline handlers
window.saveConfig = saveConfig;
window.validateConfig = validateConfig;
window.createBackup = createBackup;
window.restoreBackup = restoreBackup;
window.reloadConfig = reloadConfig;
window.switchTab = switchTab;

// Device config functions (Fase 2)
window.loadDeviceConfig = loadDeviceConfig;
window.loadRingtones = loadRingtones;
window.loadVolumes = loadVolumes;
window.loadDisplayConfig = loadDisplayConfig;
window.loadCameras = loadCameras;
`
}

func (ws *WebServer) getCSS() string {
	return `/* BTicino Bridge {{VERSION}} Dashboard Styles */
* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

body {
    font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
    min-height: 100vh;
    color: #333;
}

.navbar {
    background: rgba(255, 255, 255, 0.95);
    backdrop-filter: blur(10px);
    padding: 1rem 2rem;
    display: flex;
    justify-content: space-between;
    align-items: center;
    box-shadow: 0 2px 20px rgba(0, 0, 0, 0.1);
    position: sticky;
    top: 0;
    z-index: 1000;
}

.nav-brand {
    font-size: 1.5rem;
    font-weight: bold;
    color: #667eea;
}

.nav-links {
    display: flex;
    gap: 2rem;
}

.nav-links a {
    color: #666;
    text-decoration: none;
    font-weight: 500;
    padding: 0.5rem 1rem;
    border-radius: 8px;
    transition: all 0.3s ease;
}

.nav-links a:hover, .nav-links a.active {
    background: #667eea;
    color: white;
}

.container {
    max-width: 1200px;
    margin: 0 auto;
    padding: 2rem;
}

h1 {
    color: white;
    text-align: center;
    margin-bottom: 3rem;
    font-size: 2.5rem;
    text-shadow: 2px 2px 4px rgba(0, 0, 0, 0.3);
}

h2 {
    color: #333;
    margin-bottom: 1.5rem;
    font-size: 1.8rem;
}

/* Status Cards */
.status-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: 2rem;
    margin-bottom: 3rem;
}

.status-card {
    background: rgba(255, 255, 255, 0.95);
    border-radius: 16px;
    padding: 1.5rem;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
    backdrop-filter: blur(10px);
    transition: transform 0.3s ease, box-shadow 0.3s ease;
}

.status-card:hover {
    transform: translateY(-5px);
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.15);
}

.card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
}

.card-header h3 {
    color: #333;
    font-size: 1.2rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.status-badge {
    padding: 0.3rem 0.8rem;
    border-radius: 20px;
    font-size: 0.8rem;
    font-weight: bold;
    text-transform: uppercase;
}

.status-connected { background: #10b981; color: white; }
.status-disconnected { background: #ef4444; color: white; }
.status-enabled { background: #3b82f6; color: white; }
.status-ready { background: #8b5cf6; color: white; }
.status-active { background: #f59e0b; color: white; }

.mqtt-events-list {
    margin-top: 0.5rem;
    font-size: 0.75rem;
}

.mqtt-event {
    display: flex;
    gap: 0.5rem;
    padding: 0.25rem 0;
    border-bottom: 1px solid #eee;
}

.mqtt-event .event-time { color: #888; }
.mqtt-event .event-type { color: #333; font-weight: bold; }
.mqtt-event .event-status.success { color: #10b981; }
.mqtt-event .event-status.error { color: #ef4444; }

.card-content p {
    margin-bottom: 0.5rem;
    color: #666;
}

/* Enhanced Features */
.enhanced-features {
    background: rgba(255, 255, 255, 0.95);
    border-radius: 16px;
    padding: 2rem;
    margin-bottom: 3rem;
    backdrop-filter: blur(10px);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
}

.enhanced-features h2 {
    color: #10b981;
    text-align: center;
    margin-bottom: 2rem;
}

.features-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: 1.5rem;
}

.feature-item {
    display: flex;
    align-items: flex-start;
    gap: 1rem;
    padding: 1rem;
    border-radius: 12px;
    background: rgba(16, 185, 129, 0.1);
    border-left: 4px solid #10b981;
}

.feature-icon {
    font-size: 2rem;
    min-width: 3rem;
}

/* Messages & Settings */
.messages-container, .settings-grid {
    background: rgba(255, 255, 255, 0.95);
    border-radius: 16px;
    padding: 2rem;
    margin-bottom: 2rem;
    backdrop-filter: blur(10px);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
}

.actions-grid, .control-buttons {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    margin-top: 1rem;
}

.action-btn, .control-btn {
    padding: 1rem 1.5rem;
    border: none;
    border-radius: 12px;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.3s ease;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    justify-content: center;
}

.action-btn:hover, .control-btn:hover {
    transform: translateY(-2px);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.2);
}

.primary { background: #3b82f6; color: white; }
.secondary { background: #6b7280; color: white; }
.success { background: #10b981; color: white; }
.warning { background: #f59e0b; color: white; }
.info { background: #0ea5e9; color: white; }

/* Control Sections */
.control-section {
    margin-bottom: 2rem;
}

.control-section h2 {
    color: #333;
    margin-bottom: 1rem;
    font-size: 1.5rem;
}

.control-btn {
    flex-direction: column;
    text-align: center;
    padding: 1.5rem;
}

.btn-icon {
    font-size: 2rem;
    margin-bottom: 0.5rem;
}

.control-btn small {
    opacity: 0.8;
    font-size: 0.8rem;
    font-weight: normal;
}

/* Messages */
.messages-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 2rem;
    flex-wrap: wrap;
    gap: 1rem;
}

.messages-stats {
    display: flex;
    gap: 2rem;
}

.stat-item {
    text-align: center;
}

.stat-number {
    display: block;
    font-size: 2rem;
    font-weight: bold;
    color: #3b82f6;
}

.stat-label {
    color: #666;
    font-size: 0.9rem;
}

.message-item {
    background: rgba(255, 255, 255, 0.8);
    border-radius: 12px;
    padding: 1.5rem;
    margin-bottom: 1rem;
    border-left: 4px solid #3b82f6;
    transition: all 0.3s ease;
}

.message-item:hover {
    transform: translateX(5px);
    box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
}

.message-item.unread {
    border-left-color: #f59e0b;
    background: rgba(245, 158, 11, 0.1);
}

.message-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
}

.message-time {
    color: #666;
    font-size: 0.9rem;
}

.message-duration {
    background: #3b82f6;
    color: white;
    padding: 0.2rem 0.6rem;
    border-radius: 10px;
    font-size: 0.8rem;
}

/* Settings */
.settings-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
    gap: 2rem;
}

.setting-section {
    background: rgba(255, 255, 255, 0.8);
    border-radius: 12px;
    padding: 1.5rem;
}

.setting-section h2 {
    color: #333;
    margin-bottom: 1rem;
    font-size: 1.3rem;
}

.info-list {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
}

.info-row {
    display: flex;
    justify-content: space-between;
    padding: 0.8rem 0;
    border-bottom: 1px solid rgba(0, 0, 0, 0.1);
}

.info-label {
    font-weight: 600;
    color: #555;
}

.info-value {
    color: #333;
    text-align: right;
}

.info-value code {
    background: #f3f4f6;
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    font-family: monospace;
    font-size: 0.9rem;
}

/* Enhanced Info */
.enhanced-info {
    background: rgba(255, 255, 255, 0.95);
    border-radius: 16px;
    padding: 2rem;
    margin-bottom: 2rem;
    text-align: center;
}

.features-list {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    justify-content: center;
    margin-top: 1rem;
}

.feature-badge {
    background: linear-gradient(135deg, #10b981, #059669);
    color: white;
    padding: 0.8rem 1.2rem;
    border-radius: 25px;
    font-size: 0.9rem;
    font-weight: 600;
}

/* API Endpoints */
.api-endpoints {
    background: rgba(255, 255, 255, 0.95);
    border-radius: 16px;
    padding: 2rem;
    margin-bottom: 2rem;
}

.endpoints-list {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.endpoint-item {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 1rem;
    background: rgba(255, 255, 255, 0.8);
    border-radius: 8px;
    border-left: 4px solid #3b82f6;
}

.method {
    padding: 0.3rem 0.8rem;
    border-radius: 6px;
    font-size: 0.8rem;
    font-weight: bold;
    min-width: 60px;
    text-align: center;
}

.method.GET { background: #10b981; color: white; }
.method.POST { background: #f59e0b; color: white; }

.endpoint-item code {
    font-family: monospace;
    background: #f3f4f6;
    padding: 0.3rem 0.6rem;
    border-radius: 4px;
    flex: 1;
}

.description {
    color: #666;
    font-size: 0.9rem;
}

/* Command Log */
.command-log {
    background: rgba(255, 255, 255, 0.95);
    border-radius: 16px;
    padding: 2rem;
    margin-bottom: 2rem;
}

.command-log-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem;
    margin-bottom: 0.5rem;
    background: rgba(0, 0, 0, 0.05);
    border-radius: 8px;
}

.command-log-item.success { border-left: 4px solid #10b981; }
.command-log-item.error { border-left: 4px solid #ef4444; }

/* Filesystem Info */
.filesystem-info {
    background: rgba(255, 255, 255, 0.95);
    border-radius: 16px;
    padding: 2rem;
    margin-bottom: 2rem;
}

.info-grid {
    display: grid;
    gap: 1rem;
}

.info-item {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 1rem;
    background: rgba(102, 126, 234, 0.1);
    border-radius: 8px;
    border-left: 4px solid #667eea;
}

.info-item strong {
    color: #333;
}

.info-item code {
    background: #f3f4f6;
    padding: 0.5rem;
    border-radius: 4px;
    font-family: monospace;
    color: #667eea;
}

/* Responsive Design */
@media (max-width: 768px) {
    .container {
        padding: 1rem;
    }
    
    .navbar {
        padding: 1rem;
        flex-direction: column;
        gap: 1rem;
    }
    
    .nav-links {
        gap: 1rem;
    }
    
    h1 {
        font-size: 1.8rem;
    }
    
    .status-grid, .settings-grid {
        grid-template-columns: 1fr;
    }
    
    .actions-grid {
        grid-template-columns: 1fr;
    }
    
    .messages-header {
        flex-direction: column;
        align-items: stretch;
    }
    
    .messages-stats {
        justify-content: space-around;
    }
    
    .endpoint-item {
        flex-direction: column;
        align-items: flex-start;
        gap: 0.5rem;
    }
}

/* Loading and animations */
.loading {
    display: inline-block;
    width: 20px;
    height: 20px;
    border: 3px solid #f3f3f3;
    border-top: 3px solid #3498db;
    border-radius: 50%;
    animation: spin 2s linear infinite;
}

@keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
}

.fade-in {
    animation: fadeIn 0.5s ease-in;
}

@keyframes fadeIn {
    from { opacity: 0; transform: translateY(20px); }
    to { opacity: 1; transform: translateY(0); }
}

/* Alert styles */
.alert {
    padding: 1rem 1.5rem;
    border-radius: 8px;
    margin-bottom: 1rem;
    border-left: 4px solid;
}

.alert.success {
    background: rgba(16, 185, 129, 0.1);
    border-left-color: #10b981;
    color: #065f46;
}

.alert.error {
    background: rgba(239, 68, 68, 0.1);
    border-left-color: #ef4444;
    color: #991b1b;
}

.alert.warning {
    background: rgba(245, 158, 11, 0.1);
    border-left-color: #f59e0b;
    color: #92400e;
}

.alert.info {
    background: rgba(59, 130, 246, 0.1);
    border-left-color: #3b82f6;
    color: #1e40af;
}`
}

// getJS returns the JavaScript code for the web dashboard
func (ws *WebServer) getJS() string {
	return `// BTicino Bridge {{VERSION}} Dashboard JavaScript
class BTicinoDashboard {
    constructor() {
        this.baseURL = window.location.origin;
        this.commandHistory = [];
        this.initialize();
    }

    initialize() {
        console.log('🚀 BTicino Bridge {{VERSION}} Dashboard Initialized');
        this.setupEventListeners();
        this.loadInitialData();
    }

    setupEventListeners() {
        // Add global error handler
        window.onerror = (msg, url, lineNo, columnNo, error) => {
            console.error('Dashboard Error:', msg, 'at', url, ':', lineNo);
            this.showAlert('A dashboard error occurred. Please refresh the page.', 'error');
        };
    }

    async loadInitialData() {
        try {
            const status = await this.apiRequest('/api/status');
            this.updateStatusDisplay(status);
        } catch (error) {
            console.error('Failed to load initial data:', error);
            this.showAlert('Failed to connect to BTicino device', 'error');
        }
    }

    async apiRequest(endpoint, options = {}) {
        try {
            const response = await fetch(this.baseURL + endpoint, {
                ...options,
                headers: {
                    'Content-Type': 'application/json',
                    ...options.headers
                }
            });

            if (!response.ok) {
                throw new Error('HTTP ' + response.status + ': ' + response.statusText);
            }

            return await response.json();
        } catch (error) {
            console.error('API Request failed:', error);
            throw error;
        }
    }

    updateStatusDisplay(status) {
        // Update component status badges
        this.updateStatusBadge('openwebnet-status', status.components?.openwebnet?.status);
        this.updateStatusBadge('answering-status', 
            status.components?.message_parser?.messages_found > 0 ? 'enabled' : 'disabled');
        this.updateStatusBadge('ha-status', status.components?.ha_integration?.status);
        this.updateStatusBadge('monitoring-status', status.components?.physical_monitoring?.status);

        // Update counts
        this.updateElement('total-messages', status.components?.message_parser?.messages_found || 0);
        this.updateElement('new-messages', status.components?.message_parser?.new_messages || 0);
        this.updateElement('storage-used', status.storage_used || '0%');

        // Update MQTT card with connection info
        const mqttStatus = status.mqtt;
        if (mqttStatus) {
            const mqttBadge = document.getElementById('ha-status');
            if (mqttBadge) {
                mqttBadge.classList.remove('status-connected', 'status-disconnected', 'status-ready');
                if (mqttStatus.connected) {
                    mqttBadge.classList.add('status-connected');
                    mqttBadge.textContent = 'Connected';
                } else {
                    mqttBadge.classList.add('status-disconnected');
                    mqttBadge.textContent = 'Disconnected';
                }
            }
            // Update MQTT connection events display
            const mqttEventsDiv = document.getElementById('mqtt-events');
            if (mqttEventsDiv && mqttStatus.connection_events) {
                const events = mqttStatus.connection_events.slice(-5).reverse();
                mqttEventsDiv.innerHTML = events.map(function(e) {
                    var time = new Date(e.timestamp).toLocaleTimeString();
                    var statusText = e.success ? 'OK' : 'FAIL';
                    var statusClass = e.success ? 'success' : 'error';
                    return '<div class="mqtt-event ' + e.type + '">' +
                        '<span class="event-time">' + time + '</span>' +
                        '<span class="event-type">' + e.type + '</span>' +
                        '<span class="event-status ' + statusClass + '">' + statusText + '</span>' +
                        '</div>';
                }).join('');
            }
            // Update reconnect count
            this.updateElement('mqtt-reconnects', mqttStatus.reconnect_attempts || 0);
            if (mqttStatus.connected_since) {
                this.updateElement('mqtt-since', new Date(mqttStatus.connected_since).toLocaleString());
            }
        }
    }

    updateStatusBadge(elementId, status) {
        const element = document.getElementById(elementId);
        if (!element) return;

        // Remove existing status classes
        element.classList.remove('status-connected', 'status-enabled', 'status-ready', 
                                'status-active', 'status-disabled', 'status-error');

        // Add appropriate status class
        switch(status) {
            case 'connected':
                element.classList.add('status-connected');
                element.textContent = 'Connected';
                break;
            case 'active':
                element.classList.add('status-active');
                element.textContent = 'Active';
                break;
            case 'ready':
                element.classList.add('status-ready');
                element.textContent = 'Ready';
                break;
            case 'enabled':
                element.classList.add('status-enabled');
                element.textContent = 'Enabled';
                break;
            case 'disabled':
                element.classList.add('status-disabled');
                element.textContent = 'Disabled';
                break;
            default:
                element.classList.add('status-error');
                element.textContent = 'Unknown';
        }
    }

    updateElement(elementId, value) {
        const element = document.getElementById(elementId);
        if (element) {
            element.textContent = value;
        }
    }

    showAlert(message, type = 'info') {
        // Create alert element
        const alertDiv = document.createElement('div');
        alertDiv.className = 'alert ' + type + ' fade-in';
        alertDiv.innerHTML = message;

        // Insert at top of container
        const container = document.querySelector('.container');
        if (container) {
            container.insertBefore(alertDiv, container.firstChild);

            // Auto-remove after 5 seconds
            setTimeout(() => {
                if (alertDiv.parentNode) {
                    alertDiv.parentNode.removeChild(alertDiv);
                }
            }, 5000);
        }
    }

    showLoading(elementId) {
        const element = document.getElementById(elementId);
        if (element) {
            element.innerHTML = '<div class="loading"></div>';
        }
    }

    async executeCommand(command, description) {
        console.log('Executing command:', command, description);
        
        const logItem = {
            command: command,
            description: description,
            timestamp: new Date().toISOString(),
            success: true
        };

        try {
            // Add to command history
            this.commandHistory.unshift(logItem);
            this.updateCommandLog();

            this.showAlert('Command executed: ' + description, 'success');
            return true;
        } catch (error) {
            console.error('Command execution failed:', error);
            logItem.success = false;
            logItem.error = error.message;
            
            this.showAlert('Command failed: ' + error.message, 'error');
            return false;
        }
    }

    updateCommandLog() {
        const logContainer = document.getElementById('command-log-list');
        if (!logContainer) return;

        logContainer.innerHTML = '';
        
        this.commandHistory.slice(0, 10).forEach(item => {
            const logItem = document.createElement('div');
            logItem.className = 'command-log-item ' + (item.success ? 'success' : 'error');
            
            logItem.innerHTML = 
                '<div>' +
                    '<strong>' + item.description + '</strong><br>' +
                    '<code>' + item.command + '</code>' +
                '</div>' +
                '<div>' +
                    '<small>' + new Date(item.timestamp).toLocaleString() + '</small>' +
                '</div>';
            
            logContainer.appendChild(logItem);
        });
    }

    // Dashboard navigation and updates (keeping other dashboard methods)
}

// Global functions for button handlers
async function refreshStatus() {
    if (typeof dashboard !== 'undefined') {
        try {
            const status = await dashboard.apiRequest('/api/status');
            dashboard.updateStatusDisplay(status);
            dashboard.showAlert('Status refreshed successfully', 'success');
        } catch (error) {
            dashboard.showAlert('Failed to refresh status', 'error');
        }
    }
}

async function unlockDoor() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/door/unlock', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, 'Door unlock');
        } catch (error) {
            dashboard.showAlert('Failed to unlock door', 'error');
        }
    }
}

async function toggleAnsweringMachine() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/answering-machine/toggle?action=enable', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to toggle answering machine', 'error');
        }
    }
}

async function enableAnsweringMachine() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/answering-machine/toggle?action=enable', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to enable answering machine', 'error');
        }
    }
}

async function disableAnsweringMachine() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/answering-machine/toggle?action=disable', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to disable answering machine', 'error');
        }
    }
}

async function queryAudioStatus() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/doorbell/on', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to query audio status', 'error');
        }
    }
}

async function activateDisplay() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/display/on', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to activate display', 'error');
        }
    }
}

async function deactivateDisplay() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/display/off', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to deactivate display', 'error');
        }
    }
}

async function muteOn() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/mute/on', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to enable mute', 'error');
        }
    }
}

async function muteOff() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/mute/off', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to disable mute', 'error');
        }
    }
}

async function doorbellSoundOn() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/doorbell/on', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to enable doorbell sound', 'error');
        }
    }
}

async function doorbellSoundOff() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/doorbell/off', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to disable doorbell sound', 'error');
        }
    }
}

async function staircaseLight() {
    if (typeof dashboard !== 'undefined') {
        try {
            const result = await dashboard.apiRequest('/api/controls/light/on', {
                method: 'POST'
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to activate staircase light', 'error');
        }
    }
}

async function sendArbitraryCommand() {
    if (typeof dashboard !== 'undefined') {
        const command = document.getElementById('arbitrary-command-input').value.trim();
        if (!command) {
            dashboard.showAlert('Please enter an OpenWebNet command', 'error');
            return;
        }
        try {
            const result = await dashboard.apiRequest('/api/controls/command', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ command: command })
            });
            dashboard.executeCommand(result.command, result.message);
        } catch (error) {
            dashboard.showAlert('Failed to send command: ' + error.message, 'error');
        }
    }
}

async function refreshMessages() {
    // This function is now handled by the enhanced messages page JavaScript
    // For compatibility with pages that still call it
    if (typeof loadMessages !== 'undefined') {
        await loadMessages();
    } else if (typeof dashboard !== 'undefined') {
        console.log('Using dashboard for message refresh (legacy)');
    }
}

// Initialize dashboard when DOM is ready
let dashboard;
document.addEventListener('DOMContentLoaded', function() {
    dashboard = new BTicinoDashboard();
    
    // Auto-refresh status every 30 seconds
    setInterval(refreshStatus, 30000);
    
    // Load messages if on messages page
    if (window.location.pathname.includes('messages')) {
        refreshMessages();
    }
    
    console.log('🎯 BTicino Bridge {{VERSION}} Dashboard Ready!');
});`
}

// Enhanced Message Management API Handlers

// @Summary List messages
// @Description Returns a paginated list of messages with optional filters
// @Tags Messages
// @Accept json
// @Produce json
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (max 100, default 20)"
// @Param unread_only query bool false "Show only unread messages"
// @Success 200 {object} map[string]interface{}
// @Router /api/messages/list [get]
// handleAPIMessagesList returns paginated list of messages with optional filters
func (ws *WebServer) handleAPIMessagesList(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20 // Default page size
	}

	unreadOnly := r.URL.Query().Get("unread_only") == "true"

	// Get all messages
	messages, err := ws.messageParser.GetAllMessages()
	if err != nil {
		http.Error(w, "Failed to retrieve messages", http.StatusInternalServerError)
		ws.logger.WithError(err).Error("Failed to retrieve messages")
		return
	}

	// Filter unread messages if requested
	if unreadOnly {
		filteredMessages := make([]*messageparser.Message, 0)
		for _, msg := range messages {
			if !msg.Read {
				filteredMessages = append(filteredMessages, msg)
			}
		}
		messages = filteredMessages
	}

	// Calculate pagination
	totalMessages := len(messages)
	totalPages := (totalMessages + limit - 1) / limit
	start := (page - 1) * limit
	end := start + limit

	if start >= totalMessages {
		messages = []*messageparser.Message{}
	} else if end > totalMessages {
		messages = messages[start:]
	} else {
		messages = messages[start:end]
	}

	response := map[string]interface{}{
		"messages": messages,
		"pagination": map[string]interface{}{
			"page":         page,
			"limit":        limit,
			"total":        totalMessages,
			"total_pages":  totalPages,
			"has_next":     page < totalPages,
			"has_previous": page > 1,
		},
		"filters": map[string]interface{}{
			"unread_only": unreadOnly,
		},
	}

	// Publish to MQTT if configured
	if pagination, ok := response["pagination"].(map[string]interface{}); ok {
		ws.publishMessageListMQTT(messages, pagination)
		ws.publishMessageStatusMQTT()
	}

	ws.writeJSON(w, response)
}

// @Summary Get message detail
// @Description Returns detailed information about a specific message by ID
// @Tags Messages
// @Accept json
// @Produce json
// @Param id path int true "Message ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/messages/{id} [get]
// handleAPIMessageDetail returns detailed information about a specific message
func (ws *WebServer) handleAPIMessageDetail(w http.ResponseWriter, r *http.Request) {
	// Extract message ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/messages/")
	messageID, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	message, err := ws.messageParser.GetMessage(messageID)
	if err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	ws.writeJSON(w, message)
}

// @Summary Download message media
// @Description Downloads the media file (image or video) for a specific message
// @Tags Messages
// @Accept json
// @Produce json
// @Param id path int true "Message ID"
// @Param type path string true "Media type: image or video"
// @Success 200 {file} file
// @Router /api/messages/download/{id}/{type} [get]
// handleAPIMessageDownload handles file downloads (image, video)
func (ws *WebServer) handleAPIMessageDownload(w http.ResponseWriter, r *http.Request) {
	// Parse URL: /api/messages/download/123/image or /api/messages/download/123/video
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/messages/download/"), "/")
	if len(pathParts) != 2 {
		http.Error(w, "Invalid download path", http.StatusBadRequest)
		return
	}

	messageID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	fileType := pathParts[1]

	var data []byte
	var filename string
	var contentType string

	switch fileType {
	case "image":
		data, err = ws.messageParser.GetImageData(messageID)
		filename = fmt.Sprintf("message_%d_image.jpg", messageID)
		contentType = "image/jpeg"
	case "video":
		data, err = ws.messageParser.GetVideoData(messageID)
		filename = fmt.Sprintf("message_%d_video.avi", messageID)
		contentType = "video/x-msvideo"
	default:
		http.Error(w, "Invalid file type. Use 'image' or 'video'", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Set headers for download
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))

	// Write file data
	w.Write(data)

	ws.logger.Infof("Downloaded %s for message %d", fileType, messageID)
}

// @Summary Mark message as read
// @Description Marks a message as read or unread
// @Tags Messages
// @Accept json
// @Produce json
// @Param id path int true "Message ID"
// @Param request body object false "Request body with read status"
// @Success 200 {object} map[string]interface{}
// @Router /api/messages/mark-read/{id} [post]
// handleAPIMessageMarkRead marks a message as read or unread
func (ws *WebServer) handleAPIMessageMarkRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "PUT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract message ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/messages/mark-read/")
	messageID, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	// Parse request body for read status
	type ReadStatusRequest struct {
		Read bool `json:"read"`
	}

	var req ReadStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Read = true // Default to marking as read
	}

	if req.Read {
		err = ws.messageParser.MarkMessageAsRead(messageID)
	} else {
		err = ws.messageParser.MarkMessageAsUnread(messageID)
	}

	if err != nil {
		http.Error(w, "Failed to update message status", http.StatusInternalServerError)
		ws.logger.WithError(err).Errorf("Failed to update read status for message %d", messageID)
		return
	}

	response := map[string]interface{}{
		"success":    true,
		"message_id": messageID,
		"read":       req.Read,
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	// Publish to MQTT if configured
	ws.publishMessageOperationResultMQTT("marked_read", messageID, true, "")
	ws.publishMessageStatusMQTT()

	ws.writeJSON(w, response)
	ws.logger.Infof("Message %d marked as %s", messageID, map[bool]string{true: "read", false: "unread"}[req.Read])
}

// @Summary Mark all messages as read
// @Description Marks every answering-machine message as read
// @Tags Messages
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/messages/mark-all-read [post]
func (ws *WebServer) handleAPIMessageMarkAllRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	updated, err := ws.messageParser.MarkAllMessagesAsRead()
	if err != nil {
		http.Error(w, "Failed to mark all messages as read", http.StatusInternalServerError)
		ws.logger.WithError(err).Errorf("mark-all-read failed after %d updates", updated)
		return
	}

	ws.publishMessageStatusMQTT()
	ws.writeJSON(w, map[string]interface{}{
		"success":   true,
		"updated":   updated,
		"timestamp": time.Now().Format(time.RFC3339),
	})
	ws.logger.Infof("Marked all messages as read (%d updated)", updated)
}

// @Summary Delete message
// @Description Deletes a specific message by ID
// @Tags Messages
// @Accept json
// @Produce json
// @Param id path int true "Message ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/messages/delete/{id} [delete]
// handleAPIMessageDelete deletes a message
func (ws *WebServer) handleAPIMessageDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract message ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/messages/delete/")
	messageID, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	err = ws.messageParser.DeleteMessage(messageID)
	if err != nil {
		http.Error(w, "Failed to delete message", http.StatusInternalServerError)
		ws.logger.WithError(err).Errorf("Failed to delete message %d", messageID)
		return
	}

	response := map[string]interface{}{
		"success":    true,
		"message_id": messageID,
		"deleted_at": time.Now().Format(time.RFC3339),
	}

	// Publish to MQTT if configured
	ws.publishMessageOperationResultMQTT("deleted", messageID, true, "")
	ws.publishMessageStatusMQTT()

	ws.writeJSON(w, response)
	ws.logger.Infof("Message %d deleted successfully", messageID)
}

// @Summary Get all memos
// @Description Returns all voice and text memos from the device
// @Tags Memos
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/memos [get]
func (ws *WebServer) handleAPIMemos(w http.ResponseWriter, r *http.Request) {
	memoParser := messageparser.NewMemoParser()
	memos, err := memoParser.GetAllMemos()
	if err != nil {
		ws.logger.WithError(err).Error("Failed to get memos")
		http.Error(w, "Failed to get memos", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"memos": memos,
		"total": len(memos),
		"voice": countMemosByType(memos, "voice"),
		"text":  countMemosByType(memos, "text"),
	}
	ws.writeJSON(w, response)
}

func countMemosByType(memos []*messageparser.Memo, memoType string) int {
	count := 0
	for _, m := range memos {
		if m.Type == memoType {
			count++
		}
	}
	return count
}

// handleAPIMemoDetail handles individual memo requests
func (ws *WebServer) handleAPIMemoDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/memos/")
	if path == "" {
		http.Error(w, "Invalid memo ID", http.StatusBadRequest)
		return
	}

	memoID, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid memo ID", http.StatusBadRequest)
		return
	}

	memoParser := messageparser.NewMemoParser()
	memos, err := memoParser.GetAllMemos()
	if err != nil {
		http.Error(w, "Failed to get memo", http.StatusInternalServerError)
		return
	}

	for _, m := range memos {
		if m.ID == memoID {
			ws.writeJSON(w, m)
			return
		}
	}

	http.Error(w, "Memo not found", http.StatusNotFound)
}

// handleAPIMemoAudio serves audio files for voice memos
func (ws *WebServer) handleAPIMemoAudio(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/memos/audio/")
	memoID, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid memo ID", http.StatusBadRequest)
		return
	}

	audioPath := fmt.Sprintf("/home/bticino/cfg/extra/47/memos_voice/memo_%d/audio.wav", memoID)

	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		http.Error(w, "Audio not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, audioPath)
}

// handleAPIMemoMarkRead marks a memo as read/unread
func (ws *WebServer) handleAPIMemoMarkRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/memos/mark-read/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "Invalid request: /api/memos/mark-read/{id}/{read}", http.StatusBadRequest)
		return
	}

	memoID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid memo ID", http.StatusBadRequest)
		return
	}

	readStatus := parts[1] == "read"
	memoType := "voice"
	if len(parts) >= 3 {
		memoType = parts[2]
	}

	memoParser := messageparser.NewMemoParser()
	if readStatus {
		err = memoParser.MarkMemoAsRead(memoID, memoType)
	} else {
		err = memoParser.MarkMemoAsUnread(memoID, memoType)
	}

	if err != nil {
		http.Error(w, "Failed to update memo status", http.StatusInternalServerError)
		ws.logger.WithError(err).Errorf("Failed to mark memo %d as %s", memoID, parts[1])
		return
	}

	response := map[string]interface{}{
		"success":   true,
		"memo_id":   memoID,
		"memo_type": memoType,
		"read":      readStatus,
	}
	ws.writeJSON(w, response)
	ws.logger.Infof("Memo %d (%s) marked as %s", memoID, memoType, parts[1])
}

// handleAPIMemoDelete deletes a memo
func (ws *WebServer) handleAPIMemoDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/memos/delete/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "Invalid request: /api/memos/delete/{id}/{type}", http.StatusBadRequest)
		return
	}

	memoID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid memo ID", http.StatusBadRequest)
		return
	}

	memoType := parts[1]
	if memoType != "voice" && memoType != "text" {
		memoType = "voice"
	}

	memoParser := messageparser.NewMemoParser()
	err = memoParser.DeleteMemo(memoID, memoType)
	if err != nil {
		http.Error(w, "Failed to delete memo", http.StatusInternalServerError)
		ws.logger.WithError(err).Errorf("Failed to delete memo %d", memoID)
		return
	}

	response := map[string]interface{}{
		"success":   true,
		"memo_id":   memoID,
		"memo_type": memoType,
	}
	ws.writeJSON(w, response)
	ws.logger.Infof("Memo %d (%s) deleted successfully", memoID, memoType)
}

// MQTT Publishing Helper Functions

// publishMessageStatusMQTT publishes message count and status updates to MQTT
func (ws *WebServer) publishMessageStatusMQTT() {
	if ws.mqttPublisher == nil {
		return // MQTT not configured
	}

	// Get current message statistics
	messages, err := ws.messageParser.GetAllMessages()
	if err != nil {
		ws.logger.Warnf("Failed to get messages for MQTT update: %v", err)
		return
	}

	totalCount := len(messages)
	unreadCount := 0
	latestID := 0
	latestTimestamp := ""

	for _, msg := range messages {
		if !msg.Read {
			unreadCount++
		}
		if msg.ID > latestID {
			latestID = msg.ID
			latestTimestamp = msg.Message // Timestamp is stored in message field
		}
	}

	// Publish individual statistics
	topicPrefix := "video_intercom/messages"
	ws.mqttPublisher(fmt.Sprintf("%s/total_count", topicPrefix), fmt.Sprintf("%d", totalCount), true)
	ws.mqttPublisher(fmt.Sprintf("%s/unread_count", topicPrefix), fmt.Sprintf("%d", unreadCount), true)
	ws.mqttPublisher(fmt.Sprintf("%s/latest_id", topicPrefix), fmt.Sprintf("%d", latestID), true)
	ws.mqttPublisher(fmt.Sprintf("%s/latest_timestamp", topicPrefix), latestTimestamp, true)

	// Calculate approximate storage usage (5MB per video + 5KB per image)
	storageMB := totalCount*5 + (totalCount*5)/1024 // Rough estimate
	ws.mqttPublisher(fmt.Sprintf("%s/storage_usage", topicPrefix), fmt.Sprintf("%d", storageMB), true)

	ws.logger.Debugf("Published message statistics to MQTT: total=%d, unread=%d", totalCount, unreadCount)
}

// publishMessageListMQTT publishes the current message list and pagination info to MQTT
func (ws *WebServer) publishMessageListMQTT(messages []*messageparser.Message, pagination map[string]interface{}) {
	if ws.mqttPublisher == nil {
		return
	}

	// Create simplified message list for MQTT (without base64 images)
	simplifiedMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		simplifiedMessages[i] = map[string]interface{}{
			"id":         msg.ID,
			"caller_id":  msg.CallerID,
			"message":    msg.Message,
			"read":       msg.Read,
			"has_image":  msg.HasImage,
			"has_video":  msg.HasVideo,
			"image_path": msg.ImagePath,
			"video_path": msg.VideoPath,
		}
	}

	// Marshal to JSON
	messagesJSON, err := json.Marshal(map[string]interface{}{
		"messages": simplifiedMessages,
	})
	if err != nil {
		ws.logger.Warnf("Failed to marshal messages for MQTT: %v", err)
		return
	}

	paginationJSON, err := json.Marshal(pagination)
	if err != nil {
		ws.logger.Warnf("Failed to marshal pagination for MQTT: %v", err)
		return
	}

	// Publish to MQTT
	topicPrefix := "video_intercom/messages"
	ws.mqttPublisher(fmt.Sprintf("%s/list", topicPrefix), string(messagesJSON), false)
	ws.mqttPublisher(fmt.Sprintf("%s/pagination", topicPrefix), string(paginationJSON), false)

	ws.logger.Debugf("Published message list to MQTT: %d messages", len(messages))
}

// publishMessageOperationResultMQTT publishes the result of message operations to MQTT
func (ws *WebServer) publishMessageOperationResultMQTT(operation string, messageID int, success bool, errorMsg string) {
	if ws.mqttPublisher == nil {
		return
	}

	resultData := map[string]interface{}{
		"message_id": messageID,
		"success":    success,
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	if !success && errorMsg != "" {
		resultData["error"] = errorMsg
	}

	resultJSON, err := json.Marshal(resultData)
	if err != nil {
		ws.logger.Warnf("Failed to marshal operation result for MQTT: %v", err)
		return
	}

	topic := fmt.Sprintf("video_intercom/messages/events/%s", operation)
	ws.mqttPublisher(topic, string(resultJSON), false)

	ws.logger.Debugf("Published %s operation result to MQTT: success=%v", operation, success)
}
