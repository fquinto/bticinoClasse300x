package multicast

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"bticino_bridge/pkg/events"
)

const (
	// BTicino multicast configuration (from slyoldfox analysis)
	MulticastAddr = "239.255.76.67"
	MulticastPort = 7667
)

// BTicinoMessage represents a parsed syslog message from BTicino
type BTicinoMessage struct {
	Timestamp time.Time `json:"timestamp"`
	System    string    `json:"system"`
	Raw       string    `json:"raw"`
	Message   string    `json:"message"`
	Parsed    bool      `json:"parsed"`
}

// MessageHandler defines the interface for handling different types of messages
type MessageHandler interface {
	Handle(listener interface{}, system string, message string) bool
	GetSystemName() string
}

// MulticastListener listens for BTicino multicast events on UDP port 7667
type MulticastListener struct {
	conn       *net.UDPConn
	eventBus   events.EventBus
	handlers   map[string]MessageHandler
	running    bool
	mutex      sync.RWMutex
	logger     *logrus.Logger
	stopCh     chan struct{}
	wg         sync.WaitGroup
	statistics Stats
}

// Stats contains statistics about multicast messages
type Stats struct {
	TotalMessages     int64            `json:"total_messages"`
	MessagesBySystem  map[string]int64 `json:"messages_by_system"`
	UnhandledMessages int64            `json:"unhandled_messages"`
	ErrorCount        int64            `json:"error_count"`
	LastMessageTime   time.Time        `json:"last_message_time"`
	StartTime         time.Time        `json:"start_time"`
}

// NewMulticastListener creates a new multicast listener
func NewMulticastListener(eventBus events.EventBus, logger *logrus.Logger) *MulticastListener {
	if logger == nil {
		logger = logrus.New()
	}

	return &MulticastListener{
		eventBus: eventBus,
		handlers: make(map[string]MessageHandler),
		logger:   logger,
		stopCh:   make(chan struct{}),
		statistics: Stats{
			MessagesBySystem: make(map[string]int64),
			StartTime:        time.Now(),
		},
	}
}

// RegisterHandler registers a message handler for a specific system
func (ml *MulticastListener) RegisterHandler(system string, handler MessageHandler) {
	ml.mutex.Lock()
	defer ml.mutex.Unlock()

	ml.handlers[system] = handler
	ml.logger.Infof("Registered handler for system: %s", system)
}

// Start begins listening for multicast messages
func (ml *MulticastListener) Start() error {
	ml.mutex.Lock()
	defer ml.mutex.Unlock()

	if ml.running {
		return fmt.Errorf("multicast listener already running")
	}

	// Parse multicast group address
	groupAddr := net.ParseIP(MulticastAddr)
	if groupAddr == nil {
		return fmt.Errorf("invalid multicast address: %s", MulticastAddr)
	}

	// Find the best interface for multicast (prefer wlan0 on BTicino)
	var mcastIface *net.Interface
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("failed to get network interfaces: %v", err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagMulticast != 0 {
			// Prefer wlan0 (BTicino device), then any non-loopback
			if iface.Name == "wlan0" {
				mcastIface = &iface
				break
			}
			if mcastIface == nil && iface.Flags&net.FlagLoopback == 0 {
				mcastIface = &iface
			}
		}
	}

	// Use net.ListenMulticastUDP which sets SO_REUSEADDR automatically,
	// allowing us to share port 7667 with bt_daemon
	udpAddr := &net.UDPAddr{
		IP:   groupAddr,
		Port: MulticastPort,
	}
	ml.conn, err = net.ListenMulticastUDP("udp4", mcastIface, udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen multicast UDP (SO_REUSEADDR): %v", err)
	}

	// Also set SO_REUSEPORT via raw file descriptor for maximum compatibility
	rawConn, err := ml.conn.SyscallConn()
	if err == nil {
		rawConn.Control(func(fd uintptr) {
			if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
				ml.logger.Warnf("Failed to set SO_REUSEPORT (non-fatal): %v", err)
			} else {
				ml.logger.Debug("SO_REUSEPORT set successfully on multicast socket")
			}
		})
	}

	// Set read buffer size
	ml.conn.SetReadBuffer(65536)

	ifaceName := "default"
	if mcastIface != nil {
		ifaceName = mcastIface.Name
	}

	ml.running = true
	ml.logger.Infof("MulticastListener started on %s:%d (interface: %s, SO_REUSEADDR+SO_REUSEPORT)",
		MulticastAddr, MulticastPort, ifaceName)

	// Start message processing goroutine
	ml.wg.Add(1)
	go ml.messageHandler()

	return nil
}

// messageHandler processes incoming multicast messages
func (ml *MulticastListener) messageHandler() {
	defer ml.wg.Done()

	ml.logger.Info("Message handler started")

	buffer := make([]byte, 1024)

	for {
		select {
		case <-ml.stopCh:
			ml.logger.Info("Message handler stopping")
			return
		default:
		}

		// Set read timeout to allow periodic checks for stop signal
		ml.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, remoteAddr, err := ml.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is expected, continue
				continue
			}
			if ml.running {
				ml.logger.Errorf("Error reading UDP message: %v", err)
				ml.statistics.ErrorCount++
			}
			continue
		}

		// Process the message
		data := string(buffer[:n])
		ml.processMessage(data, remoteAddr)
	}
}

