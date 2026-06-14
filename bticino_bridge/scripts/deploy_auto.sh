#!/bin/bash
#
# Deploy automático para BTicino Bridge
# Asume que SSH ya está configurado y funciona sin password
#

set -e

echo "========================================"
echo "  BTicino Bridge - Deploy Automático"
echo "========================================"
echo ""

REMOTE="bticino"
SSH_OPTS="-o StrictHostKeyChecking=no -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa"
REMOTE_DIR="/home/bticino/cfg/extra"

echo "⚠️  IMPORTANTE: Este script asume que:"
echo "   1. Ya aceptaste la host key del dispositivo"
echo "   2. SSH funciona sin password (key-based)"
echo ""
echo "Si SSH pide password, configurá antes las keys:"
echo "  ssh-copy-id -o HostKeyAlgorithms=+ssh-rsa bticino"
echo ""
read -p "Presioná ENTER para continuar o Ctrl+C para cancelar..."
echo ""

# Paso 1: Test SSH
echo "[1/8] Testing SSH..."
if ! ssh ${SSH_OPTS} ${REMOTE} "echo 'OK'" > /dev/null 2>&1; then
    echo "❌ ERROR: SSH no funciona"
    echo "Probá: ssh ${SSH_OPTS} ${REMOTE}"
    exit 1
fi
echo "✅ SSH OK"
echo ""

# Paso 2: Remount filesystem
echo "[2/8] Remount filesystem RW..."
ssh ${SSH_OPTS} ${REMOTE} "mount -o remount,rw /" 2>/dev/null || true
echo "✅ Filesystem RW"
echo ""

# Paso 3: Crear directorio recordings
echo "[3/8] Creating recordings directory..."
ssh ${SSH_OPTS} ${REMOTE} "mkdir -p ${REMOTE_DIR}/recordings && chmod 755 ${REMOTE_DIR}/recordings"
echo "✅ Recordings dir created"
echo ""

# Paso 4: Backup si existe versión anterior
echo "[4/8] Checking for existing installation..."
if ssh ${SSH_OPTS} ${REMOTE} "test -f ${REMOTE_DIR}/bticino_bridge && echo 'exists'" 2>/dev/null | grep -q "exists"; then
    echo "⚠️  Existing installation found, creating backup..."
    ssh ${SSH_OPTS} ${REMOTE} "cp ${REMOTE_DIR}/bticino_bridge ${REMOTE_DIR}/bticino_bridge.backup.\$(date +%Y%m%d_%H%M%S) 2>/dev/null || echo 'Backup skipped'"
    echo "✅ Backup created"
else
    echo "ℹ️  No existing installation (new deploy)"
fi
echo ""

# Paso 5: Transfer binary via base64 chunks
echo "[5/8] Transferring binary (base64 method)..."
echo "    This may take 3-5 minutes..."

# Clean up old chunks on remote
ssh ${SSH_OPTS} ${REMOTE} "rm -f ${REMOTE_DIR}/bticino_bridge.base64.chunk.* ${REMOTE_DIR}/bticino_bridge.base64" 2>/dev/null || true

# Transfer each chunk
CHUNKS=(/tmp/bticino_bridge.base64.chunk.*)
TOTAL=${#CHUNKS[@]}
CURRENT=0

for chunk in "${CHUNKS[@]}"; do
    CURRENT=$((CURRENT + 1))
    chunk_name=$(basename "$chunk")
    echo "    [${CURRENT}/${TOTAL}] ${chunk_name}..."
    
    # Transfer chunk
    cat "$chunk" | ssh ${SSH_OPTS} ${REMOTE} "cat >> ${REMOTE_DIR}/${chunk_name}"
done

echo "✅ All chunks transferred"
echo ""

# Paso 6: Assemble and decode
echo "[6/8] Assembling and decoding binary..."
ssh ${SSH_OPTS} ${REMOTE} "cd ${REMOTE_DIR} && cat bticino_bridge.base64.chunk.* > bticino_bridge.base64 && base64 -d bticino_bridge.base64 > bticino_bridge.new && rm -f bticino_bridge.base64.chunk.* bticino_bridge.base64"

# Verify
ssh ${SSH_OPTS} ${REMOTE} "ls -lh ${REMOTE_DIR}/bticino_bridge.new"
echo "✅ Binary decoded"
echo ""

# Paso 7: Make executable and activate
echo "[7/8] Activating binary..."
ssh ${SSH_OPTS} ${REMOTE} "chmod +x ${REMOTE_DIR}/bticino_bridge.new && mv ${REMOTE_DIR}/bticino_bridge.new ${REMOTE_DIR}/bticino_bridge"
ssh ${SSH_OPTS} ${REMOTE} "ls -lh ${REMOTE_DIR}/bticino_bridge"
echo "✅ Binary activated"
echo ""

# Paso 8: Verify binary works
echo "[8/8] Testing binary..."
if ssh ${SSH_OPTS} ${REMOTE} "cd ${REMOTE_DIR} && ./bticino_bridge -version" 2>&1 | grep -q "v0.12"; then
    echo "✅ Binary version OK (v0.12.0)"
else
    echo "⚠️  Could not verify version (may still work)"
fi
echo ""

echo "========================================"
echo "  ✅ Deploy completado!"
echo "========================================"
echo ""
echo "Próximos pasos MANUALES:"
echo ""
echo "1. Crear configuración:"
echo "   ssh ${SSH_OPTS} ${REMOTE}"
echo "   cd ${REMOTE_DIR}"
echo "   cp config-streaming-example.yaml config.yaml"
echo "   # Editar config.yaml con tus credenciales SIP"
echo ""
echo "2. Probar en modo test:"
echo "   cd ${REMOTE_DIR}"
echo "   ./bticino_bridge -config config.yaml -test"
echo ""
echo "3. Iniciar manualmente:"
echo "   ./bticino_bridge -config config.yaml &"
echo ""
echo "4. Verificar logs:"
echo "   tail -f /var/log/bticino_bridge.log"
echo ""
echo "5. Testear RTSP:"
echo "   ffplay -f rtsp -i rtsp://192.168.1.38:6554/doorbell"
echo ""
echo "Para crear servicio init.d, ver docs/DEPLOYMENT_GUIDE.md"
echo ""
