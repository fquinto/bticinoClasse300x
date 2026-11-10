package health

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DefaultSSHStartCommand arranca el servidor SSH del equipo.
const DefaultSSHStartCommand = "/etc/init.d/dropbear start"

// SplitCommand separa una orden en programa y argumentos por espacios (sin
// shell: no hay comillas, tuberías ni expansión). Vacía => nil.
func SplitCommand(cmd string) []string {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return nil
	}
	return f
}

// CommandTimeout es el tiempo máximo que se espera a una orden del sistema.
const CommandTimeout = 20 * time.Second

// RunCommand ejecuta la orden (sin shell) con un tiempo máximo.
func RunCommand(cmd string) error {
	argv := SplitCommand(cmd)
	if argv == nil {
		return fmt.Errorf("orden vacía")
	}
	return runArgv(argv, CommandTimeout)
}

// runArgv ejecuta argv con límite de tiempo. La salida va a /dev/null a
// propósito: un demonio que el script deje en segundo plano heredaría una
// tubería y dejaría la llamada esperando para siempre.
func runArgv(argv []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.WaitDelay = time.Second
	if err := c.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s: superó el tiempo máximo (%s)", strings.Join(argv, " "), timeout)
		}
		return fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
	}
	return nil
}

// PortListening devuelve un comprobador de "¿escucha alguien en este puerto?".
func PortListening(p Proc, port int) func() (bool, error) {
	return func() (bool, error) {
		ports, err := p.ListeningPorts()
		if err != nil {
			return false, err
		}
		return ports[port], nil
	}
}

// SSHWatchdog arranca el servidor SSH si lleva caído más de `grace`, y no lo
// reintenta antes de `backoff`. En este equipo el demonio nativo puede parar
// dropbear, dejando la unidad sin acceso; el vigilante lo repone sin tocar nada
// más. Si el estado no se puede conocer, no actúa.
type SSHWatchdog struct {
	grace, backoff time.Duration
	check          func() (bool, error)
	start          func() error
	now            func() time.Time

	mu          sync.Mutex
	downSince   time.Time
	lastAttempt time.Time
}

// NewSSHWatchdog crea el vigilante.
func NewSSHWatchdog(grace, backoff time.Duration, check func() (bool, error), start func() error) *SSHWatchdog {
	return &SSHWatchdog{grace: grace, backoff: backoff, check: check, start: start, now: time.Now}
}

// Tick hace una pasada. Devuelve true si intentó arrancar SSH, y el error del intento.
func (w *SSHWatchdog) Tick() (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	up, err := w.check()
	if err != nil {
		// Estado desconocido: no tocar, y la espera vuelve a empezar (debe
		// observarse caído de forma continua).
		w.downSince = time.Time{}
		return false, nil
	}
	now := w.now()
	if up {
		w.downSince = time.Time{}
		return false, nil
	}
	if w.downSince.IsZero() {
		w.downSince = now
		return false, nil
	}
	if now.Sub(w.downSince) < w.grace {
		return false, nil
	}
	if !w.lastAttempt.IsZero() && now.Sub(w.lastAttempt) < w.backoff {
		return false, nil
	}
	w.lastAttempt = now
	return true, w.start()
}
