package sip

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"bticino_bridge/pkg/events"
	"bticino_bridge/pkg/openwebnet"
)

// VideoStreamManager handles H.264 video streaming for BTicino
type VideoStreamManager struct {
	sipClient     *BTicinoSIPClient
	openWebClient *openwebnet.Client
	eventBus      events.EventBus
	logger        *logrus.Logger

	// Stream management
	activeStreams map[string]*VideoStreamInfo
	rtpListener   net.PacketConn
	mutex         sync.RWMutex
	running       bool
	stopCh        chan struct{}
}

// NewVideoStreamManager creates a new video stream manager
func NewVideoStreamManager(sipClient *BTicinoSIPClient, openWebClient *openwebnet.Client,
	eventBus events.EventBus, logger *logrus.Logger) *VideoStreamManager {

	if logger == nil {
		logger = logrus.New()
	}

	return &VideoStreamManager{
		sipClient:     sipClient,
		openWebClient: openWebClient,
		eventBus:      eventBus,
		logger:        logger,
		activeStreams: make(map[string]*VideoStreamInfo),
		stopCh:        make(chan struct{}),
	}
}

// Start begins video stream management
func (v *VideoStreamManager) Start() error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if v.running {
		return fmt.Errorf("video stream manager already running")
	}

	v.logger.Info("Starting video stream manager")

	// Setup RTP listener for H.264 video
	config := v.sipClient.GetConfig()
	rtpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", config.MediaPorts.VideoRTP))
	if err != nil {
		return fmt.Errorf("failed to resolve RTP address: %v", err)
	}

	v.rtpListener, err = net.ListenUDP("udp", rtpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on RTP port %d: %v", config.MediaPorts.VideoRTP, err)
	}

	// Start RTP packet handler
	go v.handleRTPPackets()

	// Subscribe to doorbell events to start video streaming
	v.eventBus.Subscribe("doorbell.pressed", v.onDoorbellPressed)
	v.eventBus.Subscribe("door.unlocked", v.onDoorUnlocked)

	v.running = true
	v.logger.Infof("Video stream manager started on RTP port %d", config.MediaPorts.VideoRTP)

	return nil
}

// Stop gracefully shuts down the video stream manager
func (v *VideoStreamManager) Stop() error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if !v.running {
		return nil
	}

	v.logger.Info("Stopping video stream manager")

	// Stop all active streams
	for streamID, stream := range v.activeStreams {
		v.stopVideoStream(streamID)
		v.logger.Infof("Stopped video stream: %s", streamID)
		_ = stream // avoid unused variable
	}

	// Close RTP listener
	if v.rtpListener != nil {
		v.rtpListener.Close()
	}

	// Signal stop
	close(v.stopCh)

	v.running = false
	v.logger.Info("Video stream manager stopped")

	return nil
}

// StartVideoStream initiates a video stream using OpenWebNet commands
func (v *VideoStreamManager) StartVideoStream(reason string) (*VideoStreamInfo, error) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	config := v.sipClient.GetConfig()
	streamID := fmt.Sprintf("stream_%d", time.Now().Unix())

	// Create stream info
	streamInfo := &VideoStreamInfo{
		LocalIP:   config.LocalIP,
		LocalPort: config.MediaPorts.VideoRTP,
		Codec:     "H264",
		StartTime: time.Now(),
		RTSPUrl:   fmt.Sprintf("rtsp://%s:%d/%s", config.LocalIP, config.RTSPPort, streamID),
		Active:    false,
	}

	// Connect to OpenWebNet
	if err := v.openWebClient.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to OpenWebNet: %v", err)
	}
	defer v.openWebClient.Disconnect()

	// Send OpenWebNet command to start video stream
	// Format: *7*32#<ip_with_dots_as_hash>#<port>*##
	ipFormatted := strings.Replace(config.LocalIP, ".", "#", -1)
	videoCmd := fmt.Sprintf("*7*32#%s#%d*##", ipFormatted, config.MediaPorts.VideoRTP)

	v.logger.Infof("Sending OpenWebNet video command: %s (reason: %s)", videoCmd, reason)

	response, err := v.openWebClient.SendCommand(videoCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to send video command: %v", err)
	}

	// Check response
	if response.Raw == "*#*1##" { // ACK
		streamInfo.Active = true
		v.activeStreams[streamID] = streamInfo

		// Publish event
		v.eventBus.PublishWithSource("video.stream.started", map[string]interface{}{
			"stream_id":  streamID,
			"reason":     reason,
			"rtsp_url":   streamInfo.RTSPUrl,
			"local_ip":   streamInfo.LocalIP,
			"local_port": streamInfo.LocalPort,
			"codec":      streamInfo.Codec,
		}, "video")

		v.logger.Infof("Video stream started successfully: %s", streamID)
		return streamInfo, nil
	} else {
		return nil, fmt.Errorf("video command failed with response: %s", response.Raw)
	}
}

// StopVideoStream stops a specific video stream
func (v *VideoStreamManager) StopVideoStream(streamID string) error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	return v.stopVideoStream(streamID)
}

