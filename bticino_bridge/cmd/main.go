package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"bticino_bridge/pkg/access"
	"bticino_bridge/pkg/config"
	"bticino_bridge/pkg/deviceconfig"
	"bticino_bridge/pkg/events"
	"bticino_bridge/pkg/homekit"
	"bticino_bridge/pkg/input"
	"bticino_bridge/pkg/openwebnet"
	"bticino_bridge/pkg/sip"
	"bticino_bridge/pkg/survival"
	versionpkg "bticino_bridge/pkg/version"
	"bticino_bridge/pkg/webserver"

	// Enhanced packages from BTicino device analysis
	"bticino_bridge/pkg/bticino"
	"bticino_bridge/pkg/bticino_commands"
	"bticino_bridge/pkg/messageparser"
	"bticino_bridge/pkg/mqtt"
	"bticino_bridge/pkg/multicast"
	"bticino_bridge/pkg/multicast/handlers"
	"bticino_bridge/pkg/udpproxy"
)

var (
	BuildTime = "unknown"
)

// SimpleMQTTBridge is a basic MQTT bridge interface for web server compatibility
type SimpleMQTTBridge struct {
	logger   *logrus.Logger
	owClient *openwebnet.Client
}

func (s *SimpleMQTTBridge) GetMulticastStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":            false,
		"running":            false,
		"total_messages":     0,
		"unhandled_messages": 0,
		"error_count":        0,
	}
}

func (s *SimpleMQTTBridge) GetEventBusStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":            false,
		"total_events":       0,
		"active_subscribers": 0,
	}
}

func (s *SimpleMQTTBridge) GetVideoStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":              false,
		"sip_registered":       false,
		"rtsp_active_sessions": 0,
		"video_manager":        nil,
	}
}

// ExecuteCommand sends an OpenWebNet command via netcat (non-interfering method)
func (s *SimpleMQTTBridge) ExecuteCommand(command string) (bool, error) {
	s.logger.WithField("command", command).Info("Executing OpenWebNet command via netcat (SimpleMQTTBridge)")

	// Use netcat method to avoid interfering with native BTicino processes
	// This is the same method used in the working script
	cmd := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | nc 0 30006", command))

	if err := cmd.Run(); err != nil {
		s.logger.WithError(err).WithField("command", command).Error("Failed to execute OpenWebNet command via netcat")
		return false, fmt.Errorf("failed to send command %s via netcat: %w", command, err)
	}

	s.logger.WithFields(map[string]interface{}{
		"command": command,
		"method":  "netcat",
		"port":    "30006",
	}).Info("OpenWebNet command executed successfully via netcat")

	return true, nil
}

// SendOpenWebNetCommand implements BTicinoBridge interface for WebServer integration
func (s *SimpleMQTTBridge) SendOpenWebNetCommand(command string) error {
	s.logger.WithField("command", command).Info("Sending OpenWebNet command via SimpleMQTTBridge")

	_, err := s.ExecuteCommand(command)
	return err
}

// ownAdapter adapta *openwebnet.Client (SendCommand devuelve (*Command, error))
// a la interfaz sip.OWNSender (SendOpenWebNetCommand devuelve solo error) que
// necesita AudioCall para armar el audio bidireccional (WHO=8) antes del talk-back.
type ownAdapter struct{ client *openwebnet.Client }

func (a *ownAdapter) SendOpenWebNetCommand(cmd string) error {
	_, err := a.client.SendCommand(cmd)
	return err
}

