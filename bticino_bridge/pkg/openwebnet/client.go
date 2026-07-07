package openwebnet

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// readOWNMessage reads an OpenWebNet message from a buffered reader.
// OWN protocol uses "##" as message terminator, not newline.
// Returns the complete message including the trailing "##".
func readOWNMessage(reader *bufio.Reader) (string, error) {
	var buf strings.Builder
	prevByte := byte(0)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return buf.String(), err
		}
		buf.WriteByte(b)
		if prevByte == '#' && b == '#' {
			return buf.String(), nil
		}
		prevByte = b
	}
}

// Client represents an OpenWebNet client connection
type Client struct {
	host        string
	password    string // Password for HMAC-SHA256 auth
	ports       []int  // Support multiple ports
	timeout     time.Duration
	retryCount  int
	retryDelay  time.Duration
	connections map[int]net.Conn // Port -> Connection mapping
	connected   map[int]bool     // Port -> Connected status
	mutex       sync.RWMutex
	logger      *logrus.Logger
	cmdDatabase *CommandDatabase
	safetyMgr   *SafetyManager

	// Port specialization
	mainPort   int // 20000 - General OpenWebNet
	videoPort  int // 30007 - Video control
	configPort int // 30006 - Configuration

	// Event handlers
	onMessage    func(*Command)
	onConnected  func(port int)
	onDisconnect func(port int, err error)
	onError      func(error)
}

// ClientConfig contains configuration for OpenWebNet client
type ClientConfig struct {
	Host         string
	Ports        []int  // Multiple ports: [20000, 30006, 30007]
	MainPort     int    // 20000 - General OpenWebNet
	VideoPort    int    // 30007 - Video control
	ConfigPort   int    // 30006 - Configuration
	Password     string // Password for HMAC-SHA256 auth (port 20000)
	Timeout      time.Duration
	RetryCount   int
	RetryDelay   time.Duration
	Logger       *logrus.Logger
	EnableSafety bool // Enable safety manager
}

// NewClient creates a new OpenWebNet client
func NewClient(config ClientConfig) *Client {
	if config.Logger == nil {
		config.Logger = logrus.New()
	}

	// Set default ports if not specified
	if len(config.Ports) == 0 {
		config.Ports = []int{20000, 30006, 30007}
	}
	if config.MainPort == 0 {
		config.MainPort = 20000
	}
	if config.VideoPort == 0 {
		config.VideoPort = 30007
	}
	if config.ConfigPort == 0 {
		config.ConfigPort = 30006
	}

	client := &Client{
		host:        config.Host,
		password:    config.Password,
		ports:       config.Ports,
		timeout:     config.Timeout,
		retryCount:  config.RetryCount,
		retryDelay:  config.RetryDelay,
		logger:      config.Logger,
		cmdDatabase: NewCommandDatabase(),
		connections: make(map[int]net.Conn),
		connected:   make(map[int]bool),
		mainPort:    config.MainPort,
		videoPort:   config.VideoPort,
		configPort:  config.ConfigPort,
	}

	// Initialize safety manager if enabled
	if config.EnableSafety {
		client.safetyMgr = NewSafetyManager(config.Logger)
	}

	return client
}

