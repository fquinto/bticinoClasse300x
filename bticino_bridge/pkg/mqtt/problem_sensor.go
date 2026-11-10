package mqtt

import (
	"encoding/json"
)

// ProblemSensor describe un binary_sensor de HA con device_class "problem"
// (ON = hay un problema) que un subsistema registra para avisar de su estado.
type ProblemSensor struct {
	Key  string // identificador corto: "answerer_route", "flexisip"…
	Name string // nombre visible en HA
	Icon string // opcional
}

type problemState struct {
	problem bool
	attrs   string // JSON
}

// problemDiscoveryConfig construye el object_id y la config de discovery.
func problemDiscoveryConfig(dp string, s ProblemSensor) (string, map[string]interface{}) {
	base := dp + "/problem/" + s.Key
	cfg := map[string]interface{}{
		"name":                  s.Name,
		"unique_id":             "bticino_c300x_problem_" + s.Key,
		"object_id":             "bticino_problema_" + s.Key,
		"state_topic":           base + "/state",
		"json_attributes_topic": base + "/attributes",
		"payload_on":            "ON",
		"payload_off":           "OFF",
		"device_class":          "problem",
		"entity_category":       "diagnostic",
		// Solo identificadores: HA lo cuelga del dispositivo ya descubierto.
		"device": map[string]interface{}{"identifiers": []string{"bticino_classe300x"}},
		"availability": map[string]interface{}{
			"topic":                 dp + "/bridge/state",
			"payload_available":     "online",
			"payload_not_available": "offline",
		},
	}
	if s.Icon != "" {
		cfg["icon"] = s.Icon
	}
	return "bticino_problem_" + s.Key, cfg
}

// problemAttrs serializa los atributos del sensor (siempre un objeto JSON).
func problemAttrs(detail string, extra map[string]interface{}) string {
	m := map[string]interface{}{"detail": detail}
	for k, v := range extra {
		m[k] = v
	}
	data, err := json.Marshal(m)
	if err != nil {
		return `{}`
	}
	return string(data)
}

// RegisterProblemSensor da de alta un sensor de problema. Idempotente. Si MQTT
// ya está conectado publica su discovery; si no, se publicará al conectar.
func (b *MQTTBridge) RegisterProblemSensor(s ProblemSensor) {
	b.problemMu.Lock()
	for _, existing := range b.problemSensors {
		if existing.Key == s.Key {
			b.problemMu.Unlock()
			return
		}
	}
	b.problemSensors = append(b.problemSensors, s)
	b.problemMu.Unlock()
	if b.client == nil || !b.IsConnected() {
		return
	}
	id, cfg := problemDiscoveryConfig(b.dataPrefix(), s)
	b.discovery("binary_sensor", id, cfg)
}

// PublishProblem publica el estado de un sensor de problema (retenido). El
// último estado se recuerda para republicarlo cuando MQTT (re)conecte.
func (b *MQTTBridge) PublishProblem(key string, problem bool, detail string, extra map[string]interface{}) {
	st := problemState{problem: problem, attrs: problemAttrs(detail, extra)}
	b.problemMu.Lock()
	if b.problemStates == nil {
		b.problemStates = map[string]problemState{}
	}
	b.problemStates[key] = st
	b.problemMu.Unlock()
	if b.client == nil || !b.IsConnected() {
		return
	}
	b.publishProblemState(key, st)
}

func (b *MQTTBridge) publishProblemState(key string, st problemState) {
	base := b.dataPrefix() + "/problem/" + key
	b.publish(base+"/attributes", st.attrs, true)
	b.publish(base+"/state", boolState(st.problem), true)
}

// publishProblemSensors publica discovery y último estado de todos los
// sensores de problema registrados. Se llama desde el discovery general.
func (b *MQTTBridge) publishProblemSensors() int {
	b.problemMu.Lock()
	sensors := append([]ProblemSensor(nil), b.problemSensors...)
	states := make(map[string]problemState, len(b.problemStates))
	for k, v := range b.problemStates {
		states[k] = v
	}
	b.problemMu.Unlock()
	dp := b.dataPrefix()
	for _, s := range sensors {
		id, cfg := problemDiscoveryConfig(dp, s)
		b.discovery("binary_sensor", id, cfg)
		if st, ok := states[s.Key]; ok {
			b.publishProblemState(s.Key, st)
		}
	}
	return len(sensors)
}

// SetSSHStarter instala la acción del botón "Arrancar SSH" de HA. Debe llamarse
// antes de Start. Sin ella (o sin mqtt.enable_system_buttons) el botón no existe.
func (b *MQTTBridge) SetSSHStarter(start func() error) {
	b.problemMu.Lock()
	b.sshStarter = start
	b.problemMu.Unlock()
}

func (b *MQTTBridge) getSSHStarter() func() error {
	b.problemMu.Lock()
	defer b.problemMu.Unlock()
	return b.sshStarter
}
