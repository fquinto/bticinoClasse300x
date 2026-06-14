// Package webserver proporciona gestión de configuración para el web dashboard
package webserver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"bticino_bridge/pkg/config"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// ConfigManager gestiona la configuración del bridge con soporte para backup y hot reload
type ConfigManager struct {
	configPath    string
	backupDir     string
	maxBackups    int
	config        *config.Config
	configMutex   sync.RWMutex
	history       []ConfigChange
	historyMutex  sync.RWMutex
	logger        *logrus.Logger
	autoReload    bool
	reloadChan    chan bool
}

// ConfigChange representa un cambio en la configuración
type ConfigChange struct {
	Timestamp   time.Time              `json:"timestamp"`
	User        string                 `json:"user,omitempty"`
	Sections    []string               `json:"sections"`
	Changes     map[string]interface{} `json:"changes,omitempty"`
	BackupFile  string                 `json:"backup_file"`
	Restarted   bool                   `json:"restarted"`
	Success     bool                   `json:"success"`
	Error       string                 `json:"error,omitempty"`
}

// ConfigValidationResult representa el resultado de una validación
type ConfigValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// NewConfigManager crea un nuevo gestor de configuración
func NewConfigManager(configPath string, logger *logrus.Logger) *ConfigManager {
	backupDir := filepath.Join(filepath.Dir(configPath), "backups")
	
	logger.Infof("ConfigManager initialized with config path: %s", configPath)
	
	return &ConfigManager{
		configPath:  configPath,
		backupDir:   backupDir,
		maxBackups:  10,
		config:      nil,
		history:     make([]ConfigChange, 0),
		logger:      logger,
		autoReload:  false,
		reloadChan:  make(chan bool, 1),
	}
}