// Connect establishes connections to all configured OpenWebNet ports
func (c *Client) Connect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.logger.Infof("Connecting to OpenWebNet gateway at %s (ports: %v)", c.host, c.ports)

	var errors []error
	connectedPorts := 0

	for _, port := range c.ports {
		if c.connected[port] {
			connectedPorts++
			continue
		}

		address := fmt.Sprintf("%s:%d", c.host, port)
		c.logger.Infof("Connecting to port %d...", port)

		var err error
		for attempt := 0; attempt <= c.retryCount; attempt++ {
			if attempt > 0 {
				c.logger.Warnf("Port %d connection attempt %d/%d", port, attempt+1, c.retryCount+1)
				time.Sleep(c.retryDelay)
			}

			conn, err := net.DialTimeout("tcp", address, c.timeout)
			if err == nil {
				c.connections[port] = conn
				c.connected[port] = true
				connectedPorts++
				c.logger.Infof("Connected to port %d", port)

				// Read the initial ACK from the server
				conn.SetReadDeadline(time.Now().Add(c.timeout))
				initReader := bufio.NewReader(conn)
				ack, _ := readOWNMessage(initReader)
				c.logger.Debugf("Initial ACK from port %d: %s", port, strings.TrimSpace(ack))

				// Open an EVENT session so the server keeps the connection
				// alive and sends us real-time events (doorbell, etc.)
				if port == c.mainPort {
					c.logger.Infof("Opening EVENT session on port %d...", port)
					_, writeErr := conn.Write([]byte("*99*1##"))
					if writeErr != nil {
						c.logger.Errorf("Failed to send event session request on port %d: %v", port, writeErr)
					} else {
						conn.SetReadDeadline(time.Now().Add(c.timeout))
						sessResp, sessErr := readOWNMessage(initReader)
						if sessErr != nil {
							c.logger.Warnf("No response to event session request on port %d: %v", port, sessErr)
						} else {
							sessResp = strings.TrimSpace(sessResp)
							c.logger.Infof("Event session response on port %d: %s", port, sessResp)

							// Si el servidor requiere autenticacion HMAC-SHA256
							if sessResp == CMD_AUTH_HMAC {
								c.logger.Info("Servidor requiere autenticacion HMAC-SHA256")
								if c.password == "" {
									c.logger.Warn("HMAC requerido pero no hay password configurado, continuando sin auth")
								} else {
									writer := func(data []byte) error {
										conn.SetWriteDeadline(time.Now().Add(c.timeout))
										_, err := conn.Write(data)
										return err
									}
									conn.SetReadDeadline(time.Now().Add(c.timeout))
									if authErr := HMACAuth(initReader, writer, c.password, c.timeout); authErr != nil {
										c.logger.WithError(authErr).Error("Autenticacion HMAC fallida")
									} else {
										c.logger.Infof("Autenticacion HMAC-SHA256 exitosa en puerto %d", port)
									}
								}
							}
						}
					}
				}

				if c.onConnected != nil {
					go c.onConnected(port)
				}

				// Start reading messages in background
				go c.readMessages(port)
				break
			}

			c.logger.Errorf("Port %d connection attempt failed: %v", port, err)
		}

		if err != nil {
			errors = append(errors, fmt.Errorf("port %d: %v", port, err))
		}
	}

	if connectedPorts == 0 {
		return fmt.Errorf("failed to connect to any ports: %v", errors)
	}

	c.logger.Infof("Connected to %d/%d ports", connectedPorts, len(c.ports))
	return nil
}

// Disconnect closes all OpenWebNet connections
func (c *Client) Disconnect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var errors []error
	disconnectedPorts := 0

	for port, conn := range c.connections {
		if conn != nil {
			err := conn.Close()
			if err != nil {
				errors = append(errors, fmt.Errorf("port %d: %v", port, err))
			}
			delete(c.connections, port)
			c.connected[port] = false
			disconnectedPorts++

			c.logger.Infof("Disconnected from port %d", port)

			if c.onDisconnect != nil {
				go c.onDisconnect(port, err)
			}
		}
	}

	if disconnectedPorts > 0 {
		c.logger.Infof("Disconnected from %d ports", disconnectedPorts)
	}

	if len(errors) > 0 {
		return fmt.Errorf("disconnect errors: %v", errors)
	}
	return nil
}

// IsConnected returns true if at least one port is connected
func (c *Client) IsConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	for _, isConnected := range c.connected {
		if isConnected {
			return true
		}
	}
	return false
}

// IsPortConnected returns true if a specific port is connected
func (c *Client) IsPortConnected(port int) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.connected[port]
}

