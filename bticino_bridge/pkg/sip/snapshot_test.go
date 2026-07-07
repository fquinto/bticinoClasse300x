package sip

import (
	"bytes"
	"testing"
)

func TestScanJPEGFrames(t *testing.T) {
	// Tres fotogramas completos concatenados + basura entre medias + un
	// cuarto fotograma incompleto (SOI sin EOI) al final.
	frame := func(payload byte) []byte {
		return append(append([]byte{0xFF, 0xD8}, payload, payload, payload), 0xFF, 0xD9)
	}
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x11}) // basura previa
	buf.Write(frame(0xA1))
	buf.Write(frame(0xA2))
	buf.Write(frame(0xA3))
	buf.Write([]byte{0xFF, 0xD8, 0x99}) // fotograma incompleto

	count, last := scanJPEGFrames(buf.Bytes())
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	want := frame(0xA3)
	if !bytes.Equal(last, want) {
		t.Errorf("last = %x, want %x (el ultimo fotograma completo)", last, want)
	}
}

func TestScanJPEGFramesEmpty(t *testing.T) {
	if count, last := scanJPEGFrames([]byte{0x00, 0x01, 0x02}); count != 0 || last != nil {
		t.Errorf("got (%d, %v), want (0, nil)", count, last)
	}
}
