// Package deviceconfig proporciona funciones para publicar config del dispositivo via MQTT
package deviceconfig

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// MQTTPublisherInterface define la interfaz para publicar en MQTT
type MQTTPublisherInterface interface {
	Publish(topic, payload string, retain bool)
	IsConnected() bool
}

// DeviceMQTTPublisher publica la configuración del dispositivo en MQTT
type DeviceMQTTPublisher struct {
	logger      *logrus.Logger
	publisher   MQTTPublisherInterface
	topicPrefix string
	ticker      *time.Ticker
	stopCh      chan bool
}

// NewDeviceMQTTPublisher crea un nuevo publisher de config del dispositivo
func NewDeviceMQTTPublisher(logger *logrus.Logger, publisher MQTTPublisherInterface, topicPrefix string) *DeviceMQTTPublisher {
	if topicPrefix == "" {
		topicPrefix = "bticino"
	}
	return &DeviceMQTTPublisher{
		logger:      logger,
		publisher:   publisher,
		topicPrefix: topicPrefix,
		stopCh:      make(chan bool),
	}
}

// Start inicia la publicación periódica de config del dispositivo
func (p *DeviceMQTTPublisher) Start(interval time.Duration) {
	p.ticker = time.NewTicker(interval)
	p.logger.Info("Starting device config MQTT publisher")

	go func() {
		for {
			select {
			case <-p.ticker.C:
				p.publishAll()
			case <-p.stopCh:
				p.logger.Info("Device config MQTT publisher stopped")
				return
			}
		}
	}()
}

// Stop detiene la publicación
func (p *DeviceMQTTPublisher) Stop() {
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.stopCh <- true
}

// PublishAll publica toda la configuración del dispositivo
func (p *DeviceMQTTPublisher) PublishAll() {
	p.publishAll()
}

func (p *DeviceMQTTPublisher) publishAll() {
	if p.publisher == nil || !p.publisher.IsConnected() {
		return
	}

	// Leer configuración del dispositivo
	paths := struct {
		ConfXML      string
		ASWMSettings string
		TVCCSettings string
		SettingsXML  string
	}{
		ConfXML:      "/var/tmp/conf.xml",
		ASWMSettings: "/home/bticino/cfg/extra/47/aswm_settings.ini",
		TVCCSettings: "/home/bticino/cfg/extra/47/tvcc_settings.ini",
		SettingsXML:  "/home/bticino/cfg/extra/0/settings.xml",
	}

	cfg, err := ReadConfigFromPaths(paths)
	if err != nil {
		p.logger.WithError(err).Error("Failed to read device config for MQTT")
		return
	}

	// Publicar cada sección
	p.publishSystemConfig(cfg)
	p.publishAnsweringMachine(cfg)
	p.publishRingtones(cfg)
	p.publishVolumes(cfg)
	p.publishDisplay(cfg)
	p.publishCameras(cfg)
}

