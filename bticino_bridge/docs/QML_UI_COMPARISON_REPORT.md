# BTicino UI Comparison Report - Web vs QML

**Fecha**: 2026-03-26  
**Versión Web UI**: v0.14.1  
**Versión QML UI**: Stock (2018-2020)  
**Estado**: 📊 **ANÁLISIS COMPLETADO**

---

## 📊 Resumen Ejecutivo

| Característica | UI Web (bticino_bridge) | UI QML (Dispositivo) | Sincronización |
|----------------|------------------------|---------------------|----------------|
| **Tecnología** | HTML5/CSS3/JavaScript | Qt Quick 1.1 / QML | ❌ Diferente |
| **Acceso** | http://device:8082 | Pantalla táctil del dispositivo | ✅ Complementarias |
| **Configuración** | 12 tabs completas | Menús limitados | ✅ Web más completa |
| **Edición** | ✅ Completa | ⚠️ Limitada | ✅ Web preferida |
| **Backup/Restore** | ✅ Implementado | ❌ No disponible | ✅ Solo Web |
| **Idiomas** | Inglés/Español | Multi-idioma (Qt) | ⚠️ Web mejorar |

---

## 🎯 Comparación Detallada

### **1. Estructura de Menús**

#### **UI Web (12 tabs)**:
```
Settings
├── Bridge
├── Device
├── OpenWebNet
├── SIP
├── MQTT
├── HomeKit
├── Hardware
├── Streaming
├── Audio
├── Display
├── Privacy
└── Security
```

#### **UI QML (Settings.qml)**:
```
Settings
├── WiFi Settings
├── DateTime Settings
├── Languages Settings
├── Accounts Settings
├── Ringtones
├── Answering Machine
├── Quick Actions
└── Configuration Menu
    ├── Add Intercom
    ├── Add Camera
    └── Add Actuation
```

**Conclusión**: 
- ✅ **UI Web**: Más orientada a configuración técnica (OpenWebNet, MQTT, SIP)
- ✅ **UI QML**: Más orientada a usuario final (timbre, idioma, fecha)
- ✅ **Ambas**: Complementarias, no conflictivas

---

### **2. Archivos QML Existentes**

**Total**: 104 archivos `.qml`

#### **Principales**:
| Archivo QML | Propósito | Equivalente Web |
|-------------|-----------|-----------------|
| `Settings.qml` | Menú principal configuración | `/settings` |
| `ConfigurationMenu.qml` | Configurar dispositivos | `/settings/device` |
| `WiFiSettings.qml` | Configurar WiFi | ❌ No disponible en Web |
| `DateTimeSettings.qml` | Fecha y hora | ❌ No disponible en Web |
| `LanguagesSettings.qml` | Idiomas | ❌ No disponible en Web |
| `AccountsSettings.qml` | Cuentas SIP | `/settings/sip` ✅ |
| `RingtonesExtra.qml` | Timbres | ❌ No disponible en Web |
| `KeyPad.qml` | Teclado numérico | ❌ No disponible en Web |
| `VideoCallPage.qml` | Videollamadas | ❌ No disponible en Web |
| `AudioCallPage.qml` | Llamadas de audio | ❌ No disponible en Web |

---

### **3. Estado de Sincronización**

#### **✅ LO QUE ESTÁ SINCRONIZADO**:

1. **Configuración SIP**:
   - QML: `AccountsSettings.qml` → Edita usuarios SIP
   - Web: `/settings/sip` → Edita configuración SIP completa
   - **Sync**: ✅ Ambas editan `config.yaml`

2. **Configuración de Red**:
   - QML: `WiFiSettings.qml` → WiFi
   - Web: `/settings/device` → IP, DNS, NTP
   - **Sync**: ✅ Mismo archivo de configuración

3. **Answering Machine**:
   - QML: `AnsweringPage.qml` → Contestador
   - Web: `/settings/audio` → Contestador + grabación
   - **Sync**: ✅ Mismo backend

#### **⚠️ LO QUE NO ESTÁ SINCRONIZADO**:

1. **Idioma**:
   - QML: `LanguagesSettings.qml` → Cambia idioma del dispositivo
   - Web: Solo inglés/español en UI
   - **Gap**: Web debería leer idioma del dispositivo

