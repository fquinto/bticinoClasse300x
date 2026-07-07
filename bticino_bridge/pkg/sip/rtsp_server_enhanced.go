// Package sip proporciona funcionalidad RTSP mejorada para streaming de video
package sip

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bticino_bridge/pkg/events"
	"bticino_bridge/pkg/openwebnet"
	"github.com/sirupsen/logrus"
)

// EnhancedRTSPServer es un servidor RTSP mejorado con múltiples streams y grabación
type EnhancedRTSPServer struct {
	port            int
	listener        *net.TCPListener
	sipClient       *BTicinoSIPClient
	ownClient       *openwebnet.Client // OpenWebNet client for *7*300 video activation
	gstPipeline     *GStreamerPipeline // Direct GStreamer pipeline (bypasses bt_av_media)
	rtpRelay        *RTPRelayPair      // RTP relay for forwarding to RTSP clients
	eventBus        events.EventBus
	logger          *logrus.Logger
	sessions        map[string]*RTSPSession
	mutex           sync.RWMutex
	running         bool
	stopCh          chan struct{}
	callActive      bool
	activeClients   int
	recordingPath   string
	recordingActive bool
	recorder        *RTSPRecorder
	streams         map[string]*StreamConfig // Streams configurados
	videoBackend    string                   // "avmedia" | "gstreamer" | "auto"
	videoActivation bool                     // si false, ensureSIPCallActive NO activa nada (seguridad)
}

// StreamConfig configura un stream RTSP individual
type StreamConfig struct {
	Name         string // doorbell, doorbell-video, doorbell-recorder
	Description  string
	VideoEnabled bool
	AudioEnabled bool
	Recordable   bool
}

// RTSPRecorder maneja la grabación de streams para HKSV
type RTSPRecorder struct {
	server        *EnhancedRTSPServer
	recordingPath string
	currentFile   *os.File
	mu            sync.Mutex
	active        bool
	startTime     time.Time
	duration      time.Duration
	maxDuration   time.Duration
}

// NewEnhancedRTSPServer crea un servidor RTSP mejorado
func NewEnhancedRTSPServer(port int, sipClient *BTicinoSIPClient, eventBus events.EventBus, logger *logrus.Logger) *EnhancedRTSPServer {
	if logger == nil {
		logger = logrus.New()
	}

	gstDefaults := DefaultGStreamerConfig()

	server := &EnhancedRTSPServer{
		port:        port,
		sipClient:   sipClient,
		gstPipeline: NewGStreamerPipeline(logger),
		rtpRelay:    NewRTPRelayPair(gstDefaults.VideoPort, gstDefaults.AudioPort, logger),
		eventBus:    eventBus,
		logger:      logger,
		sessions:    make(map[string]*RTSPSession),
		stopCh:      make(chan struct{}),
		streams:     make(map[string]*StreamConfig),
	}

	// Configurar streams por defecto (compatibles con slyoldfox)
	server.streams = map[string]*StreamConfig{
		"/doorbell": {
			Name:         "doorbell",
			Description:  "Full stream (video + audio)",
			VideoEnabled: true,
			AudioEnabled: true,
			Recordable:   false,
		},
		"/doorbell-video": {
			Name:         "doorbell-video",
			Description:  "Video only stream",
			VideoEnabled: true,
			AudioEnabled: false,
			Recordable:   false,
		},
		"/doorbell-recorder": {
			Name:         "doorbell-recorder",
			Description:  "HKSV recording stream",
			VideoEnabled: true,
			AudioEnabled: true,
			Recordable:   true,
		},
		"/video": {
			Name:         "video",
			Description:  "Generic video stream",
			VideoEnabled: true,
			AudioEnabled: true,
			Recordable:   false,
		},
		"/stream": {
			Name:         "stream",
			Description:  "Default stream",
			VideoEnabled: true,
			AudioEnabled: true,
			Recordable:   false,
		},
	}

	return server
}

// SetRecordingPath configura el directorio de grabación
func (r *EnhancedRTSPServer) SetRecordingPath(path string) {
	r.recordingPath = path
	if path != "" {
		os.MkdirAll(path, 0755)
		r.logger.Infof("RTSP recording path set to: %s", path)
	}
}

// Start inicia el servidor RTSP mejorado
func (r *EnhancedRTSPServer) Start() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.running {
		return fmt.Errorf("RTSP server already running")
	}

	// Create TCP address
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", r.port))
	if err != nil {
		return fmt.Errorf("failed to resolve TCP address: %v", err)
	}

	r.logger.Infof("Attempting to listen on %s", addr.String())

	// Create listener with SO_REUSEADDR
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start RTSP server: %v", err)
	}

	r.listener = listener
	r.running = true

	// Start the RTP relay listeners (receive from GStreamer, forward to RTSP clients)
	if err := r.rtpRelay.Start(); err != nil {
		r.logger.WithError(err).Warn("Failed to start RTP relay (RTSP clients will not receive RTP)")
	} else {
		r.logger.Info("RTP relay started for video and audio forwarding")
	}

	r.logger.Infof("RTSP server is now listening on: %s", r.listener.Addr().String())

	r.logger.Infof("Enhanced RTSP server started on port %d", r.port)
	r.logger.Infof("Available streams:")
	for path, cfg := range r.streams {
		r.logger.Infof("  - rtsp://<ip>:%d%s (%s)", r.port, path, cfg.Description)
	}

	// Iniciar recorder si hay path configurado
	if r.recordingPath != "" {
		r.recorder = NewRTSPRecorder(r, r.recordingPath)
	}

	go r.acceptConnections()
	go r.sessionCleanup()

	return nil
}

