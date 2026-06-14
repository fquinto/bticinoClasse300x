#!/bin/bash
# Test script to verify MQTT device config topics
# Usage: ./test_mqtt_device_config.sh

BROKER="${1:-localhost}"
PORT="${2:-1883}"
TOPIC_PREFIX="${3:-bticino}"

echo "=== MQTT Device Config Test ==="
echo "Broker: $BROKER:$PORT"
echo "Topic prefix: $TOPIC_PREFIX"
echo ""

# Subscribe to all device config topics
echo "Subscribing to device config topics..."

# Use mosquitto_sub if available, otherwise use generic test
if command -v mosquitto_sub &> /dev/null; then
    echo "Using mosquitto_sub..."
    mosquitto_sub -h "$BROKER" -p "$PORT" -t "$TOPIC_PREFIX/+/+" -v -C 5 2>/dev/null || echo "Timeout or no messages"
else
    echo "mosquitto_sub not found, using curl to test API instead"
    
    echo ""
    echo "=== Testing API Endpoints ==="
    echo "1. /api/config/device:"
    curl -s "http://localhost:8082/api/config/device" | head -100
    
    echo ""
    echo "2. /api/config/language:"
    curl -s "http://localhost:8082/api/config/language"
    
    echo ""
    echo "3. /api/config/ringtones:"
    curl -s "http://localhost:8082/api/config/ringtones"
    
    echo ""
    echo "4. /api/config/volumes:"
    curl -s "http://localhost:8082/api/config/volumes"
    
    echo ""
    echo "5. /api/config/display:"
    curl -s "http://localhost:8082/api/config/display"
    
    echo ""
    echo "6. /api/config/cameras:"
    curl -s "http://localhost:8082/api/config/cameras"
    
    echo ""
    echo "7. /api/config/answering:"
    curl -s "http://localhost:8082/api/config/answering"
fi

echo ""
echo "=== Expected MQTT Topics ==="
echo "bticino/system/language"
echo "bticino/system/timezone"
echo "bticino/system/datetime"
echo "bticino/system/device"
echo "bticino/answering/state"
echo "bticino/audio/ringtone/s0"
echo "bticino/audio/ringtone/door"
echo "bticino/audio/volume/s0"
echo "bticino/audio/volume/door"
echo "bticino/display/brightness"
echo "bticino/camera/20/config"
echo ""
echo "=== Test Complete ==="