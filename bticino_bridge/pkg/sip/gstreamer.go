// Package sip provides GStreamer pipeline management for direct video/audio streaming.
// This bypasses bt_av_media's MQTT interface (which has a libjel.so routing bug where
// registered topics intercept messages before they reach the command handler) by launching
// gst-launch-1.0 processes directly on the device.
package sip

import (
	"bytes"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// gstProcess wraps an exec.Cmd with a done channel for clean lifecycle management
type gstProcess struct {
	cmd    *exec.Cmd
	done   chan struct{} // closed when process exits
	err    error         // set before done is closed
	stderr *bytes.Buffer // captured stderr for error diagnostics
}

// GStreamerPipeline manages gst-launch-1.0 processes for video and audio streaming.
// It captures from the i.MX camera (/dev/video0) using hardware VPU encoding and
// streams RTP packets to a local UDP port where go2rtc or other consumers can receive them.
type GStreamerPipeline struct {
	logger    *logrus.Logger
	video     *gstProcess
	audio     *gstProcess
	mu        sync.Mutex
	running   bool
	startTime time.Time
	videoPort int
	audioPort int
	targetIP  string
	bitrate   int // kbps
}

// GStreamerConfig holds configuration for the GStreamer pipeline
type GStreamerConfig struct {
	TargetIP    string // IP to send RTP to (usually 127.0.0.1)
	VideoPort   int    // UDP port for video RTP (default 10002)
	AudioPort   int    // UDP port for audio RTP (default 10000)
	Bitrate     int    // Video bitrate in kbps (default 1500)
	GOPSize     int    // Frames per GOP (default 7, matches ~1s at camera's ~7fps)
	IDRInterval int    // IDR every N GOPs; 1=every GOP has IDR (default 1)
}

// DefaultGStreamerConfig returns sensible defaults.
// Camera outputs ~7fps (720x576 interlaced PAL via imxv4l2videosrc).
// GOPSize=7 + IDRInterval=1 gives one IDR per second for fast stream joining.
func DefaultGStreamerConfig() GStreamerConfig {
	return GStreamerConfig{
		TargetIP:    "127.0.0.1",
		VideoPort:   10002,
		AudioPort:   10000,
		Bitrate:     1500,
		GOPSize:     7,
		IDRInterval: 1,
	}
}

// NewGStreamerPipeline creates a new pipeline manager
func NewGStreamerPipeline(logger *logrus.Logger) *GStreamerPipeline {
	if logger == nil {
		logger = logrus.New()
	}
	return &GStreamerPipeline{
		logger: logger,
	}
}

// startProcess launches a gst-launch-1.0 command and returns a gstProcess wrapper.
// A goroutine is started to wait for the process to exit.
func (g *GStreamerPipeline) startProcess(name string, args ...string) (*gstProcess, error) {
	cmd := exec.Command("gst-launch-1.0", args...)
	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start %s pipeline: %w", name, err)
	}

	proc := &gstProcess{
		cmd:    cmd,
		done:   make(chan struct{}),
		stderr: stderrBuf,
	}

	// Single goroutine owns cmd.Wait() — no double-wait possible
	go func() {
		proc.err = cmd.Wait()
		close(proc.done)
		if proc.err != nil {
			stderrStr := stderrBuf.String()
			if stderrStr != "" {
				g.logger.Warnf("GStreamer %s pipeline (PID %d) exited: %v\nStderr: %s", name, cmd.Process.Pid, proc.err, stderrStr)
			} else {
				g.logger.Warnf("GStreamer %s pipeline (PID %d) exited: %v", name, cmd.Process.Pid, proc.err)
			}
		} else {
			g.logger.Infof("GStreamer %s pipeline (PID %d) exited cleanly", name, cmd.Process.Pid)
		}
	}()

	return proc, nil
}

// stopProcess sends SIGTERM, waits up to 2s, then SIGKILL
func (g *GStreamerPipeline) stopProcess(name string, proc *gstProcess) {
	if proc == nil || proc.cmd == nil || proc.cmd.Process == nil {
		return
	}

	pid := proc.cmd.Process.Pid

	// Check if already exited
	select {
	case <-proc.done:
		g.logger.Debugf("GStreamer %s (PID %d) already exited", name, pid)
		return
	default:
	}

	// Send SIGTERM
	_ = proc.cmd.Process.Signal(syscall.SIGTERM)

	select {
	case <-proc.done:
		g.logger.Debugf("GStreamer %s (PID %d) exited after SIGTERM", name, pid)
	case <-time.After(2 * time.Second):
		// Force kill
		_ = proc.cmd.Process.Kill()
		<-proc.done
		g.logger.Debugf("Force-killed GStreamer %s (PID %d)", name, pid)
	}
}

