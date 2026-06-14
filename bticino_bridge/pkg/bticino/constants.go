// Package bticino contiene constantes, rutas y comandos especificos de BTicino
// derivados del analisis real del dispositivo Classe 300X (firmware ARM7).
//
// Toda la informacion de este fichero fue verificada mediante pruebas fisicas
// en el dispositivo (192.168.1.38) durante marzo 2026. Los patrones OWN,
// funciones GPIO, mapeos de botones y significados de LEDs fueron confirmados
// observando eventos reales en el bus OpenWebNet (puerto 20000) y en
// /dev/input/event0-2.
package bticino

import (
	"fmt"
	"time"
)

// BTicino Real Device Filesystem Paths
const (
	// Messages Directory (Answering Machine)
	MessagesDir       = "/home/bticino/cfg/extra/47/messages/"
	MessageDirPattern = "message_"
	MessageInfoFile   = "msg_info.ini"
	MessageImageFile  = "aswm.jpg"
	MessageVideoFile  = "aswm.avi"

	// Configuration Files
	DeviceConfigXML    = "/home/bticino/sp/dbfiles_ws.xml"
	SystemConfigXML    = "/var/tmp/conf.xml"
	StackOpenXML       = "/var/tmp/stack_open.xml"
	LicenseVersionFile = "/home/bticino/cfg/extra/.license_ver"
	FirmwareMetaXML    = "/home/bticino/cfg/extra/FW/meta.xml"

	// System Monitoring
	LEDsPath         = "/sys/class/leds"
	GPIOPath         = "/sys/class/gpio"
	InputEventDevice = "/dev/input/event0"
	TTYSerialDevice  = "/dev/ttymxc2"
	ThermalZonePath  = "/sys/class/thermal/thermal_zone0/temp"

	// Dispositivos de entrada (/dev/input/)
	InputDeviceKeypad      = "/dev/input/event0" // I2C_KB_TOUCH: teclado fisico (4 botones)
	InputDeviceTouchscreen = "/dev/input/event1" // TSC2005: pantalla tactil
	InputDeviceGPIOKeys    = "/dev/input/event2" // gpio-keys: teclas GPIO

	// Answering Machine Temp Storage
	AnsweringMachineTemp = "/var/tmp/segreteria"

	// Network and Ports
	OpenWebNetMainPort   = 20000
	OpenWebNetLocalPort  = 30006
	OpenWebNetConfigPort = 30007
	SIPServerPort        = 5060
	SIPMediaPort         = 5007
)

// ==================== COMANDOS OpenWebNet (verificados en dispositivo real) ====================

