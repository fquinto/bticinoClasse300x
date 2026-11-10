package input

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// OnKeyRaw debe llamarse en el mismo hilo y en el orden de llegada (el callback
// OnButtonPress va en goroutines y puede desordenar press/release).
func TestProcessKeyEvent_OnKeyRawIsSynchronousAndOrdered(t *testing.T) {
	im := NewInputMonitor(logrus.New())
	var got []int32
	im.OnKeyRaw = func(code int, value int32) { got = append(got, value) }
	for i := 0; i < 200; i++ {
		im.processKeyEvent("/dev/input/event0", 2, 1, time.Now())
		im.processKeyEvent("/dev/input/event0", 2, 0, time.Now())
	}
	if len(got) != 400 {
		t.Fatalf("llamadas = %d, want 400 (¿asíncrono?)", len(got))
	}
	for i, v := range got {
		if v != int32(1-i%2) {
			t.Fatalf("orden roto en %d: %v", i, v)
		}
	}
}
