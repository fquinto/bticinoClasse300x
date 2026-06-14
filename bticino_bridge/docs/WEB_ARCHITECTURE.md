# BTicino Bridge - Web Architecture Documentation

**Fecha**: 2026-03-29  
**Versión**: 0.14.3  
**Estado**: ⚠️ **TRANSICIÓN** (HTML embebido → Svelte)

---

## 📊 **Arquitectura Actual (v0.14.3)**

### **Estado del Frontend**

| Componente | Estado | Ubicación |
|------------|--------|-----------|
| **Dashboard** | ⚠️ HTML embebido en Go | `pkg/webserver/server.go` |
| **Settings** | ⚠️ HTML embebido en Go | `pkg/webserver/server.go` |
| **Controls** | ⚠️ HTML embebido en Go | `pkg/webserver/server.go` |
| **Logs** | ⚠️ HTML embebido en Go | `pkg/webserver/server.go` |
| **CSS** | ⚠️ Embebido en Go | `pkg/webserver/server.go` |
| **JavaScript** | ⚠️ Embebido en Go | `pkg/webserver/server.go` |
| **Svelte Build** | ✅ Compilado | `web/dist/` |
| **Svelte Source** | ✅ Listo | `web/src/` |

---

## 🏗️ **Arquitectura Actual**

```
┌─────────────────────────────────────────────────────────┐
│                   BTicino Bridge                        │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Go Web Server (pkg/webserver/server.go)          │ │
│  │                                                   │ │
│  │  HTML Templates (embebidos en el código):        │ │
│  │  ├── getDashboardHTML()                          │ │
│  │  ├── getSettingsHTML()                           │ │
│  │  ├── getControlsHTML()                           │ │
│  │  └── getLogsHTML()                               │ │
│  │                                                   │ │
│  │  CSS (embebido):                                 │ │
│  │  └── getCSS()                                    │ │
│  │                                                   │ │
│  │  JavaScript (embebido):                          │ │
│  │  └── getJS()                                     │ │
│  │                                                   │ │
│  │  API Routes:                                     │ │
│  │  ├── /api/status                                 │ │
│  │  ├── /api/config                                 │ │
│  │  ├── /api/controls/*                             │ │
│  │  └── /api/logs                                   │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Svelte Build (web/dist/) - ⚠️ NO USADO          │ │
│  │  ├── index.html                                   │ │
│  │  └── assets/                                      │ │
│  │      ├── index-*.js                               │ │
│  │      ├── vendor-*.js                              │ │
│  │      └── index-*.css                              │ │
│  └───────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

---

## 🔍 **Cómo Funciona Actualmente**

### **1. Server Startup**

```go
// pkg/webserver/server.go
func NewWebServer(cfg *config.Config, bridge BTicinoBridge, logger *logrus.Logger) *WebServer {
    return &WebServer{
        config:        cfg,
        bridge:        bridge,
        logger:        logger,
        staticDir:     cfg.Web.StaticDir,  // ⚠️ No se usa para HTML
        messageParser: messageparser.NewMessageParser(),
        // ...
    }
}
```

### **2. Route Registration**

```go
// Routes para HTML embebido
mux.HandleFunc("/", ws.handleDashboard)
mux.HandleFunc("/dashboard", ws.handleDashboard)
mux.HandleFunc("/messages", ws.handleMessagesPage)
mux.HandleFunc("/controls", ws.handleControlsPage)
mux.HandleFunc("/settings", ws.handleSettingsPage)
mux.HandleFunc("/logs", ws.handleLogsPage)

// Routes para API
mux.HandleFunc("/api/status", ws.handleAPIStatus)
mux.HandleFunc("/api/config", ws.handleAPIConfig)
// ...
```

### **3. HTML Rendering**

```go
// El HTML se genera desde strings embebidos en Go
func (ws *WebServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    w.Write([]byte(ws.injectVersion(ws.getDashboardHTML())))
}

func (ws *WebServer) getDashboardHTML() string {
    return `<!DOCTYPE html>
<html lang="en">
<head>
    <title>BTicino Bridge {{VERSION}}</title>
    ...
</head>
<body>
    <!-- HTML embebido en el código Go -->
    ...
</body>
</html>`
}
```

### **4. CSS y JavaScript**

```go
// CSS embebido
func (ws *WebServer) getCSS() string {
    return `/* Dashboard Styles */
:root {
    --primary-color: #2196F3;
    ...
}
...`
}

