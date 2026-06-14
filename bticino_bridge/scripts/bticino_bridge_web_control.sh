#!/bin/bash

# BTicino Bridge Web Control Script
# Enhanced version with web dashboard support

BRIDGE_BINARY="./bticino_mqtt_bridge_web_v1717_arm"
CONFIG_FILE="config_production_homekit.yaml"
PID_FILE="bticino_bridge.pid"
LOG_FILE="bticino_bridge_web.log"

start() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "Bridge is already running (PID: $(cat $PID_FILE))"
        return 1
    fi
    
    echo "Starting BTicino Bridge with Web Dashboard..."
    nohup $BRIDGE_BINARY --config "$CONFIG_FILE" --log-level info > "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    
    sleep 2
    
    if kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "Bridge started successfully (PID: $(cat $PID_FILE))"
        echo "Web Dashboard: http://192.168.1.38:8082"
        echo "HomeKit Bridge: Port 8081 (PIN: 19371287)"
        return 0
    else
        echo "Failed to start bridge"
        rm -f "$PID_FILE"
        return 1
    fi
}

stop() {
    if [ ! -f "$PID_FILE" ]; then
        echo "Bridge is not running"
        return 0
    fi
    
    PID=$(cat "$PID_FILE")
    echo "Stopping BTicino Bridge (PID: $PID)..."
    
    if kill -TERM "$PID" 2>/dev/null; then
        # Wait for graceful shutdown
        for i in {1..10}; do
            if ! kill -0 "$PID" 2>/dev/null; then
                break
            fi
            sleep 1
        done
        
        # Force kill if still running
        if kill -0 "$PID" 2>/dev/null; then
            kill -KILL "$PID" 2>/dev/null
        fi
        
        rm -f "$PID_FILE"
        echo "Bridge stopped"
        return 0
    else
        echo "Failed to stop bridge or already stopped"
        rm -f "$PID_FILE"
        return 1
    fi
}

status() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        PID=$(cat "$PID_FILE")
        echo "Bridge is running (PID: $PID)"
        echo "Web Dashboard: http://192.168.1.38:8082"
        echo "HomeKit Bridge: Port 8081 (PIN: 19371287)"
        echo "MQTT Broker: 192.168.1.3:1883"
        echo "Log file: $LOG_FILE"
        return 0
    else
        echo "Bridge is not running"
        if [ -f "$PID_FILE" ]; then
            rm -f "$PID_FILE"
        fi
        return 1
    fi
}

restart() {
    stop
    sleep 2
    start
}

logs() {
    if [ -f "$LOG_FILE" ]; then
        tail -f "$LOG_FILE"
    else
        echo "Log file not found: $LOG_FILE"
        return 1
    fi
}

case "$1" in
    start)
        start
        ;;
    stop)
        stop
        ;;
    status)
        status
        ;;
    restart)
        restart
        ;;
    logs)
        logs
        ;;
    *)
        echo "Usage: $0 {start|stop|status|restart|logs}"
        echo ""
        echo "BTicino Bridge Web Interface Control"
        echo "  start   - Start the bridge with web dashboard"
        echo "  stop    - Stop the bridge"
        echo "  status  - Show bridge status"
        echo "  restart - Restart the bridge"
        echo "  logs    - Show real-time logs"
        echo ""
        echo "Web Dashboard will be available at: http://192.168.1.38:8082"
        exit 1
        ;;
esac
