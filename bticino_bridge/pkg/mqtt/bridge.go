package mqtt

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/sirupsen/logrus"

	"bticino_bridge/pkg/bticino"
	"bticino_bridge/pkg/bticino_commands"
	"bticino_bridge/pkg/config"
	"bticino_bridge/pkg/deviceconfig"
	"bticino_bridge/pkg/events"
	"bticino_bridge/pkg/messageparser"
	"bticino_bridge/pkg/openwebnet"
	"bticino_bridge/pkg/version"
)

// ==================== MQTT BRIDGE ====================

// MQTTBridge manages MQTT communication with Home Assistant
type MQTTBridge struct {
	config         *config.Config
	logger         *logrus.Logger
	client         pahomqtt.Client
	eventBus       events.EventBus
	openwebnet     *openwebnet.Client
	messageParser  *messageparser.MessageParser
	commandHandler *bticino_commands.BTicinoCommandHandler
	isConnected    bool
	mu             sync.RWMutex

	// State tracking for HA entities
	voicemailEnabled   bool
	doorbellSoundOn    bool
	muteOn             bool
	displayOn          bool
	sipRegistered      bool
	lastDoorbellTime   time.Time
	lastButtonPress    time.Time
	lastButtonKey      string
	eventsPublished    int64
	commandsReceived   int64
	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectAttempts  int32
	connectionEvents   []ConnectionEvent // Ring buffer de últimos eventos
	cePos              int
	ceFull             bool
}

// ConnectionEvent representa un evento de conexión MQTT
type ConnectionEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "connect", "disconnect", "reconnect"
	Success   bool      `json:"success"`
	Message   string    `json:"message,omitempty"`
}

// NewMQTTBridge creates a new MQTT bridge instance
func NewMQTTBridge(cfg *config.Config, logger *logrus.Logger) (*MQTTBridge, error) {
	return &MQTTBridge{
		config:           cfg,
		logger:           logger,
		voicemailEnabled: true,  // Default: enabled
		doorbellSoundOn:  true,  // Default: enabled
		displayOn:        false, // Default: off (screensaver)
	}, nil
}

// dataPrefix returns the data topic prefix (for state/command topics).
// This is separate from the HA discovery prefix ("homeassistant").
func (b *MQTTBridge) dataPrefix() string {
	p := b.config.MQTT.TopicPrefix
	// Avoid using "homeassistant" as data prefix — it collides with HA discovery
	if p == "" || p == "homeassistant" || p == "homeassistant/sensor/bticino" {
		return "bticino"
	}
	return p
}

// Start initializes and starts the MQTT bridge
func (b *MQTTBridge) Start(ctx context.Context) error {
	b.logger.Info("Starting MQTT Bridge for Home Assistant...")

	opts := pahomqtt.NewClientOptions()
	brokerURL := fmt.Sprintf("tcp://%s:%d", b.config.MQTT.Host, b.config.MQTT.Port)
	opts.AddBroker(brokerURL)
	opts.SetClientID(fmt.Sprintf("bticino_bridge_%d", time.Now().Unix()))
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetKeepAlive(60 * time.Second)

	// LWT — mark device offline when disconnected
	opts.SetWill(b.dataPrefix()+"/bridge/state", "offline", 1, true)

	if b.config.MQTT.Username != "" {
		opts.SetUsername(b.config.MQTT.Username)
	}
	if b.config.MQTT.Password != "" {
		opts.SetPassword(b.config.MQTT.Password)
	}

	opts.SetConnectionLostHandler(func(client pahomqtt.Client, err error) {
		b.logger.WithError(err).Error("MQTT connection lost")
		b.mu.Lock()
		b.isConnected = false
		b.lastDisconnectTime = time.Now()
		b.addConnectionEvent("disconnect", false, err.Error())
		b.mu.Unlock()
	})

	opts.SetOnConnectHandler(func(client pahomqtt.Client) {
		b.mu.Lock()
		// Check if this is a reconnect (was previously connected)
		isReconnect := b.connectTime.IsZero() == false && b.lastDisconnectTime.IsZero() == false
		b.isConnected = true
		b.connectTime = time.Now()

		eventType := "connect"
		if isReconnect {
			b.reconnectAttempts++
			eventType = "reconnect"
			b.logger.Infof("MQTT reconnected to Home Assistant (attempt %d)", b.reconnectAttempts)
		} else {
			b.logger.Info("MQTT Connected to Home Assistant")
		}

		b.addConnectionEvent(eventType, true, "")
		b.mu.Unlock()

		// Publish availability
		b.publish(b.dataPrefix()+"/bridge/state", "online", true)

		// Publish HA discovery entities, subscribe to commands, publish initial states
		b.publishHomeAssistantDiscovery()
		b.subscribeToCommands()
		b.publishAllStates()
	})

	mqttClient := pahomqtt.NewClient(opts)
	b.client = mqttClient // Assign BEFORE Connect() to avoid nil pointer in OnConnectHandler callback
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	b.logger.Infof("MQTT Bridge connected to %s", brokerURL)
	return nil
}

// IsConnected returns the MQTT connection state
func (b *MQTTBridge) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.isConnected
}

// addConnectionEvent adds a connection event to the ring buffer
func (b *MQTTBridge) addConnectionEvent(eventType string, success bool, message string) {
	if b.connectionEvents == nil {
		b.connectionEvents = make([]ConnectionEvent, 20) // Max 20 eventos
	}
	b.connectionEvents[b.cePos] = ConnectionEvent{
		Timestamp: time.Now(),
		Type:      eventType,
		Success:   success,
		Message:   message,
	}
	b.cePos = (b.cePos + 1) % 20
	if b.cePos == 0 {
		b.ceFull = true
	}
}

// GetConnectionEvents returns recent connection events
func (b *MQTTBridge) GetConnectionEvents() []ConnectionEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.connectionEvents == nil {
		return nil
	}

	result := make([]ConnectionEvent, 0, 20)
	if !b.ceFull {
		for i := 0; i < b.cePos; i++ {
			if b.connectionEvents[i].Timestamp.IsZero() == false {
				result = append(result, b.connectionEvents[i])
			}
		}
	} else {
		// Return in chronological order
		for i := b.cePos; i < 20; i++ {
			if b.connectionEvents[i].Timestamp.IsZero() == false {
				result = append(result, b.connectionEvents[i])
			}
		}
		for i := 0; i < b.cePos; i++ {
			if b.connectionEvents[i].Timestamp.IsZero() == false {
				result = append(result, b.connectionEvents[i])
			}
		}
	}
	return result
}

// SetEventBus sets the event bus and configures subscriptions
func (b *MQTTBridge) SetEventBus(eventBus events.EventBus) {
	b.eventBus = eventBus
	b.setupEventSubscriptions()
}

// SetOpenWebNet sets the OpenWebNet client for command execution
func (b *MQTTBridge) SetOpenWebNet(client *openwebnet.Client) {
	b.openwebnet = client
}

// SetMessageParser sets the message parser for answering machine stats
func (b *MQTTBridge) SetMessageParser(mp *messageparser.MessageParser) {
	b.messageParser = mp
}

// SetCommandHandler sets the enhanced command handler for full voicemail protocol
func (b *MQTTBridge) SetCommandHandler(ch *bticino_commands.BTicinoCommandHandler) {
	b.commandHandler = ch
}

// SetDeviceConfigPublisher inicializa el publicador de configuración del dispositivo vía MQTT
func (b *MQTTBridge) SetDeviceConfigPublisher(interval time.Duration) {
	if !b.IsConnected() {
		b.logger.Warn("Cannot start device config publisher: MQTT not connected")
		return
	}

	publisher := deviceconfig.NewDeviceMQTTPublisher(b.logger, b, b.dataPrefix())
	publisher.Start(interval)
	b.logger.Infof("Device config MQTT publisher started with interval: %v", interval)

	// Also start the file watcher for real-time sync
	watcher := deviceconfig.NewFileWatcher(b.logger, b, b.dataPrefix())
	watcher.Start()
	b.logger.Info("Device config file watcher started for real-time sync")
}

