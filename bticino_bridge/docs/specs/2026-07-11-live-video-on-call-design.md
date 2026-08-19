# Spec de diseño — Vídeo en vivo en HA durante una llamada entrante

**Fecha:** 2026-07-11
**Estado:** SUPERSEDIDO en el mecanismo de activación (2026-08-18). La Fase 0 con
timbrazo real demostró que el `*7*300` propio se ACKea pero se ignora durante una
llamada (el nativo ocupa los dos slots de vídeo con sus legs SIP: tipo 0→5007,
tipo 1→5002). El mecanismo vigente es un **espejo pasivo** (tap AF_PACKET en
`lo:5007`, RTP H.264 en claro, → relay 10002) con CERO comandos al bus — ver
`pkg/sip/rtp_tap.go` y el resultado de Fase 0 en el plan
(`docs/plans/2026-07-11-live-video-on-call.md`). El resto del diseño (eventos,
gating de privacidad, relay→RTSP→go2rtc→HA) sigue vigente.
**Autor:** sesión de brainstorming (BTicino bridge)
**Supersede parcialmente:** `docs/ROADMAP_VIDEO_AUDIO.md` (fase de vídeo)

## 1. Objetivo

Cuando suena el timbre de la **placa de entrada**, mostrar en Home Assistant el
**vídeo en vivo** de la puerta (no solo el snapshot que ya existe), **sin la app
oficial** y **sin interferir** con los procesos nativos. El usuario sigue
atendiendo/hablando por panel o app; HA es para **ver quién llama en directo**.

El **audio** queda **fuera de alcance** de este spec (ver §12): el spike del
2026-07-11 demostró que la puerta solo emite audio como RTP cuando hay una parte
SIP en la llamada, lo que requiere una fase distinta ("el bridge como parte SIP").

## 2. Estado actual (infra que ya existe y se reutiliza)

- **`EnhancedRTSPServer`** (`pkg/sip/rtsp_server_enhanced.go`, puerto 6554) con
  streams `/doorbell` (v+a), `/doorbell-video` (solo vídeo), `/doorbell-recorder`.
- **`RTPRelayPair`** (`pkg/sip/rtp_relay.go`): escucha RTP en **10002 (vídeo)** y
  **10000 (audio)** y hace fan-out a los clientes RTSP (go2rtc). Ya funciona.
- **`Go2RTCManager`** (`pkg/sip/go2rtc_manager.go`): gestiona go2rtc, que tira de
  `rtsp://<ip>:6554/doorbell` y lo publica por WebRTC/HLS a HA.
- **Captura cooperativa** (`pkg/sip/avmedia_capture.go`): patrón probado de enviar
  **un** `*7*300` para que bt_av_media duplique su RTP a un puerto local. El
  snapshot al sonar el timbre ya lo usa.
- **Eventos de llamada en el bus**: `sip.call.incoming`, `sip.call.connected`,
  `sip.call.ended` (el snapshot ya se suscribe a `sip.call.incoming`).
- **Gate de seguridad**: `EnhancedRTSPServer.videoActivation` (config
  `streaming.video_on_demand`, **desactivado por defecto**). Cuando está en false,
  el servidor RTSP escucha pero **no dispara nada** al firmware.

### Por qué el path on-demand actual está desactivado
`ensureSIPCallActive()` hace un **self-INVITE** (`MakeCall("c300x")`) para crear
artificialmente una sesión de vídeo y luego la alimenta con `*7*300`/GStreamer.
Ese self-INVITE es lo que dio problemas en el dispositivo real (v0.16.1), por eso
`video_on_demand` está en false por defecto.

## 3. Hallazgos del spike (2026-07-11) que fundamentan el diseño

(Detalle completo en `_investigacion/docs/SPIKE_LISTEN_HA_2026-07-11.md`.)

- Durante una **llamada entrante**, bt_av_media **ya produce vídeo H.264 RTP de
  forma continua** (visto 45 s seguidos). → **No hace falta self-INVITE.**
