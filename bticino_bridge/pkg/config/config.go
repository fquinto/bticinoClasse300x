package config

import (
	"fmt"
	"os"
	"time"

	"bticino_bridge/pkg/version"
	"gopkg.in/yaml.v2"
)

// Config represents the complete bridge configuration
type Config struct {
	Bridge          BridgeConfig        `yaml:"bridge"`
	OpenWebNet      OpenWebNetConfig    `yaml:"openwebnet"`
	SIP             SIPConfig           `yaml:"sip"`
	MQTT            MQTTConfig          `yaml:"mqtt"`
	HomeKit         HomeKitConfig       `yaml:"homekit"`
	Hardware        HardwareConfig      `yaml:"hardware"`
	Web             WebConfig           `yaml:"web"`
	Logging         LoggingConfig       `yaml:"logging"`
	AdditionalLocks []AdditionalLock    `yaml:"additional_locks"`
	UDPProxy        UDPProxyConfig      `yaml:"udp_proxy"`
	Streaming       StreamingConfig     `yaml:"streaming"`
	Network         NetworkConfig       `yaml:"network,omitempty"`
	Servers         ServersConfig       `yaml:"servers,omitempty"`
	Notifications   NotificationsConfig `yaml:"notifications,omitempty"`
	Privacy         PrivacyConfig       `yaml:"privacy,omitempty"`
	Security        SecurityConfig      `yaml:"security,omitempty"`
}

type BridgeConfig struct {
	Name     string `yaml:"name"`
	Version  string `yaml:"version"`
	LogLevel string `yaml:"log_level"`
}

type OpenWebNetConfig struct {
	Host          string        `yaml:"host"`
	Port          int           `yaml:"port"`
	Timeout       time.Duration `yaml:"timeout"`
	RetryAttempts int           `yaml:"retry_attempts"`
	RetryDelay    time.Duration `yaml:"retry_delay"`
}

