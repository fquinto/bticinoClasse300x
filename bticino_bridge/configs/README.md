# BTicino Bridge - Configuración Única

**IMPORTANTE**: Solo existe UN archivo de configuración válido:

```
config.yaml  ← ÚNICO archivo de configuración
```

## 📁 Estructura

```
configs/
├── config.yaml              # ← ÚNICO archivo de configuración
└── homeassistant/           # Configuraciones de Home Assistant (opcional)
```

## 🚀 Uso

### **En tu máquina de desarrollo**:
```bash
# El archivo configs/config.yaml se usa como template
# Para deploy, copiar al dispositivo
```

### **En el dispositivo**:
```bash
# Ubicación: /home/bticino/cfg/extra/config.yaml
# El bridge se inicia con:
./bticino_bridge -config config.yaml
```

## 📝 Secciones de Configuración

1. **Bridge** - Nombre, versión, log level
2. **OpenWebNet** - Conexión al bus OpenWebNet
3. **SIP** - Configuración de servidor SIP (flexisip local o externo)
4. **MQTT** - Conexión a Home Assistant
5. **Web** - Dashboard web (puerto, directorios)
6. **HomeKit** - Integración con Apple HomeKit
7. **Hardware** - GPIO, input devices
8. **Logging** - Configuración de logs
9. **Streaming** - RTSP/WebRTC para video
10. **Network** - NTP, DNS, firewall
11. **Servers** - Servidores externos (cloud, updates, SIP)
12. **Notifications** - MQTT, email, push
13. **Privacy** - Privacidad y telemetría
14. **Security** - Autenticación, HTTPS, rate limiting

## ⚠️ Archivos Históricos (ELIMINADOS)

Estos archivos ya NO existen, fueron consolidados en `config.yaml`:

- ❌ `config_ha.yaml` - Eliminado
- ❌ `config_production_homekit.yaml` - Eliminado
- ❌ `config-streaming-example.yaml` - Eliminado
- ❌ `config_with_servers.yaml` - Eliminado
- ❌ `config_streaming.yaml` (dispositivo) - Eliminado

## 🔄 Backup

Los backups se crean automáticamente en:
```
/home/bticino/cfg/extra/backups/config_YYYYMMDD_HHMMSS.yaml
```

## 🛠️ Edición

### **Desde la UI Web** (Recomendado):
```
http://192.168.1.38:8082/settings
```

### **Manual** (SSH):
```bash
ssh bticino
cd /home/bticino/cfg/extra
nano config.yaml
# Reiniciar bridge
pkill -9 bticino_bridge
./bticino_bridge -config config.yaml &
```

---

**Última actualización**: 2026-03-26  
**Versión**: v0.13.0
