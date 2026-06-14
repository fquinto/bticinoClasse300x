# BTicino Classe 300X Enhanced Bridge

Bridge en Go para el videoportero BTicino Classe 300X. Integra el dispositivo con
Web dashboard, Apple HomeKit, Home Assistant (MQTT), API REST y **streaming WebRTC/RTSP**.

**Version:** 0.12.0
**Target:** Linux ARM7 (i.MX6ULL del Classe 300X)
**Dispositivo:** 192.168.1.38

## Estado real

| Componente | Version | Estado |
|---|---|---|
| **WebRTC/RTSP Streaming** | v0.12.0 | ✅ **NUEVO** (5 streams, HKSV recording, REST API) |
| Web dashboard (:8082) | v0.12.0 | ✅ Funciona (5 paginas + streaming controls) |
| API REST (24 endpoints) | v0.12.0 | ✅ Funciona (10 streaming + 14 existentes) |
| OpenWebNet client (80+ cmds) | v0.12.0 | ✅ Funciona (puerto 20000 monitor + 30006 comandos) |
| MQTT Home Assistant (39+ entidades) | v0.12.0 | ✅ Funciona (discovery + object_id) |
| Monitor botones fisicos | v0.12.0 | ✅ Funciona (event0 teclado, event1 touch, event2 GPIO) |
| Monitor eventos bus (timbre) | v0.12.0 | ✅ Funciona (deteccion timbre desde bus OWN) |
| Monitorizacion fisica (temp, LEDs, GPIO) | v0.12.0 | ✅ Funciona (41.1C, 7 LEDs, 13 GPIO) |
| Visor de logs web (/logs) | v0.12.0 | ✅ Funciona (ring buffer 500 entradas) |
| Multicast listener (:7667) | v0.12.0 | ⚠️ Implementado (puerto ocupado por nativo) |
| HomeKit (:8081) | v0.12.0 | ⚠️ No verificado |
| SIP/RTSP video | v0.12.0 | ✅ **Enhanced** (auto SIP call, recording) |

**v0.12.0** añade **streaming WebRTC/RTSP completo** con paridad a slyoldfox/c300x-controller:
- 5 stream paths (/doorbell, /doorbell-video, /doorbell-recorder, /video, /stream)
- HKSV recording automático
- 10 endpoints REST API para streaming
- go2rtc integration documentada
- Base64 deployment script


---

## Arquitectura

```
BTicino Classe 300X (192.168.1.38)
 Procesos nativos: bt_vct, openserver, btvideophone (NO se tocan)
 Puertos OpenWebNet: 20000 (lectura), 30006 (comandos), 30007 (video)

 bticino-bridge (binario ARM, ~8-13MB)
  ├── OpenWebNet Client ──── Monitorea puerto 20000 (read-only)
  │                          Comandos via netcat a 30006 (no-persistente)
  ├── Web Server (:8082) ─── Dashboard HTML/CSS/JS embebido
  │                          API REST: /api/status, /api/controls/door/unlock, ...
  ├── MQTT Bridge ────────── Paho MQTT client -> HA broker (192.168.1.3:1883)
  │                          37 entidades auto-discovery + object_id
  ├── HomeKit Bridge ─────── brutella/hap en puerto 8081
  │                          Accessories: lock, doorbell, camera
  ├── Event Bus ──────────── Pub/sub interno entre componentes
  ├── Multicast Listener ─── UDP 239.255.76.67:7667 (syslog BTicino)
  ├── SIP/RTSP Client ────── Registro SIP, streaming H.264
  ├── Input Monitor ──────── /dev/input/event0 (botones), GPIO
  └── Message Parser ─────── /home/bticino/cfg/extra/47/messages/
```

## Paquetes Go