// LoadConfig carga la configuración desde el archivo YAML
func (cm *ConfigManager) LoadConfig() (*config.Config, error) {
	cm.configMutex.Lock()
	defer cm.configMutex.Unlock()

	data, err := ioutil.ReadFile(cm.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &config.Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	cm.config = cfg
	cm.logger.Infof("Configuration loaded from %s", cm.configPath)
	return cfg, nil
}

// GetConfig devuelve la configuración actual
func (cm *ConfigManager) GetConfig() *config.Config {
	cm.configMutex.RLock()
	defer cm.configMutex.RUnlock()
	return cm.config
}

// SaveConfig guarda la configuración en el archivo YAML
func (cm *ConfigManager) SaveConfig(cfg *config.Config, user string) error {
	cm.configMutex.Lock()
	defer cm.configMutex.Unlock()

	// Crear backup antes de guardar
	backupFile, err := cm.createBackup()
	if err != nil {
		cm.logger.WithError(err).Warn("Failed to create backup, proceeding anyway")
	}

	// Serializar a YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Escribir archivo
	if err := ioutil.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Actualizar configuración en memoria
	oldConfig := cm.config
	cm.config = cfg

	// Registrar cambio
	sections := cm.detectChanges(oldConfig, cfg)
	cm.addHistoryEntry(ConfigChange{
		Timestamp:  time.Now(),
		User:       user,
		Sections:   sections,
		BackupFile: backupFile,
		Restarted:  false,
		Success:    true,
	})

	cm.logger.Infof("Configuration saved to %s (backup: %s)", cm.configPath, backupFile)
	return nil
}

// ValidateConfig valida la configuración
func (cm *ConfigManager) ValidateConfig(cfg *config.Config) ConfigValidationResult {
	result := ConfigValidationResult{
		Valid:    true,
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
	}

	// Validar configuración básica
	if cfg.Bridge.Name == "" {
		result.Errors = append(result.Errors, "Bridge name is required")
		result.Valid = false
	}

	// Validar OpenWebNet
	if cfg.OpenWebNet.Port <= 0 || cfg.OpenWebNet.Port > 65535 {
		result.Errors = append(result.Errors, "OpenWebNet port must be between 1-65535")
		result.Valid = false
	}

	// Validar Web
	if cfg.Web.Port <= 0 || cfg.Web.Port > 65535 {
		result.Errors = append(result.Errors, "Web port must be between 1-65535")
		result.Valid = false
	}

	if cfg.Web.Port == cfg.OpenWebNet.Port {
		result.Errors = append(result.Errors, "Web port and OpenWebNet port cannot be the same")
		result.Valid = false
	}

	// Validar SIP (warnings, no errors)
	if cfg.SIP.Enabled {
		if cfg.SIP.ServerHost == "" {
			result.Warnings = append(result.Warnings, "SIP enabled but no server host configured")
		}
		if cfg.SIP.Username == "" {
			result.Warnings = append(result.Warnings, "SIP enabled but no username configured")
		}
	}

	// Validar MQTT
	if cfg.MQTT.Enabled {
		if cfg.MQTT.Host == "" {
			result.Errors = append(result.Errors, "MQTT enabled but no host configured")
			result.Valid = false
		}
	}

	// Validar Streaming (si está presente)
	if cfg.Streaming.RTSPPort != 0 {
		if cfg.Streaming.RTSPPort <= 0 || cfg.Streaming.RTSPPort > 65535 {
			result.Errors = append(result.Errors, "RTSP port must be between 1-65535")
			result.Valid = false
		}
	}

	// Validar HomeKit
	if cfg.HomeKit.Enabled {
		if len(cfg.HomeKit.Pin) != 8 {
			result.Warnings = append(result.Warnings, "HomeKit PIN should be 8 digits")
		}
	}

	// Validar servidores externos (privacy warnings)
	if cfg.Servers.Cloud.Enabled {
		result.Warnings = append(result.Warnings, "Cloud connection enabled - data will be sent to Netatmo servers")
	}

	if cfg.Servers.Logging.Enabled {
		result.Warnings = append(result.Warnings, "Remote logging enabled - telemetry will be sent to BTicino servers")
	}

	return result
}

// CreateBackup crea un backup de la configuración actual
func (cm *ConfigManager) CreateBackup() (string, error) {
	cm.configMutex.RLock()
	defer cm.configMutex.RUnlock()
	return cm.createBackup()
}

func (cm *ConfigManager) createBackup() (string, error) {
	// Crear directorio de backups si no existe
	if err := os.MkdirAll(cm.backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Leer archivo actual
	data, err := ioutil.ReadFile(cm.configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	// Generar nombre de archivo con timestamp
	timestamp := time.Now().Format("20060102_150405")
	backupFile := filepath.Join(cm.backupDir, fmt.Sprintf("config_%s.yaml", timestamp))

	// Escribir backup
	if err := ioutil.WriteFile(backupFile, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	// Limpiar backups antiguos
	cm.cleanupOldBackups()

	cm.logger.Infof("Backup created: %s", backupFile)
	return backupFile, nil
}

// RestoreBackup restaura un backup
func (cm *ConfigManager) RestoreBackup(backupFile string) error {
	cm.configMutex.Lock()
	defer cm.configMutex.Unlock()

	// Verificar que el backup existe
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("backup file does not exist: %s", backupFile)
	}

	// Leer backup
	data, err := ioutil.ReadFile(backupFile)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

	// Crear backup de la configuración actual antes de restaurar
	_, err = cm.createBackup()
	if err != nil {
		cm.logger.WithError(err).Warn("Failed to create pre-restore backup")
	}

	// Escribir backup sobre config actual
	if err := ioutil.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to restore config file: %w", err)
	}

	// Recargar configuración
	cfg := &config.Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse restored config: %w", err)
	}

	cm.config = cfg

	cm.addHistoryEntry(ConfigChange{
		Timestamp:  time.Now(),
		Sections:   []string{"restore"},
		BackupFile: backupFile,
		Restarted:  false,
		Success:    true,
	})

	cm.logger.Infof("Configuration restored from %s", backupFile)
	return nil
}

// GetHistory devuelve el historial de cambios
func (cm *ConfigManager) GetHistory() []ConfigChange {
	cm.historyMutex.RLock()
	defer cm.historyMutex.RUnlock()

	// Devolver copia para evitar race conditions
	history := make([]ConfigChange, len(cm.history))
	copy(history, cm.history)
	return history
}

// GetBackups devuelve la lista de backups disponibles
func (cm *ConfigManager) GetBackups() ([]string, error) {
	files, err := ioutil.ReadDir(cm.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	// Filtrar y ordenar archivos YAML
	var backups []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".yaml") {
			backups = append(backups, filepath.Join(cm.backupDir, file.Name()))
		}
	}

	// Ordenar por nombre (timestamp)
	sort.Strings(backups)

	return backups, nil
}