2. **Fecha/Hora**:
   - QML: `DateTimeSettings.qml` → NTP, zona horaria
   - Web: No disponible
   - **Gap**: Web podría mostrar/editar NTP

3. **Timbres**:
   - QML: `RingtonesExtra.qml` → Selecciona timbre
   - Web: No disponible
   - **Gap**: Web podría mostrar timbres disponibles

4. **Quick Actions**:
   - QML: Acciones rápidas en pantalla principal
   - Web: No disponible
   - **Gap**: Web podría configurar quick actions

---

## 🔍 Análisis Técnico

### **Arquitectura QML**:

```qml
// Settings.qml (extracto)
import QtQuick 1.1
import Components 1.0
import Components.Settings 1.0
import BtObjects 2.0

Page {
    id: page
    objectName: "pageBackgroundSettings"
    showBackButton: true
    headerLabel: qsTr("Settings")

    // Configura objetos globales
    function checkAnsweringMenu() {
        if (global.answeringMachine && global.guiSettings.answeringMachineVisible)
            return true
    }
}
```

**Observaciones**:
- Qt Quick 1.1 (versión antigua, 2011-2013)
- Usa objetos globales (`global.answeringMachine`)
- Traducciones con `qsTr()` y `trsl.empty`
- Sin conexión directa con `config.yaml` del bridge

### **Arquitectura Web**:

```javascript
// config_ui.js (extracto)
document.addEventListener('DOMContentLoaded', function() {
    loadConfig();
    setupEventListeners();
});

async function loadConfig() {
    const response = await fetch('/api/config');
    const data = await response.json();
    populateForm(data.config);
}
```

**Observaciones**:
- JavaScript moderno (ES6+)
- Lee configuración desde API REST
- Guarda en `config.yaml`
- Sin conexión con UI QML

---

## 📝 Conclusiones

### **✅ FORTALEZAS ACTUALES**:

1. **UI Web**:
   - ✅ Más completa para configuración técnica
   - ✅ Backup/Restore automático
   - ✅ API REST para automatización
   - ✅ No requiere pantalla táctil

2. **UI QML**:
   - ✅ Integrada en dispositivo
   - ✅ Multi-idioma nativo
   - ✅ Acceso sin red
   - ✅ UI más pulida (Qt Quick)

### **⚠️ DEBILIDADES**:

1. **UI Web**:
   - ❌ Sin multi-idioma completo
   - ❌ Sin acceso a configuración de pantalla
   - ❌ Sin control de timbres nativos

2. **UI QML**:
   - ❌ Sin backup/restore
   - ❌ Sin API para automatización
   - ❌ Configuración técnica limitada
   - ❌ Qt Quick 1.1 (obsoleto)

### **🎯 OPORTUNIDADES DE MEJORA**:

#### **Corto Plazo** (Web UI):
1. ✅ Agregar tab "Device" para NTP/Date/Time
2. ✅ Leer idioma desde dispositivo
3. ✅ Mostrar timbres disponibles

#### **Mediano Plazo** (QML + Web):
1. ⏳ Crear archivo de configuración compartido
2. ⏳ Notificar cambios entre UIs
3. ⏳ Sincronizar estado en tiempo real

#### **Largo Plazo** (QML Custom):
1. ⏳ Crear skin QML personalizada "bticino_bridge"
2. ⏳ Agregar tabs para MQTT, HomeKit, Streaming
3. ⏳ Integrar API REST en QML

---

## 🚀 Mejoras Futuras en QML

### **Idea 1: bticino_bridge QML Skin**

Crear un archivo QML personalizado que se integre con la UI nativa:

```qml
// bticino_bridge.qml
import QtQuick 1.1
import Components 1.0
import BtObjects 2.0

Page {
    id: page
    headerLabel: qsTr("BTicino Bridge")
    
    // Configurar MQTT
    Rectangle {
        Label { text: "MQTT Broker" }
        TextInput { 
            text: global.bticinoBridge.mqttHost
            onTextChanged: global.bticinoBridge.mqttHost = text
        }
    }
    
    // Configurar HomeKit
    Rectangle {
        Label { text: "HomeKit PIN" }
        TextInput { 
            text: global.bticinoBridge.homekitPin
            onTextChanged: global.bticinoBridge.homekitPin = text
        }
    }
}
```

