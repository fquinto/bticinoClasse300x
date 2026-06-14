// Package deviceconfig proporciona funciones para leer la configuración del dispositivo BTicino
package deviceconfig

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config representa la configuración completa del dispositivo
type Config struct {
	DeviceInfo DeviceInfo `json:"device_info"`
	Language   string     `json:"language"`
	Timezone   string     `json:"timezone"`
	NTPServer  string     `json:"ntp_server"`
	NTPAlgo    string     `json:"ntp_algo"`
	DateTime   time.Time  `json:"datetime"`
	Answering  Answering  `json:"answering"`
	Ringtones  Ringtones  `json:"ringtones"`
	Volumes    Volumes    `json:"volumes"`
	Display    Display    `json:"display"`
	Cameras    Cameras    `json:"cameras"`
	Timestamp  time.Time  `json:"timestamp"`
}

// DeviceInfo información del dispositivo
type DeviceInfo struct {
	Model   string `json:"model"`
	IP      string `json:"ip"`
	Version string `json:"version"`
}

// Answering configuración del contestador automático
type Answering struct {
	Activated  bool   `json:"activated"`
	RingEnable bool   `json:"ring_enabled"`
	LedEnable  bool   `json:"led_enable"`
	MemoryUsed string `json:"memory_used"`
}

// Ringtones configuración de tonos
type Ringtones struct {
	S0       int `json:"s0"`
	S1       int `json:"s1"`
	S2       int `json:"s2"`
	Door     int `json:"door"`
	External int `json:"external"`
	Alarm    int `json:"alarm"`
	Message  int `json:"message"`
	Internal int `json:"internal"`
	S3       int `json:"s3"`
}

// Volumes configuración de volúmenes
type Volumes struct {
	S0       int `json:"s0"`
	S1       int `json:"s1"`
	S2       int `json:"s2"`
	Intercom int `json:"intercom"`
	Door     int `json:"door"`
	SIP      int `json:"sip"`
}

// Display configuración de display
type Display struct {
	Brightness int `json:"brightness"`
	CleanTime  int `json:"clean_time"`
}

// Cameras configuración de cámaras
type Cameras map[string]CameraConfig

type CameraConfig struct {
	Brightness int `json:"brightness"`
	Contrast   int `json:"contrast"`
	Saturation int `json:"saturation"`
	Quality    int `json:"quality"`
}

// DevicePaths rutas de los archivos de configuración del dispositivo
var DevicePaths = struct {
	ConfXML      string
	ASWMSettings string
	TVCCSettings string
	SettingsXML  string
}{
	ConfXML:      "/var/tmp/conf.xml",
	ASWMSettings: "/home/bticino/cfg/extra/47/aswm_settings.ini",
	TVCCSettings: "/home/bticino/cfg/extra/47/tvcc_settings.ini",
	SettingsXML:  "/home/bticino/cfg/extra/0/settings.xml",
}

// ReadConfig lee toda la configuración del dispositivo
func ReadConfig(ip string) (*Config, error) {
	cfg := &Config{
		DeviceInfo: DeviceInfo{
			Model:   "Classe 300 X13E",
			IP:      ip,
			Version: "1.7.17",
		},
		Timestamp: time.Now(),
	}

	// Leer conf.xml (Language, Timezone, NTP)
	confXML, err := ReadConfXML(DevicePaths.ConfXML)
	if err != nil {
		return nil, fmt.Errorf("error reading conf.xml: %w", err)
	}
	cfg.Language = confXML.Language
	cfg.Timezone = confXML.Town
	cfg.NTPServer = confXML.NTPServer
	cfg.NTPAlgo = confXML.NTPalgo

	// Intentar obtener datetime del sistema (si está disponible)
	// Por ahora usamos el timestamp de lectura
	cfg.DateTime = time.Now()

	// Leer aswm_settings.ini (Answering Machine)
	aswm, err := ReadASWMSettings(DevicePaths.ASWMSettings)
	if err != nil {
		return nil, fmt.Errorf("error reading aswm_settings.ini: %w", err)
	}
	cfg.Answering = Answering{
		Activated:  aswm.Activated == 1,
		RingEnable: aswm.RingEnable == 1,
		LedEnable:  aswm.LedEnable == 1,
		MemoryUsed: aswm.Memused,
	}

	// Leer tvcc_settings.ini (Cámaras)
	tvcc, err := ReadTVCCSettings(DevicePaths.TVCCSettings)
	if err != nil {
		return nil, fmt.Errorf("error reading tvcc_settings.ini: %w", err)
	}
	// Convertir TVCCSettings a Cameras
	cfg.Cameras = make(Cameras, len(tvcc))
	for k, v := range tvcc {
		cfg.Cameras[k] = CameraConfig{
			Brightness: v.Brightness,
			Contrast:   v.Contrast,
			Saturation: v.Saturation,
			Quality:    v.Quality,
		}
	}

	// Leer settings.xml (Ringtones, Volúmenes, Display)
	settings, err := ReadSettingsXML(DevicePaths.SettingsXML)
	if err != nil {
		return nil, fmt.Errorf("error reading settings.xml: %w", err)
	}
	cfg.Ringtones = settings.Ringtones
	cfg.Volumes = settings.Volumes
	cfg.Display = settings.Display

	return cfg, nil
}

