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

These complement the `binary_sensor`/`sensor` versions: the binary sensors show
current state, the event entities give a clean per-trigger event for automations.

## Call (incoming-call detection)

| Entity | Domain | State topic | Values |
|---|---|---|---|
| Estado llamada | `sensor` | `bticino/call/state` | `INCOMING` / `CONNECTED` / `IDLE` |
| Quién llama | `sensor` | `bticino/call/caller` | SIP user (e.g. `c300x`) |

**Total auto-discovered: ~42 entities** (43 with the doorbell camera below).

---

## Configuration — restart buttons (optional)

Two buttons in HA's **Configuration** section, **off by default**. Enable with
`mqtt.enable_system_buttons: true`:

| Button | Command topic | Action |
|---|---|---|
| Reiniciar bridge | `bticino/system/restart_bridge/set` | Relaunches the `bticino_bridge` process (setsid, ~2 s gap) |
| Reiniciar dispositivo | `bticino/system/reboot/set` | **Reboots the whole intercom** (`/sbin/reboot`) |

> ⚠️ The reboot button restarts the entire door unit. Restarting dropbear (SSH)
> is intentionally **not** offered — the device has no service supervisor for it,
> so a failed restart would drop SSH permanently.

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
