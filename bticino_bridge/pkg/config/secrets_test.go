package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// writeFiles escribe config.yaml (y secrets.yaml si secrets != "") en un
// directorio temporal y devuelve la ruta del config.
func writeFiles(t *testing.T, config, secrets string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if secrets != "" {
		if err := os.WriteFile(filepath.Join(dir, "secrets.yaml"), []byte(secrets), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestOverlayMergesSecretsOverConfig(t *testing.T) {
	p := writeFiles(t,
		"sip:\n  enabled: true\n  username: \"webrtc\"\n  password: \"\"\nmqtt:\n  host: \"10.0.0.1\"\n",
		"sip:\n  password: \"s3cret\"\nsecurity:\n  web_password: \"w\"\nnotifications:\n  telegram:\n    bot_token: \"tok\"\n")
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SIP.Password != "s3cret" || cfg.SIP.Username != "webrtc" || !cfg.SIP.Enabled {
		t.Errorf("sip mal fusionado: %+v", cfg.SIP)
	}
	if cfg.MQTT.Host != "10.0.0.1" {
		t.Errorf("mqtt.host = %q", cfg.MQTT.Host)
	}
	if cfg.Security.WebPassword != "w" {
		t.Errorf("web_password = %q", cfg.Security.WebPassword)
	}
	if cfg.Notifications.Telegram.BotToken != "tok" {
		t.Errorf("bot_token = %q", cfg.Notifications.Telegram.BotToken)
	}
}

func TestLoadConfigWithoutSecretsFileIsNotAnError(t *testing.T) {
	p := writeFiles(t, "mqtt:\n  host: \"10.0.0.1\"\n  password: \"x\"\n", "")
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.MQTT.Host != "10.0.0.1" || cfg.MQTT.Password != "x" {
		t.Errorf("mqtt = %+v", cfg.MQTT)
	}
}

func TestMalformedSecretsFileIsAnError(t *testing.T) {
	p := writeFiles(t, "mqtt:\n  host: \"h\"\n", ":\n\t- [")
	_, err := LoadConfig(p)
	if err == nil || !strings.Contains(err.Error(), "secrets.yaml") {
		t.Fatalf("esperaba error nombrando secrets.yaml, got %v", err)
	}
}

func TestLoadConfigWithSecretsExplicitPath(t *testing.T) {
	p := writeFiles(t, "mqtt:\n  host: \"h\"\n", "mqtt:\n  password: \"sibling\"\n")
	other := filepath.Join(filepath.Dir(p), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	explicit := filepath.Join(other, "s.yaml")
	if err := os.WriteFile(explicit, []byte("mqtt:\n  password: \"explicit\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigWithSecrets(p, explicit)
	if err != nil {
		t.Fatalf("LoadConfigWithSecrets: %v", err)
	}
	if cfg.MQTT.Password != "explicit" {
		t.Errorf("password = %q, want explicit", cfg.MQTT.Password)
	}
}

func TestSecretsFileTooOpen(t *testing.T) {
	dir := t.TempDir()
	open := filepath.Join(dir, "open.yaml")
	closed := filepath.Join(dir, "closed.yaml")
	os.WriteFile(open, []byte("a: b\n"), 0o644)
	os.Chmod(open, 0o644)
	os.WriteFile(closed, []byte("a: b\n"), 0o600)
	os.Chmod(closed, 0o600)
	if !SecretsFileTooOpen(open) {
		t.Error("0644 debería ser too open")
	}
	if SecretsFileTooOpen(closed) {
		t.Error("0600 no debería ser too open")
	}
	if SecretsFileTooOpen(filepath.Join(dir, "missing.yaml")) {
		t.Error("fichero ausente no debería ser too open")
	}
}

func TestSecretsPathIsSiblingOfConfig(t *testing.T) {
	if got := SecretsPath("/a/b/config.yaml"); got != "/a/b/secrets.yaml" {
		t.Errorf("SecretsPath = %q", got)
	}
}

func allEnabled() *Config {
	c := &Config{}
	c.SIP.Enabled, c.MQTT.Enabled, c.HomeKit.Enabled = true, true, true
	c.Web.Enabled, c.Security.WebAuthRequired = true, true
	return c
}

func missingKeys(t *testing.T, err error) []string {
	t.Helper()
	var m *MissingSecretsError
	if !errors.As(err, &m) {
		t.Fatalf("esperaba *MissingSecretsError, got %v", err)
	}
	return m.Keys
}

func TestValidateListsAllMissingKeys(t *testing.T) {
	err := allEnabled().Validate()
	want := []string{"sip.domain", "sip.password", "mqtt.password", "homekit.pin", "security.web_password"}
	if got := missingKeys(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
	if !strings.Contains(err.Error(), "secrets.yaml") {
		t.Errorf("el error no apunta a secrets.yaml: %v", err)
	}
}

func TestValidateIgnoresDisabledSubsystems(t *testing.T) {
	if err := (&Config{}).Validate(); err != nil {
		t.Errorf("todo desactivado debería validar, got %v", err)
	}
}

func TestValidateWebPasswordOnlyWhenAuthRequired(t *testing.T) {
	c := &Config{}
	c.Web.Enabled = true
	if err := c.Validate(); err != nil {
		t.Errorf("web sin auth no exige password, got %v", err)
	}
}

func TestValidateTelegramOnlyWhenEnabled(t *testing.T) {
	c := &Config{}
	if err := c.Validate(); err != nil {
		t.Errorf("telegram desactivado: %v", err)
	}
	c.Notifications.Telegram.Enabled = true
	want := []string{"notifications.telegram.bot_token"}
	if got := missingKeys(t, c.Validate()); !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestValidateTreatsPlaceholdersAsMissing(t *testing.T) {
	c := &Config{}
	c.SIP.Enabled, c.MQTT.Enabled, c.HomeKit.Enabled = true, true, true
	c.SIP.Domain = "YOUR_GATEWAY_ID.bs.iotleg.com"
	c.SIP.Password = "REDACTED_HA1"
	c.MQTT.Password = "CHANGE_ME"
	c.HomeKit.Pin = "SET_IN_SECRETS"
	want := []string{"sip.domain", "sip.password", "mqtt.password", "homekit.pin"}
	if got := missingKeys(t, c.Validate()); !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestValidatePassesWhenAllPresent(t *testing.T) {
	c := allEnabled()
	c.Notifications.Telegram.Enabled = true
	c.SIP.Domain, c.SIP.Password = "gw.example.test", "abc"
	c.MQTT.Password, c.HomeKit.Pin = "m", "11122333"
	c.Security.WebPassword = "w"
	c.Notifications.Telegram.BotToken = "tok"
	if err := c.Validate(); err != nil {
		t.Errorf("debería validar, got %v", err)
	}
}

func TestEmptySecretKeyBlanksValueAndFailsValidation(t *testing.T) {
	p := writeFiles(t,
		"mqtt:\n  enabled: true\n  password: \"x\"\n",
		"mqtt:\n  password: \"\"\n")
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"mqtt.password"}
	if got := missingKeys(t, cfg.Validate()); !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestStripSecretsBlanksEveryFieldAndNothingElse(t *testing.T) {
	c := &Config{}
	c.SIP.Domain, c.SIP.Password, c.SIP.Username = "d", "p", "webrtc"
	c.MQTT.Username, c.MQTT.Password, c.MQTT.Host = "u", "p", "h"
	c.HomeKit.Pin = "11122333"
	c.Security.WebUsername, c.Security.WebPassword = "a", "w"
	c.Notifications.Telegram.BotToken = "t"
	c.Notifications.Email.Password = "e"
	c.Notifications.Pushover.APIKey, c.Notifications.Pushover.UserKey = "k", "uk"
	StripSecrets(c)
	for name, v := range map[string]string{
		"sip.domain": c.SIP.Domain, "sip.password": c.SIP.Password,
		"mqtt.username": c.MQTT.Username, "mqtt.password": c.MQTT.Password,
		"homekit.pin":           c.HomeKit.Pin,
		"security.web_username": c.Security.WebUsername, "security.web_password": c.Security.WebPassword,
		"telegram.bot_token": c.Notifications.Telegram.BotToken,
		"email.password":     c.Notifications.Email.Password,
		"pushover.api_key":   c.Notifications.Pushover.APIKey, "pushover.user_key": c.Notifications.Pushover.UserKey,
	} {
		if v != "" {
			t.Errorf("%s no se vació: %q", name, v)
		}
	}
	if c.SIP.Username != "webrtc" || c.MQTT.Host != "h" {
		t.Errorf("StripSecrets tocó campos no secretos: %q %q", c.SIP.Username, c.MQTT.Host)
	}
}

// El config.yaml del repo es una PLANTILLA: ningún campo secreto con valor.
func TestRepoConfigTemplateHasNoSecrets(t *testing.T) {
	data, err := os.ReadFile("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// yaml.Unmarshal directo (no LoadConfig): no superponer un secrets.yaml local.
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		t.Fatal(err)
	}
	for _, f := range secretFields {
		if v := *f.ptr(cfg); v != "" {
			t.Errorf("configs/config.yaml lleva valor en el campo secreto %s", f.key)
		}
	}
}

// secrets.example.yaml solo lleva placeholders: copiado sin rellenar no arranca.
func TestSecretsExampleIsAllPlaceholders(t *testing.T) {
	cfg := allEnabled()
	cfg.Notifications.Telegram.Enabled = true
	applied, err := OverlaySecrets(cfg, "../../configs/secrets.example.yaml")
	if err != nil || !applied {
		t.Fatalf("secrets.example.yaml debe existir y parsear: applied=%v err=%v", applied, err)
	}
	want := []string{"sip.domain", "sip.password", "mqtt.password", "homekit.pin",
		"security.web_password", "notifications.telegram.bot_token"}
	if got := missingKeys(t, cfg.Validate()); !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

// --- Hallazgos de la revisión final ---

// Un secrets.yaml vacío o solo con comentarios (touch, o el example con todo
// comentado) no es un fichero roto: equivale a "sin claves".
func TestEmptyOrCommentOnlySecretsFileIsNotAnError(t *testing.T) {
	for name, content := range map[string]string{"vacío": "", "comentarios": "# nada\n# aún\n"} {
		p := writeFiles(t, "mqtt:\n  host: \"h\"\n", "")
		if err := os.WriteFile(SecretsPath(p), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(p)
		if err != nil {
			t.Errorf("%s: no debería ser error: %v", name, err)
			continue
		}
		if cfg.MQTT.Host != "h" {
			t.Errorf("%s: config alterado", name)
		}
	}
}

// El error de un secrets.yaml roto es distinguible (el arranque aborta en vez
// de caer a los defaults).
func TestMalformedSecretsFileErrorIsTyped(t *testing.T) {
	p := writeFiles(t, "mqtt:\n  host: \"h\"\n", ":\n\t- [")
	_, err := LoadConfig(p)
	var se *SecretsFileError
	if !errors.As(err, &se) || se.Path != SecretsPath(p) {
		t.Fatalf("esperaba *SecretsFileError con la ruta, got %v", err)
	}
}

// UnmigratedSecrets: secretos que siguen en config.yaml y que secrets.yaml no
// aporta (device sin migrar). Quitarlos de config.yaml los perdería.
func TestUnmigratedSecrets(t *testing.T) {
	inline := func() *Config {
		c := &Config{}
		c.MQTT.Password = "x"
		c.HomeKit.Pin = "11122333"
		return c
	}
	dir := t.TempDir()
	missing := filepath.Join(dir, "none.yaml")
	got, err := UnmigratedSecrets(inline(), missing)
	if err != nil || !reflect.DeepEqual(got, []string{"mqtt.password", "homekit.pin"}) {
		t.Errorf("sin secrets.yaml: got %v err %v", got, err)
	}
	partial := filepath.Join(dir, "partial.yaml")
	os.WriteFile(partial, []byte("homekit:\n  pin: \"11122333\"\n"), 0o600)
	got, err = UnmigratedSecrets(inline(), partial)
	if err != nil || !reflect.DeepEqual(got, []string{"mqtt.password"}) {
		t.Errorf("secrets parcial: got %v err %v", got, err)
	}
	full := filepath.Join(dir, "full.yaml")
	os.WriteFile(full, []byte("homekit:\n  pin: \"1\"\nmqtt:\n  password: \"y\"\n"), 0o600)
	got, err = UnmigratedSecrets(inline(), full)
	if err != nil || len(got) != 0 {
		t.Errorf("secrets completo: got %v err %v", got, err)
	}
	if got, _ := UnmigratedSecrets(&Config{}, missing); len(got) != 0 {
		t.Errorf("plantilla sin secretos: got %v", got)
	}
}
