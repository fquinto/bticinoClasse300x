package bticino

import (
	"fmt"
	"sync"
)

// Direcciones de bus de esta instalación usadas en las tramas WHO=8 de
// "abrir puerta" y "luz de escalera". Antes eran constantes fijas (20 y 16,
// las de la instalación de referencia); ahora se resuelven al arrancar
// (config > conf.xml del equipo > valor histórico) y se consultan aquí.

const (
	// DefaultDoorAddress es la placa exterior por defecto histórica (dev 2, address 0).
	DefaultDoorAddress = "20"
	// DefaultLightAddress es la dirección propia histórica de la unidad (dev 1, address 6).
	DefaultLightAddress = "16"

	maxAddressLen = 4
)

var (
	addrMu       sync.RWMutex
	doorAddress  = DefaultDoorAddress
	lightAddress = DefaultLightAddress
)

// ValidAddress indica si s es una dirección de bus utilizable en una trama:
// solo dígitos, de 1 a 4. Las tramas se envían por shell a nc, así que nada
// que no sea un número corto debe llegar a ellas.
func ValidAddress(s string) bool {
	if len(s) == 0 || len(s) > maxAddressLen {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// SetAddresses fija las direcciones de puerta y luz. Un valor no válido se
// ignora y se conserva el vigente.
func SetAddresses(door, light string) {
	addrMu.Lock()
	defer addrMu.Unlock()
	if ValidAddress(door) {
		doorAddress = door
	}
	if ValidAddress(light) {
		lightAddress = light
	}
}

// ResetAddresses vuelve a los valores históricos (para tests).
func ResetAddresses() {
	addrMu.Lock()
	defer addrMu.Unlock()
	doorAddress, lightAddress = DefaultDoorAddress, DefaultLightAddress
}

// DoorAddress devuelve la dirección vigente de la placa/cerradura principal.
func DoorAddress() string {
	addrMu.RLock()
	defer addrMu.RUnlock()
	return doorAddress
}

// LightAddress devuelve la dirección vigente para la luz de escalera (la propia).
func LightAddress() string {
	addrMu.RLock()
	defer addrMu.RUnlock()
	return lightAddress
}

// DoorOpenPressCmd es la trama de pulsar "abrir puerta" con la dirección vigente.
func DoorOpenPressCmd() string { return fmt.Sprintf("*8*19*%s##", DoorAddress()) }

// DoorOpenReleaseCmd es la trama de soltar "abrir puerta".
func DoorOpenReleaseCmd() string { return fmt.Sprintf("*8*20*%s##", DoorAddress()) }

// LightOnPressCmd es la trama de pulsar "luz de escalera" con la dirección vigente.
func LightOnPressCmd() string { return fmt.Sprintf("*8*21*%s##", LightAddress()) }

// LightOnReleaseCmd es la trama de soltar "luz de escalera".
func LightOnReleaseCmd() string { return fmt.Sprintf("*8*22*%s##", LightAddress()) }

// ResolveAddress elige una dirección por prioridad: valor forzado en la
// configuración, valor leído del conf.xml del equipo, valor histórico.
// Devuelve también de dónde salió ("config", "conf.xml" o "default").
func ResolveAddress(override, discovered, fallback string) (string, string) {
	if ValidAddress(override) {
		return override, "config"
	}
	if ValidAddress(discovered) {
		return discovered, "conf.xml"
	}
	return fallback, "default"
}