- El vídeo se emite **incluso con la llamada sonando y sin contestar** (en la toma,
  el RTP iba a 5007/5002 sin consumidor → 12.526 ICMP "port unreachable"). → Se puede
  obtener vídeo para "ver quién llama" **antes del descuelgue**, lo que encaja con el
  caso de uso. Solo hay que redirigir una copia a nuestro puerto con `*7*300` tipo 1.
- **Bloqueo por privacidad a nivel de edificio (clave):** cuando pican a un piso, el
  videoportero queda **ocupado para TODO el edificio** hasta que ese piso cuelgue o
  venza el timeout (un vecino no puede ver por la cámara la llamada de otro piso). Es
  **comportamiento de diseño**, no un fallo. → El bridge **solo** puede obtener vídeo
  cuando la llamada es **a nuestra unidad**; para llamadas a otros pisos la cámara está
  ocupada (NACK esperado) y **no debe intentar cogerla** (privacidad + no-interferencia
  a nivel de edificio). En la toma del spike, ese estado "ocupado" explica los RTP a
  puertos muertos. (Distinto de una sesión *genuinamente* colgada, ver §11.5.)
- El `*7*300` **tipo 1** duplica vídeo; **tipo 2** duplica audio (Speex PT=110).
- El audio **no** existe como RTP en llamadas atendidas solo en panel (va por DSP
  interno). → El vídeo es cooperativo y fiable; el audio no. Por eso este spec es
  **solo vídeo**.

## 4. Alcance

**Incluido:**
- Vídeo en vivo de la placa en HA mientras dura una llamada entrante.
- Disparo automático por evento de llamada; parada al colgar.
- Camino **cooperativo** (`*7*300` tipo 1), **sin self-INVITE**.
- Nuevo gate de config independiente y seguro.

**Excluido (fases futuras, §12):** audio (requiere bridge-parte-SIP), talk-back
bidireccional, "llamar a casa", vídeo bajo demanda en reposo (sin llamada).

## 5. Arquitectura propuesta

```
Timbre placa → monitor OWN (:20000) → evento sip.call.incoming / .connected
                                              │
                    ┌─────────────────────────┴───────────────────────┐
                    │ (ya existe) snapshot cooperativo                 │
                    │   → camera.camara_timbre (MQTT base64)           │
                    │                                                  │
                    │ NUEVO: LiveVideoOnCall                           │
                    │   1. marca RTSP.callActive=true (evita self-INVITE)
                    │   2. *7*300 tipo1 (vídeo) → 127.0.0.1:10002 [keepalive]
                    │   3. bt_av_media duplica su H.264 RTP → relay    │
                    └─────────────────────────┬───────────────────────┘
                                              │
                     RTPRelayPair (10002) ── fan-out ──> go2rtc (rtsp 6554/doorbell-video)
                                              │
                                         WebRTC/HLS → tarjeta cámara en HA (EN VIVO)
                                              │
                    evento sip.call.ended → LiveVideoOnCall.Stop()
                                              (para keepalive; *7*0 opcional)
```

### Principio rector
Reutilizar el **mismo mecanismo cooperativo del snapshot**, pero **sostenido**
durante la llamada, y **enganchado a los eventos de llamada** en vez de a la
conexión de un cliente RTSP (que es lo que hoy dispara el self-INVITE).

## 6. Componentes y responsabilidades

### 6.1 `LiveVideoOnCall` (nuevo, `pkg/sip/live_video.go`)
- **Qué hace:** orquesta el ciclo de vida del vídeo en vivo durante una llamada.
- **Interfaz:**
  - `Start(reason string) error` — marca la sesión activa y arranca el keepalive
    de `*7*300` tipo 1 hacia el puerto del relay (10002).
  - `Stop()` — detiene el keepalive; el relay se queda sin fuente (los consumidores
    go2rtc/HA verán fin de stream).
  - `IsActive() bool`.
- **Depende de:** `openwebnet.Client` (para `ActivateVideoStream`), el
  `EnhancedRTSPServer` (para marcar `callActive` y no disparar self-INVITE), un
  `logger`.
- **Keepalive:** re-enviar `*7*300` tipo 1 cada N segundos (a validar, ver §11). El
  envío sigue siendo **cooperativo** (duplicar, no competir) y respeta el espaciado
  de ~310 ms de BTicino.

