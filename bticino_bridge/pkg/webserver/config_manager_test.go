package webserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bticino_bridge/pkg/config"

	"github.com/sirupsen/logrus"
)

func newTestCM(t *testing.T) *ConfigManager {
	t.Helper()
	dir := t.TempDir()
	return NewConfigManager(filepath.Join(dir, "config.yaml"), logrus.New())
}

func baseValidConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Bridge.Name = "test-bridge"
	cfg.OpenWebNet.Port = 30006
	cfg.Web.Port = 8082
	return cfg
}

// --- FromJSON: merge sobre la config actual (evita poner a cero lo no enviado) ---

func TestFromJSONPartialPreservesCurrentValues(t *testing.T) {
	cm := newTestCM(t)
	current := baseValidConfig()
	current.MQTT.Password = "secreto"
	current.Streaming.LiveOnCall = true
	cm.config = current

	cfg, err := cm.FromJSON([]byte(`{"Bridge":{"Name":"nuevo-nombre"}}`))
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	if cfg.Bridge.Name != "nuevo-nombre" {
		t.Fatalf("esperaba nombre actualizado, got %q", cfg.Bridge.Name)
	}
	if cfg.MQTT.Password != "secreto" {
		t.Fatal("un JSON parcial no debe borrar la contraseña MQTT actual")
	}
	if !cfg.Streaming.LiveOnCall {
		t.Fatal("un JSON parcial no debe apagar streaming.live_on_call")
	}
}

func TestFromJSONDoesNotMutateCurrent(t *testing.T) {
	cm := newTestCM(t)
	cm.config = baseValidConfig()
	if _, err := cm.FromJSON([]byte(`{"Bridge":{"Name":"otro"}}`)); err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	if cm.config.Bridge.Name != "test-bridge" {
		t.Fatal("FromJSON no debe mutar la config en memoria (solo devuelve la candidata)")
	}
}

func TestFromJSONWithoutCurrentConfig(t *testing.T) {
	cm := newTestCM(t) // cm.config == nil
	cfg, err := cm.FromJSON([]byte(`{"Bridge":{"Name":"solo"}}`))
	if err != nil || cfg.Bridge.Name != "solo" {
		t.Fatalf("FromJSON sin config previa debe funcionar: cfg=%v err=%v", cfg, err)
	}
}

// --- ValidateConfig: reglas nuevas ---

func hasErrorContaining(r ConfigValidationResult, sub string) bool {
	for _, e := range r.Errors {
		if strings.Contains(e, sub) {
			return true
		}
	}
	return false
}

