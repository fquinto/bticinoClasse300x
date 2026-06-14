# BTicino Bridge - Features and Architecture

**Version**: 0.15.5
**Language**: Go 1.24 + Svelte 4
**Target**: Linux ARM7 (NXP i.MX6ULL, BTicino Classe 300X)

---

## What is BTicino Bridge?

BTicino Bridge is a lightweight, self-contained Go application that runs directly
on the BTicino Classe 300X video intercom. It acts as a protocol translator
between the device's proprietary OpenWebNet (OWN) bus and modern smart home
ecosystems.

The bridge coexists with the device's native processes (`bt_vct`, `openserver`,
`btvideophone`, `bt_av_media`) without modifying or interfering with them. It
reads events from the OWN bus in monitor mode and sends commands through
non-persistent netcat connections, ensuring the original firmware continues
working normally.

A single ~15MB ARM binary provides:

- **RTSP/WebRTC video streaming** with hardware-accelerated H.264 encoding
- **Apple HomeKit** integration (doorbell, lock, camera)
- **Home Assistant** integration via MQTT with auto-discovery of 39+ entities
- **REST API** with 45+ endpoints and Swagger documentation
- **Web dashboard** (Svelte SPA) for real-time monitoring and control
- **Hardware monitoring** of physical buttons, GPIO pins, LEDs, and temperature
- **Answering machine** management (messages and memos)
- **Device configuration** management via web or MQTT

All of this runs on the intercom's constrained ARM Cortex-A7 SoC with 512MB RAM.

---

## Architecture

```
BTicino Classe 300X (192.168.1.38)
 Native processes: bt_vct, openserver, btvideophone, bt_av_media (untouched)
 OpenWebNet ports: 20000 (events), 30006 (commands), 30007 (video control)
 SIP proxy: Flexisip on 127.0.0.1:5060

 bticino_bridge (single ARM binary, ~15MB)
  +----------------------------------------------------------------------+
  |                                                                      |
  |  OpenWebNet Client                                                   |
  |    - EVENT session on port 20000 (read-only monitoring)              |
  |    - Commands via netcat to port 30006 (non-persistent)              |
  |    - HMAC-SHA256 authentication                                      |
  |    - 158+ command definitions across 15 WHO systems                  |
  |    - 4-level safety system with audit logging                        |
  |                                                                      |
  |  SIP/Video Pipeline                                                  |
  |    - Dual-role SIP client (webrtc + c300x identities)                |
  |    - Self-INVITE via local Flexisip proxy                            |
  |    - GStreamer: imxv4l2videosrc -> imxvpuenc_h264 -> rtph264pay      |
  |    - RTP relay with UDP fan-out to multiple clients                  |
  |    - Enhanced RTSP server on port 6554                               |
  |                                                                      |
  |  MQTT Bridge                                                         |
  |    - Paho MQTT client with auto-reconnect and LWT                    |
  |    - 39+ Home Assistant auto-discovery entities                      |
  |    - Command topics for remote control                               |
  |    - Device config publishing with file change detection             |
  |                                                                      |
  |  HomeKit Bridge                                                      |
  |    - HAP server with persistent pairing                              |
  |    - Accessories: doorbell, lock, camera                             |
  |                                                                      |
  |  Web Server                                                          |
  |    - Embedded Svelte SPA (Dashboard, Controls, Messages, etc.)       |
  |    - 45+ REST API endpoints                                          |
  |    - Server-Sent Events for real-time updates                        |
  |    - Swagger UI at /api/docs/                                        |
  |    - Config management with backup/restore/hot-reload                |
  |                                                                      |
  |  Hardware Monitors                                                   |
  |    - /dev/input/event0 (keypad), event1 (touchscreen), event2 (GPIO) |
  |    - 13 GPIO pins via sysfs                                          |
  |    - 9 LED states                                                    |
  |    - CPU temperature                                                 |
  |    - Multicast listener on 239.255.76.67:7667                        |
  |                                                                      |
  |  Event Bus                                                           |
  |    - Async pub/sub with wildcard pattern matching (e.g. door.*)      |
  |    - Worker pool with 1000-event buffer                              |
  |    - Connects all subsystems together                                |
  |                                                                      |
  +----------------------------------------------------------------------+
```

