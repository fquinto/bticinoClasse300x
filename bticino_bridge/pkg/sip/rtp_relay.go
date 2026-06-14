// Package sip provides an RTP relay that receives RTP packets from GStreamer's
// udpsink (on localhost UDP ports) and forwards them to RTSP clients.
//
// Architecture:
//
//	GStreamer -> udpsink 127.0.0.1:10002 -> [RTPRelay] -> RTSP clients (UDP or TCP interleaved)
//	GStreamer -> udpsink 127.0.0.1:10000 -> [RTPRelay] -> RTSP clients (UDP or TCP interleaved)
//
// Each relay instance handles one media type (video or audio). It listens on a
// UDP port, receives RTP packets, and fans them out to all registered consumers.
// Consumers can be UDP destinations (host:port) or TCP connections (RTSP interleaved).
package sip

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// RTPConsumer represents a destination for RTP packets.
// NOTE: uint64 fields MUST be at the top of struct for 64-bit alignment on 32-bit ARM.
type RTPConsumer struct {
	// Stats — must be first for 64-bit atomic alignment on ARM
	PacketsSent uint64
	BytesSent   uint64
	Errors      uint64

	ID   string
	Type string // "udp" or "tcp"

	// UDP consumer fields
	UDPAddr *net.UDPAddr
	UDPConn *net.UDPConn // shared sender socket (not per-consumer)

	// TCP interleaved consumer fields
	TCPConn    net.Conn
	RTPChannel uint8 // RTSP interleaved channel number for RTP

	LastSent time.Time
}

// RTPRelay receives RTP packets on a UDP port and fans out to consumers.
// NOTE: uint64 fields MUST be at the top of struct for 64-bit alignment on 32-bit ARM.
type RTPRelay struct {
	// Stats — must be first for 64-bit atomic alignment on ARM
	packetsReceived uint64
	bytesReceived   uint64

	name      string // "video" or "audio"
	port      int
	logger    *logrus.Logger
	conn      *net.UDPConn // listening socket
	consumers map[string]*RTPConsumer
	mu        sync.RWMutex
	running   int32 // atomic
	stopCh    chan struct{}
	udpSender *net.UDPConn // reusable socket for sending UDP to consumers
	startTime time.Time
}

// NewRTPRelay creates a new relay for the given media type and port.
func NewRTPRelay(name string, port int, logger *logrus.Logger) *RTPRelay {
	if logger == nil {
		logger = logrus.New()
	}
	return &RTPRelay{
		name:      name,
		port:      port,
		logger:    logger,
		consumers: make(map[string]*RTPConsumer),
		stopCh:    make(chan struct{}),
	}
}

// Start begins listening for RTP packets on the configured UDP port.
func (r *RTPRelay) Start() error {
	if !atomic.CompareAndSwapInt32(&r.running, 0, 1) {
		return fmt.Errorf("RTP relay %s already running", r.name)
	}

	addr := &net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: r.port}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		atomic.StoreInt32(&r.running, 0)
		return fmt.Errorf("failed to listen on UDP :%d for %s relay: %w", r.port, r.name, err)
	}

	// Set a generous read buffer for bursty video
	conn.SetReadBuffer(512 * 1024)
	r.conn = conn

	// Create a reusable UDP socket for sending to UDP consumers
	r.udpSender, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		conn.Close()
		atomic.StoreInt32(&r.running, 0)
		return fmt.Errorf("failed to create UDP sender for %s relay: %w", r.name, err)
	}

	r.startTime = time.Now()
	r.stopCh = make(chan struct{})

	go r.readLoop()

	r.logger.Infof("RTP relay '%s' started on UDP :%d", r.name, r.port)
	return nil
}

// Stop terminates the relay.
func (r *RTPRelay) Stop() {
	if !atomic.CompareAndSwapInt32(&r.running, 1, 0) {
		return
	}

	close(r.stopCh)

	if r.conn != nil {
		r.conn.Close()
	}
	if r.udpSender != nil {
		r.udpSender.Close()
	}

	r.mu.Lock()
	// Clear consumers
	r.consumers = make(map[string]*RTPConsumer)
	r.mu.Unlock()

	r.logger.Infof("RTP relay '%s' stopped (received %d packets, %d bytes)",
		r.name, atomic.LoadUint64(&r.packetsReceived), atomic.LoadUint64(&r.bytesReceived))
}