const (
	// --- Contestador automatico (Voicemail / Answering Machine) ---
	CmdVoicemailOn          = "*8*91##"                 // Activar contestador
	CmdVoicemailOff         = "*8*92##"                 // Desactivar contestador
	CmdVoicemailOnApp       = "*#8**40*1*0*9815*1*25##" // Activar via protocolo de app movil
	CmdVoicemailOffApp      = "*#8**40*0*0*9815*1*25##" // Desactivar via protocolo de app movil
	CmdVoicemailStatusQuery = "*#8**40##"               // Consultar estado contestador (resp: *#8**40*VM*WM*XXXX*X*XX##)

	// --- Estado de audio (DIM 35, WHO=8) ---
	// Nota: *#8**35*V1*V2*V3## es estado de audio, NO de voicemail.
	// V1=canal activo (0=inactivo, 1=iniciando, 3=conectando, 12=canal 12 activo), V2 y V3 = parametros adicionales
	CmdVoicemailStatus = "*#8**35*0*0*0##"  // DEPRECATED: nombre incorrecto, usar CmdAudioStatusIdle
	CmdAudioStatusIdle = "*#8**35*0*0*0##"  // Audio inactivo (silencio total)
	CmdAudioStatusInit = "*#8**35*1*0*0##"  // Audio canal 1: iniciando sesion — NUEVO 2026-03-12
	CmdAudioStatusConn = "*#8**35*3*0*0##"  // Audio canal 3: conectando/negociando — NUEVO 2026-03-12
	CmdAudioStatusBusy = "*#8**35*12*0*0##" // Audio activo en canal 12

	// --- Control de audio (capturado en sesion real) ---
	CmdAudioControl416 = "*8*3#5#4*416##" // Control audio canal 416 (hangup/fin llamada)

	// --- Modo de audio *8*87#N*## (WHO=8, WHAT=87#N) --- NUEVO 2026-03-12
	// Descubierto durante sesion de pruebas navegando menus del dispositivo.
	// Controla el modo de audio del sistema (codec/canal de audio activo).
	CmdAudioModeOff = "*8*87#0*##" // Audio OFF / inactivo (aparece al detener video/llamada)
	CmdAudioMode1   = "*8*87#1*##" // Audio modo 1: intercomunicacion / monitorizacion
	CmdAudioMode2   = "*8*87#2*##" // Audio modo 2: llamada interna
	CmdAudioMode4   = "*8*87#4*##" // Audio modo 4: Studio Profesional
	CmdAudioMode5   = "*8*87#5*##" // Audio modo 5: videollamada (con video stream activo)

	// --- Configuracion canales de audio (DIM 100#C y 101#C#S, WHO=8) --- NUEVO 2026-03-12
	// Descubierto durante sesion de pruebas. Configura parametros de codec/audio SIP/VoIP.
	// DIM 100#C: *#8**100#C*P1*P2*...## — Canal C con parametros
	// DIM 101#C#S: *#8**101#C#S*V## — Canal C, sub-parametro S, valor V (0 o 1)
	// Ejemplo real: *#8**100#3*1*2## — Canal 3: params 1, 2
	// Ejemplo real: *#8**101#3#1*1## — Canal 3, sub-param 1 = 1

	// --- WiFi (WHO=8, WHAT=101#N) --- NUEVO 2026-03-12
	// Descubierto al activar/desactivar WiFi desde el menu del dispositivo.
	CmdWiFiOff = "*8*101#0*##" // WiFi desactivado (WHO=8, WHAT=101#0)
	CmdWiFiOn  = "*8*101#1*##" // WiFi activado (WHO=8, WHAT=101#1)

	// --- MUTE ---
	CmdMuteOn  = "*8*30*20##" // Activar silencio (sin audio)
	CmdMuteOff = "*8*31*20##" // Desactivar silencio

	// --- Sonido del timbre (corregido segun slyoldfox: # antes de 33) ---
	CmdBellOn    = "*#8**#33*1##" // Activar sonido timbre (SET dimension 33 = 1)
	CmdBellOff   = "*#8**#33*0##" // Desactivar sonido timbre (SET dimension 33 = 0)
	CmdBellQuery = "*#8**33##"    // Consultar estado timbre (resp: *#8**33*0## muted o *#8**33*1## unmuted)

	// --- Cerradura / puerta ---
	// Requieren patron press + espera 1s + release
	CmdDoorOpenPress      = "*8*19*20##"  // Pulsar boton puerta (llave)
	CmdDoorOpenRelease    = "*8*20*20##"  // Soltar boton puerta
	CmdDoorOpenPress116   = "*8*19*116##" // Pulsar boton cerradura WHERE=116 — NUEVO 2026-03-12
	CmdDoorOpenRelease116 = "*8*20*116##" // Soltar boton cerradura WHERE=116 — NUEVO 2026-03-12
	CmdLightOnPress       = "*8*21*16##"  // Pulsar boton luz escalera (estrella)
	CmdLightOnRelease     = "*8*22*16##"  // Soltar boton luz escalera

	// --- Display / pantalla ---
	CmdDisplayOn  = "*7*73#1#100*##" // Encender pantalla (brillo maximo 100%)
	CmdDisplayDim = "*7*73#1#10*##"  // Atenuar pantalla (brillo 10%, modo salvapantallas)
	CmdDisplayOff = "*7*73#1#10*##"  // DEPRECATED: nombre incorrecto, no apaga sino atenua. Usar CmdDisplayDim

	// --- Estado del sistema ---
	CmdStatusRequest = "*#130**1##" // Solicitar estado del sistema
	CmdACK           = "*#*1##"     // Acknowledgment
	CmdNACK          = "*#*0##"     // Negative acknowledgment

	// --- Eventos de timbre (detectados en bus puerto 20000) ---
	CmdDoorbellPress = "*8*1#1#4#21*16##" // Llamada desde placa exterior (timbre)

	// --- Eventos de video/llamada (capturados en sesiones reales) ---
	CmdSelfCallCamera   = "*8*1#5#4#20*16##" // Auto-llamada: boton ojo activa camara
	CmdAnswerPickup     = "*8*2#5#4*16##"    // Descolgar: activar audio bidireccional
	CmdHangupIncoming   = "*8*3#1#4*416##"   // Colgar llamada entrante (WHERE=416)
	CmdHangupSelf       = "*8*3#5#4*416##"   // Colgar auto-llamada (WHERE=416)
	CmdHangupSelf420    = "*8*3#5#4*420##"   // Colgar auto-llamada (WHERE=420, variante) — NUEVO 2026-03-12
	CmdRingIncoming     = "*8*9#1#4*20##"    // Notificacion de timbre (llamada entrante)
	CmdRingSelf         = "*8*9#5#4*20##"    // Notificacion de auto-activacion (ojo)
	CmdVoiceActive      = "*8*40#1#4*20##"   // Voz activa en llamada entrante
	CmdIntercomActive   = "*8*100#5#4*20##"  // Intercomunicacion bidireccional activa
	CmdSessionCleanup   = "*8*94#11#0*##"    // Limpieza de sesion (canales 11, 0)
	CmdSessionCleanup23 = "*8*94#2#3*##"     // Limpieza de sesion (canales 2, 3) — NUEVO 2026-03-12
	CmdSessionEnd       = "*8*96#9#0*##"     // Fin de sesion completo

	// --- Video (WHO=7, capturados en sesiones reales) ---
	CmdVideoDisplayOn  = "*7*73#1#100*##" // Encender display (= CmdDisplayOn)
	CmdVideoDisplayOff = "*7*73#0#0*##"   // Apagar display (variante)
	CmdVideoStop       = "*7*55*##"       // Detener video
	CmdVideoDisconnect = "*7*219*##"      // Desconectar video
	CmdVideoStopFinal  = "*7*0*##"        // Detener video (final)
	CmdVideoKeepalive  = "*7*74*##"       // Keepalive durante conversacion
	CmdVideoTwoWay     = "*7*72*20##"     // Modo video bidireccional
	CmdVideoMode220    = "*7*220#0*##"    // Modo visualizacion video 220 (aparece al activar camara) — NUEVO 2026-03-12

	// --- Video streams (WHO=7, WHAT=35/36/50) --- NUEVO 2026-03-12
	// Descubierto durante sesion de pruebas al abrir camara y Studio Profesional.
	// *7*35#T#S*## — Iniciar stream video tipo T, fuente S
	// *7*36*## — Detener stream video
	// *7*50#1*## — Iniciar camara/video (activacion directa)
	CmdVideoStreamStart41 = "*7*35#4#1*##"  // Iniciar stream tipo 4 fuente 1 (videollamada)
	CmdVideoStreamStart32 = "*7*35#3#2**##" // Iniciar stream tipo 3 fuente 2 (nota: doble * antes de ##)
	CmdVideoStreamStop    = "*7*36*##"      // Detener stream video
	CmdVideoCameraStart   = "*7*50#1*##"    // Iniciar camara/video (activacion directa)

	// --- Ajustes de imagen de camara (DIM 5/6/7/20, WHO=7) --- NUEVO 2026-03-12
	// Descubierto al tocar ajustes de brillo/contraste/color en la vista de camara.
	// Formato dimension read: *#7**DIM*VALUE##
	// DIM 5 = Brillo (brightness). Rango 0-100. Ejemplo: *#7**5*40## = brillo 40%
	// DIM 6 = Contraste (contrast). Rango 0-100. Ejemplo: *#7**6*50## = contraste 50%
	// DIM 7 = Color/Saturacion (color/saturation). Rango 0-100. Ejemplo: *#7**7*50## = color 50%
	// DIM 20 = Config completa imagen (WHERE=20). Formato: *#7**20*BR*CO*CL*P4*P5##
	//          BR=brillo, CO=contraste, CL=color, P4=desconocido (75), P5=desconocido (50)
	//          Ejemplo real: *#7**20*40*50*50*75*50## = brillo 40, contraste 50, color 50

	// --- Control de volumen (DIM 31#1, WHO=7) --- NUEVO 2026-03-12
	// *#7**31#1*VALUE## — Valor de volumen actual (0-100)
	// Ejemplo real: *#7**31#1*80## = volumen al 80%

	// --- Info sistema (WHO=13, verificado en slyoldfox c300x-controller) ---
	// Estas consultas usan dimension read: *#13**DIM##
	// Respuesta: *#13**DIM*valor1*valor2*...##
	// En localhost (netcat 30006) no requieren autenticacion HMAC.
	CmdQueryIP      = "*#13**10##" // Direccion IP (resp: *#13**10*192*168*1*38##)
	CmdQueryNetmask = "*#13**11##" // Mascara de red (resp: *#13**11*255*255*255*0##)
	CmdQueryMAC     = "*#13**12##" // Direccion MAC (resp: *#13**12*D1*D2*D3*D4*D5*D6## en decimal)
	CmdQueryFWVer   = "*#13**16##" // Version firmware (resp: *#13**16*V1*V2*V3##)
	CmdQueryHWVer   = "*#13**17##" // Version hardware (resp: *#13**17*V1*V2*V3##)
	CmdQueryKernel  = "*#13**23##" // Version kernel (resp: *#13**23*V1*V2*V3##)
	CmdQueryDistro  = "*#13**24##" // Version distribucion (resp: *#13**24*V1*V2*V3##)

	// --- Estado del sistema WHO=130 --- NUEVO 2026-03-12
	// *#130**1*2## — Respuesta de estado del sistema (WHO=130, WHERE=1)
	CmdSystemStatus = "*#130**1*2##" // Respuesta estado del sistema (valor 2)
)