| Paquete | Archivos | Lineas | Descripcion |
|---|---|---|---|
| `cmd/` | 1 | ~1095 | Entry point. Orquesta todos los subsistemas |
| `pkg/openwebnet/` | 4 | 2211 | Cliente OpenWebNet: TCP multi-puerto, auth HMAC, safety manager |
| `pkg/webserver/` | 1 | ~3400 | Web server + API REST + UI embebido (archivo mas grande) |
| `pkg/sip/` | 3 | 1454 | Cliente SIP/RTSP: registro TLS, streaming H.264, video manager |
| `pkg/input/` | 3 | 936 | Monitor de hardware: botones, touchscreen, GPIO sysfs |
| `pkg/home_assistant/` | 1 | 652 | Integracion HA: discovery payloads, entidades MQTT |
| `pkg/homekit/` | 4 | 551 | Bridge HomeKit: lock, doorbell, camera accessories |
| `pkg/messageparser/` | 1 | 505 | Parser del contestador: msg_info.ini, imagenes base64 |
| `pkg/events/` | 2 | 489 | Event bus pub/sub con pattern matching (+ 1 test) |
| `pkg/multicast/` | 2 | 618 | Listener multicast UDP + handler OpenWebNet |
| `pkg/bticino_commands/` | 1 | 368 | Comandos alto nivel: secuencias multi-paso (puerta, contestador) |
| `pkg/mqtt/` | 1 | ~990 | Bridge MQTT: Paho client, 37 entidades HA, 8 cmd topics, LWT |
| `pkg/config/` | 1 | 227 | Carga de configuracion YAML con defaults |
| `pkg/bticino/` | 1 | 225 | Constantes: paths del filesystem, puertos, comandos OWN |
| `pkg/version/` | 1 | 85 | Gestion de version con inyeccion en build time |
| **Total** | **27** | **~13100** | |

## Dependencias

| Modulo | Version | Uso |
|---|---|---|
| `github.com/brutella/hap` | v0.0.35 | HomeKit Accessory Protocol |
| `github.com/eclipse/paho.mqtt.golang` | v1.5.1 | Cliente MQTT |
| `github.com/sirupsen/logrus` | v1.9.3 | Logging estructurado |
| `golang.org/x/net` | v0.44.0 | Networking extendido (multicast/ipv4) |
| `gopkg.in/yaml.v2` | v2.4.0 | Parsing YAML |

## Compilar y desplegar

```bash
# Compilar para ARM (desde el host)
make build-arm
# o manualmente:
GOOS=linux GOARCH=arm GOARM=7 go build -ldflags "-s -w" -o bticino-bridge cmd/main.go

# Desplegar al dispositivo
make deploy-unified
# o manualmente:
scp bticino-bridge root2@192.168.1.38:/home/bticino/cfg/extra/
scp configs/config.yaml root2@192.168.1.38:/home/bticino/cfg/extra/configs/

# Iniciar en el dispositivo
ssh bticino "cd /home/bticino/cfg/extra && ./bticino-bridge -config configs/config.yaml"

# O usando el script estandar (desde el root del repo)
../deploy-standard.sh full
```

### Makefile targets

| Target | Descripcion |
|---|---|
| `make build` | Compilar para arquitectura local |
| `make build-arm` | Cross-compile para ARM7 (BTicino) |
| `make build-unified` | Build con ldflags (build time injection) |
| `make deploy-unified` | SCP binario + config al dispositivo |
| `make test-unified` | Deploy + restart en dispositivo |
| `make monitor-unified` | Tail de logs en dispositivo via SSH |
| `make connect` | SSH al dispositivo |
| `make clean` | Eliminar artefactos de build |
| `make deps` | go mod tidy + download |
| `make status` | Ver procesos nativos BTicino en dispositivo |

## Configuracion

Archivo principal: `configs/config.yaml`

```yaml
bridge:
  name: "BTicino Classe 300X"
  log_level: "info"

openwebnet:
  host: "127.0.0.1"       # localhost cuando corre en el dispositivo
  ports: [20000, 30006, 30007]
  timeout: 10s

web:
  enabled: true
  port: 8082

mqtt:
  enabled: true
  host: "192.168.1.3"     # IP del broker Home Assistant
  port: 1883
  username: "mqtt_user"
  password: "CHANGE_ME"
  topic_prefix: "bticino"  # Prefijo de datos (NO usar "homeassistant")

homekit:
  enabled: true
  port: "8081"
  pin: "14725803"
  storage_path: "./homekit_data"
```

Variante para Home Assistant: `configs/config_ha.yaml`

## API REST

```bash
# Estado del sistema (incluye MQTT, LEDs, storage)
GET  /api/status

# Logs del bridge (filtro por nivel y cantidad)
GET  /api/logs?level=info&count=200

# Control de puerta (press + release automatico tras 1s)
POST /api/controls/door/unlock

# Display
POST /api/controls/display/on
POST /api/controls/display/off

# Audio (mute)
POST /api/controls/mute/on
POST /api/controls/mute/off

# Timbre (doorbell sound)
POST /api/controls/doorbell/on
POST /api/controls/doorbell/off

# Contestador automatico
POST /api/controls/answering-machine/toggle

# Luz de escalera (press + release automatico tras 1s)
POST /api/controls/light/on

# Comando OpenWebNet arbitrario
POST /api/controls/command   # Body: {"command": "*8*19*20##"}

# Mensajes del contestador
GET  /api/messages

# Componentes del bridge
GET  /api/components
```

## Tests

