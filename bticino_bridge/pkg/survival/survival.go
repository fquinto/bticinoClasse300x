// Package survival gobierna la supervivencia a la nube (Pilar D) desde el
// bridge: invoca survival.sh (apply/revert/status) por shell-out — coherente
// con la no-interferencia (no abre sockets ni toca los procesos nativos) — y
// expone el estado a Home Assistant. Fail-closed: si no está habilitado, no
// aplica nada.
package survival

import (
	"fmt"
	"os/exec"
	"strings"

	"bticino_bridge/pkg/config"
)

// ExecRunner es el runner de producción: ejecuta `sh <script> <arg>` y devuelve
// su salida combinada. survival.sh usa sus valores por defecto de entorno
// (SURVIVAL_ROOT=/ etc.), apropiados en el equipo.
func ExecRunner() CommandRunner {
	return func(name string, args ...string) (string, error) {
		cmdArgs := append([]string{name}, args...)
		out, err := exec.Command("sh", cmdArgs...).CombinedOutput()
		return string(out), err
	}
}

// CommandRunner ejecuta survival.sh con un argumento y devuelve su salida. Es
// inyectable para tests; en producción es un wrapper sobre exec.Command.
type CommandRunner func(name string, args ...string) (string, error)

// Manager aplica/revierte la supervivencia invocando survival.sh.
type Manager struct {
	cfg    config.SurvivalConfig
	runner CommandRunner
}

// New crea un Manager con la config y el runner dados.
func New(cfg config.SurvivalConfig, runner CommandRunner) *Manager {
	return &Manager{cfg: cfg, runner: runner}
}

// run invoca survival.sh con arg, salvo que la feature esté deshabilitada
// (fail-closed: devuelve error sin invocar nada).
func (m *Manager) run(arg string) (string, error) {
	if !m.cfg.Enabled {
		return "", fmt.Errorf("survival: deshabilitado (survival.enabled=false)")
	}
	if m.runner == nil {
		return "", fmt.Errorf("survival: runner no configurado")
	}
	out, err := m.runner(m.cfg.ScriptPath, arg)
	return strings.TrimSpace(out), err
}

// Apply activa la supervivencia (neutraliza OTA + sinkhole + iptables).
func (m *Manager) Apply() error { _, err := m.run("apply"); return err }

// Revert la desactiva y restaura el estado original.
func (m *Manager) Revert() error { _, err := m.run("revert"); return err }

// Status devuelve el estado real leído del script: "on", "off" o "partial".
func (m *Manager) Status() (string, error) { return m.run("status") }

// Start aplica la supervivencia al arrancar si está habilitada y en auto_apply.
func (m *Manager) Start() error {
	if m.cfg.Enabled && m.cfg.AutoApply {
		return m.Apply()
	}
	return nil
}