// Stop detiene el servidor RTSP mejorado
func (r *EnhancedRTSPServer) Stop() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.running {
		return nil
	}

	r.logger.Info("Stopping Enhanced RTSP server")

	// Detener grabación activa
	if r.recorder != nil && r.recorder.IsActive() {
		r.recorder.Stop()
	}

	// Stop RTP relay
	if r.rtpRelay != nil {
		r.rtpRelay.Stop()
	}

	// Terminar llamada SIP si hay clientes activos
	if r.callActive && r.sipClient != nil {
		r.logger.Info("Ending active SIP call due to server stop")
		r.sipClient.Hangup()
	}

	if r.listener != nil {
		r.listener.Close()
	}

	close(r.stopCh)

	r.running = false
	r.logger.Info("Enhanced RTSP server stopped")

	return nil
}

func (r *EnhancedRTSPServer) acceptConnections() {
	r.logger.Info("RTSP acceptConnections goroutine started, waiting for connections...")
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			select {
			case <-r.stopCh:
				r.logger.Info("RTSP acceptConnections stopping")
				return
			default:
				r.logger.WithError(err).Error("Failed to accept RTSP connection")
				continue
			}
		}

		r.logger.Infof("RTSP: Accepted connection from %s", conn.RemoteAddr())
		go r.handleConnection(conn)
	}
}

func (r *EnhancedRTSPServer) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		// Clean up any sessions that used this TCP connection (interleaved mode)
		r.cleanupConnectionSessions(conn)
	}()

	clientAddr := conn.RemoteAddr().String()
	r.logger.Infof("New RTSP connection from %s", clientAddr)

	reader := bufio.NewReader(conn)
	var requestLines []string

	r.logger.Debugf("RTSP: Reading request from %s", clientAddr)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				r.logger.Debugf("RTSP connection closed: %s", clientAddr)
				return
			}
			r.logger.WithError(err).Debug("RTSP connection read error")
			return
		}

		line = strings.TrimSpace(line)
		r.logger.Tracef("RTSP read line: %q", line)

		if line == "" {
			r.logger.Debugf("RTSP: Empty line, processing request with %d lines", len(requestLines))
			if len(requestLines) > 0 {
				request := r.parseRTSPRequest(requestLines)
				if request != nil {
					r.logger.Infof("RTSP: Got %s request for %s", request.Method, request.URL)
					response := r.handleRTSPRequest(request, clientAddr, conn)
					r.logger.Debugf("RTSP: Raw response: %q", response)
					writer := bufio.NewWriter(conn)
					writer.WriteString(response)
					writer.Flush()
					r.logger.Debugf("RTSP: Sent response")
				} else {
					r.logger.Warnf("RTSP: Failed to parse request lines: %v", requestLines)
				}
			}
			requestLines = nil
		} else {
			requestLines = append(requestLines, line)
		}
	}
}

func (r *EnhancedRTSPServer) parseRTSPRequest(lines []string) *RTSPRequest {
	if len(lines) == 0 {
		return nil
	}

	parts := strings.Fields(lines[0])
	if len(parts) != 3 {
		return nil
	}

	request := &RTSPRequest{
		Method:  parts[0],
		URL:     parts[1],
		Version: parts[2],
		Headers: make(map[string]string),
	}

	for _, line := range lines[1:] {
		if colonIndex := strings.Index(line, ":"); colonIndex > 0 {
			key := strings.TrimSpace(line[:colonIndex])
			value := strings.TrimSpace(line[colonIndex+1:])
			request.Headers[key] = value

			switch strings.ToLower(key) {
			case "cseq":
				if cseq, err := strconv.Atoi(value); err == nil {
					request.CSeq = cseq
				}
			case "session":
				request.Session = value
			case "transport":
				request.Transport = value
			}
		}
	}

	return request
}

func (r *EnhancedRTSPServer) handleRTSPRequest(request *RTSPRequest, clientAddr string, conn net.Conn) string {
	if request == nil {
		return r.buildErrorResponse(400, "Bad Request", 0)
	}

	r.logger.Debugf("RTSP %s %s from %s (CSeq=%d)", request.Method, request.URL, clientAddr, request.CSeq)

	switch request.Method {
	case "OPTIONS":
		return r.handleOptions(request)
	case "DESCRIBE":
		return r.handleDescribe(request)
	case "SETUP":
		return r.handleSetup(request, clientAddr, conn)
	case "PLAY":
		return r.handlePlay(request, conn)
	case "TEARDOWN":
		return r.handleTeardown(request)
	default:
		return r.buildErrorResponse(501, "Not Implemented", request.CSeq)
	}
}

func (r *EnhancedRTSPServer) handleOptions(request *RTSPRequest) string {
	return r.buildResponse(200, "OK", request.CSeq, map[string]string{
		"Public": "DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE, RECORD",
	})
}

