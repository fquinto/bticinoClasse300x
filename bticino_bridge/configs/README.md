# BTicino Bridge - Configuración Única

**IMPORTANTE**: Solo existe UN archivo de configuración válido:

```
config.yaml  ← ÚNICO archivo de configuración
```

## 📁 Estructura

```
configs/
├── config.yaml              # ← ÚNICO archivo de configuración (plantilla, sin secretos)
├── secrets.example.yaml     # plantilla de secretos (placeholders)
├── secrets.yaml             # secretos reales — gitignored, 0600 (lo creas tú)
└── homeassistant/           # Configuraciones de Home Assistant (opcional)
```

## 🔐 Secretos

`config.yaml` es una **plantilla**: no contiene credenciales. Todas las
credenciales reales (dominio/HA1 SIP, MQTT, PIN HomeKit, auth web, tokens) viven
en **un único fichero**, `secrets.yaml`, junto a `config.yaml`:

```bash
cp configs/secrets.example.yaml configs/secrets.yaml
chmod 600 configs/secrets.yaml      # y rellenar los valores
```

- `secrets.yaml` está en `.gitignore` y **nunca se commitea**.
- Al arrancar, el bridge superpone `secrets.yaml` sobre `config.yaml`.
- Si falta un secreto de un subsistema activado, el bridge **no arranca** y
  nombra las claves que faltan (fail-closed).
- El dashboard web no muestra ni modifica secretos.

Detalles y migración del dispositivo: `docs/SECRETS.md`.

## 🚀 Uso

### **En tu máquina de desarrollo**:
```bash
# El archivo configs/config.yaml se usa como template
# Para deploy, copiar al dispositivo
```

### **En el dispositivo**:
```bash
# Ubicación: /home/bticino/cfg/extra/configs/config.yaml
#            /home/bticino/cfg/extra/configs/secrets.yaml  (0600, credenciales)
# El bridge se inicia desde /home/bticino/cfg/extra sin argumentos
# (por defecto lee configs/config.yaml y el secrets.yaml que tenga al lado):
./bticino_bridge
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
15. **Health** - Vigilante opcional de SSH (`health.ssh_watchdog`)

En **OpenWebNet**, `door_address` y `light_address` van vacías por defecto: el
bridge lee las direcciones del `conf.xml` del equipo. Rellénalas solo para
forzarlas.

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
nano configs/config.yaml   # credenciales: configs/secrets.yaml
# Reiniciar bridge
pkill -9 bticino_bridge
./bticino_bridge &
```

---

**Última actualización**: 2026-03-26  
**Versión**: v0.13.0