func main() {
	// Parse command line flags
	var (
		configPath       = flag.String("config", "configs/config.yaml", "Path to configuration file")
		secretsPath      = flag.String("secrets", "", "Path to secrets file (default: secrets.yaml next to -config)")
		logLevel         = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		version          = flag.Bool("version", false, "Show version information")
		testMode         = flag.Bool("test", false, "Run in test mode (no actual device connection)")
		webPort          = flag.Int("web-port", 0, "Override web server port (0 = use config)")
		enableOpenWebNet = flag.Bool("enable-openwebnet", true, "Enable OpenWebNet client")
		enableWeb        = flag.Bool("enable-web", true, "Enable web interface")
		enableHomeKit    = flag.Bool("enable-homekit", true, "Enable HomeKit bridge")
		enableVideo      = flag.Bool("enable-video", true, "Enable video/SIP system")
		enableAnswerer   = flag.Bool("enable-answerer", true, "Enable SIP answerer (contestador)")
		enableSurvival   = flag.Bool("enable-survival", true, "Enable cloud-survival runtime toggle (Pilar D)")
	)
	flag.Parse()

	if *version {
		fmt.Printf("BTicino Bridge v%s (built: %s)\n", versionpkg.GetVersion(), BuildTime)
		fmt.Println("🚀 BTicino Bridge - Real Device Integration:")
		fmt.Println("  ✅ OpenWebNet (50+ proven commands)")
		fmt.Println("  🌐 Web Dashboard")
		fmt.Println("  🏠 HomeKit + Home Assistant (MQTT integration)")
		fmt.Println("  📹 Video/SIP System (Streaming, RTSP)")
		fmt.Println("  📱 Answering Machine (Message parsing)")
		os.Exit(0)
	}

	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	// Set log level from command line
	switch *logLevel {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}

	logger.Info("🚀 === BTicino Bridge ===")
	logger.Infof("Version: %s (built: %s)", versionpkg.GetVersion(), BuildTime)
	logger.Info("Starting BTicino Bridge:")
	logger.Info("  🔌 OpenWebNet (50+ proven commands)")
	logger.Info("  🌐 Web Dashboard")
	logger.Info("  🏠 HomeKit + Home Assistant (MQTT)")
	logger.Info("  📹 Video/SIP System (Streaming)")
	logger.Info("  📱 Answering Machine")
	logger.Info("  🔄 Enhanced EventBus & Real-time Processing")

	// Load configuration
	// Secretos: fichero único (secrets.yaml hermano del config, o -secrets)
	// superpuesto sobre config.yaml. Ver docs/SECRETS.md.
	effectiveSecrets := *secretsPath
	if effectiveSecrets == "" {
		effectiveSecrets = config.SecretsPath(*configPath)
	}
	cfg, err := config.LoadConfigWithSecrets(*configPath, effectiveSecrets)
	if err != nil {
		// Un secrets.yaml ilegible o mal formado ABORTA: nunca se degrada a
		// "arrancar con los defaults" por un fichero de secretos roto.
		var secretsErr *config.SecretsFileError
		if errors.As(err, &secretsErr) {
			logger.Fatalf("%v", err)
		}
		logger.Warnf("Failed to load config from %s, using defaults: %v", *configPath, err)
		cfg = config.GetDefaultConfig()
	}
	// Dispositivo sin migrar: config.yaml aún guarda credenciales que
	// secrets.yaml no aporta. Sigue funcionando, pero hay que moverlas.
	if keys, _ := config.UnmigratedSecretsInFile(*configPath, effectiveSecrets); len(keys) > 0 {
		logger.Warnf("config.yaml still holds credentials (%s) that %s does not provide: move them to secrets.yaml (see docs/SECRETS.md). The web dashboard will refuse to save the configuration until then.",
			strings.Join(keys, ", "), effectiveSecrets)
	}

	// Override web port if specified
	if *webPort > 0 {
		cfg.Web.Port = *webPort
		cfg.Web.Enabled = *enableWeb
		logger.WithField("port", *webPort).Info("Web server port overridden via command line")
	}

	// Apply component enable/disable flags
	cfg.Web.Enabled = cfg.Web.Enabled && *enableWeb
	cfg.HomeKit.Enabled = cfg.HomeKit.Enabled && *enableHomeKit
	cfg.SIP.Enabled = cfg.SIP.Enabled && *enableVideo
	cfg.SIP.Answerer.Enabled = cfg.SIP.Answerer.Enabled && cfg.SIP.Enabled && *enableAnswerer

	// Gate fail-closed de secretos. Va DESPUÉS del AND con los flags: solo se
	// exigen los secretos de los subsistemas realmente activados. Sin bypass.
	if config.SecretsFileTooOpen(effectiveSecrets) {
		logger.Warnf("Secrets file %s is readable by group/others; restrict it: chmod 0600 %s", effectiveSecrets, effectiveSecrets)
	}
	if err := cfg.Validate(); err != nil {
		logger.Fatalf("%v (secrets file: %s)", err, effectiveSecrets)
	}

	logger.Infof("Configuration loaded: device=%s", cfg.OpenWebNet.Host)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Direcciones de bus (abrir puerta / luz de escalera): config > conf.xml
	// del equipo > valores históricos. Solo lee un fichero local.
	confXMLPath := cfg.OpenWebNet.ConfXMLPath
	if confXMLPath == "" {
		confXMLPath = "/var/tmp/conf.xml"
	}
	busAddr := deviceconfig.ApplyBusAddresses(cfg.OpenWebNet.DoorAddress, cfg.OpenWebNet.LightAddress, confXMLPath)
	if cfg.OpenWebNet.DoorAddress != "" && busAddr.DoorSource != "config" {
		logger.Warnf("openwebnet.door_address %q no es válida (solo dígitos, máx. 4); se ignora", cfg.OpenWebNet.DoorAddress)
	}
	if cfg.OpenWebNet.LightAddress != "" && busAddr.LightSource != "config" {
		logger.Warnf("openwebnet.light_address %q no es válida (solo dígitos, máx. 4); se ignora", cfg.OpenWebNet.LightAddress)
	}
	logger.Infof("Bus addresses: door=%s (%s), stair light=%s (%s)", busAddr.Door, busAddr.DoorSource, busAddr.Light, busAddr.LightSource)
	if busAddr.ReadErr != nil && (busAddr.DoorSource == "default" || busAddr.LightSource == "default") {
		// /var/tmp es tmpfs: si el bridge arranca antes de que el equipo escriba
		// su conf.xml, se reintenta un rato en vez de quedarse con las históricas.
		logger.Warnf("Bus addresses: no se pudo leer %s (%v); uso las históricas y reintento", confXMLPath, busAddr.ReadErr)
		go func() {
			var r deviceconfig.BusAddresses
			if retryUntilRead(ctx, 10*time.Second, 30, func() error {
				r = deviceconfig.ApplyBusAddresses(cfg.OpenWebNet.DoorAddress, cfg.OpenWebNet.LightAddress, confXMLPath)
				return r.ReadErr
			}) {
				logger.Infof("Bus addresses: door=%s (%s), stair light=%s (%s)", r.Door, r.DoorSource, r.Light, r.LightSource)
			} else {
				logger.Warnf("Bus addresses: %s sigue sin poder leerse; se mantienen las históricas (fuérzalas con openwebnet.door_address / light_address si no son las tuyas)", confXMLPath)
			}
		}()
	}

	// Create central EventBus for all components
	eventBus := events.NewEventBus(10, logger)
	logger.Info("✅ Central EventBus created for component communication")

	// Clasificador de accesos (portal → telefonillo/llave). Suscriptor puro del
	// EventBus; no toca el bus OWN. Se gobierna con access.enabled del config.
	if cfg.Access.Enabled {
		accessClassifier := access.NewClassifier(cfg.Access, eventBus, logger)
		accessClassifier.Start()
		defer accessClassifier.Stop()
		logger.Info("Clasificador de accesos activado")
	}

	// Component variables
	var owClient *openwebnet.Client
	var webServer *webserver.WebServer
	var homekitBridge *homekit.BticinoBridge
	var sipClient *sip.BTicinoSIPClient
	var videoManager *sip.VideoStreamManager
	var rtspServer *sip.EnhancedRTSPServer
	var mqttBridge *mqtt.MQTTBridge

	// Enhanced components from BTicino device analysis
	var messageParser *messageparser.MessageParser
	var commandHandler *bticino_commands.BTicinoCommandHandler
	var inputMonitor *input.InputMonitor
	var multicastListener *multicast.MulticastListener

	// Simple MQTT bridge for web compatibility
	simpleMQTT := &SimpleMQTTBridge{logger: logger}

	// ==================== COMPONENT 1: Enhanced OpenWebNet Client ====================
	if *enableOpenWebNet {
		logger.Info("🔌 Initializing Enhanced OpenWebNet client...")
		logger.Info("🤝 Using non-interfering mode: monitoring only on port 20000, commands via netcat")

		owConfig := openwebnet.ClientConfig{
			Host:         cfg.OpenWebNet.Host,
			Ports:        []int{20000}, // ONLY port 20000 for monitoring - NO interference with native processes
			MainPort:     20000,
			VideoPort:    0, // Disabled - avoids competing with bt_vct (video control)
			ConfigPort:   0, // Disabled - avoids competing with openserver native process
			Timeout:      cfg.OpenWebNet.Timeout,
			RetryCount:   cfg.OpenWebNet.RetryAttempts,
			RetryDelay:   cfg.OpenWebNet.RetryDelay,
			EnableSafety: true, // Enable 4-level security system
			Logger:       logger,
		}

		owClient = openwebnet.NewClient(owConfig)

		if !*testMode {
			logger.Info("Connecting to BTicino device...")
			if err := owClient.Connect(); err != nil {
				logger.Errorf("Failed to connect to BTicino device: %v", err)
				logger.Warn("Continuing without OpenWebNet - features limited")
			} else {
				logger.Info("✅ OpenWebNet: Successfully connected to BTicino device")

				// Enable critical commands for door unlock functionality
				logger.Warn("🔓 Enabling critical commands for door unlock functionality")
				owClient.EnableCriticalCommands(true)

				// Test basic connectivity with proven BTicino commands from device analysis
				logger.Info("🔍 Testing real BTicino device commands...")

				if commandHandler != nil {
					// Test answering machine status using enhanced commands
					logger.Info("Testing enhanced answering machine commands...")

					// Use a simple OpenWebNet query first to verify basic connectivity
					response, err := owClient.SendCommand("*99*0##")
					if err != nil {
						logger.Warnf("Basic status query failed: %v", err)
					} else {
						logger.Infof("Basic device connectivity: %v", response)
					}

					// Test real BTicino commands with proper timing
					realTestCommands := []struct {
						command string
						desc    string
					}{
						{bticino.CmdBellQuery, "Bell/ringer status query"},
						{"*7*73#1#100*##", "Display activation (required before answering machine control)"},
						{"*1*0*11##", "Light status on point 11"},
					}

					for _, test := range realTestCommands {
						response, err := owClient.SendCommand(test.command)
						if err != nil {
							logger.Warnf("Real command %s (%s) failed: %v", test.command, test.desc, err)
						} else {
							logger.Infof("%s: %v", test.desc, response)
						}
						time.Sleep(310 * time.Millisecond) // Real BTicino timing from device analysis
					}

					logger.Info("Enhanced command handler active with real BTicino integration")
				} else {
					// Fallback to basic test commands if enhanced handler not available
					testCommands := []struct {
						command string
						desc    string
					}{
						{"*99*0##", "Basic status query"},
						{"*1*0*11##", "Light status on point 11"},
						{"*2*1*11##", "Shutter UP on point 11"},
					}

					for _, test := range testCommands {
						response, err := owClient.SendCommand(test.command)
						if err != nil {
							logger.Warnf("Command %s (%s) failed: %v", test.command, test.desc, err)
						} else {
							logger.Infof("%s: %v", test.desc, response)
						}
						time.Sleep(200 * time.Millisecond)
					}
				}
			}
		} else {
			logger.Info("Running in test mode - no OpenWebNet device connection")
		}
	}

	// ==================== COMPONENTS: BTicino Integration ====================
	logger.Info("🔧 Initializing Enhanced BTicino Components...")

	// Initialize Message Parser for answering machine
	messageParser = messageparser.NewMessageParser()
	logger.Info("✅ Enhanced Message Parser: Configured for real BTicino filesystem")

	// Initialize Enhanced Command Handler
	if owClient != nil {
		commandHandler = bticino_commands.NewBTicinoCommandHandler(owClient, logger)
		logger.Info("✅ Enhanced Command Handler: 50+ proven BTicino commands ready")
	} else {
		logger.Warn("Enhanced Command Handler disabled: No OpenWebNet connection")
	}

	// ==================== MQTT INTEGRATION: Real MQTT Client ====================
	if cfg.MQTT.Enabled {
		logger.Info("📡 Initializing Real MQTT Bridge for Home Assistant...")

		// Create real MQTT bridge
		var err error
		mqttBridge, err = mqtt.NewMQTTBridge(cfg, logger)
		if err != nil {
			logger.WithError(err).Error("Failed to create MQTT bridge")
		} else {
			logger.Info("✅ MQTT Bridge: Real client created for Home Assistant integration")
			logger.Infof("📡 MQTT Broker: %s:%d (user: %s)", cfg.MQTT.Host, cfg.MQTT.Port, cfg.MQTT.Username)

			// Iniciar publicador de configuración del dispositivo vía MQTT (Fase 3)
			go func() {
				time.Sleep(5 * time.Second)                           // Esperar a que MQTT se conecte
				mqttBridge.SetDeviceConfigPublisher(60 * time.Second) // Publicar cada 60s
			}()
		}
	} else {
		logger.Info("MQTT Bridge disabled in configuration")
	}

	logger.Info("🔧 Enhanced BTicino Components initialization complete")

	// ==================== COMPONENT 2: Web Dashboard ====================
	if cfg.Web.Enabled {
		logger.Info("🌐 Initializing Web Dashboard...")

		// Update SimpleMQTTBridge with owClient for command execution
		simpleMQTT.owClient = owClient

		webServer = webserver.NewWebServer(cfg, simpleMQTT, logger, *configPath, effectiveSecrets)
		webServer.SetGlobalWebServer()

		if err := webServer.Start(ctx); err != nil {
			logger.WithError(err).Error("Failed to start web server, continuing without web interface")
			webServer = nil
		} else {
			logger.Infof("✅ Web Dashboard: Started successfully on http://192.168.1.38:%d", cfg.Web.Port)
			logger.Info("  - Enhanced with real BTicino filesystem integration")
			logger.Info("  - Message parsing from /home/bticino/cfg/extra/47/messages/")
		}
	} else {
		logger.Info("Web dashboard disabled in configuration")
	}

	// ==================== COMPONENT 3: HomeKit Bridge ====================
	if cfg.HomeKit.Enabled {
		logger.Info("🏠 Initializing HomeKit Bridge...")

		homekitConfig := &homekit.Config{
			Name:         cfg.HomeKit.Name,
			Manufacturer: cfg.HomeKit.Manufacturer,
			Model:        cfg.HomeKit.Model,
			Port:         cfg.HomeKit.Port,
			Pin:          cfg.HomeKit.Pin,
			StoragePath:  cfg.HomeKit.StoragePath,
			Enabled:      cfg.HomeKit.Enabled,
		}

		homekitBridge, err = homekit.NewBticinoBridge(homekitConfig, eventBus, logger)
		if err != nil {
			logger.WithError(err).Error("Failed to create HomeKit bridge, continuing without HomeKit")
		} else {
			if err := homekitBridge.Start(); err != nil {
				logger.WithError(err).Error("Failed to start HomeKit bridge, continuing without HomeKit")
				homekitBridge = nil
			} else {
				logger.Infof("✅ HomeKit: Bridge started on port %s (PIN: %s)", cfg.HomeKit.Port, cfg.HomeKit.Pin)
			}
		}
	} else {
		logger.Info("HomeKit bridge disabled in configuration")
	}

	// ==================== COMPONENT 4: Video/SIP System ====================
	if cfg.SIP.Enabled {
		logger.Info("📹 Initializing Video/SIP System...")

		// Determine local IP for SIP
		localIP := cfg.SIP.LocalHost
		if localIP == "" {
			localIP = "192.168.1.38"
		}

		// Create SIP configuration from main config
		// Uses unified SIP config (compatible with both old and new config formats)
		serverAddr := cfg.SIP.ServerHost
		if serverAddr == "" {
			serverAddr = "sipserver.bs.iotleg.com:5061"
		} else if !strings.Contains(serverAddr, ":") {
			serverAddr = fmt.Sprintf("%s:%d", serverAddr, cfg.SIP.ServerPort)
		}

		// Sin fallbacks de credenciales: dominio y password vienen de
		// secrets.yaml y cfg.Validate() ya garantizó que no están vacíos.
		domain := cfg.SIP.Domain
		password := cfg.SIP.Password

		username := cfg.SIP.Username
		if username == "" {
			username = "webrtc"
		}

		devAddr := cfg.SIP.DevAddr
		if devAddr == "" {
			devAddr = "20"
		}

		logger.Infof("SIP Config: server=%s, domain=%s, username=%s, devaddr=%s",
			serverAddr, domain, username, devAddr)

		sipConfig := &sip.SIPConfig{
			ServerAddr:    serverAddr,
			Domain:        domain,
			Username:      username,
			Password:      password,
			LocalIP:       localIP,
			LocalPort:     cfg.SIP.LocalPort,
			DevAddr:       devAddr,
			ExpirySeconds: 300,
			RTSPPort:      6554,
			UseHA1:        cfg.SIP.UseHA1,
			InsecureTLS:   cfg.SIP.InsecureTLS,
			Transport:     cfg.SIP.Transport,
			SIPTarget:     cfg.SIP.SIPTarget,
			MediaPorts: sip.MediaPortConfig{
				AudioRTP: 7076,
				VideoRTP: 9078,
				RTCPPort: 9079,
			},
		}

		// Create SIP client
		sipClient = sip.NewBTicinoSIPClient(sipConfig, eventBus, logger)

		sipStarted := true
		// Retry SIP registration with backoff — Flexisip may not be ready yet after reboot
		var sipErr error
		for attempt := 1; attempt <= 5; attempt++ {
			sipErr = sipClient.Start()
			if sipErr == nil {
				break
			}
			logger.WithError(sipErr).Warnf("SIP start attempt %d/5 failed, retrying in %ds...", attempt, attempt*3)
			time.Sleep(time.Duration(attempt*3) * time.Second)
			// Reset client for retry
			sipClient = sip.NewBTicinoSIPClient(sipConfig, eventBus, logger)
		}
		if sipErr != nil {
			logger.WithError(sipErr).Warn("Failed to start SIP client after 5 attempts, continuing without full video features")
			sipClient = nil
			sipStarted = false
		} else {
			logger.Info("SIP: Client started successfully (dual-role: webrtc + c300x)")
			// Expose SIP call state + hangup to the web API
			if webServer != nil {
				webServer.SetCallController(sipClient)
				logger.Info("   Call control: GET /api/call, POST /api/controls/call/hangup")
			}

			// Talk-back v2: el bridge posee una llamada de audio local a c300x
			// (INVITE con SDP de audio real → 200 OK → WAV en SRTP al altavoz).
			if webServer != nil && owClient != nil {
				audioSender := sip.NewAudioSender("/usr/local/bin/ffmpeg", logger)
				audioReceiver := sip.NewAudioReceiver("/usr/local/bin/ffmpeg", logger)
				audioPort := sip.DefaultGStreamerConfig().VideoPort + 100 // puerto local libre para el leg de audio, sin colisionar con relay/RTSP
				audioCall := sip.NewAudioCall(sipClient, &ownAdapter{client: owClient}, audioSender, audioReceiver, "127.0.0.1", audioPort, logger)
				webServer.SetAudioCall(audioCall)
				logger.Infof("   Talk-back v2: POST /api/talkback/say (audio local port=%d)", audioPort)

				// Audio full-duplex de validación local (sdd 2026-08-21-duplex-door-audio):
				// solo se inyecta el controlador si el flag está activo en config
				// (fail-closed — sin el flag, los handlers /api/audio/duplex/* siguen
				// devolviendo 404 porque ws.duplexController nunca se asigna).
				if cfg.Audio.DuplexEnabled {
					webServer.SetDuplexController(audioCall)
					logger.Info("   Audio full-duplex activado")
				}
			}
		}

		// Create Enhanced RTSP server - can work without SIP (for go2rtc integration)
		rtspServer = sip.NewEnhancedRTSPServer(cfg.Streaming.RTSPPort, sipClient, eventBus, logger)

		// Set OpenWebNet client for *7*300 video stream activation
		if owClient != nil {
			rtspServer.SetOpenWebNetClient(owClient)
			logger.Info("   RTSP: OpenWebNet client connected for video activation")
		}

		// Select the video backend (default avmedia = cooperative *7*300)
		rtspServer.SetVideoBackend(cfg.Streaming.VideoBackend)

		// SEGURIDAD: activación de vídeo bajo demanda (self-INVITE, *7*300,
		// snapshots) desactivada por defecto. Con false el servidor RTSP escucha
		// pero nunca dispara comandos hacia el firmware nativo.
		rtspServer.SetVideoActivation(cfg.Streaming.VideoOnDemand)

		// Vídeo en vivo durante una llamada entrante a nuestra unidad, por espejo
		// PASIVO (tap AF_PACKET del leg de vídeo nativo en lo:5007 → relay 10002).
		// CERO comandos al bus (Fase 0 2026-08-18: un *7*300 propio en llamada
		// real se ignora). Independiente de video_on_demand.
		// `sip.call.incoming` es específico de nuestro c300x (lo publica el cliente
		// SIP al recibir un INVITE dirigido a nosotros), no actividad de otros pisos.
		if cfg.Streaming.LiveOnCall {
			tap := sip.NewLoopbackRTPTap(sip.NativeVideoRTPPort,
				sip.DefaultGStreamerConfig().VideoPort, logger)
			liveVideo := sip.NewLiveVideoOnCall(tap, rtspServer, logger)
			// Guard: el fin de llamada se detecta por tramas OWN (heurístico);
			// si se pierde, desarmar solo (el tap es pasivo, pero callActive
			// inhibiría el self-INVITE indefinidamente).
			liveVideo.SetGuardTimeout(5 * time.Minute)

			// Armado principal: doorbell.pressed viene del monitor OWN (fiable
			// en producción). Los sip.call.* se mantienen por si el cliente SIP
			// llega a registrarse (en producción el REGISTER local suele fallar
			// y esos eventos no se emiten). Start/Stop son idempotentes.
			eventBus.Subscribe("doorbell.pressed", func(_ *events.Event) {
				liveVideo.Start("doorbell.pressed")
			})
			// Auto-activación de cámara/micro de la app: no genera INVITE SIP,
			// solo canal-up nativo en el bus OWN (ver 2026-08-22-media-arm-on-channel-up).
			eventBus.Subscribe("media.channel.up", func(_ *events.Event) {
				liveVideo.Start("media.channel.up")
			})
			eventBus.Subscribe("call.hangup", func(_ *events.Event) {
				liveVideo.Stop()
			})
			eventBus.Subscribe("sip.call.incoming", func(_ *events.Event) {
				liveVideo.Start("sip.call.incoming")
			})
			eventBus.Subscribe("sip.call.connected", func(_ *events.Event) {
				liveVideo.Start("sip.call.connected")
			})
			eventBus.Subscribe("sip.call.ended", func(_ *events.Event) {
				liveVideo.Stop()
			})
			logger.Info("   Live video on call: habilitado (espejo pasivo lo:5007, cero comandos)")
		}

		// Audio en vivo de la puerta durante una llamada entrante, por espejo
		// PASIVO del leg de audio nativo (lo:5000, Speex ya casado con el códec
		// del relay) hacia el relay de audio del RTSP (10000). Espejo exacto de
		// liveVideo pero SIN CallActiveMarker: el audio no dispara self-INVITE,
		// es puramente pasivo. CERO comandos al bus. Fail-closed: nuevo, off por
		// defecto salvo que se active explícitamente (sdd 2026-08-21-b1-audio-to-ha).
		if cfg.Streaming.LiveAudioOnCall {
			audioTap := sip.NewLoopbackRTPTap(sip.NativeAudioRTPPort,
				sip.DefaultGStreamerConfig().AudioPort, logger)
			liveAudio := sip.NewLiveAudioOnCall(audioTap, logger)
			// Guard: mismo criterio que liveVideo — desarme automático si se
			// pierde el fin de llamada (heurístico OWN).
			liveAudio.SetGuardTimeout(5 * time.Minute)

			// Mismo armado que liveVideo: doorbell.pressed es el disparador
			// fiable en producción (monitor OWN); los sip.call.* se mantienen
			// por si el cliente SIP llega a registrarse. Start/Stop idempotentes.
			eventBus.Subscribe("doorbell.pressed", func(_ *events.Event) {
				liveAudio.Start("doorbell.pressed")
			})
			// Auto-activación de cámara/micro de la app: no genera INVITE SIP,
			// solo canal-up nativo en el bus OWN (ver 2026-08-22-media-arm-on-channel-up).
			eventBus.Subscribe("media.channel.up", func(_ *events.Event) {
				liveAudio.Start("media.channel.up")
			})
			eventBus.Subscribe("call.hangup", func(_ *events.Event) {
				liveAudio.Stop()
			})
			eventBus.Subscribe("sip.call.incoming", func(_ *events.Event) {
				liveAudio.Start("sip.call.incoming")
			})
			eventBus.Subscribe("sip.call.connected", func(_ *events.Event) {
				liveAudio.Start("sip.call.connected")
			})
			eventBus.Subscribe("sip.call.ended", func(_ *events.Event) {
				liveAudio.Stop()
			})
			logger.Info("   Live audio on call: habilitado (espejo pasivo lo:5000 → relay 10000)")
		}

		// Audio-listen en llamada: mismo patrón que el vídeo, espejo PASIVO
		// del leg de audio de bajada nativo (lo:5000, RTP Speex en claro —
		// ver NativeAudioRTPPort) decodificado a WAV en local. CERO comandos
		// al bus. Gateado por audio.listen_tap_enabled (fail-closed): sin el
		// flag, nada de esto se cablea (sdd 2026-08-21-audio-tap-5000).
		if cfg.Audio.ListenTapEnabled {
			const audioListenRelayPort = 11000 // dedicado, no colisiona con el relay de vídeo (10000/10002)
			const nativeAudioPayloadType = 110 // PT Speex del leg nativo (ver AudioProbe)
			tap := sip.NewLoopbackRTPTap(sip.NativeAudioRTPPort, audioListenRelayPort, logger)
			decoder := sip.NewAudioTapDecoder("/usr/local/bin/ffmpeg", logger)
			audioListen := sip.NewAudioListenOnCall(tap, decoder, audioListenRelayPort, nativeAudioPayloadType, cfg.Audio.ListenSinkPath, logger)
			// Guard: igual que liveVideo — si el fin de llamada se pierde
			// (heurístico OWN), el desarme automático evita que el tap/decoder
			// queden armados indefinidamente.
			audioListen.SetGuardTimeout(5 * time.Minute)

			// Mismo armado que liveVideo: doorbell.pressed es el disparador
			// fiable en producción (monitor OWN); los sip.call.* se mantienen
			// por si el cliente SIP llega a registrarse. Start/Stop idempotentes.
			eventBus.Subscribe("doorbell.pressed", func(_ *events.Event) {
				audioListen.Start("doorbell.pressed")
			})
			// Auto-activación de cámara/micro de la app: no genera INVITE SIP,
			// solo canal-up nativo en el bus OWN (ver 2026-08-22-media-arm-on-channel-up).
			eventBus.Subscribe("media.channel.up", func(_ *events.Event) {
				audioListen.Start("media.channel.up")
			})
			eventBus.Subscribe("call.hangup", func(_ *events.Event) {
				audioListen.Stop()
			})
			eventBus.Subscribe("sip.call.incoming", func(_ *events.Event) {
				audioListen.Start("sip.call.incoming")
			})
			eventBus.Subscribe("sip.call.connected", func(_ *events.Event) {
				audioListen.Start("sip.call.connected")
			})
			eventBus.Subscribe("sip.call.ended", func(_ *events.Event) {
				audioListen.Stop()
			})

			if webServer != nil {
				webServer.SetAudioListenController(audioListen)
				logger.Infof("   Audio-listen: POST/GET /api/audio/listen/{start,stop,status} (relay 127.0.0.1:%d)", audioListenRelayPort)
			}
			logger.Info("   Audio-listen (tap 5000) activado")
		}

		// Sonda de vídeo cooperativa (diagnóstico): un único *7*300, sin retry ni
		// self-INVITE. Endpoint gateado por POST + ?confirm=yes. Solo se cablea si
		// hay cliente OpenWebNet. Es seguro (un solo comando) aunque el vídeo esté
		// desactivado, por eso NO depende de VideoOnDemand.
		if webServer != nil && owClient != nil {
			ownForProbe := owClient
			webServer.SetVideoProbeFunc(func() (map[string]interface{}, []byte, error) {
				res, jpeg, err := sip.VideoProbe(ownForProbe, 8*time.Second, logger)
				if err != nil {
					return nil, nil, err
				}
				return map[string]interface{}{
					"ack":        res.Ack,
					"ack_error":  res.AckError,
					"frames":     res.Frames,
					"jpeg_bytes": res.JPEGBytes,
					"note":       res.Note,
				}, jpeg, nil
			})
			logger.Info("   Video probe: POST /api/video/probe?confirm=yes (diagnóstico, *7*300 único)")

			// Sonda de audio cooperativa (diagnóstico): un único *7*300 tipo 2,
			// mide el RTP Speex entrante. Igual de segura (un solo comando).
			webServer.SetAudioProbeFunc(func() (map[string]interface{}, error) {
				res, err := sip.AudioProbe(ownForProbe, 6*time.Second, logger)
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{
					"ack":          res.Ack,
					"ack_error":    res.AckError,
					"packets":      res.Packets,
					"bytes":        res.Bytes,
					"payload_type": res.PayloadType,
					"note":         res.Note,
				}, nil
			})
			logger.Info("   Audio probe: POST /api/audio/probe?confirm=yes (diagnóstico, *7*300 tipo audio)")
		}

		// Set recording path if configured
		if cfg.Streaming.RecordingPath != "" {
			rtspServer.SetRecordingPath(cfg.Streaming.RecordingPath)
			logger.Infof("   RTSP recording enabled: %s", cfg.Streaming.RecordingPath)
		}

		if err := rtspServer.Start(); err != nil {
			logger.WithError(err).Error("Failed to start RTSP server, continuing without RTSP")
			rtspServer = nil
		} else {
			logger.Infof("✅ RTSP: Enhanced server started on port %d", cfg.Streaming.RTSPPort)
			logger.Infof("   RTSP streams:")
			logger.Infof("     - rtsp://192.168.1.38:%d/doorbell (Full stream)", cfg.Streaming.RTSPPort)
			logger.Infof("     - rtsp://192.168.1.38:%d/doorbell-video (Video only)", cfg.Streaming.RTSPPort)
			logger.Infof("     - rtsp://192.168.1.38:%d/doorbell-recorder (HKSV recording)", cfg.Streaming.RTSPPort)

			// Servicio de snapshots JPEG bajo demanda (patrón cooperativo *7*300,
			// sin self-INVITE). Solo se cablea si la activación de vídeo está
			// habilitada y hay cliente OpenWebNet.
			if webServer != nil && owClient != nil && cfg.Streaming.VideoOnDemand {
				snapshotSvc := sip.NewSnapshotService(owClient, logger)
				webServer.SetSnapshotFunc(snapshotSvc.Capture)
				logger.Info("   Snapshot: capturas JPEG disponibles en GET /api/snapshot")

				// Cámara del timbre en HA: al sonar el timbre / entrar una llamada,
				// el MQTT bridge captura un snapshot cooperativo y lo publica.
				if mqttBridge != nil {
					mqttBridge.SetCameraCapture(func() ([]byte, error) {
						return snapshotSvc.Capture(12*time.Second, 0)
					})
					logger.Info("   Camera: entidad de cámara del timbre habilitada en Home Assistant")
				}
			}
		}

		// Create video stream manager only if SIP started AND on-demand video is
		// enabled. Este componente auto-arranca vídeo (*7*32) al pulsar el timbre,
		// así que por seguridad NO se crea si video_on_demand está desactivado.
		if sipStarted && sipClient != nil && cfg.Streaming.VideoOnDemand {
			videoManager = sip.NewVideoStreamManager(sipClient, owClient, eventBus, logger)

			if err := videoManager.Start(); err != nil {
				logger.WithError(err).Warn("Failed to start video manager")
				videoManager = nil
			} else {
				logger.Info("✅ Video: Stream manager started successfully")
			}
		}
	} else {
		logger.Info("Video/SIP system disabled in configuration")
	}

	// ==================== COMPONENT 4b: SIP Answerer (contestador, F1) ====================
	// Fuera del `if cfg.SIP.Enabled { ... }` para que compile igual con SIP off;
	// el gate de más arriba ya garantiza Answerer.Enabled ⟹ SIP.Enabled.
	var answerer *sip.Answerer
	var talkSender *sip.TalkSender
	if cfg.SIP.Answerer.Enabled {
		// La ruta del contestador la vigila startHealthWatchers (solo lectura,
		// periódica, con sensor de problema en HA); no bloquea el arranque.
		serverAddr := fmt.Sprintf("%s:%d", cfg.SIP.ServerHost, cfg.SIP.ServerPort) // 127.0.0.1:5060
		tr, err := sip.NewTCPTransport(serverAddr, logger)
		if err != nil {
			logger.Errorf("[ANSWERER] no se pudo conectar al Flexisip local (%s): %v — contestador deshabilitado", serverAddr, err)
		} else {
			answerer = sip.NewAnswerer(cfg.SIP.Answerer, cfg.SIP.Domain, tr, eventBus, logger)
			if err := answerer.Start(); err != nil {
				logger.Errorf("[ANSWERER] Start falló: %v", err)
				answerer = nil
			} else {
				if webServer != nil {
					webServer.SetAnswerer(answerer)
				} else {
					logger.Warn("[ANSWERER] web dashboard deshabilitado: sin API REST para contestar/colgar desde HA")
				}
				logger.Info("[ANSWERER] contestador SIP activo (bajo demanda desde HA)")

				// F2: talk-back desde HA (fail-closed por talk_enabled).
				if cfg.SIP.Answerer.TalkEnabled {
					ffmpegPath := "/usr/local/bin/ffmpeg" // ffmpeg estático del device (mismo binario que audio_sender/receiver/tap)
					talkSender = sip.NewTalkSender(cfg.SIP.Answerer, sip.NewExecRunner(), eventBus, logger,
						ffmpegPath, "127.0.0.1", 40004, 97)
					if err := talkSender.Start(); err != nil {
						logger.Errorf("[TALK] Start falló: %v", err)
						talkSender = nil
					} else {
						logger.Info("[TALK] talk-back activo (hablar desde HA durante la llamada)")
					}
				}
			}
		}
	}

	// ==================== COMPONENT 4c: PanelView ('Ver placa') ====================
	// Fail-closed: requiere streaming.panelview.enabled + DEVADDR, y SIP, OWN y RTSP vivos
	// (punteros nil dentro de una interfaz no serían nil: se evalúan aquí).
	if cfg.Streaming.PanelView.Enabled && cfg.Streaming.PanelView.DEVADDR != "" &&
		sipClient != nil && owClient != nil && rtspServer != nil {
		eyeReceiver := sip.NewAudioReceiver("/usr/local/bin/ffmpeg", logger) // modo forward (CaptureForward)
		eyeAudioPort := sip.DefaultGStreamerConfig().VideoPort + 102         // 10104: distinto del 10102 del AudioCall
		eyeVideoPort := sip.DefaultGStreamerConfig().VideoPort + 104         // 10106: vídeo del offer (no se consume)
		panelView := sip.NewPanelView(
			cfg.Streaming.PanelView,
			sipClient,
			eyeReceiver,
			&ownAdapter{client: owClient},
			rtspServer, // CallActiveChecker
			eventBus,
			"127.0.0.1",
			eyeAudioPort, eyeVideoPort,
			logger,
		)
		if err := panelView.Start(); err != nil {
			logger.WithError(err).Warn("panelview: no se pudo iniciar; deshabilitado")
		} else {
			logger.Info("panelview: 'Ver placa' activo")
		}
	}

	// ==================== COMPONENT 5: MQTT Bridge ====================
	if mqttBridge != nil {
		logger.Info("Starting MQTT Bridge for Home Assistant...")
		installSSHStarter(cfg, mqttBridge) // botón "Arrancar SSH" (si enable_system_buttons)
		if err := mqttBridge.Start(ctx); err != nil {
			logger.WithError(err).Error("Failed to start MQTT bridge, continuing without MQTT")
			mqttBridge = nil
		} else {
			logger.Infof("MQTT: Connected to Home Assistant at %s:%d", cfg.MQTT.Host, cfg.MQTT.Port)

			// Wire EventBus to MQTT bridge for event forwarding
			mqttBridge.SetEventBus(eventBus)
			logger.Info("MQTT: EventBus connected - events will be forwarded to MQTT")

			// Wire MessageParser to MQTT bridge for answering machine stats
			if messageParser != nil {
				mqttBridge.SetMessageParser(messageParser)
				logger.Info("MQTT: MessageParser connected - message stats will be published to HA")
			}

			// Wire CommandHandler for full 4-step voicemail protocol
			if commandHandler != nil {
				mqttBridge.SetCommandHandler(commandHandler)
				logger.Info("MQTT: CommandHandler connected - 4-step voicemail protocol enabled")
			}
		}
	} else {
		logger.Info("MQTT bridge disabled in configuration")
	}

	// ==================== COMPONENT 5b: Salud del equipo ====================
	// Comprobaciones periódicas de solo lectura (ruta del contestador, Flexisip)
	// con sensores de problema en HA, y vigilante opcional de SSH.
	stopHealth := func() {}
	if !*testMode {
		stopHealth = startHealthWatchers(cfg, mqttBridge, logger)
	}

	// ==================== CLOUD SURVIVAL (Pilar D) ====================
	// Supervivencia a la nube: neutraliza OTA + sinkhole, conmutable desde HA.
	// Fail-closed: solo actúa si cfg.Survival.Enabled y el flag lo permiten.
	if cfg.Survival.Enabled && *enableSurvival {
		sv := survival.New(cfg.Survival, survival.ExecRunner())
		if err := sv.Start(); err != nil {
			logger.WithError(err).Warn("Survival: autoaplicar al arranque falló")
		} else if cfg.Survival.AutoApply {
			logger.Info("Survival: supervivencia a la nube aplicada al arranque")
		}
		if mqttBridge != nil {
			prefix := cfg.MQTT.TopicPrefix
			topic, payload := sv.DiscoveryConfig(prefix)
			mqttBridge.Publish(topic, string(payload), true)
			mqttBridge.Subscribe(prefix+"/survival/set", func(p string) {
				st, err := sv.HandleCommand(p)
				if err != nil {
					logger.WithError(err).Warn("Survival: comando desde HA falló")
					return
				}
				mqttBridge.Publish(prefix+"/survival/state", st, true)
			})
			if st, err := sv.Status(); err == nil {
				mqttBridge.Publish(prefix+"/survival/state", st, true)
			}
			logger.Info("Survival: switch de supervivencia expuesto en Home Assistant")
		}
	}

	// Wire MQTT and LED status providers to web server for dashboard display
	if webServer != nil {
		if mqttBridge != nil {
			webServer.SetMQTTStatusProvider(mqttBridge)
			logger.Info("Web Dashboard: MQTT status provider connected")
		}
		webServer.SetLEDStatusFunc(readLEDStates)
		logger.Info("Web Dashboard: LED status reader connected")
	}

	// ==================== INITIAL DEVICE STATE QUERIES ====================
	// Consultar estado real del dispositivo al arranque (voicemail, timbre)
	if mqttBridge != nil {
		go func() {
			// Esperar 5s para que el bridge MQTT este completamente conectado
			time.Sleep(5 * time.Second)
			logger.Info("Consultando estado inicial del dispositivo...")

			mqttBridge.QueryBellStatus()
			time.Sleep(bticino.CommandRetryDelay)

			mqttBridge.QueryVoicemailStatus()
			time.Sleep(bticino.CommandRetryDelay)

			mqttBridge.QuerySystemInfo()

			logger.Info("Consulta de estado inicial completada")
		}()
	}

	// ==================== COMPONENT 5.5: Multicast Listener (239.255.76.67:7667) ====================
	logger.Info("Initializing multicast listener (BTicino syslog on 239.255.76.67:7667)...")
	multicastListener = multicast.NewMulticastListener(eventBus, logger)

	// Register OpenWebNet handler for OPEN system messages
	ownHandler := handlers.NewOpenWebNetHandler(eventBus, logger)
	multicastListener.RegisterHandler("OPEN", ownHandler)

	if err := multicastListener.Start(); err != nil {
		logger.WithError(err).Warn("Failed to start multicast listener (non-critical)")
		multicastListener = nil
	} else {
		logger.Info("Multicast: Listening on 239.255.76.67:7667 with OPEN handler")
	}

	// ==================== COMPONENT 5.6: UDP Proxy (40004 -> 4000) ====================
	var udpProxy *udpproxy.UDPProxy
	if cfg.UDPProxy.Enabled {
		logger.Infof("Inicializando UDP Proxy: puerto %d -> %d...", cfg.UDPProxy.ListenPort, cfg.UDPProxy.TargetPort)
		udpProxy = udpproxy.New(cfg.UDPProxy.ListenPort, cfg.UDPProxy.TargetPort, logger)
		if err := udpProxy.Start(); err != nil {
			logger.WithError(err).Warn("Fallo al iniciar UDP Proxy (no critico)")
			udpProxy = nil
		} else {
			logger.Infof("UDP Proxy: Reenviando %d -> %d", cfg.UDPProxy.ListenPort, cfg.UDPProxy.TargetPort)
		}
	} else {
		logger.Info("UDP Proxy deshabilitado en configuracion")
	}

	// ==================== COMPONENT 6: Input Monitor (Physical Buttons) ====================
	logger.Info("Initializing physical input monitor...")
	inputMonitor = input.NewInputMonitor(logger)

	// Gestos de tecla (corta / larga) -> entidad event "Teclas" de HA.
	// Solo observa los press/release; no cambia lo que el equipo hace con la tecla.
	keyGestures := input.NewGestureDetector(input.DefaultLongPressThreshold, func(key int, gesture string) {
		name := physicalKeyName(key, "")
		eventBus.PublishWithSource("button.gesture", map[string]interface{}{
			"key_code": key,
			"key_name": name,
			"gesture":  gesture,
		}, "input_monitor")
		if mqttBridge != nil {
			mqttBridge.PublishKeyEvent(name, gesture)
		}
	})

	// Síncrono y en orden: OnButtonPress va en goroutines y podría desordenar
	// pulsar/soltar.
	inputMonitor.OnKeyRaw = func(code int, value int32) { feedKeyGesture(keyGestures, code, value) }

	// Set button press callback -> publish to EventBus and MQTT
	inputMonitor.OnButtonPress = func(evt input.ButtonEvent) {

		logger.WithFields(logrus.Fields{
			"key":    evt.KeyName,
			"action": evt.Action,
			"device": evt.Device,
		}).Info("Physical button event detected")

		// Map key codes to BTicino button names
		buttonName := physicalKeyName(evt.Key, evt.KeyName)

		// Publish to EventBus
		eventBus.PublishWithSource("button.pressed", map[string]interface{}{
			"key_code":    evt.Key,
			"key_name":    buttonName,
			"action":      evt.Action,
			"device":      evt.Device,
			"raw_keyname": evt.KeyName,
		}, "input_monitor")

		// Direct MQTT publish for button press
		if mqttBridge != nil && evt.Action == "press" {
			mqttBridge.PublishButtonPress(evt.Key, buttonName, evt.Action)

			// Publicar al activity_log con descripcion legible
			var actDesc string
			switch evt.Key {
			case bticino.KeyCodeKey:
				actDesc = "Boton llave pulsado (abrir puerta)"
			case bticino.KeyCodeStar:
				actDesc = "Boton estrella pulsado (luz escalera)"
			case bticino.KeyCodeEye:
				actDesc = "Boton ojo pulsado (activar camara)"
			case bticino.KeyCodePhone:
				actDesc = "Boton telefono pulsado (comunicacion)"
			default:
				actDesc = fmt.Sprintf("Boton %s pulsado", buttonName)
			}
			mqttBridge.PublishActivityLog(actDesc, "button_press", buttonName)
		}
	}

	// Set screen change callback -> publish to EventBus
	inputMonitor.OnScreenChange = func(evt input.ScreenEvent) {
		logger.WithFields(logrus.Fields{
			"gpio":  evt.GPIO,
			"state": evt.State,
		}).Info("Screen state change detected")

		eventBus.PublishWithSource("screen.changed", map[string]interface{}{
			"gpio":   evt.GPIO,
			"active": evt.State,
		}, "input_monitor")
	}

	// Set touch event callback -> log for device discovery
	inputMonitor.OnTouchEvent = func(evt input.TouchEvent) {
		logger.WithFields(logrus.Fields{
			"device":   evt.Device,
			"action":   evt.Action,
			"x":        evt.X,
			"y":        evt.Y,
			"pressure": evt.Pressure,
		}).Info("Touch event detected (discovery)")
	}

	if err := inputMonitor.Start(); err != nil {
		logger.WithError(err).Warn("Failed to start input monitor (may not have /dev/input access)")
		inputMonitor = nil
	} else {
		logger.Info("Input Monitor: Monitoring /dev/input/event0 (keypad), event1 (touch), event2 (gpio)")
	}

	// ==================== COMPONENT 7: OpenWebNet Event Monitor (Port 20000) ====================
	if owClient != nil {
		logger.Info("Setting up OpenWebNet bus event monitor on port 20000...")

		owClient.OnMessage(func(cmd *openwebnet.Command) {
			logger.WithFields(logrus.Fields{
				"raw":   cmd.Raw,
				"who":   cmd.WHO,
				"what":  cmd.WHAT,
				"where": cmd.WHERE,
			}).Info("OpenWebNet bus event received from port 20000")

			// Publish all bus events to EventBus
			eventBus.PublishWithSource("openwebnet.event", map[string]interface{}{
				"raw":   cmd.Raw,
				"who":   cmd.WHO,
				"what":  cmd.WHAT,
				"where": cmd.WHERE,
			}, "openwebnet_monitor")

			// Detect doorbell events specifically
			// Doorbell pattern: WHO=8, WHAT contains doorbell activation
			// Known doorbell command: *8*1#1#4#21*16##
			if cmd.Raw == bticino.CmdDoorbellPress || (cmd.WHO == "8" && strings.Contains(cmd.WHAT, "1#1#4#21")) {
				logger.Info("DOORBELL PRESS DETECTED from bus!")

				eventBus.PublishWithSource("doorbell.pressed", map[string]interface{}{
					"raw":    cmd.Raw,
					"source": "openwebnet_bus",
					"who":    cmd.WHO,
					"what":   cmd.WHAT,
					"where":  cmd.WHERE,
				}, "openwebnet_monitor")

				// Direct MQTT publish for doorbell
				if mqttBridge != nil {
					mqttBridge.PublishDoorbellEvent("openwebnet_bus", cmd.Raw)
				}
			}

			// Detectar fin de llamada (colgar) en el bus: desarma el vídeo en
			// vivo (live_on_call). Cubre llamada entrante y auto-llamada.
			if cmd.Raw == bticino.CmdHangupIncoming || cmd.Raw == bticino.CmdHangupSelf || cmd.Raw == bticino.CmdHangupSelf420 {
				eventBus.PublishWithSource("call.hangup", map[string]interface{}{
					"raw": cmd.Raw,
				}, "openwebnet_monitor")
			}

			// Detectar subida de canal de media nativo (audio 5000 / vídeo 5007).
			// Es el disparador FIABLE del armado de las pistas de media en la
			// auto-activación de la app (que NO genera INVITE SIP; ver spec
			// 2026-08-22-media-arm-on-channel-up). Solo REACCIONA a lo que el
			// device anuncia en su bus — no se envía ningún *7*300 (Fase 0).
			if bticino.IsMediaChannelUp(cmd.Raw) {
				eventBus.PublishWithSource("media.channel.up", map[string]interface{}{
					"raw":   cmd.Raw,
					"who":   cmd.WHO,
					"what":  cmd.WHAT,
					"where": cmd.WHERE,
				}, "openwebnet_monitor")
			}

			// Detectar cambios de estado del timbre en tiempo real (eventos del bus)
			if cmd.Raw == "*#8**33*0##" {
				logger.Info("Estado timbre detectado en bus: muted (OFF)")
				if mqttBridge != nil {
					mqttBridge.UpdateBellState(false)
				}
			} else if cmd.Raw == "*#8**33*1##" {
				logger.Info("Estado timbre detectado en bus: unmuted (ON)")
				if mqttBridge != nil {
					mqttBridge.UpdateBellState(true)
				}
			}

			// Detectar cambios de estado del contestador en tiempo real
			// El dispositivo envia *8*91*## o *8*92*## (con WHERE vacio),
			// ademas de la forma sin WHERE *8*91## / *8*92##.
			// Usamos WHO=8 + WHAT=91/92 ya parseados para cubrir ambas variantes.
			if cmd.WHO == "8" && cmd.WHAT == "91" {
				logger.Info("Contestador activado detectado en bus")
				if mqttBridge != nil {
					mqttBridge.UpdateVoicemailState(true)
				}
			} else if cmd.WHO == "8" && cmd.WHAT == "92" {
				logger.Info("Contestador desactivado detectado en bus")
				if mqttBridge != nil {
					mqttBridge.UpdateVoicemailState(false)
				}
			}
			// Tambien detectar via respuesta de status *#8**40*...
			if cmd.WHO == "8" && strings.HasPrefix(cmd.Raw, "*#8**40*") {
				re := regexp.MustCompile(`\*#8\*\*40\*([01])\*`)
				if m := re.FindStringSubmatch(cmd.Raw); len(m) >= 2 {
					vmEnabled := m[1] == "1"
					logger.WithField("voicemail_enabled", vmEnabled).Info("Estado contestador detectado en bus via WHO=8/40")
					if mqttBridge != nil {
						mqttBridge.UpdateVoicemailState(vmEnabled)
					}
				}
			}

			// Detectar eventos de cerraduras adicionales (WHERE=21, 22)
			if cmd.WHO == "8" && (cmd.WHAT == "19" || cmd.WHAT == "20") {
				if cmd.WHERE != "20" && cmd.WHERE != "16" {
					// Es una cerradura adicional (no la principal WHERE=20 ni luz WHERE=16)
					logger.WithField("where", cmd.WHERE).Infof("Evento cerradura adicional detectado: WHAT=%s", cmd.WHAT)
					if mqttBridge != nil {
						if cmd.WHAT == "19" {
							mqttBridge.PublishAdditionalLockEvent(cmd.WHERE, "UNLOCKED")
						} else {
							mqttBridge.PublishAdditionalLockEvent(cmd.WHERE, "LOCKED")
						}
					}
				}
			}

			// Forward all events to MQTT
			if mqttBridge != nil {
				mqttBridge.PublishOpenWebNetEvent(cmd.Raw, cmd.WHO, cmd.WHAT, cmd.WHERE)

				// Reconocimiento de patrones OWN -> activity_log sensor
				if desc, ok := bticino.OWNEventDescriptions[cmd.Raw]; ok {
					eventType := "unknown"
					if et, ok2 := bticino.OWNEventTypes[cmd.Raw]; ok2 {
						eventType = et
					}
					mqttBridge.PublishActivityLog(desc, eventType, cmd.Raw)
				}
			}
		})

		logger.Info("OpenWebNet: Bus event monitor active on port 20000 - doorbell detection enabled")
	}

	// ==================== STARTUP COMPLETE ====================
	logger.Info("===== BTicino Bridge v" + versionpkg.GetVersion() + " STARTUP COMPLETE =====")
	logger.Info("Active Components:")

	if owClient != nil {
		logger.Info("  OpenWebNet: Connected (port 20000 monitor + port 30006 commands)")
		if commandHandler != nil {
			logger.Info("  Command Handler: Active (50+ BTicino commands)")
		}
	}

	if messageParser != nil {
		messages, err := messageParser.GetAllMessages()
		if err != nil {
			logger.WithError(err).Warn("  Message Parser: Error reading messages")
		} else {
			logger.Infof("  Message Parser: Active (%d messages found)", len(messages))
		}
	}

	if mqttBridge != nil {
		logger.Infof("  MQTT: Connected to %s:%d (39+%d HA entities, 8+%d command topics)", cfg.MQTT.Host, cfg.MQTT.Port, len(cfg.AdditionalLocks), len(cfg.AdditionalLocks))
	}

	if multicastListener != nil {
		logger.Info("  Multicast: Listening on 239.255.76.67:7667 (OPEN handler)")
	}

	if inputMonitor != nil {
		logger.Info("  Input Monitor: Active (keypad buttons, touchscreen, 13 GPIO pins)")
	}

	if udpProxy != nil {
		logger.Infof("  UDP Proxy: Reenviando puerto %d -> %d", cfg.UDPProxy.ListenPort, cfg.UDPProxy.TargetPort)
	}

	if webServer != nil {
		logger.Infof("  Web Dashboard: http://192.168.1.38:%d", cfg.Web.Port)
	}

	if homekitBridge != nil {
		logger.Infof("  HomeKit: Port %s, PIN: %s", cfg.HomeKit.Port, cfg.HomeKit.Pin)
	}

	if sipClient != nil {
		logger.Infof("  SIP/Video: %s:%d", cfg.SIP.ServerHost, cfg.SIP.ServerPort)
	}

	logger.Info("  EventBus: Active (component coordination)")
	logger.Info("===================================================")

	// Physical device monitoring goroutine (GPIO/LED monitoring + MQTT publishing)
	// Track previous states for change detection
	var prevLEDStates map[string]bool
	var prevGPIOStates map[int]bool
	var tickCount int
	go func() {
		physicalMonitorTicker := time.NewTicker(30 * time.Second)
		defer physicalMonitorTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-physicalMonitorTicker.C:
				tickCount++
				// Monitor real BTicino device physical status
				monitorFields := logrus.Fields{
					"physical_monitoring": true,
				}

				// Check LED status (real BTicino device paths) — with change detection
				if ledStatus := checkLEDStatus(); ledStatus != "" {
					monitorFields["led_status"] = ledStatus
				}
				currentLEDs := readLEDStates()
				if prevLEDStates != nil {
					for name, on := range currentLEDs {
						if prevOn, exists := prevLEDStates[name]; exists && prevOn != on {
							logger.WithFields(logrus.Fields{
								"led":  name,
								"from": boolToOnOff(prevOn),
								"to":   boolToOnOff(on),
							}).Info("LED state changed")
						}
					}
				}
				prevLEDStates = currentLEDs

				// Check GPIO status — with individual pin change detection
				if gpioStatus := checkGPIOStatus(); gpioStatus != "" {
					monitorFields["gpio_status"] = gpioStatus
				}
				if inputMonitor != nil {
					currentGPIO := inputMonitor.GetCurrentGPIOStates()
					if prevGPIOStates != nil {
						for pin, on := range currentGPIO {
							if prevOn, exists := prevGPIOStates[pin]; exists && prevOn != on {
								logger.WithFields(logrus.Fields{
									"gpio_pin": pin,
									"from":     boolToOnOff(prevOn),
									"to":       boolToOnOff(on),
								}).Info("GPIO pin state changed (periodic check)")
							}
						}
					}
					prevGPIOStates = currentGPIO

					// Publish GPIO states to MQTT individually
					if mqttBridge != nil && len(currentGPIO) > 0 {
						logger.WithField("gpio", currentGPIO).Info("Publishing GPIO states to MQTT")
						mqttBridge.PublishGPIOStates(currentGPIO)

						// Broadcast GPIO states to web UI via SSE
						if ws := webserver.GetGlobalWebServer(); ws != nil {
							ws.BroadcastEvent("gpio", map[string]interface{}{"gpio": currentGPIO})
						}
					}
				}

				// Check thermal status and publish to MQTT
				if thermalTemp := checkThermalStatus(); thermalTemp != "" {
					monitorFields["device_temperature"] = thermalTemp

					// Publish temperature to MQTT
					if mqttBridge != nil {
						var tempMilli int
						data, err := os.ReadFile(bticino.ThermalZonePath)
						if err == nil {
							tempStr := strings.TrimSpace(string(data))
							if _, err := fmt.Sscanf(tempStr, "%d", &tempMilli); err == nil {
								mqttBridge.PublishTemperature(float64(tempMilli) / 1000.0)
							}
						}
					}
				}

				// Check LED status and publish to MQTT
				if mqttBridge != nil && len(currentLEDs) > 0 {
					logger.WithField("leds", currentLEDs).Info("Publishing LED states to MQTT")
					mqttBridge.PublishLEDStatus(currentLEDs)

					// Broadcast LED states to web UI via SSE
					if ws := webserver.GetGlobalWebServer(); ws != nil {
						ws.BroadcastEvent("leds", map[string]interface{}{"leds": currentLEDs})
					}
				}

				// Publish answering machine message stats to MQTT
				if mqttBridge != nil {
					mqttBridge.PublishMessageStats()
				}

				// Publish SIP status (query via netcat on port 30006, NOT port 20000)
				if mqttBridge != nil {
					_, err := mqttBridge.ExecuteCommand(bticino.CmdBellQuery)
					sipOk := err == nil
					mqttBridge.PublishSIPStatus(sipOk)
					if sipOk {
						monitorFields["sip_status"] = "registered"
					} else {
						monitorFields["sip_status"] = "unregistered"
					}
				}

				// Publish system diagnostics (heartbeat via netcat)
				if mqttBridge != nil {
					_, err := mqttBridge.ExecuteCommand("*#130**1*2##")
					if err == nil {
						mqttBridge.PublishSystemDiagnostics("ok")
						monitorFields["system_diag"] = "ok"
					} else {
						mqttBridge.PublishSystemDiagnostics("error")
						monitorFields["system_diag"] = "error"
					}
				}

				// Publish multicast stats
				if mqttBridge != nil && multicastListener != nil {
					stats := multicastListener.GetStats()
					mqttBridge.PublishMulticastStats(stats.TotalMessages)

				}

				// Consultar estado del dispositivo cada 5 minutos (cada 10 ticks de 30s)
				if mqttBridge != nil && tickCount%10 == 0 {
					go func() {
						mqttBridge.QueryBellStatus()
						time.Sleep(bticino.CommandRetryDelay)
						mqttBridge.QueryVoicemailStatus()
					}()
				}

				// Check filesystem status for messages directory
				if fsStatus := checkBTicinoFilesystem(); fsStatus != "" {
					monitorFields["filesystem_status"] = fsStatus
				}

				logger.WithFields(monitorFields).Debug("Physical device monitoring")
			}
		}
	}()

	// Status monitoring goroutine
	go func() {
		statusTicker := time.NewTicker(5 * time.Minute)
		defer statusTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-statusTicker.C:
				logFields := logrus.Fields{
					"mega_bridge_version": versionpkg.GetVersion(),
				}

				// OpenWebNet stats
				if owClient != nil {
					logFields["openwebnet_connected"] = owClient.IsConnected()
					logFields["openwebnet_ports"] = "20000,30006,30007"
				}

				// Web server stats
				if webServer != nil {
					logFields["web_server_enabled"] = true
					logFields["web_server_port"] = cfg.Web.Port
					logFields["web_dashboard_url"] = fmt.Sprintf("http://192.168.1.38:%d", cfg.Web.Port)
				}

				// HomeKit stats
				if homekitBridge != nil {
					homekitStats := homekitBridge.GetStats()
					logFields["homekit_events_processed"] = homekitStats.EventsProcessed
					logFields["homekit_accessories"] = homekitStats.AccessoriesCount
					logFields["homekit_running_since"] = time.Since(homekitStats.StartTime).Round(time.Second).String()
				}

				// Video/SIP stats
				if sipClient != nil {
					logFields["sip_registered"] = sipClient.IsRegistered()
					logFields["sip_call_state"] = sipClient.GetCallState().String()
				}
				if videoManager != nil {
					videoStats := videoManager.GetStats()
					logFields["video_active_streams"] = videoStats["active_streams"]
					logFields["video_rtp_port"] = videoStats["rtp_port"]
				}
				if rtspServer != nil {
					rtspStats := rtspServer.GetStats()
					logFields["rtsp_active_sessions"] = rtspStats["active_sessions"]
					logFields["rtsp_port"] = rtspStats["port"]
				}

				logger.WithFields(logFields).Info("BTicino Bridge Status")
			}
		}
	}()

	logger.Info("🚀 BTicino Bridge is fully operational. Press Ctrl+C to stop...")
	if webServer != nil {
		logger.Infof("🌐 Access the web dashboard at: http://192.168.1.38:%d", cfg.Web.Port)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Infof("Received signal %v, shutting down BTicino Bridge...", sig)
	case <-ctx.Done():
		logger.Info("Context cancelled, shutting down BTicino Bridge...")
	}

	// ==================== GRACEFUL SHUTDOWN ====================
	logger.Info("🔄 Graceful shutdown of BTicino Bridge...")

	// Stop components in reverse order
	cancel()

	// Stop RTSP server
	if rtspServer != nil {
		logger.Info("Stopping RTSP server...")
		if err := rtspServer.Stop(); err != nil {
			logger.WithError(err).Error("Error stopping RTSP server")
		} else {
			logger.Info("✅ RTSP server stopped")
		}
	}

	// Stop video manager
	if videoManager != nil {
		logger.Info("Stopping video stream manager...")
		if err := videoManager.Stop(); err != nil {
			logger.WithError(err).Error("Error stopping video manager")
		} else {
			logger.Info("✅ Video manager stopped")
		}
	}

	// Stop talk-back (F2) antes que el contestador, para no dejar un ffmpeg
	// huérfano si el answerer cae primero.
	if talkSender != nil {
		logger.Info("Stopping talk-back sender...")
		talkSender.Stop()
		logger.Info("✅ Talk-back sender stopped")
	}

	// Stop SIP answerer (antes que el cliente SIP, para que nuestra pata mande
	// BYE primero si hay una llamada del contestador en curso).
	if answerer != nil {
		logger.Info("Stopping SIP answerer...")
		if err := answerer.Stop(); err != nil {
			logger.WithError(err).Error("Error stopping SIP answerer")
		} else {
			logger.Info("✅ SIP answerer stopped")
		}
	}

	// Stop SIP client
	if sipClient != nil {
		logger.Info("Stopping SIP client...")
		if err := sipClient.Stop(); err != nil {
			logger.WithError(err).Error("Error stopping SIP client")
		} else {
			logger.Info("✅ SIP client stopped")
		}
	}

	// Stop HomeKit bridge
	if homekitBridge != nil {
		logger.Info("Stopping HomeKit bridge...")
		if err := homekitBridge.Stop(); err != nil {
			logger.WithError(err).Error("Error stopping HomeKit bridge")
		} else {
			logger.Info("✅ HomeKit bridge stopped")
		}
	}

	// Stop web server
	if webServer != nil {
		logger.Info("Stopping web server...")
		if err := webServer.Stop(); err != nil {
			logger.WithError(err).Error("Error stopping web server")
		} else {
			logger.Info("Web server stopped")
		}
	}

	stopHealth()

	// Stop input monitor
	if inputMonitor != nil {
		logger.Info("Stopping input monitor...")
		if err := inputMonitor.Stop(); err != nil {
			logger.WithError(err).Error("Error stopping input monitor")
		} else {
			logger.Info("Input monitor stopped")
		}
	}

	// Stop MQTT bridge
	if mqttBridge != nil {
		logger.Info("Stopping MQTT bridge...")
		mqttBridge.Stop()
		logger.Info("MQTT bridge stopped")
	}

	// Stop multicast listener
	if multicastListener != nil {
		logger.Info("Stopping multicast listener...")
		if err := multicastListener.Stop(); err != nil {
			logger.WithError(err).Error("Error stopping multicast listener")
		} else {
			logger.Info("Multicast listener stopped")
		}
	}

	// Stop UDP proxy
	if udpProxy != nil {
		logger.Info("Deteniendo UDP Proxy...")
		if err := udpProxy.Stop(); err != nil {
			logger.WithError(err).Error("Error deteniendo UDP Proxy")
		} else {
			logger.Info("UDP Proxy detenido")
		}
	}

	// Stop OpenWebNet client
	if owClient != nil {
		logger.Info("Closing OpenWebNet connections...")
		owClient.Disconnect()
		logger.Info("✅ OpenWebNet client stopped")
	}

	logger.Info("🎉 ===== BTicino Bridge SHUTDOWN COMPLETE =====")
	logger.Info("Session Summary:")
	logger.Info("  - Enhanced OpenWebNet with 80+ commands: " + func() string {
		if owClient != nil {
			return "✅ ACTIVE"
		} else {
			return "❌ DISABLED"
		}
	}())
	logger.Info("  - Web Dashboard Interface: " + func() string {
		if webServer != nil {
			return "✅ ACTIVE"
		} else {
			return "❌ DISABLED"
		}
	}())
	logger.Info("  - HomeKit Apple Home Bridge: " + func() string {
		if homekitBridge != nil {
			return "✅ ACTIVE"
		} else {
			return "❌ DISABLED"
		}
	}())
	logger.Info("  - Video/SIP H.264 Streaming: " + func() string {
		if sipClient != nil || videoManager != nil || rtspServer != nil {
			return "✅ ACTIVE"
		} else {
			return "❌ DISABLED"
		}
	}())

	logger.Info("🏁 BTicino Bridge v" + versionpkg.GetVersion() + " shutdown successful!")
}

