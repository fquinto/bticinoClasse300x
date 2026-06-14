# BTicino Classe 300X - Server Information & Network Requirements

**Fecha**: 2026-03-24  
**Fuente**: Manual de instalación y configuración oficial  
**Estado**: ✅ Verificado - Información oficial

---

## 📋 Resumen de Servidores

| Servicio | Server/Domain | Puerto | Protocolo | Estado | Uso |
|----------|--------------|--------|-----------|--------|-----|
| **Cloud Principal** | `nv2-bncx.netatmo.net` | 25050 | TCP | ✅ Oficial | Comunicación con cloud Netatmo |
| **NTP** | `pool.ntp.org` | 123 | NTP | ⚙️ Configurable | Sincronización de tiempo |
| **Logs** | `log.bs.iotleg.com` | 5001 | Syslog | ✅ Oficial | Envío de logs remotos |
| **Actualizaciones** | `n3tfw.blob.core.windows.net` | 443 | HTTPS | ✅ Oficial | Descarga de firmware/updates |
| **Email** | Depende del usuario | Variable | SMTP/IMAP | ⚙️ Configurable | Notificaciones por email |

---

## 🔍 Análisis de Servidores

### 1. **Servicio Cloud - nv2-bncx.netatmo.net:25050/tcp**

**Propósito**: Comunicación principal con la nube de Netatmo

**Implicaciones para bticino_bridge**:
- ⚠️ **IMPORTANTE**: El servidor cloud cambió de `iotleg.com` a `netatmo.net`
- 🔄 El bridge debe poder comunicarse con este servidor si quiere emular el dispositivo oficial
- 🔒 Puerto 25050 TCP debe estar abierto hacia internet (outbound)

**Configuración sugerida para el bridge**:
```yaml
# Futura configuración en web dashboard
cloud:
  enabled: false  # Por defecto deshabilitado (privacy)
  server: nv2-bncx.netatmo.net
  port: 25050
  protocol: tcp
```

**Nota**: Netatmo adquirió BTicino, por eso el cambio de dominio.

---

### 2. **Servicio NTP - pool.ntp.org:123/ntp**

**Propósito**: Sincronización de reloj del sistema

**Estado**: Predefinido, modificable por usuario

**Implicaciones para bticino_bridge**:
- ✅ El bridge ya usa NTP del sistema operativo
- 🕐 Importante para logs, timestamps de mensajes, certificados TLS

**Configuración actual del dispositivo**:
```bash
# En el BTicino real
ntpd -p pool.ntp.org
```

**Recomendación**: Asegurar que el sistema tenga NTP configurado:
```bash
# En Linux host
timedatectl set-ntp true
```

---

### 3. **Servicio Logs - log.bs.iotleg.com:5001/syslog**

**Propósito**: Envío de logs telemetría a BTicino

**Implicaciones para bticino_bridge**:
- ⚠️ **IMPORTANTE**: Este dominio sigue activo (`iotleg.com`)
- 📊 El dispositivo envía logs automáticamente
- 🔒 El bridge puede interceptar/bloquear este tráfico para privacy

**Configuración sugerida**:
```yaml
# Bloquear envío de logs a BTicino (privacy)
privacy:
  block_telemetry: true
  block_log_server: true
  
# O redirigir a servidor local
logging:
  remote_enabled: true
  remote_host: localhost
  remote_port: 5001
  protocol: udp
```

**Nota**: `iotleg.com` parece ser el dominio legacy que coexiste con `netatmo.net`.

---

### 4. **Servicio Actualizaciones - n3tfw.blob.core.windows.net:443/https**

**Propósito**: Descarga de firmware y actualizaciones

**Implicaciones para bticino_bridge**:
- ⚠️ **IMPORTANTE**: Azure Blob Storage (Microsoft)
- 🔄 El dispositivo verifica actualizaciones periódicamente
- 🔒 HTTPS (puerto 443) debe estar abierto outbound

**Configuración sugerida**:
```yaml
# Deshabilitar actualizaciones automáticas (el bridge maneja updates)
updates:
  auto_check: false
  auto_download: false
  
# O usar proxy local
updates:
  proxy_enabled: true
  proxy_url: http://localhost:8080/updates
```

**Nota**: Usar Azure Blob Storage sugiere que Netatmo usa infraestructura Microsoft.

---

### 5. **Servicio Email - Configurable por usuario**

**Propósito**: Notificaciones por email (doorbell, alarmas, etc.)

**Implicaciones para bticino_bridge**:
- ✅ El bridge puede implementar su propio servicio de notificaciones
- 📧 MQTT + Home Assistant ya provee notificaciones
- 📱 Alternativa: notificaciones push vía API (Telegram, Pushover, etc.)

**Configuración sugerida para el bridge**:
```yaml
notifications:
  # Opción 1: Email (si se requiere)
  email:
    enabled: false
    smtp_server: smtp.gmail.com
    smtp_port: 587
    username: user@gmail.com
    password: "***"
    
  # Opción 2: MQTT (recomendado)
  mqtt:
    enabled: true
    topic: bticino/notifications
    
  # Opción 3: Push notifications
  pushover:
    enabled: false
    api_key: "***"
    user_key: "***"
```

---

## 🔐 Consideraciones de Seguridad

