package input

import (
	"sync"
	"testing"
	"time"
)

type gestureLog struct {
	mu  sync.Mutex
	got []string
}

func (l *gestureLog) emit(key int, gesture string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.got = append(l.got, gesture)
}

func (l *gestureLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.got...)
}

func TestGesture_ShortPressEmitsPressOnRelease(t *testing.T) {
	l := &gestureLog{}
	d := NewGestureDetector(200*time.Millisecond, l.emit)
	d.Press(2)
	if n := len(l.snapshot()); n != 0 {
		t.Fatalf("no debe emitir al pulsar, emitió %d", n)
	}
	d.Release(2)
	time.Sleep(300 * time.Millisecond) // el temporizador cancelado no debe disparar después
	if got := l.snapshot(); len(got) != 1 || got[0] != GesturePress {
		t.Fatalf("got %v, want [press]", got)
	}
}

func TestGesture_HoldEmitsLongPressOnceWhileHeld(t *testing.T) {
	l := &gestureLog{}
	d := NewGestureDetector(40*time.Millisecond, l.emit)
	d.Press(2)
	time.Sleep(150 * time.Millisecond)
	if got := l.snapshot(); len(got) != 1 || got[0] != GestureLongPress {
		t.Fatalf("mantenida: got %v, want [long_press]", got)
	}
	d.Release(2) // soltar tras una larga no añade un "press"
	if got := l.snapshot(); len(got) != 1 {
		t.Fatalf("tras soltar: got %v, want un solo evento", got)
	}
}

func TestGesture_KeysAreIndependent(t *testing.T) {
	l := &gestureLog{}
	d := NewGestureDetector(200*time.Millisecond, l.emit)
	d.Press(2)
	d.Press(3)
	d.Release(3)
	d.Release(2)
	if got := l.snapshot(); len(got) != 2 {
		t.Fatalf("got %v, want 2 press", got)
	}
}

// Un "release" sin "press" previo (arranque a media pulsación) y un "press"
// repetido (auto-repetición del kernel) no generan eventos de más.
func TestGesture_StrayReleaseAndRepeatedPress(t *testing.T) {
	l := &gestureLog{}
	d := NewGestureDetector(200*time.Millisecond, l.emit)
	d.Release(2)
	d.Press(2)
	d.Press(2)
	d.Release(2)
	if got := l.snapshot(); len(got) != 1 || got[0] != GesturePress {
		t.Fatalf("got %v, want [press]", got)
	}
}