// JavaScript embebido
func (ws *WebServer) getJS() string {
    return `// Dashboard JavaScript
async function refreshStatus() {
    const response = await fetch('/api/status');
    ...
}
...`
}
```

---

## 📁 **Estructura de Archivos Actual**

```
bticino_bridge/
├── pkg/
│   └── webserver/
│       ├── server.go              # ⚠️ TODO el HTML/CSS/JS embebido (~4800 líneas)
│       ├── config_manager.go      # ✅ Gestión de configuración
│       ├── config_handlers.go     # ✅ API handlers
│       └── device_handlers.go     # ✅ Device API handlers
├── web/
│   ├── src/
│   │   ├── routes/
│   │   │   ├── +page.svelte       # ✅ Dashboard Svelte
│   │   │   ├── settings/
│   │   │   │   └── +page.svelte   # ✅ Settings Svelte
│   │   │   ├── controls/
│   │   │   │   └── +page.svelte   # ✅ Controls Svelte
│   │   │   └── logs/
│   │   │       └── +page.svelte   # ✅ Logs Svelte
│   │   └── App.svelte             # ✅ App shell
│   ├── dist/                      # ⚠️ Compilado pero NO USADO
│   │   ├── index.html
│   │   └── assets/
│   ├── package.json
│   └── vite.config.js
└── configs/
    └── config.yaml
