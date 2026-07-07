// video_probe.go implementa una sonda de vídeo MÍNIMA y SEGURA para diagnóstico.
//
// Reutiliza captureCooperative: pide UNA vez (sin reintentos) a bt_av_media que
// duplique su RTP de vídeo y decodifica lo que llegue. NO hace self-INVITE ni
// toca la señalización SIP. Pensada para usarse mientras el vídeo nativo ya está
// activo (p. ej. tras pulsar el botón del "ojo"/autoencendido).
package sip

import (
	"time"

	"bticino_bridge/pkg/openwebnet"

	"github.com/sirupsen/logrus"
)

// VideoProbePort es el puerto UDP local para la sonda. Distinto del snapshot
// (10004) y del relay (10002) para no colisionar.
const VideoProbePort = 10008

// probeWarmupFrames: la sonda es un diagnóstico rápido, con menos calentamiento.
const probeWarmupFrames = 8

// VideoProbeResult resume el resultado de una sonda.
type VideoProbeResult struct {
	Ack       bool   `json:"ack"`        // *7*300 respondió *#*1## (ACK)
	AckError  string `json:"ack_error"`  // vacío si ACK; si no, el error/NACK
	Frames    int    `json:"frames"`     // fotogramas JPEG decodificados
	JPEGBytes int    `json:"jpeg_bytes"` // tamaño del mejor fotograma
	Note      string `json:"note"`       // pista de diagnóstico
}

// VideoProbe envía un único *7*300 y comprueba si llega RTP de vídeo que se
// pueda decodificar. Devuelve el resultado y, si hubo, el mejor fotograma JPEG.
func VideoProbe(own *openwebnet.Client, timeout time.Duration, logger *logrus.Logger) (*VideoProbeResult, []byte, error) {
	if logger == nil {
		logger = logrus.New()
	}
	res := &VideoProbeResult{}

	cap, err := captureCooperative(own, VideoProbePort, probeWarmupFrames, timeout, logger)
	if err != nil {
		return res, nil, err
	}

	res.Ack = cap.ack
	res.AckError = cap.ackError
	res.Frames = cap.frames
	res.JPEGBytes = len(cap.jpeg)

	switch {
	case res.Frames > 0:
		res.Note = "vídeo OK: RTP fluyó y se decodificaron fotogramas"
	case res.Ack && res.Frames == 0:
		res.Note = "*7*300 aceptado pero NO llegó RTP decodificable (¿sesión nativa activa?)"
	default:
		res.Note = "*7*300 rechazado (NACK): bt_av_media no tiene stream que duplicar; pulsa el 'ojo' primero"
	}

	logger.Infof("VideoProbe resultado: ack=%v frames=%d bytes=%d — %s", res.Ack, res.Frames, res.JPEGBytes, res.Note)
	return res, cap.jpeg, nil
}
