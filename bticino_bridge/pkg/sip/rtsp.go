package sip

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"bticino_bridge/pkg/events"
)

type RTSPServer struct {
	port          int
	listener      net.Listener
	sipClient     *BTicinoSIPClient
	eventBus      events.EventBus
	logger        *logrus.Logger
	sessions      map[string]*RTSPSession
	mutex         sync.RWMutex
	running       bool
	stopCh        chan struct{}
	callActive    bool
	activeClients int
}

type RTSPSession struct {
	ID           string
	ClientAddr   string
	StreamPath   string
	State        RTSPSessionState
	SetupTime    time.Time
	LastActivity time.Time
	CSeq         int
	SessionID    string
	ClientPorts  RTSPPortPair
	ServerPorts  RTSPPortPair
	Transport    string
	RTPConn      *net.UDPConn

	// Transport mode for RTP relay
	IsInterleaved    bool     // true for TCP interleaved, false for UDP
	TCPConn          net.Conn // TCP connection for interleaved mode
	InterleavedVideo uint8    // RTP channel for video (interleaved)
	InterleavedAudio uint8    // RTP channel for audio (interleaved)
	TrackSetup       string   // "video", "audio", or "" (both)

	// Per-track UDP ports (set during individual SETUP requests)
	VideoClientPorts RTSPPortPair // client RTP/RTCP ports for video track
	AudioClientPorts RTSPPortPair // client RTP/RTCP ports for audio track
}

type RTSPSessionState int

const (
	RTSPInit RTSPSessionState = iota
	RTSPReady
	RTSPPlaying
)

func (s RTSPSessionState) String() string {
	switch s {
	case RTSPInit:
		return "INIT"
	case RTSPReady:
		return "READY"
	case RTSPPlaying:
		return "PLAYING"
	default:
		return "UNKNOWN"
	}
}

type RTSPPortPair struct {
	RTP  int
	RTCP int
}

type RTSPRequest struct {
	Method    string
	URL       string
	Version   string
	Headers   map[string]string
	Body      string
	CSeq      int
	Session   string
	Transport string
}

func NewRTSPServer(port int, sipClient *BTicinoSIPClient, eventBus events.EventBus, logger *logrus.Logger) *RTSPServer {
	if logger == nil {
		logger = logrus.New()
	}

	return &RTSPServer{
		port:      port,
		sipClient: sipClient,
		eventBus:  eventBus,
		logger:    logger,
		sessions:  make(map[string]*RTSPSession),
		stopCh:    make(chan struct{}),
	}
}

func (r *RTSPServer) Start() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.running {
		return fmt.Errorf("RTSP server already running")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", r.port))
	if err != nil {
		return fmt.Errorf("failed to start RTSP server: %v", err)
	}

	r.listener = listener
	r.running = true

	r.logger.Infof("RTSP server started on port %d", r.port)
	r.logger.Infof("RTSP streams will initiate SIP calls to: %s", r.sipClient.config.ServerAddr)

	go r.acceptConnections()
	go r.sessionCleanup()

	return nil
}

func (r *RTSPServer) Stop() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.running {
		return nil
	}

	r.logger.Info("Stopping RTSP server")

	if r.listener != nil {
		r.listener.Close()
	}

	if r.callActive {
		r.logger.Info("Ending active SIP call due to server stop")
		r.sipClient.Hangup()
	}

	close(r.stopCh)

	r.running = false
	r.logger.Info("RTSP server stopped")

	return nil
}

func (r *RTSPServer) acceptConnections() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			select {
			case <-r.stopCh:
				return
			default:
				r.logger.WithError(err).Error("Failed to accept RTSP connection")
				continue
			}
		}

		go r.handleConnection(conn)
	}
}

func (r *RTSPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()
	r.logger.Infof("New RTSP connection from %s", clientAddr)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 65536)
	var requestLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if len(requestLines) > 0 {
				request := r.parseRTSPRequest(requestLines)
				if request != nil {
					response := r.handleRTSPRequest(request, clientAddr, conn)
					if _, err := conn.Write([]byte(response)); err != nil {
						r.logger.WithError(err).Error("Failed to send RTSP response")
						return
					}
				}
			}
			requestLines = nil
		} else {
			requestLines = append(requestLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		r.logger.WithError(err).Debug("RTSP connection scanner error")
	}

	r.logger.Debugf("RTSP connection closed: %s", clientAddr)
}