// Physical device monitoring functions for real BTicino hardware
func checkLEDStatus() string {
	// Read real LED brightness values from /sys/class/leds/*/brightness
	if _, err := os.Stat(bticino.LEDsPath); os.IsNotExist(err) {
		return "leds_unavailable"
	}

	entries, err := os.ReadDir(bticino.LEDsPath)
	if err != nil {
		return "leds_error"
	}

	active := 0
	for _, entry := range entries {
		brightnessPath := fmt.Sprintf("%s/%s/brightness", bticino.LEDsPath, entry.Name())
		data, err := os.ReadFile(brightnessPath)
		if err == nil {
			val := strings.TrimSpace(string(data))
			if val != "0" && val != "" {
				active++
			}
		}
	}
	return fmt.Sprintf("leds_ok: %d/%d active", active, len(entries))
}

func checkGPIOStatus() string {
	// Read real GPIO values from /sys/class/gpio/*/value
	if _, err := os.Stat(bticino.GPIOPath); os.IsNotExist(err) {
		return "gpio_unavailable"
	}

	entries, err := os.ReadDir(bticino.GPIOPath)
	if err != nil {
		return "gpio_error"
	}

	gpioCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "gpio") {
			gpioCount++
		}
	}
	return fmt.Sprintf("gpio_ok: %d pins exported", gpioCount)
}

