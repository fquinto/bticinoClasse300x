package mqtt

import (
	"encoding/json"
	"testing"
)

func TestProblemDiscoveryConfig(t *testing.T) {
	id, cfg := problemDiscoveryConfig("bticino", ProblemSensor{Key: "answerer_route", Name: "Contestador sin ruta", Icon: "mdi:phone-alert"})
	if id != "bticino_problem_answerer_route" {
		t.Errorf("object id = %s", id)
	}
	if cfg["device_class"] != "problem" || cfg["entity_category"] != "diagnostic" {
		t.Errorf("clase/categoría: %v / %v", cfg["device_class"], cfg["entity_category"])
	}
	if cfg["state_topic"] != "bticino/problem/answerer_route/state" ||
		cfg["json_attributes_topic"] != "bticino/problem/answerer_route/attributes" {
		t.Errorf("topics: %v / %v", cfg["state_topic"], cfg["json_attributes_topic"])
	}
	if cfg["unique_id"] != "bticino_c300x_problem_answerer_route" {
		t.Errorf("unique_id = %v", cfg["unique_id"])
	}
	if _, err := json.Marshal(cfg); err != nil {
		t.Errorf("no serializable: %v", err)
	}
}

func TestProblemAttrs(t *testing.T) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(problemAttrs("falta el usuario", map[string]interface{}{"file": "/x"})), &m); err != nil {
		t.Fatal(err)
	}
	if m["detail"] != "falta el usuario" || m["file"] != "/x" {
		t.Errorf("attrs = %v", m)
	}
	// Sin extras sigue siendo un objeto JSON válido (HA exige un dict).
	if err := json.Unmarshal([]byte(problemAttrs("", nil)), &m); err != nil {
		t.Fatal(err)
	}
}

// El estado se recuerda aunque MQTT aún no esté conectado, para publicarlo al conectar.
func TestProblemStateRememberedWhileDisconnected(t *testing.T) {
	b := &MQTTBridge{}
	b.RegisterProblemSensor(ProblemSensor{Key: "flexisip", Name: "Flexisip"})
	b.RegisterProblemSensor(ProblemSensor{Key: "flexisip", Name: "Flexisip"}) // idempotente
	b.PublishProblem("flexisip", true, "caído", nil)
	if n := len(b.problemSensors); n != 1 {
		t.Fatalf("registrados %d, want 1", n)
	}
	st, ok := b.problemStates["flexisip"]
	if !ok || !st.problem || st.attrs == "" {
		t.Fatalf("estado no recordado: %+v ok=%v", st, ok)
	}
}
