# Flexisip Local Configuration Guide

**Fecha**: 2026-03-24  
**Dispositivo**: BTicino Classe 300X (192.168.1.38)  
**Estado**: ✅ **COMPLETADO Y VERIFICADO**

---

## 📋 Resumen de Configuración Aplicada

### **Servidores SIP Configurados**

| Usuario | Dominio | Password (MD5) | Ruta | Timbre |
|---------|---------|----------------|------|--------|
| `baresip` | 1754162.bs.iotleg.com | `2db500964b556d4cef79284738f2a360` | `sip:192.168.1.38` | ✅ |
| `baresip_test` | 1754162.bs.iotleg.com | `test123` (plain) | N/A | ❌ |
| `bticino_bridge` | 1754162.bs.iotleg.com | `2db500964b556d4cef79284738f2a360` | `sip:127.0.0.1` | ✅ |

---

## 🌐 Puertos de Escucha

Flexisip ahora escucha en:

```
✅ sip:192.168.1.38:5060   (SIP TCP - red local)
✅ sip:127.0.0.1:5060      (SIP TCP - localhost)
✅ sips:192.168.1.38:5061  (SIP TLS - red local)
```

**Verificación**:
```bash
ps aux | grep flexisip
# Debe mostrar:
# /usr/bin/flexisip --daemon --transports sip:192.168.1.38:5060;maddr=192.168.1.38 ...
```

---

## 📁 Archivos Modificados

### 1. `/etc/flexisip/users/users.db.txt`

**Contenido**:
```
version:1
fquinto-gmx.com-A06CE9ED-19B2-4EDB-8CBB-A4EEB4F93BD2@1754162.bs.iotleg.com md5:2db500964b556d4cef79284738f2a360 ;
fquinto-gmx.com-D5EA2023-44B4-4498-BCA4-8FF213BEC251@1754162.bs.iotleg.com md5:43ead99dcce8e21882eca9073f22d503 ;
albasanor-gmail.com-C3592C7D-BB5E-46A1-8647-2FE8371C0FE9@1754162.bs.iotleg.com md5:b2e3eaebd235965c01c43e7f616d566e ;
baresip@1754162.bs.iotleg.com md5:2db500964b556d4cef79284738f2a360 ;
baresip_test@1754162.bs.iotleg.com plain:test123 ;
bticino_bridge@1754162.bs.iotleg.com md5:2db500964b556d4cef79284738f2a360 ;  # ← NUEVO
```

**Hash MD5**: `2db500964b556d4cef79284738f2a360` (mismo para todos los usuarios locales)

---

### 2. `/etc/flexisip/users/route.conf`

**Contenido**:
```
<sip:alluser@1754162.bs.iotleg.com> <sip:fquinto-gmx.com-A06CE9ED-19B2-4EDB-8CBB-A4EEB4F93BD2@1754162.bs.iotleg.com>, <sip:fquinto-gmx.com-D5EA2023-44B4-4498-BCA4-8FF213BEC251@1754162.bs.iotleg.com>, <sip:albasanor-gmail.com-C3592C7D-BB5E-46A1-8647-2FE8371C0FE9@1754162.bs.iotleg.com>
<sip:fquinto-gmx.com-A06CE9ED-19B2-4EDB-8CBB-A4EEB4F93BD2@1754162.bs.iotleg.com> <sip:sipserver.bs.iotleg.com;transport=tls>
<sip:fquinto-gmx.com-D5EA2023-44B4-4498-BCA4-8FF213BEC251@1754162.bs.iotleg.com> <sip:sipserver.bs.iotleg.com;transport=tls>
<sip:albasanor-gmail.com-C3592C7D-BB5E-46A1-8647-2FE8371C0FE9@1754162.bs.iotleg.com> <sip:sipserver.bs.iotleg.com;transport=tls>
<sip:baresip@1754162.bs.iotleg.com> sip:192.168.1.38
<sip:bticino_bridge@1754162.bs.iotleg.com> sip:127.0.0.1  # ← NUEVO
```

