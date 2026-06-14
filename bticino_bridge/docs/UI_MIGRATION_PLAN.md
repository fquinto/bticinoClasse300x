# BTicino Bridge - UI Modernization Plan

**Fecha**: 2026-03-27  
**Versión Actual**: v0.14.2 (HTML embebido en Go)  
**Estado**: 📋 **PLANIFICANDO MIGRACIÓN**

---

## 📊 Estado Actual de la UI

| Aspecto | Estado Actual | Problemas |
|---------|---------------|-----------|
| **Tecnología** | HTML + CSS + JS embebido en Go | ❌ Difícil de mantener |
| **Líneas de código** | 4802 líneas en server.go | ❌ Todo en un archivo |
| **Templates** | 5 templates HTML embebidos | ❌ Sin reutilización |
| **Build** | Go build simple | ✅ Rápido pero limitado |
| **Hot Reload** | ❌ No disponible | ❌ Requiere recompilar |
| **Componentes** | ❌ No hay | ❌ Todo copiado/pegado |

---

## 🎯 Objetivos de la Migración

1. ✅ **Framework moderno** (Vue+Quasar o Svelte)
2. ✅ **Separación de concerns** (backend Go, frontend JS)
3. ✅ **Componentes reutilizables**
4. ✅ **Hot reload** en desarrollo
5. ✅ **Build automático** en deploy
6. ✅ **Un único script de deploy**

---

## 🔍 Comparativa: Vue+Quasar vs Svelte

### **Vue 3 + Quasar Framework**

#### ✅ **Ventajas**:
- 🟢 **Maduro** - Vue 3 estable desde 2020
- 🟢 **Quasar** - Componentes Material Design listos
- 🟢 **Ecosistema grande** - Muchos plugins, librerías
- 🟢 **TypeScript** - Soporte nativo excelente
- 🟢 **DevTools** - Vue DevTools muy completas
- 🟢 **Comunidad** - Muy grande en España/Latam
- 🟢 **Trabajo** - Más demanda laboral

#### ❌ **Desventajas**:
- 🔴 **Peso** - ~100KB (Vue) + ~200KB (Quasar)
- 🔴 **Complejidad** - Curva de aprendizaje media
- 🔴 **Build** - Requiere Node.js + Vite/Webpack
- 🔴 **Runtime** - Virtual DOM (más lento que Svelte)

#### 📦 **Tamaño Bundle**:
```
Production build: ~350-400KB (gzipped: ~120KB)
```

---

### **Svelte + SvelteKit**

#### ✅ **Ventajas**:
- 🟢 **Ligero** - ~2KB runtime (vs 300KB Vue)
- 🟢 **Rápido** - Sin Virtual DOM, compile-time
- 🟢 **Simple** - Menos boilerplate, más intuitivo
- 🟢 **SvelteKit** - Framework completo (como Next.js)
- 🟢 **Build** - Más simple, menos configuración
- 🟢 **Reactivity** - Más natural (sin reactive(), ref())

#### ❌ **Desventajas**:
- 🔴 **Joven** - Svelte 4 estable desde 2023
- 🔴 **Ecosistema** - Menos librerías que Vue
- 🔴 **TypeScript** - Soporte bueno pero no excelente
- 🔴 **Comunidad** - Más pequeña (aunque creciendo)
- 🔴 **Trabajo** - Menos demanda (por ahora)

#### 📦 **Tamaño Bundle**:
```
Production build: ~10-15KB (gzipped: ~5KB)
```

---

## 📊 Comparativa Directa

| Característica | Vue+Quasar | Svelte | Ganador |
|----------------|------------|--------|---------|
| **Tamaño** | ~350KB | ~10KB | 🏆 **Svelte** |
| **Rendimiento** | Bueno | Excelente | 🏆 **Svelte** |
| **Curva aprendizaje** | Media | Baja | 🏆 **Svelte** |
| **Componentes UI** | Quasar (100+) | Svelte Material (30) | 🏆 **Vue** |
| **Ecosistema** | Grande | Mediano | 🏆 **Vue** |
| **TypeScript** | Excelente | Bueno | 🏆 **Vue** |
| **DevTools** | Excelentes | Buenas | 🏆 **Vue** |
| **Build time** | ~5-10s | ~2-5s | 🏆 **Svelte** |
| **Comunidad ES** | Muy grande | Pequeña | 🏆 **Vue** |
| **Demanda laboral** | Alta | Media | 🏆 **Vue** |