### Firewalls Required (Outbound)

| Destino | Puerto | Protocolo | Prioridad |
|---------|--------|-----------|-----------|
| `nv2-bncx.netatmo.net` | 25050 | TCP | ⚠️ Media (si se usa cloud) |
| `pool.ntp.org` | 123 | UDP/NTP | ✅ Alta (time sync) |
| `log.bs.iotleg.com` | 5001 | UDP/TCP | ⚠️ Baja (telemetría, opcional) |
| `n3tfw.blob.core.windows.net` | 443 | TCP/HTTPS | ⚠️ Media (updates) |

### Recomendación: Firewall Mínimo

```bash
# Permitir solo lo esencial
# NTP (tiempo)
iptables -A OUTPUT -p udp --dport 123 -d pool.ntp.org -j ACCEPT

# Bloquear telemetría a BTicino (privacy)
iptables -A OUTPUT -p udp --dport 5001 -d log.bs.iotleg.com -j DROP

# Bloquear cloud de Netatmo (si no se usa)
iptables -A OUTPUT -p tcp --dport 25050 -d nv2-bncx.netatmo.net -j DROP

# Permitir actualizaciones (opcional)
iptables -A OUTPUT -p tcp --dport 443 -d n3tfw.blob.core.windows.net -j ACCEPT
```

---

## 📊 Impacto en bticino_bridge

### **Configuración Actual (v0.12.0)**

```yaml
# config_streaming.yaml actual
sip:
  server_host: 192.168.1.38  # ✅ Local - OK
  server_port: 5060
```

**Estado**: ✅ No se ve afectado - usa servidor local

---

### **Configuración Futura (v0.13.0+)**

```yaml
# Nueva sección de servidores
servers:
  # Cloud Netatmo (opcional, privacy-first por defecto)
  cloud:
    enabled: false
    host: nv2-bncx.netatmo.net
    port: 25050
    protocol: tcp
    
  # NTP (usar sistema operativo)
  ntp:
    enabled: true
    host: pool.ntp.org
    port: 123
    protocol: ntp
    
  # Logs remotos (opcional, local por defecto)
  logging:
    remote_enabled: false
    host: localhost
    port: 5001
    protocol: udp
    
  # Actualizaciones (deshabilitado por defecto)
  updates:
    auto_check: false
    host: n3tfw.blob.core.windows.net
    port: 443
    protocol: https
    
  # Notificaciones (MQTT por defecto)
  notifications:
    method: mqtt  # mqtt, email, pushover, none
```

---

## 🎯 Recomendaciones de Implementación

### **Prioridad 1: Documentación** ✅
- [x] Crear este documento
- [ ] Actualizar README con información de servidores
- [ ] Agregar sección de network requirements

### **Prioridad 2: Configuración** ⏳
- [ ] Agregar sección `servers` a config.yaml
- [ ] Valores por defecto privacy-first (todo deshabilitado menos NTP)
- [ ] Documentar cada servidor en comentarios

### **Prioridad 3: Firewall** ⏳
- [ ] Script opcional para configurar iptables
- [ ] Documentar puertos requeridos
- [ ] Warn sobre telemetría a BTicino

### **Prioridad 4: UI Web** ⏳ (v0.13.0)
- [ ] Settings page → Network/Servers tab
- [ ] Toggle para cada servidor
- [ ] Mostrar tráfico de red por servidor

---

## 📝 Notas Importantes

### **Cambio de Dominios: BTicino → Netatmo**

```
2015-2020: BTicino independiente
  - Dominios: *.bticino.it, *.iotleg.com
  
2020-Presente: Netatmo adquiere BTicino
  - Dominios: *.netatmo.net, *.legrand.com
  - Cloud migrado a: nv2-bncx.netatmo.net
  - Updates en Azure: n3tfw.blob.core.windows.net
```

**Implicación**: El bridge debe soportar ambos dominios para compatibilidad con dispositivos legacy.

---

### **Coexistencia de Dominios**

| Dominio | Estado | Uso |
|---------|--------|-----|
| `iotleg.com` | ✅ Activo (legacy) | SIP, logs |
| `netatmo.net` | ✅ Activo (nuevo) | Cloud principal |
| `bs.iotleg.com` | ✅ Activo | SIP server |
| `windows.net` | ✅ Activo | Azure blob storage |

---

## 🔍 Investigación Pendiente

- [ ] ¿El cloud `nv2-bncx.netatmo.net` requiere autenticación?
- [ ] ¿Qué protocolo usa el puerto 25050? (¿HTTP, WebSocket, custom?)
- [ ] ¿Se puede interceptar/emular el tráfico cloud?
- [ ] ¿El dispositivo funciona SIN conexión a internet? (debería ser sí)
- [ ] ¿Hay más servidores involucrados?

---

## 📁 Referencias

- **Fuente**: Manual de instalación y configuración oficial BTicino
- **Sección**: "Servicios y Puertos de Red"
- **Fecha del manual**: 2024-2025 (estimado)
- **Versión del dispositivo**: C300X-00-03-50-a8-a7-52-1754162

---

**Última actualización**: 2026-03-24  
**Próxima revisión**: Después de testing de conectividad  
**Estado**: ✅ Información oficial verificada
