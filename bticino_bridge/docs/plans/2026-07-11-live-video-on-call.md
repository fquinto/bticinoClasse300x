# Vídeo en vivo en HA durante llamada — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mostrar en Home Assistant el vídeo en vivo de la puerta cuando entra una llamada a nuestra unidad, de forma cooperativa (`*7*300`, sin self-INVITE), sin la app oficial.

**Architecture:** Un componente `LiveVideoOnCall` se suscribe a los eventos de llamada del bus (`sip.call.incoming`/`.connected`/`.ended`). Al empezar la llamada, marca el servidor RTSP como "llamada activa" (para que no dispare self-INVITE) y envía un `*7*300` de vídeo (tipo 0) al puerto de entrada del relay (10002). El relay ya existente hace fan-out a go2rtc, que sirve el directo a HA por WebRTC. Al colgar, desarma.

**Tech Stack:** Go 1.x (ARM cross-compile), logrus, GStreamer/ffmpeg en device, go2rtc, OpenWebNet.

## Global Constraints

- **SEGURIDAD — nunca reenviar `*7*300` en bucle apretado.** Reintentarlo en bucle contra el estado nativo disparó el watchdog (reinicio) el 2026-07-07. El primitivo `openwebnet.Client.ActivateVideoStream` hace **envío único sin retry**; respétalo. El keepalive por defecto es **0 = envío único**; si >0, cadencia **mínima 5s**.
- **No-interferencia:** monitor solo en :20000; comandos vía `nc` a localhost:30006 (patrón existente). No abrir sockets a 30006/30007/puerto de vídeo. Espaciado ~310 ms entre comandos OWN. `VideoPort`/`ConfigPort` = 0.
- **Sin self-INVITE en este flujo.** La sesión nativa ya existe durante una llamada entrante.
- **Privacidad de edificio:** activar vídeo **solo** para llamadas a NUESTRA unidad (evento `sip.call.incoming`, que es específico de nuestro c300x). Nunca intentar coger la cámara si la ocupa otro piso; el NACK "ocupado" es normal → degradar sin ruido.
- **Idioma:** comentarios y docs en castellano (coincide con el fichero editado).
- **Puertos del relay de entrada:** vídeo `10002`, audio `10000` (`sip.DefaultGStreamerConfig()`).
- **Firma del primitivo de vídeo:** `func (c *openwebnet.Client) ActivateVideoStream(targetIP string, rtpPort int, highRes bool) error` — `highRes=true` → `*7*300#...#<port>#0*##` (tipo 0, hi-res).
- **Build/deploy:** `make build-go` (GOOS=linux GOARCH=arm GOARM=7); `./deploy-standard.sh full`. Tests Go: `go test ./pkg/sip/ -run X -v`.
- **Audio fuera de alcance** (necesita bridge parte-SIP; ver `docs/BACKLOG.md`).

---

### Task 1: Fase 0 — validación en vivo (gate de seguridad y funcionalidad)

**Manual. Requiere el dispositivo real + una llamada de timbre a nuestra unidad + el usuario.** No tiene ciclo de test de código; su salida fija el default de `live_keepalive_seconds` y confirma que el enfoque es seguro antes de habilitarlo en el device.

**Objetivo:** confirmar que **UN** `*7*300` tipo 0 hacia `127.0.0.1:10002` durante una llamada entrante hace que bt_av_media duplique vídeo H.264 y **cuánto tiempo se sostiene** con un único comando (¿toda la llamada? ¿solo un burst?).

- [x] **Step 1: Preparar el script de validación en el device**

Crear `/tmp/video_spike.sh` en el device (vía ssh) — envía UN `*7*300` tipo 0 a 10002 y mide con minicap cuántos segundos llega RTP de vídeo:

```sh
#!/bin/sh
PORT=10002; DUR=40; PCAP=/tmp/video_spike.pcap
rm -f "$PCAP"
nohup /home/bticino/cfg/extra/minicap -o "$PCAP" -t $((DUR+5)) >/tmp/mc.log 2>&1 &
sleep 1
echo "[fase0] *7*300 tipo0 (vídeo, UNA vez) -> 127.0.0.1:$PORT"
echo "*7*300#127#0#0#1#$PORT#0*##" | nc 0 30006
sleep $((DUR+3))
echo "[fase0] === minicap ==="; tail -2 /tmp/mc.log
```

