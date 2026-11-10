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

// --- Secretos: el dashboard valida contra la vista fusionada y nunca los persiste ---

const templateConfigYAML = `bridge:
  name: "test-bridge"
openwebnet:
  port: 30006
web:
  port: 8082
homekit:
  enabled: true
  port: "8081"
  pin: ""
security:
  web_auth_required: true
  web_password: ""
`

func newCMWithFiles(t *testing.T, cfgYAML, secretsYAML string) *ConfigManager {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if secretsYAML != "" {
		if err := os.WriteFile(filepath.Join(dir, "secrets.yaml"), []byte(secretsYAML), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cm := NewConfigManager(p, logrus.New())
	if _, err := cm.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	return cm
}

func TestValidateUsesSecretsOverlay(t *testing.T) {
	cm := newCMWithFiles(t, templateConfigYAML,
		"homekit:\n  pin: \"11122333\"\nsecurity:\n  web_password: \"pw\"\n")
	r := cm.ValidateConfig(cm.GetConfig())
	if !r.Valid {
		t.Fatalf("plantilla + secrets.yaml debería validar, errores: %v", r.Errors)
	}
	if cm.GetConfig().HomeKit.Pin != "" {
		t.Error("ValidateConfig no debe mutar la config (el pin debe seguir vacío en la vista plantilla)")
	}
}

func TestValidateWithoutSecretsStillRejectsEmptyWebPassword(t *testing.T) {
	cm := newCMWithFiles(t, templateConfigYAML, "")
	r := cm.ValidateConfig(cm.GetConfig())
	if r.Valid || !hasErrorContaining(r, "web_password") {
		t.Fatalf("sin secrets.yaml debe rechazar web_password vacío: %+v", r)
	}
}

func TestSaveConfigStripsSecrets(t *testing.T) {
	cm := newCMWithFiles(t, templateConfigYAML, "")
	cfg := baseValidConfig()
	cfg.SIP.Password = "sipsecretvalue"
	cfg.MQTT.Password = "mqttsecretvalue"
	cfg.HomeKit.Pin = "11122333"
	cfg.Security.WebPassword = "websecretvalue"
	cfg.Logging.Level = "debug"
	if err := cm.SaveConfig(cfg, "t"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"sipsecretvalue", "mqttsecretvalue", "11122333", "websecretvalue"} {
		if strings.Contains(string(data), s) {
			t.Errorf("config.yaml contiene el secreto %q", s)
		}
	}
	reloaded, err := config.LoadConfig(cm.configPath)
	if err != nil || reloaded.Logging.Level != "debug" {
		t.Errorf("lo no secreto debe persistir: level=%v err=%v", reloaded, err)
	}
	if cm.GetConfig().SIP.Password != "" {
		t.Error("la config en memoria del manager no debe guardar secretos")
	}
	if cfg.SIP.Password != "sipsecretvalue" {
		t.Error("SaveConfig no debe mutar el cfg del llamante")
	}
}

// --- Hallazgos de la revisión final: device sin migrar, restore, backups ---

const inlineSecretsConfigYAML = `bridge:
  name: "test-bridge"
openwebnet:
  port: 30006
web:
  port: 8082
mqtt:
  password: "inlinemqttpw"
`

func TestLoadedViewNeverHoldsSecrets(t *testing.T) {
	cm := newCMWithFiles(t, inlineSecretsConfigYAML, "")
	if cm.GetConfig().MQTT.Password != "" {
		t.Error("la vista del dashboard (GET /api/config) no debe contener secretos aunque config.yaml los tenga")
	}
}

func TestSaveRefusedWhenConfigStillHoldsSecrets(t *testing.T) {
	cm := newCMWithFiles(t, inlineSecretsConfigYAML, "")
	err := cm.SaveConfig(baseValidConfig(), "t")
	if err == nil || !strings.Contains(err.Error(), "secrets.yaml") || !strings.Contains(err.Error(), "mqtt.password") {
		t.Fatalf("guardar debe rechazarse nombrando la clave y secrets.yaml, got %v", err)
	}
	data, _ := os.ReadFile(cm.configPath)
	if !strings.Contains(string(data), "inlinemqttpw") {
		t.Error("un guardado rechazado no debe tocar config.yaml (se perdería la credencial)")
	}
	r := cm.ValidateConfig(baseValidConfig())
	if r.Valid || !hasErrorContaining(r, "secrets.yaml") {
		t.Errorf("la validación debe explicar la migración pendiente: %+v", r)
	}
}

func TestSaveAllowedOnceSecretsFileProvidesThem(t *testing.T) {
	cm := newCMWithFiles(t, inlineSecretsConfigYAML, "mqtt:\n  password: \"inlinemqttpw\"\n")
	if err := cm.SaveConfig(baseValidConfig(), "t"); err != nil {
		t.Fatalf("migrado: guardar debe funcionar: %v", err)
	}
	data, _ := os.ReadFile(cm.configPath)
	if strings.Contains(string(data), "inlinemqttpw") {
		t.Error("config.yaml no debe conservar el secreto tras guardar")
	}
}

func writeBackup(t *testing.T, cm *ConfigManager, content string) string {
	t.Helper()
	if err := os.MkdirAll(cm.backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(cm.backupDir, "config_old.yaml")
	if err := os.WriteFile(b, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRestoreBackupStripsSecrets(t *testing.T) {
	cm := newCMWithFiles(t, templateConfigYAML, "mqtt:\n  password: \"frombackuppw\"\n")
	b := writeBackup(t, cm, inlineSecretsConfigYAML+"logging:\n  level: \"debug\"\n")
	b2 := strings.Replace(inlineSecretsConfigYAML, "inlinemqttpw", "frombackuppw", 1) + "logging:\n  level: \"debug\"\n"
	os.WriteFile(b, []byte(b2), 0o644)
	if err := cm.RestoreBackup(b); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	data, _ := os.ReadFile(cm.configPath)
	if strings.Contains(string(data), "frombackuppw") {
		t.Error("restaurar un backup antiguo no debe devolver secretos a config.yaml")
	}
	if cm.GetConfig().MQTT.Password != "" || cm.GetConfig().Logging.Level != "debug" {
		t.Errorf("vista tras restore: password=%q level=%q", cm.GetConfig().MQTT.Password, cm.GetConfig().Logging.Level)
	}
}

func TestRestoreBackupRefusedWhenSecretsWouldBeLost(t *testing.T) {
	cm := newCMWithFiles(t, templateConfigYAML, "")
	b := writeBackup(t, cm, inlineSecretsConfigYAML)
	before, _ := os.ReadFile(cm.configPath)
	err := cm.RestoreBackup(b)
	if err == nil || !strings.Contains(err.Error(), "secrets.yaml") {
		t.Fatalf("restore debe rechazarse si el backup trae secretos que secrets.yaml no tiene, got %v", err)
	}
	after, _ := os.ReadFile(cm.configPath)
	if string(before) != string(after) {
		t.Error("un restore rechazado no debe tocar config.yaml")
	}
}

func TestBackupsAreOwnerOnly(t *testing.T) {
	cm := newCMWithFiles(t, templateConfigYAML, "")
	b, err := cm.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(b)
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Errorf("backup debe ser 0600, got %v err %v", info.Mode().Perm(), err)
	}
}
