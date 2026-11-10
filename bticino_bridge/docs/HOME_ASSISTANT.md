# Home Assistant integration

The bridge integrates with Home Assistant over **MQTT with auto-discovery**.
You do **not** need to write any YAML: once the bridge is running and the MQTT
broker is configured, the entities appear automatically under a single device
(**BTicino Classe 300X**).

- **Discovery prefix:** `homeassistant/` (HA standard — do not change)
- **Data prefix:** `bticino` (configurable via `mqtt.topic_prefix`; never set it to
  `homeassistant`, it collides with discovery)
- **Availability:** every entity uses `bticino/bridge/state` (LWT) — entities show
  *unavailable* when the bridge is down.

All topics below assume the default data prefix `bticino`.

Entities are sorted into HA's standard sections via `entity_category`:
**Controls** and **Sensors** (main) hold the things you act on and the
operational readouts; **Diagnostic** holds LEDs, GPIO, network info and the
device-config mirror; **Configuration** holds the optional restart buttons.

---

## Controls (you can act on these)

| Entity | Domain | State topic | Command topic | Notes |
|---|---|---|---|---|
| Puerta (door) | `lock` | `bticino/lock/state` | `bticino/lock/set` | `LOCK`/`UNLOCK`; opens the main door |
| Contestador (answering machine) | `switch` | `bticino/voicemail/state` | `bticino/voicemail/set` | `ON`/`OFF` |
| Sonido Timbre (doorbell sound) | `switch` | `bticino/doorbellsound/state` | `bticino/doorbellsound/set` | Ringer mute/unmute |
| Pantalla (display) | `switch` | `bticino/display/state` | `bticino/display/set` | Screen on/off |
| Luz Escalera (staircase light) | `button` | — | `bticino/light/set` | Momentary (press+release) |
| Abrir Puerta (door open) | `button` | — | `bticino/door/open` | Momentary door pulse |
| Marcar todos leídos | `button` | — | `bticino/messages/markallread/set` | Marks every message read |
| Additional locks | `lock` | `bticino/lock/<id>/state` | `bticino/lock/<id>/set` | Only if configured (secondary gates) |

## Sensors (read-only)

| Entity | Domain | State topic | Notes |
|---|---|---|---|
| Temperatura | `sensor` | `bticino/sensor/temperature` | °C, device internal |
| Mensajes Nuevos | `sensor` | `bticino/sensor/new_messages` | Unread answering-machine messages |
| Total Mensajes | `sensor` | `bticino/sensor/total_messages` | |
| Almacenamiento | `sensor` | `bticino/sensor/storage_used` | % used |
| Teclado | `sensor` | `bticino/sensor/keypad` | Last physical button pressed |
| Evento Bus OpenWebNet | `sensor` | `bticino/sensor/bus_event` | Last raw OWN frame seen |
| Diagnóstico Sistema | `sensor` | `bticino/sensor/system_diag` | |
| Multicast | `sensor` | `bticino/sensor/multicast_count` | Multicast messages seen |
| Registro de Actividad | `sensor` | `bticino/sensor/activity_log` | + `.../attributes` for history |
| Información del Sistema | `sensor` | `bticino/sensor/system_info` | WHO=13 (IP/MAC/firmware…) in attributes |

### Diagnostic — network (`entity_category: diagnostic`)

| Entity | State topic | Notes |
|---|---|---|
| Dirección IP | `bticino/sensor/system_info/attributes` | via `value_json.ip_address` |
| Dirección MAC | `bticino/sensor/system_info/attributes` | via `value_json.mac_address` |
| Señal WiFi (%) | `bticino/sensor/wifi_signal` | from `connmanctl` Strength, refreshed every 60 s |

### Device configuration (read-only mirror of the native settings)

| Entity | State topic |
|---|---|
| Idioma (language) | `bticino/system/language` |
| Zona horaria (timezone) | `bticino/system/timezone` |
| Servidor NTP | `bticino/system/timezone` *(currently shares the timezone topic)* |
| Fecha/hora | `bticino/system/datetime` |
| Estado contestador | `bticino/answering/state` |
| Memoria contestador | `bticino/answering/state` |
| Tono S0 (entrance panel) | `bticino/audio/ringtone/s0` |
| Volumen S0 | `bticino/audio/volume/s0` |
| Volumen puerta | `bticino/audio/volume/door` |
| Brillo pantalla | `bticino/display/brightness` |
| Brillo cámara 20 | `bticino/camera/20/config` |