// ReadConfigFromPaths lee la configuración usando rutas personalizadas
func ReadConfigFromPaths(paths struct {
	ConfXML      string
	ASWMSettings string
	TVCCSettings string
	SettingsXML  string
}) (*Config, error) {
	cfg := &Config{
		DeviceInfo: DeviceInfo{
			Model:   "Classe 300 X13E",
			IP:      "unknown",
			Version: "1.7.17",
		},
		Timestamp: time.Now(),
	}

	// Leer conf.xml
	if paths.ConfXML != "" {
		confXML, err := ReadConfXML(paths.ConfXML)
		if err != nil {
			return nil, fmt.Errorf("error reading conf.xml: %w", err)
		}
		cfg.Language = confXML.Language
		cfg.Timezone = confXML.Town
		cfg.NTPServer = confXML.NTPServer
		cfg.NTPAlgo = confXML.NTPalgo
		cfg.DateTime = time.Now()
	}

	// Leer aswm_settings.ini
	if paths.ASWMSettings != "" {
		aswm, err := ReadASWMSettings(paths.ASWMSettings)
		if err != nil {
			return nil, fmt.Errorf("error reading aswm_settings.ini: %w", err)
		}
		cfg.Answering = Answering{
			Activated:  aswm.Activated == 1,
			RingEnable: aswm.RingEnable == 1,
			LedEnable:  aswm.LedEnable == 1,
			MemoryUsed: aswm.Memused,
		}
	}

	// Leer tvcc_settings.ini
	if paths.TVCCSettings != "" {
		tvcc, err := ReadTVCCSettings(paths.TVCCSettings)
		if err != nil {
			return nil, fmt.Errorf("error reading tvcc_settings.ini: %w", err)
		}
		// Convertir TVCCSettings a Cameras
		cfg.Cameras = make(Cameras, len(tvcc))
		for k, v := range tvcc {
			cfg.Cameras[k] = CameraConfig{
				Brightness: v.Brightness,
				Contrast:   v.Contrast,
				Saturation: v.Saturation,
				Quality:    v.Quality,
			}
		}
	}

	// Leer settings.xml
	if paths.SettingsXML != "" {
		settings, err := ReadSettingsXML(paths.SettingsXML)
		if err != nil {
			return nil, fmt.Errorf("error reading settings.xml: %w", err)
		}
		cfg.Ringtones = settings.Ringtones
		cfg.Volumes = settings.Volumes
		cfg.Display = settings.Display
	}

	return cfg, nil
}

// FileExists verifica si un archivo existe
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetBaseDir retorna el directorio base del dispositivo
func GetBaseDir() string {
	return "/home/bticino"
}

// GetConfigDir retorna el directorio de configuración
func GetConfigDir() string {
	return "/home/bticino/cfg"
}

// GetExtraConfigDir retorna el directorio de configuración extra
func GetExtraConfigDir() string {
	return "/home/bticino/cfg/extra"
}

// NormalizePath normaliza una ruta del dispositivo (remueve /home/bticino/ si es necesario)
func NormalizePath(path string) string {
	homeDir := GetBaseDir()
	if strings.HasPrefix(path, homeDir+"/") {
		return path[len(homeDir)+1:]
	}
	return path
}