```

---

## ⚠️ **Problemas de la Arquitectura Actual**

### **1. Código Monolítico**

**server.go**: ~4800 líneas
- HTML embebido: ~3000 líneas
- CSS embebido: ~1000 líneas
- JavaScript embebido: ~500 líneas
- Lógica Go: ~300 líneas

**Problema**: Difícil de mantener, sin syntax highlighting para HTML/CSS/JS.

### **2. Sin Hot Reload**

- Cada cambio requiere: `go build` → `./scripts/deploy.sh`
- No hay desarrollo ágil
- Tiempo de feedback: ~30 segundos

### **3. Sin Separation of Concerns**

- HTML, CSS, JS y Go mezclados en un solo archivo
- No hay componentización
- No hay reutilización de código

### **4. Build Size**

- Binario Go: 14MB (con HTML embebido)
- No hay code splitting
- No hay lazy loading

### **5. Svelte Compilado No Usado**

```
web/dist/ existe pero el servidor no lo usa
```

---

## 🎯 **Arquitectura Objetivo (Svelte)**

### **Estado Deseado**

| Componente | Actual | Objetivo |
|------------|--------|----------|
| **Dashboard** | HTML embebido | `web/src/routes/+page.svelte` |
| **Settings** | HTML embebido | `web/src/routes/settings/+page.svelte` |
| **Controls** | HTML embebido | `web/src/routes/controls/+page.svelte` |
| **Logs** | HTML embebido | `web/src/routes/logs/+page.svelte` |
| **CSS** | Embebido | Svelte `<style>` + componentes |
| **JavaScript** | Embebido | Svelte `<script>` + módulos |
| **Server** | HTML embebido | File server estático |

---

## 🏗️ **Arquitectura Objetivo**

```
┌─────────────────────────────────────────────────────────┐
│                   BTicino Bridge                        │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Go Web Server                                    │ │
│  │                                                   │ │
│  │  Static File Server:                              │ │
│  │  └── http.FileServer(http.Dir("web/dist"))       │ │
│  │                                                   │ │
│  │  API Routes:                                      │ │
│  │  ├── /api/status                                  │ │
│  │  ├── /api/config                                  │ │
│  │  ├── /api/controls/*                              │ │
│  │  └── /api/logs                                    │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │  Svelte App (web/dist/)                           │ │
│  │  ├── index.html                                   │ │
│  │  └── assets/                                      │ │
│  │      ├── index-*.js    (Svelte components)        │ │
│  │      ├── vendor-*.js   (dependencies)             │ │
│  │      └── index-*.css   (styles)                   │ │
│  └───────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

---

## 🔄 **Migración Pendiente**

### **Paso 1: Agregar File Server** ⏳ PENDIENTE

```go
// pkg/webserver/server.go
func (ws *WebServer) Start(ctx context.Context) error {
    mux := http.NewServeMux()
    
    // ✅ AGREGAR: File server para Svelte
    fs := http.FileServer(http.Dir("web/dist"))
    mux.Handle("/", fs)
    
    // API routes (mantener)
    mux.HandleFunc("/api/status", ws.handleAPIStatus)
    mux.HandleFunc("/api/config", ws.handleAPIConfig)
    // ...
    
    // ❌ ELIMINAR: Routes para HTML embebido
    // mux.HandleFunc("/", ws.handleDashboard)
    // mux.HandleFunc("/dashboard", ws.handleDashboard)
    // ...
}
```

### **Paso 2: Eliminar HTML Embebido** ⏳ PENDIENTE

```go
// pkg/webserver/server.go

// ❌ ELIMINAR: ~3000 líneas de HTML
func (ws *WebServer) getDashboardHTML() string { ... }
func (ws *WebServer) getSettingsHTML() string { ... }
func (ws *WebServer) getControlsHTML() string { ... }
func (ws *WebServer) getLogsHTML() string { ... }

// ❌ ELIMINAR: ~1000 líneas de CSS
func (ws *WebServer) getCSS() string { ... }

// ❌ ELIMINAR: ~500 líneas de JS
func (ws *WebServer) getJS() string { ... }

// ❌ ELIMINAR: Handlers que usan HTML embebido
func (ws *WebServer) handleDashboard(w http.ResponseWriter, r *http.Request) { ... }
func (ws *WebServer) handleMessagesPage(w http.ResponseWriter, r *http.Request) { ... }
// ...
```

### **Paso 3: Actualizar Svelte Components** ⏳ EN PROGRESO

**Archivos existentes**:
- ✅ `web/src/routes/+page.svelte` (Dashboard)
- ✅ `web/src/routes/settings/+page.svelte` (Settings)
- ✅ `web/src/routes/controls/+page.svelte` (Controls)
- ✅ `web/src/routes/logs/+page.svelte` (Logs)
- ✅ `web/src/App.svelte` (App shell)

**Falta**:
- ⏳ Messages page
- ⏳ Integration con API endpoints

### **Paso 4: Configurar Build** ⏳ PENDIENTE

```javascript
// web/vite.config.js
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
    sourcemap: true,  // Para debugging
    minify: 'terser',
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8082'  // Para desarrollo
    }
  }
})
```

### **Paso 5: Actualizar Deploy Script** ⏳ PENDIENTE

```bash
# scripts/deploy.sh

# 1. Build Svelte
cd web && npm install && npm run build

# 2. Build Go
cd .. && GOOS=linux GOARCH=arm GOARM=7 go build

# 3. Deploy binario + web/dist
./deploy_to_device.sh
```

---

## 📊 **Comparación: Actual vs Objetivo**

| Métrica | Actual | Objetivo | Mejora |
|---------|--------|----------|--------|
| **Líneas server.go** | 4800 | ~500 | -90% |
| **Separación concerns** | ❌ No | ✅ Sí | ✅ |
| **Hot Reload** | ❌ No | ✅ Sí (Vite) | ✅ |
| **Componentización** | ❌ No | ✅ Sí (Svelte) | ✅ |
| **Code Splitting** | ❌ No | ✅ Sí | ✅ |
| **Bundle Size** | 14MB (todo junto) | 14MB + 56KB | ✅ Separado |
| **Desarrollo** | Lento (30s) | Rápido (<1s) | ✅ 30x más rápido |
| **Mantenimiento** | Difícil | Fácil | ✅ |

---

## 🎯 **Roadmap de Migración**

### **Fase 1: Setup** ✅ COMPLETADA
- [x] Crear proyecto Svelte
- [x] Configurar Vite
- [x] Crear componentes básicos
- [x] Build funcional

### **Fase 2: Componentes** ✅ COMPLETADA
- [x] Dashboard component
- [x] Settings component
- [x] Controls component
- [x] Logs component

### **Fase 3: Integración** ⏳ PENDIENTE
- [ ] Agregar file server en Go
- [ ] Eliminar HTML embebido
- [ ] Eliminar CSS embebido
- [ ] Eliminar JS embebido
- [ ] Actualizar deploy.sh

### **Fase 4: Testing** ⏳ PENDIENTE
- [ ] Testear todas las rutas
- [ ] Testear API integration
- [ ] Testear en dispositivo real
- [ ] Performance testing

### **Fase 5: Producción** ⏳ PENDIENTE
- [ ] Deploy a dispositivo
- [ ] User acceptance testing
- [ ] Documentación final

---

## 📝 **Estado Actual del Proyecto**

### **Lo que SÍ funciona**:
- ✅ API REST completa (`/api/*`)
- ✅ Svelte components compilados
- ✅ Build de Go funcional
- ✅ Deploy automático
- ✅ Versión 0.14.3 estable

### **Lo que NO funciona**:
- ❌ Svelte no se usa en producción
- ❌ HTML sigue embebido en Go
- ❌ No hay hot reload
- ❌ server.go tiene ~4800 líneas

### **Lo que está EN PROGRESO**:
- ⏳ Migración a Svelte
- ⏳ Eliminación de HTML embebido
- ⏳ Reducción de server.go

---

## 🔧 **Comandos de Desarrollo**

### **Actual (HTML embebido)**:
```bash
# Build completo
make build

# Deploy
make deploy

# Desarrollo: Editar server.go → go build → deploy
# Tiempo: ~30 segundos
```

### **Objetivo (Svelte)**:
```bash
# Desarrollo con hot reload
make dev

# Build producción
make build

# Deploy
make deploy

# Desarrollo: Editar .svelte → auto reload
# Tiempo: <1 segundo
```

---

## 📚 **Recursos**

### **Documentación**:
- [Svelte Docs](https://svelte.dev/docs)
- [Vite Docs](https://vitejs.dev/guide/)
- [Svelte Tutorial](https://svelte.dev/tutorial)

### **Archivos Clave**:
- `web/src/routes/+page.svelte` - Dashboard
- `web/src/App.svelte` - App shell
- `web/vite.config.js` - Vite config
- `pkg/webserver/server.go` - Server Go (actual)

---

**Estado**: ⚠️ **EN TRANSICIÓN**  
**Próximo Paso**: Agregar file server y eliminar HTML embebido  
**ETA**: 1-2 días de desarrollo
