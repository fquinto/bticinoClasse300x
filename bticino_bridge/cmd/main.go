package main

import (
	"context"
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

	"bticino_bridge/pkg/config"
	"bticino_bridge/pkg/events"
	"bticino_bridge/pkg/homekit"
	"bticino_bridge/pkg/input"
	"bticino_bridge/pkg/openwebnet"
	"bticino_bridge/pkg/sip"
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

func main() {
	// Parse command line flags
	var (
		configPath       = flag.String("config", "configs/config.yaml", "Path to configuration file")
		logLevel         = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		version          = flag.Bool("version", false, "Show version information")
		testMode         = flag.Bool("test", false, "Run in test mode (no actual device connection)")
		webPort          = flag.Int("web-port", 0, "Override web server port (0 = use config)")
		enableOpenWebNet = flag.Bool("enable-openwebnet", true, "Enable OpenWebNet client")
		enableWeb        = flag.Bool("enable-web", true, "Enable web interface")
		enableHomeKit    = flag.Bool("enable-homekit", true, "Enable HomeKit bridge")
		enableVideo      = flag.Bool("enable-video", true, "Enable video/SIP system")
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
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Warnf("Failed to load config from %s, using defaults: %v", *configPath, err)
		cfg = config.GetDefaultConfig()
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

	logger.Infof("Configuration loaded: device=%s", cfg.OpenWebNet.Host)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create central EventBus for all components
	eventBus := events.NewEventBus(10, logger)
	logger.Info("✅ Central EventBus created for component communication")

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

		webServer = webserver.NewWebServer(cfg, simpleMQTT, logger, *configPath)
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

		domain := cfg.SIP.Domain
		if domain == "" {
			domain = "2617372.bs.iotleg.com"
		}

		username := cfg.SIP.Username
		if username == "" {
			username = "fquinto-gmx.com-1385B784-8A3B-4BF8-9037-A5FB2208B69A"
		}

		password := cfg.SIP.Password
		if password == "" {
			password = "f1d9d956df4aec9547fd8e10b94f97bd"
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
		}

		// Create Enhanced RTSP server - can work without SIP (for go2rtc integration)
		rtspServer = sip.NewEnhancedRTSPServer(cfg.Streaming.RTSPPort, sipClient, eventBus, logger)

		// Set OpenWebNet client for *7*300 video stream activation
		if owClient != nil {
			rtspServer.SetOpenWebNetClient(owClient)
			logger.Info("   RTSP: OpenWebNet client connected for video activation")
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

			// Servicio de snapshots JPEG bajo demanda (espejo del relay RTP + VPU)
			if webServer != nil {
				snapshotSvc := sip.NewSnapshotService(rtspServer, logger)
				webServer.SetSnapshotFunc(snapshotSvc.Capture)
				logger.Info("   Snapshot: capturas JPEG disponibles en GET /api/snapshot")
			}
		}

		// Create video stream manager only if SIP started
		if sipStarted && sipClient != nil {
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

	// ==================== COMPONENT 5: MQTT Bridge ====================
	if mqttBridge != nil {
		logger.Info("Starting MQTT Bridge for Home Assistant...")
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

	// Set button press callback -> publish to EventBus and MQTT
	inputMonitor.OnButtonPress = func(evt input.ButtonEvent) {
		logger.WithFields(logrus.Fields{
			"key":    evt.KeyName,
			"action": evt.Action,
			"device": evt.Device,
		}).Info("Physical button event detected")

		// Map key codes to BTicino button names
		buttonName := ""
		switch evt.Key {
		case bticino.KeyCodeKey:
			buttonName = "llave"
		case bticino.KeyCodeStar:
			buttonName = "estrella"
		case bticino.KeyCodeEye:
			buttonName = "ojo"
		case bticino.KeyCodePhone:
			buttonName = "telefono"
		default:
			buttonName = evt.KeyName
		}

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