// MQTT Topics for Home Assistant Integration
const (
	// Base topic prefix
	MQTTTopicPrefix = "video_intercom"

	// State topics
	TopicLockState      = "video_intercom/lock/state"
	TopicDisplayState   = "video_intercom/display/state"
	TopicVoicemailState = "video_intercom/voicemail/state"
	TopicDoorbellSound  = "video_intercom/doorbellsound/state"
	TopicDoorbellState  = "video_intercom/doorbell/state"
	TopicKeypadState    = "video_intercom/keypad/state"
	TopicLEDsState      = "video_intercom/leds/state"
	TopicGPIOState      = "video_intercom/gpio/state"

	// Command topics
	TopicLockSet          = "video_intercom/lock/set"
	TopicVoicemailSet     = "video_intercom/voicemail/set"
	TopicDoorbellSoundSet = "video_intercom/doorbellsound/set"

	// Message management command topics
	TopicMessagesRefresh     = "video_intercom/messages/commands/refresh"
	TopicMessagesMarkRead    = "video_intercom/messages/commands/mark_read"
	TopicMessagesDelete      = "video_intercom/messages/commands/delete"
	TopicMessagesMarkAllRead = "video_intercom/messages/commands/mark_all_read"
	TopicMessagesDownload    = "video_intercom/messages/commands/download"

	// Availability topic
	TopicAvailability = "video_intercom/state"

	// Home Assistant Discovery Topics
	HADiscoveryPrefix       = "homeassistant"
	HALockConfig            = "homeassistant/lock/intercom/door/config"
	HADisplayConfig         = "homeassistant/sensor/intercom/display/config"
	HAVoicemailConfig       = "homeassistant/switch/intercom/voicemail/config"
	HADoorbellSoundConfig   = "homeassistant/switch/intercom/doorbellsound/config"
	HADoorbellTriggerConfig = "homeassistant/device_automation/intercom/doorbell/config"
	HAKeypadConfig          = "homeassistant/event/intercom/keypad/config"
)