func (r *EnhancedRTSPServer) handleDescribe(request *RTSPRequest) string {
	streamPath := r.extractStreamPath(request.URL)

	config, exists := r.streams[streamPath]
	if !exists {
		// Si no existe, usar stream por defecto
		config = &StreamConfig{
			Name:         "default",
			VideoEnabled: true,
			AudioEnabled: true,
			Recordable:   false,
		}
	}

	sdp := r.buildSDP(streamPath, config)

	headers := map[string]string{
		"Content-Type":   "application/sdp",
		"Content-Length": strconv.Itoa(len(sdp)),
		"Accept-Ranges":  "npt",
	}

	return r.buildResponse(200, "OK", request.CSeq, headers) + sdp
}

func (r *EnhancedRTSPServer) handleSetup(request *RTSPRequest, clientAddr string, conn net.Conn) string {
	r.logger.Infof("=== HANDSETUP: Transport header: '%s'", request.Transport)

	if request.Transport == "" {
		return r.buildErrorResponse(400, "Transport Required", request.CSeq)
	}

	streamPath := r.extractStreamPath(request.URL)

	// Determine which track is being set up (video or audio)
	trackSetup := ""
	if strings.Contains(request.URL, "trackID=video") || strings.Contains(request.URL, "streamid=1") {
		trackSetup = "video"
	} else if strings.Contains(request.URL, "trackID=audio") || strings.Contains(request.URL, "streamid=0") {
		trackSetup = "audio"
	}

	// Check if this is a continuation of an existing session (SETUP for second track)
	existingSessionID := request.Session
	var session *RTSPSession

	if existingSessionID != "" {
		r.mutex.RLock()
		session = r.sessions[existingSessionID]
		r.mutex.RUnlock()
	}

	isNewSession := session == nil
	if isNewSession {
		existingSessionID = r.generateSessionID()
		session = &RTSPSession{
			ID:           existingSessionID,
			ClientAddr:   clientAddr,
			StreamPath:   streamPath,
			State:        RTSPReady,
			SetupTime:    time.Now(),
			LastActivity: time.Now(),
			SessionID:    existingSessionID,
		}
	}

	session.LastActivity = time.Now()
	session.TrackSetup = trackSetup

	// Check if it's TCP/interleaved mode
	if strings.Contains(request.Transport, "interleaved=") {
		session.IsInterleaved = true
		session.TCPConn = conn

		// Extract the channel numbers
		start := strings.Index(request.Transport, "interleaved=") + len("interleaved=")
		end := start
		for end < len(request.Transport) && request.Transport[end] != ';' && request.Transport[end] != ' ' {
			end++
		}
		channels := request.Transport[start:end]

		// Parse channel pair
		if dashIndex := strings.Index(channels, "-"); dashIndex > 0 {
			rtpCh, err1 := strconv.Atoi(channels[:dashIndex])
			_, _ = strconv.Atoi(channels[dashIndex+1:])
			if err1 == nil {
				if trackSetup == "audio" {
					session.InterleavedAudio = uint8(rtpCh)
				} else {
					// Default to video
					session.InterleavedVideo = uint8(rtpCh)
				}
			}
		}

		// Set client ports to a valid value for interleaved mode
		session.ClientPorts = RTSPPortPair{RTP: 0, RTCP: 0}

		transportResponse := fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%s", channels)

		r.mutex.Lock()
		r.sessions[existingSessionID] = session
		if isNewSession {
			r.activeClients++
		}
		r.mutex.Unlock()

		if isNewSession {
			r.logger.Infof("RTSP TCP session created: %s for %s track=%s (total clients: %d)",
				existingSessionID, clientAddr, trackSetup, r.activeClients)
			r.ensureSIPCallActiveAsync()
		}

		headers := map[string]string{
			"Transport": transportResponse,
			"Session":   existingSessionID,
		}
		return r.buildResponse(200, "OK", request.CSeq, headers)
	}

	// UDP mode
	clientPorts := r.parseClientPorts(request.Transport)
	r.logger.Infof("=== HANDSETUP: Parsed ports: RTP=%d, RTCP=%d", clientPorts.RTP, clientPorts.RTCP)

	if clientPorts.RTP == 0 {
		return r.buildErrorResponse(400, "Invalid Transport", request.CSeq)
	}

	session.ClientPorts = clientPorts
	session.IsInterleaved = false

	// Store per-track ports
	if trackSetup == "audio" {
		session.AudioClientPorts = clientPorts
	} else {
		// Default to video (including when trackSetup is "" or "video")
		session.VideoClientPorts = clientPorts
	}

	r.mutex.Lock()
	r.sessions[existingSessionID] = session
	if isNewSession {
		r.activeClients++
	}
	r.mutex.Unlock()

	if isNewSession {
		r.logger.Infof("RTSP UDP session created: %s for %s track=%s (total clients: %d)",
			existingSessionID, clientAddr, trackSetup, r.activeClients)
		r.ensureSIPCallActiveAsync()
	}

	r.eventBus.PublishWithSource("rtsp.session.setup", map[string]interface{}{
		"session_id":     existingSessionID,
		"client_addr":    clientAddr,
		"stream_path":    streamPath,
		"active_clients": r.activeClients,
		"transport":      "udp",
	}, "rtsp")

	config := r.sipClient.GetConfig()
	serverPort := config.MediaPorts.VideoRTP

	transportResponse := fmt.Sprintf("RTP/AVP;unicast;client_port=%d-%d;server_port=%d",
		clientPorts.RTP, clientPorts.RTCP, serverPort)

	headers := map[string]string{
		"Transport": transportResponse,
		"Session":   existingSessionID,
	}

	return r.buildResponse(200, "OK", request.CSeq, headers)
}

