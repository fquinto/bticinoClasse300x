package mqtt

import (
	"encoding/json"
	"testing"
)

func TestKeyEventPayload(t *testing.T) {
	p, ok := keyEventPayload("llave", "long_press")
	if !ok {
		t.Fatal("long_press debe ser válido")
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(p), &m); err != nil {
		t.Fatalf("no es JSON: %v", err)
	}
	if m["event_type"] != "long_press" || m["key"] != "llave" {
		t.Errorf("payload = %v", m)
	}
}

// HA descarta (con error en su log) un event_type no declarado en el discovery.
func TestKeyEventPayload_RejectsUndeclaredType(t *testing.T) {
	for _, g := range []string{"", "release", "double"} {
		if _, ok := keyEventPayload("llave", g); ok {
			t.Errorf("gesto %q no debería publicarse", g)
		}
	}
	for _, g := range keyEventTypes {
		if _, ok := keyEventPayload("ojo", g); !ok {
			t.Errorf("gesto declarado %q rechazado", g)
		}
	}
}
