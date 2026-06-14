# BTicino Classe 300X - Home Assistant Integration Guide

## Prerequisites

1. **Running BTicino Bridge**: The enhanced bridge must be running on your BTicino device
2. **MQTT Broker**: Home Assistant MQTT broker or external broker (like Mosquitto)
3. **Home Assistant**: Version 2023.1 or newer with MQTT integration enabled

## Quick Setup

### 1. Configure MQTT in Home Assistant

Add to your `configuration.yaml`:

```yaml
mqtt:
  broker: localhost  # or your MQTT broker IP
  port: 1883
  username: your_username  # optional
  password: your_password  # optional
  discovery: true
  discovery_prefix: homeassistant
```

### 2. Configure BTicino Bridge

Update your BTicino bridge `config.yaml`:

```yaml
mqtt:
  enabled: true
  host: "your_ha_ip_address"  # Home Assistant IP
  port: 1883
  username: "your_username"   # optional
  password: "your_password"   # optional
  client_id: "bticino_bridge"
  topic_prefix: "bticino"
```

### 3. Add Device Configuration

Add the contents of `discovery.yaml` to your Home Assistant `configuration.yaml`.

### 4. Restart Home Assistant

After adding the configuration, restart Home Assistant to load the new entities.

## Manual Entity Configuration

If auto-discovery doesn't work, manually add entities:

### Door Lock

```yaml
lock:
  - platform: mqtt
    name: "BTicino Main Door"
    command_topic: "bticino/openwebnet/commands"
    state_topic: "bticino/openwebnet/status/door/main"
    payload_lock: "*8*19*20##"
    payload_unlock: "*8*19*20##"
```

### Doorbell Button

```yaml
binary_sensor:
  - platform: mqtt
    name: "BTicino Doorbell"
    state_topic: "bticino/input/buttons"
    payload_on: "KEY_1"
    payload_off: ""
    device_class: "occupancy"
```

### System Status

```yaml
sensor:
  - platform: mqtt
    name: "BTicino Connection"
    state_topic: "bticino/system/connection"
    value_template: "{{ value }}"
```

## Available MQTT Topics

### Command Topics (send commands TO BTicino):
- `bticino/openwebnet/commands` - Send OpenWebNet commands
- `bticino/security/alerts` - Enable/disable security alerts

### Status Topics (receive data FROM BTicino):
- `bticino/input/buttons` - Hardware button presses (KEY_1, KEY_2, KEY_3, KEY_4)
- `bticino/input/screen` - Screen state (ON/OFF)
- `bticino/openwebnet/status/door/main` - Main door status
- `bticino/openwebnet/status/door/secondary` - Secondary door status
- `bticino/openwebnet/status/audio/channel1` - Audio channel 1 status
- `bticino/openwebnet/status/audio/channel2` - Audio channel 2 status
- `bticino/sip/status` - SIP connection status
- `bticino/system/connection` - Bridge connection status
- `bticino/system/temperature` - System temperature (if available)

## Common OpenWebNet Commands

### Door Control:
- `*8*19*20##` - Open/close main door
- `*8*19*11##` - Open/close secondary door
- `*#1013**1##` - Get door status

### Audio System:
- `*#8**35*1*0*0##` - Get audio channel 1 status
- `*#8**35*2*0*0##` - Get audio channel 2 status

### Video System:
- `*7*77#800#480#2500##` - Start video stream (800x480, 2500kbps)
- `*7*0##` - Stop video stream

### System Status:
- `*99*0##` - General status request
- `*#130**1*2##` - System diagnostics

## Lovelace Dashboard Example

```yaml
type: vertical-stack
cards:
  - type: horizontal-stack
    cards:
      - type: button
        tap_action:
          action: call-service
          service: lock.unlock
          service_data:
            entity_id: lock.bticino_main_door
        name: Open Door
        icon: mdi:door-open
      - type: entity
        entity: lock.bticino_main_door
        name: Door Status

  - type: horizontal-stack
    cards:
      - type: entity
        entity: binary_sensor.bticino_doorbell
        name: Doorbell
      - type: entity
        entity: binary_sensor.bticino_screen
        name: Screen
      - type: entity
        entity: sensor.bticino_connection_status
        name: Connection

  - type: entities
    entities:
      - binary_sensor.bticino_button_1
      - binary_sensor.bticino_button_2
      - binary_sensor.bticino_button_3
      - binary_sensor.bticino_button_4
    title: Hardware Buttons

  - type: conditional
    conditions:
      - entity: camera.bticino_camera
        state_not: "unavailable"
    card:
      type: picture-entity
      entity: camera.bticino_camera
      name: BTicino Camera
```

## Troubleshooting

### No Entities Appearing:
1. Check MQTT broker connection in HA logs
2. Verify BTicino bridge is publishing to MQTT
3. Check topic names match configuration
4. Enable MQTT discovery in Home Assistant

### Commands Not Working:
1. Check command topic is correct: `bticino/openwebnet/commands`
2. Verify OpenWebNet commands are properly formatted
3. Check BTicino bridge logs for safety manager blocks
4. Ensure critical commands are enabled in bridge config

### Button Events Not Received:
1. Check input monitoring is enabled in bridge
2. Verify `/dev/input/event*` permissions on BTicino device
3. Check bridge logs for input device errors
4. Test with: `mosquitto_sub -h localhost -t "bticino/input/buttons"`

### Testing MQTT:

```bash
# Subscribe to all BTicino topics
mosquitto_sub -h your_broker_ip -t "bticino/#"

# Send door open command
mosquitto_pub -h your_broker_ip -t "bticino/openwebnet/commands" -m "*8*19*20##"

# Test button simulation
mosquitto_pub -h your_broker_ip -t "bticino/input/buttons" -m "KEY_1"
```

## Advanced Features

### Security Integration:
- Use `bticino/security/alerts` topic for security system integration
- Set up automations for night-time door access alerts
- Monitor connection status for security system backup activation

### Voice Control (Alexa/Google):
- Expose lock entities for voice control
- Create scenes for "unlock door", "check door status"
- Use input_boolean helpers for voice-activated modes

### Mobile App Integration:
- Set up notifications for doorbell presses
- Create actionable notifications for door unlock requests
- Use camera snapshots in doorbell notifications

## Support

- Check BTicino bridge logs: `journalctl -u bticino-bridge -f`
- Enable debug logging in bridge config for detailed troubleshooting
- Monitor MQTT traffic during troubleshooting
- Review Home Assistant MQTT integration logs