### Data Flow

```
Physical Events                     Smart Home Platforms
  Door bell press  ----+
  Door lock/unlock ----+---> OWN Bus ---> Event Bus --+--> MQTT --> Home Assistant
  Button press     ----+         |                    +--> HomeKit --> Apple Home
  GPIO change      ----+         |                    +--> SSE --> Web Dashboard
                                 |                    +--> REST API --> Custom apps
                                 v
Remote Commands                 OpenWebNet
  MQTT set topic  -----+         |
  HomeKit control -----+---> Command ---> netcat:30006 ---> Device
  REST API POST   -----+     Builder
  Web UI button   -----+
```

---

## Features

### 1. Video Streaming

Live camera access from the intercom's built-in camera via RTSP.

**Pipeline**: The bridge bypasses the device's native `bt_av_media` process
(which has an unusable MQTT interface due to a routing bug in `libjel.so`) and
drives GStreamer directly:

```
Camera (/dev/video0)
  720x576 UYVY interlaced PAL, ~7fps
    |
    v
GStreamer Pipeline (hardware-accelerated)
  imxv4l2videosrc
    -> imxvpuenc_h264 (VPU H.264 Constrained Baseline L3.0)
       bitrate=1500, gop-size=7, idr-interval=1
    -> rtph264pay (PT=96, config-interval=1)
    -> udpsink (127.0.0.1:10002)
    |
    v
RTP Relay (fan-out)
    -> RTSP Server (:6554) -> VLC, ffplay, go2rtc
    -> UDP unicast to each connected client
```

Audio pipeline runs in parallel (non-fatal if it fails):
```
alsasrc (hw:0, S16LE 8kHz mono) -> speexenc -> rtpspeexpay -> udpsink :10000
```

**How to use**:
```bash
# VLC
vlc rtsp://192.168.1.38:6554/doorbell

# ffplay
ffplay -rtsp_transport udp rtsp://192.168.1.38:6554/doorbell

# go2rtc (for Home Assistant camera integration)
# Add to go2rtc.yaml:
#   streams:
#     doorbell:
#       - rtsp://192.168.1.38:6554/doorbell
```

**Implementation**: `pkg/sip/gstreamer.go`, `pkg/sip/rtp_relay.go`,
`pkg/sip/rtsp_server_enhanced.go`, `pkg/sip/client.go`, `pkg/sip/video.go`

---

### 2. Home Assistant Integration (MQTT)

Full auto-discovery integration with Home Assistant via MQTT. The bridge
publishes discovery payloads on startup so all entities appear automatically.

**39+ entities** across these types:

| Type | Entities | Examples |
|------|----------|---------|
| Lock | 1+ | Main door lock (+ dynamic per-WHERE locks) |
| Switch | 4 | Voicemail, doorbell sound, display, mute |
| Binary Sensor | 1 | Doorbell ringing |
| Sensor | 11 | Temperature, new/total messages, storage, keypad, bus event, SIP status, diagnostics, multicast count, activity log, system info |
| Button | 2 | Staircase light, door open |
| Select | 3 | Language, timezone, ringtone |
| Number | 3 | Intercom volume, door volume, display brightness |
| Device Trigger | 1 | Doorbell press |

**Command topics** (subscribe):

| Topic | Payload | Action |
|-------|---------|--------|
| `bticino/lock/set` | `LOCK` / `UNLOCK` | Lock or unlock the door |
| `bticino/voicemail/set` | `ON` / `OFF` | Toggle answering machine |
| `bticino/display/set` | `ON` / `OFF` | Turn display on or off |
| `bticino/mute/set` | `ON` / `OFF` | Mute or unmute |
| `bticino/doorbellsound/set` | `ON` / `OFF` | Enable or disable doorbell sound |
| `bticino/light/set` | `ON` | Turn on staircase light |
| `bticino/command/send` | OWN string | Send raw OpenWebNet command |

**State topics** (publish): All entity states are published with QoS 1 and
retained. LWT (`bticino/bridge/state`) ensures Home Assistant knows when the
bridge goes offline.