- [x] **Step 2: Coordinar la llamada y ejecutar**

Pedir al usuario: pulsar el timbre de la placa (llamada a NUESTRA unidad) y avisar "en llamada". Entonces: `ssh -o ClearAllForwardings=yes bticino 'sh /tmp/video_spike.sh'`.

- [x] **Step 3: Traer el pcap y medir sostenibilidad**

```bash
DEST=_investigacion/device_backups/video_spike_2026-07-11.pcap
ssh -o ClearAllForwardings=yes bticino 'base64 /tmp/video_spike.pcap' | base64 -d > "$DEST"
# ventana temporal del flujo de vídeo a 10002 (primer/último timestamp):
tcpdump -nn -r "$DEST" 'udp and dst port 10002' 2>/dev/null | sed -n '1p;$p' | awk '{print $1}'
# nº de paquetes:
tcpdump -nn -r "$DEST" 'udp and dst port 10002' 2>/dev/null | wc -l
```

Expected: paquetes RTP a 10002 durante la llamada. **Decisión:**
- Si el flujo dura **toda la llamada** con un único comando → `live_keepalive_seconds: 0` (Task 4). **Preferido.**
- Si solo dura un burst (p.ej. <10s) → anotar la duración; se usará `live_keepalive_seconds` ≈ (duración_burst − 1s), **mínimo 5s**. Si el burst fuese <5s, **parar y replantear** (no se puede reenviar más rápido con seguridad).

- [x] **Step 4: Documentar el resultado**

Añadir el resultado (duración sostenida, decisión de keepalive) a `_investigacion/docs/SPIKE_LISTEN_HA_2026-07-11.md`.

---

**RESULTADO FASE 0 (2026-08-18): GATE FALLIDO — replantear.** Con llamada real, el
`*7*300` propio se ACKea pero se ignora: el nativo ocupa los dos slots de vídeo
(tipo 0→5007, tipo 1→5002, legs SIP locales) en el mismo instante del ring. 0 paquetes
a 10002. A cambio, el vídeo fluye EN CLARO (RTP H.264 PT=96) por loopback a 5007 durante
toda la llamada (~205 pps sostenidos 45 s). **Pivote propuesto:** tap pasivo en `lo`
(pcap `udp and dst port 5007`) → reinyección a 127.0.0.1:10002; cero comandos al bus.
Detalle completo: `_investigacion/docs/SPIKE_LISTEN_HA_2026-07-11.md` (sección 2026-08-18)
y pcap `_investigacion/device_backups/video_spike_2026-08-18.pcap`. Las Tasks 2-6 (ya
implementadas) conservan estructura (eventos, gate de config, SetCallActive, relay), pero
el mecanismo de activación de `LiveVideoOnCall` debe sustituirse por el tap pasivo
(pendiente de decisión con el usuario).

---

### Task 2: Componente `LiveVideoOnCall` (TDD)

**Files:**
- Create: `pkg/sip/live_video.go`
- Test: `pkg/sip/live_video_test.go`

**Interfaces:**
- Consumes: nada de tareas previas.
- Produces:
  - `type VideoActivator interface { ActivateVideoStream(targetIP string, rtpPort int, highRes bool) error }`
  - `type CallActiveMarker interface { SetCallActive(active bool) }`
  - `func NewLiveVideoOnCall(activator VideoActivator, marker CallActiveMarker, videoPort int, keepalive time.Duration, logger *logrus.Logger) *LiveVideoOnCall`
  - `func (l *LiveVideoOnCall) Start(reason string)` (idempotente, no fatal)
  - `func (l *LiveVideoOnCall) Stop()`
  - `func (l *LiveVideoOnCall) IsActive() bool`
  - const `LiveVideoLocalHost = "127.0.0.1"`, `LiveVideoMinKeepalive = 5 * time.Second`

- [ ] **Step 1: Escribir los tests que fallan**

Crear `pkg/sip/live_video_test.go`:

