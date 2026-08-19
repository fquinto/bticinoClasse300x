package sip

import (
	"encoding/binary"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// buildLoopbackFrame construye una trama Ethernet+IPv4+UDP como las que entrega
// AF_PACKET en `lo` (MACs a cero, ethertype 0x0800).
func buildLoopbackFrame(dstPort int, payload []byte, ipOptWords int) []byte {
	eth := make([]byte, 14)
	eth[12], eth[13] = 0x08, 0x00

	ihl := 5 + ipOptWords
	ip := make([]byte, ihl*4)
	ip[0] = byte(0x40 | ihl) // versión 4 + IHL
	binary.BigEndian.PutUint16(ip[2:], uint16(len(ip)+8+len(payload)))
	ip[8] = 64 // TTL
	ip[9] = 17 // UDP
	copy(ip[12:16], []byte{127, 0, 0, 1})
	copy(ip[16:20], []byte{127, 0, 0, 1})

	udp := make([]byte, 8)
	binary.BigEndian.PutUint16(udp[0:], 49145)
	binary.BigEndian.PutUint16(udp[2:], uint16(dstPort))
	binary.BigEndian.PutUint16(udp[4:], uint16(8+len(payload)))

	frame := append(eth, ip...)
	frame = append(frame, udp...)
	return append(frame, payload...)
}

func TestExtractUDPPayload(t *testing.T) {
	want := []byte{0x80, 0x60, 0x19, 0xbf, 0x67, 0x42}
	port, payload, ok := extractUDPPayload(buildLoopbackFrame(5007, want, 0))
	if !ok {
		t.Fatal("esperaba ok=true para trama UDP IPv4 válida")
	}
	if port != 5007 {
		t.Fatalf("esperaba dstPort=5007, got %d", port)
	}
	if string(payload) != string(want) {
		t.Fatalf("payload incorrecto: % x", payload)
	}
}

func TestExtractUDPPayloadWithIPOptions(t *testing.T) {
	// IHL>5: el offset del UDP debe respetar las opciones IP.
	want := []byte{0xde, 0xad}
	port, payload, ok := extractUDPPayload(buildLoopbackFrame(5007, want, 2))
	if !ok || port != 5007 || string(payload) != string(want) {
		t.Fatalf("fallo con opciones IP: ok=%v port=%d payload=% x", ok, port, payload)
	}
}

func TestExtractUDPPayloadRejectsNonIPv4(t *testing.T) {
	frame := buildLoopbackFrame(5007, []byte{1}, 0)
	frame[12], frame[13] = 0x86, 0xdd // ethertype IPv6
	if _, _, ok := extractUDPPayload(frame); ok {
		t.Fatal("esperaba ok=false para ethertype no IPv4")
	}
}

func TestExtractUDPPayloadRejectsNonUDP(t *testing.T) {
	frame := buildLoopbackFrame(5007, []byte{1}, 0)
	frame[14+9] = 6 // TCP
	if _, _, ok := extractUDPPayload(frame); ok {
		t.Fatal("esperaba ok=false para protocolo no UDP")
	}
}

func TestExtractUDPPayloadTruncatedFrames(t *testing.T) {
	full := buildLoopbackFrame(5007, []byte{1, 2, 3}, 0)
	for n := 0; n < len(full); n++ {
		if _, _, ok := extractUDPPayload(full[:n]); ok {
			t.Fatalf("esperaba ok=false para trama truncada a %d bytes", n)
		}
	}
}

// fakeFrameSource entrega tramas preparadas y luego io.EOF.
type fakeFrameSource struct {
	frames []struct {
		data    []byte
		pkttype byte
	}
	closed chan struct{}
}

func (f *fakeFrameSource) ReadFrame(buf []byte) (int, byte, error) {
	if len(f.frames) == 0 {
		<-f.closed // bloquea hasta Close, como un socket real sin tráfico
		return 0, 0, io.EOF
	}
	fr := f.frames[0]
	f.frames = f.frames[1:]
	n := copy(buf, fr.data)
	return n, fr.pkttype, nil
}

func (f *fakeFrameSource) Close() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	return nil
}

func TestTapForwardsOnlyMatchingHostPackets(t *testing.T) {
	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	sinkPort := sink.LocalAddr().(*net.UDPAddr).Port

	src := &fakeFrameSource{closed: make(chan struct{})}
	add := func(dstPort int, payload []byte, pkttype byte) {
		src.frames = append(src.frames, struct {
			data    []byte
			pkttype byte
		}{buildLoopbackFrame(dstPort, payload, 0), pkttype})
	}
	add(5007, []byte("video1"), syscall.PACKET_HOST)
	add(5007, []byte("video1"), syscall.PACKET_OUTGOING) // duplicado tx de lo: descartar
	add(5002, []byte("otro"), syscall.PACKET_HOST)       // otro puerto: descartar
	add(5007, []byte("video2"), syscall.PACKET_HOST)

	tap := NewLoopbackRTPTap(5007, sinkPort, logrus.New())
	tap.newSource = func() (frameSource, error) { return src, nil }
	if err := tap.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer tap.Stop()

	var got []string
	buf := make([]byte, 2048)
	for len(got) < 2 {
		sink.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := sink.ReadFromUDP(buf)
		if err != nil {
			t.Fatalf("el sumidero no recibió los 2 paquetes esperados (got %v): %v", got, err)
		}
		got = append(got, string(buf[:n]))
	}
	if got[0] != "video1" || got[1] != "video2" {
		t.Fatalf("payloads reenviados incorrectos: %v", got)
	}
	// ningún paquete extra (el duplicado OUTGOING y el de 5002 se descartan)
	sink.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if n, _, err := sink.ReadFromUDP(buf); err == nil {
		t.Fatalf("paquete inesperado reenviado: %q", buf[:n])
	}
}

func TestTapStopClosesSourceAndIsIdempotent(t *testing.T) {
	src := &fakeFrameSource{closed: make(chan struct{})}
	tap := NewLoopbackRTPTap(5007, 19999, logrus.New())
	tap.newSource = func() (frameSource, error) { return src, nil }
	if err := tap.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	tap.Stop()
	select {
	case <-src.closed:
	case <-time.After(time.Second):
		t.Fatal("Stop no cerró la fuente")
	}
	tap.Stop() // idempotente: no debe entrar en pánico
}

func TestTapStartIdempotent(t *testing.T) {
	opened := 0
	tap := NewLoopbackRTPTap(5007, 19999, logrus.New())
	tap.newSource = func() (frameSource, error) {
		opened++
		return &fakeFrameSource{closed: make(chan struct{})}, nil
	}
	if err := tap.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer tap.Stop()
	if err := tap.Start(); err != nil {
		t.Fatalf("segundo Start: %v", err)
	}
	if opened != 1 {
		t.Fatalf("esperaba 1 fuente abierta (Start idempotente), got %d", opened)
	}
}
