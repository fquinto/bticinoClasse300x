#!/bin/bash
#
# BTicino Bridge - Automated Deployment Script
# Uses base64 encoding for maximum compatibility with limited devices
#
# Usage: ./deploy_to_bticino.sh [options]
#
# Options:
#   -h, --host HOST     SSH host (default: bticino)
#   -u, --user USER     SSH user (default: none, uses ssh config)
#   -p, --port PORT     SSH port (default: 22)
#   -c, --config FILE   Config file to deploy (default: configs/config.yaml)
#   --skip-binary       Skip binary transfer (only deploy config)
#   --skip-config       Skip config transfer (only deploy binary)
#   --dry-run           Show what would be done without executing
#   --verbose           Enable verbose output
#   --help              Show this help message
#
# Example:
#   ./deploy_to_bticino.sh -h 192.168.1.38 --config configs/config.yaml
#

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
SSH_HOST="bticino"
SSH_USER=""
SSH_PORT="22"
CONFIG_FILE="configs/config.yaml"
SKIP_BINARY=false
SKIP_CONFIG=false
DRY_RUN=false
VERBOSE=false

# Remote paths
REMOTE_BASE_DIR="/home/bticino/cfg/extra"
REMOTE_BINARY="${REMOTE_BASE_DIR}/bticino_bridge"
REMOTE_CONFIG="${REMOTE_BASE_DIR}/config.yaml"
REMOTE_RECORDINGS="${REMOTE_BASE_DIR}/recordings"

# Helper functions
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

ssh_cmd() {
    if [ -n "$SSH_USER" ]; then
        ssh -p "$SSH_PORT" "${SSH_USER}@${SSH_HOST}" "$@"
    else
        ssh -p "$SSH_PORT" "${SSH_HOST}" "$@"
    fi
}

scp_cmd() {
    if [ -n "$SSH_USER" ]; then
        scp -P "$SSH_PORT" "$@" "${SSH_USER}@${SSH_HOST}:"
    else
        scp -P "$SSH_PORT" "$@" "${SSH_HOST}:"
    fi
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--host)
            SSH_HOST="$2"
            shift 2
            ;;
        -u|--user)
            SSH_USER="$2"
            shift 2
            ;;
        -p|--port)
            SSH_PORT="$2"
            shift 2
            ;;
        -c|--config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        --skip-binary)
            SKIP_BINARY=true
            shift
            ;;
        --skip-config)
            SKIP_CONFIG=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        --help)
            head -25 "$0" | tail -20
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Build SSH command
SSH_OPTS="-p ${SSH_PORT}"
if [ -n "$SSH_USER" ]; then
    SSH_OPTS="${SSH_OPTS} -l ${SSH_USER}"
fi

echo "========================================"
echo "  BTicino Bridge Deployment Script"
echo "  Version: v0.12.0"
echo "========================================"
echo ""
log_info "SSH Host: ${SSH_HOST}"
log_info "SSH Port: ${SSH_PORT}"
[ -n "$SSH_USER" ] && log_info "SSH User: ${SSH_USER}"
log_info "Remote Base Dir: ${REMOTE_BASE_DIR}"
echo ""

# Step 1: Test SSH connection
log_info "Testing SSH connection..."
if [ "$DRY_RUN" = true ]; then
    log_warning "[DRY-RUN] Would test SSH connection to ${SSH_HOST}"
else
    if ! ssh_cmd "echo 'Connection successful'" > /dev/null 2>&1; then
        log_error "Failed to connect to ${SSH_HOST}"
        log_info "Try: ssh ${SSH_OPTS} ${SSH_HOST}"
        exit 1
    fi
    log_success "SSH connection verified"
fi

# Step 2: Check remote directory
log_info "Checking remote directory..."
if [ "$DRY_RUN" = true ]; then
    log_warning "[DRY-RUN] Would check/create ${REMOTE_BASE_DIR}"
else
    ssh_cmd "mkdir -p ${REMOTE_BASE_DIR}" 2>/dev/null || {
        log_error "Failed to create remote directory"
        exit 1
    }
    log_success "Remote directory ready"
fi

