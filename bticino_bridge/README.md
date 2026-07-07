# BTicino Classe 300X Enhanced Bridge

Go bridge for the BTicino Classe 300X video door entry unit. It runs **on the device
itself** (ARM7, i.MX6) and integrates the intercom with Home Assistant (MQTT),
Apple HomeKit, a Svelte web dashboard, a REST API and hardware-accelerated
**RTSP video streaming**.

**Version:** 0.15.5 (see `VERSION` and `CHANGELOG.md`)
**Target:** Linux ARM7 (i.MX6 inside the Classe 300X)

## Current status

| Component | Status |
|---|---|
| **RTSP video (:6554)** | ✅ Working — direct GStreamer (i.MX VPU) + RTP relay. Plays in VLC/ffplay |
| Svelte web dashboard (:8082) | ✅ Working — 6 pages (dashboard, controls, messages, memos, logs, settings) |
| REST API (40+ endpoints) | ✅ Working — documented with Swagger UI at `/api/docs/` |
| Real-time SSE (`/api/events`) | ✅ Working — live LED/GPIO updates, no polling |
| OpenWebNet client (80+ cmds) | ✅ Working — monitor on port 20000 + commands via netcat to 30006 |
| MQTT Home Assistant (39+ entities) | ✅ Working — auto-discovery + object_id |
| Device config sync (conf.xml, aswm, tvcc) | ✅ Working — QML→Bridge→MQTT (languages, ringtones, volumes, display) |
| Answering machine: messages + memos | ✅ Working — mark read/unread, delete, download video |
| Physical button monitoring | ✅ Working — event0 keyboard, event1 touch, event2 GPIO |
| Physical monitoring (temp, LEDs, GPIO) | ✅ Working — 7 LEDs, 13 GPIO pins |
| Web log viewer (/logs) | ✅ Working — 500-entry ring buffer |
| Multicast listener (:7667) | ⚠️ Implemented (port held by a native process) |
| HomeKit (:8081) | ⚠️ Implemented, not verified |

**v0.15.5** — main milestone: a complete RTSP video pipeline **without relying on
`bt_av_media`** (which has an unfixable routing bug in `libjel.so`):

```
RTSP client (VLC/ffplay) ──RTSP──> bticino-bridge:6554
                                        │
                                   SIP self-INVITE (webrtc→c300x via local Flexisip)
                                        │
                                   GStreamer pipelines:
                                     imxv4l2videosrc → imxvpuenc_h264 → rtph264pay → udpsink :10002
                                     alsasrc → speexenc → rtpspeexpay → udpsink :10000
                                        │
                                   RTP relay (fan-out to all RTSP clients)
```

Architecture details and decisions: `CHANGELOG.md` (the authoritative running log).

---

## Architecture

```
BTicino Classe 300X
 Native processes: bt_vct, openserver, bt_av_media, flexisip (NOT touched)
 OpenWebNet ports: 20000 (monitor), 30006 (commands), 30007 (video)

 bticino-bridge (ARM binary)
  ├── OpenWebNet Client ──── Monitors port 20000 (read-only)
  │                          Commands via netcat to 30006 (non-interfering mode)
  ├── Web Server (:8082) ─── Embedded Svelte SPA + REST API + Swagger UI + SSE
  ├── MQTT Bridge ────────── Paho MQTT → HA broker (39+ auto-discovery entities)
  ├── HomeKit Bridge ─────── brutella/hap on :8081 (lock, doorbell, camera)
  ├── SIP/RTSP ───────────── SIP self-INVITE → GStreamer (VPU) → RTP relay → RTSP :6554
  ├── Device Config ──────── Reads/watches conf.xml, aswm, tvcc → republishes to MQTT
  ├── Event Bus ──────────── Internal pub/sub between components
  ├── Input Monitor ──────── /dev/input/event0-2 (buttons, touch, GPIO)
  ├── Message Parser ─────── Answering machine messages + voice/text memos
  └── Multicast Listener ─── UDP 239.255.76.67:7667 (BTicino syslog)
```

