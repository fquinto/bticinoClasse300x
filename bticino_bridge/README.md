# BTicino Classe 300X Enhanced Bridge

Bridge en Go para el videoportero BTicino Classe 300X. Corre **en el propio dispositivo**
(ARM7, i.MX6) e integra el videoportero con Home Assistant (MQTT), Apple HomeKit,
un dashboard web Svelte, API REST y **streaming de vídeo RTSP** con aceleración hardware.

**Version:** 0.15.5 (ver `VERSION` y `CHANGELOG.md`)
**Target:** Linux ARM7 (i.MX6 del Classe 300X)

## Estado real

| Componente | Estado |
|---|---|
| **RTSP video (:6554)** | ✅ Funciona — GStreamer directo (VPU i.MX) + RTP relay. Reproducible en VLC/ffplay |
| Web dashboard Svelte (:8082) | ✅ Funciona — 6 páginas (dashboard, controls, messages, memos, logs, settings) |
| API REST (40+ endpoints) | ✅ Funciona — documentada con Swagger UI en `/api/docs/` |
| SSE tiempo real (`/api/events`) | ✅ Funciona — LEDs/GPIO en vivo sin polling |
| OpenWebNet client (80+ cmds) | ✅ Funciona — monitor puerto 20000 + comandos vía netcat a 30006 |
| MQTT Home Assistant (39+ entidades) | ✅ Funciona — auto-discovery + object_id |
| Device config sync (conf.xml, aswm, tvcc) | ✅ Funciona — QML→Bridge→MQTT (idiomas, tonos, volúmenes, display) |
| Contestador: mensajes + memos | ✅ Funciona — marcar leído/no leído, borrar, descargar vídeo |
| Monitor botones físicos | ✅ Funciona — event0 teclado, event1 touch, event2 GPIO |
| Monitorización física (temp, LEDs, GPIO) | ✅ Funciona — 7 LEDs, 13 pines GPIO |
| Visor de logs web (/logs) | ✅ Funciona — ring buffer 500 entradas |
| Multicast listener (:7667) | ⚠️ Implementado (puerto ocupado por proceso nativo) |
| HomeKit (:8081) | ⚠️ Implementado, no verificado |

**v0.15.5** — hito principal: pipeline completo de vídeo RTSP **sin depender de `bt_av_media`**
(que tiene un bug de enrutado incorregible en `libjel.so`):

```
Cliente RTSP (VLC/ffplay) ──RTSP──> bticino-bridge:6554
                                        │
                                   SIP self-INVITE (webrtc→c300x vía Flexisip local)
                                        │
                                   Pipelines GStreamer:
                                     imxv4l2videosrc → imxvpuenc_h264 → rtph264pay → udpsink :10002
                                     alsasrc → speexenc → rtpspeexpay → udpsink :10000
                                        │
                                   RTP Relay (fan-out a todos los clientes RTSP)
```

Detalles y decisiones de arquitectura: `CHANGELOG.md` (registro autoritativo).

---

## Arquitectura

```
BTicino Classe 300X
 Procesos nativos: bt_vct, openserver, bt_av_media, flexisip (NO se tocan)
 Puertos OpenWebNet: 20000 (monitor), 30006 (comandos), 30007 (video)

 bticino-bridge (binario ARM)
  ├── OpenWebNet Client ──── Monitorea puerto 20000 (read-only)
  │                          Comandos vía netcat a 30006 (modo no-interferente)
  ├── Web Server (:8082) ─── SPA Svelte embebida + API REST + Swagger UI + SSE
  ├── MQTT Bridge ────────── Paho MQTT → broker HA (39+ entidades auto-discovery)
  ├── HomeKit Bridge ─────── brutella/hap en :8081 (lock, doorbell, camera)
  ├── SIP/RTSP ───────────── Self-INVITE SIP → GStreamer (VPU) → RTP relay → RTSP :6554
  ├── Device Config ──────── Lee/vigila conf.xml, aswm, tvcc → republica a MQTT
  ├── Event Bus ──────────── Pub/sub interno entre componentes
  ├── Input Monitor ──────── /dev/input/event0-2 (botones, touch, GPIO)
  ├── Message Parser ─────── Mensajes del contestador + memos (voz/texto)
  └── Multicast Listener ─── UDP 239.255.76.67:7667 (syslog BTicino)
```

