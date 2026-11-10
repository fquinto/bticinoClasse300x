package deviceconfig

import (
	"bticino_bridge/pkg/bticino"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

// VDEAddresses son las direcciones de videoportero que el propio equipo guarda
// en su conf.xml (<setup><vdes><communication>).
//
// Cada dirección es la concatenación de <dev> y <address>: en la instalación
// de referencia la unidad es dev=1/address=6 (WHERE 16) y la placa por defecto
// dev=2/address=0 (WHERE 20), que coinciden con las tramas validadas en vivo.
// Con un único equipo de muestra, la regla de concatenación es una inferencia:
// por eso quien la usa conserva el valor histórico como respaldo y permite
// forzarla desde la configuración.
type VDEAddresses struct {
	Own   string // dirección propia de la unidad interior (luz de escalera)
	Panel string // placa exterior por defecto (abrir puerta, cámara)
}

// ReadVDEAddresses lee las direcciones del conf.xml indicado.
func ReadVDEAddresses(path string) (VDEAddresses, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return VDEAddresses{}, fmt.Errorf("leyendo %s: %w", path, err)
	}
	return ParseVDEAddresses(data), nil
}

// ParseVDEAddresses extrae las direcciones de un conf.xml. Nunca falla: lo que
// no se pueda leer o no sea numérico queda vacío.
func ParseVDEAddresses(data []byte) VDEAddresses {
	const base = "configuratore/setup/vdes/communication/"
	vals := map[string]string{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var path []string
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			// Solo vale un documento completo: un conf.xml a medio escribir
			// podría dar una dirección cortada pero con pinta de válida.
			if err != io.EOF || len(path) != 0 {
				return VDEAddresses{}
			}
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			path = append(path, t.Name.Local)
			text.Reset()
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			// El valor se acepta al ver su etiqueta de cierre.
			if p := strings.Join(path, "/"); strings.HasPrefix(p, base) {
				if s := strings.TrimSpace(text.String()); s != "" {
					vals[strings.TrimPrefix(p, base)] = s
				}
			}
			text.Reset()
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		}
	}
	return VDEAddresses{
		Own:   joinAddress(vals["dev"], vals["address"]),
		Panel: joinAddress(vals["p_default/dev"], vals["p_default/address"]),
	}
}

// joinAddress concatena dev+address si ambos son numéricos y el resultado es
// una dirección corta (máx. 4 dígitos); si no, devuelve "".
func joinAddress(dev, address string) string {
	if !allDigits(dev) || !allDigits(address) {
		return ""
	}
	if s := dev + address; len(s) <= 4 {
		return s
	}
	return ""
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// BusAddresses es el resultado de resolver las direcciones de la instalación.
type BusAddresses struct {
	Door, DoorSource   string
	Light, LightSource string
	ReadErr            error // error leyendo conf.xml (nil si se leyó)
}

// ApplyBusAddresses resuelve las direcciones de puerta y luz (configuración >
// conf.xml > valor histórico) y las deja vigentes en el paquete bticino.
func ApplyBusAddresses(doorOverride, lightOverride, confXMLPath string) BusAddresses {
	disc, err := ReadVDEAddresses(confXMLPath)
	var r BusAddresses
	r.ReadErr = err
	r.Door, r.DoorSource = bticino.ResolveAddress(doorOverride, disc.Panel, bticino.DefaultDoorAddress)
	r.Light, r.LightSource = bticino.ResolveAddress(lightOverride, disc.Own, bticino.DefaultLightAddress)
	bticino.SetAddresses(r.Door, r.Light)
	return r
}
