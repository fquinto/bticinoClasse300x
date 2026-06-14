package sip

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Go2RTCManager struct {
	binaryPath string
	configPath string
	logPath    string
	deviceIP   string
	cmd        *exec.Cmd
	eventBus   interface{}
	logger     *logrus.Logger
	mutex      sync.RWMutex
	running    bool
	stopCh     chan struct{}
}

type Go2RTCConfig struct {
	Enabled    bool   `yaml:"enabled"`
	BinaryPath string `yaml:"binary_path"`
	ConfigPath string `yaml:"config_path"`
	LogPath    string `yaml:"log_path"`
	DeviceIP   string `yaml:"device_ip"`
}

type Go2RTCStats struct {
	Running      bool                  `json:"running"`
	PID          int                   `json:"pid"`
	Uptime       string                `json:"uptime"`
	StartTime    time.Time             `json:"start_time"`
	StreamStatus map[string]StreamInfo `json:"streams"`
}

type StreamInfo struct {
	Active    bool   `json:"active"`
	Clients   int    `json:"clients"`
	Bandwidth string `json:"bandwidth"`
}

func NewGo2RTCManager(config *Go2RTCConfig, logger *logrus.Logger) *Go2RTCManager {
	if logger == nil {
		logger = logrus.New()
	}

	if config.BinaryPath == "" {
		config.BinaryPath = "/home/bticino/cfg/extra/go2rtc"
	}
	if config.ConfigPath == "" {
		config.ConfigPath = "/home/bticino/cfg/extra/go2rtc/go2rtc.yaml"
	}
	if config.LogPath == "" {
		config.LogPath = "/tmp/go2rtc.log"
	}

	return &Go2RTCManager{
		binaryPath: config.BinaryPath,
		configPath: config.ConfigPath,
		logPath:    config.LogPath,
		deviceIP:   config.DeviceIP,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

func (g *Go2RTCManager) Start() error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if g.running {
		return fmt.Errorf("go2rtc already running")
	}

	if _, err := os.Stat(g.binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("go2rtc binary not found at %s: %v", g.binaryPath, err)
	}

	if err := g.ensureConfigExists(); err != nil {
		return fmt.Errorf("failed to create config: %v", err)
	}

	g.logger.Info("Starting go2rtc...")

	g.cmd = exec.Command(g.binaryPath, "-config", g.configPath)
	g.cmd.Stdout, _ = os.Create(g.logPath)
	if f, err := os.OpenFile(g.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		g.cmd.Stdout = f
		g.cmd.Stderr = f
	}

	if err := g.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start go2rtc: %v", err)
	}

	g.running = true

	go g.monitor()

	g.logger.Infof("go2rtc started with PID %d", g.cmd.Process.Pid)
	g.logger.Infof("go2rtc API: http://%s:1984", g.deviceIP)
	g.logger.Infof("go2rtc WebRTC: http://%s:1984/#webrtc")

	return nil
}

func (g *Go2RTCManager) Stop() error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if !g.running {
		return nil
	}

	g.logger.Info("Stopping go2rtc...")

	close(g.stopCh)

	if g.cmd != nil && g.cmd.Process != nil {
		g.cmd.Process.Signal(os.Interrupt)
		time.Sleep(2 * time.Second)
		if g.cmd.Process.Pid > 0 {
			g.cmd.Process.Kill()
		}
	}

	g.running = false
	g.logger.Info("go2rtc stopped")

	return nil
}

func (g *Go2RTCManager) monitor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			return
		case <-ticker.C:
			if g.cmd == nil || g.cmd.Process == nil {
				g.mutex.Lock()
				g.running = false
				g.mutex.Unlock()
				g.logger.Warn("go2rtc process died unexpectedly")
				return
			}

			if err := g.cmd.Process.Signal(os.Signal(nil)); err != nil {
				g.mutex.Lock()
				g.running = false
				g.mutex.Unlock()
				g.logger.Warn("go2rtc process not responding")
				return
			}
		}
	}
}

func (g *Go2RTCManager) IsRunning() bool {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.running
}

func (g *Go2RTCManager) GetStats() (*Go2RTCStats, error) {
	stats := &Go2RTCStats{
		Running:      g.IsRunning(),
		StreamStatus: make(map[string]StreamInfo),
	}

	if g.cmd != nil && g.cmd.Process != nil {
		stats.PID = g.cmd.Process.Pid
	}

	resp, err := http.Get(fmt.Sprintf("http://%s:1984/api/streams", g.deviceIP))
	if err != nil {
		return stats, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var streams map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&streams); err == nil {
			for name, info := range streams {
				streamInfo := StreamInfo{Active: true}
				if m, ok := info.(map[string]interface{}); ok {
					if v, ok := m["consumers"].([]interface{}); ok {
						streamInfo.Clients = len(v)
					}
				}
				stats.StreamStatus[name] = streamInfo
			}
		}
	}

	return stats, nil
}

func (g *Go2RTCManager) GetAPIBase() string {
	return fmt.Sprintf("http://%s:1984", g.deviceIP)
}

func (g *Go2RTCManager) GetWebRTCURL() string {
	return fmt.Sprintf("http://%s:1984/#webrtc", g.deviceIP)
}

func (g *Go2RTCManager) GetStreamURL(streamName string) string {
	return fmt.Sprintf("rtsp://%s:6554/%s", g.deviceIP, streamName)
}

func (g *Go2RTCManager) ensureConfigExists() error {
	if _, err := os.Stat(g.configPath); err == nil {
		return nil
	}

	dir := g.configPath[:strings.LastIndex(g.configPath, "/")]
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	config := fmt.Sprintf(`streams:
  doorbell: rtsp://%s:6554/doorbell
  doorbell-video: rtsp://%s:6554/doorbell-video

webrtc:
  listen: :8555
  candidates:
    - %s:8555
    - stun:stun.l.google.com:19302

api:
  listen: :1984

rtsp:
  listen: ""

log:
  level: info
  format: text
`, g.deviceIP, g.deviceIP, g.deviceIP)

	return os.WriteFile(g.configPath, []byte(config), 0644)
}

func (g *Go2RTCManager) Restart() error {
	if err := g.Stop(); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)
	return g.Start()
}
