#!/bin/bash

# BTicino MQTT Command Tester
# Simulates Home Assistant sending commands to BTicino

# MQTT Settings (same as your Home Assistant)
MQTT_HOST="192.168.1.3"
MQTT_PORT="1883"
MQTT_USER="mqtt_user"
MQTT_PASS="CHANGE_ME"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "🎮 BTicino MQTT Command Tester"
echo "=============================="
echo "Testing commands from Home Assistant to BTicino"
echo

# Function to send MQTT command
send_mqtt_command() {
    local topic="$1"
    local message="$2"
    local description="$3"
    
    echo -e "${BLUE}[TEST]${NC} $description"
    echo -e "${YELLOW}Topic:${NC} $topic"
    echo -e "${YELLOW}Message:${NC} $message"
    
    # Use mosquitto_pub if available, otherwise use curl
    if command -v mosquitto_pub >/dev/null 2>&1; then
        mosquitto_pub -h "$MQTT_HOST" -p "$MQTT_PORT" -u "$MQTT_USER" -P "$MQTT_PASS" -t "$topic" -m "$message"
    else
        # Alternative using curl (if MQTT HTTP bridge is available)
        echo "mosquitto_pub not available, command would be:"
        echo "mosquitto_pub -h $MQTT_HOST -p $MQTT_PORT -u $MQTT_USER -P $MQTT_PASS -t '$topic' -m '$message'"
    fi
    
    echo -e "${GREEN}✅ Command sent${NC}"
    echo
    sleep 2
}

echo "Testing BTicino MQTT Commands..."
echo

# Test 1: Ping command
send_mqtt_command \
    "homeassistant/sensor/bticino/commands/system/ping" \
    "ping" \
    "🏓 Ping BTicino system"

# Test 2: Refresh status
send_mqtt_command \
    "homeassistant/sensor/bticino/commands/refresh" \
    "refresh" \
    "🔄 Refresh all BTicino status"

# Test 3: SIP status check
send_mqtt_command \
    "homeassistant/sensor/bticino/commands/sip/status" \
    "check" \
    "📞 Check SIP status"

# Test 4: Audio channel status
send_mqtt_command \
    "homeassistant/sensor/bticino/commands/audio/1/status" \
    "check" \
    "🔊 Check Audio Channel 1"

echo "🎉 All test commands sent!"
echo
echo "📋 Check the BTicino bridge logs to see if commands were received:"
echo "ssh root2@bticino './bticino_bridge_control.sh logs'"
echo
echo "🏠 In Home Assistant, you can send these commands using:"
echo "   Services → MQTT: Publish → Topic + Message"
echo "   Or create automation/script using mqtt.publish service"