// ==================== BOTONES FISICOS (verificados en /dev/input/event0) ====================
//
// El teclado fisico es un I2C_KB_TOUCH conectado a /dev/input/event0.
// Cada boton genera un evento EV_KEY con los codigos siguientes.
//
// | Boton     | Codigo | Funcion fisica                    | Comando OWN generado                         |
// |-----------|--------|-----------------------------------|----------------------------------------------|
// | Llave     | 2      | Abrir puerta (cerradura)          | *8*19*20## + *8*20*20## (press+release)       |
// | Estrella  | 3      | Encender luz escalera             | *8*21*16## + *8*22*16## (press+release)       |
// | Ojo       | 4      | Activar camara (ver+escuchar)     | Inicia secuencia *8*1#5#4#20*16##             |
// | Telefono  | 5      | Comunicacion bidireccional (mic)  | *8*2#5#4*16## (solo con camara activa)        |
//
// Notas:
// - Pulsacion larga de llave mantiene el rele de puerta abierto mas tiempo
// - Ojo activa la camara ~30 segundos, luego se apaga automaticamente
// - Telefono solo funciona cuando la camara ya esta activa (por ojo o llamada entrante)
const (
	KeyCodeKey   = 2 // Llave: abrir puerta
	KeyCodeStar  = 3 // Estrella: luz escalera
	KeyCodeEye   = 4 // Ojo: activar camara
	KeyCodePhone = 5 // Telefono: comunicacion bidireccional

	EventTypeKeypress = 1
	EventValuePress   = 1
	EventValueRelease = 0
)

// KeyNames mapea codigos de tecla a nombres descriptivos en espanol
var KeyNames = map[int]string{
	KeyCodeKey:   "llave",
	KeyCodeStar:  "estrella",
	KeyCodeEye:   "ojo",
	KeyCodePhone: "telefono",
}

// Message Types for Answering Machine
const (
	MessageTypeVoice  = "voice_message"
	MessageTypeDoor   = "door_event"
	MessageTypeSystem = "system_message"
)

// BTicino Device Information
const (
	DeviceManufacturer = "Bticino-Legrand"
	DeviceModel        = "Classe 300 X13E"
	DeviceModelShort   = "C300X"

	// Network settings
	DefaultNetworkInterface = "lo"
	DefaultGateway          = "192.168.1.1"

	// SIP Configuration
	SIPDomain          = "bs.iotleg.com"
	SIPServer          = "sipserver.bs.iotleg.com"
	SIPDefaultUsername = "c300x"

	// MyHome Web Portal
	MyHomePortal     = "www.myhomeweb.com"
	MyHomePortalPort = 25100
	MyHomeHTTPSPort  = 443
)

// Timeouts and Intervals
const (
	OpenWebNetTimeout      = 30 * time.Second
	CommandRetryDelay      = 310 * time.Millisecond // Critical: do not change from 0.31s
	StatusPollingInterval  = 5 * time.Minute
	HealthCheckInterval    = 1 * time.Minute
	MessageRefreshInterval = 30 * time.Second
)