// ensureSIPCallActiveAsync wraps ensureSIPCallActive for use from SETUP.
func (r *EnhancedRTSPServer) ensureSIPCallActiveAsync() {
	if r.sipClient == nil {
		r.logger.Warn("sipClient is nil, cannot make SIP call")
		return
	}
	if err := r.ensureSIPCallActive(); err != nil {
		r.logger.WithError(err).Warn("Failed to ensure SIP call active")
	}
}

func (r *EnhancedRTSPServer) handlePlay(request *RTSPRequest, conn net.Conn) string {
	if request.Session == "" {
		return r.buildErrorResponse(400, "Session Required", request.CSeq)
	}

	r.mutex.Lock()
	session, exists := r.sessions[request.Session]
	if exists {
		session.State = RTSPPlaying
		session.LastActivity = time.Now()

		// Iniciar grabación si es stream grabable
		if r.recorder != nil {
			config, ok := r.streams[session.StreamPath]
			if ok && config.Recordable && !r.recorder.IsActive() {
				go r.recorder.Start(session.ID)
			}
		}
	}
	r.mutex.Unlock()

	if !exists {
		return r.buildErrorResponse(454, "Session Not Found", request.CSeq)
	}

	// Register RTP consumers with the relay
	r.registerRTPConsumer(session)

	r.logger.Infof("RTSP session playing: %s (stream: %s, interleaved: %v)",
		session.ID, session.StreamPath, session.IsInterleaved)

	r.eventBus.PublishWithSource("rtsp.session.playing", map[string]interface{}{
		"session_id":  session.ID,
		"client_addr": session.ClientAddr,
		"stream_path": session.StreamPath,
		"interleaved": session.IsInterleaved,
	}, "rtsp")

	headers := map[string]string{
		"Session": request.Session,
		"Range":   "npt=0.000-",
	}

	return r.buildResponse(200, "OK", request.CSeq, headers)
}

// registerRTPConsumer adds the session as an RTP consumer on the relay.
func (r *EnhancedRTSPServer) registerRTPConsumer(session *RTSPSession) {
	if r.rtpRelay == nil {
		r.logger.Warn("RTP relay not initialized, cannot register consumer")
		return
	}

	streamCfg, _ := r.streams[session.StreamPath]
	wantVideo := streamCfg == nil || streamCfg.VideoEnabled
	wantAudio := streamCfg == nil || streamCfg.AudioEnabled

	if session.IsInterleaved {
		// TCP interleaved mode
		if session.TCPConn == nil {
			r.logger.Warn("Session has interleaved mode but no TCP connection")
			return
		}
		if wantVideo {
			r.rtpRelay.Video.AddTCPConsumer(session.ID+"-video", session.TCPConn, session.InterleavedVideo)
		}
		if wantAudio {
			r.rtpRelay.Audio.AddTCPConsumer(session.ID+"-audio", session.TCPConn, session.InterleavedAudio)
		}
		r.logger.Infof("Registered TCP interleaved consumer: %s (video ch=%d, audio ch=%d)",
			session.ID, session.InterleavedVideo, session.InterleavedAudio)
	} else {
		// UDP mode — extract client IP and port
		clientHost, _, err := net.SplitHostPort(session.ClientAddr)
		if err != nil {
			r.logger.WithError(err).Warnf("Failed to parse client address: %s", session.ClientAddr)
			return
		}
		if wantVideo && session.VideoClientPorts.RTP > 0 {
			if err := r.rtpRelay.Video.AddUDPConsumer(session.ID+"-video", clientHost, session.VideoClientPorts.RTP); err != nil {
				r.logger.WithError(err).Warn("Failed to add UDP video consumer")
			}
		}
		if wantAudio && session.AudioClientPorts.RTP > 0 {
			if err := r.rtpRelay.Audio.AddUDPConsumer(session.ID+"-audio", clientHost, session.AudioClientPorts.RTP); err != nil {
				r.logger.WithError(err).Warn("Failed to add UDP audio consumer")
			}
		}
		r.logger.Infof("Registered UDP consumer: %s -> %s (video:%d, audio:%d)",
			session.ID, clientHost, session.VideoClientPorts.RTP, session.AudioClientPorts.RTP)
	}
}

// unregisterRTPConsumer removes the session from the RTP relay.
func (r *EnhancedRTSPServer) unregisterRTPConsumer(sessionID string) {
	if r.rtpRelay == nil {
		return
	}
	r.rtpRelay.Video.RemoveConsumer(sessionID + "-video")
	r.rtpRelay.Audio.RemoveConsumer(sessionID + "-audio")
}

