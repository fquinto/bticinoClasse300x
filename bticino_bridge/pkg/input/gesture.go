package input

import (
	"sync"
	"time"
)

// Gestos que distingue GestureDetector.
const (
	GesturePress     = "press"      // pulsación corta (se emite al soltar)
	GestureLongPress = "long_press" // tecla mantenida (se emite al cumplirse el umbral)
)

// DefaultLongPressThreshold es el tiempo mantenido a partir del cual una
// pulsación cuenta como larga.
const DefaultLongPressThreshold = 800 * time.Millisecond

// GestureDetector convierte pares press/release de una tecla en un único
// gesto: "press" si se suelta antes del umbral, "long_press" si se mantiene.
// Solo observa: no altera lo que el equipo hace con la tecla.
type GestureDetector struct {
	threshold time.Duration
	emit      func(key int, gesture string)

	mu   sync.Mutex
	held map[int]*heldKey
}

type heldKey struct {
	timer *time.Timer
	long  bool // ya se emitió long_press para esta pulsación
}

// NewGestureDetector crea un detector; emit se llama fuera del cerrojo interno.
func NewGestureDetector(threshold time.Duration, emit func(key int, gesture string)) *GestureDetector {
	if threshold <= 0 {
		threshold = DefaultLongPressThreshold
	}
	return &GestureDetector{threshold: threshold, emit: emit, held: map[int]*heldKey{}}
}

// Press registra que la tecla se ha pulsado.
func (d *GestureDetector) Press(key int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, already := d.held[key]; already {
		return // auto-repetición o press duplicado
	}
	h := &heldKey{}
	h.timer = time.AfterFunc(d.threshold, func() {
		d.mu.Lock()
		if d.held[key] != h { // se soltó (o es otra pulsación) antes de disparar
			d.mu.Unlock()
			return
		}
		h.long = true
		d.mu.Unlock()
		d.emit(key, GestureLongPress)
	})
	d.held[key] = h
}

// Release registra que la tecla se ha soltado.
func (d *GestureDetector) Release(key int) {
	d.mu.Lock()
	h, ok := d.held[key]
	if !ok {
		d.mu.Unlock()
		return
	}
	delete(d.held, key)
	h.timer.Stop()
	long := h.long
	d.mu.Unlock()
	if !long {
		d.emit(key, GesturePress)
	}
}
