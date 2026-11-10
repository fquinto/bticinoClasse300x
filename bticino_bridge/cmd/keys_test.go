package main

import (
	"testing"

	"bticino_bridge/pkg/bticino"
)

func TestPhysicalKeyName(t *testing.T) {
	cases := []struct {
		code     int
		fallback string
		want     string
	}{
		{bticino.KeyCodeKey, "", "llave"},
		{bticino.KeyCodeStar, "KEY_2", "estrella"},
		{bticino.KeyCodeEye, "", "ojo"},
		{bticino.KeyCodePhone, "", "telefono"},
		{99, "KEY_X", "KEY_X"},
		{99, "", "key_99"},
	}
	for _, c := range cases {
		if got := physicalKeyName(c.code, c.fallback); got != c.want {
			t.Errorf("physicalKeyName(%d,%q)=%q want %q", c.code, c.fallback, got, c.want)
		}
	}
}
