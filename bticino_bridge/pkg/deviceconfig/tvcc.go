// Package deviceconfig proporciona parsers para archivos de configuración del dispositivo BTicino
package deviceconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// TVCCSettings representa la estructura del archivo tvcc_settings.ini
type TVCCSettings map[string]TVCCConfig

// TVCCConfig configuración de una cámara específica
type TVCCConfig struct {
	Brightness int `json:"brightness"`
	Contrast   int `json:"contrast"`
	Saturation int `json:"saturation"`
	Quality    int `json:"quality"`
}

// ReadTVCCSettings lee y parsea el archivo tvcc_settings.ini
func ReadTVCCSettings(path string) (TVCCSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	settings := make(TVCCSettings)
	currentSection := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Ignorar comentarios y líneas vacías
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Detectar sección [TVCC XX]
		if strings.HasPrefix(line, "[") && strings.HasPrefix(line, "[TVCC") {
			currentSection = strings.Trim(line, "[]")
			settings[currentSection] = TVCCConfig{
				Brightness: 50,
				Contrast:   50,
				Saturation: 50,
				Quality:    75,
			}
			continue
		}

		// Parsear key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || currentSection == "" {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value, _ := strconv.Atoi(strings.TrimSpace(parts[1]))

		cfg := settings[currentSection]
		switch key {
		case "Brightness":
			cfg.Brightness = value
		case "Contrast":
			cfg.Contrast = value
		case "Saturation":
			cfg.Saturation = value
		case "Quality":
			cfg.Quality = value
		}
		settings[currentSection] = cfg
	}

	return settings, nil
}

// GetCameraConfig retorna la configuración de una cámara específica
func GetCameraConfig(path, cameraID string) (TVCCConfig, error) {
	settings, err := ReadTVCCSettings(path)
	if err != nil {
		return TVCCConfig{}, err
	}

	cameraKey := fmt.Sprintf("TVCC %s", cameraID)
	if cfg, ok := settings[cameraKey]; ok {
		return cfg, nil
	}

	return TVCCConfig{}, fmt.Errorf("camera %s not found", cameraID)
}

// GetAllCameraIDs retorna todos los IDs de cámaras configuradas
func GetAllCameraIDs(path string) ([]string, error) {
	settings, err := ReadTVCCSettings(path)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(settings))
	for key := range settings {
		// Extraer ID de "TVCC XX"
		if len(key) > 5 {
			ids = append(ids, key[5:])
		}
	}

	return ids, nil
}

// TVCCGlobalSettings configuración global de TVCC
type TVCCGlobalSettings struct {
	Settings TVCCSettings `json:"settings"`
}

// ReadTVCCSettingsWithGlobal lee el archivo con la sección [Settings]
func ReadTVCCSettingsWithGlobal(path string) (*TVCCGlobalSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	result := &TVCCGlobalSettings{
		Settings: make(TVCCSettings),
	}

	currentSection := "global"

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			section := strings.Trim(line, "[]")
			if section == "Settings" {
				currentSection = "global"
			} else if strings.HasPrefix(section, "TVCC") {
				currentSection = section
				result.Settings[currentSection] = TVCCConfig{
					Brightness: 50,
					Contrast:   50,
					Saturation: 50,
					Quality:    75,
				}
			}
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value, _ := strconv.Atoi(strings.TrimSpace(parts[1]))

		if currentSection != "global" {
			cfg := result.Settings[currentSection]
			switch key {
			case "Brightness":
				cfg.Brightness = value
			case "Contrast":
				cfg.Contrast = value
			case "Saturation":
				cfg.Saturation = value
			case "Quality":
				cfg.Quality = value
			}
			result.Settings[currentSection] = cfg
		}
	}

	return result, nil
}