// GetConnectedPorts returns list of currently connected ports
func (c *Client) GetConnectedPorts() []int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	var connectedPorts []int
	for port, isConnected := range c.connected {
		if isConnected {
			connectedPorts = append(connectedPorts, port)
		}
	}
	return connectedPorts
}

// SendCommand sends an OpenWebNet command and returns the response
func (c *Client) SendCommand(cmdStr string) (*Command, error) {
	// Validate command with safety manager if enabled
	if c.safetyMgr != nil {
		validation, err := c.safetyMgr.ValidateCommand(cmdStr, c.cmdDatabase, "", "")
		if err != nil {
			return nil, err
		}
		if !validation.Allowed {
			return nil, fmt.Errorf("command blocked by safety manager: %s", validation.Warning)
		}
		if validation.Warning != "" {
			c.logger.Warn(validation.Warning)
		}
	}

	// Choose the appropriate port for the command
	port := c.choosePortForCommand(cmdStr)

	c.mutex.RLock()
	conn, exists := c.connections[port]
	isConnected := c.connected[port]
	c.mutex.RUnlock()

	if !exists || !isConnected || conn == nil {
		return nil, fmt.Errorf("not connected to port %d", port)
	}

	// Validate command format
	if !IsValidCommand(cmdStr) {
		return nil, fmt.Errorf("invalid command format: %s", cmdStr)
	}

	c.logger.Debugf("Sending command to port %d: %s", port, cmdStr)

	// Send command
	_, err := conn.Write([]byte(cmdStr))
	if err != nil {
		c.logger.Errorf("Failed to send command to port %d: %v", port, err)
		return nil, fmt.Errorf("failed to send command: %v", err)
	}

	// Read response (for synchronous commands)
	// OWN protocol uses "##" as message terminator
	conn.SetReadDeadline(time.Now().Add(c.timeout))
	reader := bufio.NewReader(conn)
	response, err := readOWNMessage(reader)
	if err != nil {
		// Check if it's a timeout or connection error
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			c.logger.Warn("Command response timeout")
			return nil, fmt.Errorf("command timeout")
		}
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	response = strings.TrimSpace(response)
	c.logger.Debugf("Received response from port %d: %s", port, response)

	// Parse response
	cmd, err := ParseCommand(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	// Log command execution with safety manager
	if c.safetyMgr != nil {
		safety := c.safetyMgr.GetCommandSafety(cmdStr, c.cmdDatabase)
		c.safetyMgr.LogCommandExecution(cmdStr, safety, "", "", "success")
	}

	return cmd, nil
}

// SendCommandAsync sends a command without waiting for response
func (c *Client) SendCommandAsync(cmdStr string) error {
	// Validate command with safety manager if enabled
	if c.safetyMgr != nil {
		validation, err := c.safetyMgr.ValidateCommand(cmdStr, c.cmdDatabase, "", "")
		if err != nil {
			return err
		}
		if !validation.Allowed {
			return fmt.Errorf("command blocked by safety manager: %s", validation.Warning)
		}
	}

	// Choose the appropriate port for the command
	port := c.choosePortForCommand(cmdStr)

	c.mutex.RLock()
	conn, exists := c.connections[port]
	isConnected := c.connected[port]
	c.mutex.RUnlock()

	if !exists || !isConnected || conn == nil {
		return fmt.Errorf("not connected to port %d", port)
	}

	if !IsValidCommand(cmdStr) {
		return fmt.Errorf("invalid command format: %s", cmdStr)
	}

	c.logger.Debugf("Sending async command to port %d: %s", port, cmdStr)

	_, err := conn.Write([]byte(cmdStr))
	if err != nil {
		c.logger.Errorf("Failed to send async command to port %d: %v", port, err)
		return fmt.Errorf("failed to send command: %v", err)
	}

	// Log command execution with safety manager
	if c.safetyMgr != nil {
		safety := c.safetyMgr.GetCommandSafety(cmdStr, c.cmdDatabase)
		c.safetyMgr.LogCommandExecution(cmdStr, safety, "", "", "async_sent")
	}

	return nil
}

// choosePortForCommand selects the appropriate port for a command
func (c *Client) choosePortForCommand(cmdStr string) int {
	// Route commands to specialized ports based on command type
	switch {
	case strings.Contains(cmdStr, "*7*300#") || strings.Contains(cmdStr, "*7*77#") || strings.Contains(cmdStr, "*7*220#"):
		// Video stream activation and control commands -> video port (30007)
		if c.IsPortConnected(c.videoPort) {
			return c.videoPort
		}
	case strings.Contains(cmdStr, "*#13**") || strings.Contains(cmdStr, "*13*35*"):
		// Configuration commands -> config port
		if c.IsPortConnected(c.configPort) {
			return c.configPort
		}
	}

	// Default to main port for general commands
	if c.IsPortConnected(c.mainPort) {
		return c.mainPort
	}

	// Fallback to any connected port
	connectedPorts := c.GetConnectedPorts()
	if len(connectedPorts) > 0 {
		return connectedPorts[0]
	}

	// Default to main port (even if not connected - will fail gracefully)
	return c.mainPort
}

// readMessages continuously reads messages from a specific port
func (c *Client) readMessages(port int) {
	c.logger.Infof("Starting message reader for port %d", port)

	c.mutex.RLock()
	conn := c.connections[port]
	c.mutex.RUnlock()

	if conn == nil {
		c.logger.Infof("Message reader for port %d stopped (no connection)", port)
		return
	}

	// Create reader once outside the loop to preserve buffered data
	reader := bufio.NewReader(conn)

	for {
		c.mutex.RLock()
		_, exists := c.connections[port]
		isConnected := c.connected[port]
		c.mutex.RUnlock()

		if !exists || !isConnected {
			c.logger.Infof("Message reader for port %d stopped (disconnected)", port)
			break
		}

		// Set read deadline to detect connection issues
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		message, err := readOWNMessage(reader)
		if err != nil {
			if !c.IsPortConnected(port) {
				// Connection was closed intentionally
				break
			}

			// Check if this is a read timeout (normal on idle monitoring connections)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is expected when the OWN server has no events to report.
				// Just retry the read without reconnecting.
				c.logger.Debugf("Read timeout on port %d (idle), retrying...", port)
				continue
			}

			c.logger.Errorf("Error reading message from port %d: %v", port, err)
			if c.onError != nil {
				go c.onError(err)
			}

			// Try to reconnect this specific port
			c.mutex.Lock()
			if c.connections[port] != nil {
				c.connections[port].Close()
			}
			delete(c.connections, port)
			c.connected[port] = false
			c.mutex.Unlock()

			go c.reconnectPort(port)
			break
		}

		message = strings.TrimSpace(message)
		if message == "" {
			continue
		}

		c.logger.Debugf("Received message from port %d: %s", port, message)

		// Parse and handle message
		cmd, err := ParseCommand(message)
		if err != nil {
			c.logger.Errorf("Failed to parse message from port %d: %v", port, err)
			continue
		}

		if c.onMessage != nil {
			go c.onMessage(cmd)
		}
	}

	c.logger.Infof("Message reader for port %d ended", port)
}