// RequestReload solicita un hot reload de la configuración
func (cm *ConfigManager) RequestReload() {
	if cm.autoReload {
		select {
		case cm.reloadChan <- true:
			cm.logger.Info("Configuration reload requested")
		default:
			cm.logger.Warn("Reload channel full, ignoring request")
		}
	}
}

// GetReloadChannel devuelve el canal de reload
func (cm *ConfigManager) GetReloadChannel() <-chan bool {
	return cm.reloadChan
}

// EnableAutoReload habilita el hot reload automático
func (cm *ConfigManager) EnableAutoReload() {
	cm.autoReload = true
}

// CleanupOldBackups elimina backups antiguos manteniendo solo los últimos maxBackups
func (cm *ConfigManager) cleanupOldBackups() {
	backups, err := cm.GetBackups()
	if err != nil {
		cm.logger.WithError(err).Warn("Failed to get backups for cleanup")
		return
	}

	// Eliminar backups si exceden el máximo
	for len(backups) > cm.maxBackups {
		oldFile := backups[0]
		if err := os.Remove(oldFile); err != nil {
			cm.logger.WithError(err).Warnf("Failed to delete old backup: %s", oldFile)
		} else {
			cm.logger.Infof("Deleted old backup: %s", oldFile)
		}
		backups = backups[1:]
	}
}

// addHistoryEntry agrega una entrada al historial
func (cm *ConfigManager) addHistoryEntry(entry ConfigChange) {
	cm.historyMutex.Lock()
	defer cm.historyMutex.Unlock()

	// Limitar historial a 100 entradas
	if len(cm.history) >= 100 {
		cm.history = cm.history[1:]
	}

	cm.history = append(cm.history, entry)
}

// detectChanges detecta qué secciones cambiaron
func (cm *ConfigManager) detectChanges(old, new *config.Config) []string {
	var sections []string

	if old == nil || new == nil {
		return []string{"unknown"}
	}

	// Comparar secciones principales
	if old.Bridge.Name != new.Bridge.Name {
		sections = append(sections, "bridge")
	}

	if old.OpenWebNet.Port != new.OpenWebNet.Port ||
		old.OpenWebNet.Host != new.OpenWebNet.Host {
		sections = append(sections, "openwebnet")
	}

	if old.SIP.ServerHost != new.SIP.ServerHost ||
		old.SIP.ServerPort != new.SIP.ServerPort ||
		old.SIP.Username != new.SIP.Username {
		sections = append(sections, "sip")
	}

	if old.MQTT.Host != new.MQTT.Host ||
		old.MQTT.Port != new.MQTT.Port {
		sections = append(sections, "mqtt")
	}

	if old.Web.Port != new.Web.Port {
		sections = append(sections, "web")
	}

	if old.Streaming.RTSPPort != new.Streaming.RTSPPort {
		sections = append(sections, "streaming")
	}

	if old.HomeKit.Pin != new.HomeKit.Pin {
		sections = append(sections, "homekit")
	}

	if len(sections) == 0 {
		sections = append(sections, "minor")
	}

	return sections
}

// ToJSON serializa la configuración a JSON
func (cm *ConfigManager) ToJSON() ([]byte, error) {
	cm.configMutex.RLock()
	defer cm.configMutex.RUnlock()

	return json.MarshalIndent(cm.config, "", "  ")
}

// FromJSON deserializa la configuración desde JSON
func (cm *ConfigManager) FromJSON(data []byte) (*config.Config, error) {
	cfg := &config.Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}
	return cfg, nil
}