```go
package sip

import (
	"errors"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
)

type mockActivator struct {
	mu    sync.Mutex
	calls []struct {
		ip   string
		port int
		hi   bool
	}
	err error
}

func (m *mockActivator) ActivateVideoStream(ip string, port int, hi bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, struct {
		ip   string
		port int
		hi   bool
	}{ip, port, hi})
	return m.err
}

func (m *mockActivator) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

type mockMarker struct {
	mu     sync.Mutex
	states []bool
}

func (m *mockMarker) SetCallActive(a bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = append(m.states, a)
}

func (m *mockMarker) last() (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.states) == 0 {
		return false, false
	}
	return m.states[len(m.states)-1], true
}

func newTestLive(act VideoActivator, mk CallActiveMarker) *LiveVideoOnCall {
	// keepalive=0 → envío único, determinista (sin goroutine de temporizador).
	return NewLiveVideoOnCall(act, mk, 10002, 0, logrus.New())
}

func TestStartActivatesOnceAndMarks(t *testing.T) {
	act := &mockActivator{}
	mk := &mockMarker{}
	l := newTestLive(act, mk)

	l.Start("test")

	if !l.IsActive() {
		t.Fatal("esperaba IsActive()=true tras Start")
	}
	if act.count() != 1 {
		t.Fatalf("esperaba 1 activación, hubo %d", act.count())
	}
	c := act.calls[0]
	if c.ip != "127.0.0.1" || c.port != 10002 || c.hi != true {
		t.Fatalf("activación con args incorrectos: %+v", c)
	}
	if v, ok := mk.last(); !ok || v != true {
		t.Fatalf("esperaba marker SetCallActive(true), got ok=%v v=%v", ok, v)
	}
}

func TestStartIdempotent(t *testing.T) {
	act := &mockActivator{}
	l := newTestLive(act, &mockMarker{})
	l.Start("a")
	l.Start("b")
	if act.count() != 1 {
		t.Fatalf("esperaba 1 activación (idempotente), hubo %d", act.count())
	}
}

func TestStopDisarmsAndAllowsRestart(t *testing.T) {
	act := &mockActivator{}
	mk := &mockMarker{}
	l := newTestLive(act, mk)
	l.Start("a")
	l.Stop()
	if l.IsActive() {
		t.Fatal("esperaba IsActive()=false tras Stop")
	}
	if v, ok := mk.last(); !ok || v != false {
		t.Fatalf("esperaba marker SetCallActive(false), got ok=%v v=%v", ok, v)
	}
	l.Start("b")
	if act.count() != 2 {
		t.Fatalf("esperaba 2 activaciones tras re-Start, hubo %d", act.count())
	}
}

func TestActivateErrorIsNonFatal(t *testing.T) {
	act := &mockActivator{err: errors.New("NACK ocupado")}
	l := newTestLive(act, &mockMarker{})
	l.Start("busy") // no debe entrar en pánico
	if !l.IsActive() {
		t.Fatal("esperaba IsActive()=true aunque el activador devuelva error")
	}
	if act.count() != 1 {
		t.Fatalf("esperaba 1 intento, hubo %d", act.count())
	}
}

func TestKeepaliveClampedToMinimum(t *testing.T) {
	// keepalive por debajo del mínimo debe subirse al mínimo seguro.
	l := NewLiveVideoOnCall(&mockActivator{}, &mockMarker{}, 10002, 1, logrus.New())
	if l.keepalive != LiveVideoMinKeepalive {
		t.Fatalf("esperaba keepalive clamp a %s, got %s", LiveVideoMinKeepalive, l.keepalive)
	}
}
```

- [ ] **Step 2: Ejecutar los tests para verificar que fallan**

Run: `cd bticino_bridge && go test ./pkg/sip/ -run 'TestStart|TestStop|TestActivate|TestKeepalive' -v`
Expected: FAIL (compilación: `LiveVideoOnCall`, `NewLiveVideoOnCall`, etc. no definidos).

- [ ] **Step 3: Implementar `live_video.go`**

Crear `pkg/sip/live_video.go`:

