package sip

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bticino_bridge/pkg/events"

	"github.com/sirupsen/logrus"
)

func newTestRTSP(t *testing.T) (*EnhancedRTSPServer, *int32) {
	t.Helper()
	bus := events.NewEventBus(1, logrus.New())
	var ended int32
	bus.Subscribe("sip.call.ended", func(_ *events.Event) {
		atomic.AddInt32(&ended, 1)
	})
	// sipClient nil a propósito: en producción el SIP local puede no registrar.
	r := NewEnhancedRTSPServer(0, nil, bus, logrus.New())
	return r, &ended
}

func waitEnded(c *int32, want int32) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(c) == want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return atomic.LoadInt32(c) == want
}

func TestReleaseSelfCallIgnoresNativeCall(t *testing.T) {
	// Una llamada NATIVA (marcada por LiveVideoOnCall vía SetCallActive) no es
	// nuestra: cuando el último cliente RTSP se va, NO se cuelga ni se limpia.
	r, ended := newTestRTSP(t)
	r.SetCallActive(true)

	r.releaseSelfCallIfIdle("test") // no debe entrar en pánico (sipClient nil)

	r.mutex.RLock()
	active := r.callActive
	r.mutex.RUnlock()
	if !active {
		t.Fatal("la llamada nativa no debe limpiarse al irse los clientes RTSP")
	}
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(ended) != 0 {
		t.Fatal("no debe publicarse sip.call.ended para una llamada nativa")
	}
}

func TestReleaseSelfCallHangsUpOwnCallWithNilClient(t *testing.T) {
	// Llamada self-INVITE (estado interno) con sipClient nil: debe limpiar el
	// estado y publicar sip.call.ended SIN entrar en pánico.
	r, ended := newTestRTSP(t)
	r.mutex.Lock()
	r.callActive = true // interna: no marcada por SetCallActive
	r.mutex.Unlock()

	r.releaseSelfCallIfIdle("test")

	r.mutex.RLock()
	active := r.callActive
	r.mutex.RUnlock()
	if active {
		t.Fatal("esperaba callActive=false tras liberar la llamada propia")
	}
	if !waitEnded(ended, 1) {
		t.Fatalf("esperaba 1 sip.call.ended, got %d", atomic.LoadInt32(ended))
	}
}

func TestReleaseSelfCallSkipsWhenClientsRemain(t *testing.T) {
	r, ended := newTestRTSP(t)
	r.mutex.Lock()
	r.callActive = true
	r.activeClients = 1
	r.mutex.Unlock()

	r.releaseSelfCallIfIdle("test")

	r.mutex.RLock()
	active := r.callActive
	r.mutex.RUnlock()
	if !active {
		t.Fatal("con clientes activos no debe liberarse la llamada")
	}
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(ended) != 0 {
		t.Fatal("no debe publicarse sip.call.ended con clientes activos")
	}
}

func TestSetupUDPWithNilSIPClientDoesNotPanic(t *testing.T) {
	// SETUP con transporte UDP consulta la config del sipClient para el
	// server_port; con sipClient nil debe responder igualmente (fallback).
	r, _ := newTestRTSP(t)
	resp := r.handleSetup(&RTSPRequest{
		Method:    "SETUP",
		URL:       "rtsp://127.0.0.1:6554/doorbell-video/track1",
		CSeq:      2,
		Transport: "RTP/AVP;unicast;client_port=5000-5001",
	}, "127.0.0.1:34567", nil)
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("esperaba 200 OK en SETUP UDP con sipClient nil, got: %q", resp)
	}
}

func TestSetCallActiveFalseClearsExternalMark(t *testing.T) {
	// Tras desmarcar la llamada nativa, una futura llamada self-INVITE debe
	// poder liberarse con normalidad (el flag externo no debe quedar pegado).
	r, ended := newTestRTSP(t)
	r.SetCallActive(true)
	r.SetCallActive(false)

	r.mutex.Lock()
	r.callActive = true // self-INVITE posterior
	r.mutex.Unlock()
	r.releaseSelfCallIfIdle("test")

	if !waitEnded(ended, 1) {
		t.Fatalf("esperaba 1 sip.call.ended tras llamada propia, got %d", atomic.LoadInt32(ended))
	}
}
