# BTicino Bridge - Svelte UI Setup

**Fecha**: 2026-03-27  
**Versión**: v0.15.0 (Svelte)  
**Estado**: ✅ **SETUP COMPLETADO**

---

## 📁 Estructura del Proyecto

```
bticino_bridge/
├── web/                    # 🆕 Svelte UI
│   ├── src/
│   │   ├── main.js         # Entry point
│   │   └── App.svelte      # Dashboard component
│   ├── index.html
│   ├── package.json
│   ├── vite.config.js
│   └── svelte.config.js
├── scripts/
│   └── deploy.sh           # 🆡 Deploy único (build + deploy)
├── Makefile                # 🆡 Con build Svelte
└── docs/
    └── SVELTE_SETUP.md     # Este archivo
```

---

## 🚀 Quick Start

### **1. Instalar dependencias**:
```bash
cd bticino_bridge
make install
```

### **2. Development mode** (hot reload):
```bash
make dev
```

Acceder a:
- Web dev: http://localhost:5173 (hot reload)
- Go backend: http://localhost:8082 (API)

### **3. Build completo**:
```bash
make build
```

### **4. Deploy**:
```bash
make deploy
```

---

## 📦 Comandos Make

| Comando | Descripción |
|---------|-------------|
| `make build` | Build web + go |
| `make build-web` | Solo build web |
| `make build-go` | Solo build go |
| `make deploy` | Build + deploy |
| `make dev` | Development mode |
| `make install` | Instalar dependencias |
| `make clean` | Limpiar todo |
| `make test` | Run tests |
| `make help` | Mostrar ayuda |

---

## 🎯 Componentes Svelte

### **Actuales**:
- ✅ `App.svelte` - Dashboard principal

### **Próximos**:
- ⏳ `Settings.svelte` - Configuración
- ⏳ `DeviceTab.svelte` - Device (NTP, Language)
- ⏳ `ConfigTab.svelte` - Config tabs
- ⏳ `Messages.svelte` - Mensajes
- ⏳ `Controls.svelte` - Controles
- ⏳ `Logs.svelte` - Logs viewer

---

## 🔧 Configuración

### **vite.config.js**:
```javascript
{
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    minify: 'terser'
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8082'
    }
  }
}
```

### **proxy**:
- `/api/*` → http://localhost:8082/api/*
- Permite desarrollar con backend real

---

## 📊 Bundle Size

| Build | Tamaño |
|-------|--------|
| **Svelte** | ~10-15KB (gzipped) |
| **Vendor** | ~50KB (Svelte runtime) |
| **Total** | ~65KB |

**Comparación**:
- HTML embebido: 14MB (todo en binario Go)
- Svelte: 13MB + 65KB (separado)

---

## 🎨 Estilos

**Enfoque**: CSS scoped por componente

```svelte
<style>
  .navbar {
    background: #2196F3;
    color: white;
  }
</style>
```

**Ventajas**:
- ✅ Sin conflictos de nombres
- ✅ CSS crítico por componente
- ✅ Fácil mantenimiento

---

## 🔌 API Integration

### **Ejemplo**:
```svelte
<script>
  import { onMount } from 'svelte'
  
  let status = null
  
  onMount(async () => {
    const response = await fetch('/api/status')
    status = await response.json()
  })
</script>

{#if status}
  <p>Version: {status.version}</p>
{/if}
```

### **Endpoints disponibles**:
- `GET /api/status` - Estado del sistema
- `GET /api/config` - Configuración
- `GET /api/device/ntp` - NTP config
- `GET /api/device/language` - Idioma
- `POST /api/config/save` - Guardar config

---

## 🚀 Deploy

### **Automático**:
```bash
make deploy
```

**Qué hace**:
1. ✅ Build Svelte (`web/dist/`)
2. ✅ Build Go (`bticino_bridge`)
3. ✅ SSH al dispositivo
4. ✅ Backup
5. ✅ Transferencia
6. ✅ Restart servicio
7. ✅ Verificación

### **Manual**:
```bash
# Build
cd web && npm run build
cd .. && GOOS=linux GOARCH=arm go build

# Deploy
./scripts/deploy.sh
```

---

## 🐛 Troubleshooting

### **Error: "npm not found"**:
```bash
# Instalar Node.js
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs
```

### **Error: "web/dist/ not found"**:
```bash
# Build web manualmente
cd web && npm install && npm run build
```

### **Error: "SSH connection failed"**:
```bash
# Verificar SSH keys
ssh-copy-id bticino
ssh bticino "echo OK"
```

---

## 📝 Próximos Pasos

### **Inmediatos**:
1. ✅ Setup Svelte (COMPLETADO)
2. ✅ Dashboard component (COMPLETADO)
3. ⏳ Settings page
4. ⏳ Device tab
5. ⏳ Config tabs

### **Corto plazo**:
- Messages page
- Controls page
- Logs viewer
- Dark mode

### **Largo plazo**:
- TypeScript migration
- Unit tests (Vitest)
- E2E tests (Playwright)
- PWA support

---

## 📚 Recursos

### **Documentación**:
- [Svelte Docs](https://svelte.dev/docs)
- [Svelte Tutorial](https://svelte.dev/tutorial)
- [Vite Docs](https://vitejs.dev/guide/)

### **Componentes**:
- [Svelte Material UI](https://sveltematerialui.com/)
- [Svelte Kit](https://kit.svelte.dev/)

---

**Estado**: ✅ **SETUP COMPLETADO**  
**Próximo**: Crear Settings page  
**Dificultad**: 🟢 Baja (Svelte es simple)

