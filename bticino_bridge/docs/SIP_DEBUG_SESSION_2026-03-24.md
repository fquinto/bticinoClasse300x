# Informe de Depuración SIP - Sesión 2026-03-24

**Objetivo**: Identificar causa raíz del fallo de registro SIP con flexisip local  
**Estado**: ⚠️ **SIN RESOLVER** - Se requiere más investigación

---

## 📋 Resumen de Depuración

Se realizaron **24 pruebas** para identificar por qué el bridge no se registra con flexisip local.

### Hallazgos Principales:

1. ✅ **flexisip está corriendo** y escuchando en puertos 5060 (TCP) y 5061 (TLS)
2. ✅ **Conectividad de red** verificada (puertos abiertos)
3. ✅ **Configuración de authentication** correcta (trusted-hosts, hashed-passwords)
4. ✅ **Usuarios existen** en users.db.txt (4 usuarios, incluyendo baresip)
5. ❌ **El bridge reporta `i/o timeout`** al enviar REGISTER
6. ❌ **flexisip no responde** al REGISTER del bridge
7. ❌ **El proceso del bridge se cae** después de iniciar (posible panic)

---

## 🔍 Pruebas Realizadas

### Pruebas 1-2: Conectividad de Red
```bash
# Puertos SIP escuchando (5060=0x13C4, 5061=0x13C5):
0100007F:13C4 (127.0.0.1:5060) - ✅ LISTENING
2601A8C0:13C4 (192.168.1.38:5060) - ✅ LISTENING
2601A8C0:13C5 (192.168.1.38:5061) - ✅ LISTENING
```

**Resultado**: ✅ Puertos abiertos correctamente

---

### Pruebas 3-4: Logs Debug Habilitados
```ini
# flexisip.conf
log-level=debug
syslog-level=debug

# config_streaming.yaml
log_level: debug
```

**Resultado**: ✅ Logs detallados habilitados en ambos servicios

---

### Pruebas 5-10: Configuración de Authentication
```ini
# /etc/flexisip/flexisip.conf
[module::Authentication]
enabled=true
auth-domains=1754162.bs.iotleg.com
db-implementation=file
datasource=/etc/flexisip/users/users.db.txt
trusted-hosts=127.0.0.1,192.168.1.38
hashed-passwords=true
reject-wrong-client-certificates=false
```

**Usuarios disponibles**:
```
baresip@1754162.bs.iotleg.com md5:2db500964b556d4cef79284738f2a360
baresip_test@1754162.bs.iotleg.com plain:test123 (agregado en prueba 12)
```

**Resultado**: ✅ Configuración correcta

---

### Pruebas 11-14: Prueba con Password Plano
Se agregó usuario `baresip_test` con password plano `test123` para descartar problemas de hash MD5.

**Configuración usada**:
```yaml
sip:
  username: baresip_test@1754162.bs.iotleg.com
  password: test123
  use_ha1: false
```

**Error obtenido**:
```
failed to register: failed to read REGISTER response: 
read tcp 192.168.1.38:34174->192.168.1.38:5060: i/o timeout
```

**Resultado**: ❌ Mismo error - no es problema del hash

---

### Pruebas 15-17: Verificación de Proceso Bridge
Se descubrió que el proceso viejo (PID 17923, iniciado Mar23) seguía corriendo.

**Acción**: Se mató el proceso viejo y se inició uno nuevo.

**Resultado**: ⚠️ El nuevo proceso se cae después de iniciar

---

### Pruebas 18-24: Análisis de Caída del Bridge
El bridge inicia correctamente pero se cae silenciosamente después de ~30 segundos.

**Últimos logs antes de caer**:
```
INFO[2026-03-24T10:12:13+01:00] HomeKit bridge created successfully
INFO[2026-03-24T10:12:13+01:00] HomeKit bridge started successfully
```

**No hay logs de**:
- ❌ "Starting BTicino SIP client"
- ❌ Errores fatales o panic
- ❌ Mensaje de shutdown

**Resultado**: ❌ El proceso desaparece sin dejar rastro

---

## 🐛 Problemas Identificados

### Problema 1: `i/o timeout` en Registro SIP

