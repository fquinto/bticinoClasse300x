// Package deviceconfig proporciona parsers para archivos de configuración del dispositivo BTicino
package deviceconfig

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)

// ConfXML representa la estructura del archivo /var/tmp/conf.xml
type ConfXML struct {
	Language   string `json:"language"`
	Town       string `json:"town"`
	NTPServer  string `json:"ntp_server"`
	NTPalgo    string `json:"ntp_algo"`
	DateFormat int    `json:"date_format"`
	Fuso       string `json:"fuso"`
}

// RawConfXML estructura original del XML para parsing
type RawConfXML struct {
	XMLName  xml.Name     `xml:"configuratore"`
	Setup    ConfSetup    `xml:"setup"`
	Generale ConfGenerale `xml:"generale"`
}

// ConfSetup estructura del elemento setup
type ConfSetup struct {
	Generale ConfGenerale `xml:"generale"`
}

// ConfGenerale estructura del elemento generale
type ConfGenerale struct {
	XMLName     xml.Name     `xml:"generale"`
	Language    string       `xml:"language"`
	Clock       ConfClock    `xml:"clock"`
	Temperature ConfTemp     `xml:"temperature"`
	Password    ConfPassword `xml:"password"`
}

// ConfClock estructura del elemento clock
type ConfClock struct {
	XMLName    xml.Name `xml:"clock"`
	Master     string   `xml:"master"`
	DateFormat string   `xml:"dateformat"`
	Fuso       string   `xml:"fuso"`
	Town       string   `xml:"town"`
	NTPServer  string   `xml:"ntp_server"`
	NTPAalgo   string   `xml:"ntp_algo"`
}

// ConfTemp estructura del elemento temperature
type ConfTemp struct {
	XMLName xml.Name `xml:"temperature"`
	Format  string   `xml:"format"`
}

// ConfPassword estructura del elemento password
type ConfPassword struct {
	XMLName xml.Name `xml:"password"`
	Enabled string   `xml:"enabled"`
	Pwd     string   `xml:"pwd"`
}

// ReadConfXML lee y parsea el archivo /var/tmp/conf.xml
// Usa string parsing porque la estructura XML no se mapea correctamente con encoding/xml estándar
func ReadConfXML(path string) (*ConfXML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Usar string parsing para este XML específico
	return parseConfXMLSimple(string(data)), nil
}

// parseConfXMLSimple fallback using string parsing
func parseConfXMLSimple(content string) *ConfXML {
	conf := &ConfXML{}

	extract := func(tag string) string {
		startTag := "<" + tag + ">"
		endTag := "</" + tag + ">"
		if idx := strings.Index(content, startTag); idx != -1 {
			end := strings.Index(content[idx:], endTag)
			if end > 0 {
				return strings.TrimSpace(content[idx+len(startTag) : idx+end])
			}
		}
		return ""
	}

	conf.Language = extract("language")
	conf.Town = extract("town")
	conf.NTPServer = extract("ntp_server")
	conf.NTPalgo = extract("ntp_algo")
	conf.Fuso = extract("fuso")

	if df := extract("dateformat"); df != "" {
		fmt.Sscanf(df, "%d", &conf.DateFormat)
	}

	return conf
}

// GetLanguage retorna el idioma actual del sistema
func GetLanguage(path string) (string, error) {
	conf, err := ReadConfXML(path)
	if err != nil {
		return "", err
	}
	return conf.Language, nil
}

// GetTimezone retorna la zona horaria configurada
func GetTimezone(path string) (string, error) {
	conf, err := ReadConfXML(path)
	if err != nil {
		return "", err
	}
	return conf.Town, nil
}

// GetNTPServer retorna el servidor NTP configurado
func GetNTPServer(path string) (string, error) {
	conf, err := ReadConfXML(path)
	if err != nil {
		return "", err
	}
	return conf.NTPServer, nil
}

// GetNTPConfig retorna la configuración NTP completa
func GetNTPConfig(path string) (server, algo string, err error) {
	conf, err := ReadConfXML(path)
	if err != nil {
		return "", "", err
	}
	return conf.NTPServer, conf.NTPalgo, nil
}

