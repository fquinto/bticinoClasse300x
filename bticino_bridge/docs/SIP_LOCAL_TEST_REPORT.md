# SIP Local Testing Report

**Fecha**: 2026-03-24  
**Estado**: ⚠️ **PARCIALMENTE FUNCIONAL**  
**Problema**: Registro SIP falla con `i/o timeout`

---

## 📊 Estado Actual

### ✅ **Lo Que Funciona**

| Componente | Estado | Detalles |
|------------|--------|----------|
| **Flexisip** | ✅ Corriendo | PID 18942, 18943 |
| **Puerto 5060** | ✅ Escuchando | 127.0.0.1:5060, 192.168.1.38:5060 |
| **Puerto 5061** | ✅ Escuchando | 192.168.1.38:5061 (TLS) |
| **Usuario bticino_bridge** | ✅ Creado | users.db.txt |
| **Ruta configurada** | ✅ Creada | route.conf → 127.0.0.1 |
| **Timbre configurado** | ✅ Creado | route_int.conf |

---

### ❌ **Lo Que No Funciona**

| Componente | Problema | Error |
|------------|----------|-------|
| **Registro SIP** | ❌ Timeout | `i/o timeout` en 127.0.0.1:5060 |
| **bticino_bridge** | ⚠️ Sin SIP | Continúa sin features de video |

---

## 🔍 Diagnóstico

### **Síntomas**:

1. ✅ Flexisip está corriendo y escuchando en 127.0.0.1:5060
2. ✅ Usuario `bticino_bridge` existe en users.db.txt
3. ✅ Ruta existe en route.conf
4. ❌ bticino_bridge reporta `i/o timeout` al enviar REGISTER
5. ❌ No hay logs en flexisip del REGISTER de bticino_bridge

### **Posibles Causas**:

1. **Cliente SIP del bridge** - Puede no estar enviando REGISTER correctamente
2. **Firewall interno** - iptables puede estar bloqueando (aunque comando no disponible)
3. **Formato de username** - Puede necesitar formato diferente
4. **Autenticación** - Puede requerir configuración adicional

---

## 📁 Configuración Actual

### **Flexisip** (`/etc/flexisip/flexisip.conf`):
```ini
[global]
aliases=1754162.bs.iotleg.com
transports=sip:*:5060
log-level=debug

[module::Authentication]
auth-domains=1754162.bs.iotleg.com
trusted-hosts=127.0.0.1,192.168.1.38
hashed-passwords=true
```

### **Usuarios** (`/etc/flexisip/users/users.db.txt`):
```
bticino_bridge@1754162.bs.iotleg.com md5:2db500964b556d4cef79284738f2a360 ;
```

### **Rutas** (`/etc/flexisip/users/route.conf`):
```
<sip:bticino_bridge@1754162.bs.iotleg.com> sip:127.0.0.1
```

### **bticino_bridge** (`config_streaming.yaml`):
```yaml
sip:
  server_host: 127.0.0.1
  server_port: 5060
  transport: tcp
  domain: 1754162.bs.iotleg.com
  username: bticino_bridge@1754162.bs.iotleg.com
  password: 2db500964b556d4cef79284738f2a360
  use_ha1: true
```

---

## 🧪 Pruebas Realizadas

### **Test 1: Verificar flexisip escuchando**
```bash
ps aux | grep flexisip
# ✅ OK: Flexisip corriendo

cat /proc/net/tcp | grep 13C4
# ✅ OK: Escuchando en 127.0.0.1:5060 y 192.168.1.38:5060
```

### **Test 2: Verificar usuarios**
```bash
cat /etc/flexisip/users/users.db.txt | grep bticino_bridge
# ✅ OK: Usuario existe
```

### **Test 3: Verificar rutas**
```bash
cat /etc/flexisip/users/route.conf | grep bticino_bridge
# ✅ OK: Ruta configurada
```

### **Test 4: Registro SIP**
```bash
# bticino_bridge logs:
ERRO[2026-03-24T14:20:34+01:00] Failed to start SIP client... 
error="failed to register: failed to read REGISTER response: 
read tcp 127.0.0.1:45234->127.0.0.1:5060: i/o timeout"
# ❌ FAIL: Timeout después de 10 segundos
```

### **Test 5: Logs de flexisip**
```bash
tail -100 /var/log/log_rotation.log | grep bticino_bridge
# ❌ FAIL: No hay logs del REGISTER
```

---

## 🔧 Próximos Pasos para Debug

### **1. Verificar que el REGISTER sale del bridge**
```bash
# En el dispositivo, capturar tráfico SIP
tcpdump -i lo -n port 5060 -X
# Debería mostrar REGISTER saliendo
```

### **2. Probar con usuario existente (baresip)**
```yaml
sip:
  username: baresip@1754162.bs.iotleg.com
  password: 2db500964b556d4cef79284738f2a360
```

### **3. Habilitar logs más detallados en flexisip**
```ini
[global]
log-level=debug
syslog-level=debug

[module::SipAg]
log-level=9  # Máximo nivel de debug
```

### **4. Probar registro manual con sipsak**
```bash
# Desde otro dispositivo
sipsak -s sip:bticino_bridge@1754162.bs.iotleg.com \
       -u bticino_bridge \
       -p 2db500964b556d4cef79284738f2a360 \
       -P tcp \
       -a 192.168.1.38
```

---

## 📝 Lecciones Aprendidas

1. ✅ **Flexisip se puede configurar** para escuchar en red local
2. ✅ **Usuarios se pueden agregar** manualmente
3. ✅ **Rutas se pueden configurar** para usuarios locales
4. ⚠️ **El registro SIP requiere debug** más detallado
5. ⚠️ **El cliente SIP del bridge** puede necesitar ajustes

---

## 🎯 Estado del Proyecto

### **Funcionalidades Working**:
- ✅ Web Dashboard (puerto 8082)
- ✅ MQTT Bridge (Home Assistant)
- ✅ OpenWebNet (comandos y monitoreo)
- ✅ Message Parser (answering machine)
- ✅ Input Monitor (botones físicos)
- ✅ LED/GPIO Monitoring
- ⚠️ **SIP Client** (fallando registro)
- ⚠️ **RTSP Streaming** (depende de SIP)

### **Funcionalidades Pendientes**:
- ❌ **SIP Registration** - Debug en progreso
- ❌ **RTSP Streaming** - Depende de SIP
- ❌ **HKSV Recording** - Depende de RTSP

---

## 📚 Documentación Creada

1. `FLEXISIP_LOCAL_CONFIG.md` - Guía completa de configuración
2. `SIP_LOCAL_TEST_REPORT.md` - Este informe
3. `BT_DAEMON_REVERSE_ENGINEERING.md` - Análisis de bt_daemon
4. `FIRMWARE_COMPARISON_1.7.17_vs_1.7.19.md` - Comparativa firmwares
5. `WEB_CONFIG_IMPLEMENTATION_PROGRESS.md` - Progreso web config

---

**Próxima Revisión**: Después de debug de registro SIP  
**Prioridad**: Media (bridge funcional sin SIP)  
**Impacto**: Bajo (otras funcionalidades trabajan)