**Implementation**: `pkg/mqtt/bridge.go` (~1600 lines)

---

### 3. Apple HomeKit Integration

Native HomeKit support via the HAP (HomeKit Accessory Protocol). The bridge
advertises as a HomeKit bridge with three accessories.

| Accessory | Service | Capabilities |
|-----------|---------|-------------|
| Doorbell | `service.Doorbell` | Ring notifications via `ProgrammableSwitchEvent` |
| Lock | `service.LockMechanism` | Lock/unlock door, current state feedback |
| Camera | `service.CameraRTPStreamManagement` | Stream URL, status |

**Pairing**: Use the configured PIN (default `001-02-003`) in the Apple Home app.
Pairing data persists in `/home/bticino/cfg/extra/47/homekit/`.

**Implementation**: `pkg/homekit/bridge.go`, `pkg/homekit/doorbell.go`,
`pkg/homekit/lock.go`, `pkg/homekit/camera.go`

---

### 4. Web Dashboard

A Svelte 4 single-page application served directly by the bridge. Uses
hash-based routing and Server-Sent Events for real-time updates.

| Page | Path | Description |
|------|------|-------------|
| Dashboard | `#dashboard` | System status, connection states, uptime, version, GPIO/LED states |
| Controls | `#controls` | Door unlock, display, mute, doorbell, light, answering machine, raw OWN command |
| Messages | `#messages` | Answering machine messages: list, play, download, mark read, delete |
| Memos | `#memos` | Voice and text memos: list, play, download |
| Settings | `#settings` | YAML config editor, device settings, backup/restore, streaming config |
| Logs | `#logs` | Live bridge log viewer |

**Real-time updates**: The dashboard connects to `/api/events` (SSE) for live
GPIO, LED, and state changes without polling.

**Implementation**: `web/src/` (Svelte), `pkg/webserver/server.go` (HTTP server)

---

### 5. REST API

45+ endpoints organized by function. Full OpenAPI documentation available at
`/api/docs/` (Swagger UI).

#### System

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/status` | Bridge status (uptime, connections, versions) |
| GET | `/api/system` | System information |
| GET | `/api/logs` | Recent log entries (filterable) |
| GET | `/api/logs/download` | Download full log file |
| GET | `/api/events` | Server-Sent Events stream |

#### Controls

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/controls/door/unlock` | Unlock door (1s timed sequence) |
| POST | `/api/controls/display/on` | Turn display on |
| POST | `/api/controls/display/off` | Turn display off |
| POST | `/api/controls/mute/on` | Mute intercom |
| POST | `/api/controls/mute/off` | Unmute intercom |
| POST | `/api/controls/doorbell/on` | Enable doorbell |
| POST | `/api/controls/doorbell/off` | Disable doorbell |
| POST | `/api/controls/light/on` | Staircase light (200ms timing) |
| POST | `/api/controls/answering-machine/toggle` | Toggle answering machine |
| POST | `/api/controls/command` | Send raw OWN command |

#### Messages and Memos

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/messages` | All messages summary |
| GET | `/api/messages/list` | Paginated message list |
| GET | `/api/messages/{id}` | Single message details |
| GET | `/api/messages/download/{id}/{type}` | Download message audio/image |
| POST | `/api/messages/mark-read/{id}` | Mark message as read |
| DELETE | `/api/messages/delete/{id}` | Delete a message |
| GET | `/api/memos` | All memos |
| GET | `/api/memos/{id}` | Single memo details |
| GET | `/api/memos/audio/{id}` | Download memo audio |
| POST | `/api/memos/mark-read/{id}` | Mark memo as read |
| DELETE | `/api/memos/delete/{id}` | Delete a memo |

#### Video Streaming

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/streaming` | Current streaming status |
| GET | `/api/streaming/sessions` | Active streaming sessions |
| GET | `/api/streaming/config` | Streaming configuration |
| POST | `/api/streaming/start` | Start video stream |
| POST | `/api/streaming/stop` | Stop video stream |
| POST | `/api/streaming/record` | Toggle recording |
| POST | `/api/webrtc/start` | Start WebRTC session |
| POST | `/api/webrtc/stop` | Stop WebRTC session |
| GET | `/api/webrtc/status` | WebRTC status |