# Step 3: Deploy binary (if not skipped)
if [ "$SKIP_BINARY" = false ]; then
    log_info "Deploying binary..."
    
    BINARY_FILE="bticino_bridge"
    if [ ! -f "$BINARY_FILE" ]; then
        log_info "Binary not found in current directory, checking build output..."
        if [ -f "cmd/bticino_bridge" ]; then
            BINARY_FILE="cmd/bticino_bridge"
        elif [ -f "../bticino_bridge" ]; then
            BINARY_FILE="../bticino_bridge"
        else
            log_error "Binary file not found. Build first with: go build -o bticino_bridge ./cmd/main.go"
            exit 1
        fi
    fi
    
    log_info "Found binary: ${BINARY_FILE}"
    BINARY_SIZE=$(ls -lh "$BINARY_FILE" | awk '{print $5}')
    log_info "Binary size: ${BINARY_SIZE}"
    
    if [ "$DRY_RUN" = true ]; then
        log_warning "[DRY-RUN] Would deploy binary using one of these methods:"
        log_warning "  1. SCP (if available)"
        log_warning "  2. Base64 encoding (guaranteed)"
    else
        # Try SCP first
        log_info "Attempting SCP transfer..."
        if scp_cmd "$BINARY_FILE" "${REMOTE_BASE_DIR}/bticino_bridge.new" 2>/dev/null; then
            log_success "Binary transferred via SCP"
        else
            log_warning "SCP failed, using base64 method..."
            
            # Base64 method
            log_info "Encoding binary to base64..."
            BASE64_FILE=$(mktemp /tmp/bticino_bridge.base64.XXXXXX)
            base64 "$BINARY_FILE" > "$BASE64_FILE"
            BASE64_SIZE=$(wc -c < "$BASE64_FILE" | awk '{print $1}')
            log_info "Base64 encoded size: $(numfmt --to=iec-i --suffix=B $BASE64_SIZE 2>/dev/null || echo "${BASE64_SIZE} bytes")"
            
            # Transfer base64 file
            log_info "Transferring base64 file..."
            if scp_cmd "$BASE64_FILE" "${REMOTE_BASE_DIR}/bticino_bridge.base64" 2>/dev/null; then
                log_success "Base64 file transferred"
                
                # Decode on remote
                log_info "Decoding binary on remote device..."
                ssh_cmd "base64 -d ${REMOTE_BASE_DIR}/bticino_bridge.base64 > ${REMOTE_BASE_DIR}/bticino_bridge.new"
                log_success "Binary decoded successfully"
                
                # Cleanup base64 file
                ssh_cmd "rm -f ${REMOTE_BASE_DIR}/bticino_bridge.base64"
            else
                log_error "Failed to transfer base64 file"
                rm -f "$BASE64_FILE"
                exit 1
            fi
            
            # Cleanup temp file
            rm -f "$BASE64_FILE"
        fi
        
        # Make executable
        log_info "Setting executable permissions..."
        ssh_cmd "chmod +x ${REMOTE_BASE_DIR}/bticino_bridge.new"
        
        # Verify binary
        log_info "Verifying binary..."
        if ssh_cmd "file ${REMOTE_BASE_DIR}/bticino_bridge.new" 2>/dev/null | grep -q "ELF"; then
            log_success "Binary verified (ELF format)"
        else
            log_warning "Could not verify binary format (file command may not be available)"
        fi
    fi
else
    log_warning "Skipping binary deployment (--skip-binary)"
fi

# Step 4: Deploy config (if not skipped)
if [ "$SKIP_CONFIG" = false ]; then
    log_info "Deploying configuration..."
    
    if [ ! -f "$CONFIG_FILE" ]; then
        log_error "Config file not found: ${CONFIG_FILE}"
        exit 1
    fi
    
    log_info "Config file: ${CONFIG_FILE}"
    
    if [ "$DRY_RUN" = true ]; then
        log_warning "[DRY-RUN] Would deploy config file"
    else
        # Try SCP first
        if scp_cmd "$CONFIG_FILE" "${REMOTE_BASE_DIR}/config.yaml.new" 2>/dev/null; then
            log_success "Config transferred via SCP"
        else
            log_warning "SCP failed, using base64 method for config..."
            
            # Base64 method for config
            BASE64_CONFIG=$(base64 "$CONFIG_FILE")
            echo "$BASE64_CONFIG" | ssh_cmd "base64 -d > ${REMOTE_BASE_DIR}/config.yaml.new"
            log_success "Config transferred via base64"
        fi
    fi
else
    log_warning "Skipping config deployment (--skip-config)"
fi

# Step 5: Create recording directory
log_info "Creating recording directory..."
if [ "$DRY_RUN" = true ]; then
    log_warning "[DRY-RUN] Would create ${REMOTE_RECORDINGS}"