### Non-interfering mode (critical)

The bridge shares the device with the native BTicino processes. To avoid fighting
them for ports/hardware it only **monitors** on port 20000 and sends commands by
shelling out to netcat (`echo '<frame>' | nc 0 30006`) instead of holding its own
sockets. Do not open persistent sockets to 30006/30007. BTicino requires ~310 ms
of spacing between commands.

## Go packages

| Package | Lines | Description |
|---|---|---|
| `cmd/` | ~1330 | Entry point. Wires subsystems based on config + flags |
| `pkg/webserver/` | ~6580 | Web server, REST API, config/device/streaming handlers |
| `pkg/sip/` | ~4820 | Video: SIP client, GStreamer, RTP relay, RTSP server |
| `pkg/openwebnet/` | ~2410 | OpenWebNet client: monitor, HMAC auth, 4-level safety manager |
| `pkg/deviceconfig/` | ~1750 | Read/watch conf.xml, aswm, tvcc → MQTT |
| `pkg/mqtt/` | ~1710 | MQTT bridge: Paho, HA discovery, command topics, LWT |
| `pkg/input/` | ~975 | Hardware monitor: buttons, touchscreen, GPIO sysfs |
| `pkg/messageparser/` | ~750 | Answering machine parser: msg_info.ini, voice/text memos |
| `pkg/bticino/` | ~700 | Constants: filesystem paths, ports, OWN commands |
| `pkg/multicast/` | ~610 | UDP multicast listener + OpenWebNet handler |
| `pkg/homekit/` | ~550 | HomeKit bridge: lock, doorbell, camera |
| `pkg/events/` | ~490 | Pub/sub event bus with pattern matching (+ tests) |
| `pkg/config/` | ~420 | YAML config with defaults |
| `pkg/bticino_commands/` | ~370 | High-level commands: multi-step sequences |
| `pkg/udpproxy/` | ~150 | Auxiliary UDP proxy |
| `pkg/version/` | ~90 | Version read from `VERSION` + build-time injection |

## Dependencies

| Module | Purpose |
|---|---|
| `github.com/brutella/hap` | HomeKit Accessory Protocol |
| `github.com/eclipse/paho.mqtt.golang` | MQTT client |
| `github.com/sirupsen/logrus` | Structured logging |
| `github.com/swaggo/swag` | Swagger documentation generation |
| `golang.org/x/net` | Multicast/ipv4 |
| `gopkg.in/yaml.v2` | YAML parsing |

Frontend: Svelte 4 + Vite (under `web/`, needs Node.js to build).

## Build and deploy

```bash
make build        # Svelte frontend (web/dist) + ARM Go binary
make build-go     # binary only: GOOS=linux GOARCH=arm GOARM=7
make build-web    # frontend only (npm install + vite build)
make dev          # local dev: vite :5173 + go run ./cmd/main.go
make deploy       # build + scripts/deploy.sh (scp to device, restart)
make test         # scripts/run_all_tests.sh --all (needs the device)
make clean
```

The binary is installed at `/home/bticino/cfg/extra/` on the device.
Alternative from the repo root: `../deploy-standard.sh full` (base64 streaming over SSH).

### Runtime flags (`cmd/main.go`)

`-config` (default `configs/config.yaml`), `-log-level`, `-version`,
`-test` (no device connection), `-web-port`, and the toggles
`-enable-openwebnet` / `-enable-web` / `-enable-homekit` / `-enable-video`.

## Configuration

Single file: `configs/config.yaml` (other files in `configs/` are historical examples).

```yaml
bridge:
  name: "BTicino Bridge Enhanced"
  log_level: "info"

openwebnet:
  host: "127.0.0.1"
  port: 30006

sip:
  enabled: true
  server_host: "127.0.0.1"   # local Flexisip
  transport: "tcp"
  username: "webrtc"          # self-INVITE: webrtc@domain → c300x@domain
  sip_target: "c300x"

mqtt:
  enabled: true
  host: "192.168.1.3"         # broker IP (Home Assistant)
  port: 1883
  username: "mqtt_user"
  password: "CHANGE_ME"
  topic_prefix: "bticino"     # do NOT use "homeassistant"

web:
  enabled: true
  port: 8082

homekit:
  enabled: true
  port: "8081"
  pin: "12345678"
  storage_path: "./homekit_data"
```

