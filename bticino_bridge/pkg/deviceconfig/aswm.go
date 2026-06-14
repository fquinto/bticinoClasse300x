// Package deviceconfig proporciona parsers para archivos de configuración del dispositivo BTicino
package deviceconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ASWMSettings representa la estructura del archivo aswm_settings.ini
type ASWMSettings struct {
	Activated  int    `json:"activated"`
	RingEnable int    `json:"ring_enable"`
	LedEnable  int    `json:"led_enable"`
	Memused    string `json:"memory_used"`
	Type       int    `json:"type"`
	Fps        int    `json:"fps"`
}

// ReadASWMSettings lee y parsea el archivo aswm_settings.ini
func ReadASWMSettings(path string) (*ASWMSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	aswm := &ASWMSettings{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Ignorar comentarios y líneas vacías
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parsear secciones [Answering Machine] y [Volumes]
		if strings.HasPrefix(line, "[") {
			continue
		}

		// Parsear key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Activated":
			aswm.Activated, _ = strconv.Atoi(value)
		case "RingEnable":
			aswm.RingEnable, _ = strconv.Atoi(value)
		case "LedEnable":
			aswm.LedEnable, _ = strconv.Atoi(value)
		case "Memused":
			aswm.Memused = value
		case "Type":
			aswm.Type, _ = strconv.Atoi(value)
		case "fps":
			aswm.Fps, _ = strconv.Atoi(value)
		}
	}

	return aswm, nil
}

// GetAnsweringMachineStatus retorna el estado del contestador
func GetAnsweringMachineStatus(path string) (activated, ringEnabled bool, memUsed string, err error) {
	aswm, err := ReadASWMSettings(path)
	if err != nil {
		return false, false, "", err
	}
	return aswm.Activated == 1, aswm.RingEnable == 1, aswm.Memused, nil
}

// ASWMVolumes representa los volúmenes del contestador
type ASWMVolumes struct {
	Ring         int `json:"ring"`
	PlayAvi      int `json:"play_avi"`
	EuConv       int `json:"eu_conv"`
	IuConv       int `json:"iu_conv"`
	SipCallHome  int `json:"sip_call_home"`
	AudioFileLoc int `json:"audio_file_loc"`
	GreetingBus  int `json:"greeting_bus"`
	PlayMemo     int `json:"play_memo"`
	Pager        int `json:"pager"`
	Tones        int `json:"tones"`
	SipPe        int `json:"sip_pe"`
}

// ReadASWMVolumes lee los volúmenes del aswm_settings.ini
func ReadASWMVolumes(path string) (*ASWMVolumes, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	volumes := &ASWMVolumes{}
	inVolumesSection := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Detectar sección [Volumes]
		if strings.HasPrefix(line, "[") {
			inVolumesSection = strings.Contains(line, "Volumes")
			continue
		}

		if !inVolumesSection {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value, _ := strconv.Atoi(strings.TrimSpace(parts[1]))

		switch key {
		case "Ring":
			volumes.Ring = value
		case "PlayAvi":
			volumes.PlayAvi = value
		case "EuConv":
			volumes.EuConv = value
		case "IuConv":
			volumes.IuConv = value
		case "SipCallHome":
			volumes.SipCallHome = value
		case "AudioFileLoc":
			volumes.AudioFileLoc = value
		case "GreetingBus":
			volumes.GreetingBus = value
		case "PlayMemo":
			volumes.PlayMemo = value
		case "Pager":
			volumes.Pager = value
		case "Tones":
			volumes.Tones = value
		case "SipPe":
			volumes.SipPe = value
		}
	}

	return volumes, nil
}