### Modo no-interferente (crítico)

El bridge convive con los procesos nativos del dispositivo. Para no pelearse con
ellos por puertos/hardware: solo **monitoriza** en el puerto 20000 y envía comandos
haciendo shell-out a netcat (`echo '<frame>' | nc 0 30006`) en vez de mantener
sockets propios. No abrir sockets persistentes a 30006/30007. BTicino requiere
~310 ms de separación entre comandos.

## Paquetes Go

| Paquete | Líneas | Descripción |
|---|---|---|
| `cmd/` | ~1330 | Entry point. Orquesta subsistemas según config + flags |
| `pkg/webserver/` | ~6580 | Web server, API REST, handlers de config/dispositivo/streaming |
| `pkg/sip/` | ~4820 | Vídeo: cliente SIP, GStreamer, RTP relay, servidor RTSP |
| `pkg/openwebnet/` | ~2410 | Cliente OpenWebNet: monitor, auth HMAC, safety manager 4 niveles |
| `pkg/deviceconfig/` | ~1750 | Lectura/watch de conf.xml, aswm, tvcc → MQTT |
| `pkg/mqtt/` | ~1710 | Bridge MQTT: Paho, discovery HA, command topics, LWT |
| `pkg/input/` | ~975 | Monitor hardware: botones, touchscreen, GPIO sysfs |
| `pkg/messageparser/` | ~750 | Parser contestador: msg_info.ini, memos voz/texto |
| `pkg/bticino/` | ~700 | Constantes: paths, puertos, comandos OWN |
| `pkg/multicast/` | ~610 | Listener UDP multicast + handler OpenWebNet |
| `pkg/homekit/` | ~550 | Bridge HomeKit: lock, doorbell, camera |
| `pkg/events/` | ~490 | Event bus pub/sub con pattern matching (+ tests) |
| `pkg/config/` | ~420 | Config YAML con defaults |
| `pkg/bticino_commands/` | ~370 | Comandos de alto nivel: secuencias multi-paso |
| `pkg/udpproxy/` | ~150 | Proxy UDP auxiliar |
| `pkg/version/` | ~90 | Versión leída de `VERSION` + build-time injection |

## Dependencias

| Módulo | Uso |
|---|---|
| `github.com/brutella/hap` | HomeKit Accessory Protocol |
| `github.com/eclipse/paho.mqtt.golang` | Cliente MQTT |
| `github.com/sirupsen/logrus` | Logging estructurado |
| `github.com/swaggo/swag` | Generación de documentación Swagger |
| `golang.org/x/net` | Multicast/ipv4 |
| `gopkg.in/yaml.v2` | Parsing YAML |

Frontend: Svelte 4 + Vite (bajo `web/`, requiere Node.js para compilar).

## Compilar y desplegar

```bash
make build        # frontend Svelte (web/dist) + binario Go ARM
make build-go     # solo binario: GOOS=linux GOARCH=arm GOARM=7
make build-web    # solo frontend (npm install + vite build)
make dev          # desarrollo local: vite :5173 + go run ./cmd/main.go
make deploy       # build + scripts/deploy.sh (scp al dispositivo, restart)
make test         # scripts/run_all_tests.sh --all (requiere dispositivo)
make clean
```

El binario se instala en `/home/bticino/cfg/extra/` del dispositivo.
Alternativa desde el root del repo: `../deploy-standard.sh full` (streaming base64 por SSH).

### Flags de runtime (`cmd/main.go`)

`-config` (default `configs/config.yaml`), `-log-level`, `-version`,
`-test` (sin conexión al dispositivo), `-web-port`, y toggles
`-enable-openwebnet` / `-enable-web` / `-enable-homekit` / `-enable-video`.

## Configuración

Archivo único: `configs/config.yaml` (los demás en `configs/` son ejemplos históricos).

```yaml
bridge:
  name: "BTicino Bridge Enhanced"
  log_level: "info"

openwebnet:
  host: "127.0.0.1"
  port: 30006

sip:
  enabled: true
  server_host: "127.0.0.1"   # Flexisip local
  transport: "tcp"
  username: "webrtc"          # self-INVITE: webrtc@dominio → c300x@dominio
  sip_target: "c300x"

mqtt:
  enabled: true
  host: "192.168.1.3"         # IP del broker (Home Assistant)
  port: 1883
  username: "mqtt_user"
  password: "CHANGE_ME"
  topic_prefix: "bticino"     # NO usar "homeassistant"

web:
  enabled: true
  port: 8082

homekit:
  enabled: true
  port: "8081"
  pin: "12345678"
  storage_path: "./homekit_data"
```

