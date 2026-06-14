#!/bin/bash
#
# BTicino Bridge - Test Suite Master
# Ejecuta todos los tests de forma automatizada
#
# Uso: ./run_all_tests.sh [opciones]
#
# Opciones:
#   --all          Ejecutar todos los tests (default)
#   --ui           Solo tests de UI
#   --mqtt         Solo tests de MQTT
#   --deploy       Solo tests de deploy
#   --sip          Solo tests de SIP
#   --verbose      Mostrar output detallado
#   --dry-run      Solo mostrar qué se ejecutaría
#

set -e

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Variables
DEVICE_IP="192.168.1.38"
DEVICE_USER="bticino"
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS_DIR="$TEST_DIR/scripts"
DOCS_DIR="$TEST_DIR/docs"
VERBOSE=false
DRY_RUN=false
RUN_ALL=true
RUN_UI=false
RUN_MQTT=false
RUN_DEPLOY=false
RUN_SIP=false

# Contadores
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Parsear argumentos
while [[ $# -gt 0 ]]; do
    case $1 in
        --all)
            RUN_ALL=true
            shift
            ;;
        --ui)
            RUN_UI=true
            RUN_ALL=false
            shift
            ;;
        --mqtt)
            RUN_MQTT=true
            RUN_ALL=false
            shift
            ;;
        --deploy)
            RUN_DEPLOY=true
            RUN_ALL=false
            shift
            ;;
        --sip)
            RUN_SIP=true
            RUN_ALL=false
            shift
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        *)
            echo -e "${RED}Opción desconocida: $1${NC}"
            echo "Usa --help para ver opciones"
            exit 1
            ;;
    esac
done

# Funciones de logging
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((PASSED_TESTS++))
    ((TOTAL_TESTS++))
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((FAILED_TESTS++))
    ((TOTAL_TESTS++))
}

log_test() {
    echo -e "\n${BLUE}=== TEST: $1 ===${NC}"
}

# Funciones de test
test_connectivity() {
    log_test "Conectividad con dispositivo"
    
    if ping -c 2 "$DEVICE_IP" > /dev/null 2>&1; then
        log_success "Ping al dispositivo ($DEVICE_IP)"
    else
        log_error "Ping al dispositivo ($DEVICE_IP) - FALLÓ"
    fi
    
    if ssh -o BatchMode=yes -o ConnectTimeout=5 "$DEVICE_USER@$DEVICE_IP" "echo OK" > /dev/null 2>&1; then
        log_success "SSH al dispositivo"
    else
        log_error "SSH al dispositivo - FALLÓ"
    fi
}

test_ui() {
    log_test "UI de Configuración"
    
    # Test Settings Page
    if curl -s "http://$DEVICE_IP:8082/settings" | grep -q "Configuration Settings"; then
        log_success "Settings Page accesible"
    else
        log_error "Settings Page - NO ACCESIBLE"
    fi
    
    # Test API Config
    if curl -s "http://$DEVICE_IP:8082/api/config" | grep -q "Bridge"; then
        log_success "API /api/config responde"
    else
        log_error "API /api/config - NO RESPONDE"
    fi
    
    # Test API Validate
    VALIDATE_RESPONSE=$(curl -s -X POST "http://$DEVICE_IP:8082/api/config/validate" \
        -H "Content-Type: application/json" \
        -d '{"config":{"bridge":{"name":"Test"},"openwebnet":{"port":30006},"web":{"port":8082}}}')
    
    if echo "$VALIDATE_RESPONSE" | grep -q "valid"; then
        log_success "API /api/config/validate funciona"
    else
        log_error "API /api/config/validate - FALLÓ"
    fi
    
    # Test API Backup
    BACKUP_RESPONSE=$(curl -s -X POST "http://$DEVICE_IP:8082/api/config/backup")
    
    if echo "$BACKUP_RESPONSE" | grep -q "success"; then
        log_success "API /api/config/backup funciona"
    else
        log_error "API /api/config/backup - FALLÓ"
    fi
}

test_mqtt() {
    log_test "MQTT Commands"
    
    if [ -x "$SCRIPTS_DIR/bticino_mqtt_commands_simple.sh" ]; then
        if $VERBOSE; then
            "$SCRIPTS_DIR/bticino_mqtt_commands_simple.sh"
        else
            "$SCRIPTS_DIR/bticino_mqtt_commands_simple.sh" > /dev/null 2>&1
        fi
        
        if [ $? -eq 0 ]; then
            log_success "MQTT commands script ejecutado"
        else
            log_error "MQTT commands script - FALLÓ"
        fi
    else
        log_warning "MQTT script no encontrado o no ejecutable"
    fi
}

