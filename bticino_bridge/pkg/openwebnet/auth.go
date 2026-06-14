// Autenticacion HMAC-SHA256 para OpenWebNet (protocolo BTicino Classe 300X).
//
// Implementado a partir del analisis de slyoldfox/c300x-controller (lib/openwebnet.js).
//
// Protocolo de autenticacion:
//  1. Cliente envia *99*0## (solicitud de sesion COMMAND)
//  2. Servidor responde *98*2## (indica que requiere auth HMAC-SHA256)
//     - En localhost, puede responder *#*1## directamente (sin auth)
//  3. Cliente responde *#*1## (acepta HMAC)
//  4. Servidor envia *#NONCE## (nonce Ra codificado en digitos OWN)
//  5. Cliente calcula:
//     - Ra = digitToHex(nonce del servidor)
//     - Rb = SHA256("time" + timestamp) (nonce generado por cliente)
//     - Kab = SHA256(password)
//     - HMAC = SHA256(Ra + Rb + A + B + Kab)
//     donde A="736F70653E", B="636F70653E"
//  6. Cliente envia: *#<hexToDigit(Rb)>*<hexToDigit(HMAC)>##
//  7. Servidor envia su propio HMAC (validacion del servidor)
//  8. Cliente responde *#*1## (acepta)
//  9. Servidor confirma con *#*1## -> autenticado
package openwebnet

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Constantes fijas del protocolo HMAC-SHA256 BTicino
const (
	// Valores fijos A y B para el calculo HMAC.
	// Decodificados: A = "sope>", B = "cope>"
	HMAC_A = "736F70653E"
	HMAC_B = "636F70653E"

	// Comandos de autenticacion OpenWebNet
	CMD_SESSION_COMMAND = "*99*0##" // Solicitar sesion de comando
	CMD_SESSION_EVENT   = "*99*1##" // Solicitar sesion de eventos
	CMD_AUTH_HMAC       = "*98*2##" // Servidor requiere auth HMAC-SHA256
	CMD_ACK             = "*#*1##"  // Acknowledgment
	CMD_NACK            = "*#*0##"  // Negative acknowledgment
)

// digitToHex convierte una cadena de digitos OWN a hexadecimal.
// Cada grupo de 4 digitos decimales codifica 2 caracteres hex.
// Ejemplo: "0709" -> hex: (07=07, 09=09) -> "0709"
// Mas claro: digitos[0]*10 + digitos[1] -> primer nibble hex,
//
//	digitos[2]*10 + digitos[3] -> segundo nibble hex.
//
// Ej: "0100" -> (0*10+1=1 -> "1") + (0*10+0=0 -> "0") -> "10"
func digitToHex(digits string) string {
	var out strings.Builder
	chars := []rune(digits)

	for i := 0; i+3 < len(chars); i += 4 {
		d0, _ := strconv.Atoi(string(chars[i]))
		d1, _ := strconv.Atoi(string(chars[i+1]))
		d2, _ := strconv.Atoi(string(chars[i+2]))
		d3, _ := strconv.Atoi(string(chars[i+3]))

		// Cada par de digitos codifica un valor 0-15 (un nibble hex)
		val1 := d0*10 + d1
		val2 := d2*10 + d3

		out.WriteString(fmt.Sprintf("%x%x", val1, val2))
	}

	return out.String()
}

// hexToDigit convierte una cadena hexadecimal a digitos OWN.
// Cada caracter hex (0-f, valor 0-15) se codifica como 2 digitos decimales.
// Ej: "a" (valor 10) -> "10", "f" (valor 15) -> "15", "0" -> "00"
func hexToDigit(hexString string) string {
	var out strings.Builder

	for _, c := range hexString {
		hexValue, err := strconv.ParseInt(string(c), 16, 64)
		if err != nil {
			out.WriteString("00")
			continue
		}
		if hexValue < 10 {
			out.WriteByte('0')
		}
		out.WriteString(strconv.FormatInt(hexValue, 10))
	}

	return out.String()
}

// sha256Hex calcula SHA-256 de un string y devuelve el hash en hex.
func sha256Hex(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// HMACAuth realiza la autenticacion HMAC-SHA256 completa en una conexion OWN.
//
// Precondicion: la conexion ya ha recibido el ACK inicial (*#*1##) y se ha enviado
// *99*0## y recibido *98*2## como respuesta (indicando que se requiere HMAC).
//
// Parametros:
//   - reader: lector buferizado de la conexion TCP
//   - writer: funcion para escribir bytes en la conexion
//   - password: contrasena del dispositivo
//   - timeout: timeout para lecturas
//
// Retorna error si la autenticacion falla.
func HMACAuth(reader *bufio.Reader, writer func([]byte) error, password string, timeout time.Duration) error {
	// Paso 1: Enviar ACK para aceptar challenge HMAC
	if err := writer([]byte(CMD_ACK)); err != nil {
		return fmt.Errorf("error enviando ACK para HMAC: %w", err)
	}

	// Paso 2: Leer nonce (Ra) del servidor: *#DIGITS##
	nonceMsg, err := readOWNMessage(reader)
	if err != nil {
		return fmt.Errorf("error leyendo nonce del servidor: %w", err)
	}
	nonceMsg = strings.TrimSpace(nonceMsg)

	// Extraer digitos del nonce: quitar "*#" del inicio y "##" del final
	if !strings.HasPrefix(nonceMsg, "*#") || !strings.HasSuffix(nonceMsg, "##") {
		return fmt.Errorf("formato de nonce inesperado: %s", nonceMsg)
	}
	nonceDigits := strings.TrimPrefix(nonceMsg, "*#")
	nonceDigits = strings.TrimSuffix(nonceDigits, "##")

	// Paso 3: Calcular valores de autenticacion
	ra := digitToHex(nonceDigits)
	rb := sha256Hex(fmt.Sprintf("time%d", time.Now().UnixMilli()))
	kab := sha256Hex(password)

	hmac := sha256Hex(ra + rb + HMAC_A + HMAC_B + kab)

	// Paso 4: Enviar respuesta: *#<Rb_digits>*<HMAC_digits>##
	rbDigits := hexToDigit(rb)
	hmacDigits := hexToDigit(hmac)

	authResponse := fmt.Sprintf("*#%s*%s##", rbDigits, hmacDigits)
	if err := writer([]byte(authResponse)); err != nil {
		return fmt.Errorf("error enviando respuesta HMAC: %w", err)
	}

	// Paso 5: Leer HMAC del servidor (validacion del servidor hacia nosotros)
	serverHMAC, err := readOWNMessage(reader)
	if err != nil {
		return fmt.Errorf("error leyendo HMAC del servidor: %w", err)
	}
	serverHMAC = strings.TrimSpace(serverHMAC)

	// Paso 6: Enviar ACK para aceptar el HMAC del servidor
	if err := writer([]byte(CMD_ACK)); err != nil {
		return fmt.Errorf("error enviando ACK final: %w", err)
	}

	// Paso 7: Leer resultado final: *#*1## = exito, *#*0## = fallo
	result, err := readOWNMessage(reader)
	if err != nil {
		return fmt.Errorf("error leyendo resultado de autenticacion: %w", err)
	}
	result = strings.TrimSpace(result)

	if result != CMD_ACK {
		return fmt.Errorf("autenticacion HMAC fallida: servidor respondio %s", result)
	}

	return nil
}

// IsLocalhost determina si el host es localhost (no requiere auth HMAC).
func IsLocalhost(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "0" || host == "0.0.0.0"
}