Solo hay un archivo de test: `pkg/events/bus_test.go` (129 lineas, 3 tests).

```bash
go test ./pkg/events/
```

Cobertura de tests muy baja. Solo el event bus tiene tests unitarios.

## TODOs en el codigo

| Archivo | Linea | Nota |
|---|---|---|
| `pkg/openwebnet/command.go` | 1040 | Anadir mas comandos del analisis completo |
| `pkg/sip/video.go` | 312 | Forward a RTSP server (no implementado) |

## Problemas conocidos

1. **HomeKit no escucha** - El puerto 8081 no esta abierto en el dispositivo.
2. **webserver/server.go es enorme** (~3400 lineas) - Todo el HTML/CSS/JS esta embebido
   en Go. Deberia separarse en archivos estaticos en `web/`.
3. **Binarios en el repo** - `build/`, `dist/`, binarios sueltos no deberian estar trackeados.
4. **Un solo test** - Solo `pkg/events/` tiene tests. Resto sin cobertura.

## Estructura de archivos

```
bticino_bridge/
├── cmd/
│   └── main.go                        # Entry point (~982 lineas)
├── pkg/
│   ├── bticino/constants.go           # Constantes del dispositivo
│   ├── bticino_commands/commands.go   # Comandos alto nivel
│   ├── config/config.go              # Config YAML loader
│   ├── events/
│   │   ├── bus.go                     # Event bus pub/sub
│   │   └── bus_test.go               # Tests del event bus
│   ├── homekit/
│   │   ├── bridge.go                  # HomeKit bridge principal
│   │   ├── camera.go                  # Accessory: camara
│   │   ├── doorbell.go               # Accessory: timbre
│   │   └── lock.go                    # Accessory: cerradura
│   ├── input/
│   │   ├── events.go                  # Tipos de eventos de input
│   │   ├── gpio.go                    # GPIO sysfs manager
│   │   └── monitor.go                # Monitor de dispositivos input
│   ├── messageparser/messageparser.go # Parser del contestador
│   ├── mqtt/bridge.go                # MQTT bridge (Paho)
│   ├── multicast/
│   │   ├── handlers/openwebnet_handler.go  # Handler OWN multicast
│   │   └── listener.go               # Listener UDP multicast
│   ├── openwebnet/
│   │   ├── auth.go                    # Autenticacion HMAC-SHA256
│   │   ├── client.go                  # Cliente TCP multi-puerto
│   │   ├── command.go                 # 80+ definiciones de comandos
│   │   └── safety.go                  # Safety manager + audit
│   ├── sip/
│   │   ├── client.go                  # Cliente SIP con TLS
│   │   ├── rtsp.go                    # Servidor RTSP re-streaming
│   │   └── video.go                   # Video stream manager
│   ├── version/version.go            # Gestion de version
│   └── webserver/server.go           # Web server + UI embebido (~3400 lineas)
├── configs/
│   ├── config.yaml                    # Config principal
│   ├── config_ha.yaml                # Config variante Home Assistant
│   └── homeassistant/
│       ├── automations.yaml           # Ejemplos de automatizaciones HA
│       ├── discovery.yaml             # Definicion entidades HA
│       └── README.md                  # Guia integracion HA
├── web/
│   ├── static/css/                    # (vacio - UI embebida en Go)
│   ├── static/js/                     # (vacio)
│   └── templates/                     # (vacio)
├── deployment/
│   ├── scripts/build.sh              # Script de compilacion
│   ├── scripts/deploy.sh             # Script de despliegue
│   └── systemd/bticino-bridge.service # Unit systemd
├── docs/
│   ├── HOME_ASSISTANT_DASHBOARD.yaml  # Dashboard Lovelace
│   ├── HOME_ASSISTANT_INTEGRATION.yaml
│   ├── HOMEKIT_INTEGRATION.md
│   ├── OPENWEBNET_COMMANDS.md         # Referencia de comandos OWN
│   ├── PRODUCTION_GUIDE.md
│   ├── WEBRTC_RTSP_STREAMING.md      # 🆕 Guia completa de streaming
│   ├── DEPLOYMENT_GUIDE.md           # 🆕 Deploy en dispositivo real (base64)
│   └── DEVICE_COMMANDS_REFERENCE.md  # 🆕 Comandos probados en BTicino
├── scripts/                           # Scripts de operacion
│   └── deploy_to_bticino.sh          # 🆕 Deploy automatizado con base64
├── build/                             # Binarios compilados (no trackear)
├── dist/                              # Distribucion (no trackear)
├── Makefile
├── CHANGELOG.md
├── VERSION                            # 0.10.0
├── go.mod
└── go.sum
```