// SetFileWatcher optionally start file watcher separately
func (b *MQTTBridge) SetFileWatcher() {
	if !b.IsConnected() {
		b.logger.Warn("Cannot start file watcher: MQTT not connected")
		return
	}

	watcher := deviceconfig.NewFileWatcher(b.logger, b, b.dataPrefix())
	watcher.Start()
	b.logger.Info("Device config file watcher started for real-time sync")
}

// ==================== HOME ASSISTANT DISCOVERY ====================

// publishHomeAssistantDiscovery sends MQTT Discovery config for all entities.
// Discovery topics use the HA standard prefix "homeassistant/{component}/{objectID}/config".
// State/command topics use the data prefix "bticino/..." to avoid collisions.
func (b *MQTTBridge) publishHomeAssistantDiscovery() {
	dp := b.dataPrefix()

	device := map[string]interface{}{
		"identifiers":  []string{"bticino_classe300x"},
		"name":         "BTicino Classe 300X",
		"model":        bticino.DeviceModel,
		"manufacturer": bticino.DeviceManufacturer,
		"sw_version":   "Bridge v" + version.GetVersion(),
	}

	avail := map[string]interface{}{
		"topic":                 dp + "/bridge/state",
		"payload_available":     "online",
		"payload_not_available": "offline",
	}

	entityCount := 0

	// --- 1. Lock: Puerta ---
	b.discovery("lock", "bticino_door", map[string]interface{}{
		"name":           "Puerta",
		"unique_id":      "bticino_c300x_door_lock",
		"object_id":      "bticino_puerta",
		"command_topic":  dp + "/lock/set",
		"state_topic":    dp + "/lock/state",
		"payload_lock":   "LOCK",
		"payload_unlock": "UNLOCK",
		"state_locked":   "LOCKED",
		"state_unlocked": "UNLOCKED",
		"optimistic":     false,
		"icon":           "mdi:door",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- 2. Switch: Contestador ---
	b.discovery("switch", "bticino_voicemail", map[string]interface{}{
		"name":          "Contestador",
		"unique_id":     "bticino_c300x_voicemail",
		"object_id":     "bticino_contestador",
		"command_topic": dp + "/voicemail/set",
		"state_topic":   dp + "/voicemail/state",
		"payload_on":    "ON",
		"payload_off":   "OFF",
		"state_on":      "ON",
		"state_off":     "OFF",
		"icon":          "mdi:voicemail",
		"device":        device,
		"availability":  avail,
	})
	entityCount++

	// --- 3. Switch: Sonido Timbre ---
	b.discovery("switch", "bticino_doorbell_sound", map[string]interface{}{
		"name":          "Sonido Timbre",
		"unique_id":     "bticino_c300x_doorbell_sound",
		"object_id":     "bticino_sonido_timbre",
		"command_topic": dp + "/doorbellsound/set",
		"state_topic":   dp + "/doorbellsound/state",
		"payload_on":    "ON",
		"payload_off":   "OFF",
		"state_on":      "ON",
		"state_off":     "OFF",
		"icon":          "mdi:bell",
		"device":        device,
		"availability":  avail,
	})
	entityCount++

	// --- 4. Switch: Pantalla ---
	b.discovery("switch", "bticino_display", map[string]interface{}{
		"name":          "Pantalla",
		"unique_id":     "bticino_c300x_display",
		"object_id":     "bticino_pantalla",
		"command_topic": dp + "/display/set",
		"state_topic":   dp + "/display/state",
		"payload_on":    "ON",
		"payload_off":   "OFF",
		"state_on":      "ON",
		"state_off":     "OFF",
		"icon":          "mdi:tablet",
		"device":        device,
		"availability":  avail,
	})
	entityCount++

	// --- 5. Switch: Silencio (ELIMINADO - integrado en doorbell_sound) ---
	// El control de silencio del altavoz ya está integrado en doorbell_sound
	// cuando se graba una nota de voz, el GPIO 180 se desactiva automáticamente
	// entityCount++

	// --- 6. Button: Luz Escalera ---
	b.discovery("button", "bticino_staircase_light", map[string]interface{}{
		"name":          "Luz Escalera",
		"unique_id":     "bticino_c300x_staircase_light",
		"object_id":     "bticino_luz_escalera",
		"command_topic": dp + "/light/set",
		"payload_press": "PRESS",
		"icon":          "mdi:stairs",
		"device":        device,
		"availability":  avail,
	})
	entityCount++

	// --- 7. Button: Abrir Puerta ---
	b.discovery("button", "bticino_door_open", map[string]interface{}{
		"name":          "Abrir Puerta",
		"unique_id":     "bticino_c300x_door_open",
		"object_id":     "bticino_abrir_puerta",
		"command_topic": dp + "/door/open",
		"payload_press": "PRESS",
		"icon":          "mdi:door-open",
		"device":        device,
		"availability":  avail,
	})
	entityCount++

	// --- 8. Sensor: Temperatura ---
	b.discovery("sensor", "bticino_temperature", map[string]interface{}{
		"name":                "Temperatura Dispositivo",
		"unique_id":           "bticino_c300x_temperature",
		"object_id":           "bticino_temperatura",
		"state_topic":         dp + "/sensor/temperature",
		"unit_of_measurement": "\u00b0C",
		"device_class":        "temperature",
		"state_class":         "measurement",
		"icon":                "mdi:thermometer",
		"device":              device,
		"availability":        avail,
	})
	entityCount++

	// --- 9. Sensor: Mensajes Nuevos ---
	b.discovery("sensor", "bticino_new_messages", map[string]interface{}{
		"name":         "Mensajes Nuevos",
		"unique_id":    "bticino_c300x_new_messages",
		"object_id":    "bticino_mensajes_nuevos",
		"state_topic":  dp + "/sensor/new_messages",
		"icon":         "mdi:message-badge",
		"device":       device,
		"availability": avail,
	})
	entityCount++

	// --- 10. Sensor: Total Mensajes ---
	b.discovery("sensor", "bticino_total_messages", map[string]interface{}{
		"name":         "Total Mensajes",
		"unique_id":    "bticino_c300x_total_messages",
		"object_id":    "bticino_total_mensajes",
		"state_topic":  dp + "/sensor/total_messages",
		"icon":         "mdi:message-text-clock",
		"device":       device,
		"availability": avail,
	})
	entityCount++

	// --- 11. Sensor: Almacenamiento Usado ---
	b.discovery("sensor", "bticino_storage", map[string]interface{}{
		"name":                "Almacenamiento Contestador",
		"unique_id":           "bticino_c300x_storage",
		"object_id":           "bticino_almacenamiento",
		"state_topic":         dp + "/sensor/storage_used",
		"unit_of_measurement": "%",
		"icon":                "mdi:harddisk",
		"device":              device,
		"availability":        avail,
	})
	entityCount++

	// --- 12. Sensor: Teclado (ultimo boton pulsado) ---
	b.discovery("sensor", "bticino_keypad", map[string]interface{}{
		"name":         "Teclado",
		"unique_id":    "bticino_c300x_keypad",
		"object_id":    "bticino_teclado",
		"state_topic":  dp + "/sensor/keypad",
		"icon":         "mdi:dialpad",
		"device":       device,
		"availability": avail,
	})
	entityCount++

	// --- 13. Binary Sensor: Timbre (doorbell) ---
	b.discovery("binary_sensor", "bticino_doorbell", map[string]interface{}{
		"name":         "Timbre",
		"unique_id":    "bticino_c300x_doorbell",
		"object_id":    "bticino_timbre",
		"state_topic":  dp + "/doorbell/state",
		"payload_on":   "ON",
		"payload_off":  "OFF",
		"device_class": "occupancy",
		"off_delay":    5,
		"icon":         "mdi:doorbell",
		"device":       device,
		"availability": avail,
	})
	entityCount++

	// --- LEDs as binary sensors (14-20) ---
	ledEntities := []struct {
		objectID string
		name     string
		sysfs    string
		icon     string
	}{
		{"bticino_led_memo", "LED Mensaje", "led_memo", "mdi:message-alert"},
		{"bticino_led_wifi", "LED WiFi", "led_gwifi", "mdi:wifi"},
		{"bticino_led_answering", "LED Contestador", "led_ans_machine", "mdi:voicemail"},
		{"bticino_led_lock", "LED Cerradura", "led_lock", "mdi:lock"},
		{"bticino_led_missed_call", "LED Llamada Perdida", "led_exc_call", "mdi:phone-missed"},
		{"bticino_led_call_green", "LED Llamada Verde", "led_vct_green", "mdi:phone-in-talk"},
		{"bticino_led_call_red", "LED Llamada Rojo", "led_vct_red", "mdi:phone-hangup"},
	}

	for _, led := range ledEntities {
		b.discovery("binary_sensor", led.objectID, map[string]interface{}{
			"name":         led.name,
			"unique_id":    led.objectID,
			"object_id":    led.objectID,
			"state_topic":  dp + "/led/" + led.sysfs + "/state",
			"payload_on":   "ON",
			"payload_off":  "OFF",
			"icon":         led.icon,
			"device":       device,
			"availability": avail,
		})
		entityCount++
	}

	// --- 21. Sensor: Evento Bus OpenWebNet (ultimo evento raw) ---
	b.discovery("sensor", "bticino_bus_event", map[string]interface{}{
		"name":         "Evento Bus OWN",
		"unique_id":    "bticino_c300x_bus_event",
		"object_id":    "bticino_evento_bus",
		"state_topic":  dp + "/sensor/bus_event",
		"icon":         "mdi:bus-electric",
		"device":       device,
		"availability": avail,
	})
	entityCount++

	// --- 22. Binary Sensor: Estado SIP ---
	b.discovery("binary_sensor", "bticino_sip_status", map[string]interface{}{
		"name":         "Estado SIP",
		"unique_id":    "bticino_c300x_sip_status",
		"object_id":    "bticino_estado_sip",
		"state_topic":  dp + "/sensor/sip_status",
		"payload_on":   "ON",
		"payload_off":  "OFF",
		"device_class": "connectivity",
		"icon":         "mdi:phone-voip",
		"device":       device,
		"availability": avail,
	})
	entityCount++

	// --- 23. Sensor: Diagnostico Sistema ---
	b.discovery("sensor", "bticino_system_diag", map[string]interface{}{
		"name":         "Diagnostico Sistema",
		"unique_id":    "bticino_c300x_system_diag",
		"object_id":    "bticino_diagnostico",
		"state_topic":  dp + "/sensor/system_diag",
		"icon":         "mdi:heart-pulse",
		"device":       device,
		"availability": avail,
	})
	entityCount++

	// --- 24. Sensor: Multicast ---
	b.discovery("sensor", "bticino_multicast", map[string]interface{}{
		"name":         "Mensajes Multicast",
		"unique_id":    "bticino_c300x_multicast",
		"object_id":    "bticino_multicast",
		"state_topic":  dp + "/sensor/multicast_count",
		"icon":         "mdi:access-point-network",
		"device":       device,
		"availability": avail,
	})
	entityCount++

	// --- GPIO individual sensors (25-37) ---
	gpioPins := []int{12, 13, 47, 49, 52, 54, 56, 58, 60, 154, 155, 176, 180}
	for _, pin := range gpioPins {
		objID := fmt.Sprintf("bticino_gpio_%d", pin)
		b.discovery("binary_sensor", objID, map[string]interface{}{
			"name":         fmt.Sprintf("GPIO %d", pin),
			"unique_id":    fmt.Sprintf("bticino_c300x_gpio_%d", pin),
			"object_id":    objID,
			"state_topic":  fmt.Sprintf("%s/gpio/%d/state", dp, pin),
			"payload_on":   "ON",
			"payload_off":  "OFF",
			"icon":         "mdi:chip",
			"device":       device,
			"availability": avail,
		})
		entityCount++
	}

	// --- 38. Sensor: Registro de Actividad ---
	b.discovery("sensor", "bticino_activity_log", map[string]interface{}{
		"name":                  "Registro de Actividad",
		"unique_id":             "bticino_c300x_activity_log",
		"object_id":             "bticino_actividad",
		"state_topic":           dp + "/sensor/activity_log",
		"json_attributes_topic": dp + "/sensor/activity_log/attributes",
		"icon":                  "mdi:timeline-text",
		"device":                device,
		"availability":          avail,
	})
	entityCount++

	// --- 39. Sensor: Informacion del Sistema (WHO=13) ---
	b.discovery("sensor", "bticino_system_info", map[string]interface{}{
		"name":                  "Informacion Sistema",
		"unique_id":             "bticino_c300x_system_info",
		"object_id":             "bticino_info_sistema",
		"state_topic":           dp + "/sensor/system_info",
		"json_attributes_topic": dp + "/sensor/system_info/attributes",
		"icon":                  "mdi:information-outline",
		"device":                device,
		"availability":          avail,
	})
	entityCount++

	// --- Cerraduras adicionales (dinamico, segun configuracion) ---
	for i, lock := range b.config.AdditionalLocks {
		whereStr := fmt.Sprintf("%d", lock.Where)
		objID := fmt.Sprintf("bticino_lock_%d", lock.Where)
		name := lock.Name
		if name == "" {
			name = fmt.Sprintf("Cerradura %d", i+1)
		}
		b.discovery("lock", objID, map[string]interface{}{
			"name":           name,
			"unique_id":      fmt.Sprintf("bticino_c300x_lock_%d", lock.Where),
			"object_id":      objID,
			"command_topic":  dp + "/lock/" + whereStr + "/set",
			"state_topic":    dp + "/lock/" + whereStr + "/state",
			"payload_lock":   "LOCK",
			"payload_unlock": "UNLOCK",
			"state_locked":   "LOCKED",
			"state_unlocked": "UNLOCKED",
			"optimistic":     false,
			"icon":           "mdi:door",
			"device":         device,
			"availability":   avail,
		})
		entityCount++
	}

	b.logger.Infof("MQTT: Published %d Home Assistant discovery entities", entityCount)

	// ==================== DEVICE CONFIG ENTITIES (Fase 4) ====================

	// --- Sensor: Language ---
	b.discovery("sensor", "bticino_language", map[string]interface{}{
		"name":           "Idioma Sistema",
		"unique_id":      "bticino_c300x_language",
		"object_id":      "bticino_idioma",
		"state_topic":    dp + "/system/language",
		"value_template": "{{ value_json.language }}",
		"icon":           "mdi:translate",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: Timezone ---
	b.discovery("sensor", "bticino_timezone", map[string]interface{}{
		"name":           "Zona Horaria",
		"unique_id":      "bticino_c300x_timezone",
		"object_id":      "bticino_timezone",
		"state_topic":    dp + "/system/timezone",
		"value_template": "{{ value_json.timezone }}",
		"icon":           "mdi:clock-outline",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: NTP Server ---
	b.discovery("sensor", "bticino_ntp", map[string]interface{}{
		"name":           "Servidor NTP",
		"unique_id":      "bticino_c300x_ntp",
		"object_id":      "bticino_ntp",
		"state_topic":    dp + "/system/timezone",
		"value_template": "{{ value_json.ntp_server }}",
		"icon":           "mdi:server-network",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: Datetime ---
	b.discovery("sensor", "bticino_datetime", map[string]interface{}{
		"name":           "Fecha y Hora Sistema",
		"unique_id":      "bticino_c300x_datetime",
		"object_id":      "bticino_datetime",
		"state_topic":    dp + "/system/datetime",
		"value_template": "{{ value_json.datetime }}",
		"icon":           "mdi:calendar-clock",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: Answering Machine Status ---
	b.discovery("sensor", "bticino_answering_status", map[string]interface{}{
		"name":           "Estado Contestador",
		"unique_id":      "bticino_c300x_answering_status",
		"object_id":      "bticino_answering",
		"state_topic":    dp + "/answering/state",
		"value_template": "{{ value_json.activated }}",
		"icon":           "mdi:voicemail",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: Answering Memory ---
	b.discovery("sensor", "bticino_answering_memory", map[string]interface{}{
		"name":           "Memoria Contestador",
		"unique_id":      "bticino_c300x_answering_memory",
		"object_id":      "bticino_answering_memory",
		"state_topic":    dp + "/answering/state",
		"value_template": "{{ value_json.memory_used }}",
		"icon":           "mdi:memory",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: Ringtone S0 (Placa exterior) ---
	b.discovery("sensor", "bticino_ringtone_s0", map[string]interface{}{
		"name":           "Timbre Placa Exterior",
		"unique_id":      "bticino_c300x_ringtone_s0",
		"object_id":      "bticino_ringtone_s0",
		"state_topic":    dp + "/audio/ringtone/s0",
		"value_template": "{{ value_json.value }}",
		"icon":           "mdi:bell",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: Volume S0 ---
	b.discovery("sensor", "bticino_volume_s0", map[string]interface{}{
		"name":           "Volumen S0",
		"unique_id":      "bticino_c300x_volume_s0",
		"object_id":      "bticino_volume_s0",
		"state_topic":    dp + "/audio/volume/s0",
		"value_template": "{{ value_json.value }}",
		"icon":           "mdi:volume-high",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: Volume Door ---
	b.discovery("sensor", "bticino_volume_door", map[string]interface{}{
		"name":           "Volumen Puerta",
		"unique_id":      "bticino_c300x_volume_door",
		"object_id":      "bticino_volume_door",
		"state_topic":    dp + "/audio/volume/door",
		"value_template": "{{ value_json.value }}",
		"icon":           "mdi:doorbell",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: Display Brightness ---
	b.discovery("sensor", "bticino_display_brightness", map[string]interface{}{
		"name":           "Brillo Display",
		"unique_id":      "bticino_c300x_display_brightness",
		"object_id":      "bticino_display_brightness",
		"state_topic":    dp + "/display/brightness",
		"value_template": "{{ value_json.value }}",
		"icon":           "mdi:brightness-6",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	// --- Sensor: Camera 20 Brightness ---
	b.discovery("sensor", "bticino_camera_20_brightness", map[string]interface{}{
		"name":           "Brillo Camara 20",
		"unique_id":      "bticino_c300x_camera_20_brightness",
		"object_id":      "bticino_camera_20",
		"state_topic":    dp + "/camera/20/config",
		"value_template": "{{ value_json.brightness }}",
		"icon":           "mdi:cctv",
		"device":         device,
		"availability":   avail,
	})
	entityCount++

	b.logger.Infof("MQTT: Published %d Home Assistant discovery entities (total with device config)", entityCount)
}

// discovery publishes one HA MQTT discovery config message
func (b *MQTTBridge) discovery(component, objectID string, cfg map[string]interface{}) {
	topic := fmt.Sprintf("homeassistant/%s/%s/config", component, objectID)
	jsonData, err := json.Marshal(cfg)
	if err != nil {
		b.logger.WithError(err).WithField("object_id", objectID).Error("Failed to marshal discovery config")
		return
	}
	b.publish(topic, string(jsonData), true)
}

// ==================== COMMAND SUBSCRIPTIONS ====================

func (b *MQTTBridge) subscribeToCommands() {
	dp := b.dataPrefix()
	subs := 0

	sub := func(topic string, handler pahomqtt.MessageHandler) {
		if token := b.client.Subscribe(topic, 1, handler); token.Wait() && token.Error() != nil {
			b.logger.WithError(token.Error()).WithField("topic", topic).Error("Failed to subscribe")
		} else {
			subs++
		}
	}

	sub(dp+"/lock/set", b.handleDoorCommand)
	sub(dp+"/door/open", b.handleDoorOpenButton)
	sub(dp+"/voicemail/set", b.handleVoicemailCommand)
	sub(dp+"/doorbellsound/set", b.handleDoorbellSoundCommand)
	sub(dp+"/display/set", b.handleDisplayCommand)
	sub(dp+"/mute/set", b.handleMuteCommand)
	sub(dp+"/light/set", b.handleLightCommand)
	sub(dp+"/command/send", b.handleRawCommand)

	// Suscribir a comandos de cerraduras adicionales
	for _, lock := range b.config.AdditionalLocks {
		whereStr := fmt.Sprintf("%d", lock.Where)
		lockWhere := lock.Where // capture for closure
		sub(dp+"/lock/"+whereStr+"/set", func(client pahomqtt.Client, msg pahomqtt.Message) {
			b.handleAdditionalLockCommand(lockWhere, string(msg.Payload()))
		})
	}

	b.logger.Infof("MQTT: Subscribed to %d command topics", subs)
}

// --- Command Handlers ---

func (b *MQTTBridge) handleDoorCommand(client pahomqtt.Client, msg pahomqtt.Message) {
	payload := string(msg.Payload())
	b.logger.WithField("payload", payload).Info("MQTT: Door lock command")
	b.incCommands()

	if payload == "UNLOCK" || payload == "OPEN" {
		b.executeDoorUnlock()
	}
}

func (b *MQTTBridge) handleDoorOpenButton(client pahomqtt.Client, msg pahomqtt.Message) {
	b.logger.Info("MQTT: Door open button pressed")
	b.incCommands()
	b.executeDoorUnlock()
}

func (b *MQTTBridge) executeDoorUnlock() {
	if _, err := b.executeOWN(bticino.CmdDoorOpenPress); err != nil {
		b.logger.WithError(err).Error("Failed door unlock press")
		return
	}
	dp := b.dataPrefix()
	b.publish(dp+"/lock/state", "UNLOCKED", true)

	go func() {
		time.Sleep(1 * time.Second)
		if _, err := b.executeOWN(bticino.CmdDoorOpenRelease); err != nil {
			b.logger.WithError(err).Error("Failed door unlock release")
		}
		// Reset to locked after 5s
		time.Sleep(5 * time.Second)
		b.publish(dp+"/lock/state", "LOCKED", true)
	}()
}

func (b *MQTTBridge) handleVoicemailCommand(client pahomqtt.Client, msg pahomqtt.Message) {
	payload := string(msg.Payload())
	b.logger.WithField("payload", payload).Info("MQTT: Voicemail command")
	b.incCommands()

	on := payload == "ON" || payload == "1"

	// Try 4-step protocol via commandHandler (full app-like sequence)
	if b.commandHandler != nil {
		var err error
		if on {
			b.logger.Info("MQTT: Enabling voicemail via 4-step protocol")
			err = b.commandHandler.EnableAnsweringMachine()
		} else {
			b.logger.Info("MQTT: Disabling voicemail via 4-step protocol")
			err = b.commandHandler.DisableAnsweringMachine()
		}
		if err != nil {
			b.logger.WithError(err).Warn("4-step voicemail protocol failed, falling back to simple command")
		} else {
			b.mu.Lock()
			b.voicemailEnabled = on
			b.mu.Unlock()
			b.publish(b.dataPrefix()+"/voicemail/state", boolState(on), true)
			return
		}
	}

	// Fallback: simple 2-command via netcat
	cmd := bticino.CmdVoicemailOff
	if on {
		cmd = bticino.CmdVoicemailOn
	}

	if _, err := b.executeOWN(cmd); err != nil {
		b.logger.WithError(err).Error("Failed voicemail toggle")
		return
	}

	b.mu.Lock()
	b.voicemailEnabled = on
	b.mu.Unlock()
	b.publish(b.dataPrefix()+"/voicemail/state", boolState(on), true)
}

func (b *MQTTBridge) handleDoorbellSoundCommand(client pahomqtt.Client, msg pahomqtt.Message) {
	payload := string(msg.Payload())
	b.logger.WithField("payload", payload).Info("MQTT: Doorbell sound command")
	b.incCommands()

	on := payload == "ON" || payload == "1"
	cmd := bticino.CmdBellOff
	if on {
		cmd = bticino.CmdBellOn
	}

	if _, err := b.executeOWN(cmd); err != nil {
		b.logger.WithError(err).Error("Failed doorbell sound toggle")
		return
	}

	b.mu.Lock()
	b.doorbellSoundOn = on
	b.mu.Unlock()
	b.publish(b.dataPrefix()+"/doorbellsound/state", boolState(on), true)
}

func (b *MQTTBridge) handleDisplayCommand(client pahomqtt.Client, msg pahomqtt.Message) {
	payload := string(msg.Payload())
	b.logger.WithField("payload", payload).Info("MQTT: Display command")
	b.incCommands()

	on := payload == "ON" || payload == "1"
	cmd := bticino.CmdDisplayOff
	if on {
		cmd = bticino.CmdDisplayOn
	}

	if _, err := b.executeOWN(cmd); err != nil {
		b.logger.WithError(err).Error("Failed display toggle")
		return
	}

	b.mu.Lock()
	b.displayOn = on
	b.mu.Unlock()
	b.publish(b.dataPrefix()+"/display/state", boolState(on), true)
}

func (b *MQTTBridge) handleMuteCommand(client pahomqtt.Client, msg pahomqtt.Message) {
	payload := string(msg.Payload())
	b.logger.WithField("payload", payload).Info("MQTT: Mute command")
	b.incCommands()

	on := payload == "ON" || payload == "1"
	cmd := bticino.CmdMuteOff
	if on {
		cmd = bticino.CmdMuteOn
	}

	if _, err := b.executeOWN(cmd); err != nil {
		b.logger.WithError(err).Error("Failed mute toggle")
		return
	}

	b.mu.Lock()
	b.muteOn = on
	b.mu.Unlock()
	b.publish(b.dataPrefix()+"/mute/state", boolState(on), true)
}

func (b *MQTTBridge) handleLightCommand(client pahomqtt.Client, msg pahomqtt.Message) {
	b.logger.Info("MQTT: Staircase light command")
	b.incCommands()

	if _, err := b.executeOWN(bticino.CmdLightOnPress); err != nil {
		b.logger.WithError(err).Error("Failed staircase light press")
		return
	}
	go func() {
		time.Sleep(1 * time.Second)
		if _, err := b.executeOWN(bticino.CmdLightOnRelease); err != nil {
			b.logger.WithError(err).Error("Failed staircase light release")
		}
	}()
}

func (b *MQTTBridge) handleRawCommand(client pahomqtt.Client, msg pahomqtt.Message) {
	cmd := string(msg.Payload())
	b.logger.WithField("command", cmd).Info("MQTT: Raw OWN command")
	b.incCommands()

	if _, err := b.executeOWN(cmd); err != nil {
		b.logger.WithError(err).Error("Failed raw command")
	}
}

// handleAdditionalLockCommand maneja el comando de desbloqueo de una cerradura adicional.
// Usa el mismo patron press+release que la cerradura principal (WHERE=20).
func (b *MQTTBridge) handleAdditionalLockCommand(where int, payload string) {
	b.logger.WithFields(logrus.Fields{
		"where":   where,
		"payload": payload,
	}).Info("MQTT: Additional lock command")
	b.incCommands()

	if payload != "UNLOCK" && payload != "OPEN" {
		return
	}

	pressCmd := bticino.BuildDoorOpenPress(where)
	releaseCmd := bticino.BuildDoorOpenRelease(where)
	whereStr := fmt.Sprintf("%d", where)
	dp := b.dataPrefix()

	if _, err := b.executeOWN(pressCmd); err != nil {
		b.logger.WithError(err).WithField("where", where).Error("Failed additional lock press")
		return
	}
	b.publish(dp+"/lock/"+whereStr+"/state", "UNLOCKED", true)

	go func() {
		time.Sleep(1 * time.Second)
		if _, err := b.executeOWN(releaseCmd); err != nil {
			b.logger.WithError(err).WithField("where", where).Error("Failed additional lock release")
		}
		// Re-lock after 5s
		time.Sleep(5 * time.Second)
		b.publish(dp+"/lock/"+whereStr+"/state", "LOCKED", true)
	}()
}

// ==================== EVENT BUS SUBSCRIPTIONS ====================

func (b *MQTTBridge) setupEventSubscriptions() {
	if b.eventBus == nil {
		return
	}
	dp := b.dataPrefix()

	// Door unlock events
	b.eventBus.Subscribe("door.unlocked", func(event *events.Event) {
		b.publish(dp+"/lock/state", "UNLOCKED", true)
		b.incEvents()
		go func() {
			time.Sleep(5 * time.Second)
			b.publish(dp+"/lock/state", "LOCKED", true)
		}()
	})

	// Doorbell events (from port 20000 bus monitoring)
	b.eventBus.Subscribe("doorbell.pressed", func(event *events.Event) {
		b.mu.Lock()
		b.lastDoorbellTime = time.Now()
		b.mu.Unlock()
		b.publish(dp+"/doorbell/state", "ON", false)
		b.incEvents()
	})

	// Physical button presses (from /dev/input/event0)
	b.eventBus.Subscribe("button.pressed", func(event *events.Event) {
		if data, ok := event.Data.(map[string]interface{}); ok {
			keyName, _ := data["key_name"].(string)
			b.mu.Lock()
			b.lastButtonPress = time.Now()
			b.lastButtonKey = keyName
			b.mu.Unlock()
			b.publish(dp+"/sensor/keypad", keyName, false)
		}
		b.incEvents()
	})

	// OpenWebNet bus events (any event from port 20000)
	b.eventBus.Subscribe("openwebnet.event", func(event *events.Event) {
		if data, ok := event.Data.(map[string]interface{}); ok {
			raw, _ := data["raw"].(string)
			if raw != "" {
				b.publish(dp+"/sensor/bus_event", raw, false)
			}
		}
		b.incEvents()
	})

	// Screen state changes
	b.eventBus.Subscribe("screen.changed", func(event *events.Event) {
		if data, ok := event.Data.(map[string]interface{}); ok {
			if state, ok := data["active"].(bool); ok {
				b.mu.Lock()
				b.displayOn = state
				b.mu.Unlock()
				b.publish(dp+"/display/state", boolState(state), true)
			}
		}
		b.incEvents()
	})

	// Floor/landing doorbell (timbre de rellano), distinct from the entrance panel
	b.eventBus.Subscribe("doorbell.floor.pressed", func(event *events.Event) {
		b.publish(dp+"/doorbell_floor/state", "ON", false)
		b.incEvents()
		go func() {
			time.Sleep(3 * time.Second)
			b.publish(dp+"/doorbell_floor/state", "OFF", false)
		}()
	})

	// Real incoming SIP call (someone is calling the unit)
	b.eventBus.Subscribe("sip.call.incoming", func(event *events.Event) {
		caller := ""
		if data, ok := event.Data.(map[string]interface{}); ok {
			caller, _ = data["caller"].(string)
		}
		b.publish(dp+"/call/state", "INCOMING", false)
		if caller != "" {
			b.publish(dp+"/call/caller", caller, false)
		}
		b.incEvents()
	})

	// SIP call ended
	b.eventBus.Subscribe("sip.call.ended", func(event *events.Event) {
		b.publish(dp+"/call/state", "IDLE", false)
		b.incEvents()
	})

	// SIP call connected
	b.eventBus.Subscribe("sip.call.connected", func(event *events.Event) {
		b.publish(dp+"/call/state", "CONNECTED", false)
		b.incEvents()
	})

	b.logger.Info("EventBus: Subscribed to 9 event types for MQTT publishing")

	// Multicast event subscriptions (if multicast listener is running)
	b.eventBus.Subscribe("multicast.message.raw", func(event *events.Event) {
		b.incEvents()
	})

	b.eventBus.Subscribe("multicast.message.OPEN", func(event *events.Event) {
		b.logger.Debug("Multicast OPEN message received via EventBus")
		b.incEvents()
	})

	b.logger.Info("EventBus: Subscribed to 7 event types (5 core + 2 multicast)")
}

// ==================== STATE PUBLISHING ====================

// publishAllStates publishes current state of all entities
func (b *MQTTBridge) publishAllStates() {
	dp := b.dataPrefix()

	// Lock default state
	b.publish(dp+"/lock/state", "LOCKED", true)

	// Switch states
	b.mu.RLock()
	voicemail := b.voicemailEnabled
	bell := b.doorbellSoundOn
	mute := b.muteOn
	display := b.displayOn
	sip := b.sipRegistered
	b.mu.RUnlock()

	b.publish(dp+"/voicemail/state", boolState(voicemail), true)
	b.publish(dp+"/doorbellsound/state", boolState(bell), true)
	b.publish(dp+"/mute/state", boolState(mute), true)
	b.publish(dp+"/display/state", boolState(display), true)

	// Doorbell default off
	b.publish(dp+"/doorbell/state", "OFF", true)

	// New entities: SIP status, system diagnostics, multicast
	b.publish(dp+"/sensor/sip_status", boolState(sip), true)
	b.publish(dp+"/sensor/system_diag", "iniciando", true)
	b.publish(dp+"/sensor/multicast_count", "0", true)

	// GPIO pins initial state (all OFF until first read)
	gpioPins := []int{12, 13, 47, 49, 52, 54, 56, 58, 60, 154, 155, 176, 180}
	for _, pin := range gpioPins {
		b.publish(fmt.Sprintf("%s/gpio/%d/state", dp, pin), "OFF", true)
	}

	// Publish message stats if parser available
	b.PublishMessageStats()

	// Additional locks initial state (all locked)
	for _, lock := range b.config.AdditionalLocks {
		whereStr := fmt.Sprintf("%d", lock.Where)
		b.publish(dp+"/lock/"+whereStr+"/state", "LOCKED", true)
	}
}

// ==================== DEVICE STATE QUERIES ====================

// QueryVoicemailStatus consulta el estado real del contestador via *#8**40##.
// Parsea la respuesta *#8**40*<vm_on>*<wm_on>*XXXX*X*XX## y actualiza el estado MQTT.
func (b *MQTTBridge) QueryVoicemailStatus() {
	response, err := b.executeOWNWithResponse(bticino.CmdVoicemailStatusQuery)
	if err != nil {
		b.logger.WithError(err).Warn("No se pudo consultar estado del contestador")
		return
	}

	// Regex para parsear: *#8**40*<vm_on>*<wm_on>*XXXX*X*XX##
	re := regexp.MustCompile(`\*#8\*\*40\*([01])\*([01])\*`)
	matches := re.FindStringSubmatch(response)
	if len(matches) >= 3 {
		vmEnabled := matches[1] == "1"
		wmEnabled := matches[2] == "1"

		b.mu.Lock()
		changed := b.voicemailEnabled != vmEnabled
		b.voicemailEnabled = vmEnabled
		b.mu.Unlock()

		b.publish(b.dataPrefix()+"/voicemail/state", boolState(vmEnabled), true)

		if changed {
			b.logger.Infof("Estado contestador actualizado desde dispositivo: enabled=%v, welcome_msg=%v", vmEnabled, wmEnabled)
		} else {
			b.logger.Debugf("Estado contestador: enabled=%v, welcome_msg=%v (sin cambios)", vmEnabled, wmEnabled)
		}
	} else {
		b.logger.WithField("response", response).Debug("Respuesta de consulta voicemail no reconocida")
	}
}

// QueryBellStatus consulta el estado real del sonido del timbre via *#8**33##.
// Respuesta: *#8**33*0## (muted) o *#8**33*1## (unmuted).
func (b *MQTTBridge) QueryBellStatus() {
	response, err := b.executeOWNWithResponse(bticino.CmdBellQuery)
	if err != nil {
		b.logger.WithError(err).Warn("No se pudo consultar estado del timbre")
		return
	}

	if strings.Contains(response, "*#8**33*0##") {
		b.mu.Lock()
		changed := b.doorbellSoundOn != false
		b.doorbellSoundOn = false
		b.mu.Unlock()
		b.publish(b.dataPrefix()+"/doorbellsound/state", "OFF", true)
		if changed {
			b.logger.Info("Estado timbre actualizado desde dispositivo: muted (OFF)")
		}
	} else if strings.Contains(response, "*#8**33*1##") {
		b.mu.Lock()
		changed := b.doorbellSoundOn != true
		b.doorbellSoundOn = true
		b.mu.Unlock()
		b.publish(b.dataPrefix()+"/doorbellsound/state", "ON", true)
		if changed {
			b.logger.Info("Estado timbre actualizado desde dispositivo: unmuted (ON)")
		}
	} else {
		b.logger.WithField("response", response).Debug("Respuesta de consulta timbre no reconocida")
	}
}

// UpdateBellState actualiza el estado del timbre desde un evento del bus OWN (tiempo real).
func (b *MQTTBridge) UpdateBellState(on bool) {
	b.mu.Lock()
	b.doorbellSoundOn = on
	b.mu.Unlock()
	b.publish(b.dataPrefix()+"/doorbellsound/state", boolState(on), true)
}

// UpdateVoicemailState actualiza el estado del contestador desde un evento del bus OWN (tiempo real).
func (b *MQTTBridge) UpdateVoicemailState(enabled bool) {
	b.mu.Lock()
	b.voicemailEnabled = enabled
	b.mu.Unlock()
	b.publish(b.dataPrefix()+"/voicemail/state", boolState(enabled), true)
}

// PublishAdditionalLockEvent publica un evento de cerradura adicional detectado en el bus OWN.
func (b *MQTTBridge) PublishAdditionalLockEvent(whereID string, state string) {
	dp := b.dataPrefix()
	b.publish(fmt.Sprintf("%s/lock/%s/state", dp, whereID), state, true)

	// Auto re-lock despues de 5 segundos (mismo patron que cerradura principal)
	if state == "UNLOCKED" {
		go func() {
			time.Sleep(5 * time.Second)
			b.publish(fmt.Sprintf("%s/lock/%s/state", dp, whereID), "LOCKED", true)
		}()
	}
}

// QuerySystemInfo consulta informacion del sistema via comandos WHO=13.
// Parsea las respuestas y publica como JSON en el sensor system_info.
func (b *MQTTBridge) QuerySystemInfo() {
	info := make(map[string]string)

	// Definicion de queries: {constante, clave, prefijo_respuesta}
	queries := []struct {
		cmd    string
		key    string
		prefix string
		isMAC  bool
	}{
		{bticino.CmdQueryIP, "ip_address", "*#13**10*", false},
		{bticino.CmdQueryNetmask, "netmask", "*#13**11*", false},
		{bticino.CmdQueryMAC, "mac_address", "*#13**12*", true},
		{bticino.CmdQueryFWVer, "firmware_version", "*#13**16*", false},
		{bticino.CmdQueryHWVer, "hardware_version", "*#13**17*", false},
		{bticino.CmdQueryKernel, "kernel_version", "*#13**23*", false},
		{bticino.CmdQueryDistro, "distro_version", "*#13**24*", false},
	}

	for _, q := range queries {
		response, err := b.executeOWNWithResponse(q.cmd)
		if err != nil {
			b.logger.WithError(err).WithField("query", q.key).Warn("Error en consulta WHO=13")
			continue
		}

		// Parsear respuesta: buscar la linea que empieza con el prefijo
		for _, line := range strings.Split(response, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, q.prefix) {
				// Extraer valor: quitar prefijo y ## final
				value := strings.TrimPrefix(line, q.prefix)
				value = strings.TrimSuffix(value, "##")

				if q.isMAC {
					// MAC: convertir octetos decimales a hex con ':'
					info[q.key] = parseMAC(value)
				} else {
					// IP y versiones: reemplazar * por .
					info[q.key] = strings.ReplaceAll(value, "*", ".")
				}
				break
			}
		}

		// Delay entre comandos para respetar timing BTicino
		time.Sleep(bticino.CommandRetryDelay)
	}

	if len(info) == 0 {
		b.logger.Warn("No se obtuvo informacion del sistema (WHO=13)")
		return
	}

	// Publicar como JSON en el sensor
	jsonData, err := json.Marshal(info)
	if err != nil {
		b.logger.WithError(err).Error("Error serializando info sistema")
		return
	}

	dp := b.dataPrefix()
	// Publicar resumen como estado principal del sensor
	summary := "desconocido"
	if fw, ok := info["firmware_version"]; ok {
		summary = fw
	}
	b.publish(dp+"/sensor/system_info", summary, true)
	b.publish(dp+"/sensor/system_info/attributes", string(jsonData), true)

	b.logger.WithField("info", info).Info("Informacion del sistema publicada")
}

// parseMAC convierte octetos decimales separados por * a formato MAC hex (AA:BB:CC:DD:EE:FF)
func parseMAC(starSeparated string) string {
	parts := strings.Split(starSeparated, "*")
	hexParts := make([]string, 0, len(parts))
	for _, p := range parts {
		var dec int
		fmt.Sscanf(p, "%d", &dec)
		hexParts = append(hexParts, fmt.Sprintf("%02x", dec))
	}
	return strings.Join(hexParts, ":")
}

// PublishTemperature publishes device temperature to MQTT
func (b *MQTTBridge) PublishTemperature(tempCelsius float64) {
	b.publish(b.dataPrefix()+"/sensor/temperature", fmt.Sprintf("%.1f", tempCelsius), true)
}

// PublishLEDStatus publishes individual LED states as binary_sensors
func (b *MQTTBridge) PublishLEDStatus(leds map[string]bool) {
	dp := b.dataPrefix()
	for name, on := range leds {
		// Skip mmc LEDs (SD card activity, not useful)
		if name == "mmc0::" || name == "mmc2::" {
			continue
		}
		b.publish(dp+"/led/"+name+"/state", boolState(on), true)
	}
}

// PublishButtonPress publishes a button press event
func (b *MQTTBridge) PublishButtonPress(keyCode int, keyName, action string) {
	b.publish(b.dataPrefix()+"/sensor/keypad", keyName, false)
	b.incEvents()
}

// PublishDoorbellEvent publishes a doorbell trigger
func (b *MQTTBridge) PublishDoorbellEvent(source, rawCommand string) {
	b.mu.Lock()
	b.lastDoorbellTime = time.Now()
	b.mu.Unlock()
	b.publish(b.dataPrefix()+"/doorbell/state", "ON", false)
	b.incEvents()
}

// PublishOpenWebNetEvent publishes a bus event
func (b *MQTTBridge) PublishOpenWebNetEvent(rawCommand, who, what, where string) {
	if rawCommand != "" {
		b.publish(b.dataPrefix()+"/sensor/bus_event", rawCommand, false)
	}
	b.incEvents()
}

// PublishMessageStats publishes answering machine message stats
func (b *MQTTBridge) PublishMessageStats() {
	if b.messageParser == nil {
		return
	}
	dp := b.dataPrefix()

	status, err := b.messageParser.GetAnsweringMachineStatus()
	if err != nil {
		b.logger.WithError(err).Debug("Failed to get message stats for MQTT")
		return
	}

	b.publish(dp+"/sensor/new_messages", fmt.Sprintf("%d", status.NewMessages), true)
	b.publish(dp+"/sensor/total_messages", fmt.Sprintf("%d", status.TotalMessages), true)
	b.publish(dp+"/sensor/storage_used", fmt.Sprintf("%d", status.StorageUsedMB), true)
}

// PublishSIPStatus publishes SIP registration status to MQTT
func (b *MQTTBridge) PublishSIPStatus(registered bool) {
	b.mu.Lock()
	b.sipRegistered = registered
	b.mu.Unlock()
	b.publish(b.dataPrefix()+"/sensor/sip_status", boolState(registered), true)
}

// PublishSystemDiagnostics publishes system diagnostic info to MQTT
func (b *MQTTBridge) PublishSystemDiagnostics(status string) {
	b.publish(b.dataPrefix()+"/sensor/system_diag", status, true)
}

// PublishMulticastStats publishes multicast listener message count to MQTT
func (b *MQTTBridge) PublishMulticastStats(totalMessages int64) {
	b.publish(b.dataPrefix()+"/sensor/multicast_count", fmt.Sprintf("%d", totalMessages), true)
}

// PublishGPIOStates publishes individual GPIO pin states to MQTT
func (b *MQTTBridge) PublishGPIOStates(states map[int]bool) {
	dp := b.dataPrefix()
	for pin, on := range states {
		b.publish(fmt.Sprintf("%s/gpio/%d/state", dp, pin), boolState(on), true)
	}
}

// PublishActivityLog publishes a human-readable activity event to the activity_log sensor.
// description: texto legible (ej. "Timbre: llamada desde placa exterior")
// eventType: tipo maquina (ej. "doorbell_ring", "door_open", "camera_activated")
// rawCommand: comando OWN original (ej. "*8*1#1#4#21*16##")
func (b *MQTTBridge) PublishActivityLog(description, eventType, rawCommand string) {
	dp := b.dataPrefix()

	// Publicar estado principal (texto legible, no retained para que sea evento)
	b.publish(dp+"/sensor/activity_log", description, false)

	// Publicar atributos JSON para automatizaciones
	attrs := map[string]interface{}{
		"event_type":  eventType,
		"raw_command": rawCommand,
		"timestamp":   time.Now().Format(time.RFC3339),
	}
	jsonData, err := json.Marshal(attrs)
	if err == nil {
		b.publish(dp+"/sensor/activity_log/attributes", string(jsonData), false)
	}

	b.logger.WithFields(map[string]interface{}{
		"description": description,
		"event_type":  eventType,
		"raw_command": rawCommand,
	}).Info("MQTT: Activity log published")
	b.incEvents()
}

// PublishMessage is a public wrapper for external callers
func (b *MQTTBridge) PublishMessage(topic, payload string, retain bool) {
	b.publish(topic, payload, retain)
}

// ==================== COMMAND EXECUTION ====================

// executeOWN sends an OpenWebNet command via netcat (non-interfering)
func (b *MQTTBridge) executeOWN(command string) (bool, error) {
	b.logger.WithField("command", command).Debug("Executing OWN command via netcat")

	cmd := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | nc 0 30006", command))
	if err := cmd.Run(); err != nil {
		b.logger.WithError(err).WithField("command", command).Error("OWN command failed")
		return false, fmt.Errorf("netcat command %s failed: %w", command, err)
	}

	b.logger.WithField("command", command).Info("OWN command executed via netcat")
	return true, nil
}

// ExecuteCommand public wrapper (for compatibility)
func (b *MQTTBridge) ExecuteCommand(command string) (bool, error) {
	return b.executeOWN(command)
}

// executeOWNWithResponse envia un comando OWN abriendo una sesion de comandos TCP dedicada
// al puerto 20000. Cada llamada abre una conexion nueva, envia *99*0## para abrir sesion,
// luego envia el comando y lee la respuesta (dato + ACK) en el mismo socket.
// Esto es necesario porque port 30006 solo devuelve ACK/NACK sin datos de respuesta.
func (b *MQTTBridge) executeOWNWithResponse(command string) (string, error) {
	b.logger.WithField("command", command).Debug("Executing OWN query via TCP command session (port 20000)")

	// Conectar a puerto 20000 con timeout de 5s
	conn, err := net.DialTimeout("tcp", "127.0.0.1:20000", 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("error conectando a puerto 20000: %w", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// readMessage lee un mensaje OWN terminado en ## del socket
	readMessage := func(timeout time.Duration) (string, error) {
		conn.SetReadDeadline(time.Now().Add(timeout))
		var msg strings.Builder
		for {
			b, err := reader.ReadByte()
			if err != nil {
				return msg.String(), err
			}
			msg.WriteByte(b)
			s := msg.String()
			if strings.HasSuffix(s, "##") {
				return s, nil
			}
		}
	}

	// Paso 1: Leer greeting del servidor
	greeting, err := readMessage(3 * time.Second)
	if err != nil {
		return "", fmt.Errorf("error leyendo greeting: %w", err)
	}
	b.logger.WithField("greeting", greeting).Debug("OWN query session greeting")

	// Si requiere autenticacion (*98*2##), no soportamos queries via TCP en este caso
	if strings.Contains(greeting, "*98*") {
		return "", fmt.Errorf("servidor requiere autenticacion HMAC, query no soportada")
	}

	// Paso 2: Enviar apertura de sesion de comandos *99*0##
	_, err = conn.Write([]byte("*99*0##"))
	if err != nil {
		return "", fmt.Errorf("error enviando sesion de comandos: %w", err)
	}

	// Paso 3: Leer respuesta a sesion (debe ser *#*1## = ACK)
	sessionResp, err := readMessage(3 * time.Second)
	if err != nil {
		return "", fmt.Errorf("error leyendo respuesta de sesion: %w", err)
	}
	if !strings.Contains(sessionResp, "*#*1##") {
		return "", fmt.Errorf("sesion de comandos rechazada: %s", sessionResp)
	}

	// Paso 4: Enviar el comando de consulta
	_, err = conn.Write([]byte(command))
	if err != nil {
		return "", fmt.Errorf("error enviando comando %s: %w", command, err)
	}

	// Paso 5: Leer respuesta(s) - puede ser dato + ACK o NACK
	// Leer primer mensaje (puede ser el dato o directamente ACK/NACK)
	firstResp, err := readMessage(3 * time.Second)
	if err != nil {
		return "", fmt.Errorf("error leyendo respuesta a %s: %w", command, err)
	}

	// Si es NACK, el comando no fue entendido
	if firstResp == "*#*0##" {
		return "", fmt.Errorf("comando %s rechazado (NACK)", command)
	}

	// Si es ACK directamente (sin dato), devolver vacio
	if firstResp == "*#*1##" {
		return "", nil
	}

	// Es el dato de respuesta. Intentar leer el ACK que sigue
	result := firstResp
	ackResp, err := readMessage(2 * time.Second)
	if err == nil {
		b.logger.WithField("ack", ackResp).Debug("ACK recibido tras dato de query")
	}

	b.logger.WithFields(logrus.Fields{
		"command":  command,
		"response": result,
	}).Info("OWN query response received via TCP session")
	return result, nil
}

// ==================== MQTT HELPERS ====================

// Publish sends a message to an MQTT topic (implements MQTTPublisherInterface)
func (b *MQTTBridge) Publish(topic, payload string, retain bool) {
	if b.client == nil || !b.IsConnected() {
		return
	}
	token := b.client.Publish(topic, 1, retain, payload)
	go func() {
		if token.Wait() && token.Error() != nil {
			b.logger.WithError(token.Error()).WithField("topic", topic).Error("MQTT publish failed")
		}
	}()
}

// publish sends a message to an MQTT topic (internal wrapper)
func (b *MQTTBridge) publish(topic, payload string, retain bool) {
	b.Publish(topic, payload, retain)
}

func boolState(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}

func (b *MQTTBridge) incEvents() {
	b.mu.Lock()
	b.eventsPublished++
	b.mu.Unlock()
}

func (b *MQTTBridge) incCommands() {
	b.mu.Lock()
	b.commandsReceived++
	b.mu.Unlock()
}

// ==================== STATUS AND LIFECYCLE ====================

// GetMQTTStatus returns detailed MQTT status for web API
func (b *MQTTBridge) GetMQTTStatus() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	status := map[string]interface{}{
		"connected":          b.isConnected,
		"broker":             fmt.Sprintf("%s:%d", b.config.MQTT.Host, b.config.MQTT.Port),
		"data_prefix":        b.dataPrefix(),
		"events_published":   b.eventsPublished,
		"commands_received":  b.commandsReceived,
		"entities":           39 + len(b.config.AdditionalLocks),
		"reconnect_attempts": b.reconnectAttempts,
	}

	if b.isConnected && !b.connectTime.IsZero() {
		status["connected_since"] = b.connectTime.Format(time.RFC3339)
		status["uptime"] = time.Since(b.connectTime).Round(time.Second).String()
	}
	if !b.lastDisconnectTime.IsZero() {
		status["last_disconnect"] = b.lastDisconnectTime.Format(time.RFC3339)
	}
	if !b.lastDoorbellTime.IsZero() {
		status["last_doorbell"] = b.lastDoorbellTime.Format(time.RFC3339)
	}
	if !b.lastButtonPress.IsZero() {
		status["last_button_press"] = b.lastButtonPress.Format(time.RFC3339)
		status["last_button_key"] = b.lastButtonKey
	}

	// Add connection events (no lock needed - already holding lock)
	if b.connectionEvents != nil {
		events := make([]ConnectionEvent, 0, 20)
		if !b.ceFull {
			for i := 0; i < b.cePos; i++ {
				if b.connectionEvents[i].Timestamp.IsZero() == false {
					events = append(events, b.connectionEvents[i])
				}
			}
		} else {
			for i := b.cePos; i < 20; i++ {
				if b.connectionEvents[i].Timestamp.IsZero() == false {
					events = append(events, b.connectionEvents[i])
				}
			}
			for i := 0; i < b.cePos; i++ {
				if b.connectionEvents[i].Timestamp.IsZero() == false {
					events = append(events, b.connectionEvents[i])
				}
			}
		}
		status["connection_events"] = events
	}

	return status
}

// Stop gracefully shuts down the MQTT bridge
func (b *MQTTBridge) Stop() {
	if b.client != nil {
		b.publish(b.dataPrefix()+"/bridge/state", "offline", true)
		time.Sleep(100 * time.Millisecond)
		b.client.Disconnect(250)
	}
}

// ==================== WEB SERVER COMPATIBILITY INTERFACE ====================

// SendOpenWebNetCommand implements BTicinoBridge interface for WebServer
func (b *MQTTBridge) SendOpenWebNetCommand(command string) error {
	_, err := b.executeOWN(command)
	return err
}

func (b *MQTTBridge) GetMulticastStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":   b.IsConnected(),
		"running":   b.IsConnected(),
		"broker":    fmt.Sprintf("%s:%d", b.config.MQTT.Host, b.config.MQTT.Port),
		"connected": b.IsConnected(),
	}
}

func (b *MQTTBridge) GetEventBusStats() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return map[string]interface{}{
		"enabled":            b.eventBus != nil,
		"total_events":       b.eventsPublished,
		"active_subscribers": 5,
	}
}

func (b *MQTTBridge) GetVideoStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":              false,
		"sip_registered":       false,
		"rtsp_active_sessions": 0,
		"video_manager":        nil,
	}
}
