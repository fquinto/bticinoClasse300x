package sip

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"
)

// NativeVideoRTPPort es el puerto loopback al que bt_av_media envía el vídeo
// H.264 (RTP PT=96, EN CLARO) durante una llamada: es el leg SIP local que el
// propio firmware arma con sus *7*300 (tipo 0 → 5007). Verificado en la Fase 0
// del 2026-08-18: flujo sostenido (~205 pps) desde el ring hasta colgar.
const NativeVideoRTPPort = 5007

// frameSource abstrae la fuente de tramas crudas (AF_PACKET real en el device,
// una fuente sintética en los tests).
type frameSource interface {
	// ReadFrame llena buf y devuelve el tamaño y el pkttype (PACKET_HOST,
	// PACKET_OUTGOING...) de la trama. Error de timeout → reintentable.
	ReadFrame(buf []byte) (n int, pkttype byte, err error)
	Close() error
}

// LoopbackRTPTap es un espejo PASIVO: captura en `lo` los datagramas UDP
// destinados a srcPort (el leg de vídeo nativo) y reenvía su payload RTP a
// 127.0.0.1:dstPort (la entrada del relay). No envía NINGÚN comando al bus ni
// abre sockets que compitan con los procesos nativos (patrón no-interferencia):
// el socket AF_PACKET es de solo escucha, como tcpdump/minicap.
type LoopbackRTPTap struct {
	srcPort int
	dstPort int
	logger  *logrus.Logger

	// newSource es inyectable en tests; por defecto abre AF_PACKET en `lo`.
	newSource func() (frameSource, error)

	mu      sync.Mutex
	source  frameSource
	conn    *net.UDPConn
	running bool
}

// NewLoopbackRTPTap crea el tap (sin abrir nada todavía).
func NewLoopbackRTPTap(srcPort, dstPort int, logger *logrus.Logger) *LoopbackRTPTap {
	if logger == nil {
		logger = logrus.New()
	}
	t := &LoopbackRTPTap{srcPort: srcPort, dstPort: dstPort, logger: logger}
	t.newSource = func() (frameSource, error) { return newAFPacketSource("lo") }
	return t
}

// Start abre la captura y arranca el bucle de reenvío. Idempotente.
func (t *LoopbackRTPTap) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return nil
	}
	src, err := t.newSource()
	if err != nil {
		return fmt.Errorf("tap: abriendo captura en lo: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: t.dstPort})
	if err != nil {
		src.Close()
		return fmt.Errorf("tap: abriendo destino 127.0.0.1:%d: %w", t.dstPort, err)
	}
	t.source = src
	t.conn = conn
	t.running = true
	go t.forwardLoop(src, conn)
	t.logger.Infof("rtp-tap: espejo pasivo lo:%d → 127.0.0.1:%d", t.srcPort, t.dstPort)
	return nil
}

// forwardLoop lee tramas y reenvía el payload UDP de las que van a srcPort.
// En `lo` cada paquete se entrega dos veces (PACKET_OUTGOING al enviarse y
// PACKET_HOST al recibirse); se procesa solo la copia PACKET_HOST.
func (t *LoopbackRTPTap) forwardLoop(src frameSource, conn *net.UDPConn) {
	buf := make([]byte, 65536)
	var forwarded uint64
	for {
		n, pkttype, err := src.ReadFrame(buf)
		if err != nil {
			if isRetryableReadError(err) {
				continue
			}
			t.mu.Lock()
			running := t.running
			t.mu.Unlock()
			if running {
				t.logger.WithError(err).Debug("rtp-tap: captura terminada")
			}
			t.logger.Infof("rtp-tap: parado (%d paquetes espejados)", forwarded)
			return
		}
		if pkttype == syscall.PACKET_OUTGOING {
			continue
		}
		dstPort, payload, ok := extractUDPPayload(buf[:n])
		if !ok || dstPort != t.srcPort || len(payload) == 0 {
			continue
		}
		if _, err := conn.Write(payload); err == nil {
			forwarded++
		}
	}
}

// Stop cierra la captura y el destino. Idempotente.
func (t *LoopbackRTPTap) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return
	}
	t.running = false
	if t.source != nil {
		t.source.Close()
		t.source = nil
	}
	if t.conn != nil {
		t.conn.Close()
		t.conn = nil
	}
}

// extractUDPPayload interpreta una trama Ethernet+IPv4+UDP (formato de `lo`)
// y devuelve el puerto destino y el payload UDP. ok=false si la trama no es
// IPv4/UDP o está truncada. No verifica checksums (loopback no los necesita).
func extractUDPPayload(frame []byte) (dstPort int, payload []byte, ok bool) {
	const ethLen = 14
	if len(frame) < ethLen+20+8 {
		return 0, nil, false
	}
	if frame[12] != 0x08 || frame[13] != 0x00 { // ethertype IPv4
		return 0, nil, false
	}
	ip := frame[ethLen:]
	if ip[0]>>4 != 4 {
		return 0, nil, false
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || len(ip) < ihl+8 {
		return 0, nil, false
	}
	if ip[9] != 17 { // UDP
		return 0, nil, false
	}
	if binary.BigEndian.Uint16(ip[6:8])&0x1fff != 0 { // fragmento no inicial
		return 0, nil, false
	}
	udp := ip[ihl:]
	dstPort = int(binary.BigEndian.Uint16(udp[2:4]))
	udpLen := int(binary.BigEndian.Uint16(udp[4:6]))
	if udpLen < 8 || len(udp) < udpLen {
		return 0, nil, false
	}
	return dstPort, udp[8:udpLen], true
}

// afpacketSource es la fuente real: AF_PACKET (solo lectura) ligado a una
// interfaz, con timeout de lectura para poder parar limpiamente.
type afpacketSource struct {
	fd int
}

func newAFPacketSource(iface string) (*afpacketSource, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}
	proto := htons(syscall.ETH_P_ALL)
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(proto))
	if err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrLinklayer{Protocol: proto, Ifindex: ifi.Index}); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	// Timeout de lectura: permite que el bucle observe el cierre sin quedarse
	// bloqueado en Recvfrom para siempre.
	tv := syscall.Timeval{Sec: 0, Usec: 500000}
	if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return &afpacketSource{fd: fd}, nil
}

func (s *afpacketSource) ReadFrame(buf []byte) (int, byte, error) {
	n, from, err := syscall.Recvfrom(s.fd, buf, 0)
	if err != nil {
		return 0, 0, err
	}
	var pkttype byte
	if sll, ok := from.(*syscall.SockaddrLinklayer); ok {
		pkttype = sll.Pkttype
	}
	return n, pkttype, nil
}

func (s *afpacketSource) Close() error {
	return syscall.Close(s.fd)
}

// isRetryableReadError distingue timeouts/interrupciones (reintentar) de un
// cierre real de la fuente (terminar el bucle).
func isRetryableReadError(err error) bool {
	return err == syscall.EAGAIN || err == syscall.EWOULDBLOCK || err == syscall.EINTR
}

func htons(v uint16) uint16 {
	return v<<8 | v>>8
}