```go
package sip

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// VideoActivator abstrae el envío del comando *7*300 de vídeo (cooperativo).
// Lo implementa *openwebnet.Client (ActivateVideoStream).
type VideoActivator interface {
	ActivateVideoStream(targetIP string, rtpPort int, highRes bool) error
}

// CallActiveMarker permite marcar que ya hay una llamada nativa en curso, para
// que el servidor RTSP NO dispare un self-INVITE. Lo implementa *EnhancedRTSPServer.
type CallActiveMarker interface {
	SetCallActive(active bool)
}

const (
	// LiveVideoLocalHost es el destino de la duplicación (loopback → relay).
	LiveVideoLocalHost = "127.0.0.1"
	// LiveVideoMinKeepalive es la cadencia MÍNIMA segura de reenvío de *7*300.
	// Reenviar más rápido puede disparar el watchdog del equipo (2026-07-07).
	LiveVideoMinKeepalive = 5 * time.Second
)

// LiveVideoOnCall arma el vídeo en vivo de la puerta durante una llamada entrante
// a NUESTRA unidad, de forma COOPERATIVA (*7*300 tipo 0, sin self-INVITE).
//
// SEGURIDAD (crítico): reenviar *7*300 en bucle apretado provocó un reinicio del
// equipo (watchdog) el 2026-07-07. Por eso el modo por defecto es ENVÍO ÚNICO
// (keepalive=0). Si keepalive>0, el reenvío es a cadencia lenta (mín. 5s) y SIEMPRE
// se detiene al colgar. Cada tick usa ActivateVideoStream (envío único sin retry).
type LiveVideoOnCall struct {
	activator VideoActivator
	marker    CallActiveMarker // puede ser nil
	videoPort int
	keepalive time.Duration
	logger    *logrus.Logger

	mu     sync.Mutex
	active bool
	cancel context.CancelFunc
}

// NewLiveVideoOnCall crea el orquestador. keepalive=0 → envío único (recomendado).
// Un keepalive>0 pero < LiveVideoMinKeepalive se sube al mínimo seguro.
func NewLiveVideoOnCall(activator VideoActivator, marker CallActiveMarker, videoPort int, keepalive time.Duration, logger *logrus.Logger) *LiveVideoOnCall {
	if logger == nil {
		logger = logrus.New()
	}
	if keepalive > 0 && keepalive < LiveVideoMinKeepalive {
		keepalive = LiveVideoMinKeepalive
	}
	return &LiveVideoOnCall{
		activator: activator,
		marker:    marker,
		videoPort: videoPort,
		keepalive: keepalive,
		logger:    logger,
	}
}

// Start arma el vídeo en vivo. Idempotente: si ya está activo, no hace nada.
// No devuelve error: un NACK del dispositivo (p.ej. cámara ocupada por otro piso)
// degrada con elegancia (sin vídeo, sin ruido).
func (l *LiveVideoOnCall) Start(reason string) {
	l.mu.Lock()
	if l.active {
		l.mu.Unlock()
		return
	}
	if l.marker != nil {
		l.marker.SetCallActive(true)
	}
	l.active = true
	if l.keepalive > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		l.cancel = cancel
		go l.keepaliveLoop(ctx)
	}
	l.mu.Unlock()

	l.logger.Infof("live-video: armando (motivo=%s, puerto=%d, keepalive=%s)", reason, l.videoPort, l.keepalive)
	l.activateOnce()
}

// activateOnce envía UN *7*300 de vídeo (hi-res, tipo 0). Errores no fatales.
func (l *LiveVideoOnCall) activateOnce() {
	if l.activator == nil {
		return
	}
	if err := l.activator.ActivateVideoStream(LiveVideoLocalHost, l.videoPort, true); err != nil {
		l.logger.WithError(err).Debug("live-video: *7*300 sin ACK (¿cámara ocupada?) — degradando sin vídeo")
	}
}

func (l *LiveVideoOnCall) keepaliveLoop(ctx context.Context) {
	ticker := time.NewTicker(l.keepalive)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.mu.Lock()
			active := l.active
			l.mu.Unlock()
			if !active {
				return
			}
			l.activateOnce()
		}
	}
}

// Stop desarma (para el keepalive y libera el marcador). Idempotente.
func (l *LiveVideoOnCall) Stop() {
	l.mu.Lock()
	if !l.active {
		l.mu.Unlock()
		return
	}
	l.active = false
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	l.mu.Unlock()

	if l.marker != nil {
		l.marker.SetCallActive(false)
	}
	l.logger.Info("live-video: desarmado")
}

// IsActive indica si el vídeo en vivo está armado.
func (l *LiveVideoOnCall) IsActive() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}
```

- [ ] **Step 4: Ejecutar los tests para verificar que pasan**

Run: `cd bticino_bridge && go test ./pkg/sip/ -run 'TestStart|TestStop|TestActivate|TestKeepalive' -v`
Expected: PASS (5 tests).

