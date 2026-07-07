// avmedia_capture.go contiene la captura de vídeo COOPERATIVA compartida por el
// snapshot y la sonda de diagnóstico.
//
// Patrón (validado en dispositivo real el 2026-07-07):
//   - Se lanza un pipeline gst-launch que decodifica H.264→JPEG leyendo RTP de
//     un puerto UDP local (udpsrc ! rtph264depay ! h264parse ! imxvpudec ! jpegenc).
//   - Se pide UNA sola vez (sin reintentos) a bt_av_media que duplique su RTP de
//     vídeo hacia ese puerto (*7*300). NO se hace self-INVITE ni se toca SIP.
//   - Solo produce imagen si hay una sesión de vídeo nativa activa (botón del
//     "ojo"/autoencendido o una llamada real); en reposo bt_av_media responde
//     NACK y no llega RTP.
//
// Este es el patrón seguro: blast radius de un único comando, cooperativo con el
// firmware nativo (no compite por /dev/video0).
package sip

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"bticino_bridge/pkg/openwebnet"

	"github.com/sirupsen/logrus"
)

var (
	jpegSOI = []byte{0xFF, 0xD8}
	jpegEOI = []byte{0xFF, 0xD9}
)

// captureResult resume una captura cooperativa.
type captureResult struct {
	ack      bool   // *7*300 respondió *#*1## (ACK)
	ackError string // vacío si ACK; si no, el error/NACK
	frames   int    // fotogramas JPEG decodificados
	jpeg     []byte // mejor fotograma (el último completo), nil si ninguno
}

// captureCooperative decodifica un fotograma pidiendo a bt_av_media que duplique
// su RTP de vídeo al puerto UDP indicado (un único *7*300, sin reintentos ni
// self-INVITE). warmupFrames es cuántos fotogramas acumular antes de quedarse
// con el último (el decoder produce ruido hasta un IDR limpio). timeout cubre el
// sondeo tras enviar el comando.
func captureCooperative(own *openwebnet.Client, port, warmupFrames int, timeout time.Duration, logger *logrus.Logger) (captureResult, error) {
	res := captureResult{}
	if own == nil {
		return res, fmt.Errorf("cliente OpenWebNet no disponible")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if warmupFrames <= 0 {
		warmupFrames = 12
	}

	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("bticino_cap_%d.jpg.partial", port))
	_ = os.Remove(tmp)

	// 1) Lanzar el pipeline de decodificación PRIMERO (udpsrc debe tener el
	//    puerto abierto antes de que bt_av_media empiece a enviar).
	cmd := exec.Command("gst-launch-1.0",
		"udpsrc", fmt.Sprintf("port=%d", port),
		"caps=application/x-rtp,media=(string)video,clock-rate=(int)90000,encoding-name=(string)H264,payload=(int)96",
		"!", "rtph264depay",
		"!", "h264parse",
		"!", "imxvpudec",
		"!", "jpegenc", "quality=90",
		"!", "filesink", fmt.Sprintf("location=%s", tmp), "buffer-mode=2",
	)
	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf
	if err := cmd.Start(); err != nil {
		return res, fmt.Errorf("no se pudo lanzar el pipeline de captura: %w", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		_ = os.Remove(tmp)
	}()

	// Margen para que udpsrc bindee el puerto
	time.Sleep(300 * time.Millisecond)

	// 2) Enviar UN ÚNICO *7*300 (ActivateVideoStream ya no reintenta)
	logger.Infof("captura cooperativa: *7*300 único → 127.0.0.1:%d", port)
	if err := own.ActivateVideoStream("127.0.0.1", port, true); err != nil {
		res.ackError = err.Error()
	} else {
		res.ack = true
	}

	// 3) Sondear el fichero buscando fotogramas JPEG completos
	deadline := time.Now().Add(timeout)
	var lastComplete []byte
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		data, err := os.ReadFile(tmp)
		if err != nil || len(data) < 4 {
			continue
		}
		n, last := scanJPEGFrames(data)
		res.frames = n
		if last != nil {
			lastComplete = last
		}
		if n >= warmupFrames && lastComplete != nil {
			break // el decoder ya pasó un IDR limpio
		}
	}
	res.jpeg = lastComplete

	if res.frames == 0 && stderrBuf.Len() > 0 {
		logger.Warnf("captura cooperativa: pipeline stderr: %s", stderrBuf.String())
	}
	return res, nil
}

// scanJPEGFrames recorre un buffer de JPEGs concatenados (SOI FFD8 ... EOI FFD9)
// y devuelve cuantos fotogramas completos contiene y una copia del ultimo.
func scanJPEGFrames(data []byte) (count int, last []byte) {
	pos := 0
	for {
		soi := bytes.Index(data[pos:], jpegSOI)
		if soi < 0 {
			break
		}
		soi += pos
		eoi := bytes.Index(data[soi+2:], jpegEOI)
		if eoi < 0 {
			break // JPEG incompleto (fotograma en curso de escritura)
		}
		end := soi + 2 + eoi + 2
		count++
		frame := make([]byte, end-soi)
		copy(frame, data[soi:end])
		last = frame
		pos = end
	}
	return count, last
}