// ==================== PATRONES OWN DECODIFICADOS (capturados de eventos reales) ====================
//
// Estructura de comandos OpenWebNet para videoportero (WHO=8, WHO=7, WHO=13, WHO=130):
//
// WHO=8 (Interfono/Videoportero):
//   *8*WHAT*WHERE##  (comando normal)
//   *#8**DIM*VAL##   (dimension read/status)
//   *#8**#DIM*VAL##  (dimension write/set — nota el # extra)
//
//   WHAT codifica la accion:
//     1#X#Y#Z  = Llamada (X=1 entrante, X=5 auto-activacion; Y#Z = origen)
//     2#X#Y    = Descolgar/responder
//     3#X#Y    = Colgar
//     9#X#Y    = Notificacion de timbre
//     19       = Pulsar boton puerta (press)
//     20       = Soltar boton puerta (release)
//     21       = Pulsar boton luz escalera (press)
//     22       = Soltar boton luz escalera (release)
//     30       = Mute ON
//     31       = Mute OFF
//     40#X#Y   = Voz activa en llamada
//     87#N     = Modo de audio (N: 0=off, 1=intercom, 2=interna, 4=studio, 5=video) [NUEVO]
//     91       = Contestador ON
//     92       = Contestador OFF
//     94#X#Y   = Limpieza de sesion (X#Y = canales)
//     96#X#Y   = Fin de sesion
//     100#X#Y  = Intercomunicacion activa
//     101#N    = WiFi (N: 0=off, 1=on) [NUEVO]
//
//   DIM (dimensiones WHO=8):
//     33       = Sonido timbre (read: *#8**33## / write: *#8**#33*V##, V: 0=muted, 1=unmuted)
//     35       = Estado audio (*#8**35*CH*P1*P2##, CH: 0=inactivo, 1=iniciando, 3=conectando, 12=activo)
//     40       = Estado contestador (*#8**40*VM*WM*XXXX*X*XX##, VM=voicemail, WM=welcome msg)
//     100#C    = Config canal audio C (*#8**100#C*P1*P2*...##) [NUEVO]
//     101#C#S  = Sub-param S canal C (*#8**101#C#S*V##, V: 0 o 1) [NUEVO]
//
// WHO=7 (Video/Multimedia):
//   *7*WHAT*WHERE##
//     0   = Detener video (final)
//     35#T#S = Iniciar stream video tipo T fuente S [NUEVO]
//     36  = Detener stream video [NUEVO]
//     50#1 = Iniciar camara/video (activacion directa) [NUEVO]
//     55  = Detener video
//     72  = Modo video bidireccional
//     73#B#V = Control display (B=1 encender; V=porcentaje brillo 0-100)
//     74  = Keepalive video
//     77#... = Parametros resolucion video (800x480, etc.)
//     219 = Desconectar video
//     220#M = Modo visualizacion video (M=0 normal) [NUEVO]
//     300#IP#PORT#T = Inicio stream RTP (IP, puerto, tipo)
//
//   DIM (dimensiones WHO=7):
//     5    = Brillo imagen (*#7**5*VALUE##, VALUE: 0-100%) [NUEVO]
//     6    = Contraste imagen (*#7**6*VALUE##, VALUE: 0-100%) [NUEVO]
//     7    = Color/Saturacion imagen (*#7**7*VALUE##, VALUE: 0-100%) [NUEVO]
//     20   = Config completa imagen (*#7**20*BR*CO*CL*P4*P5##) [NUEVO]
//            BR=brillo, CO=contraste, CL=color, P4/P5=desconocidos (75/50)
//     31#1 = Volumen (*#7**31#1*VALUE##, VALUE: 0-100%) [NUEVO]
//
// WHO=13 (Info Sistema):
//   *#13**DIM## -> *#13**DIM*V1*V2*...##
//     10 = IP, 11 = Netmask, 12 = MAC, 16 = FW, 17 = HW, 23 = Kernel, 24 = Distro
//
// WHO=130 (Estado Sistema):
//   *#130**WHERE*VALUE## — Estado general del sistema
//
// Nota sobre WHERE vacio: El dispositivo a veces envia WHERE vacio (trailing *)
// Ejemplo: *8*91*## en vez de *8*91##. Ambos son equivalentes.