// processMessage parses and routes a single message
func (ml *MulticastListener) processMessage(data string, remoteAddr *net.UDPAddr) {
	ml.mutex.Lock()
	ml.statistics.TotalMessages++
	ml.statistics.LastMessageTime = time.Now()
	ml.mutex.Unlock()

	ml.logger.Debugf("Received message from %s: %s", remoteAddr.String(), strings.TrimSpace(data))

	// Parse the message
	message := ml.parseMessage(data)

	// Publish raw message event
	ml.eventBus.PublishWithSource("multicast.message.raw", message, "multicast")

	// Try to find appropriate handler
	if handler, exists := ml.handlers[message.System]; exists {
		handled := handler.Handle(ml, message.System, message.Message)
		if handled {
			ml.eventBus.PublishWithSource(fmt.Sprintf("multicast.message.%s", message.System), message, "multicast")
		} else {
			ml.mutex.Lock()
			ml.statistics.UnhandledMessages++
			ml.mutex.Unlock()
			ml.logger.Debugf("Message not handled by %s handler: %s", message.System, message.Message)
		}
	} else {
		ml.mutex.Lock()
		ml.statistics.UnhandledMessages++
		ml.mutex.Unlock()
		ml.logger.Debugf("No handler for system '%s': %s", message.System, message.Message)
	}

	// Update statistics
	ml.mutex.Lock()
	ml.statistics.MessagesBySystem[message.System]++
	ml.mutex.Unlock()
}

// parseMessage parses a BTicino syslog message
// Based on slyoldfox's message-parser.js implementation
func (ml *MulticastListener) parseMessage(data string) *BTicinoMessage {
	message := &BTicinoMessage{
		Timestamp: time.Now(),
		Raw:       data,
		Parsed:    false,
	}

	// BTicino syslog format analysis from slyoldfox
	data = strings.TrimSpace(data)

	// Look for system identifiers (based on observed BTicino patterns)
	if strings.Contains(data, "OPEN") {
		message.System = "OPEN" // OpenWebNet messages
		message.Message = ml.extractOpenWebNetMessage(data)
		message.Parsed = true
	} else if strings.Contains(data, "ASWM") {
		message.System = "ASWM" // Answering machine/voicemail
		message.Message = data
		message.Parsed = true
	} else if strings.Contains(data, "SIP") {
		message.System = "SIP" // SIP-related messages
		message.Message = data
		message.Parsed = true
	} else {
		// Unknown system - use first word as system identifier
		parts := strings.Fields(data)
		if len(parts) > 0 {
			message.System = parts[0]
			message.Message = data
		} else {
			message.System = "UNKNOWN"
			message.Message = data
		}
	}

	return message
}

// extractOpenWebNetMessage extracts OpenWebNet command from syslog message
func (ml *MulticastListener) extractOpenWebNetMessage(data string) string {
	// Look for OpenWebNet command patterns *...##
	start := strings.Index(data, "*")
	if start == -1 {
		return data // No OpenWebNet command found
	}

	end := strings.Index(data[start:], "##")
	if end == -1 {
		return data // Incomplete command
	}

	// Extract the command
	command := data[start : start+end+2]
	return command
}

// Stop stops the multicast listener
func (ml *MulticastListener) Stop() error {
	ml.mutex.Lock()
	if !ml.running {
		ml.mutex.Unlock()
		return nil
	}
	ml.running = false
	ml.mutex.Unlock()

	ml.logger.Info("Stopping MulticastListener")

	// Signal stop
	close(ml.stopCh)

	// Close connection
	if ml.conn != nil {
		ml.conn.Close()
	}

	// Wait for goroutines to finish
	ml.wg.Wait()

	ml.logger.Info("MulticastListener stopped")
	return nil
}

// IsRunning returns true if the listener is currently running
func (ml *MulticastListener) IsRunning() bool {
	ml.mutex.RLock()
	defer ml.mutex.RUnlock()
	return ml.running
}

// GetStats returns current statistics
func (ml *MulticastListener) GetStats() Stats {
	ml.mutex.RLock()
	defer ml.mutex.RUnlock()

	// Create a copy to avoid race conditions
	statsCopy := Stats{
		TotalMessages:     ml.statistics.TotalMessages,
		UnhandledMessages: ml.statistics.UnhandledMessages,
		ErrorCount:        ml.statistics.ErrorCount,
		LastMessageTime:   ml.statistics.LastMessageTime,
		StartTime:         ml.statistics.StartTime,
		MessagesBySystem:  make(map[string]int64),
	}

	for k, v := range ml.statistics.MessagesBySystem {
		statsCopy.MessagesBySystem[k] = v
	}

	return statsCopy
}

// TimeLog logs a message with timestamp (compatible with slyoldfox interface)
func (ml *MulticastListener) TimeLog(message string) {
	ml.logger.Infof("= %s => %s", time.Now().Format("2006-01-02 15:04:05"), message)
}

// Handler returns a specific message handler
func (ml *MulticastListener) Handler(name string) MessageHandler {
	ml.mutex.RLock()
	defer ml.mutex.RUnlock()
	return ml.handlers[name]
}
