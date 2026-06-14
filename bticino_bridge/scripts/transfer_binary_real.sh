#!/bin/bash
#
# Transferencia de binario bticino_bridge al dispositivo real
# Usa base64 + chunks para máxima compatibilidad
#

set -e

echo "========================================"
echo "  BTicino Bridge - Deploy Real"
echo "  Transferencia de binario v0.12.0"
echo "========================================"
echo ""

# Configuración
SSH_HOST="192.168.1.38"
SSH_USER="root"
REMOTE_DIR="/home/bticino/cfg/extra"
BINARY_NAME="bticino_bridge"

# SSH options para compatibilidad con dispositivo antiguo
SSH_OPTS="-o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa"

echo "Host: ${SSH_HOST}"
echo "User: ${SSH_USER}"
echo "Remote: ${REMOTE_DIR}"
echo ""

# Paso 1: Verificar conexión SSH
echo "[1/6] Verificando conexión SSH..."
if ! ssh ${SSH_OPTS} ${SSH_USER}@${SSH_HOST} "echo 'SSH OK'" > /dev/null 2>&1; then
    echo "ERROR: No se pudo conectar por SSH"
    echo "Intenta: ssh ${SSH_USER}@${SSH_HOST}"
    exit 1
fi
echo "✅ SSH connection OK"
echo ""

# Paso 2: Remount filesystem como read-write
echo "[2/6] Remounting filesystem como read-write..."
ssh ${SSH_OPTS} ${SSH_USER}@${SSH_HOST} "mount -o remount,rw /" 2>/dev/null || echo "⚠️ Ya estaba RW o fallo (no critico)"
echo "✅ Filesystem RW"
echo ""

# Paso 3: Crear directorio de recordings
echo "[3/6] Creando directorio de recordings..."
ssh ${SSH_OPTS} ${SSH_USER}@${SSH_HOST} "mkdir -p ${REMOTE_DIR}/recordings && chmod 755 ${REMOTE_DIR}/recordings"
echo "✅ Recordings directory created"
echo ""

# Paso 4: Transferir chunks via base64
echo "[4/6] Transfiriendo binario (10 chunks)..."
echo "    Esto puede tomar 2-5 minutos..."

# Función para transferir un chunk
transfer_chunk() {
    local chunk_file=$1
    local chunk_name=$(basename "$chunk_file")
    
    # Leer chunk y enviar por SSH
    cat "$chunk_file" | ssh ${SSH_OPTS} ${SSH_USER}@${SSH_HOST} "cat >> ${REMOTE_DIR}/${chunk_name}"
    echo "    ✅ Chunk: ${chunk_name}"
}

# Transferir cada chunk
for chunk in /tmp/bticino_bridge.base64.chunk.*; do
    transfer_chunk "$chunk"
done

echo "✅ All chunks transferred"
echo ""

# Paso 5: Ensamblar y decodificar binario
echo "[5/6] Ensamblando y decodificando binario..."
ssh ${SSH_OPTS} ${SSH_USER}@${SSH_HOST} "cd ${REMOTE_DIR} && cat bticino_bridge.base64.chunk.* > bticino_bridge.base64 && base64 -d bticino_bridge.base64 > ${BINARY_NAME}.new && rm bticino_bridge.base64.chunk.* && rm bticino_bridge.base64"

# Verificar binario
ssh ${SSH_OPTS} ${SSH_USER}@${SSH_HOST} "ls -lh ${REMOTE_DIR}/${BINARY_NAME}.new"
echo "✅ Binary decoded"
echo ""

# Paso 6: Hacer ejecutable y activar
echo "[6/6] Activando binario..."
ssh ${SSH_OPTS} ${SSH_USER}@${SSH_HOST} "chmod +x ${REMOTE_DIR}/${BINARY_NAME}.new && mv ${REMOTE_DIR}/${BINARY_NAME}.new ${REMOTE_DIR}/${BINARY_NAME}"
ssh ${SSH_OPTS} ${SSH_USER}@${SSH_HOST} "ls -lh ${REMOTE_DIR}/${BINARY_NAME}"
echo "✅ Binary activated"
echo ""

echo "========================================"
echo "  Deploy completado!"
echo "========================================"
echo ""
echo "Próximos pasos:"
echo "  1. Crear configuración: ssh root@${SSH_HOST}"
echo "     cd ${REMOTE_DIR}"
echo "     nano config.yaml (o usar el ejemplo)"
echo ""
echo "  2. Probar binario: ssh root@${SSH_HOST}"
echo "     cd ${REMOTE_DIR}"
echo "     ./${BINARY_NAME} -version"
echo ""
echo "  3. Iniciar servicio manualmente:"
echo "     ./${BINARY_NAME} -config config.yaml &"
echo ""
echo "Para rollback:"
echo "  ssh root@${SSH_HOST} 'rm ${REMOTE_DIR}/${BINARY_NAME}'"
echo "  (y restaurar backup si existe)"
echo ""