// ParseConfXMLSimple hace un parsing simple del conf.xml usando strings
func ParseConfXMLSimple(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	result := make(map[string]string)

	// Extraer language
	if idx := strings.Index(content, "<language>"); idx != -1 {
		end := strings.Index(content[idx:], "</language>")
		if end > 0 {
			result["language"] = strings.TrimSpace(content[idx+len("<language>") : idx+end])
		}
	}

	// Extraer town
	if idx := strings.Index(content, "<town>"); idx != -1 {
		end := strings.Index(content[idx:], "</town>")
		if end > 0 {
			result["town"] = strings.TrimSpace(content[idx+len("<town>") : idx+end])
		}
	}

	// Extraer ntp_server
	if idx := strings.Index(content, "<ntp_server>"); idx != -1 {
		end := strings.Index(content[idx:], "</ntp_server>")
		if end > 0 {
			result["ntp_server"] = strings.TrimSpace(content[idx+len("<ntp_server>") : idx+end])
		}
	}

	// Extraer ntp_algo
	if idx := strings.Index(content, "<ntp_algo>"); idx != -1 {
		end := strings.Index(content[idx:], "</ntp_algo>")
		if end > 0 {
			result["ntp_algo"] = strings.TrimSpace(content[idx+len("<ntp_algo>") : idx+end])
		}
	}

	// Extraer dateformat
	if idx := strings.Index(content, "<dateformat>"); idx != -1 {
		end := strings.Index(content[idx:], "</dateformat>")
		if end > 0 {
			result["dateformat"] = strings.TrimSpace(content[idx+len("<dateformat>") : idx+end])
		}
	}

	// Extraer fuso
	if idx := strings.Index(content, "<fuso>"); idx != -1 {
		end := strings.Index(content[idx:], "</fuso>")
		if end > 0 {
			result["fuso"] = strings.TrimSpace(content[idx+len("<fuso>") : idx+end])
		}
	}

	// Extraer language desde general
	if result["language"] == "" {
		if idx := strings.Index(content, "<language>"); idx != -1 {
			result["language"] = extractValue(content, idx, "<language>", "</language>")
		}
	}

	return result, nil
}

// Helper para extraer valores simples
func extractValue(content string, startIdx int, openTag, closeTag string) string {
	end := strings.Index(content[startIdx:], closeTag)
	if end > 0 {
		return strings.TrimSpace(content[startIdx+len(openTag) : startIdx+end])
	}
	return ""
}

// GetDeviceInfoFromConfXML extrae información del dispositivo del conf.xml
func GetDeviceInfoFromConfXML(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	result := make(map[string]string)

	// Modelo
	if idx := strings.Index(content, "<modello>"); idx != -1 {
		result["model"] = extractValue(content, idx, "<modello>", "</modello>")
	}

	// Nombre
	if idx := strings.Index(content, "<nome>"); idx != -1 {
		result["name"] = extractValue(content, idx, "<nome>", "</nome>")
	}

	// IP (ethernet/lan/addressip)
	if idx := strings.Index(content, "<addressip>"); idx != -1 {
		result["ip"] = extractValue(content, idx, "<addressip>", "</addressip>")
	}

	// Netmask
	if idx := strings.Index(content, "<netmask>"); idx != -1 {
		result["netmask"] = extractValue(content, idx, "<netmask>", "</netmask>")
	}

	// Router
	if idx := strings.Index(content, "<router>"); idx != -1 {
		result["router"] = extractValue(content, idx, "<router>", "</router>")
	}

	// Device ID (mhe/hppp/deviceID)
	if idx := strings.Index(content, "<deviceID>"); idx != -1 {
		result["device_id"] = extractValue(content, idx, "<deviceID>", "</deviceID>")
	}

	// Version (ver_xml)
	if idx := strings.Index(content, "<ver_xml>"); idx != -1 {
		result["config_version"] = extractValue(content, idx, "<ver_xml>", "</ver_xml>")
	}

	return result, nil
}