## REST API

40+ endpoints documented in **Swagger UI**: `http://<device-ip>:8082/api/docs/`

Summary by group:

```bash
# Status / system
GET  /api/status              GET /api/system
GET  /api/events              # SSE: real-time LEDs/GPIO

# Answering machine messages and memos
GET  /api/messages            GET /api/messages/{id}
GET  /api/messages/download/{id}/{type}
POST /api/messages/mark-read/{id}
DELETE /api/messages/delete/{id}
GET  /api/memos               GET /api/memos/{id}

# Controls
POST /api/controls/door/unlock
POST /api/controls/display/on|off
POST /api/controls/mute/on|off
POST /api/controls/doorbell/on|off
POST /api/controls/answering-machine/toggle
POST /api/controls/light/on
POST /api/controls/command    # arbitrary OpenWebNet: {"command": "*8*19*20##"}

# Streaming
GET  /api/streaming           POST /api/streaming/start|stop
GET  /api/streaming/sessions|config
POST /api/streaming/record

# Configuration (bridge and native device)
GET  /api/config              POST /api/config/save|validate|backup|restore|reload
GET  /api/config/device|language|timezone|ntp|ringtones|volumes|display|cameras|answering
POST /api/device/save

# Logs
GET  /api/logs?level=info&count=200
GET  /api/logs/download
```

## RTSP video

```bash
# From any machine on the LAN:
ffplay rtsp://<device-ip>:6554/doorbell
vlc rtsp://<device-ip>:6554/doorbell
```

- Hardware H.264 (i.MX VPU), 720x576 PAL, ~7 fps, ~1500 kbps
- Supports UDP unicast and TCP interleaved, multiple simultaneous clients (fan-out)
- See `docs/WEBRTC_RTSP_STREAMING.md` and the v0.15.5 entry in `CHANGELOG.md`

## Tests

```bash
go test ./...                          # unit tests
make test                              # integration tests (needs the device)
```

Unit coverage is low: only `pkg/events/` and `pkg/multicast/` have tests. The rest
is validated with the `scripts/run_all_tests.sh` integration tests against the
real device.

## File layout

```
bticino_bridge/
├── cmd/main.go                        # Entry point (~1330 lines)
├── pkg/
│   ├── bticino/                       # Device constants
│   ├── bticino_commands/              # High-level commands
│   ├── config/                        # YAML config loader
│   ├── deviceconfig/                  # conf.xml/aswm/tvcc → MQTT (7 files)
│   ├── events/                        # Pub/sub event bus (+ tests)
│   ├── homekit/                       # HomeKit bridge (lock, doorbell, camera)
│   ├── input/                         # Buttons, touchscreen, GPIO
│   ├── messageparser/                 # Answering machine + memos
│   ├── mqtt/                          # MQTT bridge (Paho)
│   ├── multicast/                     # UDP multicast listener (+ tests)
│   ├── openwebnet/                    # OWN client: auth, commands, safety
│   ├── sip/                           # SIP + GStreamer + RTP relay + RTSP
│   ├── udpproxy/                      # UDP proxy
│   ├── version/                       # Version management
│   └── webserver/                     # REST API + handlers + Swagger
├── web/                               # Svelte 4 + Vite frontend
│   └── src/routes/                    # dashboard, controls, messages, memos, logs, settings
├── configs/config.yaml                # Single configuration file
├── deployment/                        # systemd unit + scripts
├── docs/                              # Guides (streaming, HA, HomeKit, OWN commands...)
├── scripts/                           # deploy.sh, run_all_tests.sh, MQTT utilities
├── Makefile
├── CHANGELOG.md                       # Authoritative change log
└── VERSION                            # 0.15.5
```
