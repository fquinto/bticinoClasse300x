package bticino

import "testing"

func TestAddressDefaultsMatchLegacyConstants(t *testing.T) {
	ResetAddresses()
	if DoorOpenPressCmd() != CmdDoorOpenPress || DoorOpenReleaseCmd() != CmdDoorOpenRelease {
		t.Errorf("puerta por defecto: %s / %s", DoorOpenPressCmd(), DoorOpenReleaseCmd())
	}
	if LightOnPressCmd() != CmdLightOnPress || LightOnReleaseCmd() != CmdLightOnRelease {
		t.Errorf("luz por defecto: %s / %s", LightOnPressCmd(), LightOnReleaseCmd())
	}
}

func TestSetAddressesChangesCommands(t *testing.T) {
	defer ResetAddresses()
	SetAddresses("21", "13")
	if got := DoorOpenPressCmd(); got != "*8*19*21##" {
		t.Errorf("press = %s", got)
	}
	if got := DoorOpenReleaseCmd(); got != "*8*20*21##" {
		t.Errorf("release = %s", got)
	}
	if got := LightOnPressCmd(); got != "*8*21*13##" {
		t.Errorf("light press = %s", got)
	}
	if got := LightOnReleaseCmd(); got != "*8*22*13##" {
		t.Errorf("light release = %s", got)
	}
}

// Un valor no numérico nunca debe acabar dentro de una trama (se envía por shell a nc).
func TestSetAddressesIgnoresInvalid(t *testing.T) {
	defer ResetAddresses()
	SetAddresses("20##;reboot", "")
	if DoorOpenPressCmd() != CmdDoorOpenPress || LightOnPressCmd() != CmdLightOnPress {
		t.Errorf("valor inválido aceptado: %s %s", DoorOpenPressCmd(), LightOnPressCmd())
	}
}

func TestResolveAddress(t *testing.T) {
	cases := []struct{ override, discovered, fallback, want, src string }{
		{"21", "20", "20", "21", "config"},
		{"", "23", "20", "23", "conf.xml"},
		{"", "", "20", "20", "default"},
		{"abc", "23", "20", "23", "conf.xml"},
		{"", "x;y", "20", "20", "default"},
	}
	for _, c := range cases {
		got, src := ResolveAddress(c.override, c.discovered, c.fallback)
		if got != c.want || src != c.src {
			t.Errorf("ResolveAddress(%q,%q,%q) = %q,%q; want %q,%q", c.override, c.discovered, c.fallback, got, src, c.want, c.src)
		}
	}
}
