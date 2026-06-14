# SIP Registration Debug Plan

**Fecha**: 2026-03-25  
**Estado**: ⏳ **PENDIENTE**  
**Prioridad**: Media (bridge funcional sin SIP)

---

## 📋 Estado Actual

### **Lo Que Sabemos**:

1. ✅ **Flexisip configurado** - Escuchando en 192.168.1.38:5060/5061
2. ✅ **Usuario creado** - `bticino_bridge@1754162.bs.iotleg.com`
3. ✅ **Ruta configurada** - `sip:127.0.0.1`
4. ✅ **Timbre configurado** - En route_int.conf
5. ❌ **Registro falla** - `i/o timeout` después de 10 segundos
6. ❌ **Sin logs en flexisip** - No recibe REGISTER

---

## 🎯 Objetivo

Hacer que `bticino_bridge` se registre exitosamente con `flexisip` local para habilitar:
- Streaming RTSP
- Grabación HKSV
- Llamadas salientes/entrantes

---

## 🔍 Hipótesis

### **Hipótesis 1: Cliente SIP no envía REGISTER**
**Probabilidad**: 30%  
**Test**: Capturar tráfico de red con tcpdump

### **Hipótesis 2: Flexisip no responde REGISTER**
**Probabilidad**: 40%  
**Test**: Logs detallados de flexisip (nivel 9)

### **Hipótesis 3: Formato de username incorrecto**
**Probabilidad**: 20%  
**Test**: Probar con usuario existente (baresip)

### **Hipótesis 4: Firewall interno bloquea**
**Probabilidad**: 10%  
**Test**: Verificar iptables rules

---

## 🧪 Plan de Testing

### **Fase 1: Captura de Tráfico** ⏳

**Objetivo**: Verificar si REGISTER sale del bridge

```bash
# En el dispositivo
ssh bticino

# Iniciar captura
tcpdump -i lo -n -s 0 -w /tmp/sip_register.pcap port 5060

# En otra terminal, reiniciar bridge
pkill -9 bticino_bridge
cd /home/bticino/cfg/extra
./bticino_bridge -config config_streaming.yaml &

# Detener captura después de 15 segundos
killall tcpdump

# Analizar captura
tcpdump -r /tmp/sip_register.pcap -X | head -100
```

**Resultado Esperado**:
- ✅ Debería mostrar `REGISTER sip:1754162.bs.iotleg.com SIP/2.0`
- ❌ Si no muestra nada, el bridge no está enviando

---

### **Fase 2: Logs Detallados de Flexisip** ⏳

**Objetivo**: Ver qué recibe flexisip

```bash
# Editar flexisip.conf
ssh bticino
cat > /etc/flexisip/flexisip.conf << 'EOF'
[global]
aliases=1754162.bs.iotleg.com
transports=sip:*:5060
log-level=debug
syslog-level=debug

[module::SipAg]
log-level=9

[module::Authentication]
enabled=true
auth-domains=1754162.bs.iotleg.com
trusted-hosts=127.0.0.1,192.168.1.38
hashed-passwords=true
EOF

# Reiniciar flexisip
/etc/init.d/flexisipsh restart

# Monitorear logs
tail -f /var/log/log_rotation.log | grep -i register
```

**Resultado Esperado**:
- ✅ Debería mostrar REGISTER recibido
- ❌ Si no muestra, flexisip no recibe

---

### **Fase 3: Probar con Usuario Existente** ⏳

**Objetivo**: Descartar problema de configuración

```bash
# Editar config del bridge
ssh bticino
cat > /home/bticino/cfg/extra/config_test.yaml << 'EOF'
sip:
  enabled: true
  server_host: 127.0.0.1
  server_port: 5060
  transport: tcp
  domain: 1754162.bs.iotleg.com
  username: baresip@1754162.bs.iotleg.com
  password: 2db500964b556d4cef79284738f2a360
  use_ha1: true
EOF

# Reiniciar bridge
pkill -9 bticino_bridge
cd /home/bticino/cfg/extra
./bticino_bridge -config config_test.yaml &

# Ver logs
tail -f /var/log/bticino_bridge.log | grep -i sip
```

**Resultado Esperado**:
- ✅ Si funciona con baresip → problema es configuración de bticino_bridge
- ❌ Si falla igual → problema es cliente SIP del bridge

---

### **Fase 4: Test con Sipsak** ⏳

**Objetivo**: Testear registro con herramienta externa

