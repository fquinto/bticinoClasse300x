package bticino

import "testing"

func TestIsCallEnd(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"*8*3#1#4*416##", true},
		{"*8*3#5#4*416##", true},
		{"*8*3#5#4*420##", true},
		{"*8*3#1#4*420##", true},
		{"*8*3#1#4*411##", true},
		{"*8*1#1#4#21*11##", false},
		{"*8*19*20##", false},
		{"*8*30#1#4*420##", false}, // WHAT 30#, no 3#
		{"*7*3#1#4*420##", false},  // otro WHO
		{"*8*3#1#4*420", false},    // sin terminador
		{"", false},
	}
	for _, tt := range tests {
		if got := IsCallEnd(tt.raw); got != tt.want {
			t.Errorf("IsCallEnd(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}
