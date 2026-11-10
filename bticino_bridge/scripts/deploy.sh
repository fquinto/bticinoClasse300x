#!/bin/bash
#
# BTicino Bridge - Deploy ÚNICO (Build + Deploy)
# Unifica build de web + go + deploy en un único comando
#
# Uso: ./scripts/deploy.sh [opciones]
#
# Destino: el alias SSH "bticino" por defecto. Para otro equipo:
#   BTICINO_SSH=root2@192.168.1.50 ./scripts/deploy.sh
#
# Disposición en el equipo (la misma que usa el bridge por defecto):
#   /home/bticino/cfg/extra/bticino_bridge
#   /home/bticino/cfg/extra/configs/config.yaml    ajustes (sin credenciales)
#   /home/bticino/cfg/extra/configs/secrets.yaml   credenciales (0600)
# Este script NUNCA sube ni modifica secrets.yaml. Solo en una instalación
# nueva (sin configs/config.yaml) sube la plantilla de configuración.
#
# Opciones:
#   --skip-web     No construir web (usar build anterior)
#   --skip-go      No construir go (usar build anterior)
#   --dry-run      Solo mostrar qué se haría
#   --verbose      Output detallado
#

set -e

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Variables
SKIP_WEB=false
SKIP_GO=false
DRY_RUN=false
VERBOSE=false
REMOTE="${BTICINO_SSH:-bticino}"
REMOTE_DIR="/home/bticino/cfg/extra"
REMOTE_LOG="/var/log/bticino_bridge.log"
VERSION_STR="$(cat VERSION 2>/dev/null || echo '?')"

# Parsear argumentos
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-web)
            SKIP_WEB=true
            shift
            ;;
        --skip-go)
            SKIP_GO=true
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
        *)
            echo -e "${RED}Opción desconocida: $1${NC}"
            exit 1
            ;;
    esac
done

# Logging
log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[$(date +'%H:%M:%S')] ✅ ${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[$(date +'%H:%M:%S')] ⚠️  ${NC} $1"
}

log_error() {
    echo -e "${RED}[$(date +'%H:%M:%S')] ❌ ${NC} $1"
}

# Main
echo -e "${BLUE}"
echo "╔═══════════════════════════════════════════════════════════╗"
printf '║   BTicino Bridge - Deploy  v%-30s║\n' "$VERSION_STR"
echo "╚═══════════════════════════════════════════════════════════╝"
echo -e "${NC}"

if [ "$DRY_RUN" = true ]; then
    echo -e "${YELLOW}=== DRY RUN MODE ===${NC}"
    echo "Los siguientes pasos se ejecutarían:"
    [ "$SKIP_WEB" = false ] && echo "  - Build web frontend"
    [ "$SKIP_GO" = false ] && echo "  - Build Go binary"
    echo "  - Deploy to $REMOTE:$REMOTE_DIR (config en configs/config.yaml)"
    echo "  - Arrancar: cd $REMOTE_DIR && ./bticino_bridge"
    exit 0
fi

# Paso 1: Build Web (si existe web/ y no se skip)
if [ "$SKIP_WEB" = false ] && [ -d "web" ]; then
    log "[1/3] Building web frontend..."
    if [ -f "web/package.json" ]; then
        cd web
        npm install --silent
        npm run build --silent
        cd ..
        log_success "Web frontend built (web/dist/)"
    else
        log_warning "web/package.json not found, skipping web build"
        SKIP_WEB=true
    fi
else
    if [ "$SKIP_WEB" = false ]; then
        log_warning "web/ directory not found, skipping web build"
    fi
fi

# Paso 2: Build Go
if [ "$SKIP_GO" = false ]; then
    log "[2/3] Building Go binary..."
    # Compilar el paquete entero (./cmd), no solo main.go, y sin tapar errores:
    # si falla, no se debe desplegar un binario antiguo que quedara en disco.
    rm -f bticino_bridge
    if GOOS=linux GOARCH=arm GOARM=7 go build -o bticino_bridge ./cmd && [ -f "bticino_bridge" ]; then
        log_success "Go binary built (bticino_bridge)"
    else
        log_error "Failed to build Go binary"
        exit 1
    fi
fi

# Paso 3: Deploy
log "[3/3] Deploying to device..."

# Verificar SSH
if ! ssh -o BatchMode=yes -o ConnectTimeout=5 "$REMOTE" "echo OK" > /dev/null 2>&1; then
    log_error "SSH connection failed. Check SSH keys."
    exit 1
fi
log_success "SSH OK"

# Configuración: el bridge lee configs/config.yaml (y configs/secrets.yaml).
ssh "$REMOTE" "mkdir -p $REMOTE_DIR/configs"
if ssh "$REMOTE" "[ -f $REMOTE_DIR/configs/config.yaml ]"; then
    log_success "Config existente en configs/config.yaml (no se toca)"
