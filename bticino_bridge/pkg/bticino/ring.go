package bticino

import "strings"

// Ring frames (observed on a Classe 300X13E, fw 1.7.19, unit address 11).
//
// A ring from an entrance panel shows up as two frames:
//
//	*8*1#1#4#21*11##   call to this unit (11). The "21" is the same for every
//	                   external call (it is identical for units 10, 11 and 16
//	                   in docs/openwebnet-commands.md); it is NOT the panel.
//	*8*9#1#4*20##      confirmation. WHERE is the calling panel (20, 21, ...),
//	                   exactly as in camera-on (*8*9#5#4*20##).
//
// Camera-on (autoaccensione) uses mode 5 instead of 1 (*8*1#5#4#20*11##).

// IsExternalRing reports whether a WHO/WHAT pair is an incoming external call.
func IsExternalRing(who, what string) bool {
	return who == "8" && strings.HasPrefix(what, "1#1#")
}

// RingPanel returns the calling entrance panel from the ring confirmation
// frame (*8*9#1#…*WHERE##), or false if the frame is not one.
func RingPanel(who, what, where string) (string, bool) {
	if who != "8" || !strings.HasPrefix(what, "9#1#") || where == "" {
		return "", false
	}
	return where, true
}