func (r *RTSPServer) parseRTSPRequest(lines []string) *RTSPRequest {
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

func (r *RTSPServer) handleRTSPRequest(request *RTSPRequest, clientAddr string, conn net.Conn) string {
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
		return r.handleSetup(request, clientAddr)
	case "PLAY":
		return r.handlePlay(request, conn)
	case "TEARDOWN":
		return r.handleTeardown(request)
	default:
		return r.buildErrorResponse(501, "Not Implemented", request.CSeq)
	}
}

func (r *RTSPServer) handleOptions(request *RTSPRequest) string {
	return r.buildResponse(200, "OK", request.CSeq, map[string]string{
		"Public": "DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE",
	})
}

func (r *RTSPServer) handleDescribe(request *RTSPRequest) string {
	streamPath := r.extractStreamPath(request.URL)

	if !r.isValidStream(streamPath) {
		return r.buildErrorResponse(404, "Stream Not Found", request.CSeq)
	}

	sdp := r.buildSDP(streamPath)

	headers := map[string]string{
		"Content-Type":   "application/sdp",
		"Content-Length": strconv.Itoa(len(sdp)),
	}

	return r.buildResponse(200, "OK", request.CSeq, headers) + sdp
}

func (r *RTSPServer) handleSetup(request *RTSPRequest, clientAddr string) string {
	if request.Transport == "" {
		return r.buildErrorResponse(400, "Transport Required", request.CSeq)
	}

	clientPorts := r.parseClientPorts(request.Transport)
	if clientPorts.RTP == 0 {
		return r.buildErrorResponse(400, "Invalid Transport", request.CSeq)
	}

	streamPath := r.extractStreamPath(request.URL)

	sessionID := r.generateSessionID()
	session := &RTSPSession{
		ID:           sessionID,
		ClientAddr:   clientAddr,
		StreamPath:   streamPath,
		State:        RTSPReady,
		SetupTime:    time.Now(),
		LastActivity: time.Now(),
		SessionID:    sessionID,
		ClientPorts:  clientPorts,
	}

	r.mutex.Lock()
	r.sessions[sessionID] = session
	r.mutex.Unlock()

	r.mutex.Lock()
	r.activeClients++
	r.mutex.Unlock()

	r.logger.Infof("RTSP session created: %s for %s (total clients: %d)", sessionID, clientAddr, r.activeClients)

	if err := r.ensureSIPCallActive(); err != nil {
		r.logger.WithError(err).Warn("Failed to ensure SIP call active")
	}

	r.eventBus.PublishWithSource("rtsp.session.setup", map[string]interface{}{
		"session_id":     sessionID,
		"client_addr":    clientAddr,
		"stream_path":    streamPath,
		"active_clients": r.activeClients,
	}, "rtsp")

	config := r.sipClient.GetConfig()
	serverPort := config.MediaPorts.VideoRTP

	transportResponse := fmt.Sprintf("RTP/AVP;unicast;client_port=%d-%d;server_port=%d",
		clientPorts.RTP, clientPorts.RTCP, serverPort)

	headers := map[string]string{
		"Transport": transportResponse,
		"Session":   sessionID,
	}

	return r.buildResponse(200, "OK", request.CSeq, headers)
}

func (r *RTSPServer) handlePlay(request *RTSPRequest, conn net.Conn) string {
	if request.Session == "" {
		return r.buildErrorResponse(400, "Session Required", request.CSeq)
	}

	r.mutex.Lock()
	session, exists := r.sessions[request.Session]
	if exists {
		session.State = RTSPPlaying
		session.LastActivity = time.Now()
	}
	r.mutex.Unlock()

	if !exists {
		return r.buildErrorResponse(454, "Session Not Found", request.CSeq)
	}

	r.logger.Infof("RTSP session playing: %s", session.ID)

	r.eventBus.PublishWithSource("rtsp.session.playing", map[string]interface{}{
		"session_id":  session.ID,
		"client_addr": session.ClientAddr,
	}, "rtsp")

	headers := map[string]string{
		"Session": request.Session,
		"Range":   "npt=0.000-",
	}

	return r.buildResponse(200, "OK", request.CSeq, headers)
}

