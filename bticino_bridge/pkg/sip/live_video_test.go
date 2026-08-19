package sip

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type mockTap struct {
	mu     sync.Mutex
	starts int
	stops  int
	err    error
}

func (m *mockTap) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.starts++
	return m.err
}

func (m *mockTap) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stops++
}

func (m *mockTap) counts() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.starts, m.stops
}

type mockMarker struct {
	mu     sync.Mutex
	states []bool
}

func (m *mockMarker) SetCallActive(a bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = append(m.states, a)
}

func (m *mockMarker) last() (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.states) == 0 {
		return false, false
	}
	return m.states[len(m.states)-1], true
}

func TestStartArmsTapAndMarks(t *testing.T) {
	tap := &mockTap{}
	mk := &mockMarker{}
	l := NewLiveVideoOnCall(tap, mk, logrus.New())

	l.Start("test")

	if !l.IsActive() {
		t.Fatal("esperaba IsActive()=true tras Start")
	}
	if starts, _ := tap.counts(); starts != 1 {
		t.Fatalf("esperaba 1 arranque del tap, hubo %d", starts)
	}
	if v, ok := mk.last(); !ok || v != true {
		t.Fatalf("esperaba marker SetCallActive(true), got ok=%v v=%v", ok, v)
	}
}

func TestStartIdempotent(t *testing.T) {
	tap := &mockTap{}
	l := NewLiveVideoOnCall(tap, &mockMarker{}, logrus.New())
	l.Start("a")
	l.Start("b")
	if starts, _ := tap.counts(); starts != 1 {
		t.Fatalf("esperaba 1 arranque (idempotente), hubo %d", starts)
	}
}

func TestStopDisarmsAndAllowsRestart(t *testing.T) {
	tap := &mockTap{}
	mk := &mockMarker{}
	l := NewLiveVideoOnCall(tap, mk, logrus.New())
	l.Start("a")
	l.Stop()
	if l.IsActive() {
		t.Fatal("esperaba IsActive()=false tras Stop")
	}
	if _, stops := tap.counts(); stops != 1 {
		t.Fatalf("esperaba 1 parada del tap, hubo %d", stops)
	}
	if v, ok := mk.last(); !ok || v != false {
		t.Fatalf("esperaba marker SetCallActive(false), got ok=%v v=%v", ok, v)
	}
	l.Start("b")
	if starts, _ := tap.counts(); starts != 2 {
		t.Fatalf("esperaba 2 arranques tras re-Start, hubo %d", starts)
	}
}

func TestStopWithoutStartIsNoop(t *testing.T) {
	tap := &mockTap{}
	l := NewLiveVideoOnCall(tap, &mockMarker{}, logrus.New())
	l.Stop()
	if _, stops := tap.counts(); stops != 0 {
		t.Fatalf("Stop sin Start no debe parar el tap (hubo %d)", stops)
	}
}

func TestTapErrorIsNonFatal(t *testing.T) {
	// Un fallo al abrir el tap (p.ej. sin permisos) degrada sin vídeo, pero la
	// llamada sigue marcada como activa (para inhibir el self-INVITE del RTSP).
	tap := &mockTap{err: errors.New("operation not permitted")}
	mk := &mockMarker{}
	l := NewLiveVideoOnCall(tap, mk, logrus.New())
	l.Start("sin-permisos") // no debe entrar en pánico
	if !l.IsActive() {
		t.Fatal("esperaba IsActive()=true aunque el tap falle")
	}
	if v, ok := mk.last(); !ok || v != true {
		t.Fatalf("esperaba marker true aunque el tap falle, got ok=%v v=%v", ok, v)
	}
}

func TestNilMarkerAndNilTapAreSafe(t *testing.T) {
	l := NewLiveVideoOnCall(nil, nil, logrus.New())
	l.Start("a") // no debe entrar en pánico
	l.Stop()
}

func TestGuardAutoStopsWhenHangupIsMissed(t *testing.T) {
	// El fin de llamada se detecta por tramas OWN (heurístico): si se pierde,
	// el guard debe desarmar solo.
	tap := &mockTap{}
	mk := &mockMarker{}
	l := NewLiveVideoOnCall(tap, mk, logrus.New())
	l.SetGuardTimeout(30 * time.Millisecond)
	l.Start("a")

	deadline := time.Now().Add(2 * time.Second)
	for l.IsActive() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if l.IsActive() {
		t.Fatal("esperaba auto-stop por guard, sigue activo")
	}
	if _, stops := tap.counts(); stops != 1 {
		t.Fatalf("esperaba 1 parada del tap por guard, hubo %d", stops)
	}
	if v, ok := mk.last(); !ok || v != false {
		t.Fatalf("esperaba marker false tras guard, got ok=%v v=%v", ok, v)
	}
}

func TestGuardCanceledByStop(t *testing.T) {
	tap := &mockTap{}
	l := NewLiveVideoOnCall(tap, &mockMarker{}, logrus.New())
	l.SetGuardTimeout(30 * time.Millisecond)
	l.Start("a")
	l.Stop()
	time.Sleep(80 * time.Millisecond) // el guard ya habría vencido
	if _, stops := tap.counts(); stops != 1 {
		t.Fatalf("el guard cancelado no debe parar otra vez (stops=%d)", stops)
	}
	// un re-Start tras el guard vencido debe funcionar con guard nuevo
	l.Start("b")
	if !l.IsActive() {
		t.Fatal("esperaba activo tras re-Start")
	}
	l.Stop()
}