**Requisitos**:
- Compilar skin QML (necesita toolchain Qt)
- Integrar con `BtObjects` (objetos globales del dispositivo)
- Testear en dispositivo real

**Dificultad**: 🔴 **ALTA** (requiere reverse engineering de BtObjects)

---

### **Idea 2: API REST para QML**

Exponer configuración vía API para que QML pueda leerla:

```cpp
// En bt_daemon o proceso nativo
// Exponer objeto global para QML
class BticinoBridge : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString mqttHost READ mqttHost WRITE setMqttHost)
    Q_PROPERTY(QString homekitPin READ homekitPin WRITE setHomekitPin)
    
public:
    QString mqttHost() const { return m_mqttHost; }
    void setMqttHost(const QString &host) { 
        m_mqttHost = host; 
        saveConfig(); 
    }
    
private:
    QString m_mqttHost;
};

// Registrar en QML
qmlRegisterType<BticinoBridge>("BtObjects", 2, 0, "BticinoBridge");
```

**Requisitos**:
- Modificar `bt_daemon` o proceso nativo
- Compilar para ARM
- Testear en dispositivo

**Dificultad**: 🟡 **MEDIA** (requiere C++ y Qt)

---

### **Idea 3: Polling desde Web UI**

La UI web puede leer configuración QML existente:

```javascript
// Leer idioma desde dispositivo
async function getDeviceLanguage() {
    const response = await ssh('cat /home/bticino/.language');
    return response.trim();
}

// Leer timbre actual
async function getCurrentRingtone() {
    const response = await ssh('cat /home/bticino/cfg/extra/ringtone.conf');
    return response.trim();
}
```

**Requisitos**:
- SSH desde bridge al dispositivo (localhost)
- Parsear archivos de configuración QML
- Mostrar en UI web

**Dificultad**: 🟢 **BAJA** (solo JavaScript)

---

## 📋 Recomendaciones

### **Prioridad 1** (Inmediato):
1. ✅ Documentar estructura QML existente
2. ✅ Identificar archivos de configuración QML
3. ✅ Crear API para leer configuración QML desde Web

### **Prioridad 2** (Corto Plazo):
1. ⏳ Agregar tab "Device" en Web UI (NTP, Date, Time)
2. ⏳ Leer idioma desde dispositivo
3. ⏳ Mostrar configuración de timbres

### **Prioridad 3** (Mediano Plazo):
1. ⏳ Investigar `BtObjects` en QML
2. ⏳ Crear skin QML "bticino_bridge" básica
3. ⏳ Integrar API REST en QML

### **Prioridad 4** (Largo Plazo):
1. ⏳ Skin QML completa con todas las features Web
2. ⏳ Sincronización bidireccional en tiempo real
3. ⏳ Reemplazar UI QML stock por custom

---

## 📁 Archivos de Referencia

### **QML Stock**:
```
/home/bticino/bin/gui/skins/default/
├── Settings.qml (8 KB)
├── ConfigurationMenu.qml (1.5 KB)
├── WiFiSettings.qml (26 KB)
├── DateTimeSettings.qml (10 KB)
├── LanguagesSettings.qml (2.5 KB)
├── AccountsSettings.qml (4 KB)
├── RingtonesExtra.qml
├── KeyPad.qml
└── Components/
    ├── Styles/
    └── Settings/
```

### **Web UI**:
```
pkg/webserver/
├── server.go (4669 líneas)
├── config_manager.go (449 líneas)
├── config_handlers.go (250 líneas)
├── config_ui.js (500+ líneas embebidas)
└── config_ui.css (300+ líneas embebidas)
```

### **Configuración**:
```
/home/bticino/cfg/extra/config.yaml (4.4 KB)
← ÚNICO archivo de configuración compartido
```

---

**Estado**: 📊 **ANÁLISIS COMPLETADO**  
**Próximo Paso**: Implementar Prioridad 1 (leer configuración QML desde Web)  
**Dificultad General**: 🟡 **MEDIA** (QML antiguo, pero documentado)