// CommandResponses mapea comandos OWN conocidos a su descripcion legible
var CommandResponses = map[string]string{
	// Pantalla
	"*7*73#1#100*##": "pantalla ON (brillo 100%)",
	"*7*73#1#10*##":  "pantalla atenuada (brillo 10%, salvapantallas)",
	"*7*73#0#0*##":   "pantalla OFF (variante)",
	// Video
	"*7*55*##":      "video detenido",
	"*7*219*##":     "video desconectado",
	"*7*0*##":       "video detenido (final)",
	"*7*74*##":      "video keepalive",
	"*7*72*20##":    "video bidireccional",
	"*7*220#0*##":   "modo visualizacion video (normal)",
	"*7*35#4#1*##":  "stream video tipo 4 fuente 1 (videollamada)",
	"*7*35#3#2**##": "stream video tipo 3 fuente 2",
	"*7*36*##":      "stream video detenido",
	"*7*50#1*##":    "camara/video iniciado",
	// Sonido timbre
	"*#8**33*0##": "sonido timbre OFF",
	"*#8**33*1##": "sonido timbre ON",
	// Contestador
	"*8*92##":                 "contestador OFF",
	"*8*91##":                 "contestador ON",
	"*#8**40*0*0*9815*1*25##": "contestador OFF (app)",
	"*#8**40*1*0*9815*1*25##": "contestador ON (app)",
	// Estado audio (DIM 35)
	"*#8**35*0*0*0##":  "audio inactivo (silencio)",
	"*#8**35*1*0*0##":  "audio iniciando sesion (canal 1)",
	"*#8**35*3*0*0##":  "audio conectando/negociando (canal 3)",
	"*#8**35*12*0*0##": "audio activo canal 12",
	// Modos de audio (WHAT=87#N)
	"*8*87#0*##": "audio modo OFF / inactivo",
	"*8*87#1*##": "audio modo 1 (intercomunicacion)",
	"*8*87#2*##": "audio modo 2 (llamada interna)",
	"*8*87#4*##": "audio modo 4 (Studio Profesional)",
	"*8*87#5*##": "audio modo 5 (videollamada)",
	// WiFi
	"*8*101#0*##": "WiFi desactivado",
	"*8*101#1*##": "WiFi activado",
	// Puerta
	"*8*19*20##": "puerta: boton pulsado (press)",
	"*8*20*20##": "puerta: boton soltado (release)",
	// Luz escalera
	"*8*21*16##": "luz escalera: boton pulsado (press)",
	"*8*22*16##": "luz escalera: boton soltado (release)",
	// Mute
	"*8*30*20##": "silencio ON",
	"*8*31*20##": "silencio OFF",
	// Llamadas y sesiones
	"*8*1#1#4#21*16##": "llamada entrante desde placa exterior (timbre)",
	"*8*1#5#4#20*16##": "auto-llamada: camara activada (boton ojo)",
	"*8*2#5#4*16##":    "comunicacion bidireccional activada (boton telefono)",
	"*8*9#1#4*20##":    "notificacion timbre (llamada entrante)",
	"*8*9#5#4*20##":    "notificacion auto-activacion (ojo)",
	"*8*3#1#4*416##":   "llamada entrante finalizada (colgar)",
	"*8*3#5#4*416##":   "auto-llamada finalizada (colgar)",
	"*8*3#5#4*420##":   "auto-llamada finalizada (colgar, variante WHERE=420)",
	"*8*40#1#4*20##":   "voz activa en llamada entrante",
	"*8*100#5#4*20##":  "intercomunicacion bidireccional activa",
	"*8*94#11#0*##":    "limpieza de sesion (canales 11, 0)",
	"*8*94#2#3*##":     "limpieza de sesion (canales 2, 3)",
	"*8*96#9#0*##":     "fin de sesion completo",
	// ACK/NACK
	"*#*1##": "ACK",
	"*#*0##": "NACK",
	// Estado
	"*#130**1##":   "solicitud de estado",
	"*#130**1*2##": "respuesta estado del sistema",
	// Consulta estado timbre (respuestas)
	"*#8**33##":    "consulta estado timbre",
	"*#8**#33*0##": "SET sonido timbre OFF",
	"*#8**#33*1##": "SET sonido timbre ON",
	// Consulta estado voicemail
	"*#8**40##": "consulta estado contestador",
	// Cerraduras adicionales
	"*8*19*21##": "cerradura 1: boton pulsado (press)",
	"*8*20*21##": "cerradura 1: boton soltado (release)",
	"*8*19*22##": "cerradura 2: boton pulsado (press)",
	"*8*20*22##": "cerradura 2: boton soltado (release)",
	// Cerradura WHERE=116
	"*8*19*116##": "cerradura 116: boton pulsado (press)",
	"*8*20*116##": "cerradura 116: boton soltado (release)",
}

// OWNEventDescriptions mapea patrones OWN a descripciones legibles en espanol
// para el sensor activity_log de Home Assistant.
// Los patrones se verifican con strings.HasPrefix o coincidencia exacta.
var OWNEventDescriptions = map[string]string{
	"*8*1#1#4#21*16##": "Timbre: llamada desde placa exterior",
	"*8*1#5#4#20*16##": "Camara activada (boton ojo)",
	"*8*2#5#4*16##":    "Comunicacion bidireccional activa",
	"*8*9#1#4*20##":    "Notificacion de timbre entrante",
	"*8*9#5#4*20##":    "Notificacion de auto-activacion",
	"*8*3#1#4*416##":   "Llamada entrante finalizada",
	"*8*3#5#4*416##":   "Auto-llamada finalizada",
	"*8*3#5#4*420##":   "Auto-llamada finalizada (variante WHERE=420)",
	"*8*40#1#4*20##":   "Voz activa en llamada entrante",
	"*8*100#5#4*20##":  "Intercomunicacion bidireccional activa",
	"*8*19*20##":       "Puerta abierta (boton llave)",
	"*8*20*20##":       "Puerta: boton soltado",
	"*8*21*16##":       "Luz escalera activada (boton estrella)",
	"*8*22*16##":       "Luz escalera: boton soltado",
	"*8*91##":          "Contestador activado",
	"*8*92##":          "Contestador desactivado",
	"*8*30*20##":       "Silencio activado",
	"*8*31*20##":       "Silencio desactivado",
	"*8*94#11#0*##":    "Limpieza de sesion (canales 11, 0)",
	"*8*94#2#3*##":     "Limpieza de sesion (canales 2, 3)",
	"*8*96#9#0*##":     "Sesion finalizada",
	// Modos de audio (WHAT=87#N) — NUEVO 2026-03-12
	"*8*87#0*##": "Audio modo OFF / inactivo",
	"*8*87#1*##": "Audio modo 1 (intercomunicacion)",
	"*8*87#2*##": "Audio modo 2 (llamada interna)",
	"*8*87#4*##": "Audio modo 4 (Studio Profesional)",
	"*8*87#5*##": "Audio modo 5 (videollamada)",
	// WiFi — NUEVO 2026-03-12
	"*8*101#0*##": "WiFi desactivado",
	"*8*101#1*##": "WiFi activado",
	// Video streams — NUEVO 2026-03-12
	"*7*35#4#1*##":  "Stream video tipo 4 fuente 1 iniciado",
	"*7*35#3#2**##": "Stream video tipo 3 fuente 2 iniciado",
	"*7*36*##":      "Stream video detenido",
	"*7*50#1*##":    "Camara/video iniciado",
	// Pantalla y video existentes
	"*7*73#1#100*##": "Pantalla encendida (brillo 100%)",
	"*7*73#1#10*##":  "Pantalla atenuada (brillo 10%, salvapantallas)",
	"*7*55*##":       "Video detenido",
	"*7*219*##":      "Video desconectado",
	"*7*220#0*##":    "Modo visualizacion video (normal)",
	// Cerraduras adicionales
	"*8*19*21##": "Cerradura 1 abierta",
	"*8*20*21##": "Cerradura 1: boton soltado",
	"*8*19*22##": "Cerradura 2 abierta",
	"*8*20*22##": "Cerradura 2: boton soltado",
	// Cerradura WHERE=116 — NUEVO 2026-03-12
	"*8*19*116##": "Cerradura 116 abierta",
	"*8*20*116##": "Cerradura 116: boton soltado",
	// Estado timbre (eventos del bus)
	"*#8**33*0##": "Sonido timbre desactivado (muted)",
	"*#8**33*1##": "Sonido timbre activado (unmuted)",
	// Estado audio (DIM 35) — NUEVO 2026-03-12
	"*#8**35*0*0*0##":  "Audio inactivo (silencio total)",
	"*#8**35*1*0*0##":  "Audio iniciando sesion (canal 1)",
	"*#8**35*3*0*0##":  "Audio conectando/negociando (canal 3)",
	"*#8**35*12*0*0##": "Audio activo canal 12",
	// Estado sistema (WHO=130) — NUEVO 2026-03-12
	"*#130**1*2##": "Estado del sistema (valor 2)",
}