// IsRunning returns whether the relay is active.
func (r *RTPRelay) IsRunning() bool {
	return atomic.LoadInt32(&r.running) == 1
}

// AddUDPConsumer registers a UDP destination for RTP forwarding.
func (r *RTPRelay) AddUDPConsumer(id string, host string, port int) error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return fmt.Errorf("invalid UDP address %s:%d: %w", host, port, err)
	}

	consumer := &RTPConsumer{
		ID:      id,
		Type:    "udp",
		UDPAddr: addr,
		UDPConn: r.udpSender,
	}

	r.mu.Lock()
	r.consumers[id] = consumer
	r.mu.Unlock()

	r.logger.Infof("RTP relay '%s': added UDP consumer '%s' -> %s:%d", r.name, id, host, port)
	return nil
}

// AddTCPConsumer registers a TCP interleaved destination for RTP forwarding.
func (r *RTPRelay) AddTCPConsumer(id string, conn net.Conn, rtpChannel uint8) {
	consumer := &RTPConsumer{
		ID:         id,
		Type:       "tcp",
		TCPConn:    conn,
		RTPChannel: rtpChannel,
	}

	r.mu.Lock()
	r.consumers[id] = consumer
	r.mu.Unlock()

	r.logger.Infof("RTP relay '%s': added TCP consumer '%s' channel=%d", r.name, id, rtpChannel)
}

// RemoveConsumer removes a consumer by ID.
func (r *RTPRelay) RemoveConsumer(id string) {
	r.mu.Lock()
	_, existed := r.consumers[id]
	delete(r.consumers, id)
	r.mu.Unlock()

	if existed {
		r.logger.Infof("RTP relay '%s': removed consumer '%s'", r.name, id)
	}
}

// ConsumerCount returns the number of active consumers.
func (r *RTPRelay) ConsumerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.consumers)
}

// readLoop continuously reads RTP packets from GStreamer and forwards to consumers.
func (r *RTPRelay) readLoop() {
	buf := make([]byte, 2048) // max RTP packet (usually <=1500 MTU)

	for atomic.LoadInt32(&r.running) == 1 {
		// Set deadline so we can check running flag periodically
		r.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, _, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // timeout, check running flag
			}
			select {
			case <-r.stopCh:
				return
			default:
				r.logger.WithError(err).Debugf("RTP relay '%s' read error", r.name)
				continue
			}
		}

		if n < 12 {
			continue // too small for RTP
		}

		atomic.AddUint64(&r.packetsReceived, 1)
		atomic.AddUint64(&r.bytesReceived, uint64(n))

		// Make a copy for fan-out (buf will be reused)
		packet := make([]byte, n)
		copy(packet, buf[:n])

		r.fanOut(packet)
	}
}

// fanOut sends the packet to all registered consumers.
func (r *RTPRelay) fanOut(packet []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, c := range r.consumers {
		switch c.Type {
		case "udp":
			r.sendUDP(c, packet)
		case "tcp":
			r.sendTCPInterleaved(c, packet)
		}
	}
}

// sendUDP sends an RTP packet to a UDP consumer.
func (r *RTPRelay) sendUDP(c *RTPConsumer, packet []byte) {
	if c.UDPConn == nil || c.UDPAddr == nil {
		return
	}
	_, err := c.UDPConn.WriteToUDP(packet, c.UDPAddr)
	if err != nil {
		c.Errors++
		if c.Errors%100 == 1 {
			r.logger.Warnf("RTP relay '%s': UDP send error to '%s': %v (errors: %d)",
				r.name, c.ID, err, c.Errors)
		}
		return
	}
	c.PacketsSent++
	c.BytesSent += uint64(len(packet))
	c.LastSent = time.Now()
}