type SIPConfig struct {
	Enabled     bool   `yaml:"enabled"`
	LocalHost   string `yaml:"local_host"`
	LocalPort   int    `yaml:"local_port"`
	ServerHost  string `yaml:"server_host"`
	ServerPort  int    `yaml:"server_port"`
	Transport   string `yaml:"transport"`
	Domain      string `yaml:"domain"`
	AuthFile    string `yaml:"auth_file"`
	CertFile    string `yaml:"cert_file"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	DevAddr     string `yaml:"dev_addr"`
	UseHA1      bool   `yaml:"use_ha1"`
	InsecureTLS bool   `yaml:"insecure_tls"`
	SIPTarget   string `yaml:"sip_target"` // INVITE target user (default: "c300x")
}

type MQTTConfig struct {
	Enabled     bool       `yaml:"enabled"`
	Host        string     `yaml:"host"`
	Port        int        `yaml:"port"`
	Broker      string     `yaml:"broker"` // For backward compatibility
	ClientID    string     `yaml:"client_id"`
	Username    string     `yaml:"username"`
	Password    string     `yaml:"password"`
	TopicPrefix string     `yaml:"topic_prefix"`
	Topics      MQTTTopics `yaml:"topics"`
	// EnableSystemButtons expone en HA (sección Configuration) botones de
	// reinicio: reiniciar el bridge y reiniciar el dispositivo. DESACTIVADO por
	// defecto porque son acciones potentes (el reboot reinicia todo el equipo).
	EnableSystemButtons bool `yaml:"enable_system_buttons"`
}

type MQTTTopics struct {
	// Events FROM hardware TO MQTT
	DoorEvents     string `yaml:"door_events"`
	ButtonEvents   string `yaml:"button_events"`
	SIPEvents      string `yaml:"sip_events"`
	HardwareEvents string `yaml:"hardware_events"`
	SystemStatus   string `yaml:"system_status"`

	// Commands FROM MQTT TO hardware
	DoorCommands   string `yaml:"door_commands"`
	SystemCommands string `yaml:"system_commands"`
	SIPCommands    string `yaml:"sip_commands"`

	// HomeKit integration topics
	HomeKitEvents  string `yaml:"homekit_events"`
	HomeKitStatus  string `yaml:"homekit_status"`
	HomeKitPairing string `yaml:"homekit_pairing"`

	// Compatibility with existing MQTT scripts
	RX  string `yaml:"rx"`  // Commands TO BTicino
	TX  string `yaml:"tx"`  // Events FROM BTicino
	Key string `yaml:"key"` // Key events
}

type HardwareConfig struct {
	Enabled         bool          `yaml:"enabled"`
	InputDevice     string        `yaml:"input_device"`
	GPIOMonitoring  bool          `yaml:"gpio_monitoring"`
	I2CScanning     bool          `yaml:"i2c_scanning"`
	PollingInterval time.Duration `yaml:"polling_interval"`
}

type WebConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Port         int    `yaml:"port"`
	StaticDir    string `yaml:"static_dir"`
	TemplatesDir string `yaml:"templates_dir"`
}

type HomeKitConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Name         string `yaml:"name"`
	Manufacturer string `yaml:"manufacturer"`
	Model        string `yaml:"model"`
	Port         string `yaml:"port"`
	Pin          string `yaml:"pin"`
	StoragePath  string `yaml:"storage_path"`
}

type LoggingConfig struct {
	Level    string `yaml:"level"`
	File     string `yaml:"file"`
	MaxSize  string `yaml:"max_size"`
	MaxFiles int    `yaml:"max_files"`
}

// AdditionalLock representa una cerradura adicional configurable.
// Cada cerradura tiene un WHERE address unico en el bus OpenWebNet.
// Ejemplo config YAML:
//
//	additional_locks:
//	  - name: "Puerta Trasera"
//	    where: 21
//	  - name: "Puerta Lateral"
//	    where: 22
type AdditionalLock struct {
	Name  string `yaml:"name"`  // Nombre legible (ej. "Puerta Trasera")
	Where int    `yaml:"where"` // Direccion WHERE en bus OWN (ej. 21, 22)
}

// UDPProxyConfig configura el proxy UDP que reenvía paquetes del puerto 40004 al 4000.
// Esto permite que clientes externos (como la app movil BTicino) se comuniquen
// con el dispositivo a traves del bridge.
type UDPProxyConfig struct {
	Enabled    bool `yaml:"enabled"`
	ListenPort int  `yaml:"listen_port"` // Puerto de escucha (default: 40004)
	TargetPort int  `yaml:"target_port"` // Puerto destino (default: 4000)
}

// StreamingConfig configura el streaming RTSP/WebRTC
type StreamingConfig struct {
	Enabled              bool   `yaml:"enabled"`
	RTSPPort             int    `yaml:"rtsp_port"`
	WebRTCEnabled        bool   `yaml:"webrtc_enabled"`
	WebRCTPPort          int    `yaml:"webrtc_port"`
	RecordingPath        string `yaml:"recording_path"`
	MaxDuration          int    `yaml:"max_duration"`
	AutoStopOnLastClient bool   `yaml:"auto_stop_on_last_client"`
	// VideoBackend selecciona cómo se obtiene el vídeo de la cámara:
	//   "avmedia"   → comando *7*300 pidiendo a bt_av_media que duplique su RTP
	//                 (cooperativo con el firmware nativo; toca el puerto 30007)
	//   "gstreamer" → pipeline GStreamer directo capturando /dev/video0 con la VPU
	//                 (compite con los procesos nativos; puede fallar en frío)
	//   "auto"      → intenta avmedia y, si no fluye RTP, cae a gstreamer
	VideoBackend string `yaml:"video_backend"`
	// VideoOnDemand habilita la ACTIVACIÓN de vídeo bajo demanda: self-INVITE
	// del servidor RTSP, arranque automático de vídeo al pulsar el timbre y
	// snapshots. DESACTIVADO por defecto por seguridad: activar vídeo compite
	// con el firmware nativo y, si algo se reintenta, puede reiniciar el equipo.
	// Con false, ninguna de esas vías envía comandos al dispositivo nativo.
	VideoOnDemand bool `yaml:"video_on_demand"`
}

// Backends de vídeo soportados
const (
	VideoBackendAVMedia   = "avmedia"
	VideoBackendGStreamer = "gstreamer"
	VideoBackendAuto      = "auto"
)

// GetStreamingConfig returns default streaming config
func GetStreamingConfig() StreamingConfig {
	return StreamingConfig{
		Enabled:              true,
		RTSPPort:             6554,
		WebRTCEnabled:        false,
		WebRCTPPort:          8889,
		MaxDuration:          60,
		AutoStopOnLastClient: true,
		VideoBackend:         VideoBackendAVMedia,
	}
}

// NetworkConfig configura la red y servidores externos
type NetworkConfig struct {
	NTP      NTPConfig      `yaml:"ntp"`
	DNS      DNSConfig      `yaml:"dns"`
	Firewall FirewallConfig `yaml:"firewall"`
}

type NTPConfig struct {
	Enabled bool   `yaml:"enabled"`
	Server  string `yaml:"server"`
	Port    int    `yaml:"port"`
}

type DNSConfig struct {
	Servers []string `yaml:"servers"`
}

type FirewallConfig struct {
	BlockTelemetry bool `yaml:"block_telemetry"`
	BlockCloud     bool `yaml:"block_cloud"`
	AllowUpdates   bool `yaml:"allow_updates"`
}

// ServersConfig configura los servidores externos oficiales
type ServersConfig struct {
	Cloud       CloudServerConfig       `yaml:"cloud"`
	Logging     LogServerConfig         `yaml:"logging"`
	Updates     UpdatesServerConfig     `yaml:"updates"`
	SIPOfficial SIPOfficialServerConfig `yaml:"sip_official"`
}

type CloudServerConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Protocol    string `yaml:"protocol"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

type LogServerConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Protocol    string `yaml:"protocol"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

type UpdatesServerConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Protocol    string `yaml:"protocol"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

type SIPOfficialServerConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Protocol    string `yaml:"protocol"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

// NotificationsConfig configura las notificaciones
type NotificationsConfig struct {
	MQTT     MQTTNotificationConfig     `yaml:"mqtt"`
	Email    EmailNotificationConfig    `yaml:"email"`
	Pushover PushoverNotificationConfig `yaml:"pushover"`
	Telegram TelegramNotificationConfig `yaml:"telegram"`
}

type MQTTNotificationConfig struct {
	Enabled bool   `yaml:"enabled"`
	Topic   string `yaml:"topic"`
}

type EmailNotificationConfig struct {
	Enabled    bool   `yaml:"enabled"`
	SMTPServer string `yaml:"smtp_server"`
	SMTPPort   int    `yaml:"smtp_port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	From       string `yaml:"from"`
	To         string `yaml:"to"`
}

