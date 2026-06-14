// Package webserver proporciona handlers API para gestión de configuración
package webserver

import (
	"encoding/json"
	"net/http"
	"time"
)

// @Summary Get bridge configuration
// @Description Returns the current bridge configuration
// @Tags Configuration
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/config [get]
// handleAPIConfig maneja GET /api/config - devuelve configuración actual
func (ws *WebServer) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := ws.configManager.GetConfig()
	ws.writeJSON(w, map[string]interface{}{
		"config":    config,
		"timestamp": time.Now(),
	})
}

// handleAPIConfigSave maneja POST /api/config - guarda configuración
func (ws *WebServer) handleAPIConfigSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Config map[string]interface{} `json:"config"`
		User   string                 `json:"user"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Convertir map a config.Config
	configJSON, err := json.Marshal(body.Config)
	if err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   "Failed to serialize config: " + err.Error(),
		})
		return
	}

	cfg, err := ws.configManager.FromJSON(configJSON)
	if err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse config: " + err.Error(),
		})
		return
	}

	// Validar configuración
	validation := ws.configManager.ValidateConfig(cfg)
	if !validation.Valid {
		ws.writeJSON(w, map[string]interface{}{
			"success":  false,
			"error":    "Validation failed",
			"errors":   validation.Errors,
			"warnings": validation.Warnings,
		})
		return
	}

	// Guardar configuración
	user := body.User
	if user == "" {
		user = "anonymous"
	}

	if err := ws.configManager.SaveConfig(cfg, user); err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   "Failed to save config: " + err.Error(),
		})
		return
	}

	// Solicitar reload si está habilitado
	ws.configManager.RequestReload()

	ws.writeJSON(w, map[string]interface{}{
		"success":          true,
		"message":          "Configuration saved successfully",
		"warnings":         validation.Warnings,
		"restart_required": true,
	})
}

// @Summary Save bridge configuration
// @Description Saves the bridge configuration and requests a reload
// @Tags Configuration
// @Accept json
// @Produce json
// @Param request body object true "Configuration object and user name"
// @Success 200 {object} map[string]interface{}
// @Router /api/config/save [post]
// handleAPIConfigValidate maneja POST /api/config/validate - valida configuración sin guardar
func (ws *WebServer) handleAPIConfigValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Config map[string]interface{} `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Convertir map a config.Config
	configJSON, err := json.Marshal(body.Config)
	if err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"valid": false,
			"error": "Failed to serialize config: " + err.Error(),
		})
		return
	}

	cfg, err := ws.configManager.FromJSON(configJSON)
	if err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"valid": false,
			"error": "Failed to parse config: " + err.Error(),
		})
		return
	}

	// Validar configuración
	validation := ws.configManager.ValidateConfig(cfg)

	ws.writeJSON(w, validation)
}

// @Summary Create configuration backup
// @Description Creates a backup of the current configuration
// @Tags Configuration
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/config/backup [post]
func (ws *WebServer) handleAPIConfigBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	backups, err := ws.configManager.GetBackups()
	if err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   "Failed to get backups: " + err.Error(),
		})
		return
	}

	ws.writeJSON(w, map[string]interface{}{
		"backups": backups,
		"count":   len(backups),
	})
}

// @Summary List configuration backups
// @Description Returns a list of available configuration backups
// @Tags Configuration
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/config/backups [get]
// handleAPIConfigRestore maneja POST /api/config/restore - restaura backup
func (ws *WebServer) handleAPIConfigRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		BackupFile string `json:"backup_file"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := ws.configManager.RestoreBackup(body.BackupFile); err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   "Failed to restore backup: " + err.Error(),
		})
		return
	}

	// Solicitar reload
	ws.configManager.RequestReload()

	ws.writeJSON(w, map[string]interface{}{
		"success":          true,
		"message":          "Backup restored successfully",
		"restart_required": true,
	})
}

// @Summary Restore configuration backup
// @Description Restores a configuration from a backup file
// @Tags Configuration
// @Accept json
// @Produce json
// @Param request body object true "Backup file name"
// @Success 200 {object} map[string]interface{}
// @Router /api/config/restore [post]
// handleAPIConfigHistory maneja GET /api/config/history - devuelve historial de cambios
func (ws *WebServer) handleAPIConfigHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	history := ws.configManager.GetHistory()

	ws.writeJSON(w, map[string]interface{}{
		"history": history,
		"count":   len(history),
	})
}

// @Summary Get configuration history
// @Description Returns the history of configuration changes
// @Tags Configuration
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/config/history [get]
// handleAPIConfigReload maneja POST /api/config/reload - solicita hot reload
func (ws *WebServer) handleAPIConfigReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws.configManager.RequestReload()

	ws.writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Configuration reload requested",
	})
}