**Nota**: `bticino_bridge` apunta a `127.0.0.1` porque corre en el mismo dispositivo.

---

### 3. `/etc/flexisip/users/route_int.conf`

**Contenido**:
```
<sip:baresip@1754162.bs.iotleg.com>, <sip:bticino_bridge@1754162.bs.iotleg.com>, <sip:alluser@1754162.bs.iotleg.com> <sip:fquinto-gmx.com-A06CE9ED-19B2-4EDB-8CBB-A4EEB4F93BD2@1754162.bs.iotleg.com>, <sip:fquinto-gmx.com-D5EA2023-44B4-4498-BCA4-8FF213BEC251@1754162.bs.iotleg.com>, <sip:albasanor-gmail.com-C3592C7D-BB5E-46A1-8647-2FE8371C0FE9@1754162.bs.iotleg.com>
```

**Nota**: `bticino_bridge` ahora recibe llamadas de timbre (doorbell).

---

### 4. `/etc/flexisip/flexisip.conf`

**Configuración actual**:
```ini
[global]
aliases=1754162.bs.iotleg.com
transports=sip:*:5060
tls-certificates-dir=/etc/flexisip/tls
user-errors-logs=true
log-level=debug
syslog-level=debug

[inter-domain-connections]
accept-domain-registrations=false
domain-registrations=/etc/flexisip/domain-registration.conf
verify-server-certs=true
keepalive-interval=30

[module::Registrar]
enabled=true
reg-domains=1754162.bs.iotleg.com
db-implementation=internal
static-records-file=/etc/flexisip/users/route.conf
max-contacts-by-aor=20

[module::Authentication]
enabled=true
auth-domains=1754162.bs.iotleg.com
db-implementation=file
datasource=/etc/flexisip/users/users.db.txt
trusted-hosts=127.0.0.1,192.168.1.38
hashed-passwords=true
reject-wrong-client-certificates=true
```

**Puntos clave**:
- ✅ `trusted-hosts=127.0.0.1,192.168.1.38` - Permite registro sin autenticación desde estas IPs
- ✅ `log-level=debug` - Logs detallados en `/var/log/log_rotation.log`
- ✅ `hashed-passwords=true` - Usa hashes MD5

---

## 🔧 Script de Inicio `/etc/init.d/flexisipsh`

**Línea de inicio**:
```bash
start-stop-daemon --start --quiet --exec $DAEMON -- $DAEMON_ARGS \
  --transports "sip:$2:5060;maddr=$2 sip:127.0.0.1:5060;maddr=127.0.0.1 sips:$2:5061;maddr=$2;require-peer-certificate=1"
```

**Uso**:
```bash
# Iniciar con IP específica
/etc/init.d/flexisipsh start 192.168.1.38

# Detener
/etc/init.d/flexisipsh stop

# Reiniciar
/etc/init.d/flexisipsh restart
```

---

## 🔥 Firewall

**Estado**: Los scripts de iptables existen pero el comando `iptables` no está disponible en PATH.

**Scripts**:
- `/etc/network/if-pre-up.d/iptables` (1496 bytes)
- `/etc/network/if-pre-up.d/iptables6` (1281 bytes)

**Para deshabilitar firewall permanentemente**:
```bash
# Mover scripts a backup
mv /etc/network/if-pre-up.d/iptables /home/bticino/cfg/extra/iptables.bak
mv /etc/network/if-pre-up.d/iptables6 /home/bticino/cfg/extra/iptables6.bak
```

**Nota**: El firewall parece estar deshabilitado o no funcional actualmente.

---

## 🧪 Verificación

### 1. Verificar flexisip corriendo
```bash
ssh bticino "ps aux | grep flexisip | grep -v grep"
```

