package main

import (
	"testing"
	"time"

	"bticino_bridge/pkg/bticino"
	"bticino_bridge/pkg/input"
)

func TestFeedKeyGesture(t *testing.T) {
	var got []string
	d := input.NewGestureDetector(time.Hour, func(key int, g string) { got = append(got, physicalKeyName(key, "")+":"+g) })

	// Auto-repetición del kernel (value 2) mientras se mantiene: no es un "soltar".
	feedKeyGesture(d, bticino.KeyCodeKey, 1)
	feedKeyGesture(d, bticino.KeyCodeKey, 2)
	feedKeyGesture(d, bticino.KeyCodeKey, 2)
	if len(got) != 0 {
		t.Fatalf("la auto-repetición emitió %v", got)
	}
	feedKeyGesture(d, bticino.KeyCodeKey, 0)
	if len(got) != 1 || got[0] != "llave:press" {
		t.Fatalf("got %v", got)
	}

	// Códigos que no son las cuatro teclas (p. ej. BTN_TOUCH=330 de la pantalla) se ignoran.
	feedKeyGesture(d, 330, 1)
	feedKeyGesture(d, 330, 0)
	if len(got) != 1 {
		t.Fatalf("un toque de pantalla generó evento: %v", got)
	}
}