// reconnectPort attempts to reconnect to a specific port
func (c *Client) reconnectPort(port int) {
	c.logger.Infof("Attempting to reconnect to port %d...", port)

	for attempt := 1; attempt <= c.retryCount; attempt++ {
		time.Sleep(c.retryDelay)

		address := fmt.Sprintf("%s:%d", c.host, port)
		conn, err := net.DialTimeout("tcp", address, c.timeout)
		if err == nil {
			c.mutex.Lock()
			c.connections[port] = conn
			c.connected[port] = true
			c.mutex.Unlock()

			c.logger.Infof("Reconnected to port %d successfully", port)

			// Read the initial ACK from the server
			conn.SetReadDeadline(time.Now().Add(c.timeout))
			initReader := bufio.NewReader(conn)
			ack, _ := readOWNMessage(initReader)
			c.logger.Debugf("Initial ACK from port %d: %s", port, strings.TrimSpace(ack))

			// Open an EVENT session on the main monitoring port
			if port == c.mainPort {
				c.logger.Infof("Opening EVENT session on port %d...", port)
				_, writeErr := conn.Write([]byte("*99*1##"))
				if writeErr != nil {
					c.logger.Errorf("Failed to send event session request on port %d: %v", port, writeErr)
				} else {
					conn.SetReadDeadline(time.Now().Add(c.timeout))
					sessResp, sessErr := readOWNMessage(initReader)
					if sessErr != nil {
						c.logger.Warnf("No response to event session request on port %d: %v", port, sessErr)
					} else {
						sessResp = strings.TrimSpace(sessResp)
						c.logger.Infof("Event session response on port %d: %s", port, sessResp)

						// Autenticacion HMAC-SHA256 si requerida
						if sessResp == CMD_AUTH_HMAC {
							c.logger.Info("Reconexion: servidor requiere autenticacion HMAC-SHA256")
							if c.password != "" {
								writer := func(data []byte) error {
									conn.SetWriteDeadline(time.Now().Add(c.timeout))
									_, err := conn.Write(data)
									return err
								}
								conn.SetReadDeadline(time.Now().Add(c.timeout))
								if authErr := HMACAuth(initReader, writer, c.password, c.timeout); authErr != nil {
									c.logger.WithError(authErr).Error("Reconexion: autenticacion HMAC fallida")
								} else {
									c.logger.Infof("Reconexion: autenticacion HMAC-SHA256 exitosa en puerto %d", port)
								}
							}
						}
					}
				}
			}

			if c.onConnected != nil {
				go c.onConnected(port)
			}

			// Restart message reader for this port
			go c.readMessages(port)
			return
		}

		c.logger.Errorf("Reconnection attempt %d to port %d failed: %v", attempt, port, err)
	}

	c.logger.Errorf("Failed to reconnect to port %d after %d attempts", port, c.retryCount)
}

