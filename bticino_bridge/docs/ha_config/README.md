# Home Assistant Configuration for BTicino Video Streaming

## Prerequisites

1. **go2rtc installed and running** (see `docs/GO2RTC_INTEGRATION.md`)
2. **bticino_bridge v0.11.5+ running** with SIP and RTSP enabled
3. **Home Assistant 2024.11+** (for native go2rtc integration)

---

## Option 1: Native go2rtc Integration (Recommended)

### Add to `configuration.yaml`:

```yaml
# Home Assistant Configuration for BTicino

# go2rtc integration (HA 2024.11+)
go2rtc:
  streams:
    videoportero: rtsp://192.168.1.38:6554/doorbell

# Camera entity
camera:
  - name: Videoportero
    icon: mdi:doorbell-video
    unique_id: bticino_videoportero_cam
    stream_source: go2rtc://videoportero

# Sensor to show stream status
sensor:
  - platform: mqtt
    name: Videoportero Stream Status
    state_topic: "bticino/events/door"
    value_template: >-
      {% if value_json.event == "doorbell_pressed" %} active {% else %} idle {% endif %}
    icon: mdi:doorbell-video

# Automation to notify when someone rings
automation:
  - alias: "Videoportero - Notify when someone rings"
    trigger:
      - platform: mqtt
        topic: "bticino/events/door"
    condition:
      - condition: template
        value_template: "{{ 'doorbell_pressed' in trigger.payload }}"
    action:
      - service: notify.mobile_app_phone
        data:
          title: "Doorbell"
          message: "Someone is at the door!"
          data:
            entity_id: camera.videoportero
            clickAction: /dashboard-cameras
```

---

## Option 2: Manual WebRTC Configuration

### Camera with RTSPtoWebRTC proxy

```yaml
camera:
  - name: Videoportero
    icon: mdi:doorbell-video
    unique_id: bticino_videoportero_cam
    still_image_url: http://192.168.1.38:1984/apiframe/videoportero?jpg
    stream_source: >-
      rtsp://192.168.1.38:1984/stream/videoportero
```

### Using Picture Entity Card

```yaml
type: picture-entity
entity: camera.videoportero
name: Videoportero
camera_image: camera.videoportero
```

---

## Option 3: With Picture Glance (Dashboard Card)

```yaml
type: custom: Picture Glance Card
entities:
  - entity: sensor.videoportero_stream_status
    icon: mdi:doorbell
    name: Timbre
  - entity: lock.bticino_main_door
    icon: mdi:door
    name: Puerta
camera_entity: camera.videoportero
title: Videoportero
image: >-
  /api/camera_proxy/camera.videoportero?t={{ states.sensor.date_time_last_changed.attributes.timestamp }}
```

---

## Lovelace Cards Configuration

### Card 1: Video with Controls

```yaml
type: horizontal-stack
cards:
  - type: picture-entity
    entity: camera.videoportero
    name: Videoportero
    camera_view: live
    show_state: true
    tap_action:
      action: more-info
    hold_action:
      action: none

  - type: entities
    entities:
      - entity: binary_sensor.bticino_timbre
        icon: mdi:bell-ring
        name: Timbre
      - entity: lock.bticino_main_door
        icon: mdi:door
        name: Puerta Principal
      - entity: switch.bticino_voicemail
        icon: mdi:voicemail
        name: Contestador
      - entity: sensor.bticino_temperature
        icon: mdi:thermometer
        name: Temperatura
```

### Card 2: With Custom WebRTC Camera

```yaml
type: custom:webrtc-camera
entity: camera.videoportero
name: Videoportero
url: rtsp://192.168.1.38:1984/stream/videoportero
```

---

## Notifications Configuration

### Mobile App Notification with Camera Preview

```yaml
automation:
  - alias: "Doorbell - Notify with camera"
    id: doorbell_notify
    trigger:
      - platform: mqtt
        topic: "bticino/events/door"
        payload: '"doorbell_pressed"'
    action:
      - service: notify.mobile_app_phone
        data:
          title: "🚪 Videoportero"
          message: "Someone is at the door!"
          data:
            tag: doorbell-videoportero
            priority: high
            ttl: 0
            image: >-
              http://192.168.1.38:1984/apiframe/videoportero?jpg
            actions:
              - action: "UNLOCK"
                title: "Abrir Puerta"
                uri: "lovelace/0"
              - action: "VIEW"
                title: "Ver Video"
                uri: "camera://camera.videoportero"
```

---

## Door Unlock Button

```yaml
button:
  - platform: mqtt
    name: Abrir Puerta
    command_topic: "bticino/commands/door"
    payload_press: "unlock"
    icon: mdi:door-open

# Or using lock entity
lock:
  - platform: mqtt
    name: Puerta Principal
    state_topic: "bticino/status/door"
    command_topic: "bticino/commands/door"
    payload_lock: "lock"
    payload_unlock: "unlock"
    value_template: >-
      {% if value_json.locked %} locked {% else %} unlocked {% endif %}
    icon: mdi:door
```

---

## Full Dashboard Configuration

Create a new dashboard with these cards:

```yaml
title: Videoportero
icon: mdi:doorbell-video
cards:
  - type: vertical-stack
    cards:
      # Live Camera Feed
      - type: picture-entity
        entity: camera.videoportero
        name: Videoportero - Vista en Vivo
        camera_view: live
        aspect_ratio: 16/9
        show_state: false

      # Controls Row
      - type: horizontal-stack
        cards:
          - type: button
            name: Abrir
            icon: mdi:door-open
            tap_action:
              action: call-service
              service: mqtt.publish
              data:
                topic: bticino/commands/door
                payload: unlock
          - type: button
            name: Llamar
            icon: mdi:phone
            tap_action:
              action: more-info
          - type: button
            name: Captura
            icon: mdi:camera
            tap_action:
              action: call-service
              service: camera.snapshot
              data:
                entity_id: camera.videoportero
                filename: /config/www/snapshots/videoportero_{{ now().strftime("%Y%m%d_%H%M%S") }}.jpg

  # Status and Info
  - type: entities
    title: Estado
    entities:
      - entity: binary_sensor.bticino_timbre
        name: Timbre
      - entity: sensor.bticino_temperature
        name: Temperatura Dispositivo
      - entity: sensor.bticino_uptime
        name: Tiempo Activo
      - entity: binary_sensor.bticino_sip_registered
        name: SIP Registrado
```

---

## Troubleshooting

### Camera not showing stream

1. Check go2rtc is running: `ssh bticino './start_go2rtc.sh status'`
2. Check stream URL: `curl http://192.168.1.38:1984/api/streams`
3. Check firewall: `ssh bticino 'iptables -L'`

### High latency

1. Use WebRTC instead of HLS
2. Reduce quality in intercom settings
3. Use TCP transport:

```yaml
go2rtc:
  streams:
    videoportero: rtsp://192.168.1.38:6554/doorbell#transport=tcp
```

### Audio not working

1. Verify Speex codec support
2. Check backchannel configuration in go2rtc
3. Try without audio first:

```yaml
go2rtc:
  streams:
    videoportero: rtsp://192.168.1.38:6554/doorbell#video=copy#audio=off
```
