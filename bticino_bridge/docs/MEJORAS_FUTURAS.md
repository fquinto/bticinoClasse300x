# Plan de Mejoras Futuras - BTicino Classe 300X Bridge

Estado actual: **v0.15.5** desplegada y funcionando (2026-04-04).
Ultima actualizacion: 2026-04-04 — Video streaming RTSP funcional end-to-end.

---

## RESUMEN DE ESTADO v0.15.5

### Funcionalidades COMPLETADAS

| # | Feature | Version | Estado |
|---|---------|---------|--------|
| 1 | MQTT Home Assistant (39+ entidades, 8 command topics) | v0.10.0 | COMPLETADO |
| 2 | OpenWebNet bus monitor (puertos 20000/30006) | v0.10.0 | COMPLETADO |
| 3 | GPIO monitoring (13 pines, tiempo real) | v0.10.0 | COMPLETADO |
| 4 | LED status publishing | v0.14.8 | COMPLETADO |
| 5 | Contestador automatico (protocolo 4 pasos) | v0.10.0 | COMPLETADO |
| 6 | Web Dashboard Svelte (Dashboard, Settings, Controls, Logs, Messages) | v0.14.2 | COMPLETADO |
| 7 | Swagger UI API docs (/api/docs/) | v0.14.5 | COMPLETADO |
| 8 | Device config parser (conf.xml, settings.xml, aswm, tvcc) | v0.14.0 | COMPLETADO |
| 9 | Device config MQTT publisher + file watcher | v0.14.8 | COMPLETADO |
| 10 | SSE (Server-Sent Events) para updates en tiempo real | v0.15.0 | COMPLETADO |
| 11 | Memos API (voice + text) | v0.15.0 | COMPLETADO |
| 12 | Input monitor (keypad, touchscreen, GPIO events) | v0.10.0 | COMPLETADO |
| 13 | Multicast listener (239.255.76.67:7667, handler OPEN) | v0.10.0 | COMPLETADO |
| 14 | SIP dual-role client (webrtc + c300x via Flexisip local) | v0.15.5 | COMPLETADO |
| 15 | RTSP server (:6554) con SDP H.264+Speex | v0.15.5 | COMPLETADO |
| 16 | GStreamer direct pipelines (imxvpuenc_h264 VPU + alsasrc) | v0.15.5 | COMPLETADO |
| 17 | RTP relay (UDP fan-out a clientes RTSP) | v0.15.5 | COMPLETADO |
| 18 | Video streaming end-to-end (RTSP->SIP->GStreamer->RTP->client) | v0.15.5 | COMPLETADO |

### Totales v0.15.5
- **49 entidades Home Assistant** (locks, switches, buttons, sensors, binary_sensors)
- **8 command topics** escuchando
- **RTSP streams**: `/doorbell`, `/doorbell-video`, `/doorbell-recorder`
- **Puertos**: 8082 (web), 6554 (RTSP), 5060 (SIP local), 10000 (audio RTP), 10002 (video RTP)

---

## EVOLUTIVOS PENDIENTES (ordenados por prioridad)

### Prioridad ALTA — Video streaming mejoras

#### A.1 Reducir packet loss en RTP relay
- **Problema**: ~7.5 Mbps real vs 1.5 Mbps configurado. El VPU no respeta bien el bitrate con `idr-interval=1`.
- **Causa**: IDR cada 7 frames genera I-frames grandes que saturan WiFi.
- **Solucion propuesta**: Probar `idr-interval=0` con `gop-size=15` para reducir I-frames, o bajar resolucion a 352x288.
- **Impacto**: Video mas fluido, menos artefactos.

#### A.2 TCP interleaved RTSP mode
- **Problema**: `handleConnection` usa `ReadString('\n')` que rompe con datos binarios RTP interleaved tras PLAY.
- **Solucion**: Tras enviar respuesta PLAY, cambiar reader loop a leer frames binarios `$channel+length+data`.
- **Impacto**: Permite streaming TCP (mas fiable que UDP sobre WiFi).

#### A.3 VPU/IPU hardware state recovery
- **Problema**: Tras muchos ciclos open/close de GStreamer, el VPU puede quedar en estado stuck (`mxc_v4l_dqueue timeout enc_counter 0`). Solo se recupera con power cycle.
- **Solucion propuesta**: Limitar ciclos, o hacer reset VPU entre sesiones con `/dev/mxc_vpu`.
- **Impacto**: Estabilidad a largo plazo.

#### A.4 Integrar con go2rtc / Home Assistant camera entity
- **Estado**: Documentacion y scripts creados, pero no probado end-to-end con go2rtc.
- **Solucion**: Configurar go2rtc en HA apuntando a `rtsp://192.168.1.38:6554/doorbell`.
- **Impacto**: Video en dashboard de Home Assistant via WebRTC/HLS.

### Prioridad ALTA — Web Dashboard

