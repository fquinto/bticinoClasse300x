# Deploy Real - Instrucciones Paso a Paso

**Fecha**: 2026-03-23  
**Dispositivo**: BTicino Classe 300X (192.168.1.38)  
**Versión**: v0.12.0 con WebRTC/RTSP

---

## ⚠️ PRE-REQUISITOS

### 1. Aceptar Host Key del Dispositivo

El dispositivo puede tener una host key diferente. Ejecutá:

```bash
ssh-keygen -f '/home/fquinto/.ssh/known_hosts' -R '192.168.1.38'
```

Luego conectate manualmente para aceptar la nueva key:

```bash
ssh root@192.168.1.38
# Cuando pregunte "Are you sure you want to continue connecting (yes/no/[fingerprint])?"
# Escribí: yes
```

Si pide password, necesitás configurarlo. Si tenés la password, podés copiar la key:

```bash
# Si tenés password del dispositivo:
ssh-copy-id root@192.168.1.38
# (te va a pedir password una vez)
```

---

## 📋 PASOS DE DEPLOY MANUAL

### Paso 1: Verificar conexión SSH

```bash
ssh root@192.168.1.38 "echo 'SSH OK'"
```

**Esperado**: `SSH OK`

---

### Paso 2: Remount filesystem como read-write

```bash
ssh root@192.168.1.38 "mount -o remount,rw /"
```

---

### Paso 3: Crear directorio de recordings

```bash
ssh root@192.168.1.38 "mkdir -p /home/bticino/cfg/extra/recordings && chmod 755 /home/bticino/cfg/extra/recordings"
```

---

### Paso 4: Transferir binario (MÉTODO BASE64)

#### 4.1: Codificar binario en tu máquina de desarrollo

```bash
cd /home/fquinto/DATOS/USB_Security/Seguridad/Dispositivos/Videoportero_classe300_interfono_bticino/bticino_bridge

# Codificar a base64
base64 bticino_bridge > /tmp/bticino_bridge.base64

# Verificar tamaño
wc -l /tmp/bticino_bridge.base64
# Esperado: ~256000 líneas
```

#### 4.2: Transferir base64 al dispositivo

**Opción A: Si scp funciona con las opciones especiales**

```bash
scp -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa \
    /tmp/bticino_bridge.base64 \
    root@192.168.1.38:/home/bticino/cfg/extra/
```

**Opción B: Si scp NO funciona (más probable)**

Usá este script que hace la transferencia automáticamente:

```bash
cd /home/fquinto/DATOS/USB_Security/Seguridad/Dispositivos/Videoportero_classe300_interfono_bticino/bticino_bridge

# El script divide el archivo en chunks de 2MB y los transfiere uno por uno
./scripts/deploy_auto.sh
```

El script va a:
1. Dividir el base64 en 10 chunks de 2MB
2. Transferir cada chunk vía SSH
3. Ensamblar y decodificar en el dispositivo
4. Verificar el binario

**Tiempo estimado**: 3-5 minutos

---

### Paso 5: Verificar binario transferido

```bash
ssh root@192.168.1.38 "ls -lh /home/bticino/cfg/extra/bticino_bridge"
```

**Esperado**: `-rwxr-xr-x ... 14M ... bticino_bridge`

---

### Paso 6: Verificar versión del binario

```bash
ssh root@192.168.1.38 "/home/bticino/cfg/extra/bticino_bridge -version"
```

**Esperado**:
```
BTicino Classe 300X ENHANCED MEGA Bridge v0.12.0
🚀 ENHANCED UNIFIED BRIDGE - Real Device Integration:
  ✅ Enhanced OpenWebNet (50+ proven commands, real device analysis)
  🌐 Web Dashboard (Port 8082, Enhanced API endpoints)
  ...
```

---

### Paso 7: Crear configuración

