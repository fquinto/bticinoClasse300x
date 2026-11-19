package mqtt

import (
	"fmt"
	"strings"
)

// Entity display names for Home Assistant MQTT discovery.
//
// HA has no translation layer for MQTT-discovered entities (there is no
// translation_key for them), so the bridge itself must send the name in the
// language the user wants (config: mqtt.language).
//
// Only the discovery "name" is localised. unique_id and object_id never
// change with the language, so existing entity_ids, history and dashboards
// keep working when the language is switched.
//
// Catalogs are keyed by discovery object_id. "es" is the historical language
// and the fallback: a missing key in the chosen language falls back to "es",
// and a missing key in "es" keeps the name hard-coded at the call site.
// To add a language see docs/HOME_ASSISTANT.md ("Entity names and language").

// defaultLanguage is used when mqtt.language is empty or unknown.
const defaultLanguage = "es"

// Keys for names built from a template at the call site (dynamic entities).
const (
	nameKeyGPIO           = "_gpio"            // %d = pin number
	nameKeyAdditionalLock = "_additional_lock" // %d = 1-based position in additional_locks
)

var entityNames = map[string]map[string]string{
	"es": {
		"bticino_door":                 "Puerta",
		"bticino_voicemail":            "Contestador",
		"bticino_doorbell_sound":       "Sonido Timbre",
		"bticino_display":              "Pantalla",
		"bticino_staircase_light":      "Luz Escalera",
		"bticino_door_open":            "Abrir Puerta",
		"bticino_contestar":            "Contestar",
		"bticino_colgar":               "Colgar",
		"bticino_contestador":          "Contestador",
		"bticino_ver_placa":            "Ver placa",
		"bticino_placa_en_vista":       "Placa en vista",
		"bticino_temperature":          "Temperatura Dispositivo",
		"bticino_new_messages":         "Mensajes Nuevos",
		"bticino_total_messages":       "Total Mensajes",
		"bticino_storage":              "Almacenamiento Contestador",
		"bticino_keypad":               "Teclado",
		"bticino_doorbell":             "Timbre",
		"bticino_led_memo":             "LED Mensaje",
		"bticino_led_wifi":             "LED WiFi",
		"bticino_led_answering":        "LED Contestador",
		"bticino_led_lock":             "LED Cerradura",
		"bticino_led_missed_call":      "LED Llamada Perdida",
		"bticino_led_call_green":       "LED Llamada Verde",
		"bticino_led_call_red":         "LED Llamada Rojo",
		"bticino_bus_event":            "Evento Bus OWN",
		"bticino_sip_status":           "Estado SIP",
		"bticino_system_diag":          "Diagnostico Sistema",
		"bticino_multicast":            "Mensajes Multicast",
		"bticino_activity_log":         "Registro de Actividad",
		"bticino_system_info":          "Informacion Sistema",
		"bticino_language":             "Idioma Sistema",
		"bticino_timezone":             "Zona Horaria",
		"bticino_ntp":                  "Servidor NTP",
		"bticino_datetime":             "Fecha y Hora Sistema",
		"bticino_answering_status":     "Estado Contestador",
		"bticino_answering_memory":     "Memoria Contestador",
		"bticino_ringtone_s0":          "Timbre Placa Exterior",
		"bticino_volume_s0":            "Volumen S0",
		"bticino_volume_door":          "Volumen Puerta",
		"bticino_display_brightness":   "Brillo Display",
		"bticino_camera_20_brightness": "Brillo Camara 20",
		"bticino_call_state":           "Estado llamada",
		"bticino_call_caller":          "Quién llama",
		"bticino_doorbell_floor":       "Timbre rellano",
		"bticino_event_doorbell":       "Timbre (evento)",
		"bticino_event_doorbell_floor": "Timbre rellano (evento)",
		"bticino_event_call":           "Llamada (evento)",
		"bticino_event_keys":           "Teclas (evento)",
		"bticino_event_bus_own":        "Evento Bus OWN",
		"bticino_ip_address":           "Dirección IP",
		"bticino_mac_address":          "Dirección MAC",
		"bticino_wifi_signal":          "Señal WiFi",
		"bticino_mark_all_read":        "Marcar todos leídos",
		"bticino_restart_bridge":       "Reiniciar bridge",
		"bticino_start_ssh":            "Arrancar SSH",
		"bticino_reboot_device":        "Reiniciar dispositivo",
		"bticino_doorbell_camera":      "Cámara timbre",
		"bticino_access":               "Acceso portal",
		"bticino_last_access":          "Último acceso",
		nameKeyGPIO:                    "GPIO %d",
		nameKeyAdditionalLock:          "Cerradura %d",
	},
	"en": {
		"bticino_door":                 "Door",
		"bticino_voicemail":            "Answering machine",
		"bticino_doorbell_sound":       "Doorbell sound",
		"bticino_display":              "Display",
		"bticino_staircase_light":      "Stair light",
		"bticino_door_open":            "Open door",
		"bticino_contestar":            "Answer",
		"bticino_colgar":               "Hang up",
		"bticino_contestador":          "Answering machine",
		"bticino_ver_placa":            "View door panel",
		"bticino_placa_en_vista":       "Door panel in view",
		"bticino_temperature":          "Device temperature",
		"bticino_new_messages":         "New messages",
		"bticino_total_messages":       "Total messages",
		"bticino_storage":              "Answering machine storage",
		"bticino_keypad":               "Keypad",
		"bticino_doorbell":             "Doorbell",
		"bticino_led_memo":             "Message LED",
		"bticino_led_wifi":             "WiFi LED",
		"bticino_led_answering":        "Answering machine LED",
		"bticino_led_lock":             "Lock LED",
		"bticino_led_missed_call":      "Missed call LED",
		"bticino_led_call_green":       "Call LED green",
		"bticino_led_call_red":         "Call LED red",
		"bticino_bus_event":            "OWN bus event",
		"bticino_sip_status":           "SIP status",
		"bticino_system_diag":          "System diagnostics",
		"bticino_multicast":            "Multicast messages",
		"bticino_activity_log":         "Activity log",
		"bticino_system_info":          "System information",
		"bticino_language":             "System language",
		"bticino_timezone":             "Time zone",
		"bticino_ntp":                  "NTP server",
		"bticino_datetime":             "System date and time",
		"bticino_answering_status":     "Answering machine status",
		"bticino_answering_memory":     "Answering machine memory",
		"bticino_ringtone_s0":          "Door panel ringtone",
		"bticino_volume_s0":            "S0 volume",
		"bticino_volume_door":          "Door volume",
		"bticino_display_brightness":   "Display brightness",
		"bticino_camera_20_brightness": "Camera 20 brightness",
		"bticino_call_state":           "Call state",
		"bticino_call_caller":          "Caller",
		"bticino_doorbell_floor":       "Floor doorbell",
		"bticino_event_doorbell":       "Doorbell (event)",
		"bticino_event_doorbell_floor": "Floor doorbell (event)",
		"bticino_event_call":           "Call (event)",
		"bticino_event_keys":           "Keys (event)",
		"bticino_event_bus_own":        "OWN bus event",
		"bticino_ip_address":           "IP address",
		"bticino_mac_address":          "MAC address",
		"bticino_wifi_signal":          "WiFi signal",
		"bticino_mark_all_read":        "Mark all as read",
		"bticino_restart_bridge":       "Restart bridge",
		"bticino_start_ssh":            "Start SSH",
		"bticino_reboot_device":        "Reboot device",
		"bticino_doorbell_camera":      "Doorbell camera",
		"bticino_access":               "Building access",
		"bticino_last_access":          "Last access",
		nameKeyGPIO:                    "GPIO %d",
		nameKeyAdditionalLock:          "Additional lock %d",
	},
	"it": {
		"bticino_door":                 "Porta",
		"bticino_voicemail":            "Segreteria",
		"bticino_doorbell_sound":       "Suoneria campanello",
		"bticino_display":              "Schermo",
		"bticino_staircase_light":      "Luce scale",
		"bticino_door_open":            "Apri porta",
		"bticino_contestar":            "Rispondi",
		"bticino_colgar":               "Chiudi chiamata",
		"bticino_contestador":          "Segreteria",
		"bticino_ver_placa":            "Vedi targa",
		"bticino_placa_en_vista":       "Targa in visione",
		"bticino_temperature":          "Temperatura dispositivo",
		"bticino_new_messages":         "Nuovi messaggi",
		"bticino_total_messages":       "Messaggi totali",
		"bticino_storage":              "Memoria segreteria",
		"bticino_keypad":               "Tastiera",
		"bticino_doorbell":             "Campanello",
		"bticino_led_memo":             "LED messaggio",
		"bticino_led_wifi":             "LED WiFi",
		"bticino_led_answering":        "LED segreteria",
		"bticino_led_lock":             "LED serratura",
		"bticino_led_missed_call":      "LED chiamata persa",
		"bticino_led_call_green":       "LED chiamata verde",
		"bticino_led_call_red":         "LED chiamata rosso",
		"bticino_bus_event":            "Evento bus OWN",
		"bticino_sip_status":           "Stato SIP",
		"bticino_system_diag":          "Diagnostica sistema",
		"bticino_multicast":            "Messaggi multicast",
		"bticino_activity_log":         "Registro attività",
		"bticino_system_info":          "Informazioni sistema",
		"bticino_language":             "Lingua sistema",
		"bticino_timezone":             "Fuso orario",
		"bticino_ntp":                  "Server NTP",
		"bticino_datetime":             "Data e ora sistema",
		"bticino_answering_status":     "Stato segreteria",
		"bticino_answering_memory":     "Memoria segreteria",
		"bticino_ringtone_s0":          "Suoneria targa esterna",
		"bticino_volume_s0":            "Volume S0",
		"bticino_volume_door":          "Volume porta",
		"bticino_display_brightness":   "Luminosità schermo",
		"bticino_camera_20_brightness": "Luminosità telecamera 20",
		"bticino_call_state":           "Stato chiamata",
		"bticino_call_caller":          "Chi chiama",
		"bticino_doorbell_floor":       "Campanello di piano",
		"bticino_event_doorbell":       "Campanello (evento)",
		"bticino_event_doorbell_floor": "Campanello di piano (evento)",
		"bticino_event_call":           "Chiamata (evento)",
		"bticino_event_keys":           "Tasti (evento)",
		"bticino_event_bus_own":        "Evento bus OWN",
		"bticino_ip_address":           "Indirizzo IP",
		"bticino_mac_address":          "Indirizzo MAC",
		"bticino_wifi_signal":          "Segnale WiFi",
		"bticino_mark_all_read":        "Segna tutti come letti",
		"bticino_restart_bridge":       "Riavvia bridge",
		"bticino_start_ssh":            "Avvia SSH",
		"bticino_reboot_device":        "Riavvia dispositivo",
		"bticino_doorbell_camera":      "Telecamera campanello",
		"bticino_access":               "Accesso portone",
		"bticino_last_access":          "Ultimo accesso",
		nameKeyGPIO:                    "GPIO %d",
		nameKeyAdditionalLock:          "Serratura aggiuntiva %d",
	},
}