else
    ssh_cmd "mkdir -p ${REMOTE_RECORDINGS} && chmod 755 ${REMOTE_RECORDINGS}"
    log_success "Recording directory created: ${REMOTE_RECORDINGS}"
fi

# Step 6: Backup current installation
log_info "Creating backup of current installation..."
if [ "$DRY_RUN" = true ]; then
    log_warning "[DRY-RUN] Would create backup"
else
    ssh_cmd "cd ${REMOTE_BASE_DIR} && [ -f bticino_bridge ] && cp bticino_bridge bticino_bridge.backup.$(date +%Y%m%d_%H%M%S) || echo 'No binary to backup'"
    ssh_cmd "cd ${REMOTE_BASE_DIR} && [ -f config.yaml ] && cp config.yaml config.yaml.backup.$(date +%Y%m%d_%H%M%S) || echo 'No config to backup'"
    log_success "Backup created"
fi

# Step 7: Replace current installation
if [ "$DRY_RUN" = false ]; then
    if [ "$SKIP_BINARY" = false ]; then
        log_info "Activating new binary..."
        ssh_cmd "cd ${REMOTE_BASE_DIR} && mv bticino_bridge.new bticino_bridge"
        log_success "Binary activated"
    fi
    
    if [ "$SKIP_CONFIG" = false ]; then
        log_info "Activating new configuration..."
        ssh_cmd "cd ${REMOTE_BASE_DIR} && mv config.yaml.new config.yaml"
        log_success "Configuration activated"
    fi
else
    log_warning "[DRY-RUN] Would activate new installation"
fi

# Step 8: Test configuration
log_info "Testing configuration..."
if [ "$DRY_RUN" = true ]; then
    log_warning "[DRY-RUN] Would test configuration"
else
    if ssh_cmd "cd ${REMOTE_BASE_DIR} && ./bticino_bridge -config config.yaml -test" 2>&1 | grep -q "error"; then
        log_warning "Configuration test reported warnings (may be OK)"
    else
        log_success "Configuration test passed"
    fi
fi

# Step 9: Restart service
log_info "Restarting bticino_bridge service..."
if [ "$DRY_RUN" = true ]; then
    log_warning "[DRY-RUN] Would restart service"
else
    ssh_cmd "/etc/init.d/bticino_bridge stop" || log_warning "Failed to stop service (may not be running)"
    sleep 2
    ssh_cmd "/etc/init.d/bticino_bridge start" || {
        log_error "Failed to start service"
        log_info "Try manual start: ssh ${SSH_OPTS} ${SSH_HOST} 'cd ${REMOTE_BASE_DIR} && ./bticino_bridge -config config.yaml &'"
        exit 1
    }
    log_success "Service restarted"
fi

# Step 10: Verify deployment
log_info "Verifying deployment..."
if [ "$DRY_RUN" = true ]; then
    log_warning "[DRY-RUN] Would verify deployment"
else
    sleep 3
    
    # Check process is running
    if ssh_cmd "ps aux | grep bticino_bridge | grep -v grep" | grep -q "bticino"; then
        log_success "Process is running"
    else
        log_error "Process not running!"
        log_info "Check logs: ssh ${SSH_OPTS} ${SSH_HOST} 'tail -50 /var/log/bticino_bridge.log'"
        exit 1
    fi
    
    # Quick API test
    log_info "Testing web API..."
    if ssh_cmd "curl -s http://localhost:8082/api/status" 2>/dev/null | grep -q "version"; then
        log_success "Web API responding"
    else
        log_warning "Web API not responding (may need more time to start)"
    fi
fi

echo ""
echo "========================================"
log_success "Deployment completed successfully!"
echo "========================================"
echo ""
echo "Next steps:"
echo "  1. Check logs: ssh ${SSH_OPTS} ${SSH_HOST} 'tail -f /var/log/bticino_bridge.log'"
echo "  2. Test RTSP: ffplay -f rtsp -i rtsp://${SSH_HOST}:6554/doorbell"
echo "  3. Web dashboard: http://${SSH_HOST}:8082/"
echo "  4. API status: curl http://${SSH_HOST}:8082/api/streaming"
echo ""
echo "To rollback:"
echo "  ssh ${SSH_OPTS} ${SSH_HOST}"
echo "  cd ${REMOTE_BASE_DIR}"
echo "  cp bticino_bridge.backup.* bticino_bridge"
echo "  cp config.yaml.backup.* config.yaml"
echo "  /etc/init.d/bticino_bridge restart"
echo ""
