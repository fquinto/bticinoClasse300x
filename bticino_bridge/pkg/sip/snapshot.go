// snapshot.go implementa la captura de fotogramas JPEG bajo demanda usando el
// patrón cooperativo (*7*300 → bt_av_media duplica su RTP), reutilizando
// captureCooperative. NO hace self-INVITE ni compite por la cámara.
//
// Requiere que haya una sesión de vídeo nativa activa (botón del "ojo" o una
// llamada real); en reposo devuelve error (NACK del *7*300).
package sip

import (
	"fmt"
	"sync"
	"time"

	"bticino_bridge/pkg/openwebnet"

	"github.com/sirupsen/logrus"
)

const (
	// SnapshotMirrorPort es el puerto UDP local al que pedimos que bt_av_media
	// envíe el RTP de vídeo durante un snapshot. Distinto del de la sonda (10008).
	SnapshotMirrorPort = 10004

	// snapshotWarmupFrames: fotogramas a acumular antes de quedarnos con el
	// último. El decoder VPU produce ruido hasta un IDR limpio; ~12 frames a
	// ~7fps garantizan haber pasado un keyframe.
	snapshotWarmupFrames = 12
)

// SnapshotService captura fotogramas JPEG de la cámara de forma cooperativa.
type SnapshotService struct {
	own    *openwebnet.Client
	logger *logrus.Logger

	mu       sync.Mutex // serializa capturas
	lastJPEG []byte
	lastTime time.Time
}

// NewSnapshotService crea el servicio de snapshots sobre el cliente OpenWebNet.
func NewSnapshotService(own *openwebnet.Client, logger *logrus.Logger) *SnapshotService {
	if logger == nil {
		logger = logrus.New()
	}
	return &SnapshotService{
		own:    own,
		logger: logger,
	}
}

// Capture devuelve un fotograma JPEG de la cámara. Si existe una captura más
// reciente que maxAge se reutiliza (maxAge <= 0 desactiva la caché). timeout
// cubre el sondeo tras enviar el *7*300.
func (s *SnapshotService) Capture(timeout, maxAge time.Duration) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if maxAge > 0 && s.lastJPEG != nil && time.Since(s.lastTime) < maxAge {
		s.logger.Debugf("Snapshot: devolviendo captura cacheada (%s de antiguedad)", time.Since(s.lastTime).Round(time.Millisecond))
		return s.lastJPEG, nil
	}

	if timeout <= 0 {
		timeout = 12 * time.Second
	}

	res, err := captureCooperative(s.own, SnapshotMirrorPort, snapshotWarmupFrames, timeout, s.logger)
	if err != nil {
		return nil, err
	}
	if len(res.jpeg) == 0 {
		if !res.ack {
			return nil, fmt.Errorf("*7*300 rechazado (NACK): no hay sesión de vídeo nativa activa (%s)", res.ackError)
		}
		return nil, fmt.Errorf("no llegó vídeo decodificable (ack=%v, fotogramas=%d)", res.ack, res.frames)
	}

	s.lastJPEG = res.jpeg
	s.lastTime = time.Now()
	s.logger.Infof("Snapshot capturado: %d bytes (%d fotogramas)", len(res.jpeg), res.frames)
	return res.jpeg, nil
}

// LastSnapshot devuelve la última captura y su antigüedad (nil si no hay).
func (s *SnapshotService) LastSnapshot() ([]byte, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastJPEG, s.lastTime
}