// OWNEventTypes mapea patrones OWN a tipos de evento para automatizaciones HA
var OWNEventTypes = map[string]string{
	"*8*1#1#4#21*16##": "doorbell_ring",
	"*8*1#5#4#20*16##": "camera_activated",
	"*8*2#5#4*16##":    "intercom_active",
	"*8*3#1#4*416##":   "call_ended",
	"*8*3#5#4*416##":   "call_ended",
	"*8*3#5#4*420##":   "call_ended",
	"*8*19*20##":       "door_open",
	"*8*20*20##":       "door_release",
	"*8*21*16##":       "stairlight_on",
	"*8*22*16##":       "stairlight_release",
	"*8*91##":          "voicemail_on",
	"*8*92##":          "voicemail_off",
	"*8*94#11#0*##":    "session_cleanup",
	"*8*94#2#3*##":     "session_cleanup",
	"*8*96#9#0*##":     "session_end",
	// Cerraduras adicionales
	"*8*19*21##": "door_open_lock1",
	"*8*20*21##": "door_release_lock1",
	"*8*19*22##": "door_open_lock2",
	"*8*20*22##": "door_release_lock2",
	// Cerradura WHERE=116 — NUEVO 2026-03-12
	"*8*19*116##": "door_open_lock116",
	"*8*20*116##": "door_release_lock116",
	// Estado timbre
	"*#8**33*0##": "bell_muted",
	"*#8**33*1##": "bell_unmuted",
	// Modos de audio — NUEVO 2026-03-12
	"*8*87#0*##": "audio_mode_off",
	"*8*87#1*##": "audio_mode_intercom",
	"*8*87#2*##": "audio_mode_internal",
	"*8*87#4*##": "audio_mode_studio",
	"*8*87#5*##": "audio_mode_videocall",
	// WiFi — NUEVO 2026-03-12
	"*8*101#0*##": "wifi_off",
	"*8*101#1*##": "wifi_on",
	// Video streams — NUEVO 2026-03-12
	"*7*35#4#1*##":  "video_stream_start",
	"*7*35#3#2**##": "video_stream_start",
	"*7*36*##":      "video_stream_stop",
	"*7*50#1*##":    "camera_start",
	// Pantalla
	"*7*73#1#100*##": "display_on",
	"*7*73#1#10*##":  "display_dim",
	// Video modo visualizacion — NUEVO 2026-03-12
	"*7*220#0*##": "video_mode_display",
	// Estado audio — NUEVO 2026-03-12
	"*#8**35*0*0*0##":  "audio_idle",
	"*#8**35*1*0*0##":  "audio_initializing",
	"*#8**35*3*0*0##":  "audio_connecting",
	"*#8**35*12*0*0##": "audio_active",
	// Estado sistema — NUEVO 2026-03-12
	"*#130**1*2##": "system_status",
}

// ==================== GPIO (verificados mediante pruebas fisicas) ====================
//
// | GPIO | Funcion confirmada                  | Evidencia                                             |
// |------|--------------------------------------|-------------------------------------------------------|
// | 12   | Amplificador audio (altavoz activo)  | ON con video/llamada, OFF al colgar                   |
// | 13   | Desconocido                          | Sin cambios en ninguna prueba                          |
// | 47   | Permanente ON                        | Hardware/alimentacion                                  |
// | 49   | Sensor proximidad / despertar        | Solo se activa con timbre externo (pulso 3s)           |
// | 52   | Permanente ON                        | Hardware/alimentacion                                  |
// | 54   | Permanente ON                        | Hardware/alimentacion                                  |
// | 56   | Modo inactivo/standby                | ON=reposo. OFF cuando comms bidireccionales activas    |
// | 58   | Desconocido                          | Sin cambios en ninguna prueba                          |
// | 60   | Rele audio bidireccional (microfono) | ON solo al pulsar telefono. OFF=solo escucha           |
// | 154  | Indicador llamada entrante           | Pulso breve solo con timbre externo                    |
// | 155  | Conversacion activa (entrante)       | ON durante conversacion entrante, no en auto-activacion|
// | 176  | Permanente ON                        | Hardware/bus SCS                                       |
// | 180  | Comunicacion establecida             | ON durante cualquier comunicacion activa               |