// stopVideoStream internal method (assumes mutex is held)
func (v *VideoStreamManager) stopVideoStream(streamID string) error {
	stream, exists := v.activeStreams[streamID]
	if !exists {
		return fmt.Errorf("stream not found: %s", streamID)
	}

	// Connect to OpenWebNet
	if err := v.openWebClient.Connect(); err != nil {
		v.logger.WithError(err).Error("Failed to connect to OpenWebNet for stream stop")
		return err
	}
	defer v.openWebClient.Disconnect()

	// Send stop command
	stopCmd := "*7*0*##"
	v.logger.Infof("Sending OpenWebNet stop video command: %s", stopCmd)

	response, err := v.openWebClient.SendCommand(stopCmd)
	if err != nil {
		v.logger.WithError(err).Error("Failed to send stop video command")
	} else if response.Raw == "*#*1##" {
		v.logger.Info("Video stream stop command acknowledged")
	}

	// Mark stream as inactive
	stream.Active = false
	delete(v.activeStreams, streamID)

	// Publish event
	v.eventBus.PublishWithSource("video.stream.stopped", map[string]interface{}{
		"stream_id": streamID,
		"duration":  time.Since(stream.StartTime).Seconds(),
	}, "video")

	return nil
}

// StopAllVideoStreams stops all active video streams
func (v *VideoStreamManager) StopAllVideoStreams() error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	for streamID := range v.activeStreams {
		if err := v.stopVideoStream(streamID); err != nil {
			v.logger.WithError(err).WithField("stream_id", streamID).Error("Failed to stop video stream")
		}
	}

	return nil
}

// GetActiveStreams returns information about all active video streams
func (v *VideoStreamManager) GetActiveStreams() map[string]*VideoStreamInfo {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	// Create a copy to avoid race conditions
	streams := make(map[string]*VideoStreamInfo)
	for id, stream := range v.activeStreams {
		streamCopy := *stream
		streams[id] = &streamCopy
	}

	return streams
}

// handleRTPPackets processes incoming H.264 RTP packets
func (v *VideoStreamManager) handleRTPPackets() {
	v.logger.Info("Starting RTP packet handler")

	buffer := make([]byte, 1500) // Standard MTU size

	for {
		select {
		case <-v.stopCh:
			v.logger.Info("RTP packet handler stopping")
			return
		default:
		}

		// Set read timeout to allow periodic checks for stop signal
		v.rtpListener.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, addr, err := v.rtpListener.ReadFrom(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is expected, continue
				continue
			}
			if v.running {
				v.logger.WithError(err).Error("Error reading RTP packet")
			}
			continue
		}

		// Process RTP packet
		v.processRTPPacket(buffer[:n], addr)
	}
}

// processRTPPacket handles a single H.264 RTP packet
func (v *VideoStreamManager) processRTPPacket(packet []byte, addr net.Addr) {
	if len(packet) < 12 {
		return // Invalid RTP packet
	}

	// Parse basic RTP header
	version := (packet[0] >> 6) & 0x03
	payloadType := packet[1] & 0x7F
	sequenceNumber := uint16(packet[2])<<8 | uint16(packet[3])
	timestamp := uint32(packet[4])<<24 | uint32(packet[5])<<16 | uint32(packet[6])<<8 | uint32(packet[7])

	if version != 2 {
		return // Not RTP version 2
	}

	v.logger.Debugf("Received H.264 RTP packet: PT=%d, Seq=%d, TS=%d, Size=%d, From=%s",
		payloadType, sequenceNumber, timestamp, len(packet), addr.String())

	// Publish RTP packet event for RTSP server or other consumers
	v.eventBus.PublishWithSource("video.rtp.packet", map[string]interface{}{
		"payload_type":    payloadType,
		"sequence_number": sequenceNumber,
		"timestamp":       timestamp,
		"packet_size":     len(packet),
		"source_addr":     addr.String(),
	}, "video")

	// TODO: Forward to RTSP server when implemented
}

// Event handlers

func (v *VideoStreamManager) onDoorbellPressed(event *events.Event) {
	v.logger.Info("Doorbell pressed - starting video stream")

	stream, err := v.StartVideoStream("doorbell_press")
	if err != nil {
		v.logger.WithError(err).Error("Failed to start video stream on doorbell press")
		return
	}

	v.logger.Infof("Video stream started for doorbell: %s", stream.RTSPUrl)

	// Auto-stop after 60 seconds
	go func() {
		time.Sleep(60 * time.Second)
		v.mutex.RLock()
		streamID := ""
		for id, s := range v.activeStreams {
			if s == stream {
				streamID = id
				break
			}
		}
		v.mutex.RUnlock()

		if streamID != "" {
			if err := v.StopVideoStream(streamID); err != nil {
				v.logger.WithError(err).Error("Failed to auto-stop video stream")
			} else {
				v.logger.Info("Auto-stopped video stream after 60 seconds")
			}
		}
	}()
}

func (v *VideoStreamManager) onDoorUnlocked(event *events.Event) {
	v.logger.Info("Door unlocked - extending video stream if active")

	// If there are active streams, extend them by restarting
	streams := v.GetActiveStreams()
	if len(streams) > 0 {
		v.logger.Infof("Extending %d active video streams", len(streams))
		// The streams will continue running, this could trigger additional recording logic

		v.eventBus.PublishWithSource("video.stream.extended", map[string]interface{}{
			"reason":       "door_unlocked",
			"active_count": len(streams),
		}, "video")
	}
}

// GetStats returns statistics about video streaming
func (v *VideoStreamManager) GetStats() map[string]interface{} {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	return map[string]interface{}{
		"running":        v.running,
		"active_streams": len(v.activeStreams),
		"rtp_port":       v.sipClient.config.MediaPorts.VideoRTP,
		"rtsp_port":      v.sipClient.config.RTSPPort,
	}
}
