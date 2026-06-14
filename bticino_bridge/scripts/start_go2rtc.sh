#!/bin/bash
# go2rtc startup script for BTicino intercom
# This script starts go2rtc which connects to bticino_bridge RTSP server

set -e

# Configuration
GO2RTC_DIR="/home/bticino/cfg/extra/go2rtc"
GO2RTC_BINARY="/home/bticino/cfg/extra/go2rtc"
CONFIG_FILE="$GO2RTC_DIR/go2rtc.yaml"
LOG_FILE="/tmp/go2rtc.log"
DEVICE_IP="192.168.1.38"

# Create config directory if not exists
mkdir -p "$GO2RTC_DIR"

# Check if bticino_bridge is running
check_bridge() {
    if ! pgrep -x "bticino-bridge" > /dev/null 2>&1; then
        echo "[go2rtc] WARNING: bticino-bridge is not running!"
        echo "[go2rtc] Please start bticino-bridge first"
        return 1
    fi
    return 0
}

# Generate default config if not exists
generate_config() {
    if [ ! -f "$CONFIG_FILE" ]; then
        echo "[go2rtc] Generating default configuration..."
        cat > "$CONFIG_FILE" << 'EOF'
# go2rtc configuration for BTicino Classe 300X
# This config connects to bticino_bridge RTSP server

streams:
  doorbell: rtsp://DEVICE_IP:6554/doorbell
  doorbell-video: rtsp://DEVICE_IP:6554/doorbell-video

# WebRTC configuration
webrtc:
  listen: :8555
  candidates:
    - DEVICE_IP:8555
    - stun:stun.l.google.com:19302

# API server (for Home Assistant)
api:
  listen: :1984

# RTSP server - disabled (we only use as client)
rtsp:
  listen: ""

# Logging
log:
  level: info
  format: text
EOF
        # Replace DEVICE_IP placeholder
        sed -i "s/DEVICE_IP/$DEVICE_IP/g" "$CONFIG_FILE"
        echo "[go2rtc] Configuration created at $CONFIG_FILE"
    else
        echo "[go2rtc] Using existing configuration at $CONFIG_FILE"
    fi
}

# Check for go2rtc binary
check_binary() {
    if [ ! -f "$GO2RTC_BINARY" ]; then
        echo "[go2rtc] Binary not found at $GO2RTC_BINARY"
        echo "[go2rtc] Please download go2rtc for ARM7:"
        echo "[go2rtc]   curl -L -o $GO2RTC_BINARY https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_arm"
        echo "[go2rtc]   chmod +x $GO2RTC_BINARY"
        return 1
    fi
    chmod +x "$GO2RTC_BINARY"
    return 0
}

# Start go2rtc
start_go2rtc() {
    echo "[go2rtc] Starting go2rtc..."
    echo "[go2rtc] Log file: $LOG_FILE"
    echo "[go2rtc] Config: $CONFIG_FILE"
    
    cd "$GO2RTC_DIR"
    nohup "$GO2RTC_BINARY" -config "$CONFIG_FILE" > "$LOG_FILE" 2>&1 &
    GO2RTC_PID=$!
    
    echo "[go2rtc] Started with PID: $GO2RTC_PID"
    echo "$GO2RTC_PID" > /var/run/go2rtc.pid
    
    # Wait a moment and check if still running
    sleep 2
    if kill -0 "$GO2RTC_PID" 2>/dev/null; then
        echo "[go2rtc] Successfully started"
        echo "[go2rtc] API: http://$DEVICE_IP:1984"
        echo "[go2rtc] WebRTC: http://$DEVICE_IP:1984/#webrtc"
        echo "[go2rtc] Streams: rtsp://$DEVICE_IP:6554/doorbell"
        return 0
    else
        echo "[go2rtc] Failed to start! Check $LOG_FILE"
        cat "$LOG_FILE"
        return 1
    fi
}

# Stop go2rtc
stop_go2rtc() {
    if [ -f /var/run/go2rtc.pid ]; then
        PID=$(cat /var/run/go2rtc.pid)
        if kill -0 "$PID" 2>/dev/null; then
            echo "[go2rtc] Stopping PID $PID..."
            kill "$PID"
            rm /var/run/go2rtc.pid
            echo "[go2rtc] Stopped"
        else
            echo "[go2rtc] Process not running"
            rm /var/run/go2rtc.pid
        fi
    else
        echo "[go2rtc] PID file not found, trying pgrep..."
        if pgrep -x "go2rtc" > /dev/null; then
            pkill -x "go2rtc"
            echo "[go2rtc] Stopped"
        else
            echo "[go2rtc] Not running"
        fi
    fi
}

# Status check
status() {
    if [ -f /var/run/go2rtc.pid ]; then
        PID=$(cat /var/run/go2rtc.pid)
        if kill -0 "$PID" 2>/dev/null; then
            echo "[go2rtc] Running with PID: $PID"
            return 0
        fi
    fi
    
    if pgrep -x "go2rtc" > /dev/null; then
        echo "[go2rtc] Running (PID: $(pgrep -x go2rtc))"
        return 0
    fi
    
    echo "[go2rtc] Not running"
    return 1
}

# Main
case "$1" in
    start)
        check_bridge || exit 1
        check_binary || exit 1
        generate_config
        start_go2rtc
        ;;
    stop)
        stop_go2rtc
        ;;
    restart)
        stop_go2rtc
        sleep 1
        check_bridge || exit 1
        check_binary || exit 1
        generate_config
        start_go2rtc
        ;;
    status)
        status
        ;;
    config)
        generate_config
        cat "$CONFIG_FILE"
        ;;
    logs)
        if [ -f "$LOG_FILE" ]; then
            tail -50 "$LOG_FILE"
        else
            echo "[go2rtc] No log file found"
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status|config|logs}"
        exit 1
        ;;
esac