// Event handler setters
func (c *Client) OnMessage(handler func(*Command)) {
	c.onMessage = handler
}

func (c *Client) OnConnected(handler func(port int)) {
	c.onConnected = handler
}

func (c *Client) OnDisconnected(handler func(port int, err error)) {
	c.onDisconnect = handler
}

func (c *Client) OnError(handler func(error)) {
	c.onError = handler
}

// Helper methods for common commands

// OpenDoor sends door open command (CRITICAL SAFETY - requires confirmation)
func (c *Client) OpenDoor(doorID string) error {
	cmd := BuildCommand("8", "19", doorID)
	return c.SendCommandAsync(cmd)
}

// CloseDoor sends door close command
func (c *Client) CloseDoor(doorID string) error {
	cmd := BuildCommand("8", "20", doorID)
	return c.SendCommandAsync(cmd)
}

// QueryDoorStatus requests door status
func (c *Client) QueryDoorStatus(doorID string) (*Command, error) {
	cmd := BuildStatusQuery("1013", "", doorID)
	return c.SendCommand(cmd)
}

// QuerySystemStatus requests system status
func (c *Client) QuerySystemStatus() (*Command, error) {
	return c.SendCommand("*#130**1*2##")
}

// SendHeartbeat sends system heartbeat
func (c *Client) SendHeartbeat() error {
	return c.SendCommandAsync("*#130**1*2##")
}

// OpenSession opens a command session
func (c *Client) OpenSession() (*Command, error) {
	return c.SendCommand("*99*0##")
}

// OpenEventSession opens an event session
func (c *Client) OpenEventSession() (*Command, error) {
	return c.SendCommand("*99*1##")
}