// cleanupConnectionSessions removes all sessions that used a specific TCP connection.
// Called when the TCP connection is closed (e.g. client disconnects without TEARDOWN).
func (r *EnhancedRTSPServer) cleanupConnectionSessions(conn net.Conn) {
	r.mutex.Lock()
	var toRemove []string
	for id, session := range r.sessions {
		if session.TCPConn == conn || session.ClientAddr == conn.RemoteAddr().String() {
			toRemove = append(toRemove, id)
		}
	}
	for _, id := range toRemove {
		delete(r.sessions, id)
		r.activeClients--
		if r.activeClients < 0 {
			r.activeClients = 0
		}
	}
	remaining := r.activeClients
	callWasActive := r.callActive
	r.mutex.Unlock()

	// Unregister RTP consumers for removed sessions
	for _, id := range toRemove {
		r.unregisterRTPConsumer(id)
		r.logger.Infof("Cleaned up session %s due to connection close (remaining: %d)", id, remaining)
	}

	// If no more clients, stop pipelines and end SIP call
	if len(toRemove) > 0 && remaining == 0 && callWasActive {
		r.logger.Info("All RTSP clients disconnected, stopping GStreamer and SIP")
		go func() {
			if r.gstPipeline != nil && r.gstPipeline.IsRunning() {
				r.gstPipeline.Stop()
				r.eventBus.PublishWithSource("video.streams.stopped", map[string]interface{}{
					"reason": "connection_closed",
				}, "gstreamer")
			}
			time.Sleep(5 * time.Second)
			r.mutex.RLock()
			if r.activeClients == 0 && r.callActive {
				r.mutex.RUnlock()
				r.sipClient.Hangup()
				r.mutex.Lock()
				r.callActive = false
				r.mutex.Unlock()
				r.eventBus.PublishWithSource("sip.call.ended", map[string]interface{}{
					"reason": "connection_closed",
				}, "sip")
			} else {
				r.mutex.RUnlock()
			}
		}()
	}
}

func (r *EnhancedRTSPServer) handleTeardown(request *RTSPRequest) string {
	if request.Session == "" {
		return r.buildErrorResponse(400, "Session Required", request.CSeq)
	}

	r.mutex.Lock()
	session, exists := r.sessions[request.Session]
	if exists {
		delete(r.sessions, request.Session)
		r.activeClients--
		if r.activeClients < 0 {
			r.activeClients = 0
		}
	}
	r.mutex.Unlock()

	if !exists {
		return r.buildErrorResponse(454, "Session Not Found", request.CSeq)
	}

	// Remove RTP consumers for this session
	r.unregisterRTPConsumer(session.ID)

	duration := time.Since(session.SetupTime).Seconds()
	r.logger.Infof("RTSP session torn down: %s (duration: %.1fs, remaining clients: %d)",
		session.ID, duration, r.activeClients)

	r.eventBus.PublishWithSource("rtsp.session.teardown", map[string]interface{}{
		"session_id":     session.ID,
		"client_addr":    session.ClientAddr,
		"duration":       duration,
		"active_clients": r.activeClients,
	}, "rtsp")

	// Detener grabación si no hay más clientes
	if r.activeClients == 0 && r.recorder != nil && r.recorder.IsActive() {
		r.logger.Info("No more RTSP clients, stopping recorder")
		go func() {
			time.Sleep(2 * time.Second)
			r.mutex.RLock()
			if r.activeClients == 0 && r.recorder.IsActive() {
				r.mutex.RUnlock()
				r.recorder.Stop()
			} else {
				r.mutex.RUnlock()
			}
		}()
	}

	// Terminar llamada SIP y GStreamer pipelines si no hay más clientes
	if r.activeClients == 0 && r.callActive {
		r.logger.Info("No more RTSP clients, stopping GStreamer pipelines and ending SIP call")
		go func() {
			// Stop GStreamer pipelines immediately
			if r.gstPipeline != nil && r.gstPipeline.IsRunning() {
				r.gstPipeline.Stop()
				r.eventBus.PublishWithSource("video.streams.stopped", map[string]interface{}{
					"reason": "no_rtsp_clients",
				}, "gstreamer")
			}

			time.Sleep(5 * time.Second)
			r.mutex.RLock()
			if r.activeClients == 0 && r.callActive {
				r.mutex.RUnlock()
				r.sipClient.Hangup()
				r.mutex.Lock()
				r.callActive = false
				r.mutex.Unlock()
				r.eventBus.PublishWithSource("sip.call.ended", map[string]interface{}{
					"reason": "no_rtsp_clients",
				}, "sip")
			} else {
				r.mutex.RUnlock()
			}
		}()
	}

	return r.buildResponse(200, "OK", request.CSeq, map[string]string{})
}

func (r *EnhancedRTSPServer) buildSDP(streamPath string, config *StreamConfig) string {
	var localIP string

	if r.sipClient != nil {
		cfg := r.sipClient.GetConfig()
		localIP = cfg.LocalIP
	} else {
		localIP = "192.168.1.38"
		r.logger.Warn("RTSP: No SIP client, using default IP for SDP")
	}

	sdp := fmt.Sprintf(`v=0
o=- 0 0 IN IP4 %s
s=BTicino Video Doorbell - %s
c=IN IP4 %s
t=0 0
a=tool:BTicino-Bridge-RTSP/Enhanced
a=type:broadcast
a=control:*
`, localIP, config.Name, localIP)

	if config.VideoEnabled {
		// Camera: 720x576 interlaced PAL (~7fps actual), H.264 Constrained Baseline Level 3.0
		// VPU encoder: imxvpuenc_h264, byte-stream format
		// profile-level-id=42C01E = Constrained Baseline Profile, Level 3.0
		sdp += fmt.Sprintf(`m=video 0 RTP/AVP 96
b=AS:1500
a=rtpmap:96 H264/90000
a=fmtp:96 profile-level-id=42C01E;packetization-mode=1
a=control:trackID=video
a=framesize:96 720-576
a=framerate:7
`)
	}

	if config.AudioEnabled {
		sdp += fmt.Sprintf(`m=audio 0 RTP/AVP 110
b=AS:16
a=rtpmap:110 speex/8000
a=fmtp:110 vbr=on
a=control:trackID=audio
a=maxptime:20
`)
	}

	return sdp
}

