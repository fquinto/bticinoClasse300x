#!/bin/bash
# Deployment script for bticino_bridge v0.14.0
# Usage: ./deploy_v0.14.0.sh

set -e

DEVICE="bticino"
REMOTE_DIR="/home/bticino/cfg/extra"
LOCAL_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== BTicino Bridge v0.14.0 Deployment ==="
echo "Device: $DEVICE"
echo "Remote dir: $REMOTE_DIR"
echo ""

# Check if binary exists
if [ ! -f "$LOCAL_DIR/bticino_bridge_v0.14.0" ]; then
    echo "ERROR: Binary not found: $LOCAL_DIR/bticino_bridge_v0.14.0"
    exit 1
fi

echo "1. Checking device connectivity..."
if ! ssh -o ConnectTimeout=5 $DEVICE "echo 'OK'" >/dev/null 2>&1; then
    echo "ERROR: Cannot connect to device"
    exit 1
fi
echo "   ✓ Device reachable"

echo "2. Checking current bridge status..."
ssh $DEVICE "pkill -0 bticino_bridge && echo 'Running' || echo 'Not running'"
echo "   (No action needed - we will restart)"

echo "3. Stopping current bridge..."
ssh $DEVICE "pkill bticino_bridge || true"
sleep 2
echo "   ✓ Bridge stopped"

echo "4. Creating backup..."
BACKUP_NAME="bticino_bridge_backup_$(date +%Y%m%d_%H%M%S)"
ssh $DEVICE "cp $REMOTE_DIR/bticino_bridge $REMOTE_DIR/$BACKUP_NAME"
echo "   ✓ Backup: $BACKUP_NAME"

echo "5. Uploading new binary..."
scp "$LOCAL_DIR/bticino_bridge_v0.14.0" "$DEVICE:$REMOTE_DIR/bticino_bridge"
ssh $DEVICE "chmod +x $REMOTE_DIR/bticino_bridge"
echo "   ✓ Binary uploaded"

echo "6. Updating version file..."
echo "0.14.0" | ssh $DEVICE "cat > $REMOTE_DIR/VERSION"

echo "7. Starting new bridge..."
ssh $DEVICE "cd $REMOTE_DIR && nohup ./bticino_bridge -config config.yaml > /var/log/bticino_bridge.log 2>&1 &"
sleep 3

echo "8. Verifying bridge is running..."
if ssh $DEVICE "pkill -0 bticino_bridge"; then
    echo "   ✓ Bridge running"
else
    echo "   ✗ Bridge failed to start"
    ssh $DEVICE "tail -20 /var/log/bticino_bridge.log"
    exit 1
fi

echo ""
echo "=== Deployment Complete ==="
echo ""
echo "Features v0.14.0:"
echo "  - QML → Bridge sync (language, timezone, NTP)"
echo "  - Web UI new tabs: Device, Audio, Display"
echo "  - MQTT device config publishing (60s)"
echo "  - Home Assistant discovery entities"
echo "  - File watcher for real-time sync"
echo ""
echo "Test commands:"
echo "  curl http://192.168.1.38:8082/api/config/device"
echo "  curl http://192.168.1.38:8082/settings"
echo ""
echo "MQTT topics to watch:"
echo "  bticino/system/language"
echo "  bticino/audio/ringtone/s0"
echo "  bticino/display/brightness"