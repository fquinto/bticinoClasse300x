# Gestión de Configuración vía Web Dashboard - Análisis de Viabilidad

**Fecha**: 2026-03-24  
**Versión**: bticino_bridge v0.12.0  
**Tipo**: Análisis de evolución arquitectónica

---

## 📊 Resumen Ejecutivo

**¿Es una evolución coherente?** → ✅ **SÍ, ABSOLUTAMENTE**

La gestión de configuración vía web dashboard es una **evolución natural y coherente** para el proyecto bticino_bridge, especialmente considerando:

1. ✅ **Ya existe la infraestructura** (web server, API REST, settings page)
2. ✅ **Resuelve problemas reales** (deploy SSH, edición manual de YAML)
3. ✅ **Alineado con la arquitectura** (todo se gestiona vía web menos config)
4. ✅ **Mejora la experiencia de usuario** significativamente
5. ✅ **Bajo riesgo técnico** (config ya está en memoria, solo falta persistencia)

---

## 🎯 Estado Actual vs Estado Deseado

### **Estado Actual (v0.12.0)**

```
┌─────────────────────────────────────────────────────────┐
│  Web Dashboard (:8082)                                  │
│  ✅ Dashboard - Estado del sistema                      │
│  ✅ Messages - Gestión de mensajes                      │
│  ✅ Controls - Control de dispositivos                  │
│  ✅ Settings - SOLO LECTURA (HTML estático)             │
│  ✅ Logs - Visor de logs                                │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
              ┌─────────────────────────┐
              │  API REST               │
              │  ✅ 24 endpoints        │
              │  ❌ Sin endpoints POST  │
              │     para configuración  │
              └─────────────────────────┘
                            │
                            ▼
              ┌─────────────────────────┐
              │  config.yaml            │
              │  Edición manual vía SSH │
              │  Requiere restart       │
              └─────────────────────────┘
```

**Problemas**:
- ❌ Editar config requiere SSH + editor de texto
- ❌ Requiere restart manual del servicio
- ❌ Sin validación de sintaxis YAML
- ❌ Sin backup automático
- ❌ Sin rollback fácil
- ❌ Error-prone para usuarios no técnicos

---

### **Estado Deseado (v0.13.0)**

```
┌─────────────────────────────────────────────────────────┐
│  Web Dashboard (:8082)                                  │
│  ✅ Dashboard - Estado del sistema                      │
│  ✅ Messages - Gestión de mensajes                      │
│  ✅ Controls - Control de dispositivos                  │
│  ✅ Settings - EDITABLE (formularios + validación)      │
│  ✅ Logs - Visor de logs                                │
│  ✅ Config - Gestión completa (CRUD)                    │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
              ┌─────────────────────────┐
              │  API REST               │
              │  ✅ GET /api/config     │
              │  ✅ POST /api/config    │
              │  ✅ POST /api/config/validate │
              │  ✅ POST /api/config/backup │
              │  ✅ POST /api/config/restart │
              └─────────────────────────┘
                            │
                            ▼
              ┌─────────────────────────┐
              │  Config Manager         │
              │  ✅ Validación YAML     │
              │  ✅ Backup automático   │
              │  ✅ Hot reload          │
              │  ✅ Rollback            │
              └─────────────────────────┘
                            │
                            ▼
              ┌─────────────────────────┐
              │  config.yaml            │
              │  Actualización automática │
              │  Backup: config.yaml.bak │
              └─────────────────────────┘
```

**Beneficios**:
- ✅ Edición desde navegador (sin SSH)
- ✅ Validación en tiempo real
- ✅ Backup automático antes de cambios
- ✅ Hot reload (restart automático)
- ✅ Rollback con 1 click
- ✅ Accesible para usuarios no técnicos

---

## 🔍 Análisis Técnico

### 1. **Infraestructura Existente** ✅

| Componente | Estado | Reutilización |
|------------|--------|---------------|
| Web Server | ✅ Implementado | 100% reutilizable |
| API REST | ✅ 24 endpoints | Patrón existente |
| Settings Page | ✅ HTML existe | Extender con forms |
| Config en Memoria | ✅ `ws.config` | Ya disponible |
| YAML Parser | ✅ `gopkg.in/yaml.v2` | Ya importado |

**Código a escribir**: ~500-800 líneas (estimado)

---

### 2. **Endpoints Necesarios**

