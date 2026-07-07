// audio_probe.go implementa una sonda de AUDIO mínima y segura para diagnóstico.
//
// Pide UNA vez (sin reintentos) a bt_av_media que duplique su RTP de audio Speex
// hacia un puerto UDP local (*7*300 tipo 2) y MIDE el flujo que llega (paquetes,
// bytes, payload type). No decodifica ni reproduce — solo confirma que el audio
// fluye. NO hace self-INVITE.
//
// El audio del BTicino requiere una sesión de vídeo activa primero: la secuencia
// normal es pulsar el "ojo" (vídeo) y luego el "teléfono" (audio). En reposo o
// solo con teléfono, el *7*300 de audio no trae RTP.
package sip

import (
	"fmt"
	"net"
	"time"

	"bticino_bridge/pkg/openwebnet"

	"github.com/sirupsen/logrus"
)

// AudioProbePort es el puerto UDP local para la sonda de audio. Distinto del
// vídeo (10004/snapshot, 10008/sonda) para no colisionar.
const AudioProbePort = 10010

// AudioProbeResult resume el resultado de una sonda de audio.
type AudioProbeResult struct {
	Ack         bool   `json:"ack"`          // *7*300 respondió *#*1## (ACK)
	AckError    string `json:"ack_error"`    // vacío si ACK; si no, el error/NACK
	Packets     int    `json:"packets"`      // paquetes RTP recibidos
	Bytes       int    `json:"bytes"`        // bytes RTP recibidos
	PayloadType int    `json:"payload_type"` // PT del primer paquete (-1 si ninguno; Speex ≈ 110)
	Note        string `json:"note"`         // pista de diagnóstico
}

// AudioProbe envía un único *7*300 de audio y mide cuánto RTP Speex llega.
func AudioProbe(own *openwebnet.Client, timeout time.Duration, logger *logrus.Logger) (*AudioProbeResult, error) {
	if logger == nil {
		logger = logrus.New()
	}
	res := &AudioProbeResult{PayloadType: -1}
	if own == nil {
		return res, fmt.Errorf("cliente OpenWebNet no disponible")
	}
	if timeout <= 0 {
		timeout = 6 * time.Second
	}

	// Escuchar el RTP de audio entrante
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: AudioProbePort})
	if err != nil {
		return res, fmt.Errorf("no se pudo escuchar en :%d: %w", AudioProbePort, err)
	}
	defer conn.Close()
	_ = conn.SetReadBuffer(256 * 1024)

	// Margen y petición del stream de audio (envío único)
	time.Sleep(200 * time.Millisecond)
	logger.Infof("AudioProbe: *7*300 (audio) único → 127.0.0.1:%d", AudioProbePort)
	if err := own.ActivateAudioStream("127.0.0.1", AudioProbePort); err != nil {
		res.AckError = err.Error()
	} else {
		res.Ack = true
	}

	// Medir el flujo durante la ventana
	buf := make([]byte, 2048)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			break
		}
		if n < 12 { // cabecera RTP mínima
			continue
		}
		res.Packets++
		res.Bytes += n
		if res.PayloadType < 0 {
			res.PayloadType = int(buf[1] & 0x7f) // PT = 7 bits bajos del segundo byte
		}
	}

	switch {
	case res.Packets > 0:
		res.Note = fmt.Sprintf("audio OK: %d paquetes RTP recibidos (PT=%d)", res.Packets, res.PayloadType)
	case res.Ack:
		res.Note = "*7*300 audio aceptado pero NO llegó RTP (¿pulsaste ojo + teléfono?)"
	default:
		res.Note = "*7*300 audio rechazado (NACK): activa vídeo (ojo) y llamada (teléfono) primero"
	}

	logger.Infof("AudioProbe: ack=%v packets=%d bytes=%d pt=%d — %s", res.Ack, res.Packets, res.Bytes, res.PayloadType, res.Note)
	return res, nil
}