func checkThermalStatus() string {
	// Read actual temperature from /sys/class/thermal/thermal_zone0/temp
	data, err := os.ReadFile(bticino.ThermalZonePath)
	if err != nil {
		return "thermal_unavailable"
	}

	tempStr := strings.TrimSpace(string(data))
	// Value is in millidegrees, convert to degrees
	var tempMilli int
	if _, err := fmt.Sscanf(tempStr, "%d", &tempMilli); err == nil {
		return fmt.Sprintf("%.1f C", float64(tempMilli)/1000.0)
	}
	return "thermal_read_error"
}

func checkBTicinoFilesystem() string {
	// Check BTicino filesystem paths availability
	paths := []string{
		bticino.MessagesDir,
		bticino.DeviceConfigXML,
		bticino.SystemConfigXML,
	}

	availableCount := 0
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			availableCount++
		}
	}

	if availableCount == len(paths) {
		return "full_integration_available"
	} else if availableCount > 0 {
		return fmt.Sprintf("partial_integration_%d_of_%d", availableCount, len(paths))
	}

	return "filesystem_unavailable"
}

// readLEDStates reads the brightness of all LEDs and returns a map of name -> on/off
func readLEDStates() map[string]bool {
	result := make(map[string]bool)

	if _, err := os.Stat(bticino.LEDsPath); os.IsNotExist(err) {
		return result
	}

	entries, err := os.ReadDir(bticino.LEDsPath)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		brightnessPath := fmt.Sprintf("%s/%s/brightness", bticino.LEDsPath, entry.Name())
		data, err := os.ReadFile(brightnessPath)
		if err == nil {
			val := strings.TrimSpace(string(data))
			result[entry.Name()] = val != "0" && val != ""
		}
	}
	return result
}

