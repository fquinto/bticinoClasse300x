package sip

import (
	"testing"

	"github.com/sirupsen/logrus"
)

// TestVideoActivationGateDisabled es la aserción de seguridad clave: con la
// activación de vídeo desactivada (el valor por defecto), ensureSIPCallActive
// rechaza el intento SIN tocar el cliente SIP (aquí nil). Así ni un cliente
// RTSP ni un snapshot pueden disparar comandos (self-INVITE / *7*300) hacia el
// firmware nativo. Regresión del incidente que reinició el dispositivo.
func TestVideoActivationGateDisabled(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel) // silenciar

	srv := NewEnhancedRTSPServer(6554, nil /*sipClient*/, nil /*eventBus*/, logger)

	// videoActivation es false por defecto (no se llamó a SetVideoActivation).
	err := srv.ensureSIPCallActive()
	if err == nil {
		t.Fatal("ensureSIPCallActive debería rechazar con la activación desactivada")
	}
	if err.Error() != "video on-demand activation is disabled" {
		t.Fatalf("error inesperado: %v", err)
	}
}

// TestVideoActivationGateEnables comprueba que SetVideoActivation(true) levanta
// el gate (deja de ser quien bloquea).
func TestVideoActivationGateEnables(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	srv := NewEnhancedRTSPServer(6554, nil, nil, logger)
	srv.SetVideoActivation(true)
	if !srv.videoActivation {
		t.Fatal("SetVideoActivation(true) no habilitó la activación")
	}
}