```bash
ssh root@192.168.1.38
cd /home/bticino/cfg/extra

# Copiar ejemplo
cp configs/config-streaming-example.yaml config.yaml

# Editar configuración (nano o vi si están disponibles)
nano config.yaml

# Si nano no está disponible, usá el método echo:
cat > config.yaml << 'EOF'
bridge:
  name: "BTicino Bridge Enhanced"
  log_level: "info"

openwebnet:
  host: "localhost"
  port: 30006

sip:
  enabled: true
  local_host: "192.168.1.38"
  server_host: "sipserver.bs.iotleg.com"
  server_port: 5061
  transport: "tls"
  domain: "bs.iotleg.com"
  username: "TU_USERNAME_AQUI"
  password: "TU_PASSWORD_AQUI"
  dev_addr: "20"
  use_ha1: false
  insecure_tls: true

mqtt:
  enabled: true
  host: "192.168.1.3"
  port: 1883
  username: "mqtt_user"
  password: "CHANGE_ME"
  topic_prefix: "bticino"

streaming:
  enabled: true
  rtsp_port: 6554
  recording_path: "/home/bticino/cfg/extra/recordings"
  max_duration: 60
  auto_stop_on_last_client: true
EOF
```

**IMPORTANTE**: Reemplazá `TU_USERNAME_AQUI` y `TU_PASSWORD_AQUI` con tus credenciales SIP reales.

---

### Paso 8: Probar configuración

```bash
ssh root@192.168.1.38
cd /home/bticino/cfg/extra

# Modo test (no inicia servicios, solo verifica config)
./bticino_bridge -config config.yaml -test
```

**Esperado**: Mensajes de configuración cargada sin errores críticos.

---

### Paso 9: Iniciar servicio manualmente

```bash
ssh root@192.168.1.38
cd /home/bticino/cfg/extra

# Iniciar en background
nohup ./bticino_bridge -config config.yaml > /var/log/bticino_bridge.log 2>&1 &

# Verificar que está corriendo
ps aux | grep bticino_bridge | grep -v grep
```

**Esperado**: Proceso `./bticino_bridge -config config.yaml` en la lista.

---

### Paso 10: Verificar logs

```bash
ssh root@192.168.1.38 "tail -50 /var/log/bticino_bridge.log"
```

**Esperado** (buscar estas líneas):
```
✅ SIP: Client started successfully
✅ RTSP: Enhanced server started on port 6554
   RTSP streams:
     - rtsp://192.168.1.38:6554/doorbell (Full stream)
     - rtsp://192.168.1.38:6554/doorbell-video (Video only)
     - rtsp://192.168.1.38:6554/doorbell-recorder (HKSV recording)
✅ Video: Stream manager started successfully
```

---

### Paso 11: Testear API Web

```bash
# Desde tu máquina de desarrollo
curl http://192.168.1.38:8082/api/status
```

**Esperado**: JSON con versión, uptime, componentes.

---

### Paso 12: Testear streaming RTSP

```bash
# Con ffplay
ffplay -f rtsp -i rtsp://192.168.1.38:6554/doorbell

# Con VLC
vlc rtsp://192.168.1.38:6554/doorbell

# Con go2rtc (si tenés configurado)
# Verificar logs de go2rtc
```

**Nota**: El stream puede tardar ~10-20 segundos en iniciar porque el dispositivo necesita establecer la llamada SIP.

---

### Paso 13: Testear grabación (HKSV)

```bash
# Iniciar grabación vía API
curl -X POST http://192.168.1.38:8082/api/streaming/record \
  -H "Content-Type: application/json" \
  -d '{"duration": 10}'

# Esperar 10 segundos...

# Verificar archivo de grabación
ssh root@192.168.1.38 "ls -lh /home/bticino/cfg/extra/recordings/"
```

**Esperado**: Archivo `.ts` nuevo con timestamp.

---

## 🔧 TROUBLESHOOTING

### Problema: SSH pide password constantemente

**Solución**: Configurar SSH key-based auth

