# Puesta en marcha del contestador SIP (F1 + F2)

Runbook operativo para activar el contestador SIP bajo demanda desde Home
Assistant (`sip.answerer`, introducido en v0.25.0). Contesta/cuelga la
llamada SIP que el c300x hace hacia Flexisip al pulsar el timbre, sin
necesidad de la app. El vídeo y el audio de la puerta durante la llamada se
sirven con los taps pasivos ya existentes (v0.24.x); las secciones 1-5 cubren
el alta del contestador en sí (F1); la sección 6 cubre **hablar** hacia la
puerta desde HA (F2, v0.26.0).

> **F2 (hablar) ya está implementado en código** desde v0.26.0
> (`sip.answerer.talk_enabled`), pero el gate en vivo (oírse de verdad por la
> placa) está **pendiente de validar** — depende de que F1 esté validado en
> vivo primero. Hasta confirmarlo, trátalo como función en pruebas; ver
> sección 6.

## 1. Dar de alta la ruta en Flexisip (en el device)

Por SSH al alias `bticino`:

```bash
ssh bticino
cd /home/bticino/cfg/extra   # o donde tengas los scripts desplegados
scripts/setup_answerer_route.sh --add --file /etc/flexisip/users/route_ext.conf
```

El script añade el AOR del bridge (`bridge@<dominio>`) a la línea `alluser`
del fichero de rutas indicado, con backup automático
(`route_ext.conf.bak.<timestamp>`). Es idempotente: si el AOR ya está
presente, no hace nada. Reversión con `--restore` (recupera el backup más
reciente) o `--check` para verificar sin modificar.

> **Qué fichero se comprueba:** Flexisip lee `/etc/flexisip/users/route.conf`,
> un enlace que el equipo apunta a `route_ext.conf` o `route_int.conf`. El
> bridge resuelve ese enlace y vigila el fichero activo cada minuto (no solo al
> arrancar); si el usuario del bridge desaparece de la línea `alluser` —por
> ejemplo tras cerrar y abrir sesión en la app, que reescribe esos ficheros—
> lo verás en el log (`[ANSWERER] el contestador no recibirá llamadas…`) y en
> el sensor de problema de HA "Contestador sin ruta", cuyo atributo `file`
> dice qué fichero se ha comprobado. Usa ese mismo fichero con `--file` al
> volver a ejecutar el script.

## 2. Reiniciar Flexisip (NUNCA con SIGHUP)

Un `kill -HUP` o `/etc/init.d/flexisipsh restart` a secas no siempre
recarga la tabla de rutas de forma fiable en este montaje. La receta
verificada es parar y arrancar explícitamente pasando el fifo del
contestador nativo:

```bash
/etc/init.d/flexisipsh start 192.168.1.38 /var/tmp/bt_answering_machine.fifo
printf 'a\n' > /var/tmp/bt_answering_machine.fifo
```

El segundo comando "despierta" el proceso escribiendo en su fifo de control;
sin él, el arranque puede quedarse en un estado en que no procesa el fork
nuevo hacia el AOR del bridge.

## 3. Activar el bloque `sip.answerer` en el config del device

**`deploy-standard.sh` sube el binario y `VERSION`, pero NO el
`config.yaml`.** Hay que editar a mano el config que vive en el device
(`/home/bticino/cfg/extra/configs/config.yaml` o la ruta equivalente que use
tu despliegue) y asegurarte de que, bajo `sip:`, el bloque `answerer` tenga
`enabled: true`:

```yaml
sip:
  enabled: true
  # ... resto del bloque sip sin cambios ...
  answerer:
    enabled: true            # <-- cambiar de false a true en el device
    user: "bridge"
    audio_port: 10100
    video_port: 10102        # -1 = rechazar m=video (audio-only)
    max_call_seconds: 120
    talk_enabled: false      # F2 (hablar); ver sección 6 antes de activarlo
    route_file: "/etc/flexisip/users/route_ext.conf"
```

Reinicia el bridge tras el cambio:

```bash
./deploy-standard.sh restart
# o, si ya está desplegado y solo cambiaste config:
ssh bticino "/home/bticino/cfg/extra/start-bridge.sh restart"
```