#### Configuration

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/config` | Current YAML configuration |
| POST | `/api/config/save` | Save configuration changes |
| POST | `/api/config/validate` | Validate without saving |
| POST | `/api/config/backup` | Create config backup |
| POST | `/api/config/restore` | Restore from backup |
| POST | `/api/config/reload` | Hot-reload configuration |
| GET | `/api/config/backups` | List available backups |
| GET | `/api/config/history` | Change history (last 100) |
| GET | `/api/config/device` | Device config from system files |
| GET | `/api/config/sip-accounts` | SIP accounts from Flexisip |

#### Device Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/device/ntp` | NTP configuration |
| GET | `/api/device/timezone` | Device timezone |
| GET | `/api/device/language` | Device language |
| GET | `/api/device/ringtone` | Current ringtone |
| GET | `/api/device/ringtones` | Available ringtones |
| GET | `/api/device/languages` | Available languages |
| POST | `/api/device/save` | Save device settings |

---

### 6. OpenWebNet Protocol

The bridge implements the BTicino OpenWebNet protocol for bus communication.

**Connection modes**:
- **EVENT session** (port 20000): Read-only monitoring of all bus traffic
- **COMMAND session** (port 30006): Send commands via fresh netcat TCP connections
- **VIDEO control** (port 30007): Video stream start/stop commands

**Authentication**: HMAC-SHA256 with BCD digit-to-hex encoding and
nonce-challenge handshake.

**Command database**: 158+ command definitions across 15 WHO systems:

| WHO | System | Command Count |
|-----|--------|---------------|
| 1 | Lighting | ON, OFF, dimmer, timed |
| 2 | Automation | UP, DOWN, STOP (shutters) |
| 4 | Temperature | Set temp, mode, read |
| 5 | Alarm | Arm, disarm, zone status |
| 6 | Door Entry | Lock, unlock, call, open |
| 7 | Multimedia/Video | Stream start/stop |
| 8 | Audio/Video Door Entry | Door station management |
| 9 | Auxiliary | AUX on/off/status |
| 13 | Gateway | Date, time, IP, firmware, MAC |
| 15 | CEN/CEN+ | Scenario commands |
| 17 | Energy | Counters, consumption |
| 130 | Diagnostics | Diagnostic queries |
| 1013 | Door Lock | Lock/unlock specific doors |

**Safety system** (4 levels):
- **Low**: All commands allowed, audit logging only
- **Medium**: Blocks user enumeration, rate limiting
- **High**: Blocks door control and video, read-only mode
- **Critical**: Only status requests allowed

**Command format**: `*WHO*WHAT*WHERE##` (command), `*#WHO*WHERE##` (status request)

**Implementation**: `pkg/openwebnet/client.go`, `pkg/openwebnet/command.go`,
`pkg/openwebnet/safety.go`, `pkg/openwebnet/auth.go`

---

### 7. Hardware Monitoring

The bridge reads physical hardware events from the intercom device.

**Input devices**:
- `/dev/input/event0` - Keypad (physical buttons)
- `/dev/input/event1` - Touchscreen
- `/dev/input/event2` - GPIO keys

**GPIO monitoring** (13 pins via sysfs):
Pins 12, 13, 47, 49, 52, 54, 56, 58, 60, 154, 155, 176, 180. States published
to MQTT every 30 seconds and via SSE on change.

**LED monitoring** (9 LEDs):
`led_ans_machine`, `led_exc_call`, `led_gwifi`, `led_lock`, `led_memo`,
`led_vct_green`, `led_vct_red`, `mmc0::`, `mmc2::`. States published to MQTT
and SSE.

**Temperature**: Read from the device's thermal zone, published as an MQTT sensor.

**Implementation**: `pkg/input/monitor.go`, `pkg/input/gpio.go`,
`pkg/input/events.go`

---

### 8. Answering Machine Management

Full access to the intercom's built-in answering machine messages and memos.