// Start launches both video and audio GStreamer pipelines.
// Video: imxv4l2videosrc -> imxvpuenc_h264 -> rtph264pay -> udpsink
// Audio: alsasrc -> speexenc -> rtpspeexpay -> udpsink
func (g *GStreamerPipeline) Start(cfg GStreamerConfig) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.running {
		g.logger.Warn("GStreamer pipeline already running, stopping first")
		g.stopLocked()
	}

	g.targetIP = cfg.TargetIP
	g.videoPort = cfg.VideoPort
	g.audioPort = cfg.AudioPort
	g.bitrate = cfg.Bitrate

	// Apply defaults for optional fields
	gopSize := cfg.GOPSize
	if gopSize <= 0 {
		gopSize = 7
	}
	idrInterval := cfg.IDRInterval
	if idrInterval <= 0 {
		idrInterval = 1
	}

	// Start video pipeline
	// Camera: imxv4l2videosrc -> /dev/video0, 720x576 UYVY interlaced PAL (~7fps actual)
	// Encoder: imxvpuenc_h264, hardware VPU, Constrained Baseline
	//   gop-size=N: I-frame every N frames
	//   idr-interval=N: IDR every N GOPs (1 = every GOP starts with IDR)
	// RTP: config-interval=1 inserts SPS/PPS with every IDR for mid-stream joining
	g.logger.Infof("Starting video pipeline: imxv4l2videosrc ! imxvpuenc_h264 bitrate=%d gop-size=%d idr-interval=%d ! rtph264pay config-interval=1 ! udpsink %s:%d",
		cfg.Bitrate, gopSize, idrInterval, cfg.TargetIP, cfg.VideoPort)

	var err error
	g.video, err = g.startProcess("video",
		"imxv4l2videosrc",
		"!", "imxvpuenc_h264",
		fmt.Sprintf("bitrate=%d", cfg.Bitrate),
		fmt.Sprintf("gop-size=%d", gopSize),
		fmt.Sprintf("idr-interval=%d", idrInterval),
		"!", "rtph264pay", "config-interval=1", "pt=96",
		"!", "udpsink", fmt.Sprintf("host=%s", cfg.TargetIP), fmt.Sprintf("port=%d", cfg.VideoPort),
	)
	if err != nil {
		return err
	}
	g.logger.Infof("Video pipeline started (PID %d), H.264 RTP -> %s:%d",
		g.video.cmd.Process.Pid, cfg.TargetIP, cfg.VideoPort)

	// Small delay to let video pipeline grab /dev/video0 before starting audio
	time.Sleep(200 * time.Millisecond)

	// Start audio pipeline
	g.logger.Infof("Starting audio pipeline: alsasrc ! speexenc ! rtpspeexpay ! udpsink %s:%d",
		cfg.TargetIP, cfg.AudioPort)

	g.audio, err = g.startProcess("audio",
		"alsasrc", "device=hw:0",
		"!", "audio/x-raw,format=S16LE,rate=8000,channels=1",
		"!", "speexenc",
		"!", "rtpspeexpay",
		"!", "udpsink", fmt.Sprintf("host=%s", cfg.TargetIP), fmt.Sprintf("port=%d", cfg.AudioPort),
	)
	if err != nil {
		g.logger.WithError(err).Warn("Failed to start audio pipeline, continuing with video only")
		// Don't fail entirely — video alone is still useful
	} else {
		g.logger.Infof("Audio pipeline started (PID %d), Speex RTP -> %s:%d",
			g.audio.cmd.Process.Pid, cfg.TargetIP, cfg.AudioPort)
	}

	g.running = true
	g.startTime = time.Now()

	return nil
}

// StartVideoOnly launches only the video pipeline (no audio)
func (g *GStreamerPipeline) StartVideoOnly(cfg GStreamerConfig) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.running {
		g.logger.Warn("GStreamer pipeline already running, stopping first")
		g.stopLocked()
	}

	g.targetIP = cfg.TargetIP
	g.videoPort = cfg.VideoPort
	g.bitrate = cfg.Bitrate

	gopSize := cfg.GOPSize
	if gopSize <= 0 {
		gopSize = 7
	}
	idrInterval := cfg.IDRInterval
	if idrInterval <= 0 {
		idrInterval = 1
	}

	var err error
	g.video, err = g.startProcess("video",
		"imxv4l2videosrc",
		"!", "imxvpuenc_h264",
		fmt.Sprintf("bitrate=%d", cfg.Bitrate),
		fmt.Sprintf("gop-size=%d", gopSize),
		fmt.Sprintf("idr-interval=%d", idrInterval),
		"!", "rtph264pay", "config-interval=1", "pt=96",
		"!", "udpsink", fmt.Sprintf("host=%s", cfg.TargetIP), fmt.Sprintf("port=%d", cfg.VideoPort),
	)
	if err != nil {
		return err
	}
	g.logger.Infof("Video-only pipeline started (PID %d), H.264 RTP -> %s:%d",
		g.video.cmd.Process.Pid, cfg.TargetIP, cfg.VideoPort)

	g.running = true
	g.startTime = time.Now()

	return nil
}

// Stop terminates all running GStreamer pipelines
func (g *GStreamerPipeline) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stopLocked()
}

// stopLocked stops pipelines (must be called with mu held)
func (g *GStreamerPipeline) stopLocked() {
	if !g.running {
		return
	}

	duration := time.Since(g.startTime)
	g.logger.Infof("Stopping GStreamer pipelines (ran for %.1fs)", duration.Seconds())

	// Stop audio first (less critical), then video
	if g.audio != nil {
		g.stopProcess("audio", g.audio)
		g.audio = nil
	}

	if g.video != nil {
		g.stopProcess("video", g.video)
		g.video = nil
	}

	g.running = false
	g.logger.Info("GStreamer pipelines stopped")
}

// IsRunning returns whether pipelines are currently active
func (g *GStreamerPipeline) IsRunning() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running
}

// GetStats returns current pipeline statistics
func (g *GStreamerPipeline) GetStats() map[string]interface{} {
	g.mu.Lock()
	defer g.mu.Unlock()

	stats := map[string]interface{}{
		"running": g.running,
	}

	if g.running {
		stats["uptime_seconds"] = time.Since(g.startTime).Seconds()
		stats["target_ip"] = g.targetIP
		stats["video_port"] = g.videoPort
		stats["audio_port"] = g.audioPort
		stats["bitrate_kbps"] = g.bitrate
	}

	if g.video != nil && g.video.cmd.Process != nil {
		stats["video_pid"] = g.video.cmd.Process.Pid
	}
	if g.audio != nil && g.audio.cmd.Process != nil {
		stats["audio_pid"] = g.audio.cmd.Process.Pid
	}

	return stats
}