```go
// GET /api/config - Obtener configuración actual
GET /api/config
Response: {
  "config": { ... },
  "sections": ["bridge", "openwebnet", "sip", "mqtt", ...],
  "timestamp": "2026-03-24T10:00:00Z"
}

// POST /api/config - Guardar configuración
POST /api/config
Body: { "config": { ... } }
Response: {
  "success": true,
  "message": "Config saved",
  "backup": "config.yaml.20260324_100000.bak",
  "restart_required": true
}

// POST /api/config/validate - Validar configuración
POST /api/config/validate
Body: { "config": { ... } }
Response: {
  "valid": true,
  "errors": [],
  "warnings": ["SIP server not reachable"]
}

// POST /api/config/backup - Crear backup
POST /api/config/backup
Response: {
  "success": true,
  "backup_file": "config.yaml.20260324_100000.bak"
}

// POST /api/config/restore - Restaurar backup
POST /api/config/restore
Body: { "backup_file": "config.yaml.20260324_100000.bak" }
Response: {
  "success": true,
  "message": "Config restored"
}

// POST /api/config/restart - Reiniciar servicio
POST /api/config/restart
Response: {
  "success": true,
  "message": "Restarting..."
}

// GET /api/config/history - Historial de cambios
GET /api/config/history
Response: {
  "history": [
    {"timestamp": "...", "file": "...", "changes": "..."}
  ]
}
```

---

### 3. **UI/UX - Settings Page**

**Secciones editables**:

```
┌─────────────────────────────────────────────────────────┐
│  Settings - bticino_bridge v0.13.0                      │
├─────────────────────────────────────────────────────────┤
│  [Bridge] [OpenWebNet] [SIP] [MQTT] [Web] [Streaming]   │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  === SIP Configuration ===                              │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Enabled:        [✓] Enable SIP                  │   │
│  │ Server Host:    [192.168.1.38         ]         │   │
│  │ Server Port:    [5060                 ]         │   │
│  │ Transport:      [TCP ▼]                         │   │
│  │ Domain:         [1754162.bs.iotleg.com]         │   │
│  │ Username:       [baresip@...]         ]         │   │
│  │ Password:       [••••••••             ] [Show]  │   │
│  │ Use HA1:        [ ] Use pre-computed hash       │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  [Validate] [Save] [Backup] [Cancel]                    │
│                                                         │
│  Last saved: 2026-03-24 10:00:00 by admin              │
│  Backups available: 5                                   │
└─────────────────────────────────────────────────────────┘
```

---

### 4. **Arquitectura Propuesta**

```go
// pkg/webserver/config_manager.go

type ConfigManager struct {
    configPath     string
    backupPath     string
    maxBackups     int
    config         *config.Config
    configMutex    sync.RWMutex
    changeHistory  []ConfigChange
    logger         *logrus.Logger
}

type ConfigChange struct {
    Timestamp   time.Time
    User        string
    Changes     map[string]interface{}
    BackupFile  string
    Restarted   bool
}

// Methods:
// - LoadConfig() *Config
// - SaveConfig(*Config) error
// - ValidateConfig(*Config) ([]error, []warning)
// - CreateBackup() (string, error)
// - RestoreBackup(string) error
// - GetHistory() []ConfigChange
// - HotReload() error
```

---

## 📈 Análisis de Riesgos

### **Riesgos Técnicos**

| Riesgo | Probabilidad | Impacto | Mitigación |
|--------|--------------|---------|------------|
| Config inválida rompe servicio | Media | Alto | Validación + backup automático |
| Hot reload falla | Media | Medio | Restart manual como fallback |
| Pérdida de configuración | Baja | Alto | Múltiples backups + history |
| Race conditions | Media | Medio | Mutex + transacciones |
| Security (auth) | Alta | Alto | Login obligatorio + HTTPS |

---

### **Riesgos de Seguridad**

```yaml
# CRÍTICO: La configuración contiene credenciales
sip:
  password: "sensitive"
mqtt:
  password: "sensitive"
  
# Requiere:
1. Autenticación obligatoria para /api/config
2. HTTPS en producción
3. Encriptación de passwords en UI
4. Audit log de cambios
5. Rate limiting en endpoints
```

---

## 🎯 Implementación Propuesta

### **Fase 1: API Básica (v0.13.0-alpha)**
- [ ] GET /api/config
- [ ] POST /api/config (sin validación)
- [ ] Backup manual antes de guardar
- [ ] Settings page con forms básicos

**Líneas de código**: ~300  
**Tiempo estimado**: 4-6 horas  
**Riesgo**: Bajo

---

### **Fase 2: Validación (v0.13.0-beta)**
- [ ] POST /api/config/validate
- [ ] Validación de sintaxis YAML
- [ ] Validación semántica (puertos, IPs)
- [ ] Warnings para configs riesgosas

**Líneas de código**: ~200  
**Tiempo estimado**: 3-4 horas  
**Riesgo**: Bajo

---

### **Fase 3: Hot Reload (v0.13.0-rc)**
- [ ] POST /api/config/restart
- [ ] Graceful shutdown
- [ ] Auto-restart después de guardar
- [ ] Health check post-restart

**Líneas de código**: ~150  
**Tiempo estimado**: 4-5 horas  
**Riesgo**: Medio

---

### **Fase 4: Seguridad (v0.13.0)**
- [ ] Autenticación para endpoints config
- [ ] HTTPS opcional
- [ ] Audit log de cambios
- [ ] Rate limiting