En el log de arranque deberías ver:

```
[ANSWERER] contestador SIP activo (bajo demanda desde HA)
```

Si en cambio ves `route-check no disponible` o `el AOR ... NO está en
route_ext.conf`, el arranque continúa igualmente (no es fatal) pero el fork
del timbre no llegará al contestador hasta resolver el paso 1.

Si ves `no se pudo conectar al Flexisip local`, el bridge sigue arrancando
sin contestador (fail-open a "sin contestador", no crashea) — revisa que
Flexisip esté arriba y escuchando en `sip.server_host:sip.server_port`
(por defecto `127.0.0.1:5060`).

## 4. Tarjeta de dashboard en Home Assistant

El MQTT bridge publica automáticamente por discovery (sin YAML manual)
tres entidades nuevas:

- `button.bticino_contestar` — contesta la llamada entrante.
- `button.bticino_colgar` — cuelga la llamada en curso.
- `sensor.bticino_contestador` — estado del contestador (p. ej. `idle`,
  `ringing`, `answered`, `ended`), reflejo de los eventos `doorcall.*` del
  bus interno.

Ejemplo de tarjeta para el dashboard `portero-bticino`, junto al iframe de
go2rtc que ya sirve `/doorbell` (vídeo+audio):

```yaml
type: vertical-stack
cards:
  - type: iframe
    url: "http://192.168.1.3:1984/stream.html?src=doorbell"
    aspect_ratio: 16:9
  - type: entities
    title: Contestador
    entities:
      - entity: sensor.bticino_contestador
        name: Estado
      - entity: button.bticino_contestar
        name: Contestar
      - entity: button.bticino_colgar
        name: Colgar
```

### Automatización opcional: notificación móvil al sonar el timbre

```yaml
automation:
  - alias: "Portero BTicino - notificar timbre"
    trigger:
      - platform: state
        entity_id: sensor.bticino_contestador
        to: "ringing"
    action:
      - service: notify.mobile_app_tu_movil
        data:
          title: "Portero"
          message: "Están llamando al telefonillo"
          data:
            actions:
              - action: "BTICINO_CONTESTAR"
                title: "Contestar"
            push:
              category: "bticino_doorcall"
```

(La acción `BTICINO_CONTESTAR` necesita un automation adicional que escuche
`mobile_app_notification_action` y llame a `button.press` sobre
`button.bticino_contestar`; se deja fuera de este runbook por brevedad.)

## 5. Riesgo residual sin confirmar (F0)

La coexistencia del contestador con el **monitor interior** (el telefonillo
físico de pared) mientras hay una llamada contestada desde HA **no se ha
validado en vivo todavía**. En teoría, al ser el bridge una parte de red más
del fork (igual que la app), debería sonar/coexistir igual que cuando
contesta la app — pero no hay confirmación empírica de que el monitor
interior no se comporte de forma distinta (p.ej. que dejar de sonar,
o que el fork se cierre) cuando quien contesta es el bridge en vez de la
app o el propio monitor. Hasta confirmar esto en una llamada real, trata el
contestador como una función en validación, no como reemplazo garantizado
del monitor interior.

## 6. Hablar (F2)

Con F1 ya contestando/colgando, esta sección activa el sentido que faltaba:
que el visitante te oiga por la placa hablando desde HA. **Hazlo solo
después de haber validado F1 en vivo** (contestar/colgar funcionando de
verdad contra el device real).

### 6.1 Activar `talk_enabled` en el device y reiniciar

Igual que en el paso 3, edita a mano el `config.yaml` **del device**
(`deploy-standard.sh` tampoco sube esto) y pon `talk_enabled: true` dentro
del mismo bloque `sip.answerer`:

```yaml
sip:
  answerer:
    enabled: true
    talk_enabled: true       # <-- F2: activa el sender de talk-back
    # ... resto del bloque sin cambios ...
```

Reinicia el bridge:

```bash
ssh bticino "/home/bticino/cfg/extra/start-bridge.sh restart"
```

En el log de arranque, tras la línea del contestador, deberías ver:

```
[ANSWERER] contestador SIP activo (bajo demanda desde HA)
[TALK] talk-back activo (hablar desde HA durante la llamada)
```

Si en cambio ves `[TALK] Start falló: ...`, el contestador sigue funcionando
con normalidad (oír/ver la puerta no se ve afectado) pero el sender de
talk-back no se suscribió al bus; revisa el motivo del error en el log antes
de seguir.

### 6.2 Backchannel de go2rtc → `127.0.0.1:40004` (no tocar)

go2rtc del device **ya tiene cableado** el backchannel `exec:` que, cuando
alguien pulsa el micro en la tarjeta WebRTC, envía RTP speex sin cifrar a
`127.0.0.1:40004` (ver `docs/GO2RTC_INTEGRATION.md` /
`docs/WEBRTC_RTSP_STREAMING.md` para la línea exacta del `go2rtc.yaml`). Esta
sección **no cambia esa configuración**; el bridge solo consume ese extremo.

Nuestro `TalkSender` **BIND-ea `127.0.0.1:40004` como receptor** (vía el
`ffmpeg` que lanza en `doorcall.answered`). Antes de dar por bueno el gate en
vivo, confirma que **ningún otro proceso** tiene ese puerto tomado — por
ejemplo, si dejaste corriendo a mano un `ffmpeg`/`nc` de pruebas de una
sesión anterior de reverse engineering:

```bash
ssh bticino "netstat -lun | grep 40004"
```

Si aparece un proceso que no sea el `ffmpeg` que acaba de lanzar el bridge,
mátalo antes de probar — si no, el bind del `TalkSender` puede fallar (se
loguearía como `[TALK] Start falló`, sin tumbar el contestador) o competir
por los paquetes con el proceso previo.

### 6.3 Botón de micro en la tarjeta de Home Assistant

La tarjeta WebRTC de go2rtc (`portero-bticino`, la misma que ya sirve
`/doorbell` vídeo+audio) trae de serie el botón de micro cuando el stream
tiene un backchannel configurado — no hace falta YAML adicional para esto,
solo que el iframe/tarjeta de la sección 4 apunte al stream con backchannel
habilitado en `go2rtc.yaml`.

### 6.4 Gate en vivo F2 (tras validar F1)

1. Timbre pulsado en la puerta → `button.bticino_contestar` desde HA.
2. Confirmar que se oye/ve la calle (F1 ya validado).
3. Pulsar el botón de micro de la tarjeta WebRTC y hablar.
4. Confirmar **de viva voz junto a la placa** (o con alguien al otro lado)
   que el visitante te oye por el altavoz de la puerta.

Si no se oye nada (o se oye entrecortado/con mucho delay), antes de tocar
código revisa:

- **Buffering de ffmpeg:** el sender ya usa `-fflags nobuffer`; si el
  problema es en el lado de *entrada* (el backchannel de go2rtc tarda en
  arrancar o llega en ráfagas), prueba a añadir `-probesize 32` /
  `-analyzeduration 0` a `buildFFmpegArgs` (`pkg/sip/talk_sender.go`) para
  que ffmpeg no espere a acumular datos antes de reenviar.
- **Payload type:** el sender asume que el PT de salida (el que anunciamos
  en nuestro SDP de answer, hoy `97` en el wiring de `cmd/main.go`) coincide
  con lo que la placa espera para speex/8000. Si el portero no reproduce
  nada pese a que el ffmpeg está vivo y reenviando paquetes (verificable con
  `tcpdump` en el propio device hacia el puerto de audio del answer), es el
  primer sospechoso a revisar — comprueba el PT negociado en el SDP real del
  `INVITE`/`200 OK` de esa llamada.
- **Puerto/clave SRTP:** el destino y la clave SRTP los saca el `TalkSender`
  del payload de `doorcall.answered` (`media_host`/`media_port`/`srtp_key`),
  no están hardcodeados; si la llamada se contestó pero esos campos vinieran
  vacíos por algún cambio futuro en el emisor del evento, el sender no
  lanzaría ffmpeg en absoluto (fail-closed) — revisar el log `[TALK]` para
  distinguir "no arrancó" de "arrancó pero no se oye".
