// Package webserver proporciona handlers para la API de configuración del dispositivo
package webserver

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"bticino_bridge/pkg/deviceconfig"
)

// @Summary Get device configuration
// @Description Returns the complete device configuration from system files
// @Tags Device Configuration
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/config/device [get]
func (ws *WebServer) handleAPIDeviceConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Usar las rutas del dispositivo
	paths := struct {
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

	cfg, err := deviceconfig.ReadConfigFromPaths(paths)
	if err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ws.writeJSON(w, map[string]interface{}{
		"success": true,
		"config":  cfg,
	})
}

// @Summary Get device language
// @Description Returns the current device language setting
// @Tags Device Configuration
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/config/language [get]
func (ws *WebServer) handleAPILanguage(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lang, err := deviceconfig.GetLanguage("/var/tmp/conf.xml")
	ws.logger.WithError(err).WithField("language", lang).Info("Language request")
	if err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ws.writeJSON(w, map[string]interface{}{
		"success":  true,
		"language": lang,
	})
}

// @Summary Get SIP accounts
// @Description Returns SIP accounts configured on the device from flexisip
// @Tags Device Configuration
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/config/sip-accounts [get]
func (ws *WebServer) handleAPISIPAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flexisipUsersPath := "/home/bticino/cfg/extra/90/flexisip/users/users.db.txt"

	data, err := ioutil.ReadFile(flexisipUsersPath)
	if err != nil {
		ws.writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   "Failed to read SIP accounts: " + err.Error(),
		})
		return
	}

	var accounts []map[string]interface{}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version:") {
			continue
		}
		if strings.Contains(line, "@") && strings.Contains(line, "md5:") {
			// Format: user@domain md5:hash ;
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				username := parts[0]
				authType := parts[1]
				hash := strings.TrimSuffix(parts[2], ";")

				accounts = append(accounts, map[string]interface{}{
					"username":      username,
					"auth_type":     strings.Trim(authType, ":"),
					"password_hash": strings.Trim(hash, ":"),
				})
			}
		} else if strings.Contains(line, "@") && strings.Contains(line, "plain:") {
			// Format: user@domain plain:password ;
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				username := parts[0]
				authType := parts[1]
				password := strings.TrimSuffix(parts[2], ";")

				accounts = append(accounts, map[string]interface{}{
					"username":  username,
					"auth_type": strings.Trim(authType, ":"),
					"password":  strings.Trim(password, ":"),
				})
			}
		}
	}

	ws.writeJSON(w, map[string]interface{}{
		"success":  true,
		"accounts": accounts,
		"source":   flexisipUsersPath,
	})
}

// ensure we implement http.Handler if needed (for future use)
var _ = func() {
	// This ensures the handlers are registered
	_ = json.Marshal
	_ = time.Now()
}