func hasWarningContaining(r ConfigValidationResult, sub string) bool {
	for _, w := range r.Warnings {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func TestValidateBaseConfigIsValid(t *testing.T) {
	cm := newTestCM(t)
	r := cm.ValidateConfig(baseValidConfig())
	if !r.Valid {
		t.Fatalf("config base debería ser válida, errores: %v", r.Errors)
	}
}

func TestValidateLogLevelEnum(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.Bridge.LogLevel = "verbose"
	if r := cm.ValidateConfig(cfg); r.Valid || !hasErrorContaining(r, "log level") {
		t.Fatalf("esperaba error de log level, got %+v", r)
	}
	cfg.Bridge.LogLevel = "debug"
	if r := cm.ValidateConfig(cfg); !r.Valid {
		t.Fatalf("'debug' debe ser válido: %v", r.Errors)
	}
}

func TestValidateSIPTransportAndPort(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.SIP.Enabled = true
	cfg.SIP.ServerHost = "127.0.0.1"
	cfg.SIP.Username = "u"
	cfg.SIP.Transport = "sctp"
	cfg.SIP.ServerPort = 0
	r := cm.ValidateConfig(cfg)
	if r.Valid || !hasErrorContaining(r, "transport") || !hasErrorContaining(r, "SIP server port") {
		t.Fatalf("esperaba errores de transporte y puerto SIP, got %+v", r)
	}
	cfg.SIP.Transport = "tcp"
	cfg.SIP.ServerPort = 5060
	if r := cm.ValidateConfig(cfg); !r.Valid {
		t.Fatalf("SIP tcp:5060 debe ser válido: %v", r.Errors)
	}
}

func TestValidateMQTTPortRange(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.MQTT.Enabled = true
	cfg.MQTT.Host = "192.168.1.3"
	cfg.MQTT.Port = 0
	if r := cm.ValidateConfig(cfg); r.Valid || !hasErrorContaining(r, "MQTT port") {
		t.Fatalf("esperaba error de puerto MQTT, got %+v", r)
	}
}

func TestValidateRTSPPortZeroWhenStreamingEnabled(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.Streaming.Enabled = true
	cfg.Streaming.RTSPPort = 0
	if r := cm.ValidateConfig(cfg); r.Valid || !hasErrorContaining(r, "RTSP port") {
		t.Fatalf("RTSPPort=0 con streaming activo debe ser error, got %+v", r)
	}
}

func TestValidateVideoBackendEnum(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.Streaming.VideoBackend = "vlc"
	if r := cm.ValidateConfig(cfg); r.Valid || !hasErrorContaining(r, "video backend") {
		t.Fatalf("esperaba error de backend de vídeo, got %+v", r)
	}
	cfg.Streaming.VideoBackend = config.VideoBackendAVMedia
	if r := cm.ValidateConfig(cfg); !r.Valid {
		t.Fatalf("'avmedia' debe ser válido: %v", r.Errors)
	}
}

func TestValidateDNSServersMustBeIPs(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.Network.DNS.Servers = []string{"1.1.1.1", "not-an-ip"}
	if r := cm.ValidateConfig(cfg); r.Valid || !hasErrorContaining(r, "DNS") {
		t.Fatalf("esperaba error de DNS, got %+v", r)
	}
}

func TestValidateHomeKitPortNumericString(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.HomeKit.Enabled = true
	cfg.HomeKit.Pin = "12345678"
	cfg.HomeKit.Port = "ochenta"
	if r := cm.ValidateConfig(cfg); r.Valid || !hasErrorContaining(r, "HomeKit port") {
		t.Fatalf("esperaba error de puerto HomeKit, got %+v", r)
	}
	cfg.HomeKit.Port = "8081"
	if r := cm.ValidateConfig(cfg); !r.Valid {
		t.Fatalf("'8081' debe ser válido: %v", r.Errors)
	}
}

func TestValidateHomeKitPinDigits(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.HomeKit.Enabled = true
	cfg.HomeKit.Port = "8081"
	cfg.HomeKit.Pin = "abcdefgh"
	if r := cm.ValidateConfig(cfg); r.Valid || !hasErrorContaining(r, "PIN") {
		t.Fatalf("PIN no numérico debe ser error, got %+v", r)
	}
}

func TestValidateVideoOnDemandWarning(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.Streaming.VideoOnDemand = true
	r := cm.ValidateConfig(cfg)
	if !r.Valid {
		t.Fatalf("video_on_demand es válido (con aviso): %v", r.Errors)
	}
	if !hasWarningContaining(r, "video_on_demand") {
		t.Fatalf("esperaba warning de video_on_demand, got %+v", r.Warnings)
	}
}

// --- RestoreBackup: path traversal ---

func TestRestoreBackupRejectsPathOutsideBackupDir(t *testing.T) {
	cm := newTestCM(t)
	// Fichero YAML válido pero FUERA del directorio de backups
	outside := filepath.Join(t.TempDir(), "evil.yaml")
	if err := os.WriteFile(outside, []byte("bridge:\n  name: evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cm.RestoreBackup(outside); err == nil {
		t.Fatal("RestoreBackup debe rechazar rutas fuera del directorio de backups")
	}
}
