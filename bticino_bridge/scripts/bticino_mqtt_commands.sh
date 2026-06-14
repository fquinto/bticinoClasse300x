#!/bin/bash

# BTicino Audio/Answering Machine Control Script
# This script listens to MQTT commands and sends OpenWebNet commands to the BTicino device

MQTT_BROKER="192.168.1.3"
MQTT_PORT="1883"
MQTT_USER="mqtt_user"
MQTT_PASS="CHANGE_ME"
BTICINO_IP="127.0.0.1"
BTICINO_PORT="20000"

echo "Starting BTicino MQTT Command Handler..."
echo "Listening for MQTT commands and forwarding to OpenWebNet..."

# Function to send OpenWebNet command
send_openwebnet_command() {
    local command=$1
    local description=$2
    echo "$(date): Sending OpenWebNet command: $command ($description)"
    
    # Send command via netcat to OpenWebNet gateway
    echo -n "$command" | nc -w 3 $BTICINO_IP $BTICINO_PORT
    
    if [ $? -eq 0 ]; then
        echo "$(date): ✅ OpenWebNet command sent successfully: $description"
        return 0
    else
        echo "$(date): ❌ Failed to send OpenWebNet command: $description"
        return 1
    fi
}

# Function to publish MQTT status
publish_mqtt_status() {
    local topic=$1
    local message=$2
    mosquitto_pub -h $MQTT_BROKER -p $MQTT_PORT -u $MQTT_USER -P $MQTT_PASS -t "$topic" -m "$message" -r
}

# Function to handle audio mute command
handle_audio_mute() {
    echo "$(date): 🔇 Audio Mute command received from Home Assistant"
    if send_openwebnet_command "*#8**33*0##" "Audio Mute"; then
        publish_mqtt_status "homeassistant/sensor/bticino/audio/mute" "ON"
        publish_mqtt_status "homeassistant/sensor/bticino/events/audio/mute_success" "$(date -Iseconds)"
    else
        publish_mqtt_status "homeassistant/sensor/bticino/events/audio/mute_failed" "$(date -Iseconds)"
    fi
}

# Function to handle audio unmute command
handle_audio_unmute() {
    echo "$(date): 🔊 Audio Unmute command received from Home Assistant"
    if send_openwebnet_command "*#8**33*1##" "Audio Unmute"; then
        publish_mqtt_status "homeassistant/sensor/bticino/audio/mute" "OFF"
        publish_mqtt_status "homeassistant/sensor/bticino/events/audio/unmute_success" "$(date -Iseconds)"
    else
        publish_mqtt_status "homeassistant/sensor/bticino/events/audio/unmute_failed" "$(date -Iseconds)"
    fi
}

# Function to handle answering machine enable
handle_answering_enable() {
    echo "$(date): 📞 Answering Machine Enable command received from Home Assistant"
    if send_openwebnet_command "*8*30*20##" "Answering Machine Enable"; then
        publish_mqtt_status "homeassistant/sensor/bticino/answering_machine/status" "ENABLED"
        publish_mqtt_status "homeassistant/sensor/bticino/events/answering_machine/enable_success" "$(date -Iseconds)"
    else
        publish_mqtt_status "homeassistant/sensor/bticino/events/answering_machine/enable_failed" "$(date -Iseconds)"
    fi
}

# Function to handle answering machine disable
handle_answering_disable() {
    echo "$(date): 📵 Answering Machine Disable command received from Home Assistant"
    if send_openwebnet_command "*8*31*20##" "Answering Machine Disable"; then
        publish_mqtt_status "homeassistant/sensor/bticino/answering_machine/status" "DISABLED"
        publish_mqtt_status "homeassistant/sensor/bticino/events/answering_machine/disable_success" "$(date -Iseconds)"
    else
        publish_mqtt_status "homeassistant/sensor/bticino/events/answering_machine/disable_failed" "$(date -Iseconds)"
    fi
}

echo "$(date): BTicino MQTT Command Handler started"
echo "$(date): Monitoring MQTT topics for commands..."

# Subscribe to MQTT command topics and handle them
{
    mosquitto_sub -h $MQTT_BROKER -p $MQTT_PORT -u $MQTT_USER -P $MQTT_PASS -t "homeassistant/sensor/bticino/commands/audio/mute" | while read -r message; do
        handle_audio_mute
    done &
    
    mosquitto_sub -h $MQTT_BROKER -p $MQTT_PORT -u $MQTT_USER -P $MQTT_PASS -t "homeassistant/sensor/bticino/commands/audio/unmute" | while read -r message; do
        handle_audio_unmute
    done &
    
    mosquitto_sub -h $MQTT_BROKER -p $MQTT_PORT -u $MQTT_USER -P $MQTT_PASS -t "homeassistant/sensor/bticino/commands/answering_machine/enable" | while read -r message; do
        handle_answering_enable
    done &
    
    mosquitto_sub -h $MQTT_BROKER -p $MQTT_PORT -u $MQTT_USER -P $MQTT_PASS -t "homeassistant/sensor/bticino/commands/answering_machine/disable" | while read -r message; do
        handle_answering_disable
    done &
    
    # Wait for all background jobs
    wait
}

echo "$(date): BTicino MQTT Command Handler stopped"