**Líneas de código**: ~250  
**Tiempo estimado**: 6-8 horas  
**Riesgo**: Medio

---

### **Fase 5: UX Avanzado (v0.14.0)**
- [ ] Historial de cambios
- [ ] Diff entre versiones
- [ ] Rollback con 1 click
- [ ] Import/export de config
- [ ] Templates de configuración

**Líneas de código**: ~400  
**Tiempo estimado**: 8-10 horas  
**Riesgo**: Bajo

---

## 📊 Comparación con Alternativas

### **Alternativa 1: Mantener edición manual (Status Quo)**

| Pros | Contras |
|------|---------|
| ✅ Sin desarrollo | ❌ Requiere SSH |
| ✅ Sin bugs nuevos | ❌ Error-prone |
| | ❌ No accesible para usuarios |
| | ❌ Sin validación |
| | ❌ Sin backup automático |

**Veredicto**: ❌ No recomendado - limita adopción

---

### **Alternativa 2: Web Dashboard (Propuesta)**

| Pros | Contras |
|------|---------|
| ✅ Accesible desde navegador | ⚠️ Requiere desarrollo (~1000 LOC) |
| ✅ Validación automática | ⚠️ Riesgo de seguridad (mitigable) |
| ✅ Backup automático | ⚠️ Complejidad adicional |
| ✅ Hot reload | |
| ✅ Rollback fácil | |
| ✅ Audit log | |

**Veredicto**: ✅ **RECOMENDADO** - mejora significativa UX

---

### **Alternativa 3: CLI Tool + Web**

| Pros | Contras |
|------|---------|
| ✅ Reutilizable en scripts | ⚠️ Más desarrollo (CLI + Web) |
| ✅ SSH aún funciona | ⚠️ Duplicación de lógica |
| ✅ Power users felices | ⚠️ Mantenimiento adicional |

**Veredicto**: ⚠️ **OPCIONAL** - nice to have, no crítico

---

## 🎯 Recomendación Final

### **¿Es una evolución coherente?**

**✅ SÍ** - Por las siguientes razones:

1. **Alineado con la arquitectura existente**
   - Web dashboard ya existe
   - API REST ya existe
   - Config ya está en memoria
   - Solo falta persistencia + UI

2. **Resuelve problemas reales**
   - SSH no siempre disponible
   - Edición manual es error-prone
   - Usuarios no técnicos necesitan acceso

3. **Bajo riesgo técnico**
   - Infraestructura 90% lista
   - Validación YAML ya existe
   - Hot reload es simple (signal + exec)

4. **Alto valor para usuarios**
   - Reduce tiempo de configuración
   - Reduce errores
   - Mejora experiencia general

5. **Evolución natural del proyecto**
   - v0.10.0: Web dashboard (lectura)
   - v0.11.0: Streaming RTSP
   - **v0.12.0: Web dashboard (escritura)** ← Next logical step
   - v0.13.0: Advanced features (HKSV, etc.)

---

## 📝 Plan de Implementación Sugerido

### **Sprint 1: API Básica (v0.13.0-alpha)**
```bash
# Semana 1
- [ ] Crear pkg/webserver/config_manager.go
- [ ] Implementar GET /api/config
- [ ] Implementar POST /api/config
- [ ] Backup automático antes de guardar
- [ ] Settings page con forms básicos
```

### **Sprint 2: Validación + Hot Reload (v0.13.0-beta)**
```bash
# Semana 2
- [ ] Implementar validación YAML
- [ ] Validación semántica (puertos, IPs)
- [ ] POST /api/config/restart
- [ ] Graceful shutdown
- [ ] Health check post-restart
```

### **Sprint 3: Seguridad (v0.13.0-rc)**
```bash
# Semana 3
- [ ] Autenticación para endpoints
- [ ] HTTPS opcional
- [ ] Audit log
- [ ] Testing exhaustivo
```

### **Release: v0.13.0**
```bash
# Semana 4
- [ ] Documentación
- [ ] Testing en dispositivo real
- [ ] Release candidate
- [ ] Release final
```

---

## 🏆 Conclusión

**La gestión de configuración vía web dashboard es:**

1. ✅ **Técnicamente viable** (90% infraestructura existe)
2. ✅ **Arquitectónicamente coherente** (sigue patrones existentes)
3. ✅ **Comercialmente valiosa** (mejora UX significativamente)
4. ✅ **Estratégicamente correcta** (alineado con roadmap)
5. ✅ **Bajo riesgo** (mitigaciones disponibles)

**Recomendación**: **IMPLEMENTAR en v0.13.0**

---

**Próximos pasos**:
1. Crear issue en GitHub: "Web-based configuration management"
2. Priorizar en roadmap v0.13.0
3. Asignar desarrollador (estimado: 3 sprints)
4. Comenzar Sprint 1 (API Básica)
