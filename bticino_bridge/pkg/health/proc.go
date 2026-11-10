// Package health reúne comprobaciones de salud de solo lectura del equipo
// (procesos y puertos en escucha vía /proc) y el vigilante opcional de SSH.
// Nada aquí abre sockets hacia los servicios nativos.
package health

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Proc lee el pseudo-sistema de ficheros /proc (Root configurable para tests).
type Proc struct {
	Root string
}

func (p Proc) root() string {
	if p.Root == "" {
		return "/proc"
	}
	return p.Root
}

// ListeningPorts devuelve los puertos TCP en escucha (IPv4 e IPv6). Da error si
// no se puede leer /proc/net/tcp: "no lo sé" no es lo mismo que "nada escucha".
func (p Proc) ListeningPorts() (map[int]bool, error) {
	ports := map[int]bool{}
	if err := readListening(filepath.Join(p.root(), "net", "tcp"), ports); err != nil {
		return nil, err
	}
	_ = readListening(filepath.Join(p.root(), "net", "tcp6"), ports) // IPv6 puede no existir
	return ports, nil
}

func readListening(path string, ports map[int]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// "sl: local_address rem_address st ..." — st 0A = LISTEN
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 || fields[3] != "0A" {
			continue
		}
		i := strings.LastIndex(fields[1], ":")
		if i < 0 {
			continue
		}
		port, err := strconv.ParseUint(fields[1][i+1:], 16, 16)
		if err != nil {
			continue
		}
		ports[int(port)] = true
	}
	return sc.Err()
}

// FindProcess busca un proceso cuyo ejecutable (argv[0]) se llame name y
// devuelve su línea de comandos.
func (p Proc) FindProcess(name string) ([]string, bool) {
	entries, err := os.ReadDir(p.root())
	if err != nil {
		return nil, false
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(p.root(), e.Name(), "cmdline"))
		if err != nil || len(data) == 0 {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
		if filepath.Base(argv[0]) == name {
			return argv, true
		}
	}
	return nil, false
}

// FlexisipState es el resultado de la comprobación del proxy SIP local.
// Es comparable: sirve directamente como valor de Watch.
type FlexisipState struct {
	Problem          bool   // Flexisip no puede dar servicio (ni local ni hacia la nube)
	Detail           string // explicación legible
	Running          bool   // hay proceso flexisip
	SIPPortListening bool   // escucha en el puerto SIP local
	TLSPortListening bool   // escucha en 5061 (dato; la app/nube lo usan)
	PassphraseArg    bool   // arrancado con --p12-passphrase-file (dato)
}

// FlexisipTLSPort es el puerto SIP-TLS de Flexisip.
const FlexisipTLSPort = 5061

// CheckFlexisip comprueba, sin abrir conexiones, que Flexisip está en ejecución
// y escuchando en sipPort. Solo eso marca problema: sin Flexisip no hay llamadas
// ni registro contra la nube (el caso "la app dice no conectada"). El puerto TLS
// y el argumento de passphrase se informan como datos de diagnóstico, porque no
// está verificado que sean obligatorios en todas las unidades.
func CheckFlexisip(p Proc, sipPort int) FlexisipState {
	var s FlexisipState
	ports, err := p.ListeningPorts()
	if err != nil {
		s.Detail = fmt.Sprintf("no se pudo leer el estado del sistema: %v", err)
		return s
	}
	argv, running := p.FindProcess("flexisip")
	s.Running = running
	s.SIPPortListening = ports[sipPort]
	s.TLSPortListening = ports[FlexisipTLSPort]
	for _, a := range argv {
		if strings.HasPrefix(a, "--p12-passphrase-file") {
			s.PassphraseArg = true
		}
	}
	switch {
	case !running:
		s.Problem = true
		s.Detail = "Flexisip no está en ejecución: sin llamadas ni conexión con la nube"
	case !s.SIPPortListening:
		s.Problem = true
		s.Detail = fmt.Sprintf("Flexisip está en ejecución pero no escucha en el puerto %d", sipPort)
	default:
		s.Detail = "Flexisip en ejecución y escuchando"
	}
	return s
}