### 6.2 Cambios en `EnhancedRTSPServer`
- Nuevo método `SetCallActive(bool)` (o exponer un flag) para que `LiveVideoOnCall`
  marque que **ya hay una llamada nativa**; así `ensureSIPCallActive()` ve
  `callActive==true` y **omite el `MakeCall`** (línea ya existente que hace
  `if r.callActive { return nil }`).
- Nuevo gate `SetLiveOnCall(bool)` independiente de `videoActivation`.

### 6.3 Disparo (en `cmd/main.go` o donde se cablean las suscripciones)
- Suscribir `LiveVideoOnCall.Start` a `sip.call.incoming` **y/o**
  `sip.call.connected` (idempotente: si ya activo, no-op).
- **Solo para llamadas a NUESTRA unidad** (nuestro c300x), no para actividad genérica
  del bus ni llamadas a otros pisos (ver §11.5, privacidad de edificio).
- Suscribir `LiveVideoOnCall.Stop` a `sip.call.ended`.
- Gateado por `cfg.Streaming.LiveOnCall`.

### 6.4 go2rtc
- El bridge debe **tomar control** de la config go2rtc de los streams `doorbell` /
  `doorbell-video` (hoy hay un `go2rtc.yaml` hecho a mano estilo Companion que
  `ensureConfigExists()` **no** sobrescribe). Decisión: que el bridge genere/gestione
  su config, o coexista de forma explícita. (Ver §11, decisión pendiente.)

## 7. Configuración (`configs/config.yaml`)

```yaml
streaming:
  video_on_demand: false      # (existente) self-INVITE en reposo — sigue en false
  live_on_call: true          # NUEVO: vídeo en vivo durante llamada (cooperativo, seguro)
  video_backend: avmedia      # (existente) *7*300 cooperativo
  live_keepalive_seconds: 5   # NUEVO: cada cuánto re-armar *7*300 tipo1 (0 = una vez)
```

`live_on_call` puede ir a **true por defecto**: usa exactamente el mismo mecanismo
que el snapshot-al-sonar (ya activo por defecto), sin self-INVITE. Decisión de
default a confirmar con el usuario.

## 8. Seguridad / no-interferencia (restricción crítica)

- **Nunca** self-INVITE en este flujo. Solo `*7*300` (duplicar), como el snapshot.
- Monitor solo en :20000; comandos vía `nc` a localhost:30006 (patrón existente).
- No abrir sockets a 30006/30007/puerto de vídeo nativo.
- Respetar espaciado ~310 ms entre comandos OWN.
- Si no hay llamada activa, **no** se envía ningún `*7*300` (el vídeo en reposo NACKea
  y no debe forzarse).
- **No-interferencia a nivel de edificio:** solo activar vídeo para llamadas a nuestra
  unidad. Nunca intentar coger la cámara si está ocupada por otro piso (§11.5) —
  privacidad y para no romper la llamada del vecino.

## 9. Integración con Home Assistant

- El vídeo en vivo se sirve por **go2rtc** (WebRTC/HLS), no por MQTT. La entidad
  `camera.camara_timbre` mantiene el **snapshot** (MQTT base64) como
  `still`/preview; el **stream en vivo** se expone vía go2rtc.
- Documentar en `docs/HOME_ASSISTANT.md` cómo apuntar HA al stream go2rtc
  (WebRTC en `http://<ip>:1984` o la tarjeta `webrtc`/`go2rtc`).
- Idealmente: mientras dura la llamada, la tarjeta de cámara reproduce el directo;
  fuera de llamada, muestra el último snapshot.

## 10. Manejo de errores

- `*7*300` NACK (sin sesión nativa) → log + no reintentar agresivamente; el vídeo en
  vivo simplemente no aparece (degradación elegante; el snapshot sigue).
- go2rtc caído → el `Go2RTCManager` ya monitoriza/reinicia.
- Llamada que termina sin evento `sip.call.ended` → timeout de seguridad en
  `LiveVideoOnCall` (auto-stop a los N s, como el auto-stop de 60 s de `video.go`).