elif ssh "$REMOTE" "[ -f $REMOTE_DIR/config.yaml ]"; then
    log_error "Hay un config.yaml antiguo en $REMOTE_DIR pero no en configs/."
    echo "    Muévelo y vuelve a lanzar el deploy:"
    echo "      ssh $REMOTE 'cd $REMOTE_DIR && mv config.yaml configs/config.yaml'"
    echo "    Después pasa las credenciales a configs/secrets.yaml (docs/SECRETS.md)."
    exit 1
else
    log_warning "Instalación nueva: subo la plantilla a configs/config.yaml"
    base64 configs/config.yaml | ssh "$REMOTE" "base64 -d > $REMOTE_DIR/configs/config.yaml"
fi
if ! ssh "$REMOTE" "[ -s $REMOTE_DIR/configs/secrets.yaml ]"; then
    log_warning "No hay configs/secrets.yaml en el equipo. Si config.yaml no trae las"
    log_warning "credenciales, el bridge no arrancará y dirá cuáles faltan."
    echo "    Crea el fichero a partir de configs/secrets.example.yaml (docs/SECRETS.md):"
    echo "      scp configs/secrets.yaml $REMOTE:$REMOTE_DIR/configs/secrets.yaml"
    echo "      ssh $REMOTE 'chmod 600 $REMOTE_DIR/configs/secrets.yaml'"
fi

# Transferir binario
log "Transferring binary..."
# Transfer VERSION file
echo "Transferring VERSION file..."
cat VERSION | ssh "$REMOTE" "cat > $REMOTE_DIR/VERSION"
log_success "VERSION file transferred"

base64 bticino_bridge | ssh "$REMOTE" "cd $REMOTE_DIR && base64 -d > bticino_bridge.new && chmod +x bticino_bridge.new"
log_success "Binary transferred"

# Backup
log "Creating backup..."
ssh "$REMOTE" "cd $REMOTE_DIR && [ -f bticino_bridge ] && cp bticino_bridge bticino_bridge.backup.\$(date +%Y%m%d_%H%M%S) || true"
log_success "Backup created"

# Activar
log "Activating new binary..."
ssh "$REMOTE" "cd $REMOTE_DIR && mv bticino_bridge.new bticino_bridge"
log_success "Binary activated"

# Transferir web Svelte (si existe)
if [ "$SKIP_WEB" = false ] && [ -d "web/dist" ]; then
    log "Transferring Svelte web files..."
    ssh "$REMOTE" "cd $REMOTE_DIR && mkdir -p web/dist/assets"
    # Copiar archivos con base64 (más confiable que scp)
    base64 web/dist/index.html | ssh "$REMOTE" "base64 -d > $REMOTE_DIR/web/dist/index.html"
    for file in web/dist/assets/*; do
        if [ -f "$file" ]; then
            filename=$(basename "$file")
            base64 "$file" | ssh "$REMOTE" "base64 -d > $REMOTE_DIR/web/dist/assets/$filename"
        fi
    done
    log_success "Svelte web files transferred"
fi

# Reiniciar servicio
log "Restarting service..."
# Parar con TERM (apagado ordenado) y solo después forzar. Arrancar SIN
# -config: por defecto lee configs/config.yaml y el secrets.yaml de al lado.
ssh "$REMOTE" "pkill -TERM -x bticino_bridge 2>/dev/null; sleep 3; pkill -9 -x bticino_bridge 2>/dev/null; true"
ssh "$REMOTE" "cd $REMOTE_DIR && setsid sh -c 'exec ./bticino_bridge > $REMOTE_LOG 2>&1' < /dev/null > /dev/null 2>&1 &"
sleep 6
log_success "Service restarted"

# Verificar
log "Verifying..."
if ssh "$REMOTE" "ps | grep -v grep | grep -q 'bticino_bridge'"; then
    log_success "Service running"
else
    log_error "Service not running. Últimas líneas del log:"
    ssh "$REMOTE" "tail -15 $REMOTE_LOG" 2>/dev/null | sed 's/^/    /'
    echo "    Si el log dice 'missing required secrets', falta configs/secrets.yaml"
    echo "    o alguna de sus claves (docs/SECRETS.md)."
    exit 1
fi

echo
echo -e "${GREEN}╔═══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║              ✅ Deploy completado exitosamente!            ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════════════════════╝${NC}"
echo
echo "Próximos pasos:"
echo "  1. Web UI: http://<ip-del-equipo>:8082/"
echo "  2. Logs: ssh $REMOTE 'tail -f $REMOTE_LOG'"
echo "  3. Si la web no responde desde otra máquina: el equipo trae su propio"
echo "     firewall; instala las reglas de patches/bticino_bridge/."
echo