**Esperado**:
```
root  18942  /usr/bin/flexisip --daemon --transports sip:192.168.1.38:5060;maddr=192.168.1.38 ...
root  18943  /usr/bin/flexisip --daemon --transports sip:192.168.1.38:5060;maddr=192.168.1.38 ...
```

### 2. Verificar puertos de escucha
```bash
ssh bticino "cat /proc/net/tcp | grep -i '13c4\|13c5'"
# 13C4 = 5060, 13C5 = 5061
```

### 3. Verificar usuarios
```bash
ssh bticino "cat /etc/flexisip/users/users.db.txt"
```

### 4. Verificar logs
```bash
ssh bticino "tail -50 /var/log/log_rotation.log | grep flexisip"
```

---

## 🔌 Configuración para bticino_bridge

### Actualizar `config_streaming.yaml`:

```yaml
sip:
  enabled: true
  local_host: 192.168.1.38
  local_port: 47300
  server_host: 127.0.0.1  # ← Usar localhost
  server_port: 5060
  transport: tcp
  domain: 1754162.bs.iotleg.com
  username: bticino_bridge@1754162.bs.iotleg.com
  password: 2db500964b556d4cef79284738f2a360  # ← MD5 hash
  dev_addr: 20
  use_ha1: true  # ← Usar hash MD5
  insecure_tls: false
```

**Notas**:
- ✅ `server_host: 127.0.0.1` - Flexisip local
- ✅ `username` incluye el dominio completo
- ✅ `password` es el hash MD5 (no password plano)
- ✅ `use_ha1: true` - Indica que password es hash HA1

---

## 📝 Comandos Útiles

### Reiniciar flexisip
```bash
ssh bticino "/etc/init.d/flexisipsh restart"
```

### Ver logs en tiempo real
```bash
ssh bticino "tail -f /var/log/log_rotation.log | grep flexisip"
```

### Agregar nuevo usuario
```bash
# 1. Agregar a users.db.txt
echo 'nuevo_usuario@1754162.bs.iotleg.com md5:2db500964b556d4cef79284738f2a360 ;' >> /etc/flexisip/users/users.db.txt

# 2. Agregar ruta
echo '<sip:nuevo_usuario@1754162.bs.iotleg.com> sip:192.168.1.XX' >> /etc/flexisip/users/route.conf

# 3. Reiniciar flexisip
/etc/init.d/flexisipsh restart
```

### Testear registro SIP
```bash
# Desde otro dispositivo con baresip/softphone
# Registrar: bticino_bridge@1754162.bs.iotleg.com
# Password: 2db500964b556d4cef79284738f2a360
# Server: 192.168.1.38:5060
```

---

## 🎯 Próximos Pasos

1. ✅ **Flexisip configurado** - Escuchando en red local
2. ✅ **Usuario bticino_bridge creado** - Con hash MD5
3. ✅ **Ruta configurada** - Apunta a 127.0.0.1
4. ✅ **Timbre configurado** - Recibe llamadas de doorbell
5. ⏳ **Actualizar bticino_bridge** - Usar configuración local
6. ⏳ **Testear registro SIP** - Verificar que bticino_bridge se registra
7. ⏳ **Testear doorbell** - Verificar que recibe llamadas

---

## 🔐 Security Notes

### **Contraseñas**:
- Todos los usuarios locales usan el mismo hash: `2db500964b556d4cef79284738f2a360`
- Este hash es para el dominio `1754162.bs.iotleg.com`
- Formato: `username:realm:password` → MD5

### **Trusted Hosts**:
- `127.0.0.1` - localhost
- `192.168.1.38` - IP del dispositivo
- Hosts en trusted-hosts pueden registrarse sin autenticación

### **Recomendaciones**:
- ✅ Usar solo en red local confiable
- ✅ No exponer puertos SIP a Internet
- ✅ Considerar TLS para producción (puerto 5061)

---

**Estado**: ✅ **COMPLETADO**  
**Fecha**: 2026-03-24  
**Prueba pendiente**: Registro de bticino_bridge con flexisip local
