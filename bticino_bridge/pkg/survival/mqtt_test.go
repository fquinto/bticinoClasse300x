package survival

import (
	"strings"
	"testing"

	"bticino_bridge/pkg/config"
)

func TestSurvivalCommandPublishesRealState(t *testing.T) {
	state := "off"
	m := New(config.SurvivalConfig{Enabled: true, ScriptPath: "s.sh"},
		func(_ string, args ...string) (string, error) {
			switch args[0] {
			case "apply":
				state = "on"
			case "revert":
				state = "off"
			}
			return state + "\n", nil
		})
	ns, err := m.HandleCommand("ON")
	if err != nil || ns != "on" {
		t.Fatalf("ON debe aplicar y leer el estado real: %q %v", ns, err)
	}
	ns, err = m.HandleCommand("OFF")
	if err != nil || ns != "off" {
		t.Fatalf("OFF debe revertir y leer el estado real: %q %v", ns, err)
	}
}

func TestSurvivalDiscoveryIsSwitch(t *testing.T) {
	m := New(config.SurvivalConfig{Enabled: true}, nil)
	topic, payload := m.DiscoveryConfig("bticino")
	if !strings.Contains(topic, "switch") {
		t.Fatalf("el topic de discovery debe ser de un switch: %s", topic)
	}
	if !strings.Contains(string(payload), "command_topic") || !strings.Contains(string(payload), "state_topic") {
		t.Fatalf("el payload debe tener command_topic y state_topic: %s", payload)
	}
}