// QueryAudioChannel queries specific audio channel status
func (c *Client) QueryAudioChannel(channel int) (*Command, error) {
	cmd := fmt.Sprintf("*#8**35*%d*0*0##", channel)
	return c.SendCommand(cmd)
}

// QuerySIPStatus queries SIP connection status via bell query
func (c *Client) QuerySIPStatus() (*Command, error) {
	return c.SendCommand("*#8**33##")
}

// QuerySIPCodecStatus queries SIP codec status
func (c *Client) QuerySIPCodecStatus() (*Command, error) {
	return c.SendCommand("*#8**37*3##")
}

// SetVideoParameters sets video stream parameters (requires safety confirmation)
func (c *Client) SetVideoParameters(width, height, bitrate int) error {
	cmd := fmt.Sprintf("*7*77#%d#%d#%d#148#83#0#800#180#10#15#400#288#0#4000*##", width, height, bitrate)
	return c.SendCommandAsync(cmd)
}

// ControlVideo enables/disables video
func (c *Client) ControlVideo(enable bool) error {
	var cmd string
	if enable {
		cmd = "*7*220#1*##"
	} else {
		cmd = "*7*220#0*##"
	}
	return c.SendCommandAsync(cmd)
}

// QueryVideoStatus queries video system status
func (c *Client) QueryVideoStatus() (*Command, error) {
	return c.SendCommand("*7*59#8#0#0*##")
}

// ActivateVideoStream sends *7*300 command to bt_av_media on port 30007
// to start an unencrypted RTP video stream to the specified IP and port.
// streamType: 0=high-res video, 1=low-res video, 2=audio
// IP is encoded as hash: 192.168.1.38 -> 192#168#1#38
func (c *Client) ActivateVideoStream(targetIP string, rtpPort int, highRes bool) error {
	ipHash := strings.ReplaceAll(targetIP, ".", "#")
	streamType := 0
	if !highRes {
		streamType = 1
	}
	cmd := fmt.Sprintf("*7*300#%s#%d#%d*##", ipHash, rtpPort, streamType)
	c.logger.Infof("Activating video stream: %s (target=%s:%d, type=%d)", cmd, targetIP, rtpPort, streamType)

	// SEGURIDAD: envío ÚNICO, sin reintentos. Reintentar *7*300 en bucle contra
	// el estado nativo provocó una tormenta de comandos que hizo clickar el relé
	// de conmutación y disparó el watchdog del sistema (reinicio) el 2026-07-07.
	return c.sendToVideoPortWithRetry(cmd, 1)
}

// ActivateAudioStream sends *7*300 command to start audio stream
func (c *Client) ActivateAudioStream(targetIP string, rtpPort int) error {
	ipHash := strings.ReplaceAll(targetIP, ".", "#")
	cmd := fmt.Sprintf("*7*300#%s#%d#2*##", ipHash, rtpPort)
	c.logger.Infof("Activating audio stream: %s (target=%s:%d)", cmd, targetIP, rtpPort)

	// SEGURIDAD: envío único, sin reintentos (ver ActivateVideoStream)
	return c.sendToVideoPortWithRetry(cmd, 1)
}

