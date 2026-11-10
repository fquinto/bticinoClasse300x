package survival

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DiscoveryConfig devuelve el topic y el payload de auto-discovery de Home
// Assistant para un switch que conmuta la supervivencia a la nube.
func (m *Manager) DiscoveryConfig(devicePrefix string) (topic string, payload []byte) {
	topic = fmt.Sprintf("homeassistant/switch/%s_survival/config", devicePrefix)
	cfg := map[string]interface{}{
		"name":          "Cloud survival",
		"unique_id":     devicePrefix + "_survival",
		"command_topic": fmt.Sprintf("%s/survival/set", devicePrefix),
		"state_topic":   fmt.Sprintf("%s/survival/state", devicePrefix),
		"payload_on":    "ON",
		"payload_off":   "OFF",
		"state_on":      "on",
		"state_off":     "off",
		"icon":          "mdi:cloud-off-outline",
	}
	payload, _ = json.Marshal(cfg)
	return topic, payload
}

// HandleCommand procesa un comando del switch ("ON"/"OFF"), aplica o revierte,
// y devuelve el estado LEÍDO del script (no el comando), para que lo que se
// publique a HA refleje el estado real del equipo.
func (m *Manager) HandleCommand(payload string) (newState string, err error) {
	switch strings.ToUpper(strings.TrimSpace(payload)) {
	case "ON":
		if err = m.Apply(); err != nil {
			return "", err
		}
	case "OFF":
		if err = m.Revert(); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("survival: comando desconocido %q", payload)
	}
	return m.Status()
}