// parseClientPorts extrae los puertos del cliente del header Transport
func (r *EnhancedRTSPServer) parseClientPorts(transport string) RTSPPortPair {
	// Handle UDP mode: client_port=xxx-yyy
	if strings.Contains(transport, "client_port=") {
		start := strings.Index(transport, "client_port=") + len("client_port=")
		end := start
		for end < len(transport) && transport[end] != ';' && transport[end] != ' ' {
			end++
		}

		portRange := transport[start:end]
		if dashIndex := strings.Index(portRange, "-"); dashIndex > 0 {
			rtpPort, err1 := strconv.Atoi(portRange[:dashIndex])
			rtcpPort, err2 := strconv.Atoi(portRange[dashIndex+1:])
			if err1 == nil && err2 == nil {
				return RTSPPortPair{RTP: rtpPort, RTCP: rtcpPort}
			} else if err1 == nil {
				return RTSPPortPair{RTP: rtpPort, RTCP: rtpPort + 1}
			}
		} else {
			if rtpPort, err := strconv.Atoi(portRange); err == nil {
				return RTSPPortPair{RTP: rtpPort, RTCP: rtpPort + 1}
			}
		}
	}

	// Handle TCP mode (interleaved): interleaved=0-1
	if strings.Contains(transport, "interleaved=") {
		start := strings.Index(transport, "interleaved=") + len("interleaved=")
		end := start
		for end < len(transport) && transport[end] != ';' && transport[end] != ' ' {
			end++
		}

		channelRange := transport[start:end]
		if dashIndex := strings.Index(channelRange, "-"); dashIndex > 0 {
			rtpChannel, err1 := strconv.Atoi(channelRange[:dashIndex])
			rtcpChannel, err2 := strconv.Atoi(channelRange[dashIndex+1:])
			if err1 == nil && err2 == nil {
				// For TCP/interleaved, we don't need actual ports - just mark as valid
				return RTSPPortPair{RTP: 9000 + rtpChannel, RTCP: 9000 + rtcpChannel}
			}
		}
	}

	return RTSPPortPair{}
}

func (r *EnhancedRTSPServer) buildResponse(code int, status string, cseq int, headers map[string]string) string {
	lines := []string{
		fmt.Sprintf("RTSP/1.0 %d %s", code, status),
		fmt.Sprintf("CSeq: %d", cseq),
		fmt.Sprintf("Date: %s", time.Now().UTC().Format(time.RFC1123)),
		"Server: BTicino-Bridge-RTSP-Enhanced/1.0",
	}

	for key, value := range headers {
		lines = append(lines, fmt.Sprintf("%s: %s", key, value))
	}

	lines = append(lines, "", "")
	return strings.Join(lines, "\r\n")
}

func (r *EnhancedRTSPServer) buildErrorResponse(code int, status string, cseq int) string {
	return r.buildResponse(code, status, cseq, map[string]string{})
}

func (r *EnhancedRTSPServer) extractStreamPath(url string) string {
	path := strings.TrimPrefix(url, "rtsp://")
	if slashIndex := strings.Index(path, "/"); slashIndex >= 0 {
		return path[slashIndex:]
	}
	return "/" + path
}

func (r *EnhancedRTSPServer) generateSessionID() string {
	return fmt.Sprintf("%s-%d", time.Now().Format("20060102150405"), time.Now().UnixNano()%10000)
}

func (r *EnhancedRTSPServer) sessionCleanup() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.mutex.Lock()
			now := time.Now()
			for sessionID, session := range r.sessions {
				if now.Sub(session.LastActivity) > 10*time.Minute {
					delete(r.sessions, sessionID)
					r.activeClients--
					if r.activeClients < 0 {
						r.activeClients = 0
					}
					r.logger.Infof("Cleaned up inactive RTSP session: %s", sessionID)
				}
			}
			r.mutex.Unlock()
		}
	}
}

func (r *EnhancedRTSPServer) ensureSIPCallActive() error {
	// SEGURIDAD: si la activación de vídeo bajo demanda está desactivada,
	// no disparamos NADA hacia el dispositivo nativo (ni self-INVITE ni *7*300).
	if !r.videoActivation {
		r.logger.Warn("Video on-demand disabled: ignoring RTSP/snapshot activation request")
		return fmt.Errorf("video on-demand activation is disabled")
	}

	r.mutex.Lock()
	if r.callActive {
		r.mutex.Unlock()
		r.logger.Debug("SIP call already active, skipping")
		return nil
	}
	r.mutex.Unlock()

	r.logger.Info("=== ENSURESIP: Initiating SIP call for RTSP streaming ===")

	// Per slyoldfox: INVITE target is the intercom's SIP identity (e.g., "c300x")
	// NOT the domain. buildInviteRequest will format it as c300x@domain.
	target := r.sipClient.config.SIPTarget
	if target == "" {
		target = "c300x"
	}
	if err := r.sipClient.MakeCall(target); err != nil {
		return fmt.Errorf("failed to make SIP call: %v", err)
	}

	r.mutex.Lock()
	r.callActive = true
	r.mutex.Unlock()

	r.eventBus.PublishWithSource("sip.call.started", map[string]interface{}{
		"reason": "rtsp_client_connected",
		"target": target,
	}, "sip")

	// Activate media streams ONLY after SIP call is fully connected
	// Primary: Direct GStreamer pipelines (bypasses bt_av_media MQTT bug)
	// Fallback: *7*300 commands (needs ownClient)
	go r.activateMediaStreamsWhenConnected()

	r.logger.Infof("=== SIP call initiated to %s for RTSP streaming ===", target)
	return nil
}

