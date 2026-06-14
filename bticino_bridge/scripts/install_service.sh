#!/bin/bash

# BTicino MQTT Bridge - Installation Script
# This script sets up the MQTT bridge as a systemd service on BTicino hardware

set -e

echo "🚀 BTicino MQTT Bridge Installation Script"
echo "=========================================="

# Configuration
SERVICE_NAME="bticino-mqtt-bridge"
INSTALL_DIR="/home/bticino/cfg/extra"
BINARY_NAME="bticino_mqtt_bridge_arm"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if we're running on BTicino
if [ ! -f "/etc/flexisip/flexisip.conf" ]; then
    log_warn "This doesn't appear to be BTicino hardware"
    read -p "Continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Check if binary exists
if [ ! -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
    log_error "Binary ${BINARY_NAME} not found in ${INSTALL_DIR}"
    log_info "Please deploy the binary first using: make deploy"
    exit 1
fi

# Make binary executable
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
log_info "Made binary executable"

# Create systemd service file
log_info "Creating systemd service..."

cat > "${SERVICE_FILE}" << EOF
[Unit]
Description=BTicino MQTT Bridge for Home Assistant
Documentation=https://github.com/bticino-bridge
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
ExecReload=/bin/kill -HUP \$MAINPID

# Restart configuration
Restart=always
RestartSec=10
StartLimitInterval=60s
StartLimitBurst=3

# Security settings (minimal for BTicino environment)
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${INSTALL_DIR}
ReadWritePaths=/var/log

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=bticino-mqtt-bridge

# Environment
Environment=HOME=${INSTALL_DIR}
Environment=USER=root

[Install]
WantedBy=multi-user.target
EOF

log_info "Service file created at ${SERVICE_FILE}"

# Reload systemd
systemctl daemon-reload
log_info "Systemd configuration reloaded"

# Enable service
systemctl enable "${SERVICE_NAME}"
log_info "Service enabled for automatic startup"

# Start service
systemctl start "${SERVICE_NAME}"
log_info "Service started"

# Wait a moment for service to start
sleep 3

# Check service status
if systemctl is-active --quiet "${SERVICE_NAME}"; then
    log_info "✅ Service is running successfully!"
    systemctl status "${SERVICE_NAME}" --no-pager -l
else
    log_error "❌ Service failed to start"
    log_info "Check logs with: journalctl -u ${SERVICE_NAME} -f"
    exit 1
fi

echo
echo "🎉 Installation completed successfully!"
echo
echo "📋 Useful commands:"
echo "  Start service:   systemctl start ${SERVICE_NAME}"
echo "  Stop service:    systemctl stop ${SERVICE_NAME}"
echo "  Restart service: systemctl restart ${SERVICE_NAME}"
echo "  View status:     systemctl status ${SERVICE_NAME}"
echo "  View logs:       journalctl -u ${SERVICE_NAME} -f"
echo "  Disable service: systemctl disable ${SERVICE_NAME}"
echo
echo "📊 Home Assistant Integration:"
echo "  - Device: BTicino Classe 300X"
echo "  - Entities: 11 sensors/binary_sensors"
echo "  - MQTT Topics: homeassistant/sensor/bticino/*"
echo "  - Commands: homeassistant/sensor/bticino/commands/*"
echo
log_info "The BTicino MQTT Bridge is now running and will start automatically on boot!"