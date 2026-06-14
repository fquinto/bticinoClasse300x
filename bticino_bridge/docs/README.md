# BTicino Bridge - Documentation Index

**Version**: 0.15.5
**Last Updated**: 2026-04-04

---

## Overview

BTicino Bridge is a Go application that runs directly on the BTicino Classe 300X
video intercom (i.MX6ULL ARM7, Linux). It bridges the device's proprietary
OpenWebNet protocol to modern smart home platforms: Apple HomeKit, Home Assistant
(via MQTT), REST API, and RTSP/WebRTC video streaming.

See [FEATURES.md](FEATURES.md) for a complete description of the project
architecture and capabilities.

---

## Documentation Map

### Getting Started

| Document | Description |
|----------|-------------|
| [FEATURES.md](FEATURES.md) | Project architecture, features, and technical details |
| [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) | Full deployment guide for the BTicino device |
| [DEPLOY_REAL_INSTRUCTIONS.md](DEPLOY_REAL_INSTRUCTIONS.md) | Step-by-step real device deployment |
| [PRODUCTION_GUIDE.md](PRODUCTION_GUIDE.md) | Production checklist and hardening |

### Configuration

| Document | Description |
|----------|-------------|
| [FLEXISIP_LOCAL_CONFIG.md](FLEXISIP_LOCAL_CONFIG.md) | Flexisip SIP proxy configuration on the device |
| [SERVER_INFORMATION.md](SERVER_INFORMATION.md) | Device details: firmware, network, filesystem |
| [VERSIONING.md](VERSIONING.md) | Versioning strategy and release process |

### Integrations

| Document | Description |
|----------|-------------|
| [HOMEKIT_INTEGRATION.md](HOMEKIT_INTEGRATION.md) | Apple HomeKit setup (doorbell, lock, camera) |
| [GO2RTC_INTEGRATION.md](GO2RTC_INTEGRATION.md) | go2rtc + Home Assistant camera integration |
| [WEBRTC_RTSP_STREAMING.md](WEBRTC_RTSP_STREAMING.md) | RTSP and WebRTC streaming implementation |
| [ha_config/](ha_config/) | Home Assistant YAML configs (dashboard, automations) |

### Protocol References

| Document | Description |
|----------|-------------|
| [OPENWEBNET_COMMANDS.md](OPENWEBNET_COMMANDS.md) | OpenWebNet protocol command reference (158+ commands) |
| [DEVICE_COMMANDS_REFERENCE.md](DEVICE_COMMANDS_REFERENCE.md) | Tested OWN commands for the Classe 300X |
| [VIDEO_STREAMING_RESEARCH.md](VIDEO_STREAMING_RESEARCH.md) | Video pipeline research: VPU, GStreamer, bt_av_media |

### Development & Debugging

| Document | Description |
|----------|-------------|
| [WEB_ARCHITECTURE.md](WEB_ARCHITECTURE.md) | Web frontend architecture (Svelte SPA) |
| [SVELTE_MIGRATION_COMPLETE.md](SVELTE_MIGRATION_COMPLETE.md) | Migration from embedded HTML to Svelte |
| [SIP_DEBUG_PLAN.md](SIP_DEBUG_PLAN.md) | SIP debugging plan and methodology |
| [AUDIT_REPORT.md](AUDIT_REPORT.md) | Security audit report |
| [MEJORAS_FUTURAS.md](MEJORAS_FUTURAS.md) | Future improvements roadmap |

### API Documentation

| Resource | Description |
|----------|-------------|
| [swagger.yaml](swagger.yaml) | OpenAPI 2.0 specification |
| [swagger.json](swagger.json) | OpenAPI 2.0 specification (JSON) |
| `http://<device>:8080/api/docs/` | Interactive Swagger UI (when bridge is running) |

---

## Quick Start

### Build and Deploy

