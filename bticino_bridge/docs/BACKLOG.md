# Backlog de features (BTicino bridge)

Ideas y fases futuras, fuera del alcance activo. Cada una necesita su propio
spec → plan antes de implementarse.

## Vídeo/audio (relacionado con el spec de vídeo en vivo)

- **Audio en vivo (escuchar la puerta en HA).** Requiere que el bridge sea **parte
  SIP** de la llamada (auto-answer como endpoint de audio) para que la puerta emita
  Speex RTP; transcodificar Speex→Opus con el ffmpeg del device
  (`/usr/local/bin/ffmpeg`, tiene libspeex+libopus) → go2rtc → HA. Base: cuenta SIP
  standalone (200 OK verificado) + auto-answer de INVITEs existente.
  Ver `_investigacion/docs/SPIKE_LISTEN_HA_2026-07-11.md`.
- **Talk-back (hablar hacia la puerta).** `pkg/sip/talkback.go` + RE hecho; requiere
  que el bridge posea la llamada (colisión SSRC si se inyecta en paralelo a la app).
- **"Llamar a casa".** El bridge origina una llamada SIP al interfono (intercom
  saliente). Su audio NO pasa por bt_av_media (confirmado en el spike).

## Otras

- **Clasificador de eventos de acceso / cerradura por duración.** El bus OpenWebNet
  emite eventos de cerradura (apertura → cierre + duración) de **cualquier piso**
  (sin identificar el piso concreto — privacidad de edificio). Patrón empírico
  observado por el usuario:
  - **~2-3 s** → apertura tras telefonillo (visita a la que le abren).
  - **~4 s** → apertura con **llave RFID** (típicamente residente llegando).

  **Valor:** publicar en HA un evento clasificado ("entrada con llave" vs "entrada por
  telefonillo") para automatizaciones/notificaciones, señales de seguridad (apertura a
  horas raras / con casa vacía) y un timeline "quién entró" correlacionado con nuestras
  llamadas de timbre.

  **Caveats:** señal a **nivel de edificio** (no identifica piso); el umbral 2-3 s vs
  4 s es heurística observada y **hay que validarla con datos reales** (puede variar
  por puerta/persona/estación) antes de fiarla. La base ya existe parcialmente: el
  bridge ya emite eventos `door.unlocked` y una entidad de cerradura en MQTT.