## API REST

40+ endpoints documentados en **Swagger UI**: `http://<ip-dispositivo>:8082/api/docs/`

Resumen por grupos:

```bash
# Estado / sistema
GET  /api/status              GET /api/system
GET  /api/events              # SSE: LEDs/GPIO en tiempo real

# Mensajes del contestador y memos
GET  /api/messages            GET /api/messages/{id}
GET  /api/messages/download/{id}/{type}
POST /api/messages/mark-read/{id}
DELETE /api/messages/delete/{id}
GET  /api/memos               GET /api/memos/{id}

# Controles
POST /api/controls/door/unlock
POST /api/controls/display/on|off
POST /api/controls/mute/on|off
POST /api/controls/doorbell/on|off
POST /api/controls/answering-machine/toggle
POST /api/controls/light/on
POST /api/controls/command    # OpenWebNet arbitrario: {"command": "*8*19*20##"}

# Streaming
GET  /api/streaming           POST /api/streaming/start|stop
GET  /api/streaming/sessions|config
POST /api/streaming/record

# Configuración (bridge y dispositivo nativo)
GET  /api/config              POST /api/config/save|validate|backup|restore|reload
GET  /api/config/device|language|timezone|ntp|ringtones|volumes|display|cameras|answering
POST /api/device/save

# Logs
GET  /api/logs?level=info&count=200
GET  /api/logs/download
```

## Vídeo RTSP

```bash
# Desde cualquier equipo de la LAN:
ffplay rtsp://<ip-dispositivo>:6554/doorbell
vlc rtsp://<ip-dispositivo>:6554/doorbell
```

- H.264 hardware (VPU i.MX), 720x576 PAL, ~7 fps, ~1500 kbps
- Soporta UDP unicast y TCP interleaved, múltiples clientes simultáneos (fan-out)
- Ver `docs/WEBRTC_RTSP_STREAMING.md` y la entrada v0.15.5 del `CHANGELOG.md`

## Tests

```bash
go test ./...                          # unit tests (pkg/events)
make test                              # tests de integración (requiere dispositivo)
```

La cobertura unitaria es baja: solo `pkg/events/` tiene tests. El resto se valida
con los tests de integración de `scripts/run_all_tests.sh` contra el dispositivo real.

## Estructura de archivos

```
bticino_bridge/
├── cmd/main.go                        # Entry point (~1330 líneas)
├── pkg/
│   ├── bticino/                       # Constantes del dispositivo
│   ├── bticino_commands/              # Comandos de alto nivel
│   ├── config/                        # Config YAML loader
│   ├── deviceconfig/                  # conf.xml/aswm/tvcc → MQTT (7 archivos)
│   ├── events/                        # Event bus pub/sub (+ tests)
│   ├── homekit/                       # Bridge HomeKit (lock, doorbell, camera)
│   ├── input/                         # Botones, touchscreen, GPIO
│   ├── messageparser/                 # Contestador + memos
│   ├── mqtt/                          # Bridge MQTT (Paho)
│   ├── multicast/                     # Listener UDP multicast
│   ├── openwebnet/                    # Cliente OWN: auth, comandos, safety
│   ├── sip/                           # SIP + GStreamer + RTP relay + RTSP
│   ├── udpproxy/                      # Proxy UDP
│   ├── version/                       # Gestión de versión
│   └── webserver/                     # API REST + handlers + Swagger
├── web/                               # Frontend Svelte 4 + Vite
│   └── src/routes/                    # dashboard, controls, messages, memos, logs, settings
├── configs/config.yaml                # Configuración única
├── deployment/                        # systemd unit + scripts
├── docs/                              # Guías (streaming, HA, HomeKit, comandos OWN...)
├── scripts/                           # deploy.sh, run_all_tests.sh, utilidades MQTT
├── Makefile
├── CHANGELOG.md                       # Registro autoritativo de cambios
└── VERSION                            # 0.15.5
```