## Binary sensors

| Entity | Domain | State topic | Notes |
|---|---|---|---|
| Timbre (doorbell) | `binary_sensor` | `bticino/doorbell/state` | Entrance-panel ring |
| Timbre rellano (floor) | `binary_sensor` | `bticino/doorbell_floor/state` | Landing/floor button (`*7*59#`), `device_class: occupancy` |
| Estado SIP | `binary_sensor` | `bticino/sensor/sip_status` | SIP registration up/down |
| Contestador sin ruta | `binary_sensor` | `bticino/problem/answerer_route/state` | `device_class: problem`. ON when the bridge's SIP user is missing from the route file Flexisip actually uses (checked every 60 s; attribute `file`). Only with `sip.answerer.enabled` |
| Flexisip (llamadas y nube) | `binary_sensor` | `bticino/problem/flexisip/state` | `device_class: problem`. ON when the local Flexisip is not running or not listening on its SIP port. Attributes: `running`, `sip_port_listening`, `tls_port_listening`, `passphrase_file_arg` |
| LEDs (×7) | `binary_sensor` | `bticino/led/<name>/state` | Missed call, call green/red, etc. |
| GPIO (×13) | `binary_sensor` | `bticino/gpio/<pin>/state` | Raw GPIO pins |

## Events (`event` platform)

Momentary **event entities** — ideal for automations ("when the doorbell rings…").
Each fires with a timestamp and an `event_type`; the payload is JSON on the topic.

| Entity | Topic | event_types |
|---|---|---|
| Timbre (evento) | `bticino/event/doorbell` | `pressed` |
| Timbre rellano (evento) | `bticino/event/doorbell_floor` | `pressed` |
| Llamada (evento) | `bticino/event/call` | `incoming`, `connected`, `ended` (incoming carries `caller`) |
| Teclas (evento) | `bticino/event/keys` | `press` (fired on release), `long_press` (key held 0.8 s). Attribute `key`: `llave`, `estrella`, `ojo`, `telefono` |

The key event only observes the four physical keys; it does not change what the
unit itself does with them. Example trigger for a long press on the star key:

```yaml
trigger:
  - platform: state
    entity_id: event.bticino_evento_teclas
condition:
  - "{{ trigger.to_state.attributes.event_type == 'long_press' }}"
  - "{{ trigger.to_state.attributes.key == 'estrella' }}"
```

These complement the `binary_sensor`/`sensor` versions: the binary sensors show
current state, the event entities give a clean per-trigger event for automations.

## Call (incoming-call detection)

| Entity | Domain | State topic | Values |
|---|---|---|---|
| Estado llamada | `sensor` | `bticino/call/state` | `INCOMING` / `CONNECTED` / `IDLE` |
| Quién llama | `sensor` | `bticino/call/caller` | SIP user (e.g. `c300x`) |

**Total auto-discovered: about 45 entities** (one more with the doorbell camera below).

---

## Configuration — system buttons (optional)

Buttons in HA's **Configuration** section, **off by default**. Enable with
`mqtt.enable_system_buttons: true`:

| Button | Command topic | Action |
|---|---|---|
| Reiniciar bridge | `bticino/system/restart_bridge/set` | Relaunches the `bticino_bridge` process (setsid, ~2 s gap) |
| Reiniciar dispositivo | `bticino/system/reboot/set` | **Reboots the whole intercom** (`/sbin/reboot`) |
| Arrancar SSH | `bticino/system/start_ssh/set` | Runs `/etc/init.d/dropbear start` (20 s limit, no shell) |

> ⚠️ The reboot button restarts the entire door unit.
>
> "Arrancar SSH" only *starts* the SSH server: the unit's init script uses
> `start-stop-daemon -S`, which does nothing when dropbear is already running.
> A *restart* is intentionally not offered — the device has no service
> supervisor, so a failed restart would drop SSH permanently.

### SSH watchdog (optional)

