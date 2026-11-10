package survival

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bticino_bridge/pkg/config"
)

func TestSurvivalFailClosedWhenDisabled(t *testing.T) {
	called := false
	m := New(config.SurvivalConfig{Enabled: false}, func(string, ...string) (string, error) {
		called = true
		return "", nil
	})
	if err := m.Apply(); err == nil {
		t.Fatal("Apply con Enabled=false debe devolver error fail-closed")
	}
	if called {
		t.Fatal("no debe invocar el script si está deshabilitado")
	}
}

func TestSurvivalStatusReadsScript(t *testing.T) {
	m := New(config.SurvivalConfig{Enabled: true, ScriptPath: "survival.sh"},
		func(name string, args ...string) (string, error) { return "on\n", nil })
	s, err := m.Status()
	if err != nil || s != "on" {
		t.Fatalf("Status debe derivar del script: %q %v", s, err)
	}
}

// TestSurvivalStartRespectsConfig: Start es no-op (sin invocar el script) si la
// feature está deshabilitada, y aplica si enabled+auto_apply.
func TestSurvivalStartRespectsConfig(t *testing.T) {
	calledDisabled := false
	md := New(config.SurvivalConfig{Enabled: false, AutoApply: true},
		func(string, ...string) (string, error) { calledDisabled = true; return "", nil })
	if err := md.Start(); err != nil {
		t.Fatalf("Start deshabilitado debe ser no-op sin error: %v", err)
	}
	if calledDisabled {
		t.Fatal("Start no debe invocar el script si está deshabilitado")
	}

	var gotArg string
	me := New(config.SurvivalConfig{Enabled: true, AutoApply: true, ScriptPath: "s.sh"},
		func(_ string, args ...string) (string, error) { gotArg = args[0]; return "", nil })
	if err := me.Start(); err != nil {
		t.Fatal(err)
	}
	if gotArg != "apply" {
		t.Fatalf("Start con auto_apply debe invocar apply, got %q", gotArg)
	}
}

// TestExecRunnerRunsScript: el runner real ejecuta `sh <script> <arg>` y
// devuelve su salida.
func TestExecRunnerRunsScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "echo.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho got:$1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := ExecRunner()(script, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "got:apply") {
		t.Fatalf("ExecRunner debe ejecutar el script y devolver su salida: %q", out)
	}
}
