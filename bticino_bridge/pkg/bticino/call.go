package bticino

import "strings"

// IsCallEnd indica si una trama OWN del bus es un fin de llamada: WHO=8
// (videoportero) con WHAT que empieza por "3#" (colgar). Casa cualquier WHERE,
// porque varía según la instalación (416, 420, 411...). Formas reales:
//
//	*8*3#1#4*420##   (fin de citofonata)
//	*8*3#1#4*411##   (fin de llamada contestada en el monitor)
//	*8*3#5#4*416##   (fin de auto-llamada)
//
// Función pura, solo clasifica; no envía nada al bus.
func IsCallEnd(raw string) bool {
	return strings.HasPrefix(raw, "*8*3#") && strings.HasSuffix(raw, "##")
}
