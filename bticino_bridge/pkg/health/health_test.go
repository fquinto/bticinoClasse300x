package health

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeProc monta un /proc mínimo: procesos (pid -> argv) y sockets TCP en escucha.
func fakeProc(t *testing.T, procs map[string][]string, listen4, listen6 []int) Proc {
	t.Helper()
	root := t.TempDir()
	for pid, argv := range procs {
		dir := filepath.Join(root, pid)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(strings.Join(argv, "\x00")+"\x00"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	const hdr = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
	v4 := hdr
	for i, p := range listen4 {
		v4 += tcpLine(i, "00000000", p, "0A")
	}
	// Una conexión establecida (st 01) con puerto local 9999: no es escucha.
	v4 += tcpLine(99, "0100007F", 9999, "01")
	v6 := hdr
	for i, p := range listen6 {
		v6 += tcpLine(i, "00000000000000000000000000000000", p, "0A")
	}
	os.WriteFile(filepath.Join(root, "net", "tcp"), []byte(v4), 0o644)
	os.WriteFile(filepath.Join(root, "net", "tcp6"), []byte(v6), 0o644)
	return Proc{Root: root}
}

func tcpLine(i int, addr string, port int, st string) string {
	const hex = "0123456789ABCDEF"
	p := []byte{hex[port>>12&15], hex[port>>8&15], hex[port>>4&15], hex[port&15]}
	return "   " + string(rune('0'+i%10)) + ": " + addr + ":" + string(p) + " 00000000:0000 " + st + " 00000000:00000000 00:00000000 00000000     0        0 1234 1 0000000000000000 100 0 0 10 0\n"
}

func TestListeningPorts(t *testing.T) {
	p := fakeProc(t, nil, []int{5060, 22}, []int{8082})
	ports, err := p.ListeningPorts()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{5060, 22, 8082} {
		if !ports[want] {
			t.Errorf("falta el puerto %d en %v", want, ports)
		}
	}
	if ports[9999] {
		t.Error("una conexión establecida no es un puerto en escucha")
	}
}

func TestListeningPorts_NoProcIsError(t *testing.T) {
	if _, err := (Proc{Root: filepath.Join(t.TempDir(), "nope")}).ListeningPorts(); err == nil {
		t.Error("sin /proc/net/tcp debe dar error, no 'nada escucha'")
	}
}

func TestFindProcess(t *testing.T) {
	p := fakeProc(t, map[string][]string{
		"1":   {"/sbin/init"},
		"812": {"/usr/bin/flexisip", "--p12-passphrase-file", "/var/tmp/fifo"},
		"abc": {"flexisip-no-es-un-pid"},
	}, nil, nil)
	argv, ok := p.FindProcess("flexisip")
	if !ok || len(argv) != 3 || argv[1] != "--p12-passphrase-file" {
		t.Fatalf("argv=%v ok=%v", argv, ok)
	}
	if _, ok := p.FindProcess("dropbear"); ok {
		t.Error("dropbear no existe")
	}
}

func TestCheckFlexisip(t *testing.T) {
	flexi := map[string][]string{"812": {"/usr/bin/flexisip", "--p12-passphrase-file", "/x"}}

	ok := CheckFlexisip(fakeProc(t, flexi, []int{5060, 5061}, nil), 5060)
	if ok.Problem || !ok.Running || !ok.TLSPortListening || !ok.PassphraseArg {
		t.Errorf("sano: %+v", ok)
	}

	down := CheckFlexisip(fakeProc(t, nil, []int{5060}, nil), 5060)
	if !down.Problem || down.Running || down.Detail == "" {
		t.Errorf("sin proceso: %+v", down)
	}

	deaf := CheckFlexisip(fakeProc(t, flexi, []int{22}, nil), 5060)
	if !deaf.Problem || !deaf.Running || deaf.Detail == "" {
		t.Errorf("sin puerto SIP: %+v", deaf)
	}

	// Sin 5061 o sin el argumento de passphrase NO se marca problema (no está
	// verificado que en todo C300X sean obligatorios): quedan como datos.
	noTLS := CheckFlexisip(fakeProc(t, map[string][]string{"9": {"flexisip"}}, []int{5060}, nil), 5060)
	if noTLS.Problem || noTLS.TLSPortListening || noTLS.PassphraseArg {
		t.Errorf("sin 5061: %+v", noTLS)
	}

	// Si no se puede leer /proc no se inventa un problema.
	blind := CheckFlexisip(Proc{Root: filepath.Join(t.TempDir(), "nope")}, 5060)
	if blind.Problem || blind.Detail == "" {
		t.Errorf("sin /proc: %+v", blind)
	}
}

func TestWatch_InitialThenChangesOnly(t *testing.T) {
	var mu sync.Mutex
	val := 1
	var got []int
	stop := Watch(5*time.Millisecond, func() int { mu.Lock(); defer mu.Unlock(); return val },
		func(v int) { mu.Lock(); got = append(got, v); mu.Unlock() })
	defer stop()
	snap := func() []int { mu.Lock(); defer mu.Unlock(); return append([]int(nil), got...) }
	wait(t, func() bool { return len(snap()) == 1 })
	time.Sleep(40 * time.Millisecond)
	if n := len(snap()); n != 1 {
		t.Fatalf("sin cambios emitió %d", n)
	}
	mu.Lock()
	val = 2
	mu.Unlock()
	wait(t, func() bool { return len(snap()) == 2 })
	if s := snap(); s[0] != 1 || s[1] != 2 {
		t.Fatalf("got %v", s)
	}
	stop()
	stop() // idempotente
}

func wait(t *testing.T, cond func() bool) {
	t.Helper()
	for dl := time.Now().Add(2 * time.Second); time.Now().Before(dl); time.Sleep(3 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("timeout")
}

type sshFake struct {
	listening bool
	err       error
	starts    int
}

func (f *sshFake) check() (bool, error) { return f.listening, f.err }
func (f *sshFake) start() error         { f.starts++; return nil }

func TestSSHWatchdog_StartsOnlyAfterGraceAndBacksOff(t *testing.T) {
	f := &sshFake{}
	now := time.Unix(1000, 0)
	w := NewSSHWatchdog(5*time.Minute, 10*time.Minute, f.check, f.start)
	w.now = func() time.Time { return now }

	w.Tick() // primera vez que se ve caído: empieza a contar
	now = now.Add(4 * time.Minute)
	w.Tick()
	if f.starts != 0 {
		t.Fatalf("arrancó antes de la espera (%d)", f.starts)
	}
	now = now.Add(2 * time.Minute) // 6 min caído
	w.Tick()
	if f.starts != 1 {
		t.Fatalf("debía arrancar tras 5 min caído, starts=%d", f.starts)
	}
	now = now.Add(6 * time.Minute) // sigue caído, pero dentro del backoff
	w.Tick()
	if f.starts != 1 {
		t.Fatalf("reintentó dentro del backoff, starts=%d", f.starts)
	}
	now = now.Add(5 * time.Minute) // 11 min desde el intento
	w.Tick()
	if f.starts != 2 {
		t.Fatalf("debía reintentar pasado el backoff, starts=%d", f.starts)
	}
}

func TestSSHWatchdog_RecoveryResetsTheClock(t *testing.T) {
	f := &sshFake{}
	now := time.Unix(1000, 0)
	w := NewSSHWatchdog(5*time.Minute, 10*time.Minute, f.check, f.start)
	w.now = func() time.Time { return now }
	w.Tick()
	now = now.Add(4 * time.Minute)
	f.listening = true
	w.Tick() // se recuperó solo
	f.listening = false
	now = now.Add(2 * time.Minute)
	w.Tick() // vuelve a caer: la cuenta empieza de cero
	if f.starts != 0 {
		t.Fatalf("una caída breve tras recuperarse no debe arrancar nada, starts=%d", f.starts)
	}
}

// Si no se puede saber si escucha (no hay /proc), no se toca nada.
func TestSSHWatchdog_UnknownStateNeverStarts(t *testing.T) {
	f := &sshFake{err: os.ErrNotExist}
	now := time.Unix(1000, 0)
	w := NewSSHWatchdog(time.Minute, time.Minute, f.check, f.start)
	w.now = func() time.Time { return now }
	for i := 0; i < 10; i++ {
		now = now.Add(time.Minute)
		w.Tick()
	}
	if f.starts != 0 {
		t.Fatalf("arrancó sin saber el estado, starts=%d", f.starts)
	}
}

func TestSplitCommand(t *testing.T) {
	if got := SplitCommand("  /etc/init.d/dropbear   start "); len(got) != 2 || got[0] != "/etc/init.d/dropbear" || got[1] != "start" {
		t.Errorf("got %q", got)
	}
	if got := SplitCommand("   "); got != nil {
		t.Errorf("vacío debe ser nil, got %q", got)
	}
}

func TestRunArgv_TimesOut(t *testing.T) {
	start := time.Now()
	err := runArgv([]string{"sleep", "5"}, 100*time.Millisecond)
	if err == nil {
		t.Fatal("debía fallar por tiempo")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("tardó %v: no respeta el límite", d)
	}
}

// Un hijo en segundo plano que hereda la salida no debe dejar colgada la llamada.
func TestRunArgv_BackgroundChildDoesNotBlock(t *testing.T) {
	start := time.Now()
	if err := runArgv([]string{"sh", "-c", "sleep 5 &"}, 3*time.Second); err != nil {
		t.Fatalf("err = %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("tardó %v: se quedó esperando al hijo", d)
	}
}

// "No lo sé" interrumpe la cuenta: la espera debe observarse de forma continua.
func TestSSHWatchdog_UnknownResetsGrace(t *testing.T) {
	f := &sshFake{}
	now := time.Unix(1000, 0)
	w := NewSSHWatchdog(5*time.Minute, 10*time.Minute, f.check, f.start)
	w.now = func() time.Time { return now }
	w.Tick() // caído
	f.err = os.ErrNotExist
	now = now.Add(30 * time.Minute)
	w.Tick() // desconocido
	f.err = nil
	now = now.Add(time.Minute)
	w.Tick() // caído otra vez: empieza a contar ahora
	if f.starts != 0 {
		t.Fatalf("arrancó sin espera continua, starts=%d", f.starts)
	}
}
