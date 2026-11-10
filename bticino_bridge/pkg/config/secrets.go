package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// SecretsPath devuelve la ruta por defecto del fichero de secretos: un
// secrets.yaml hermano del config.yaml.
func SecretsPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "secrets.yaml")
}

// OverlaySecrets superpone sobre cfg las claves presentes en secretsPath
// (mismo esquema que config.yaml, solo las claves secretas). Un fichero
// ausente no es error: lo decide después Validate() (fail-closed). Un
// fichero ilegible o mal formado SÍ es error: nunca se ignora en silencio.
func OverlaySecrets(cfg *Config, secretsPath string) (bool, error) {
	f, err := os.Open(secretsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, &SecretsFileError{Path: secretsPath, Err: err}
	}
	defer f.Close()

	// io.EOF = fichero vacío o solo con comentarios: no aporta claves, no es error.
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return false, &SecretsFileError{Path: secretsPath, Err: err}
	}
	return true, nil
}

// SecretsFileError: el fichero de secretos existe pero no se puede leer o
// parsear. Es distinguible para que el arranque ABORTE (un secrets.yaml roto
// nunca debe degradar a "arrancar con los defaults").
type SecretsFileError struct {
	Path string
	Err  error
}

func (e *SecretsFileError) Error() string {
	return fmt.Sprintf("invalid secrets file %s: %v", e.Path, e.Err)
}

func (e *SecretsFileError) Unwrap() error { return e.Err }

// UnmigratedSecrets devuelve las claves secretas que cfg (la vista de
// config.yaml SIN superponer) lleva con valor y que secretsPath no aporta. Es
// el estado de un dispositivo sin migrar: quitar esos valores de config.yaml
// los perdería, así que quien escribe config.yaml debe negarse.
func UnmigratedSecrets(cfg *Config, secretsPath string) ([]string, error) {
	provided := &Config{}
	if _, err := OverlaySecrets(provided, secretsPath); err != nil {
		return nil, err
	}
	var keys []string
	for _, f := range secretFields {
		if *f.ptr(cfg) != "" && *f.ptr(provided) == "" {
			keys = append(keys, f.key)
		}
	}
	return keys, nil
}

// SecretsFileTooOpen indica si el fichero de secretos existe y es accesible
// por grupo u otros (debería ser 0600).
func SecretsFileTooOpen(secretsPath string) bool {
	info, err := os.Stat(secretsPath)
	if err != nil {
		return false
	}
	return info.Mode().Perm()&0o077 != 0
}

// secretField describe un campo secreto del Config: su clave YAML y cómo
// llegar a él. Es la ÚNICA lista de secretos: StripSecrets y Validate parten
// de aquí, así que no pueden divergir.
type secretField struct {
	key string
	ptr func(*Config) *string
	// required indica si el subsistema que usa el campo está activado; nil =
	// el campo nunca es obligatorio (se mueve a secrets.yaml, pero no se exige).
	required func(*Config) bool
}

var secretFields = []secretField{
	{"sip.domain", func(c *Config) *string { return &c.SIP.Domain }, func(c *Config) bool { return c.SIP.Enabled }},
	{"sip.password", func(c *Config) *string { return &c.SIP.Password }, func(c *Config) bool { return c.SIP.Enabled }},
	{"mqtt.username", func(c *Config) *string { return &c.MQTT.Username }, nil},
	{"mqtt.password", func(c *Config) *string { return &c.MQTT.Password }, func(c *Config) bool { return c.MQTT.Enabled }},
	{"homekit.pin", func(c *Config) *string { return &c.HomeKit.Pin }, func(c *Config) bool { return c.HomeKit.Enabled }},
	{"security.web_username", func(c *Config) *string { return &c.Security.WebUsername }, nil},
	{"security.web_password", func(c *Config) *string { return &c.Security.WebPassword }, func(c *Config) bool { return c.Web.Enabled && c.Security.WebAuthRequired }},
	{"notifications.telegram.bot_token", func(c *Config) *string { return &c.Notifications.Telegram.BotToken }, func(c *Config) bool { return c.Notifications.Telegram.Enabled }},
	{"notifications.email.password", func(c *Config) *string { return &c.Notifications.Email.Password }, nil},
	{"notifications.pushover.api_key", func(c *Config) *string { return &c.Notifications.Pushover.APIKey }, nil},
	{"notifications.pushover.user_key", func(c *Config) *string { return &c.Notifications.Pushover.UserKey }, nil},
}

// secretPlaceholders son los valores de plantilla (secrets.example.yaml y
// saneado del repo público). Un secreto que los contenga cuenta como ausente:
// así no se arranca con la plantilla sin rellenar.
var secretPlaceholders = []string{"CHANGE_ME", "REDACTED_HA1", "YOUR_GATEWAY_ID", "SET_IN_SECRETS"}

func secretMissing(v string) bool {
	if v == "" {
		return true
	}
	for _, p := range secretPlaceholders {
		if strings.Contains(v, p) {
			return true
		}
	}
	return false
}

// MissingSecretsError lista TODAS las claves secretas requeridas que faltan.
type MissingSecretsError struct {
	Keys []string
}

func (e *MissingSecretsError) Error() string {
	return "missing required secrets: " + strings.Join(e.Keys, ", ") +
		" — set them in secrets.yaml (see configs/secrets.example.yaml)"
}

// Validate es el gate fail-closed de secretos: exige los secretos de cada
// subsistema ACTIVADO. Debe llamarse después de aplicar los flags de
// activación sobre los campos Enabled.
func (c *Config) Validate() error {
	var missing []string
	for _, f := range secretFields {
		if f.required != nil && f.required(c) && secretMissing(*f.ptr(c)) {
			missing = append(missing, f.key)
		}
	}
	if len(missing) > 0 {
		return &MissingSecretsError{Keys: missing}
	}
	return nil
}

// StripSecrets vacía en el propio c todos los campos secretos. Lo usa quien
// escribe config.yaml: los secretos solo viven en secrets.yaml.
func StripSecrets(c *Config) {
	for _, f := range secretFields {
		*f.ptr(c) = ""
	}
}

// UnmigratedSecretsInFile aplica UnmigratedSecrets al config.yaml en disco
// (sin superponer). Un config.yaml ausente o ilegible no aporta claves.
func UnmigratedSecretsInFile(configPath, secretsPath string) ([]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil
	}
	onDisk := &Config{}
	if err := yaml.Unmarshal(data, onDisk); err != nil {
		return nil, nil
	}
	return UnmigratedSecrets(onDisk, secretsPath)
}