type PushoverNotificationConfig struct {
	Enabled bool   `yaml:"enabled"`
	APIKey  string `yaml:"api_key"`
	UserKey string `yaml:"user_key"`
}

type TelegramNotificationConfig struct {
	Enabled  bool   `yaml:"enabled"`
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

// PrivacyConfig configura la privacidad
type PrivacyConfig struct {
	BlockExternalTelemetry bool `yaml:"block_external_telemetry"`
	BlockLogServer         bool `yaml:"block_log_server"`
	BlockCloud             bool `yaml:"block_cloud"`
	DisableAutoUpdates     bool `yaml:"disable_auto_updates"`
	LocalLogging           bool `yaml:"local_logging"`
}

// SecurityConfig configura la seguridad
type SecurityConfig struct {
	WebAuthRequired bool `yaml:"web_auth_required"`
	WebHTTPSEnabled bool `yaml:"web_https_enabled"`
	APIRateLimit    int  `yaml:"api_rate_limit"`
	ConfigAuditLog  bool `yaml:"config_audit_log"`
}

// LoadConfig loads configuration from YAML file
func LoadConfig(configPath string) (*Config, error) {
	config := &Config{}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	// Set defaults if not specified
	if config.OpenWebNet.Host == "" {
		config.OpenWebNet.Host = "localhost"
	}
	if config.OpenWebNet.Port == 0 {
		config.OpenWebNet.Port = 30006
	}
	if config.Streaming.RTSPPort == 0 {
		config.Streaming.RTSPPort = 6554
	}
	if config.MQTT.ClientID == "" {
		config.MQTT.ClientID = "bticino_bridge"
	}

	return config, nil
}

// GetDefaultConfig returns a configuration with sensible defaults
func GetDefaultConfig() *Config {
	// Initialize version system
	version.InitVersion()

	return &Config{
		Bridge: BridgeConfig{
			Name:     "BTicino Bridge",
			Version:  version.GetVersion(),
			LogLevel: "info",
		},
		OpenWebNet: OpenWebNetConfig{
			Host:          "localhost",
			Port:          30006,
			Timeout:       5 * time.Second,
			RetryAttempts: 3,
			RetryDelay:    1 * time.Second,
		},
		SIP: SIPConfig{
			Enabled:    true,
			LocalHost:  "127.0.0.1",
			LocalPort:  47300,
			ServerHost: "sipserver.bs.iotleg.com",
			ServerPort: 5061,
			Transport:  "tls",
			Domain:     "bs.iotleg.com",
		},
		MQTT: MQTTConfig{
			Enabled:     true,
			Host:        "localhost",
			Port:        1883,
			Broker:      "localhost:1883", // For backward compatibility
			ClientID:    "bticino_bridge",
			TopicPrefix: "bticino",
			Topics: MQTTTopics{
				DoorEvents:     "bticino/events/door",
				ButtonEvents:   "bticino/events/buttons",
				SIPEvents:      "bticino/events/sip",
				HardwareEvents: "bticino/events/hardware",
				SystemStatus:   "bticino/status/system",
				DoorCommands:   "bticino/commands/door",
				SystemCommands: "bticino/commands/system",
				SIPCommands:    "bticino/commands/sip",
				HomeKitEvents:  "bticino/homekit/events",
				HomeKitStatus:  "bticino/homekit/status",
				HomeKitPairing: "bticino/homekit/pairing",
				RX:             "Bticino/rx",
				TX:             "Bticino/tx",
				Key:            "Bticino/key",
			},
		},
		HomeKit: HomeKitConfig{
			Enabled:      true,
			Name:         "BTicino Bridge",
			Manufacturer: "BTicino",
			Model:        "Class 300X",
			Port:         "8080",
			Pin:          "12345678",
			StoragePath:  "./homekit_data",
		},
		Hardware: HardwareConfig{
			Enabled:         true,
			InputDevice:     "/dev/input/event0",
			GPIOMonitoring:  true,
			I2CScanning:     false,
			PollingInterval: 100 * time.Millisecond,
		},
		Web: WebConfig{
			Enabled:      true,
			Port:         8080,
			StaticDir:    "web/static",
			TemplatesDir: "web/templates",
		},
		Logging: LoggingConfig{
			Level:    "info",
			File:     "/var/log/bticino_bridge.log",
			MaxSize:  "10MB",
			MaxFiles: 5,
		},
		UDPProxy: UDPProxyConfig{
			Enabled:    false,
			ListenPort: 40004,
			TargetPort: 4000,
		},
		Streaming: StreamingConfig{
			Enabled:              true,
			RTSPPort:             6554,
			WebRTCEnabled:        false,
			WebRCTPPort:          8889,
			RecordingPath:        "/home/bticino/cfg/extra/recordings",
			MaxDuration:          60,
			AutoStopOnLastClient: true,
		},
	}
}