- [ ] **Step 5: `go vet` y build ARM**

Run: `cd bticino_bridge && go vet ./pkg/sip/ && make build-go`
Expected: sin errores; binario ARM generado.

- [ ] **Step 6: Commit**

```bash
git add bticino_bridge/pkg/sip/live_video.go bticino_bridge/pkg/sip/live_video_test.go
git commit -m "feat(sip): LiveVideoOnCall — vídeo en vivo cooperativo durante llamada

Componente que arma *7*300 tipo 0 al puerto del relay durante una llamada
entrante, sin self-INVITE. Envío único por defecto (keepalive=0) por seguridad
(reenvío en bucle disparó el watchdog el 2026-07-07). TDD con mocks.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: `EnhancedRTSPServer.SetCallActive` + reordenar `ensureSIPCallActive`

**Files:**
- Modify: `pkg/sip/rtsp_server_enhanced.go` (añadir método `SetCallActive`; reordenar guardas en `ensureSIPCallActive`, ~líneas 946-960)

**Interfaces:**
- Consumes: nada.
- Produces: `func (r *EnhancedRTSPServer) SetCallActive(active bool)` — satisface `sip.CallActiveMarker`.

- [ ] **Step 1: Añadir el método `SetCallActive`**

Añadir en `pkg/sip/rtsp_server_enhanced.go` (junto a los otros setters, p.ej. tras `SetVideoBackend`):

```go
// SetCallActive marca si hay una llamada nativa en curso. Cuando es true,
// ensureSIPCallActive() OMITE el self-INVITE (la sesión de vídeo ya existe).
// Lo usa LiveVideoOnCall durante una llamada entrante.
func (r *EnhancedRTSPServer) SetCallActive(active bool) {
	r.mutex.Lock()
	r.callActive = active
	r.mutex.Unlock()
	if active {
		r.logger.Info("RTSP: callActive=true (self-INVITE omitido; sesión nativa en curso)")
	} else {
		r.logger.Info("RTSP: callActive=false")
	}
}
```

- [ ] **Step 2: Reordenar `ensureSIPCallActive` (callActive ANTES del gate videoActivation)**

Reemplazar el principio de `ensureSIPCallActive()` (el bloque `if !r.videoActivation {...}` seguido de la comprobación de `callActive`) por:

```go
func (r *EnhancedRTSPServer) ensureSIPCallActive() error {
	// Si ya hay una llamada nativa en curso (marcada por LiveVideoOnCall), NO
	// hace falta self-INVITE: la sesión de vídeo ya existe. Se comprueba ANTES
	// del gate videoActivation para no bloquear el flujo ni ensuciar logs cuando
	// go2rtc conecta durante una llamada.
	r.mutex.Lock()
	if r.callActive {
		r.mutex.Unlock()
		r.logger.Debug("SIP call already active (native call in progress), skipping self-INVITE")
		return nil
	}
	r.mutex.Unlock()

	// SEGURIDAD: si la activación de vídeo bajo demanda está desactivada, no
	// disparamos NADA hacia el dispositivo nativo (ni self-INVITE ni *7*300).
	if !r.videoActivation {
		r.logger.Warn("Video on-demand disabled: ignoring RTSP/snapshot activation request")
		return fmt.Errorf("video on-demand activation is disabled")
	}

	r.logger.Info("=== ENSURESIP: Initiating SIP call for RTSP streaming ===")
	// ... (resto sin cambios: SIPTarget, MakeCall, activateMediaStreamsWhenConnected)
```

(Mantener el resto del cuerpo original a partir de `target := r.sipClient.config.SIPTarget`.)

- [ ] **Step 3: Verificar build + vet**

Run: `cd bticino_bridge && go vet ./pkg/sip/ && go build ./...`
Expected: sin errores. (No hay unit test: `ensureSIPCallActive` depende de un `sipClient` real; se valida en la integración con device, Task 7.)

- [ ] **Step 4: Verificar que `*EnhancedRTSPServer` satisface `CallActiveMarker`**

Añadir una aserción de compilación al final de `rtsp_server_enhanced.go`:

```go
// Aserción de compilación: el servidor RTSP satisface CallActiveMarker.
var _ CallActiveMarker = (*EnhancedRTSPServer)(nil)
```

Run: `cd bticino_bridge && go build ./...`
Expected: sin errores.

- [ ] **Step 5: Commit**

```bash
git add bticino_bridge/pkg/sip/rtsp_server_enhanced.go
git commit -m "feat(sip): SetCallActive en RTSP + skip self-INVITE si llamada nativa activa

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Config — `live_on_call` y `live_keepalive_seconds`

**Files:**
- Modify: `pkg/config/config.go` (struct `StreamingConfig` ~línea 160; defaults en `GetStreamingConfig` ~línea 194)
- Modify: `configs/config.yaml` (sección `streaming:` ~línea 86)

**Interfaces:**
- Produces: `cfg.Streaming.LiveOnCall bool`, `cfg.Streaming.LiveKeepaliveSeconds int`.

- [ ] **Step 1: Añadir campos al struct `StreamingConfig`**

Tras el campo `VideoOnDemand` en `pkg/config/config.go`:

```go
	// LiveOnCall habilita el vídeo en vivo COOPERATIVO durante una llamada
	// entrante a nuestra unidad (*7*300 tipo 0, sin self-INVITE). Seguro: mismo
	// mecanismo cooperativo que el snapshot al sonar. Independiente de VideoOnDemand.
	LiveOnCall bool `yaml:"live_on_call"`
	// LiveKeepaliveSeconds: cada cuántos segundos re-armar el *7*300 de vídeo
	// durante la llamada. 0 = ENVÍO ÚNICO (por defecto, lo más seguro). Si >0, el
	// mínimo efectivo son 5s: reenviar en bucle apretado reinició el equipo el
	// 2026-07-07. Ajustar solo si la Fase 0 muestra que un envío único no sostiene
	// el vídeo toda la llamada.
	LiveKeepaliveSeconds int `yaml:"live_keepalive_seconds"`
```

- [ ] **Step 2: Añadir defaults en `GetStreamingConfig`**

En el `return StreamingConfig{...}` de `GetStreamingConfig()`, añadir:

```go
		LiveOnCall:           true,
		LiveKeepaliveSeconds: 0,
```

(Default `LiveOnCall: true` porque es cooperativo y seguro, igual que el snapshot al sonar. Ajustar `LiveKeepaliveSeconds` según Task 1.)

- [ ] **Step 3: Añadir a `configs/config.yaml`**

En la sección `streaming:`, tras `video_backend: "avmedia"`:

```yaml
  # Vídeo en vivo cooperativo durante una llamada entrante a NUESTRA unidad
  # (*7*300 tipo 0, sin self-INVITE). Seguro; independiente de video_on_demand.
  live_on_call: true
  # Re-armado del *7*300 durante la llamada. 0 = envío único (recomendado y por
  # defecto). Si >0, mínimo efectivo 5s (reenviar en bucle apretado reinició el
  # equipo el 2026-07-07). Ajustar solo si la Fase 0 muestra que un envío único
  # no sostiene el vídeo toda la llamada.
  live_keepalive_seconds: 0
```

- [ ] **Step 4: Verificar build**

Run: `cd bticino_bridge && go build ./... && go run ./cmd/main.go -test -config configs/config.yaml 2>&1 | head -5`
Expected: compila; el binario en modo `-test` arranca leyendo la config sin error de parseo.

- [ ] **Step 5: Commit**

```bash
git add bticino_bridge/pkg/config/config.go bticino_bridge/configs/config.yaml
git commit -m "feat(config): streaming.live_on_call + live_keepalive_seconds

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 5: Cableado en `main.go` (suscripción a eventos de llamada)

**Files:**
- Modify: `cmd/main.go` (tras el setup de `rtspServer` y con `owClient`/`eventBus` disponibles, ~línea 500)

**Interfaces:**
- Consumes: `sip.NewLiveVideoOnCall`, `(*EnhancedRTSPServer).SetCallActive`, `sip.DefaultGStreamerConfig()`, `cfg.Streaming.LiveOnCall/LiveKeepaliveSeconds`, eventos `sip.call.incoming/.connected/.ended`.
- Produces: nada (efecto de arranque).

- [ ] **Step 1: Añadir el cableado**

Tras `rtspServer.SetVideoActivation(cfg.Streaming.VideoOnDemand)` en `cmd/main.go`:

```go
		// Vídeo en vivo COOPERATIVO durante una llamada entrante a nuestra unidad
		// (*7*300 tipo 0, sin self-INVITE). Independiente de video_on_demand.
		// `sip.call.incoming` es específico de nuestro c300x (lo publica el cliente
		// SIP al recibir un INVITE dirigido a nosotros), no actividad de otros pisos.
		if cfg.Streaming.LiveOnCall && owClient != nil {
			keepalive := time.Duration(cfg.Streaming.LiveKeepaliveSeconds) * time.Second
			liveVideo := sip.NewLiveVideoOnCall(owClient, rtspServer,
				sip.DefaultGStreamerConfig().VideoPort, keepalive, logger)

			eventBus.Subscribe("sip.call.incoming", func(_ *events.Event) {
				liveVideo.Start("sip.call.incoming")
			})
			eventBus.Subscribe("sip.call.connected", func(_ *events.Event) {
				liveVideo.Start("sip.call.connected")
			})
			eventBus.Subscribe("sip.call.ended", func(_ *events.Event) {
				liveVideo.Stop()
			})
			logger.Info("   Live video on call: habilitado (cooperativo, sin self-INVITE)")
		}
```

- [ ] **Step 2: Verificar imports**

Comprobar que `cmd/main.go` importa `time` y `bticino_bridge/pkg/events` (ya se usan en el fichero). Si falta alguno, añadirlo.

Run: `cd bticino_bridge && go build ./...`
Expected: compila sin errores.

- [ ] **Step 3: Verificar arranque en modo test**

Run: `cd bticino_bridge && go run ./cmd/main.go -test -config configs/config.yaml 2>&1 | grep -i "live video"`
Expected: línea `Live video on call: habilitado` (si el owClient se inicializa en modo test; si no, al menos compila y no rompe el arranque).

- [ ] **Step 4: Commit**

```bash
git add bticino_bridge/cmd/main.go
git commit -m "feat(main): cablear LiveVideoOnCall a eventos sip.call.* (gateado por config)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 6: go2rtc + documentación HA

**Files:**
- Modify: `docs/HOME_ASSISTANT.md` (sección nueva: vídeo en vivo durante llamada)
- Reference: `pkg/sip/go2rtc_manager.go` (config `doorbell-video` ya existe: `rtsp://<ip>:6554/doorbell-video`)

**Interfaces:** ninguna (config/ops/doc).

- [ ] **Step 1: Verificar que go2rtc apunta a `/doorbell-video`**

Confirmar en el device que la config go2rtc incluye un stream que tira de
`rtsp://127.0.0.1:6554/doorbell-video` (solo vídeo, sin audio, que es el alcance).
Si la config actual (hecha a mano/Companion) no lo tiene o mezcla audio inexistente,
añadir/ajustar un stream `doorbell-video`:

```yaml
streams:
  doorbell-video: rtsp://127.0.0.1:6554/doorbell-video
```

Decisión de ownership (del spec §11.2): **no sobrescribir** la config Companion
existente en este MVP; **añadir** el stream `doorbell-video` si falta, coexistiendo.
Documentar en el commit qué se tocó.

- [ ] **Step 2: Documentar en HA**

Añadir a `docs/HOME_ASSISTANT.md` una sección "Vídeo en vivo durante una llamada":

```markdown
## Vídeo en vivo durante una llamada (live_on_call)

Cuando entra una llamada a la unidad, el bridge activa de forma cooperativa el
vídeo de la puerta y lo publica por go2rtc. Para verlo en vivo en HA:

- go2rtc expone WebRTC en `http://<ip-device>:1984` (stream `doorbell-video`).
- En HA, usar la tarjeta `custom:webrtc-camera` o la integración go2rtc apuntando
  al stream `doorbell-video`, o añadir una cámara `generic`/`rtsp` a
  `rtsp://<ip-device>:6554/doorbell-video`.
- Fuera de llamada no hay vídeo (es normal): la entidad `camera.camara_timbre`
  sigue mostrando el último snapshot como preview.

Solo funciona para llamadas a **nuestra** unidad; si el videoportero está ocupado
por una llamada a otro piso, no hay vídeo (privacidad de edificio).
```

- [ ] **Step 3: Commit**

```bash
git add bticino_bridge/docs/HOME_ASSISTANT.md
git commit -m "docs(ha): vídeo en vivo durante llamada (go2rtc doorbell-video)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 7: Integración en device + no-regresión + CHANGELOG

**Files:**
- Modify: `CHANGELOG.md`, `VERSION`

**Interfaces:** ninguna.

- [x] **Step 1: Estado de salud PRE**

```bash
ssh -o ClearAllForwardings=yes bticino 'ps | grep -E "bt_av_media|bt_vct|openserver" | grep -v grep'
```
Expected: los 3 procesos nativos vivos. Anotar PIDs.

- [x] **Step 2: Desplegar**

Run: `./deploy-standard.sh full`
Expected: build ARM + deploy + health check OK; el bridge arranca y loguea `Live video on call: habilitado`.

- [x] **Step 3: Prueba en vivo (con el usuario)** — timbrazo real 17:04: arme por doorbell.pressed + frame 688×480 vía go2rtc OK; cazó un panic de teardown (corregido con TDD). Desarme verificado en el timbrazo del 2026-08-19 10:01: ciclo completo arme (doorbell.pressed) → frame en vivo por go2rtc (19,6 KB) → desarme al colgar (call.hangup, 27 s de llamada, 2770 paquetes RTP espejados, tap parado limpio), sin panics y nativos 2/2. E2E CERRADO.

Pedir al usuario una llamada de timbre a nuestra unidad. Verificar en logs del bridge:
```bash
ssh -o ClearAllForwardings=yes bticino 'tail -40 /tmp/bridge.log' | grep -iE "live-video|callActive|sip.call"
```
Expected: `live-video: armando`, `RTSP: callActive=true`. Abrir la cámara en HA / go2rtc (`http://<ip>:1984`) durante la llamada y **confirmar imagen en vivo**. Al colgar: `live-video: desarmado`, `callActive=false`.

- [x] **Step 4: No-regresión de nativos** — bt_vct/bt_av_media intactos tras 2 timbrazos y 3 despliegues (uptime device 39 días).

```bash
ssh -o ClearAllForwardings=yes bticino 'ps | grep -E "bt_av_media|bt_vct|openserver" | grep -v grep'
```
Expected: los 3 procesos siguen vivos (mismos o reiniciados limpiamente, sin watchdog). Confirmar que NO hubo reinicio del equipo (uptime estable): `ssh ... 'uptime'`.

- [ ] **Step 5: Prueba de gate off (no-regresión)**

Poner `live_on_call: false` en la config, redeploy, y confirmar que el comportamiento vuelve al actual (sin activación de vídeo por llamada). Restaurar a `true` después.

- [x] **Step 6: CHANGELOG + VERSION + commit** — v0.18.0.

Añadir entrada a `CHANGELOG.md` (nueva versión menor) describiendo el vídeo en vivo cooperativo durante llamada, y bump de `VERSION`.

```bash
git add bticino_bridge/CHANGELOG.md bticino_bridge/VERSION
git commit -m "chore: vídeo en vivo durante llamada — CHANGELOG + version bump

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Notas de ejecución

- **Orden recomendado:** Task 1 (Fase 0, gate de seguridad) **primero** — si un envío único no sostiene el vídeo y el keepalive seguro (≥5s) no basta, replantear antes de escribir código de más. Tasks 2-6 son offline (código + unit + docs) y pueden avanzar en paralelo a la espera de la ventana de llamada para Task 1/7. Task 7 cierra en device.
- **Fuera de alcance (BACKLOG):** audio (bridge parte-SIP), talk-back, "llamar a casa", clasificador de accesos por cerradura.

### Diferido explícitamente de este MVP (con justificación)
- **Auto-stop por timeout de seguridad (spec §10):** si `sip.call.ended` no llegara,
  `active`/`callActive` quedarían colgados. Se difiere porque el default `keepalive=0`
  no envía comandos repetidos (el único efecto de un cuelgue perdido es que un futuro
  self-INVITE se omita), y `sip.call.ended` se publica de forma fiable desde varios
  puntos del cliente SIP. Si en la integración (Task 7) se observa estado colgado,
  añadir un `time.AfterFunc` de guarda en `Start` que llame a `Stop` tras N minutos.
- **Limpieza de sesión genuinamente colgada (spec §11.6):** caso raro; no se
  automatiza en el MVP. Comandos manuales documentados: `*8*3#5#4*416##` + `*7*219*##`.
