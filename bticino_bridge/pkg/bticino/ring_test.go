package bticino

import "testing"

func TestIsExternalRing(t *testing.T) {
	cases := []struct {
		who, what string
		want      bool
	}{
		{"8", "1#1#4#21", true},  // real ring, this unit
		{"8", "1#1#4#22", true},  // other constant value still a ring
		{"8", "1#5#4#20", false}, // camera on (autoaccensione)
		{"8", "9#1#4", false},    // confirmation, not the call
		{"7", "1#1#4#21", false},
	}
	for _, c := range cases {
		if got := IsExternalRing(c.who, c.what); got != c.want {
			t.Errorf("IsExternalRing(%q,%q) = %v, want %v", c.who, c.what, got, c.want)
		}
	}
}

func TestRingPanel(t *testing.T) {
	if p, ok := RingPanel("8", "9#1#4", "20"); !ok || p != "20" {
		t.Errorf("ring confirmation from 20: got %q %v", p, ok)
	}
	if p, ok := RingPanel("8", "9#1#4", "21"); !ok || p != "21" {
		t.Errorf("ring confirmation from 21: got %q %v", p, ok)
	}
	if _, ok := RingPanel("8", "9#5#4", "20"); ok {
		t.Error("camera-on confirmation must not count as a ring")
	}
	if _, ok := RingPanel("8", "1#1#4#21", "11"); ok {
		t.Error("call frame must not count as confirmation")
	}
}
