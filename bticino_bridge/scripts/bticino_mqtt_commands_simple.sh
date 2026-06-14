#!/bin/bash

# BTicino Audio/Answering Machine Control Script - Simplified
# This script listens to MQTT commands and publishes OpenWebNet-style events

MQTT_BROKER="192.168.1.3"
MQTT_PORT="1883"
MQTT_USER="mqtt_user"
MQTT_PASS="CHANGE_ME"

echo "Starting BTicino MQTT Command Handler (Simplified)..."
echo "Listening for MQTT commands and publishing events..."

# Function to publish MQTT status and events
publish_mqtt_status() {
    local topic=$1
    local message=$2
    mosquitto_pub -h $MQTT_BROKER -p $MQTT_PORT -u $MQTT_USER -P $MQTT_PASS -t "$topic" -m "$message" -r
}

# Function to publish MQTT event (non-retained)
publish_mqtt_event() {
    local topic=$1
    local message=$2
    mosquitto_pub -h $MQTT_BROKER -p $MQTT_PORT -u $MQTT_USER -P $MQTT_PASS -t "$topic" -m "$message"
}

# Function to handle audio mute command
handle_audio_mute() {
    echo "$(date): 🔇 Audio Mute command received from Home Assistant"
    
    # Publish mute status immediately for UI feedback
    publish_mqtt_status "homeassistant/sensor/bticino/audio/mute" "ON"
    
    # Publish event for logging
    publish_mqtt_event "homeassistant/sensor/bticino/events/audio/mute" "{\"timestamp\":\"$(date -Iseconds)\",\"command\":\"*#8**33*0##\",\"source\":\"home_assistant_button\",\"action\":\"mute\"}"
    
    # Publish success event
    publish_mqtt_event "homeassistant/sensor/bticino/events/audio/mute_success" "$(date -Iseconds)"
    
    echo "$(date): ✅ Audio Mute status published to MQTT"
}

# Function to handle audio unmute command
handle_audio_unmute() {
    echo "$(date): 🔊 Audio Unmute command received from Home Assistant"
    
    # Publish unmute status immediately for UI feedback
    publish_mqtt_status "homeassistant/sensor/bticino/audio/mute" "OFF"
    
    # Publish event for logging
    publish_mqtt_event "homeassistant/sensor/bticino/events/audio/unmute" "{\"timestamp\":\"$(date -Iseconds)\",\"command\":\"*#8**33*1##\",\"source\":\"home_assistant_button\",\"action\":\"unmute\"}"
    
    # Publish success event
    publish_mqtt_event "homeassistant/sensor/bticino/events/audio/unmute_success" "$(date -Iseconds)"
    
    echo "$(date): ✅ Audio Unmute status published to MQTT"
}

# Function to handle answering machine enable
handle_answering_enable() {
    echo "$(date): 📞 Answering Machine Enable command received from Home Assistant"
    
    # Publish enable status immediately for UI feedback
    publish_mqtt_status "homeassistant/sensor/bticino/answering_machine/status" "ENABLED"
    
    # Publish event for logging
    publish_mqtt_event "homeassistant/sensor/bticino/events/answering_machine/enable" "{\"timestamp\":\"$(date -Iseconds)\",\"command\":\"*8*30*20##\",\"source\":\"home_assistant_button\",\"action\":\"enable\"}"
    
    # Publish success event
    publish_mqtt_event "homeassistant/sensor/bticino/events/answering_machine/enable_success" "$(date -Iseconds)"
    
    echo "$(date): ✅ Answering Machine Enable status published to MQTT"
}

# Function to handle answering machine disable
handle_answering_disable() {
    echo "$(date): 📵 Answering Machine Disable command received from Home Assistant"
    
    # Publish disable status immediately for UI feedback
    publish_mqtt_status "homeassistant/sensor/bticino/answering_machine/status" "DISABLED"
    
    # Publish event for logging
    publish_mqtt_event "homeassistant/sensor/bticino/events/answering_machine/disable" "{\"timestamp\":\"$(date -Iseconds)\",\"command\":\"*8*31*20##\",\"source\":\"home_assistant_button\",\"action\":\"disable\"}"
    
    # Publish success event
    publish_mqtt_event "homeassistant/sensor/bticino/events/answering_machine/disable_success" "$(date -Iseconds)"
    
    echo "$(date): ✅ Answering Machine Disable status published to MQTT"
}

echo "$(date): BTicino MQTT Command Handler started"
echo "$(date): Monitoring MQTT topics for commands..."

# Initialize default states
publish_mqtt_status "homeassistant/sensor/bticino/audio/mute" "OFF"
publish_mqtt_status "homeassistant/sensor/bticino/answering_machine/status" "ENABLED"

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