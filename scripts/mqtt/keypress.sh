#!/bin/sh

CFGFILE=/etc/tcpdump2mqtt/TcpDump2Mqtt.conf

# Check config file
if ! ([ -f $CFGFILE ] && [ -s $CFGFILE ]); then
	echo 'Configuration file missing or empty!'
	exit 1
fi

# Import configs
set -a
. $CFGFILE
set +a

# Define the input event device (replace /dev/input/eventX with your device)
INPUT_DEVICE="/dev/input/event0"

# Check if the input device exists
if [ ! -e "$INPUT_DEVICE" ]; then
    echo "Input device $INPUT_DEVICE not found."
    exit 1
fi

# Use evtest to read input events from the device
evtest "$INPUT_DEVICE" | while read -r line; do
    # Process the input event here
    echo "Input event received: $line"
    
    # Extract event details (type, code, value) using awk or other methods
    event_type=$(echo "$line" | awk '{print $5}')
    event_code=$(echo "$line" | awk '{print $8}')
    event_value=$(echo "$line" | awk '{print $11}')
    # Clean variables
    key_info=
    draw=
    value=

    # Perform actions based on event_type and event_code
    case "$event_type-$event_code" in
        "1-2")
            key_info="KEY_1"
            draw="key"
            if [ "$event_value" -eq 1 ]; then
                echo "KEY_1 was pressed"
                value="pressed"
            else
                echo "KEY_1 was released"
                value="released"
            fi
            ;;
        "1-3")
            key_info="KEY_2"
            draw="star"
            if [ "$event_value" -eq 1 ]; then
                echo "KEY_2 was pressed"
                value="pressed"
            else
                echo "KEY_2 was released"
                value="released"
            fi
            ;;
        "1-4")
            key_info="KEY_3"
            draw="eye"
            if [ "$event_value" -eq 1 ]; then
                echo "KEY_3 was pressed"
                value="pressed"
            else
                echo "KEY_3 was released"
                value="released"
            fi
            ;;
        "1-5")
            key_info="KEY_4"
            draw="phone"
            if [ "$event_value" -eq 1 ]; then
                echo "KEY_4 was pressed"
                value="pressed"
            else
                echo "KEY_4 was released"
                value="released"
            fi
            ;;
    esac
    if ([ -n "${key_info}" ] && [ -n "${draw}" ] && [ -n "${value}" ]); then
        JSON_CONTENT=$(jq -n --arg key_info "$key_info" --arg draw "$draw" --arg value "$value" '{"key_info":$key_info,"draw":$draw,"value":$value}')
        # sent mqtt message
        if [ -n "${MQTT_USER}" ]; then
            mosquitto_pub -h "${MQTT_HOST}" -p "${MQTT_PORT}" -t "Bticino/key" -m "$JSON_CONTENT" -u "${MQTT_USER}" -P "${MQTT_PASS}"
        elif [ -n "${MQTT_CAFILE}" ]; then
            mosquitto_pub -h "${MQTT_HOST}" -p "${MQTT_PORT}" -t "Bticino/key" -m "$JSON_CONTENT" --cafile "${MQTT_CAFILE}" --cert "${MQTT_CERTFILE}" --key "${MQTT_KEYFILE}"
        else
            mosquitto_pub -h "${MQTT_HOST}" -p "${MQTT_PORT}" -t "Bticino/key" -m "$JSON_CONTENT"
        fi
    fi
done