---

## 🎯 **Recomendación para BTicino Bridge**

### **Para este proyecto específico**: 🏆 **SVELTE**

**Razones**:
1. ✅ **Tamaño crítico** - 10KB vs 350KB (dispositivo embebido)
2. ✅ **Simplicidad** - Menos complejidad = menos bugs
3. ✅ **Rendimiento** - Sin Virtual DOM, más rápido en hardware limitado
4. ✅ **Build simple** - Fácil de integrar con Go
5. ✅ **Curva baja** - Más rápido de implementar

**Vue+Quasar sería mejor si**:
- Necesitás muchos componentes UI complejos
- El equipo ya conoce Vue
- Priorizás ecosistema sobre tamaño

---

## 🚀 **Plan de Migración a Svelte**

### **Fase 1: Setup (2-3 horas)**

```bash
# 1. Crear proyecto Svelte
cd bticino_bridge
npm create svelte@latest web
cd web
npm install

# 2. Instalar dependencias
npm install -D @sveltejs/vite-plugin-svelte
npm install svelte-material-ui

# 3. Estructura
web/
├── src/
│   ├── lib/
│   │   ├── components/
│   │   │   ├── ConfigTab.svelte
│   │   │   ├── DeviceTab.svelte
│   │   │   └── ...
│   │   └── api.js
│   ├── routes/
│   │   ├── +page.svelte (Dashboard)
│   │   ├── settings/
│   │   │   └── +page.svelte
│   │   └── ...
│   └── app.html
├── package.json
└── vite.config.js
```

---

### **Fase 2: Componentes (4-6 horas)**

**Componentes a crear**:
1. ✅ `Dashboard.svelte` - Estado del sistema
2. ✅ `Settings.svelte` - Configuración principal
3. ✅ `ConfigTab.svelte` - Tabs de configuración
4. ✅ `DeviceTab.svelte` - Device (NTP, Language, etc.)
5. ✅ `Messages.svelte` - Mensajes
6. ✅ `Controls.svelte` - Controles
7. ✅ `Logs.svelte` - Logs viewer

---

### **Fase 3: Integración con Go (2-3 horas)**

**Modificar `pkg/webserver/server.go`**:
```go
// Servir archivos estáticos desde web/dist/
func (ws *WebServer) handleStatic(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "web/dist"+r.URL.Path)
}

// API endpoints (ya existen)
mux.HandleFunc("/api/config", ws.handleAPIConfig)
mux.HandleFunc("/api/device/ntp", ws.handleAPIDeviceNTP)
// ...
```

---

### **Fase 4: Build Automático (1-2 horas)**

**Modificar `Makefile`**:
```makefile
build: build-web build-go

build-web:
	cd web && npm install && npm run build

build-go:
	GOOS=linux GOARCH=arm GOARM=7 go build -o bticino_bridge ./cmd/main.go

deploy: build
	./scripts/deploy_auto.sh
```

---

### **Fase 5: Unificar Deploy (1 hora)**

**Modificar `scripts/deploy_auto.sh`**:
```bash
#!/bin/bash
# Deploy automático UNIFICADO

echo "[1/3] Building web frontend..."
cd web && npm install && npm run build
cd ..

echo "[2/3] Building Go binary..."
GOOS=linux GOARCH=arm GOARM=7 go build -o bticino_bridge ./cmd/main.go

echo "[3/3] Deploying to device..."
./scripts/deploy_to_bticino.sh
```

---

## 📁 **Estructura Final del Proyecto**

```
bticino_bridge/
├── cmd/
│   └── main.go
├── pkg/
│   └── webserver/
│       ├── server.go          # Go backend + API
│       ├── config_manager.go
│       └── device_handlers.go
├── web/                        # 🆕 Svelte frontend
│   ├── src/
│   │   ├── lib/
│   │   │   ├── components/
│   │   │   │   ├── Dashboard.svelte
│   │   │   │   ├── Settings.svelte
│   │   │   │   └── ...
│   │   │   └── api.js
│   │   └── routes/
│   │       ├── +page.svelte
│   │       ├── settings/
│   │       │   └── +page.svelte
│   │       └── ...
│   ├── static/
│   ├── package.json
│   └── vite.config.js
├── scripts/
│   ├── deploy_auto.sh         # 🆡 Unificado (build + deploy)
│   ├── deploy_to_bticino.sh
│   └── ...
├── configs/
│   └── config.yaml
├── Makefile                    # 🆡 Con build web
├── go.mod
└── README.md
```

