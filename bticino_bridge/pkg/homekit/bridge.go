package homekit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
	"github.com/sirupsen/logrus"

	"bticino_bridge/pkg/events"
)

// Config holds HomeKit bridge configuration
type Config struct {
	Name         string `yaml:"name"`
	Manufacturer string `yaml:"manufacturer"`
	Model        string `yaml:"model"`
	Port         string `yaml:"port"`
	Pin          string `yaml:"pin"`
	StoragePath  string `yaml:"storage_path"`
	Enabled      bool   `yaml:"enabled"`
}

// BticinoBridge represents the main HomeKit bridge for BTicino devices
type BticinoBridge struct {
	config      *Config
	eventBus    events.EventBus
	logger      *logrus.Logger
	server      *hap.Server
	bridge      *accessory.Bridge
	accessories []*accessory.A

	// Device accessories
	doorbell *DoorbellAccessory
	lock     *LockAccessory
	camera   *CameraAccessory

	// State tracking
	mu      sync.RWMutex
	running bool
	stats   BridgeStats

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// BridgeStats tracks HomeKit bridge statistics
type BridgeStats struct {
	StartTime         time.Time `json:"start_time"`
	EventsProcessed   int64     `json:"events_processed"`
	ActiveConnections int       `json:"active_connections"`
	AccessoriesCount  int       `json:"accessories_count"`
	LastEventTime     time.Time `json:"last_event_time"`
}

// NewBticinoBridge creates a new HomeKit bridge instance
func NewBticinoBridge(config *Config, eventBus events.EventBus, logger *logrus.Logger) (*BticinoBridge, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if eventBus == nil {
		return nil, fmt.Errorf("eventBus cannot be nil")
	}

	if logger == nil {
		logger = logrus.New()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create the main bridge accessory
	bridgeInfo := accessory.Info{
		Name:         config.Name,
		Manufacturer: config.Manufacturer,
		Model:        config.Model,
		SerialNumber: fmt.Sprintf("BTI-%d", time.Now().Unix()),
	}

	bridge := accessory.NewBridge(bridgeInfo)

	b := &BticinoBridge{
		config:      config,
		eventBus:    eventBus,
		logger:      logger,
		bridge:      bridge,
		accessories: make([]*accessory.A, 0),
		running:     false,
		stats: BridgeStats{
			StartTime:         time.Now(),
			EventsProcessed:   0,
			ActiveConnections: 0,
			AccessoriesCount:  0,
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize accessories
	if err := b.initializeAccessories(); err != nil {
		return nil, fmt.Errorf("failed to initialize accessories: %w", err)
	}

	// Setup event subscriptions
	b.setupEventSubscriptions()

	logger.WithFields(logrus.Fields{
		"name":         config.Name,
		"manufacturer": config.Manufacturer,
		"model":        config.Model,
		"port":         config.Port,
	}).Info("HomeKit bridge created successfully")

	return b, nil
}

// initializeAccessories creates and configures all HomeKit accessories
func (b *BticinoBridge) initializeAccessories() error {
	var err error

	// Create doorbell accessory
	b.doorbell, err = NewDoorbellAccessory(b.logger)
	if err != nil {
		return fmt.Errorf("failed to create doorbell accessory: %w", err)
	}
	b.accessories = append(b.accessories, b.doorbell.A)

	// Create lock accessory
	b.lock, err = NewLockAccessory(b.logger)
	if err != nil {
		return fmt.Errorf("failed to create lock accessory: %w", err)
	}
	b.accessories = append(b.accessories, b.lock.A)

	// Create camera accessory
	b.camera, err = NewCameraAccessory(b.logger)
	if err != nil {
		return fmt.Errorf("failed to create camera accessory: %w", err)
	}
	b.accessories = append(b.accessories, b.camera.Camera.A)

	b.stats.AccessoriesCount = len(b.accessories)

	b.logger.WithField("count", b.stats.AccessoriesCount).Info("Initialized HomeKit accessories")
	return nil
}

// setupEventSubscriptions sets up event listeners for BTicino events
func (b *BticinoBridge) setupEventSubscriptions() {
	// Subscribe to door events
	b.eventBus.SubscribePattern("door.*", func(eventName string, event *events.Event) {
		b.handleDoorEvent(eventName, event)
	})

	// Subscribe to doorbell events
	b.eventBus.SubscribePattern("doorbell.*", func(eventName string, event *events.Event) {
		b.handleDoorbellEvent(eventName, event)
	})

	// Subscribe to video events
	b.eventBus.SubscribePattern("video.*", func(eventName string, event *events.Event) {
		b.handleVideoEvent(eventName, event)
	})

	// Subscribe to audio events
	b.eventBus.SubscribePattern("audio.*", func(eventName string, event *events.Event) {
		b.handleAudioEvent(eventName, event)
	})

	b.logger.Info("HomeKit event subscriptions configured")
}

// Start starts the HomeKit bridge server
func (b *BticinoBridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return fmt.Errorf("bridge is already running")
	}

	if !b.config.Enabled {
		b.logger.Info("HomeKit bridge is disabled in configuration")
		return nil
	}

	// Create file system store for HAP data
	fs := hap.NewFsStore(b.config.StoragePath)

	// Create and start the server with bridge and accessories
	var err error
	b.server, err = hap.NewServer(fs, b.bridge.A, b.accessories...)
	if err != nil {
		return fmt.Errorf("failed to create HAP server: %w", err)
	}

	// Configure server
	b.server.Pin = b.config.Pin
	if b.config.Port != "" {
		b.server.Addr = ":" + b.config.Port
	}

	// Start the server in a goroutine
	go func() {
		b.logger.WithFields(logrus.Fields{
			"port":        b.config.Port,
			"pin":         b.config.Pin,
			"storage":     b.config.StoragePath,
			"accessories": b.stats.AccessoriesCount,
		}).Info("Starting HomeKit bridge server")

		if err := b.server.ListenAndServe(b.ctx); err != nil && err != context.Canceled {
			b.logger.WithError(err).Error("HomeKit server error")
		}
	}()

	// Setup periodic stats updates
	go b.statsUpdateLoop()

	b.running = true
	b.stats.StartTime = time.Now()

	b.logger.Info("HomeKit bridge started successfully")
	return nil
}

// Stop gracefully stops the HomeKit bridge
func (b *BticinoBridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return nil
	}

	b.logger.Info("Stopping HomeKit bridge")

	// Cancel context to stop all goroutines
	b.cancel()

	// Server will stop automatically when context is cancelled

	b.running = false
	b.logger.Info("HomeKit bridge stopped")
	return nil
}

// GetStats returns current bridge statistics
func (b *BticinoBridge) GetStats() BridgeStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := b.stats
	// Active connections are managed internally by HAP server
	stats.ActiveConnections = 0

	return stats
}

// IsRunning returns true if the bridge is currently running
func (b *BticinoBridge) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.running
}