// SetOpenWebNetClient sets the OpenWebNet client for *7*300 video stream activation
func (r *EnhancedRTSPServer) SetOpenWebNetClient(client *openwebnet.Client) {
	r.ownClient = client
	r.logger.Info("OpenWebNet client set for video stream activation")
}

// SetVideoBackend selects how camera video is obtained: "avmedia" (*7*300 to
// bt_av_media, cooperative), "gstreamer" (direct VPU capture) or "auto".
func (r *EnhancedRTSPServer) SetVideoBackend(backend string) {
	if backend == "" {
		backend = "avmedia"
	}
	r.videoBackend = backend
	r.logger.Infof("Video backend set to: %s", backend)
}

// SetVideoActivation habilita/deshabilita la activación de vídeo bajo demanda.
// Con false (por defecto), ensureSIPCallActive rechaza cualquier intento, de
// modo que ni un cliente RTSP ni un snapshot pueden disparar comandos al nativo.
func (r *EnhancedRTSPServer) SetVideoActivation(enabled bool) {
	r.videoActivation = enabled
	if enabled {
		r.logger.Info("Video on-demand activation ENABLED")
	} else {
		r.logger.Info("Video on-demand activation DISABLED (safe default)")
	}
}

// activateMediaStreamsWhenConnected waits for the SIP call to reach Connected state,
// then starts GStreamer pipelines directly to capture video/audio and stream via RTP.
// This bypasses bt_av_media's MQTT interface which has a libjel.so routing bug where
// registered topics intercept external messages before they reach the command handler.
func (r *EnhancedRTSPServer) activateMediaStreamsWhenConnected() {
	r.logger.Info("=== Waiting for SIP call to reach Connected state before activating media ===")

	// Wait for the SIP session to be fully established
	connected := r.sipClient.WaitForConnected(30 * time.Second)
	if !connected {
		callState := r.sipClient.GetCallState()
		r.logger.Warnf("=== SIP call did not reach Connected state (current: %s), trying GStreamer anyway ===", callState)
	} else {
		r.logger.Info("=== SIP call Connected! Starting GStreamer pipelines for video/audio ===")
	}

	// Small delay after ACK
	time.Sleep(500 * time.Millisecond)

	localIP := "127.0.0.1"
	videoPort := 10002
	audioPort := 10000

	backend := r.videoBackend
	if backend == "" {
		backend = "avmedia"
	}

	switch backend {
	case "gstreamer":
		// Solo GStreamer directo (compite con la cámara nativa)
		if r.startGStreamer(localIP, videoPort, audioPort) {
			return
		}
		r.logger.Warn("=== GStreamer backend failed and no fallback configured (video_backend=gstreamer) ===")

	case "avmedia":
		// Solo *7*300: pedir a bt_av_media que duplique su RTP (cooperativo)
		if r.ownClient != nil {
			r.activateVia7300(localIP, videoPort, audioPort)
		} else {
			r.logger.Error("=== video_backend=avmedia pero no hay cliente OpenWebNet; sin vídeo ===")
		}

	default: // "auto": avmedia primero, GStreamer como respaldo
		activated := false
		if r.ownClient != nil {
			r.logger.Info("=== auto: intentando backend avmedia (*7*300) primero ===")
			r.activateVia7300(localIP, videoPort, audioPort)
			// Confirmar por flujo RTP real antes de dar por buena la activación
			activated = r.waitVideoFlow(6 * time.Second)
		}
		if !activated {
			r.logger.Info("=== auto: avmedia sin flujo RTP, cayendo a GStreamer directo ===")
			r.startGStreamer(localIP, videoPort, audioPort)
		}
	}
}

// startGStreamer lanza el pipeline GStreamer directo. Devuelve true si arrancó.
func (r *EnhancedRTSPServer) startGStreamer(localIP string, videoPort, audioPort int) bool {
	cfg := DefaultGStreamerConfig()
	cfg.TargetIP = localIP
	cfg.VideoPort = videoPort
	cfg.AudioPort = audioPort

	r.logger.Infof("=== Starting GStreamer pipelines: video -> %s:%d, audio -> %s:%d ===",
		localIP, videoPort, localIP, audioPort)

	if err := r.gstPipeline.Start(cfg); err != nil {
		r.logger.WithError(err).Error("=== Failed to start GStreamer pipelines ===")
		return false
	}

	r.logger.Info("=== GStreamer pipelines started! Video/audio streaming via RTP ===")
	r.eventBus.PublishWithSource("video.streams.activated", map[string]interface{}{
		"video_port": videoPort,
		"audio_port": audioPort,
		"target_ip":  localIP,
		"method":     "gstreamer_direct",
	}, "gstreamer")
	return true
}

