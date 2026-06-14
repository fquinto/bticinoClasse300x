# BTicino Bridge - Versioning & Deploy Guide

**Fecha**: 2026-03-27  
**Versión Actual**: 0.14.2

---

## 📋 **Cómo Funciona la Versión**

### **1. Archivo VERSION**

El archivo `VERSION` en la raíz del proyecto contiene el número de versión:

```
0.14.2
```

### **2. Lectura en Runtime**

El binario lee el archivo `VERSION` desde el directorio donde está ubicado:

```go
// pkg/version/version.go
func GetVersionFromFile() (string, error) {
    execDir := filepath.Dir(os.Executable())
    
    // Busca VERSION en:
    // 1. Mismo directorio que el binario
    // 2. Directorio padre
    // 3. Dos niveles arriba
    // 4. Directorio actual de trabajo
    
    versionPaths := []string{
        filepath.Join(execDir, "VERSION"),
        filepath.Join(execDir, "..", "VERSION"),
        filepath.Join(execDir, "..", "..", "VERSION"),
        "VERSION",
    }
    
    // Retorna el primer VERSION que encuentra
}
```

### **3. Flujo de Versión**

```
1. Developer actualiza VERSION (ej: 0.14.2)
       ↓
2. git commit -m "bump version to 0.14.2"
       ↓
3. make build (compila binario)
       ↓
4. ./scripts/deploy.sh (deploy a dispositivo)
       ↓
   a. Copia binario
   b. Copia archivo VERSION ← ¡IMPORTANTE!
   c. Reinicia servicio
       ↓
5. Binario lee VERSION en startup
       ↓
6. API muestra: {"version": "0.14.2"}
```

---

## 🚀 **Deploy Automático**

### **Comando**:
```bash
./scripts/deploy.sh
```

### **Qué hace**:
1. ✅ Build web frontend (`npm run build`)
2. ✅ Build Go binary (`go build`)
3. ✅ SSH al dispositivo
4. ✅ Backup del binario anterior
5. ✅ Transferencia del nuevo binario
6. ✅ **Transferencia del archivo VERSION** ← ¡CRÍTICO!
7. ✅ Activación del nuevo binario
8. ✅ Restart del servicio
9. ✅ Verificación

---

## 🐛 **Problemas Comunes**

### **1. Versión incorrecta en dispositivo**

**Síntoma**: La API muestra versión anterior

**Causa**: El archivo `VERSION` no se copió al dispositivo

**Solución**:
```bash
# Copiar manualmente
cat VERSION | ssh bticino "cat > /home/bticino/cfg/extra/VERSION"

# Reiniciar servicio
ssh bticino "pkill -9 bticino_bridge && cd /home/bticino/cfg/extra && ./bticino_bridge -config config.yaml &"

# Verificar
curl http://192.168.1.38:8082/api/status | python3 -c "import sys,json; print(json.load(sys.stdin)['version'])"
```

### **2. Versión 0.0.0**

**Síntoma**: API muestra `{"version": "0.0.0"}`

**Causa**: No existe archivo `VERSION` en el dispositivo

**Solución**:
```bash
# Crear archivo VERSION
echo "0.14.2" | ssh bticino "cat > /home/bticino/cfg/extra/VERSION"

# Reiniciar
ssh bticino "pkill -9 bticino_bridge && cd /home/bticino/cfg/extra && ./bticino_bridge -config config.yaml &"
```

---

## 📝 **Proceso de Release**

### **1. Actualizar Versión**:
```bash
echo "0.14.3" > VERSION
git add VERSION
git commit -m "bump version to 0.14.3"
```

### **2. Actualizar CHANGELOG**:
```bash
# Editar CHANGELOG.md
git add CHANGELOG.md
git commit -m "docs: Update CHANGELOG for 0.14.3"
```

### **3. Build y Test**:
```bash
make build
./bticino_bridge -version
```

### **4. Deploy**:
```bash
./scripts/deploy.sh
```

### **5. Verificar**:
```bash
# API
curl http://192.168.1.38:8082/api/status

# Web UI
curl http://192.168.1.38:8082/ | grep 'BTicino Bridge'

# Logs
ssh bticino "grep 'Version:' /var/log/bticino_bridge.log | tail -1"
```

### **6. Git Tag**:
```bash
git tag v0.14.3
git push origin v0.14.3
git push origin main
```

---

## 🔍 **Archivos Involucrados**

| Archivo | Propósito | Ubicación |
|---------|-----------|-----------|
| `VERSION` | Número de versión | Raíz del proyecto |
| `CHANGELOG.md` | Historial de cambios | Raíz del proyecto |
| `pkg/version/version.go` | Lógica de lectura | Código Go |
| `scripts/deploy.sh` | Deploy automático | Scripts |
| `/home/bticino/cfg/extra/VERSION` | Versión en dispositivo | Dispositivo |

---

## ✅ **Checklist de Deploy**

- [ ] 1. Actualizar archivo `VERSION`
- [ ] 2. Actualizar `CHANGELOG.md`
- [ ] 3. Commit de cambios
- [ ] 4. `make build`
- [ ] 5. Verificar versión local
- [ ] 6. `./scripts/deploy.sh`
- [ ] 7. Verificar versión en dispositivo
- [ ] 8. Verificar Web UI
- [ ] 9. Git tag y push

---

**Estado**: ✅ **DOCUMENTADO**  
**Versión Actual**: 0.14.2