---

## 🎯 **Makefile Actualizado**

```makefile
.PHONY: build build-web build-go deploy clean test

# Build completo (web + go)
build: build-web build-go
	@echo "✅ Build completo exitoso"

# Build frontend Svelte
build-web:
	@echo "🔨 Building web frontend..."
	cd web && npm install
	cd web && npm run build
	@echo "✅ Web frontend built (web/dist/)"

# Build backend Go
build-go:
	@echo "🔨 Building Go binary..."
	GOOS=linux GOARCH=arm GOARM=7 go build -o bticino_bridge ./cmd/main.go
	@echo "✅ Go binary built (bticino_bridge)"

# Deploy a dispositivo
deploy: build
	@echo "🚀 Deploying to device..."
	./scripts/deploy_auto.sh
	@echo "✅ Deploy completed"

# Development mode (hot reload)
dev:
	@echo "🔧 Starting development mode..."
	cd web && npm run dev &
	go run ./cmd/main.go -config configs/config.yaml

# Tests
test:
	@echo "🧪 Running tests..."
	./scripts/run_all_tests.sh --all

# Clean
clean:
	@echo "🧹 Cleaning..."
	rm -rf web/dist
	rm -f bticino_bridge
	rm -rf web/node_modules
	@echo "✅ Cleaned"
```

---

## 📊 **Comparativa: Antes vs Después**

| Aspecto | Antes (HTML embebido) | Después (Svelte) | Mejora |
|---------|----------------------|------------------|--------|
| **Tamaño UI** | Embebido en Go (14MB total) | Separado (14MB + 10KB) | ✅ Más limpio |
| **Build time** | 10s (solo Go) | 15s (Go + Svelte) | ⚠️ +5s |
| **Hot reload** | ❌ No | ✅ Sí (SvelteKit) | ✅ Excelente |
| **Componentes** | ❌ No | ✅ Reutilizables | ✅ Excelente |
| **Mantenimiento** | ❌ Difícil (todo en server.go) | ✅ Fácil (separado) | ✅ Excelente |
| **TypeScript** | ❌ No | ✅ Opcional | ✅ Bueno |
| **DevTools** | ❌ Browser DevTools | ✅ Svelte DevTools | ✅ Excelente |

---

## 🚀 **Próximos Pasos Inmediatos**

### **1. Decidir framework** (HOY):
- [ ] ¿Svelte o Vue+Quasar?
- [ ] **Recomendación**: Svelte (por tamaño y simplicidad)

### **2. Setup inicial** (MAÑANA):
- [ ] Crear proyecto Svelte
- [ ] Instalar dependencias
- [ ] Configurar Vite

### **3. Primer componente** (2 DÍAS):
- [ ] Dashboard.svelte
- [ ] Conectar con API `/api/status`
- [ ] Testear hot reload

### **4. Migrar tabs** (3-5 DÍAS):
- [ ] Settings.svelte
- [ ] DeviceTab.svelte
- [ ] ConfigTab.svelte
- [ ] Messages.svelte
- [ ] Controls.svelte
- [ ] Logs.svelte

### **5. Integrar con Go** (6-7 DÍAS):
- [ ] Modificar server.go para servir web/dist/
- [ ] Actualizar Makefile
- [ ] Unificar deploy script

### **6. Testing** (8-9 DÍAS):
- [ ] Test en dispositivo real
- [ ] Performance testing
- [ ] Documentar

---

## 💾 **Bundle Size Comparison**

```
Actual (HTML embebido):
├── server.go: 4802 líneas
├── Binario: 14MB (con HTML/CSS/JS embebido)
└── Total: 14MB

Svelte (propuesto):
├── Go backend: ~3000 líneas (sin HTML)
├── Svelte frontend: ~10KB (gzipped)
├── Binario: 13MB (sin HTML embebido)
└── Total: 13MB + 10KB
```

---

## ✅ **Conclusión**

**Recomendación**: **Svelte** para este proyecto específico.

**Razones principales**:
1. ✅ **Tamaño** - 10KB vs 350KB (crítico para dispositivo embebido)
2. ✅ **Simplicidad** - Menos complejidad = menos bugs
3. ✅ **Rendimiento** - Sin Virtual DOM
4. ✅ **Build simple** - Fácil integración con Go

**¿Comenzamos con el setup de Svelte?**