// waitVideoFlow espera hasta timeout a que el relay de vídeo reciba paquetes.
func (r *EnhancedRTSPServer) waitVideoFlow(timeout time.Duration) bool {
	if r.rtpRelay == nil || r.rtpRelay.Video == nil {
		return false
	}
	relay := r.rtpRelay.Video
	start := atomic.LoadUint64(&relay.packetsReceived)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if atomic.LoadUint64(&relay.packetsReceived) > start {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// activateVia7300 is the legacy fallback using *7*300 OpenWebNet commands.
// This only works when bt_av_media already has a pipeline running (started internally
// by bt_answering_machine via SIP), which redirects the RTP stream to our ports.
func (r *EnhancedRTSPServer) activateVia7300(localIP string, videoPort, audioPort int) {
	// Activate video first (high-res)
	r.logger.Infof("=== Fallback: Activating video stream via *7*300 to %s:%d ===", localIP, videoPort)
	if err := r.ownClient.ActivateVideoStream(localIP, videoPort, true); err != nil {
		r.logger.WithError(err).Error("Failed to activate video stream via OpenWebNet")
		return
	}

	// Wait 300ms per slyoldfox timing
	time.Sleep(300 * time.Millisecond)

	// Activate audio
	r.logger.Infof("=== Fallback: Activating audio stream via *7*300 to %s:%d ===", localIP, audioPort)
	if err := r.ownClient.ActivateAudioStream(localIP, audioPort); err != nil {
		r.logger.WithError(err).Error("Failed to activate audio stream via OpenWebNet")
		return
	}

	r.logger.Info("=== Fallback: Media streams activated via OpenWebNet *7*300 ===")

	r.eventBus.PublishWithSource("video.streams.activated", map[string]interface{}{
		"video_port": videoPort,
		"audio_port": audioPort,
		"target_ip":  localIP,
		"method":     "openwebnet_7300",
	}, "openwebnet")
}

// GetActiveSessions devuelve las sesiones activas
func (r *EnhancedRTSPServer) GetActiveSessions() map[string]*RTSPSession {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	sessions := make(map[string]*RTSPSession)
	for id, session := range r.sessions {
		sessionCopy := *session
		sessions[id] = &sessionCopy
	}

	return sessions
}

// GetStats devuelve estadísticas del servidor
func (r *EnhancedRTSPServer) GetStats() map[string]interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":         r.running,
		"port":            r.port,
		"active_sessions": len(r.sessions),
		"active_clients":  r.activeClients,
		"call_active":     r.callActive,
		"streams":         make(map[string]interface{}),
	}

	for path, cfg := range r.streams {
		stats["streams"].(map[string]interface{})[path] = map[string]interface{}{
			"name":          cfg.Name,
			"description":   cfg.Description,
			"video_enabled": cfg.VideoEnabled,
			"audio_enabled": cfg.AudioEnabled,
			"recordable":    cfg.Recordable,
		}
	}

	if r.recorder != nil {
		stats["recording"] = r.recorder.GetStats()
	}

	if r.gstPipeline != nil {
		stats["gstreamer"] = r.gstPipeline.GetStats()
	}

	if r.rtpRelay != nil {
		stats["rtp_relay"] = r.rtpRelay.GetStats()
	}

	return stats
}

// NewRTSPRecorder crea un grabador RTSP
func NewRTSPRecorder(server *EnhancedRTSPServer, recordingPath string) *RTSPRecorder {
	return &RTSPRecorder{
		server:        server,
		recordingPath: recordingPath,
		maxDuration:   30 * time.Second, // Duración máxima por defecto
	}
}

// Start inicia la grabación
func (r *RTSPRecorder) Start(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.active {
		return fmt.Errorf("recorder already active")
	}

	if r.recordingPath == "" {
		return fmt.Errorf("recording path not configured")
	}

	// Crear archivo de grabación
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(r.recordingPath, fmt.Sprintf("recording_%s_%s.ts", timestamp, sessionID))

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create recording file: %v", err)
	}

	r.currentFile = file
	r.active = true
	r.startTime = time.Now()

	r.server.logger.Infof("RTSP recording started: %s", filename)

	r.server.eventBus.PublishWithSource("rtsp.recording.started", map[string]interface{}{
		"session_id": sessionID,
		"filename":   filename,
		"start_time": r.startTime,
	}, "rtsp")

	// Programar parada automática
	go func() {
		time.Sleep(r.maxDuration)
		r.Stop()
	}()

	return nil
}

// Stop detiene la grabación
func (r *RTSPRecorder) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.active {
		return nil
	}

	r.active = false
	r.duration = time.Since(r.startTime)

	if r.currentFile != nil {
		r.currentFile.Close()
		r.currentFile = nil
	}

	r.server.logger.Infof("RTSP recording stopped (duration: %.1fs)", r.duration.Seconds())

	r.server.eventBus.PublishWithSource("rtsp.recording.stopped", map[string]interface{}{
		"duration": r.duration.Seconds(),
	}, "rtsp")

	return nil
}

// IsActive devuelve si el grabador está activo
func (r *RTSPRecorder) IsActive() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

// GetStats devuelve estadísticas de grabación
func (r *RTSPRecorder) GetStats() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	return map[string]interface{}{
		"active":           r.active,
		"recording_path":   r.recordingPath,
		"current_duration": 0,
		"max_duration":     r.maxDuration.Seconds(),
	}
}
