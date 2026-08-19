package sip

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/sirupsen/logrus"
)

// TalkbackInjector envía audio ascendente (talk-back) hacia el portero c300x.
//
// Mecanismo (reverseado 2026-07-09; ver _investigacion/docs/
// TALKBACK_AUDIO_ASCENDENTE_2026-07-09.md): el audio de la llamada NO viaja por
// SRTP real — el c300x usa una clave crypto "dummy" en el SDP, así que el media
// es RTP PLANO. El talk-back consiste en enviar RTP Speex/8000 (payload type
// 110, el codec de audio del c300x) al puerto de recepción de audio del portero,
// con la sesión de audio bidireccional ARMADA mediante el comando OpenWebNet
//   *8*2#5#4*16##  (CmdAnswerPickup — "activar audio bidireccional").
//
// Este componente maneja SOLO el pipeline de media (GStreamer). El comando de
// armado lo envía quien invoca (webserver / bridge) a través del cliente OWN.
//
// Pipeline en el device (tiene speexenc + rtpspeexpay; NO tiene audiotestsrc):
//
//	filesrc/multifilesrc → decodebin → audioconvert → audioresample →
//	audio/x-raw,rate=8000,channels=1 → speexenc → rtpspeexpay pt=110 → udpsink
//
// El puerto de recepción del portero es SDP-negociado por sesión (dinámico), por
// eso Start() lo recibe como parámetro: durante una llamada activa se prueba el
// puerto observado (ver el doc). Es una herramienta para cerrar el RE del
// talk-back — no un camino de producción todavía.
type TalkbackInjector struct {
	logger *logrus.Logger
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	active bool
	target string // "host:port" en curso (informativo)
}

// NewTalkbackInjector crea el inyector.
func NewTalkbackInjector(logger *logrus.Logger) *TalkbackInjector {
	return &TalkbackInjector{logger: logger}
}

// Start lanza el pipeline de subida hacia host:port usando `source` (ruta a un
// fichero de audio decodificable: WAV, etc.). Si loop es true, el fichero se
// repite de forma continua (multifilesrc). El armado del audio bidireccional
// NO se hace aquí (lo hace el llamante con el comando OWN).
//
// pt es el RTP payload type del Speex/8000. IMPORTANTE: el c300x anuncia
// `a=rtpmap:98 speex/8000` en el SDP de audio, así que el PT correcto es 98
// (si pt<=0 se usa 98). Un PT distinto (p.ej. 110) hace que el portero IGNORE
// el stream — confirmado en el test en vivo de 2026-07-09.
func (t *TalkbackInjector) Start(host string, port, pt int, source string, loop bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.active {
		return fmt.Errorf("talkback ya activo hacia %s", t.target)
	}
	if source == "" {
		return fmt.Errorf("talkback: falta la fuente de audio (ruta a un fichero WAV)")
	}
	if port <= 0 {
		return fmt.Errorf("talkback: puerto de destino inválido (%d)", port)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if pt <= 0 {
		pt = 98 // c300x: a=rtpmap:98 speex/8000
	}

	var src []string
	if loop {
		src = []string{"multifilesrc", "location=" + source, "loop=true"}
	} else {
		src = []string{"filesrc", "location=" + source}
	}
	args := append([]string{}, src...)
	args = append(args,
		// El device tiene gstreamer mínimo (SIN decodebin) → se usa wavparse.
		// Por tanto `source` debe ser un WAV PCM. Loop con multifilesrc es
		// best-effort (wavparse puede no reencadenar cabeceras limpiamente).
		"!", "wavparse",
		"!", "audioconvert",
		"!", "audioresample",
		"!", "audio/x-raw,rate=8000,channels=1",
		"!", "speexenc",
		"!", "rtpspeexpay", fmt.Sprintf("pt=%d", pt),
		"!", "udpsink", "host="+host, fmt.Sprintf("port=%d", port),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "gst-launch-1.0", args...)

	t.logger.Infof("talkback: subiendo audio Speex/8000 (pt=%d) → %s:%d (fuente=%s, loop=%v)",
		pt, host, port, source, loop)
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("talkback: no se pudo lanzar gstreamer: %w", err)
	}

	t.cmd = cmd
	t.cancel = cancel
	t.active = true
	t.target = fmt.Sprintf("%s:%d", host, port)

	// Vigilar el fin del pipeline para limpiar el estado.
	go func() {
		_ = cmd.Wait()
		t.mu.Lock()
		t.active = false
		t.mu.Unlock()
		t.logger.Info("talkback: pipeline terminado")
	}()

	return nil
}

// Stop detiene el pipeline en curso (si lo hay).
func (t *TalkbackInjector) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active {
		return
	}
	if t.cancel != nil {
		t.cancel()
	}
	t.active = false
	t.logger.Infof("talkback: detenido (%s)", t.target)
}

// IsActive indica si hay un pipeline de talk-back en curso.
func (t *TalkbackInjector) IsActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

// Target devuelve el destino "host:port" en curso (vacío si inactivo).
func (t *TalkbackInjector) Target() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active {
		return ""
	}
	return t.target
}
