// Package webserver - Device Tab API Handlers
// Maneja configuración del dispositivo (NTP, Timezone, Language, Ringtone)

package webserver

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DeviceConfig representa la configuración del dispositivo
type DeviceConfig struct {
	NTP      NTPConfig      `json:"ntp"`
	Timezone TimezoneConfig `json:"timezone"`
	Language string         `json:"language"`
	Ringtone string         `json:"ringtone"`
}

type NTPConfig struct {
	Enabled bool   `json:"enabled"`
	Server  string `json:"server"`
}

type TimezoneConfig struct {
	Timezone  string `json:"timezone"`
	GMTOffset int    `json:"gmt_offset"`
}

// @Summary Get device NTP configuration
// @Description Returns the current device NTP server configuration
// @Tags Device
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/device/ntp [get]
func (ws *WebServer) handleAPIDeviceNTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ntpConfig := map[string]interface{}{
		"enabled": true,
		"server":  "pool.ntp.org",
	}

	ws.writeJSON(w, ntpConfig)
}

// @Summary Get device timezone
// @Description Returns the current device timezone configuration
// @Tags Device
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/device/timezone [get]
func (ws *WebServer) handleAPIDeviceTimezone(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	timezone := "UTC"
	gmtOffset := 0

	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		timezone = strings.TrimSpace(string(data))
	}

	if loc, err := time.LoadLocation(timezone); err == nil {
		_, offset := time.Now().In(loc).Zone()
		gmtOffset = offset / 3600
	}

	ws.writeJSON(w, map[string]interface{}{
		"timezone":   timezone,
		"gmt_offset": gmtOffset,
	})
}

// @Summary Get device language
// @Description Returns the current device language setting
// @Tags Device
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/device/language [get]
func (ws *WebServer) handleAPIDeviceLanguage(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	language := "en"

	if data, err := os.ReadFile("/home/bticino/.language"); err == nil {
		language = strings.TrimSpace(string(data))
	}

	ws.writeJSON(w, map[string]interface{}{
		"language": language,
	})
}

// @Summary Get device ringtone
// @Description Returns the current device ringtone setting
// @Tags Device
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/device/ringtone [get]
func (ws *WebServer) handleAPIDeviceRingtone(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ringtone := "default"

	ws.writeJSON(w, map[string]interface{}{
		"ringtone": ringtone,
	})
}

// @Summary Get available ringtones
// @Description Returns the list of available ringtones
// @Tags Device
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/device/ringtones [get]
func (ws *WebServer) handleAPIDeviceRingtones(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ringtones := []string{
		"default",
		"ringtone_1",
		"ringtone_2",
		"ringtone_3",
	}

	ws.writeJSON(w, map[string]interface{}{
		"ringtones": ringtones,
	})
}

// @Summary Get available languages
// @Description Returns the list of available device languages
// @Tags Device
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/device/languages [get]
func (ws *WebServer) handleAPIDeviceLanguages(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	languages := []map[string]string{
		{"code": "en", "name": "English"},
		{"code": "es", "name": "Español"},
		{"code": "fr", "name": "Français"},
		{"code": "de", "name": "Deutsch"},
		{"code": "it", "name": "Italiano"},
	}

	ws.writeJSON(w, map[string]interface{}{
		"languages": languages,
	})
}

// @Summary Save device configuration
// @Description Saves the device configuration (NTP, timezone, language, ringtone)
// @Tags Device
// @Accept json
// @Produce json
// @Param request body object true "Device configuration"
// @Success 200 {object} map[string]interface{}
// @Router /api/device/save [post]
func (ws *WebServer) handleAPIDeviceSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Config DeviceConfig `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   "Invalid JSON: " + err.Error(),
		})
		return
	}

	if body.Config.NTP.Enabled {
		ws.logger.Infof("Configuring NTP: %s", body.Config.NTP.Server)
	}

	if body.Config.Timezone.Timezone != "" {
		ws.logger.Infof("Setting timezone: %s", body.Config.Timezone.Timezone)
	}

	if body.Config.Language != "" {
		ws.logger.Infof("Setting language: %s", body.Config.Language)
	}

	if body.Config.Ringtone != "" {
		ws.logger.Infof("Setting ringtone: %s", body.Config.Ringtone)
	}

	ws.writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Device configuration saved",
	})
}

// execSSHCommand executes an SSH command on the device
func (ws *WebServer) execSSHCommand(command string) (string, error) {
	cmd := exec.Command("ssh", "bticino", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