On some units the native daemon stops the SSH server some time after boot. The
bridge can bring it back by itself. Off by default:

```yaml
health:
  ssh_watchdog:
    enabled: true
    port: 22
    grace: 5m          # how long SSH must be down before it is started
    start_command: "/etc/init.d/dropbear start"
```

It checks the listening ports once a minute (read-only, through `/proc`),
starts SSH after `grace` of continuous downtime and waits 10 minutes between
attempts. If the state cannot be read it does nothing.

## Doorbell camera

A **camera** entity (`camera.camara_timbre`, image topic `bticino/camera/doorbell`,
base64 JPEG) that shows *who rang the bell*. When a doorbell rings or a call comes
in — the moments when the native video session is active — the bridge captures a
cooperative snapshot (single `*7*300`, validated at 688×480) and publishes it.

This entity appears **only when `streaming.video_on_demand: true`**, because it
needs the video-activation path. It is **off by default** for safety:

```yaml
streaming:
  video_on_demand: true    # enables /api/snapshot + the doorbell camera
  video_backend: "avmedia" # cooperative *7*300 (recommended)
```

> ⚠️ Enabling video competes with the native firmware for the camera; only turn
> it on after validating on your unit. See the safety note in the main `README.md`.
> With it **off** (default), the camera entity simply isn't created; call/floor
> sensors and everything else still work.

Related REST endpoints (diagnostics, always available):

- `POST /api/video/probe?confirm=yes` — single `*7*300`, returns a JPEG if a
  native session is active
- `POST /api/audio/probe?confirm=yes` — measures the Speex audio RTP flow
- `GET /api/snapshot` — JPEG (only wired when `video_on_demand: true`)

---

## Live video during a call (`live_on_call`)

When a call comes in **to your unit**, the bridge passively mirrors the native
video leg (a read-only packet tap on `lo:5007`, where the firmware already streams
cleartext H.264 RTP for the whole call) into the RTSP relay, which **go2rtc**
publishes as a live WebRTC/HLS stream. HA can then show **live video** (motion) —
not just the still snapshot — for the duration of the call.

- **Enabled by default** (`streaming.live_on_call: true`). It sends **zero commands
  to the device bus** (safer than the cooperative snapshot) and is **independent of
  `video_on_demand`** (no self-INVITE, the native call session already exists).
- go2rtc serves the stream **`doorbell-video`** (video only). Point HA at it via the
  go2rtc / `webrtc-camera` integration (`http://<device-ip>:1984`), or add a generic
  camera to `rtsp://<device-ip>:6554/doorbell-video`.
- Outside a call there is no live video (that's normal); `camera.camara_timbre` keeps
  showing the last snapshot as a preview.

```yaml
streaming:
  live_on_call: true  # live door video during an incoming call (default)
```

> **Building privacy:** live video only works for calls to *your* unit. If the door
> entry system is busy with a call to another flat, no video is available — this is
> **by design** (a neighbour can't watch your camera), not a fault.

> **Audio is not included.** The door only emits audio as RTP when a SIP party is in
> the call, so *listening* in HA needs the bridge to join the call as a SIP endpoint —
> a future phase (see `docs/BACKLOG.md`).

---

## Example Lovelace card

```yaml
type: vertical-stack
cards:
  - type: picture-entity          # only if video_on_demand: true
    entity: camera.camara_timbre
    name: Cámara timbre
    camera_view: auto
  - type: entities
    title: Videoportero BTicino
    entities:
      - entity: lock.bticino_puerta
      - entity: binary_sensor.timbre
      - entity: binary_sensor.timbre_rellano
      - entity: sensor.estado_llamada
      - entity: sensor.quien_llama
      - entity: switch.contestador
      - entity: sensor.mensajes_nuevos
      - entity: sensor.temperatura
  - type: button
    name: Abrir puerta
    tap_action:
      action: call-service
      service: button.press
      target:
        entity_id: button.abrir_puerta
```

## Automations

Ready-made examples live in [`configs/homeassistant/automations.yaml`](../configs/homeassistant/automations.yaml)
(notify on ring, open door on button, etc.). A manual discovery/entity reference
is in [`configs/homeassistant/discovery.yaml`](../configs/homeassistant/discovery.yaml).