#### B.1 Pagina de Video Streaming en web
- **Nuevo**: Pagina dedicada para ver video en vivo desde el dashboard web.
- **Implementacion**: Embeber player HLS/MSE que consuma el RTSP via proxy, o snapshot JPEG periodico.
- **Info a mostrar**: Estado SIP (registrado/conectado/idle), GStreamer (running/stopped), RTP stats (packets, bytes, consumers), sesiones RTSP activas.
- **Controles**: Boton Start/Stop stream, seleccion de calidad.

#### B.2 Pagina de estado del sistema mejorada
- **Mostrar**: Version correcta (v0.15.5), uptime, memoria, CPU, temperatura.
- **RTP relay stats**: Paquetes recibidos/enviados, consumidores activos, errores.
- **SIP status**: Estado de registro, llamada activa, duracion.
- **GStreamer**: Pipeline activo, PID, duracion, stderr errors.

#### B.3 Configuracion de streaming desde web
- **Parametros editables**: Bitrate, GOP size, IDR interval, resolucion.
- **Guardado**: En config.yaml, recarga sin reiniciar bridge.
- **Preset**: "Alta calidad" (1500kbps, gop=7), "Baja latencia" (800kbps, gop=3), "Bajo ancho de banda" (500kbps, gop=15).

### Prioridad MEDIA — MQTT / Home Assistant

#### C.1 Sensor de estado de puerta (real)
- **Comando**: `*#1013**1##`
- **Ya existe**: `QueryDoorStatus(doorID)` en client.go
- **Mejora**: El lock actual es optimista. Con query real confirmamos estado.
- **IMPORTANTE**: Usar `ExecuteCommand()` (netcat 30006), NO `owClient.SendCommand()` (puerto 20000).

#### C.2 Camera entity MQTT discovery
- **Nuevo**: Publicar MQTT discovery para entidad camera de HA.
- **Topic**: `homeassistant/camera/bticino_doorbell/config`
- **Payload**: Snapshot URL + RTSP stream URL.
- **Impacto**: Aparece automaticamente en HA como camara.

#### C.3 Bug contestador: doble fuente de verdad
- **Problema**: MQTTBridge usa `b.voicemailEnabled` (memoria), pero la API web usa `messageParser.GetAnsweringMachineStatus()` (filesystem). Se desincronizan.
- **Solucion recomendada**: Consultar estado real del dispositivo tras cada comando.
- **Verificar**: Si mismo problema aplica a `doorbell_sound` y `display`.

#### C.4 Cerradura secundaria (puerta 2)
- **Comandos**: `*8*19*11##` / `*8*20*11##`
- **Prerequisito**: Verificar si hay puerta secundaria fisicamente conectada.

### Prioridad MEDIA — Calidad tecnica

#### D.1 Tests unitarios
- **Solo existe**: `pkg/events/bus_test.go` (3 tests).
- **Prioridad**: Tests para `pkg/mqtt/bridge.go` (discovery configs), `pkg/messageparser/`, command handlers, `pkg/sip/rtp_relay.go`.

#### D.2 HomeKit
- **Estado**: Puerto 8081, PIN 12345678 (insecure), sin SRTP real.
- **Opciones**: (a) Arreglar con PIN valido y SRTP, (b) Eliminar codigo HomeKit y usar MQTT->HA->HomeKit bridge.
- **Recomendacion**: Opcion (b) — menos mantenimiento.

#### D.3 Reconexion robusta OWN
- **Problema**: El cliente OWN se reconecta pero puede perder el callback `OnMessage`.
- **Solucion**: Watchdog que valide eventos cada N minutos.

### Prioridad BAJA — Investigacion

| Tema | Descripcion |
|---|---|
| Puerto serial `/dev/ttymxc2` | Proposito desconocido |
| Canales audio 0-25 | Que representa cada canal |
| Gateway WHO=13 | Queries de red del gateway SCS |
| GPIO 47,52,54,56,176 | Siempre ON, identificar que representan |
| Button-to-Command mapping | Ejecutar OWN cuando se presionan botones fisicos |

---

## ARQUITECTURA ACTUAL v0.15.5

### Flujo de video streaming (FUNCIONAL)

```
Cliente RTSP (ffplay/VLC/go2rtc)
    │
    │ RTSP (TCP :6554)
    │ OPTIONS -> DESCRIBE -> SETUP (video+audio) -> PLAY
    ▼
bticino_bridge (RTSP server)
    │
    │ SIP INVITE (webrtc -> c300x via Flexisip local :5060)
    │ Auto-answer 200 OK -> ACK -> Connected
    ▼
GStreamer pipelines (lanzados por bridge):
    Video: imxv4l2videosrc ! imxvpuenc_h264 gop-size=7 idr-interval=1
           ! rtph264pay config-interval=1 ! udpsink 127.0.0.1:10002
    Audio: alsasrc hw:0 ! speexenc ! rtpspeexpay ! udpsink 127.0.0.1:10000
    │
    │ RTP (UDP localhost)
    ▼
RTP Relay (fan-out)
    │
    │ UDP unicast a cada cliente RTSP
    ▼
Cliente recibe H.264 720x576 ~7fps + Speex 8kHz mono
```

### Descubrimientos tecnicos clave