**Síntoma**:
```
failed to register: failed to read REGISTER response: 
read tcp 192.168.1.38:34174->192.168.1.38:5060: i/o timeout
```

**Posibles causas**:
1. flexisip no está procesando REGISTERs entrantes
2. El REGISTER se envía a una IP/puerto incorrecto
3. Firewall interno bloquea la conexión
4. flexisip requiere parámetros adicionales en REGISTER

**Evidencia**:
- flexisip está escuchando en 5060
- No hay logs en flexisip de REGISTER recibido
- El timeout es de ~10 segundos (default TCP)

---

### Problema 2: Proceso Bridge Se Cae Silenciosamente

**Síntoma**:
- Proceso inicia normalmente
- Logs se detienen abruptamente
- No hay mensaje de error o panic
- Proceso desaparece de `ps aux`

**Posibles causas**:
1. Panic en Go no capturado
2. Segfault en código C (si hay CGO)
3. Out of memory (OOM killer)
4. Señal externa (kill)

**Evidencia**:
- No hay logs de "fatal" o "panic"
- No hay core dump generado
- Memoria disponible: ~425 MB (suficiente)

---

## 📊 Estado Actual

| Componente | Estado | Notas |
|------------|--------|-------|
| **flexisip** | ✅ Corriendo | PID 11415, 11416 |
| **Puertos SIP** | ✅ Escuchando | 5060, 5061 |
| **Configuración** | ✅ Correcta | Auth, usuarios, dominios |
| **Bridge** | ❌ Caído | Se cae después de ~30s |
| **Registro SIP** | ❌ Fallido | i/o timeout |
| **Logs flexisip** | ❌ Vacíos | No muestra REGISTERs |

---

## 🔧 Próximos Pasos Recomendados

### 1. Verificar si flexisip recibe REGISTERs

```bash
# En el dispositivo, con tcpdump si está disponible:
tcpdump -i lo -n port 5060 -X

# O usar strace en flexisip:
strace -p <PID_flexisip> -e trace=network
```

### 2. Ejecutar bridge en foreground para ver panic

```bash
cd /home/bticino/cfg/extra
./bticino_bridge -config config_streaming.yaml
# Observar salida estándar para panic
```

### 3. Verificar OOM killer

```bash
dmesg | grep -i 'out of memory\|killed process'
cat /proc/meminfo | grep -i 'memavailable'
```

### 4. Probar cliente SIP alternativo

```bash
# Usar sipsak o pjsua para testear registro:
sipsak -s sip:baresip@1754162.bs.iotleg.com -u baresip -p test123 -P tcp
```

### 5. Habilitar logs más detallados en flexisip

```ini
# Agregar a flexisip.conf
[module::SipAg]
log-level=9
```

---

## 📁 Archivos de Configuración Actuales

### /etc/flexisip/flexisip.conf
```ini
[global]
aliases=1754162.bs.iotleg.com
transports=sip:*:5060
log-level=debug
syslog-level=debug

[module::Authentication]
enabled=true
auth-domains=1754162.bs.iotleg.com
datasource=/etc/flexisip/users/users.db.txt
trusted-hosts=127.0.0.1,192.168.1.38
hashed-passwords=true
```

### /home/bticino/cfg/extra/config_streaming.yaml
```yaml
sip:
  enabled: true
  server_host: 192.168.1.38
  server_port: 5060
  transport: tcp
  domain: 1754162.bs.iotleg.com
  username: baresip_test@1754162.bs.iotleg.com
  password: test123
  use_ha1: false
```

---

## 📝 Conclusiones

1. **flexisip está configurado correctamente** pero no responde a REGISTERs
2. **El bridge tiene un bug** que causa caída silenciosa después de iniciar
3. **Se requiere depuración a nivel de red** (tcpdump) para ver si REGISTER sale del bridge
4. **Se necesita ejecutar bridge en foreground** para capturar panic

**Próxima sesión**: Ejecutar bridge en foreground + tcpdump para capturar tráfico SIP

---

**Fecha**: 2026-03-24  
**Pruebas realizadas**: 24  
**Estado**: ⏸️ Pendiente de más depuración  
**Prioridad**: Media (streaming es feature opcional)