// GPIOPins lista de todos los pines GPIO monitorizados
var GPIOPins = []int{12, 13, 47, 49, 52, 54, 56, 58, 60, 154, 155, 176, 180}

// GPIONames mapea pin GPIO a nombre legible (legacy: usado por webserver)
var GPIONames = []string{
	"gpio12", "gpio13", "gpio47", "gpio49", "gpio52",
	"gpio54", "gpio56", "gpio58", "gpio60",
	"gpio154", "gpio155", "gpio176", "gpio180",
}

// GPIOFunctions mapea pin GPIO a descripcion de su funcion confirmada
var GPIOFunctions = map[int]string{
	12:  "Amplificador audio (altavoz)",
	13:  "Desconocido",
	47:  "Permanente ON (alimentacion)",
	49:  "Sensor proximidad / despertar",
	52:  "Permanente ON (alimentacion)",
	54:  "Permanente ON (alimentacion)",
	56:  "Modo inactivo/standby",
	58:  "Desconocido",
	60:  "Rele audio bidireccional (microfono)",
	154: "Indicador llamada entrante",
	155: "Conversacion activa (entrante)",
	176: "Permanente ON (bus SCS)",
	180: "Comunicacion establecida",
}

// ==================== LEDs (verificados en /sys/class/leds/) ====================
//
// | LED sysfs       | Funcion confirmada                                                |
// |-----------------|-------------------------------------------------------------------|
// | led_lock        | Puerta abierta: se ilumina durante pulsacion larga de llave       |
// | led_vct_green   | Comunicacion video activa: ON durante sesion video/audio           |
// | led_memo        | Mensajes pendientes: se apaga al acceder al menu de mensajes      |
// | led_exc_call    | Desconocido (nunca visto ON en pruebas)                           |
// | led_gwifi       | Desconocido (nunca visto ON en pruebas)                           |
// | led_ans_machine | Desconocido (nunca visto ON en pruebas)                           |
// | led_vct_red     | Desconocido (nunca visto ON en pruebas)                           |
//
// Nota: Cada boton fisico tambien tiene su propio LED, pero estos son controlados
// por el chip I2C del teclado, no via GPIO sysfs.

// LEDNames lista de LEDs sysfs del dispositivo (nombres reales en /sys/class/leds/)
var LEDNames = []string{
	"led_lock",
	"led_vct_green",
	"led_vct_red",
	"led_memo",
	"led_exc_call",
	"led_gwifi",
	"led_ans_machine",
}

// LEDFunctions mapea nombre LED sysfs a descripcion de su funcion confirmada
var LEDFunctions = map[string]string{
	"led_lock":        "Puerta abierta (cerradura activa)",
	"led_vct_green":   "Comunicacion video activa",
	"led_vct_red":     "Desconocido",
	"led_memo":        "Mensajes pendientes",
	"led_exc_call":    "Desconocido (llamada perdida?)",
	"led_gwifi":       "Desconocido (WiFi?)",
	"led_ans_machine": "Desconocido (contestador?)",
}

// Network Filter Ports (for tcpdump filtering)
var FilteredPorts = []int{
	5007,  // SIP media
	5060,  // SIP signaling
	20000, // OpenWebNet main
	30006, // OpenWebNet local
}

// Home Assistant Device Class Mappings
const (
	HADeviceClassLock    = "lock"
	HADeviceClassSensor  = "sensor"
	HADeviceClassSwitch  = "switch"
	HADeviceClassTrigger = "device_trigger"
	HADeviceClassEvent   = "event"

	// Icons
	HAIconLock        = "mdi:lock"
	HAIconTablet      = "mdi:tablet"
	HAIconVoicemail   = "mdi:voicemail"
	HAIconBell        = "mdi:bell"
	HAIconDoorbell    = "mdi:doorbell"
	HAIconDialpad     = "mdi:dialpad"
	HAIconMessage     = "mdi:phone-message"
	HAIconActivityLog = "mdi:timeline-text"
)

// ==================== FUNCIONES HELPER ====================

// BuildDoorOpenPress genera el comando OWN de pulsar boton puerta para un WHERE dado.
// Ejemplo: BuildDoorOpenPress(20) -> "*8*19*20##", BuildDoorOpenPress(21) -> "*8*19*21##"
func BuildDoorOpenPress(where int) string {
	return fmt.Sprintf("*8*19*%d##", where)
}

// BuildDoorOpenRelease genera el comando OWN de soltar boton puerta para un WHERE dado.
func BuildDoorOpenRelease(where int) string {
	return fmt.Sprintf("*8*20*%d##", where)
}