- **bt_av_media MQTT es inutilizable** desde publishers externos (bug en libjel.so routing).
- **GStreamer directo funciona**: bypass completo de bt_av_media.
- **Camera**: /dev/video0, 720x576 UYVY interlaced PAL, ~7fps reales.
- **VPU encoder**: H.264 Constrained Baseline Level 3.0, byte-stream.
- **SIP dual-role**: Bridge registra como `webrtc` Y `c300x` en Flexisip. Self-INVITE.
- **ARM 32-bit**: uint64 atómicos deben estar alineados a 64-bit (campos primero en struct).
- **Root FS read-only**: Usar `mount -oremount,rw /` para cambios.

### Configuracion SIP actual

```yaml
sip:
  enabled: true
  server_host: "127.0.0.1"
  server_port: 5060
  transport: "tcp"
  domain: "2617372.bs.iotleg.com"
  username: "webrtc"
  c300x_username: "c300x"
```

- **webrtc HA1**: `2834e60a07dabee339f91b000222edee`
- **c300x HA1**: `69abb96449f850f15aa6f2378eac76a0`
- **Trusted-hosts**: `127.0.0.1` (sin autenticacion para localhost)

### Puertos del sistema

| Puerto | Protocolo | Servicio | Notas |
|--------|-----------|----------|-------|
| 5060 | TCP | Flexisip SIP (local) | Solo localhost |
| 5061 | TLS | Flexisip SIP (externo) | Certificados |
| 6554 | TCP | RTSP server (bridge) | Video streaming |
| 8082 | TCP | Web dashboard (bridge) | Admin UI |
| 8081 | TCP | HomeKit (bridge) | PIN inseguro |
| 10000 | UDP | RTP relay audio | GStreamer -> relay |
| 10002 | UDP | RTP relay video | GStreamer -> relay |
| 20000 | TCP | OpenWebNet eventos | Solo lectura |
| 30006 | TCP | OpenWebNet comandos | Via netcat |

### iptables (persistidos en `/etc/network/if-pre-up.d/iptables`)

```bash
# BTicino Bridge ports
for i in 6554 8081 8082; do
    iptables -A INPUT -p tcp -m tcp --dport $i -j ACCEPT
done
iptables -A INPUT -p udp --dport 10000:10010 -j ACCEPT
```

---

## ARCHIVOS CLAVE DEL PROYECTO

### Codigo fuente (pkg/sip/)
| Archivo | Descripcion |
|---------|-------------|
| `client.go` | SIP dual-role client (webrtc + c300x), INVITE/BYE/REGISTER |
| `rtsp_server_enhanced.go` | RTSP server con RTP relay, SETUP/PLAY/TEARDOWN |
| `rtsp.go` | Tipos RTSP (RTSPSession, RTSPRequest) |
| `rtp_relay.go` | RTP relay UDP fan-out, ARM-safe atomics |
| `gstreamer.go` | GStreamer pipeline launcher, VPU H.264 encoding |
| `video.go` | VideoStreamManager |

### Configuracion
| Archivo | Descripcion |
|---------|-------------|
| `configs/config.yaml` | Config principal (SIP, MQTT, streaming, web) |
| `VERSION` | Version actual (0.15.5) |

### Documentacion (docs/)
| Archivo | Descripcion | Estado |
|---------|-------------|--------|
| `DEPLOYMENT_GUIDE.md` | Guia de despliegue | Actualizado |
| `DEVICE_COMMANDS_REFERENCE.md` | Referencia comandos OpenWebNet | Actualizado |
| `VIDEO_STREAMING_RESEARCH.md` | Investigacion video (52K) | Actualizado v0.15.5 |
| `GO2RTC_INTEGRATION.md` | Integracion go2rtc | Pendiente verificar |
| `HOMEKIT_INTEGRATION.md` | HomeKit (puede eliminarse) | Obsoleto |
| `FLEXISIP_LOCAL_CONFIG.md` | Config Flexisip local | Actualizado |
| `ha_config/` | YAML para Home Assistant | Actualizado |

---

## COMPARACION CON SLYOLDFOX/C300X-CONTROLLER

| Feature | slyoldfox (Node.js) | bticino_bridge v0.15.5 (Go) |
|---------|---------------------|---------------------------|
| Video RTSP | RTSP relay server | RTSP + SIP + GStreamer directo |
| MQTT | Simple events | 49 entidades HA discovery |
| Web UI | Dashboard basico | Svelte moderno (5 paginas) |
| API REST | Limitada | 40+ endpoints + Swagger |
| HomeKit | Bundle experimental | Basico (PIN inseguro) |
| Deploy | Manual | Script automatizado |
| Lenguaje | Node.js | Go (mas eficiente en ARM) |
| Config sync | No | File watcher + MQTT publish |
| Device config | No | Parser completo (conf.xml, settings) |
| Input monitor | No | Keypad + touchscreen + GPIO |

---

*Documento creado: 2026-03-11, v0.9.0*
*Reescrito: 2026-04-04, v0.15.5 — Estado real del proyecto con evolutivos priorizados*