## 11. Riesgos y validaciones pendientes (antes/durante implementación)

1. **Sostenibilidad del `*7*300` tipo 1 hacia NUESTRO puerto (10002):** el spike
   confirmó que el *nativo* mantiene vídeo 45 s a *sus* puertos, pero no que un único
   `*7*300` nuestro a 10002 dure toda la llamada. **Validación Fase 0** (test en vivo:
   enviar `*7*300` tipo1 a 10002 durante una llamada y medir cuántos segundos llega
   RTP). Define si hace falta keepalive y su periodo.
2. **Ownership de go2rtc.yaml:** decidir si el bridge sobrescribe la config actual
   (hecha a mano/Companion) o coexiste. Riesgo de pisar la config del backchannel.
3. **Interacción con el snapshot:** el snapshot y el vídeo en vivo comparten el
   mecanismo `*7*300`; verificar que no compiten (secuenciar: snapshot primero, luego
   arrancar el directo, o compartir la misma activación).
4. **Default de `live_on_call`:** true (cómodo) vs false (conservador). A confirmar.
5. **Cámara ocupada por otra llamada (privacidad de edificio) — esperado:** si el
   videoportero está ocupado por una llamada a **otro piso**, `*7*300` NACKea. Es
   **normal**, no un error. `LiveVideoOnCall` debe: (a) activarse **solo** para llamadas
   a **nuestra unidad** (usar el evento `sip.call.incoming` de nuestro c300x, no
   actividad genérica del bus), (b) tratar el NACK "ocupado" como **degradación
   elegante** (sin vídeo, sin ruido de logs), y (c) **nunca** intentar arrebatar la
   cámara a la llamada de otro piso (interferencia a nivel de edificio). El estado se
   libera solo al colgar el otro piso o por timeout.
6. **Sesión genuinamente colgada (raro):** dim 35 no vuelve a 0 tras colgar todos.
   Caso distinto del anterior; limpieza segura opcional (`*8*3#5#4*416##` +
   `*7*219*##`), manual o automática con guardas. No confundir con el bloqueo por
   privacidad (que es normal y se resuelve solo).

## 12. Fuera de alcance — fases futuras

- **Audio en vivo (escuchar):** requiere que el **bridge sea parte SIP** de la
  llamada (auto-answer como endpoint de audio) para que la puerta emita Speex RTP;
  luego transcodificar Speex→Opus con el ffmpeg del device (`/usr/local/bin/ffmpeg`,
  tiene libspeex+libopus) → go2rtc. Reutiliza la cuenta SIP standalone (200 OK
  verificado) y el auto-answer de INVITEs existente.
- **Talk-back (hablar):** inyección de audio ascendente; ya hay `talkback.go` y el
  RE hecho, pero requiere que el bridge posea la llamada (colisión SSRC si se inyecta
  en paralelo a la app).
- **"Llamar a casa":** el bridge origina una llamada SIP al interfono (intercom
  saliente). Su audio NO pasa por bt_av_media (confirmado en el spike).

## 13. Testing

- **Fase 0 (manual/vivo):** medir sostenibilidad `*7*300` tipo1 → 10002 durante una
  llamada real (script tipo el del spike, con minicap + conteo de paquetes/tiempo).
- **Unit:** `LiveVideoOnCall` con un `openwebnet.Client` mock — verificar que
  Start/Stop arma/desarma el keepalive y que Start es idempotente.
- **Integración (device):** llamada real → confirmar en HA (go2rtc) que el directo
  aparece durante la llamada y se corta al colgar; confirmar que los nativos siguen
  sanos (`ps` de bt_av_media/bt_vct/openserver antes y después).
- **No-regresión:** con `live_on_call:false`, comportamiento idéntico al actual.

## 14. Plan de fases (resumen)

- **Fase 0:** validar sostenibilidad `*7*300` tipo1 → 10002 (test en vivo).
- **Fase 1:** `LiveVideoOnCall` + gate config + suscripción a eventos + `SetCallActive`.
- **Fase 2:** ownership/gestión de go2rtc + doc HA.
- **Fase 3:** pruebas en device + no-regresión + CHANGELOG.