**Messages**: Stored in `/home/bticino/cfg/extra/47/messages/`. Each message
includes metadata (`msg_info.ini`), audio recording, and a snapshot image.
The bridge parses these files and exposes them via REST API and MQTT
(new/total message count, storage usage percentage).

**Memos**: Stored in `memos_text/` and `memos_voice/` directories. Text memos
contain `message.txt`, voice memos contain `audio.wav`.

**Operations**: List, view details, play audio, download files, mark as
read/unread, delete. All available through the web dashboard and REST API.

**Implementation**: `pkg/messageparser/messageparser.go`

---

### 9. Device Configuration Management

The bridge reads and monitors the device's native configuration files.

**Config sources**:
- `/var/tmp/conf.xml` - Language, timezone, NTP, device info
- `aswm_settings.ini` - Answering machine settings
- `tvcc_settings.ini` - Camera settings (brightness, contrast, saturation)
- `settings.xml` - Ringtones, volumes, display settings

**File watching**: Polls config files every 2 seconds for changes. When a change
is detected, the new values are published to MQTT with throttling to prevent
flooding.

**Bridge configuration**: The bridge's own config (`configs/config.yaml`) supports
hot-reload, backup (10 max), restore, validation, and change history (100 entries).
All manageable through the web dashboard or REST API.

**Implementation**: `pkg/deviceconfig/reader.go`, `pkg/deviceconfig/confxml.go`,
`pkg/deviceconfig/settings.go`, `pkg/deviceconfig/aswm.go`,
`pkg/deviceconfig/tvcc.go`, `pkg/deviceconfig/file_watcher.go`,
`pkg/deviceconfig/mqtt_publisher.go`

---

### 10. Multicast Listener

Listens on UDP multicast address `239.255.76.67:7667` for OpenWebNet bus events
broadcast by the device's native processes. This provides an alternative event
source alongside the TCP EVENT session.

Detected events include: door lock/unlock, doorbell ring, video stream
start/stop, and mute toggle.

Note: The multicast port may be occupied by the device's native syslog process,
in which case the listener falls back gracefully.

**Implementation**: `pkg/multicast/listener.go`,
`pkg/multicast/handlers/openwebnet_handler.go`

---

### 11. Event Bus

Internal asynchronous publish/subscribe system that connects all bridge
subsystems. Uses a worker pool with a 1000-event buffer and supports wildcard
pattern matching (e.g., subscribing to `door.*` receives both `door.lock` and
`door.unlock` events).

Includes panic recovery per handler and runtime statistics (events
published/delivered, handler count).

**Implementation**: `pkg/events/bus.go`, `pkg/events/bus_test.go`

---

### 12. SIP Client

Dual-role SIP client that registers two identities (`webrtc` and `c300x`) with
the device's local Flexisip SIP proxy at `127.0.0.1:5060`.

**Self-INVITE flow**: The `webrtc` identity sends a SIP INVITE to `c300x`.
Flexisip routes it back to the bridge, which auto-answers with 200 OK and
completes the dialog with ACK. This triggers the SIP call state that enables
camera capture.

The Flexisip domain is `2617372.bs.iotleg.com` and localhost connections
(`127.0.0.1`) are trusted (no authentication required).

**Implementation**: `pkg/sip/client.go`

---

## Network Ports

| Port | Protocol | Service | Direction |
|------|----------|---------|-----------|
| 8080 | TCP | Web dashboard + REST API | Inbound |
| 6554 | TCP | RTSP video streaming | Inbound |
| 5002 | TCP | HomeKit (HAP) | Inbound |
| 10000 | UDP | RTP audio relay | Internal |
| 10002 | UDP | RTP video relay | Internal |
| 20000 | TCP | OpenWebNet EVENT session | Outbound (localhost) |
| 30006 | TCP | OpenWebNet commands | Outbound (localhost) |
| 30007 | TCP | OpenWebNet video control | Outbound (localhost) |
| 5060 | TCP | SIP (Flexisip) | Outbound (localhost) |
| 1883 | TCP | MQTT broker | Outbound (to HA) |