// normalizeLanguage maps "", "EN", "en-US", "it_IT" to a catalog key; an
// unknown language yields defaultLanguage.
func normalizeLanguage(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(l, "-_"); i > 0 {
		l = l[:i]
	}
	if _, ok := entityNames[l]; ok {
		return l
	}
	return defaultLanguage
}

// lookupName returns the catalog name for key in lang, falling back to the
// default language, then to def (the name hard-coded at the call site).
func lookupName(lang, key, def string) string {
	if n, ok := entityNames[normalizeLanguage(lang)][key]; ok {
		return n
	}
	if n, ok := entityNames[defaultLanguage][key]; ok {
		return n
	}
	return def
}

// entityName is the single place that decides the discovery "name" of an
// entity. Today it only applies the language catalog. This is the hook where
// an explicit per-entity override (e.g. mqtt.entities) must be consulted
// first, so that it wins over the language.
func (b *MQTTBridge) entityName(objectID, def string) string {
	return lookupName(b.config.MQTT.Language, objectID, def)
}

// entityNamef resolves a templated name (GPIO %d, ...) from the catalog.
func (b *MQTTBridge) entityNamef(key, def string, args ...interface{}) string {
	return fmt.Sprintf(lookupName(b.config.MQTT.Language, key, def), args...)
}
