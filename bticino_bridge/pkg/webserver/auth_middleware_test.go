package webserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bticino_bridge/pkg/config"

	"github.com/sirupsen/logrus"
)

func newAuthTestWS(authRequired bool, user, pass string) *WebServer {
	cfg := &config.Config{}
	cfg.Security.WebAuthRequired = authRequired
	cfg.Security.WebUsername = user
	cfg.Security.WebPassword = pass
	return &WebServer{config: cfg, logger: logrus.New()}
}

func doAuthRequest(ws *WebServer, user, pass string) *httptest.ResponseRecorder {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	rec := httptest.NewRecorder()
	ws.authMiddleware(next).ServeHTTP(rec, req)
	return rec
}

func TestAuthDisabledPassesThrough(t *testing.T) {
	ws := newAuthTestWS(false, "", "")
	if rec := doAuthRequest(ws, "", ""); rec.Code != http.StatusOK {
		t.Fatalf("con auth desactivada esperaba 200, got %d", rec.Code)
	}
}

func TestAuthEnabledRejectsMissingCredentials(t *testing.T) {
	ws := newAuthTestWS(true, "admin", "s3cret")
	rec := doAuthRequest(ws, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin credenciales esperaba 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("el 401 debe llevar WWW-Authenticate para que el navegador pida login")
	}
}

func TestAuthEnabledRejectsWrongPassword(t *testing.T) {
	ws := newAuthTestWS(true, "admin", "s3cret")
	if rec := doAuthRequest(ws, "admin", "mala"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("con contraseña incorrecta esperaba 401, got %d", rec.Code)
	}
}

func TestAuthEnabledAcceptsCorrectCredentials(t *testing.T) {
	ws := newAuthTestWS(true, "admin", "s3cret")
	if rec := doAuthRequest(ws, "admin", "s3cret"); rec.Code != http.StatusOK {
		t.Fatalf("con credenciales correctas esperaba 200, got %d", rec.Code)
	}
}

func TestAuthEnabledWithoutPasswordFailsClosed(t *testing.T) {
	// Activado pero sin contraseña: NUNCA abrir en silencio (falsa seguridad);
	// se responde 503 hasta que la config sea coherente.
	ws := newAuthTestWS(true, "admin", "")
	if rec := doAuthRequest(ws, "admin", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("auth activada sin contraseña debe dar 503, got %d", rec.Code)
	}
}

func TestValidateWebAuthRequiresPassword(t *testing.T) {
	cm := newTestCM(t)
	cfg := baseValidConfig()
	cfg.Security.WebAuthRequired = true
	cfg.Security.WebPassword = ""
	if r := cm.ValidateConfig(cfg); r.Valid || !hasErrorContaining(r, "web_password") {
		t.Fatalf("web_auth_required sin contraseña debe ser error de validación, got %+v", r)
	}
	cfg.Security.WebUsername = "admin"
	cfg.Security.WebPassword = "s3cret"
	if r := cm.ValidateConfig(cfg); !r.Valid {
		t.Fatalf("auth bien configurada debe ser válida: %v", r.Errors)
	}
}
