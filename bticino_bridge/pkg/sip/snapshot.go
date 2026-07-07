// snapshot.go implementa la captura de fotogramas JPEG desde el stream H.264.
//
// Funcionamiento:
//  1. Garantiza que el pipeline de video esta activo (llamada SIP + GStreamer).
//  2. Registra un consumidor UDP temporal en el relay de video hacia un puerto
//     loopback dedicado (SnapshotMirrorPort) — un "espejo" del RTP en vivo.
//  3. Lanza un gst-launch-1.0 que depaquetiza el RTP, decodifica con la VPU
//     (imxvpudec) y codifica JPEG a un fichero temporal (h264parse se encarga
//     de sincronizar en el primer SPS/PPS+IDR; config-interval=1 garantiza
//     uno por segundo).
//  4. Sondea el fichero hasta encontrar un JPEG completo (SOI FFD8 ... EOI FFD9),
//     mata el pipeline, recorta al primer JPEG y lo cachea.
//
// El stream se deja activo tras la captura (mismo comportamiento que un
// cliente RTSP que se desconecta).
package sip

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// SnapshotMirrorPort es el puerto loopback al que el relay espeja el RTP
	// de video mientras hay una captura en curso. Las capturas se serializan,
	// asi que un puerto fijo es suficiente.
	SnapshotMirrorPort = 10004

	snapshotConsumerID = "snapshot"
	snapshotTmpFile    = "bticino_snapshot.jpg.partial"
)

var (
	jpegSOI = []byte{0xFF, 0xD8}
	jpegEOI = []byte{0xFF, 0xD9}
)

// SnapshotService captura fotogramas JPEG del stream de la camara.
type SnapshotService struct {
	server *EnhancedRTSPServer
	logger *logrus.Logger

	mu       sync.Mutex // serializa capturas
	lastJPEG []byte
	lastTime time.Time
}

// NewSnapshotService crea el servicio de snapshots sobre un servidor RTSP.
func NewSnapshotService(server *EnhancedRTSPServer, logger *logrus.Logger) *SnapshotService {
	if logger == nil {
		logger = logrus.New()
	}
	return &SnapshotService{
		server: server,
		logger: logger,
	}
}

// Capture devuelve un fotograma JPEG de la camara. Si existe una captura mas
// reciente que maxAge se reutiliza (maxAge <= 0 desactiva la cache). timeout
// cubre todo el proceso, incluida la activacion del stream si estaba parado
// (el establecimiento de la llamada SIP puede tardar varios segundos).
func (s *SnapshotService) Capture(timeout, maxAge time.Duration) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if maxAge > 0 && s.lastJPEG != nil && time.Since(s.lastTime) < maxAge {
		s.logger.Debugf("Snapshot: devolviendo captura cacheada (%s de antiguedad)", time.Since(s.lastTime).Round(time.Millisecond))
		return s.lastJPEG, nil
	}

	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	deadline := time.Now().Add(timeout)

	// 1. Asegurar que el pipeline de video esta activo (el servidor RTSP puede
	// existir sin cliente SIP; en ese caso confiamos en que el RTP llegue por
	// otra via y lo validamos en waitForVideoFlow)
	if s.server.sipClient != nil {
		if err := s.server.ensureSIPCallActive(); err != nil {
			return nil, fmt.Errorf("no se pudo activar el stream de video: %w", err)
		}
	}

	// 2. Esperar a que el RTP de video fluya realmente
	if err := s.waitForVideoFlow(deadline); err != nil {
		return nil, err
	}

	// 3. Lanzar el pipeline JPEG y espejar el RTP hacia el
	tmpPath := filepath.Join(os.TempDir(), snapshotTmpFile)
	_ = os.Remove(tmpPath)

	cmd := exec.Command("gst-launch-1.0",
		"udpsrc", fmt.Sprintf("port=%d", SnapshotMirrorPort),
		"caps=application/x-rtp,media=(string)video,clock-rate=(int)90000,encoding-name=(string)H264,payload=(int)96",
		"!", "rtph264depay",
		"!", "h264parse",
		"!", "imxvpudec",
		"!", "jpegenc", "quality=90",
		"!", "filesink", fmt.Sprintf("location=%s", tmpPath), "buffer-mode=2",
	)
	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("no se pudo lanzar el pipeline de snapshot: %w", err)
	}

	defer func() {
		s.server.rtpRelay.Video.RemoveConsumer(snapshotConsumerID)
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		_ = os.Remove(tmpPath)
	}()

	// Pequeño margen para que udpsrc tenga el puerto abierto antes de espejar
	time.Sleep(150 * time.Millisecond)

	if err := s.server.rtpRelay.Video.AddUDPConsumer(snapshotConsumerID, "127.0.0.1", SnapshotMirrorPort); err != nil {
		return nil, fmt.Errorf("no se pudo registrar el espejo RTP: %w", err)
	}

	// 4. Sondear el fichero hasta tener un JPEG completo
	jpeg, err := s.pollForJPEG(tmpPath, deadline, stderrBuf)
	if err != nil {
		return nil, err
	}

	s.lastJPEG = jpeg
	s.lastTime = time.Now()
	s.logger.Infof("Snapshot capturado: %d bytes", len(jpeg))
	return jpeg, nil
}

// LastSnapshot devuelve la ultima captura y su antiguedad (nil si no hay).
func (s *SnapshotService) LastSnapshot() ([]byte, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastJPEG, s.lastTime
}

// waitForVideoFlow espera hasta que el relay de video reciba paquetes nuevos.
// "Llegan paquetes RTP" es la confirmacion real de que el pipeline funciona,
// independientemente de lo que dijera el protocolo al activarlo.
func (s *SnapshotService) waitForVideoFlow(deadline time.Time) error {
	relay := s.server.rtpRelay.Video
	start := atomic.LoadUint64(&relay.packetsReceived)

	for time.Now().Before(deadline) {
		if atomic.LoadUint64(&relay.packetsReceived) > start {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout esperando flujo RTP de video (0 paquetes nuevos)")
}

// pollForJPEG sondea tmpPath hasta que contenga un JPEG completo (SOI...EOI)
// y devuelve el primer JPEG recortado.
func (s *SnapshotService) pollForJPEG(tmpPath string, deadline time.Time, stderrBuf *bytes.Buffer) ([]byte, error) {
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)

		data, err := os.ReadFile(tmpPath)
		if err != nil || len(data) < 4 {
			continue
		}

		soi := bytes.Index(data, jpegSOI)
		if soi < 0 {
			continue
		}
		eoi := bytes.Index(data[soi+2:], jpegEOI)
		if eoi < 0 {
			continue
		}

		end := soi + 2 + eoi + 2
		jpeg := make([]byte, end-soi)
		copy(jpeg, data[soi:end])
		return jpeg, nil
	}

	stderr := stderrBuf.String()
	if stderr != "" {
		s.logger.Warnf("Snapshot: pipeline stderr: %s", stderr)
	}
	return nil, fmt.Errorf("timeout esperando un JPEG completo del pipeline")
}