test_deploy() {
    log_test "Deploy Scripts"
    
    # Verificar scripts de deploy
    for script in deploy_to_bticino.sh deploy_auto.sh deploy_and_test_safe.sh; do
        if [ -x "$SCRIPTS_DIR/$script" ]; then
            log_success "Script $script existe y es ejecutable"
        else
            log_error "Script $script - NO ENCONTRADO O NO EJECUTABLE"
        fi
    done
    
    # Verificar binario
    if [ -x "$TEST_DIR/bticino_bridge" ]; then
        log_success "Binario bticino_bridge existe y es ejecutable"
        
        if $VERBOSE; then
            "$TEST_DIR/bticino_bridge" -version
        fi
    else
        log_error "Binario bticino_bridge - NO ENCONTRADO"
    fi
}

test_sip() {
    log_test "SIP Configuration"
    
    # Verificar flexisip en dispositivo
    FLEXISIP_STATUS=$(ssh "$DEVICE_USER@$DEVICE_IP" "ps aux | grep flexisip | grep -v grep" 2>/dev/null)
    
    if [ -n "$FLEXISIP_STATUS" ]; then
        log_success "Flexisip corriendo en dispositivo"
    else
        log_warning "Flexisip NO está corriendo"
    fi
    
    # Verificar logs de SIP
    SIP_LOGS=$(ssh "$DEVICE_USER@$DEVICE_IP" "grep -i 'sip\|register' /var/log/bticino_bridge.log 2>/dev/null | tail -5")
    
    if [ -n "$SIP_LOGS" ]; then
        if $VERBOSE; then
            echo "$SIP_LOGS"
        fi
        log_success "Logs de SIP disponibles"
    else
        log_warning "No hay logs de SIP recientes"
    fi
}

test_documentation() {
    log_test "Documentación"
    
    # Verificar README maestro
    if [ -f "$DOCS_DIR/README.md" ]; then
        log_success "README.md maestro existe"
    else
        log_error "README.md maestro - NO ENCONTRADO"
    fi
    
    # Contar documentos
    DOC_COUNT=$(ls -1 "$DOCS_DIR"/*.md 2>/dev/null | wc -l)
    log_success "$DOC_COUNT documentos en docs/"
    
    # Verificar scripts
    SCRIPT_COUNT=$(ls -1 "$SCRIPTS_DIR"/*.sh 2>/dev/null | wc -l)
    log_success "$SCRIPT_COUNT scripts en scripts/"
}

# Main
echo -e "${BLUE}"
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║     BTicino Bridge - Test Suite Master v0.13.0           ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo -e "${NC}"

if [ "$DRY_RUN" = true ]; then
    echo -e "${YELLOW}=== DRY RUN MODE ===${NC}"
    echo "Los siguientes tests se ejecutarían:"
    
    if [ "$RUN_ALL" = true ] || [ "$RUN_UI" = true ]; then
        echo "  - UI Tests"
    fi
    if [ "$RUN_ALL" = true ] || [ "$RUN_MQTT" = true ]; then
        echo "  - MQTT Tests"
    fi
    if [ "$RUN_ALL" = true ] || [ "$RUN_DEPLOY" = true ]; then
        echo "  - Deploy Tests"
    fi
    if [ "$RUN_ALL" = true ] || [ "$RUN_SIP" = true ]; then
        echo "  - SIP Tests"
    fi
    
    echo ""
    echo "Documentación:"
    echo "  - Verificar docs/README.md"
    echo "  - Verificar scripts ejecutables"
    exit 0
fi

echo "Configuración:"
echo "  Device: $DEVICE_USER@$DEVICE_IP"
echo "  Test Dir: $TEST_DIR"
echo "  Scripts: $SCRIPTS_DIR"
echo "  Docs: $DOCS_DIR"
echo "  Verbose: $VERBOSE"
echo ""

# Ejecutar tests
test_connectivity

if [ "$RUN_ALL" = true ] || [ "$RUN_UI" = true ]; then
    test_ui
fi

if [ "$RUN_ALL" = true ] || [ "$RUN_MQTT" = true ]; then
    test_mqtt
fi

if [ "$RUN_ALL" = true ] || [ "$RUN_DEPLOY" = true ]; then
    test_deploy
fi

if [ "$RUN_ALL" = true ] || [ "$RUN_SIP" = true ]; then
    test_sip
fi

test_documentation

# Resumen
echo -e "\n${BLUE}╔═══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                    TEST SUMMARY                            ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed:       $PASSED_TESTS${NC}"
echo -e "${RED}Failed:       $FAILED_TESTS${NC}"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✅ ALL TESTS PASSED!${NC}"
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    echo ""
    echo "Revisa los logs arriba para más detalles."
    echo "Para más información, usa --verbose"
    exit 1
fi