```bash
# En tu máquina de desarrollo
ssh-keygen -t rsa -b 4096 -f ~/.ssh/bticino_key

# Copiar key al dispositivo (si tenés password)
ssh-copy-id -o HostKeyAlgorithms=+ssh-rsa root@192.168.1.38

# O manualmente (si no funciona ssh-copy-id)
cat ~/.ssh/bticino_key.pub | ssh root@192.168.1.38 "mkdir -p ~/.ssh && cat >> ~/.ssh/authorized_keys"
```

---

### Problema: Binario no ejecuta

```bash
ssh root@192.168.1.38
cd /home/bticino/cfg/extra

# Verificar formato del binario
file bticino_bridge
# Esperado: ELF 32-bit LSB executable, ARM

# Verificar permisos
ls -l bticino_bridge
# Esperado: -rwxr-xr-x

# Si no es ejecutable:
chmod +x bticino_bridge

# Intentar ejecutar con debug
./bticino_bridge -log-level debug 2>&1 | head -20
```

---

### Problema: Puerto 6554 ya en uso

```bash
ssh root@192.168.1.38 "netstat -tlnp | grep 6554 || ss -tlnp | grep 6554"

# Si hay otro proceso, matarlo
ssh root@192.168.1.38 "fuser -k 6554/tcp 2>/dev/null || kill \$(lsof -t -i:6554) 2>/dev/null || echo 'Manual kill required'"
```

---

### Problema: No hay video/audio en RTSP

1. **Verificar SIP registrado**:
   ```bash
   curl http://192.168.1.38:8082/api/status | grep -i sip
   ```

2. **Verificar credenciales SIP en config.yaml**

3. **Probar llamada SIP manual**:
   ```bash
   ssh root@192.168.1.38
   cd /home/bticino/cfg/extra
   # Iniciar bridge en modo debug
   ./bticino_bridge -config config.yaml -log-level debug
   ```

---

### Problema: Recording no funciona

```bash
# Verificar directorio existe y es writable
ssh root@192.168.1.38 "ls -ld /home/bticino/cfg/extra/recordings"
ssh root@192.168.1.38 "touch /home/bticino/cfg/extra/recordings/test && rm /home/bticino/cfg/extra/recordings/test && echo 'Writable OK'"

# Verificar config
ssh root@192.168.1.38 "grep recording_path /home/bticino/cfg/extra/config.yaml"
```

---

## 📊 CHECKLIST DE DEPLOY

Marcar cada paso completado:

- [ ] Host key aceptada
- [ ] SSH sin password funciona
- [ ] Filesystem remounted como RW
- [ ] Directorio recordings creado
- [ ] Binario transferido (base64)
- [ ] Binario decodificado y verificado
- [ ] Versión v0.12.0 confirmada
- [ ] Configuración creada y editada
- [ ] Test mode passed
- [ ] Servicio iniciado manualmente
- [ ] Logs verificados (sin errores críticos)
- [ ] API Web responde
- [ ] RTSP stream funciona (ffplay/VLC)
- [ ] Grabación funciona

---

## 🔄 ROLLBACK (si algo sale mal)

```bash
ssh root@192.168.1.38
cd /home/bticino/cfg/extra

# Detener proceso actual
ps aux | grep bticino_bridge | grep -v grep | awk '{print $2}' | xargs kill -9 2>/dev/null || true

# Restaurar backup (si existe)
if ls bticino_bridge.backup.* 1>/dev/null 2>&1; then
    cp bticino_bridge.backup.* bticino_bridge
    echo "Backup restored"
else
    echo "No backup available"
fi

# Restaurar config (si existe)
if ls config.yaml.backup.* 1>/dev/null 2>&1; then
    cp config.yaml.backup.* config.yaml
    echo "Config restored"
fi

# Reiniciar
./bticino_bridge -config config.yaml &
```

---

**Última actualización**: 2026-03-23  
**Estado**: Probado en laboratorio ✅  
**Próximo**: Deploy en dispositivo real