```bash
cd bticino_bridge

# Cross-compile for the device
GOOS=linux GOARCH=arm GOARM=7 go build -o /tmp/bticino_bridge ./cmd/main.go

# Deploy via base64 transfer
base64 /tmp/bticino_bridge | ssh bticino "base64 -d > /home/bticino/cfg/extra/bticino_bridge_new \
  && chmod +x /home/bticino/cfg/extra/bticino_bridge_new \
  && rm -f /home/bticino/cfg/extra/bticino_bridge \
  && cp /home/bticino/cfg/extra/bticino_bridge_new /home/bticino/cfg/extra/bticino_bridge \
  && chmod +x /home/bticino/cfg/extra/bticino_bridge \
  && rm /home/bticino/cfg/extra/bticino_bridge_new \
  && echo 'Binary deployed'"

# Start on device
ssh bticino "cd /home/bticino/cfg/extra && nohup ./bticino_bridge > /tmp/bticino_test.log 2>&1 &"
```

### Access Points (on device 192.168.1.38)

| Service | URL / Port |
|---------|------------|
| Web Dashboard | `http://192.168.1.38:8080` |
| REST API | `http://192.168.1.38:8080/api/` |
| Swagger UI | `http://192.168.1.38:8080/api/docs/` |
| RTSP Stream | `rtsp://192.168.1.38:6554/doorbell` |
| HomeKit | Port 5002 (mDNS advertised) |
| MQTT | Connects to broker at `192.168.1.99:1883` |

### Verify

```bash
# Check process
ssh bticino "ps aux | grep bticino_bridge"

# Check API
curl http://192.168.1.38:8080/api/status

# Check MQTT
mosquitto_sub -h 192.168.1.99 -t "bticino/#" -v
```

---

## Project Structure

```
bticino_bridge/
├── cmd/main.go                   # Entry point, subsystem orchestration
├── configs/config.yaml           # Main YAML configuration
├── pkg/
│   ├── config/                   # Configuration loader and structs
│   ├── bticino/                  # Device constants (GPIO, LEDs, paths)
│   ├── bticino_commands/         # High-level command sequences
│   ├── openwebnet/               # OWN protocol: TCP client, auth, commands, safety
│   ├── mqtt/                     # MQTT bridge with HA auto-discovery
│   ├── homekit/                  # HomeKit bridge (doorbell, lock, camera)
│   ├── sip/                      # SIP client, RTSP server, RTP relay, GStreamer
│   ├── webserver/                # HTTP server, REST API, SSE, config management
│   ├── events/                   # Internal async event bus with pattern matching
│   ├── input/                    # Hardware input: keypad, touchscreen, GPIO
│   ├── multicast/                # UDP multicast listener (239.255.76.67:7667)
│   ├── messageparser/            # Answering machine message/memo parser
│   ├── deviceconfig/             # Device config reader (XML/INI), file watcher
│   ├── udpproxy/                 # UDP proxy for scrcpy compatibility
│   └── version/                  # Version management
├── web/                          # Svelte SPA frontend source
│   └── src/routes/               # Dashboard, Controls, Messages, Memos, Settings, Logs
├── docs/                         # This documentation directory
├── scripts/                      # Deployment and test scripts
├── Makefile                      # Build targets (build, deploy, test, swagger)
├── CHANGELOG.md                  # Version history
└── VERSION                       # Current version (0.15.5)
```

---

## Key Dependencies

| Module | Purpose |
|--------|---------|
| `github.com/brutella/hap` | HomeKit Accessory Protocol |
| `github.com/eclipse/paho.mqtt.golang` | MQTT client |
| `github.com/sirupsen/logrus` | Structured logging |
| `gopkg.in/yaml.v2` | YAML configuration |
| `github.com/swaggo/swag` | Swagger/OpenAPI generation |

---

## Device Details

| Property | Value |
|----------|-------|
| Model | BTicino Classe 300X (344642) |
| SoC | NXP i.MX6ULL (ARM Cortex-A7) |
| Firmware | 1.7.17 |
| Distribution | 2.19.5 |
| Kernel | 4.9.11 |
| Network | wlan0, 192.168.1.38 |
| Root FS | Read-only (remount for changes) |
| Persistent Storage | `/home/bticino/cfg/extra/` |
| Camera | 720x576 UYVY interlaced PAL, ~7fps |
| VPU | i.MX hardware H.264 encoder |