// sendToVideoPortWithRetry sends a command to port 30007 with retry logic.
// Per slyoldfox's bt-av-media.js: NO initial handshake/ACK is read from port 30007.
// The command is sent immediately after TCP connect, and the response is then read.
// On NACK (*#*0##), retry up to maxRetries times with 1s delay, using a fresh connection each time.
func (c *Client) sendToVideoPortWithRetry(cmd string, maxRetries int) error {
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Warnf("Retry %d/%d for command %s", attempt+1, maxRetries, cmd)
			time.Sleep(1 * time.Second)
		}

		// Connect directly to port 30007 for video commands
		// Use a fresh TCP connection for each attempt (like slyoldfox)
		address := fmt.Sprintf("%s:%d", c.host, c.videoPort)
		conn, err := net.DialTimeout("tcp", address, 5*time.Second)
		if err != nil {
			c.logger.Errorf("Failed to connect to video port %d: %v", c.videoPort, err)
			continue
		}

		// Send the command IMMEDIATELY after connect — no handshake on port 30007
		// (slyoldfox: client.once('connect', () => { client.write(seq) }))
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err = conn.Write([]byte(cmd))
		if err != nil {
			conn.Close()
			c.logger.Errorf("Failed to write to video port: %v", err)
			continue
		}
		c.logger.Debugf("Sent to video port %d: %s", c.videoPort, cmd)

		// Read response
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(conn)
		response, err := readOWNMessage(reader)
		if err != nil {
			conn.Close()
			c.logger.Errorf("No response from video port: %v", err)
			continue
		}

		response = strings.TrimSpace(response)
		c.logger.Infof("Video port response: %s", response)

		// Check for ACK (*#*1## or *#*1##*#*1## double ACK)
		if strings.Contains(response, "*#*1##") {
			c.logger.Infof("Video stream command succeeded: %s", cmd)
			// Keep connection alive with idle timeout (like slyoldfox's 5s setTimeout)
			// The stream will be sent as long as this connection (or the SIP call) is alive
			go func() {
				time.Sleep(5 * time.Second)
				conn.Close()
			}()
			return nil
		}

		// NACK (*#*0##) - destroy connection and retry with fresh one (like slyoldfox)
		if strings.Contains(response, "*#*0##") {
			c.logger.Warnf("NACK received for %s, will retry", cmd)
			conn.Close()
			continue
		}

		// Unknown/unsupported response — abort (like slyoldfox: client.destroy())
		c.logger.Warnf("Unsupported response for %s: %s, aborting", cmd, response)
		conn.Close()
		return fmt.Errorf("unsupported response from video port: %s", response)
	}

	return fmt.Errorf("video stream command failed after %d attempts: %s", maxRetries, cmd)
}

// GetCommandDatabase returns the command database
func (c *Client) GetCommandDatabase() *CommandDatabase {
	return c.cmdDatabase
}

// GetSafetyManager returns the safety manager (if enabled)
func (c *Client) GetSafetyManager() *SafetyManager {
	return c.safetyMgr
}

// EnableCriticalCommands enables/disables critical command execution
func (c *Client) EnableCriticalCommands(enable bool) {
	if c.safetyMgr != nil {
		c.safetyMgr.EnableCriticalCommands(enable)
	}
}

// SendCommandToPort sends a command to a specific port
func (c *Client) SendCommandToPort(cmdStr string, port int) (*Command, error) {
	c.mutex.RLock()
	conn, exists := c.connections[port]
	isConnected := c.connected[port]
	c.mutex.RUnlock()

	if !exists || !isConnected || conn == nil {
		return nil, fmt.Errorf("not connected to port %d", port)
	}

	// Validate command format
	if !IsValidCommand(cmdStr) {
		return nil, fmt.Errorf("invalid command format: %s", cmdStr)
	}

	// Validate with safety manager if enabled
	if c.safetyMgr != nil {
		validation, err := c.safetyMgr.ValidateCommand(cmdStr, c.cmdDatabase, "", "")
		if err != nil {
			return nil, err
		}
		if !validation.Allowed {
			return nil, fmt.Errorf("command blocked: %s", validation.Warning)
		}
	}

	c.logger.Debugf("Sending command to specific port %d: %s", port, cmdStr)

	// Send command
	_, err := conn.Write([]byte(cmdStr))
	if err != nil {
		return nil, fmt.Errorf("failed to send command to port %d: %v", port, err)
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(c.timeout))
	reader := bufio.NewReader(conn)
	response, err := readOWNMessage(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from port %d: %v", port, err)
	}

	response = strings.TrimSpace(response)
	c.logger.Debugf("Received response from port %d: %s", port, response)

	return ParseCommand(response)
}
