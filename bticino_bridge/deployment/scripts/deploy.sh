#!/bin/bash
# BTicino Bridge Production Deployment Script
# Usage: ./deploy.sh [device_ip] [target_user]

set -euo pipefail

DEVICE_IP="${1:-192.168.1.38}"
TARGET_USER="${2:-root2}"
TARGET_DIR="/home/bticino/cfg/extra"
SERVICE_NAME="bticino-bridge"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# SSH options for BTicino device compatibility
SSH_OPTS="-o ConnectTimeout=10 -o PubkeyAcceptedKeyTypes=+ssh-rsa -o HostKeyAlgorithms=+ssh-rsa"

# Pre-deployment checks
check_prerequisites() {
    log_info "Checking deployment prerequisites..."
    
    # Check if binary exists
    if [[ ! -f "./bticino-bridge" ]]; then
        log_error "Binary './bticino-bridge' not found. Run build script first."
        exit 1
    fi
    
    # Check if config exists
    if [[ ! -f "./configs/config.yaml" ]]; then
        log_error "Configuration file './configs/config.yaml' not found."
        exit 1
    fi
    
    # Test SSH connectivity
    if ! ssh $SSH_OPTS -o BatchMode=yes "${TARGET_USER}@${DEVICE_IP}" exit 2>/dev/null; then
        log_error "Cannot connect to ${TARGET_USER}@${DEVICE_IP}. Check SSH access."
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Create bticino user if needed
setup_user() {
    log_info "Setting up bticino user on target device..."
    
    ssh $SSH_OPTS "${TARGET_USER}@${DEVICE_IP}" bash << 'EOF'
        # Create bticino user if doesn't exist
        if ! id bticino >/dev/null 2>&1; then
            useradd -r -s /bin/bash -d /home/bticino -m bticino
            usermod -a -G input,dialout bticino
            echo "Created bticino user"
        else
            echo "User bticino already exists"
        fi
        
        # Ensure proper permissions for input devices
        chmod 644 /dev/input/event* 2>/dev/null || true
        
        # Create log directory
        mkdir -p /var/log/bticino
        chown bticino:bticino /var/log/bticino
EOF
    
    log_success "User setup completed"
}

# Deploy application files
deploy_application() {
    log_info "Deploying BTicino Bridge application..."
    
    # Stop existing service if running
    ssh $SSH_OPTS "${TARGET_USER}@${DEVICE_IP}" "systemctl stop ${SERVICE_NAME} 2>/dev/null || true"
    
    # Create target directory
    ssh $SSH_OPTS "${TARGET_USER}@${DEVICE_IP}" "mkdir -p ${TARGET_DIR}/{configs,logs,data}"
    
    # Copy application files
    log_info "Copying binary..."
    scp $SSH_OPTS ./bticino-bridge "${TARGET_USER}@${DEVICE_IP}:${TARGET_DIR}/"
    
    log_info "Copying configuration..."
    scp $SSH_OPTS ./configs/config.yaml "${TARGET_USER}@${DEVICE_IP}:${TARGET_DIR}/configs/"
    
    # Copy systemd service
    log_info "Installing systemd service..."
    scp $SSH_OPTS ./deployment/systemd/bticino-bridge.service "${TARGET_USER}@${DEVICE_IP}:/etc/systemd/system/"
    
    # Set permissions
    ssh $SSH_OPTS "${TARGET_USER}@${DEVICE_IP}" bash << 'EOF'
        chown -R bticino:bticino /home/bticino/cfg/extra/
        chmod +x /home/bticino/cfg/extra/bticino-bridge
        chmod 644 /etc/systemd/system/bticino-bridge.service
EOF
    
    log_success "Application deployment completed"
}

# Configure and start service
configure_service() {
    log_info "Configuring and starting BTicino Bridge service..."
    
    ssh $SSH_OPTS "${TARGET_USER}@${DEVICE_IP}" bash << 'EOF'
        # Reload systemd
        systemctl daemon-reload
        
        # Enable service
        systemctl enable bticino-bridge
        
        # Start service
        systemctl start bticino-bridge
        
        # Wait for service to start
        sleep 3
        
        # Check service status
        if systemctl is-active --quiet bticino-bridge; then
            echo "✅ Service started successfully"
            systemctl status bticino-bridge --no-pager -l
        else
            echo "❌ Service failed to start"
            journalctl -u bticino-bridge --no-pager -l -n 20
            exit 1
        fi
EOF
    
    log_success "Service configuration completed"
}

# Verify deployment
verify_deployment() {
    log_info "Verifying deployment..."
    
    # Test OpenWebNet connectivity
    ssh $SSH_OPTS "${TARGET_USER}@${DEVICE_IP}" bash << 'EOF'
        # Check if service is listening
        if netstat -ln | grep -q ":8080"; then
            echo "✅ Web interface accessible on port 8080"
        else
            echo "⚠️  Web interface not detected on port 8080"
        fi
        
        # Check logs for successful startup
        if journalctl -u bticino-bridge --since "1 minute ago" | grep -q "Bridge started successfully"; then
            echo "✅ Bridge started successfully according to logs"
        else
            echo "⚠️  Bridge startup not confirmed in logs"
        fi
        
        # Test basic OpenWebNet command
        echo "Testing OpenWebNet connectivity..."
        timeout 5 bash -c 'echo "*99*0##" | nc 127.0.0.1 20000' || echo "⚠️  OpenWebNet test failed"
EOF
    
    log_success "Deployment verification completed"
}

# Main deployment flow
main() {
    log_info "Starting BTicino Bridge deployment to ${DEVICE_IP}"
    log_info "Target user: ${TARGET_USER}"
    log_info "Target directory: ${TARGET_DIR}"
    echo
    
    check_prerequisites
    setup_user
    deploy_application
    configure_service
    verify_deployment
    
    echo
    log_success "🎉 BTicino Bridge deployment completed successfully!"
    log_info "Web interface: http://${DEVICE_IP}:8080"
    log_info "Service logs: ssh ${TARGET_USER}@${DEVICE_IP} 'journalctl -u ${SERVICE_NAME} -f'"
    log_info "Service control: ssh ${TARGET_USER}@${DEVICE_IP} 'systemctl [start|stop|restart|status] ${SERVICE_NAME}'"
}

# Handle script interruption
trap 'log_error "Deployment interrupted"; exit 1' INT TERM

# Run main function
main "$@"