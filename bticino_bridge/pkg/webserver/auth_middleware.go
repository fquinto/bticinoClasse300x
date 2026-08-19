package webserver

import (
	"crypto/subtle"
	"net/http"
)

// authMiddleware protege todo el servidor web con HTTP Basic Auth cuando
// security.web_auth_required está activo. Comparación en tiempo constante.
// Activado sin contraseña → 503 (fail-closed): nunca se abre en silencio.
func (ws *WebServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sec := ws.config.Security
		if !sec.WebAuthRequired {
			next.ServeHTTP(w, r)
			return
		}
		if sec.WebPassword == "" {
			ws.logger.Error("web_auth_required activo sin web_password — sirviendo 503 (fail-closed)")
			http.Error(w, "Web auth enabled but no password configured", http.StatusServiceUnavailable)
			return
		}
		user, pass, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(user), []byte(sec.WebUsername)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(sec.WebPassword)) == 1
		if !ok || !userOK || !passOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="BTicino Bridge"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