func (r *RTSPServer) handleTeardown(request *RTSPRequest) string {
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

	duration := time.Since(session.SetupTime).Seconds()
	r.logger.Infof("RTSP session torn down: %s (duration: %.1fs, remaining clients: %d)",
		session.ID, duration, r.activeClients)

	r.eventBus.PublishWithSource("rtsp.session.teardown", map[string]interface{}{
		"session_id":     session.ID,
		"client_addr":    session.ClientAddr,
		"duration":       duration,
		"active_clients": r.activeClients,
	}, "rtsp")

	if r.activeClients == 0 && r.callActive {
		r.logger.Info("No more RTSP clients, ending SIP call")
		go func() {
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

func (r *RTSPServer) buildSDP(streamPath string) string {
	config := r.sipClient.GetConfig()
	localIP := config.LocalIP

	return fmt.Sprintf(`v=0
o=- 0 0 IN IP4 %s
s=BTicino Video Doorbell
c=IN IP4 %s
t=0 0
a=tool:BTicino-Bridge-RTSP
a=type:broadcast
m=video 0 RTP/AVP 96
a=rtpmap:96 H264/90000
a=fmtp:96 profile-level-id=42801F;packetization-mode=1
a=control:%s
m=audio 0 RTP/AVP 110
a=rtpmap:110 speex/8000
a=control:%s
`, localIP, localIP, streamPath, streamPath)
}

func (r *RTSPServer) buildResponse(code int, status string, cseq int, headers map[string]string) string {
	lines := []string{
		fmt.Sprintf("RTSP/1.0 %d %s", code, status),
		fmt.Sprintf("CSeq: %d", cseq),
		fmt.Sprintf("Date: %s", time.Now().UTC().Format(time.RFC1123)),
		"Server: BTicino-Bridge-RTSP/1.0",
	}

	for key, value := range headers {
		lines = append(lines, fmt.Sprintf("%s: %s", key, value))
	}

	lines = append(lines, "", "")
	return strings.Join(lines, "\r\n")
}

func (r *RTSPServer) buildErrorResponse(code int, status string, cseq int) string {
	return r.buildResponse(code, status, cseq, map[string]string{})
}

func (r *RTSPServer) parseClientPorts(transport string) RTSPPortPair {
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
	return RTSPPortPair{}
}

func (r *RTSPServer) extractStreamPath(url string) string {
	path := strings.TrimPrefix(url, "rtsp://")
	if slashIndex := strings.Index(path, "/"); slashIndex >= 0 {
		return path[slashIndex:]
	}
	return "/" + path
}

func (r *RTSPServer) isValidStream(path string) bool {
	validPaths := []string{"/doorbell", "/doorbell-video", "/doorbell-recorder", "/video", "/stream"}
	for _, valid := range validPaths {
		if path == valid || strings.HasPrefix(path, valid) {
			return true
		}
	}
	return true
}

func (r *RTSPServer) generateSessionID() string {
	return fmt.Sprintf("%s-%d", time.Now().Format("20060102150405"), time.Now().UnixNano()%10000)
}

func (r *RTSPServer) sessionCleanup() {
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

func (r *RTSPServer) ensureSIPCallActive() error {
	r.mutex.Lock()
	if r.callActive {
		r.mutex.Unlock()
		return nil
	}
	r.mutex.Unlock()

	r.logger.Info("Initiating SIP call for RTSP streaming")

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

	r.logger.Infof("SIP call initiated to %s for RTSP streaming", target)
	return nil
}

func (r *RTSPServer) GetActiveSessions() map[string]*RTSPSession {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	sessions := make(map[string]*RTSPSession)
	for id, session := range r.sessions {
		sessionCopy := *session
		sessions[id] = &sessionCopy
	}

	return sessions
}

func (r *RTSPServer) GetStats() map[string]interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return map[string]interface{}{
		"running":         r.running,
		"port":            r.port,
		"active_sessions": len(r.sessions),
		"active_clients":  r.activeClients,
		"call_active":     r.callActive,
	}
}

type RTPPacket struct {
	Version        uint8
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	Payload        []byte
}

func ParseRTPPacket(data []byte) (*RTPPacket, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("packet too short")
	}

	packet := &RTPPacket{
		Version:        (data[0] >> 6) & 0x03,
		PayloadType:    data[1] & 0x7F,
		SequenceNumber: binary.BigEndian.Uint16(data[2:4]),
		Timestamp:      binary.BigEndian.Uint32(data[4:8]),
		SSRC:           binary.BigEndian.Uint32(data[8:12]),
	}

	if len(data) > 12 {
		packet.Payload = data[12:]
	}

	return packet, nil
}

func CreateRTPPacket(payloadType uint8, seq uint16, timestamp uint32, ssrc uint32, payload []byte) []byte {
	packet := make([]byte, 12+len(payload))

	packet[0] = 0x80
	packet[1] = payloadType & 0x7F
	binary.BigEndian.PutUint16(packet[2:4], seq)
	binary.BigEndian.PutUint32(packet[4:8], timestamp)
	binary.BigEndian.PutUint32(packet[8:12], ssrc)

	copy(packet[12:], payload)

	return packet
}