// Event handlers

func (b *BticinoBridge) handleDoorEvent(eventName string, event *events.Event) {
	b.mu.Lock()
	b.stats.EventsProcessed++
	b.stats.LastEventTime = event.Timestamp
	b.mu.Unlock()

	b.logger.WithFields(logrus.Fields{
		"event": eventName,
		"data":  event.Data,
	}).Debug("Processing door event for HomeKit")

	if b.lock != nil {
		switch eventName {
		case "door.lock":
			b.lock.SetLocked(true)
		case "door.unlock":
			b.lock.SetLocked(false)
		}
	}
}

func (b *BticinoBridge) handleDoorbellEvent(eventName string, event *events.Event) {
	b.mu.Lock()
	b.stats.EventsProcessed++
	b.stats.LastEventTime = event.Timestamp
	b.mu.Unlock()

	b.logger.WithFields(logrus.Fields{
		"event": eventName,
		"data":  event.Data,
	}).Debug("Processing doorbell event for HomeKit")

	if b.doorbell != nil {
		switch eventName {
		case "doorbell.ring":
			if state, ok := event.Data.(string); ok && state == "ON" {
				b.doorbell.Ring()
			}
		}
	}
}

func (b *BticinoBridge) handleVideoEvent(eventName string, event *events.Event) {
	b.mu.Lock()
	b.stats.EventsProcessed++
	b.stats.LastEventTime = event.Timestamp
	b.mu.Unlock()

	b.logger.WithFields(logrus.Fields{
		"event": eventName,
		"data":  event.Data,
	}).Debug("Processing video event for HomeKit")

	if b.camera != nil {
		switch eventName {
		case "video.stream.started":
			if streamURL, ok := event.Data.(string); ok {
				b.camera.SetStreamURL(streamURL)
				b.camera.SetStreamingStatus(true)
			}
		case "video.stream.stopped":
			b.camera.SetStreamingStatus(false)
		}
	}
}

func (b *BticinoBridge) handleAudioEvent(eventName string, event *events.Event) {
	b.mu.Lock()
	b.stats.EventsProcessed++
	b.stats.LastEventTime = event.Timestamp
	b.mu.Unlock()

	b.logger.WithFields(logrus.Fields{
		"event": eventName,
		"data":  event.Data,
	}).Debug("Processing audio event for HomeKit")

	// Audio events can be used for additional HomeKit services if needed
}

// statsUpdateLoop periodically updates bridge statistics
func (b *BticinoBridge) statsUpdateLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			// Publish stats to event bus for monitoring
			stats := b.GetStats()
			b.eventBus.PublishWithSource("homekit.stats", stats, "homekit-bridge")
		}
	}
}

// GetAccessoryByType returns an accessory of the specified type
func (b *BticinoBridge) GetAccessoryByType(accessoryType string) interface{} {
	switch accessoryType {
	case "doorbell":
		return b.doorbell
	case "lock":
		return b.lock
	case "camera":
		return b.camera
	default:
		return nil
	}
}

// DefaultConfig returns a default HomeKit configuration
func DefaultConfig() *Config {
	return &Config{
		Name:         "BTicino Bridge",
		Manufacturer: "BTicino",
		Model:        "Class 300X",
		Port:         "8080",
		Pin:          "12345678",
		StoragePath:  "./homekit_data",
		Enabled:      true,
	}
}
