#!/bin/sh

# BTicino MQTT Bridge - Simple Startup Script
# For BTicino systems without full systemd support

BRIDGE_DIR="/home/bticino/cfg/extra"
BRIDGE_BINARY="bticino_mqtt_bridge_arm"
BRIDGE_PID_FILE="/tmp/bticino_bridge.pid"
BRIDGE_LOG_FILE="/var/log/bticino_bridge.log"

# Colors for output  
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to start the bridge
start_bridge() {
    if [ -f "$BRIDGE_PID_FILE" ]; then
        PID=$(cat "$BRIDGE_PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            log_warn "Bridge is already running (PID: $PID)"
            return 0
        else
            log_info "Removing stale PID file"
            rm -f "$BRIDGE_PID_FILE"
        fi
    fi
    
    log_info "Starting BTicino MQTT Bridge..."
    cd "$BRIDGE_DIR"
    
    # Start bridge in background and save PID
    nohup "./$BRIDGE_BINARY" > "$BRIDGE_LOG_FILE" 2>&1 &
    echo $! > "$BRIDGE_PID_FILE"
    
    # Wait a moment and check if it's still running
    sleep 3
    PID=$(cat "$BRIDGE_PID_FILE")
    if kill -0 "$PID" 2>/dev/null; then
        log_info "✅ Bridge started successfully (PID: $PID)"
        log_info "📋 Log file: $BRIDGE_LOG_FILE"
        return 0
    else
        log_error "❌ Bridge failed to start"
        rm -f "$BRIDGE_PID_FILE"
        return 1
    fi
}

# Function to stop the bridge
stop_bridge() {
    if [ ! -f "$BRIDGE_PID_FILE" ]; then
        log_warn "Bridge is not running (no PID file found)"
        return 0
    fi
    
    PID=$(cat "$BRIDGE_PID_FILE")
    if kill -0 "$PID" 2>/dev/null; then
        log_info "Stopping BTicino MQTT Bridge (PID: $PID)..."
        kill -TERM "$PID"
        
        # Wait for graceful shutdown
        for i in 1 2 3 4 5; do
            if ! kill -0 "$PID" 2>/dev/null; then
                log_info "✅ Bridge stopped successfully"
                rm -f "$BRIDGE_PID_FILE"
                return 0
            fi
            sleep 1
        done
        
        # Force kill if still running
        log_warn "Force killing bridge..."
        kill -KILL "$PID" 2>/dev/null
        rm -f "$BRIDGE_PID_FILE"
        log_info "✅ Bridge force stopped"
    else
        log_info "Bridge was not running, removing PID file"
        rm -f "$BRIDGE_PID_FILE"
    fi
}

# Function to check status
status_bridge() {
    if [ ! -f "$BRIDGE_PID_FILE" ]; then
        log_info "❌ Bridge is not running (no PID file)"
        return 1
    fi
    
    PID=$(cat "$BRIDGE_PID_FILE")
    if kill -0 "$PID" 2>/dev/null; then
        log_info "✅ Bridge is running (PID: $PID)"
        
        # Show recent log entries
        echo "📋 Recent log entries:"
        tail -10 "$BRIDGE_LOG_FILE" 2>/dev/null || log_warn "No log file found"
        return 0
    else
        log_info "❌ Bridge is not running (stale PID file)"
        rm -f "$BRIDGE_PID_FILE"
        return 1
    fi
}

# Function to restart the bridge
restart_bridge() {
    log_info "Restarting BTicino MQTT Bridge..."
    stop_bridge
    sleep 2
    start_bridge
}

# Function to show logs
logs_bridge() {
    if [ -f "$BRIDGE_LOG_FILE" ]; then
        log_info "📋 Following bridge logs (Ctrl+C to exit):"
        tail -f "$BRIDGE_LOG_FILE"
    else
        log_error "No log file found at $BRIDGE_LOG_FILE"
        return 1
    fi
}

# Main script logic
case "$1" in
    start)
        start_bridge
        ;;
    stop)
        stop_bridge
        ;;
    restart)
        restart_bridge
        ;;
    status)
        status_bridge
        ;;
    logs)
        logs_bridge
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status|logs}"
        echo
        echo "BTicino MQTT Bridge Control Script"
        echo "Commands:"
        echo "  start   - Start the bridge service"
        echo "  stop    - Stop the bridge service" 
        echo "  restart - Restart the bridge service"
        echo "  status  - Show service status and recent logs"
        echo "  logs    - Follow live logs (Ctrl+C to exit)"
        echo
        exit 1
        ;;
esac

exit $?