// boolToOnOff converts a boolean to "ON"/"OFF" for logging
func boolToOnOff(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}

// physicalKeyName traduce el código de una tecla física al nombre que usa el
// bridge ("llave", "estrella", "ojo", "telefono"). Para un código desconocido
// devuelve fallback o, si está vacío, "key_<código>".
func physicalKeyName(code int, fallback string) string {
	switch code {
	case bticino.KeyCodeKey:
		return "llave"
	case bticino.KeyCodeStar:
		return "estrella"
	case bticino.KeyCodeEye:
		return "ojo"
	case bticino.KeyCodePhone:
		return "telefono"
	}
	if fallback != "" {
		return fallback
	}
	return fmt.Sprintf("key_%d", code)
}

// feedKeyGesture pasa un EV_KEY crudo al detector de gestos. Solo cuentan las
// cuatro teclas físicas (la pantalla táctil y los GPIO también emiten EV_KEY) y
// solo pulsar (1) y soltar (0): la auto-repetición del kernel (2) se ignora.
func feedKeyGesture(d *input.GestureDetector, code int, value int32) {
	switch code {
	case bticino.KeyCodeKey, bticino.KeyCodeStar, bticino.KeyCodeEye, bticino.KeyCodePhone:
	default:
		return
	}
	switch value {
	case 1:
		d.Press(code)
	case 0:
		d.Release(code)
	}
}