Firewall rules for inbound ports are persisted in
`/etc/network/if-pre-up.d/iptables` on the device.

---

## Go Package Reference

| Package | Files | Description |
|---------|-------|-------------|
| `cmd/` | 1 | Entry point. Initializes and orchestrates all subsystems |
| `pkg/config` | 1 | YAML configuration loader with defaults and validation |
| `pkg/bticino` | 1 | Device constants: GPIO pins, LEDs, filesystem paths, ports |
| `pkg/bticino_commands` | 1 | High-level command sequences with timing (door unlock, light) |
| `pkg/openwebnet` | 4 | OWN protocol: TCP client, HMAC auth, 158+ commands, safety |
| `pkg/mqtt` | 1 | MQTT bridge: Paho client, 39+ HA entities, command handling |
| `pkg/homekit` | 4 | HomeKit bridge: doorbell, lock, camera accessories |
| `pkg/sip` | 7 | SIP client, RTSP server, RTP relay, GStreamer, video manager |
| `pkg/webserver` | 6 | HTTP server, REST API, SSE, config management handlers |
| `pkg/events` | 2 | Async event bus with pattern matching |
| `pkg/input` | 3 | Hardware input: keypad, touchscreen, GPIO via sysfs |
| `pkg/multicast` | 2 | UDP multicast listener + OWN event handler |
| `pkg/messageparser` | 1 | Answering machine message and memo parser |
| `pkg/deviceconfig` | 7 | Device config readers (XML/INI), file watcher, MQTT publisher |
| `pkg/udpproxy` | 1 | UDP proxy (port 40004 to 4000 for scrcpy) |
| `pkg/version` | 1 | Version management from VERSION file |

**Total**: ~44 Go source files, 16 packages, ~17,000 lines of Go code.

---

## Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/brutella/hap` | v0.0.35 | HomeKit Accessory Protocol |
| `github.com/eclipse/paho.mqtt.golang` | v1.5.1 | MQTT client |
| `github.com/sirupsen/logrus` | v1.9.3 | Structured logging |
| `golang.org/x/net` | v0.44.0 | Multicast/IPv4 networking |
| `golang.org/x/sys` | - | Linux system calls |
| `gopkg.in/yaml.v2` | v2.4.0 | YAML parsing |
| `github.com/swaggo/swag` | - | Swagger/OpenAPI generation |

---

## Comparison with slyoldfox/c300x-controller

The [c300x-controller](https://github.com/slyoldfox/c300x-controller) by
slyoldfox is a Node.js/TypeScript project with similar goals. Key differences:

| Aspect | bticino_bridge | c300x-controller |
|--------|---------------|-----------------|
| Language | Go | TypeScript/Node.js |
| Binary | Single static binary (~15MB) | Node.js runtime + dependencies |
| HomeKit | Native Go (brutella/hap) | hap-nodejs |
| MQTT | Paho with 39+ HA entities | Basic MQTT |
| Web UI | Svelte SPA with SSE | Minimal |
| Video | Direct GStreamer + RTSP server | ffmpeg/scrcpy-based |
| OWN commands | 158+ definitions, 4-level safety | Focused set |
| Config | Hot-reloadable YAML with backup | JSON config |
| Answering machine | Full management (list/play/delete) | Not implemented |
| Device config | Reads native XML/INI + file watcher | Not implemented |

---

## Known Limitations

1. **Camera framerate**: The hardware camera outputs ~7fps (not 25fps as V4L2
   caps suggest). This is a hardware limitation of the i.MX6ULL VPU.

2. **VPU state**: After many GStreamer pipeline open/close cycles, the VPU/IPU
   can enter a stuck state requiring a device power cycle.

3. **TCP interleaved RTSP**: The reader loop breaks on binary RTP data after
   PLAY. UDP transport works reliably.

4. **Packet loss**: The VPU encoder does not respect bitrate caps well with
   `idr-interval=1`, causing occasional jitter buffer overflows.

5. **Read-only root filesystem**: System-level changes require
   `mount -oremount,rw /` and must be reverted after.

6. **Test coverage**: Only `pkg/events/` has unit tests. Other packages lack
   automated test coverage.