```bash
# Desde otra máquina (no el dispositivo)
# Instalar sipsak
sudo apt-get install sipsak

# Probar registro
sipsak -s sip:bticino_bridge@1754162.bs.iotleg.com \
       -u bticino_bridge \
       -p 2db500964b556d4cef79284738f2a360 \
       -P tcp \
       -a 192.168.1.38 \
       -v
```

**Resultado Esperado**:
- ✅ `SIP/2.0 200 OK` → Flexisip funciona
- ❌ `SIP/2.0 403 Forbidden` → Problema de auth
- ❌ Timeout → Flexisip no responde

---

### **Fase 5: Debug del Código SIP** ⏳

**Objetivo**: Agregar logs al cliente SIP del bridge

```bash
# En el código Go del bridge
# pkg/sip/client.go

func (c *BTicinoSIPClient) register() error {
    c.logger.Debug("Starting SIP registration...")  # AGREGAR
    c.logger.Debugf("Server: %s:%d", c.config.ServerHost, c.config.ServerPort)  # AGREGAR
    c.logger.Debugf("Username: %s", c.config.Username)  # AGREGAR
    c.logger.Debugf("Domain: %s", c.config.Domain)  # AGREGAR
    
    // ... resto del código
}
```

**Compilar con logs**:
```bash
go build -tags debug -o bticino_bridge_debug ./cmd/main.go
```

---

## 🔧 Soluciones Potenciales

### **Solución 1: Cambiar formato de username**

```yaml
# Opción A: Sin dominio en username
sip:
  username: bticino_bridge
  domain: 1754162.bs.iotleg.com

# Opción B: Con dominio completo
sip:
  username: bticino_bridge@1754162.bs.iotleg.com
  domain: 1754162.bs.iotleg.com
```

### **Solución 2: Cambiar transporte**

```yaml
# Probar UDP en lugar de TCP
sip:
  transport: udp
  server_port: 5060
```

### **Solución 3: Deshabilitar hashed-passwords**

```ini
# En flexisip.conf
[module::Authentication]
hashed-passwords=false
```

### **Solución 4: Agregar usuario sin hash**

```bash
# En users.db.txt
bticino_bridge@1754162.bs.iotleg.com plain:password123 ;
```

### **Solución 5: Usar IP en lugar de localhost**

```yaml
sip:
  server_host: 192.168.1.38  # En lugar de 127.0.0.1
```

---

## 📊 Timeline Estimado

| Fase | Duración | Prioridad |
|------|----------|-----------|
| **Fase 1: Captura** | 30 min | 🔴 Alta |
| **Fase 2: Logs Flexisip** | 30 min | 🔴 Alta |
| **Fase 3: Usuario Existente** | 30 min | 🟡 Media |
| **Fase 4: Sipsak** | 1 hora | 🟡 Media |
| **Fase 5: Debug Código** | 2 horas | 🟢 Baja |

**Total estimado**: 4-5 horas

---

## 🎯 Criterios de Éxito

### **Registro Exitoso**:
```
✅ Logs muestran: "SIP registration successful"
✅ API muestra: "sip_registered": true
✅ Flexisip logs muestran: REGISTER 200 OK
✅ No más errores de timeout
```

### **Funcionalidad Habilitada**:
```
✅ RTSP streaming funciona
✅ Grabación HKSV disponible
✅ Llamadas entrantes/salientes
```

---

## 📁 Recursos Necesarios

### **Herramientas**:
- [x] tcpdump (o wireshark)
- [x] sipsak
- [x] SSH al dispositivo
- [x] Binario compilado v0.13.0

### **Accesos**:
- [x] SSH: `ssh bticino`
- [x] Web: `http://192.168.1.38:8082`
- [x] Logs: `/var/log/bticino_bridge.log`

---

## 🔄 Plan B

Si el registro SIP local no funciona después de debug:

### **Opción A: Usar servidor SIP externo**
```yaml
sip:
  server_host: sipserver.bs.iotleg.com
  server_port: 5061
  transport: tls
```

### **Opción B: Skip SIP por ahora**
- Bridge funcional sin streaming
- Enfocarse en otras features
- Retornar a SIP después

### **Opción C: Implementar cliente SIP alternativo**
- Usar librería Go diferente
- Pion SIP o similar
- Más trabajo pero más control

---

**Próximo Paso**: Comenzar Fase 1 (captura de tráfico)  
**Responsable**: Por definir  
**Fecha Inicio**: Inmediata