// sendTCPInterleaved sends an RTP packet wrapped in RTSP interleaved framing.
// Format: $<channel><length-BE16><RTP data>
func (r *RTPRelay) sendTCPInterleaved(c *RTPConsumer, packet []byte) {
	if c.TCPConn == nil {
		return
	}

	// RTSP interleaved header: $ + channel(1) + length(2)
	header := make([]byte, 4)
	header[0] = '$'
	header[1] = c.RTPChannel
	binary.BigEndian.PutUint16(header[2:4], uint16(len(packet)))

	// Write header + packet atomically (as much as possible)
	frame := append(header, packet...)

	c.TCPConn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	_, err := c.TCPConn.Write(frame)
	if err != nil {
		c.Errors++
		if c.Errors%100 == 1 {
			r.logger.Warnf("RTP relay '%s': TCP send error to '%s': %v (errors: %d)",
				r.name, c.ID, err, c.Errors)
		}
		return
	}
	c.PacketsSent++
	c.BytesSent += uint64(len(frame))
	c.LastSent = time.Now()
}

// GetStats returns relay statistics.
func (r *RTPRelay) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"name":             r.name,
		"port":             r.port,
		"running":          r.IsRunning(),
		"packets_received": atomic.LoadUint64(&r.packetsReceived),
		"bytes_received":   atomic.LoadUint64(&r.bytesReceived),
	}

	if r.IsRunning() {
		stats["uptime_seconds"] = time.Since(r.startTime).Seconds()
	}

	r.mu.RLock()
	consumers := make([]map[string]interface{}, 0, len(r.consumers))
	for _, c := range r.consumers {
		consumers = append(consumers, map[string]interface{}{
			"id":           c.ID,
			"type":         c.Type,
			"packets_sent": c.PacketsSent,
			"bytes_sent":   c.BytesSent,
			"errors":       c.Errors,
		})
	}
	r.mu.RUnlock()
	stats["consumers"] = consumers
	stats["consumer_count"] = len(consumers)

	return stats
}

// RTPRelayPair manages video and audio relays together.
type RTPRelayPair struct {
	Video  *RTPRelay
	Audio  *RTPRelay
	logger *logrus.Logger
}

// NewRTPRelayPair creates video and audio relays.
func NewRTPRelayPair(videoPort, audioPort int, logger *logrus.Logger) *RTPRelayPair {
	return &RTPRelayPair{
		Video:  NewRTPRelay("video", videoPort, logger),
		Audio:  NewRTPRelay("audio", audioPort, logger),
		logger: logger,
	}
}

// Start starts both relays.
func (p *RTPRelayPair) Start() error {
	if err := p.Video.Start(); err != nil {
		return fmt.Errorf("video relay: %w", err)
	}
	if err := p.Audio.Start(); err != nil {
		p.Video.Stop()
		return fmt.Errorf("audio relay: %w", err)
	}
	return nil
}

// Stop stops both relays.
func (p *RTPRelayPair) Stop() {
	p.Video.Stop()
	p.Audio.Stop()
}

// AddUDPConsumer registers a UDP destination for both video and audio.
func (p *RTPRelayPair) AddUDPConsumer(id string, host string, videoPort, audioPort int) error {
	if err := p.Video.AddUDPConsumer(id, host, videoPort); err != nil {
		return err
	}
	if err := p.Audio.AddUDPConsumer(id, host, audioPort); err != nil {
		p.Video.RemoveConsumer(id)
		return err
	}
	return nil
}

// AddTCPConsumer registers a TCP interleaved destination for both video and audio.
func (p *RTPRelayPair) AddTCPConsumer(id string, conn net.Conn, videoChannel, audioChannel uint8) {
	p.Video.AddTCPConsumer(id, conn, videoChannel)
	p.Audio.AddTCPConsumer(id, conn, audioChannel)
}

// RemoveConsumer removes a consumer from both relays.
func (p *RTPRelayPair) RemoveConsumer(id string) {
	p.Video.RemoveConsumer(id)
	p.Audio.RemoveConsumer(id)
}

// HasConsumers returns true if any relay has consumers.
func (p *RTPRelayPair) HasConsumers() bool {
	return p.Video.ConsumerCount() > 0 || p.Audio.ConsumerCount() > 0
}

// GetStats returns stats for both relays.
func (p *RTPRelayPair) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"video": p.Video.GetStats(),
		"audio": p.Audio.GetStats(),
	}
}