// publishSystemConfig publica la configuración del sistema
func (p *DeviceMQTTPublisher) publishSystemConfig(cfg *Config) {
	prefix := p.topicPrefix

	// Language
	p.publishJSON(prefix+"/system/language", map[string]interface{}{
		"language":  cfg.Language,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)

	// Timezone
	p.publishJSON(prefix+"/system/timezone", map[string]interface{}{
		"timezone":   cfg.Timezone,
		"ntp_server": cfg.NTPServer,
		"ntp_algo":   cfg.NTPAlgo,
		"timestamp":  time.Now().Format(time.RFC3339),
	}, true)

	// Datetime
	p.publishJSON(prefix+"/system/datetime", map[string]interface{}{
		"datetime":  cfg.DateTime.Format(time.RFC3339),
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)

	// Device info
	p.publishJSON(prefix+"/system/device", map[string]interface{}{
		"model":     cfg.DeviceInfo.Model,
		"ip":        cfg.DeviceInfo.IP,
		"version":   cfg.DeviceInfo.Version,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
}

// publishAnsweringMachine publica el estado del contestador
func (p *DeviceMQTTPublisher) publishAnsweringMachine(cfg *Config) {
	prefix := p.topicPrefix + "/answering"

	p.publishJSON(prefix+"/state", map[string]interface{}{
		"activated":    cfg.Answering.Activated,
		"ring_enabled": cfg.Answering.RingEnable,
		"led_enabled":  cfg.Answering.LedEnable,
		"memory_used":  cfg.Answering.MemoryUsed,
		"timestamp":    time.Now().Format(time.RFC3339),
	}, true)
}

// publishRingtones publica la configuración de timbres
func (p *DeviceMQTTPublisher) publishRingtones(cfg *Config) {
	prefix := p.topicPrefix + "/audio/ringtone"

	p.publishJSON(prefix+"/s0", map[string]interface{}{
		"value":     cfg.Ringtones.S0,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/s1", map[string]interface{}{
		"value":     cfg.Ringtones.S1,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/s2", map[string]interface{}{
		"value":     cfg.Ringtones.S2,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/door", map[string]interface{}{
		"value":     cfg.Ringtones.Door,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/external", map[string]interface{}{
		"value":     cfg.Ringtones.External,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/alarm", map[string]interface{}{
		"value":     cfg.Ringtones.Alarm,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/message", map[string]interface{}{
		"value":     cfg.Ringtones.Message,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
}

// publishVolumes publica la configuración de volúmenes
func (p *DeviceMQTTPublisher) publishVolumes(cfg *Config) {
	prefix := p.topicPrefix + "/audio/volume"

	p.publishJSON(prefix+"/s0", map[string]interface{}{
		"value":     cfg.Volumes.S0,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/s1", map[string]interface{}{
		"value":     cfg.Volumes.S1,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/s2", map[string]interface{}{
		"value":     cfg.Volumes.S2,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/intercom", map[string]interface{}{
		"value":     cfg.Volumes.Intercom,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/door", map[string]interface{}{
		"value":     cfg.Volumes.Door,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/sip", map[string]interface{}{
		"value":     cfg.Volumes.SIP,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
}

// publishDisplay publica la configuración del display
func (p *DeviceMQTTPublisher) publishDisplay(cfg *Config) {
	prefix := p.topicPrefix + "/display"

	p.publishJSON(prefix+"/brightness", map[string]interface{}{
		"value":     cfg.Display.Brightness,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
	p.publishJSON(prefix+"/clean_time", map[string]interface{}{
		"value":     cfg.Display.CleanTime,
		"timestamp": time.Now().Format(time.RFC3339),
	}, true)
}

// publishCameras publica la configuración de cámaras
func (p *DeviceMQTTPublisher) publishCameras(cfg *Config) {
	prefix := p.topicPrefix + "/camera"

	for id, camera := range cfg.Cameras {
		p.publishJSON(prefix+"/"+id+"/config", map[string]interface{}{
			"brightness": camera.Brightness,
			"contrast":   camera.Contrast,
			"saturation": camera.Saturation,
			"quality":    camera.Quality,
			"timestamp":  time.Now().Format(time.RFC3339),
		}, true)
	}
}

// publishJSON publica un objeto JSON
func (p *DeviceMQTTPublisher) publishJSON(topic string, data map[string]interface{}, retain bool) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		p.logger.WithError(err).WithField("topic", topic).Error("Failed to marshal JSON")
		return
	}
	p.publisher.Publish(topic, string(jsonBytes), retain)
}

// GetAllTopics retorna todos los topics que se publican
func (p *DeviceMQTTPublisher) GetAllTopics() []string {
	return []string{
		p.topicPrefix + "/system/language",
		p.topicPrefix + "/system/timezone",
		p.topicPrefix + "/system/datetime",
		p.topicPrefix + "/system/device",
		p.topicPrefix + "/answering/state",
		p.topicPrefix + "/audio/ringtone/s0",
		p.topicPrefix + "/audio/ringtone/s1",
		p.topicPrefix + "/audio/ringtone/s2",
		p.topicPrefix + "/audio/ringtone/door",
		p.topicPrefix + "/audio/ringtone/external",
		p.topicPrefix + "/audio/ringtone/alarm",
		p.topicPrefix + "/audio/ringtone/message",
		p.topicPrefix + "/audio/volume/s0",
		p.topicPrefix + "/audio/volume/s1",
		p.topicPrefix + "/audio/volume/s2",
		p.topicPrefix + "/audio/volume/intercom",
		p.topicPrefix + "/audio/volume/door",
		p.topicPrefix + "/audio/volume/sip",
		p.topicPrefix + "/display/brightness",
		p.topicPrefix + "/display/clean_time",
		// Cámaras dinámicas
	}
}

// VerifyTopicos verifica que los topics están siendo publicados (para testing)
func VerifyTopicos() {
	fmt.Println("Device Config MQTT Topics:")
	fmt.Println("- bticino/system/language")
	fmt.Println("- bticino/system/timezone")
	fmt.Println("- bticino/system/datetime")
	fmt.Println("- bticino/system/device")
	fmt.Println("- bticino/answering/state")
	fmt.Println("- bticino/audio/ringtone/s0")
	fmt.Println("- bticino/audio/volume/s0")
	fmt.Println("- bticino/display/brightness")
	fmt.Println("- bticino/camera/20/config")
}

// Ensure we implement the interface
var _ MQTTPublisherInterface = (*DeviceMQTTPublisher)(nil)

// Dummy implementation to satisfy the interface (actual implementation via real publisher)
func (p *DeviceMQTTPublisher) Publish(topic, payload string, retain bool) {
	if p.publisher != nil {
		p.publisher.Publish(topic, payload, retain)
	}
}

func (p *DeviceMQTTPublisher) IsConnected() bool {
	if p.publisher == nil {
		return false
	}
	return p.publisher.IsConnected()
}
