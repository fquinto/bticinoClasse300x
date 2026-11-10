package survival

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// scriptPath localiza el survival.sh autoritativo (patches/bticino_bridge) a
// partir de la ubicación de este fichero de test, independientemente del CWD.
func scriptPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(thisFile), "../../../patches/bticino_bridge/survival.sh")
	if _, err := os.Stat(p); err != nil {
		// El script se distribuye aparte (patches/bticino_bridge). Si este
		// árbol no lo trae, los tests que lo ejecutan no aplican.
		t.Skipf("survival.sh no está en este árbol (%s): test omitido", p)
	}
	return p
}

type fakeEnv struct {
	root   string
	script string
	hosts  string
}

// setupFakeRoot crea un rootfs falso (hook OTA, /etc/hosts, ota-hosts.txt).
func setupFakeRoot(t *testing.T) fakeEnv {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	mustMkdir(t, filepath.Join(root, "etc/rcS.d"))
	mustMkdir(t, filepath.Join(root, "home/bticino/cfg/extra"))
	mustWrite(t, filepath.Join(root, "etc/rcS.d/S96-dedrifu.sh"), "#!/bin/sh\n# OTA hook\n")
	mustWrite(t, filepath.Join(root, "etc/hosts"), "127.0.0.1\tlocalhost\n192.168.1.2\tbroker-box\n")
	mustWrite(t, filepath.Join(root, "home/bticino/cfg/extra/ota-hosts.txt"),
		"# hosts OTA genéricos\nc300x.bs.iotleg.com\ndispatchregistration.legrand.com\n")
	return fakeEnv{root: root, script: scriptPath(t), hosts: filepath.Join(root, "etc/hosts")}
}

func (e fakeEnv) run(t *testing.T, arg string) string {
	t.Helper()
	cmd := exec.Command("sh", e.script, arg)
	cmd.Env = append(os.Environ(),
		"SURVIVAL_ROOT="+e.root,
		"OTA_HOSTS_FILE="+filepath.Join(e.root, "home/bticino/cfg/extra/ota-hosts.txt"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("survival.sh %s: %v\n%s", arg, err, out)
	}
	return strings.TrimSpace(string(out))
}

// snapshot produce una huella estable del árbol root, para comprobar que revert
// restaura el estado original byte a byte.
func snapshot(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if info.IsDir() {
			lines = append(lines, "d "+rel)
			return nil
		}
		data, _ := os.ReadFile(p)
		lines = append(lines, fmt.Sprintf("f %s %o %x", rel, info.Mode().Perm(), sha256.Sum256(data)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestSurvivalApplyRevertRoundTrip(t *testing.T) {
	e := setupFakeRoot(t)
	before := snapshot(t, e.root)

	e.run(t, "apply")
	if s := e.run(t, "status"); s != "on" {
		t.Fatalf("tras apply, status debe ser on, got %q", s)
	}
	// El sinkhole debe contener los hosts OTA, no tocar las entradas existentes.
	hosts, _ := os.ReadFile(e.hosts)
	if !strings.Contains(string(hosts), "127.0.0.1 c300x.bs.iotleg.com") {
		t.Fatalf("el sinkhole debe incluir los hosts OTA: %s", hosts)
	}
	if !strings.Contains(string(hosts), "broker-box") {
		t.Fatal("apply no debe perder las entradas existentes de /etc/hosts")
	}

	e.run(t, "apply") // idempotente
	e.run(t, "revert")
	if s := e.run(t, "status"); s != "off" {
		t.Fatalf("tras revert, status debe ser off, got %q", s)
	}
	after := snapshot(t, e.root)
	if after != before {
		t.Fatalf("revert no restauró el estado original:\n--- antes ---\n%s\n--- después ---\n%s", before, after)
	}
}

// TestSurvivalNoIptablesNoLoopbackHarm documenta que survival.sh NO instala
// reglas iptables (evita el REJECT a 127.0.0.1 por resolución contra el
// sinkhole): el script no debe invocar iptables en absoluto.
func TestSurvivalNoIptablesReferences(t *testing.T) {
	data, err := os.ReadFile(scriptPath(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue // comentarios pueden mencionar iptables para explicar por qué no se usa
		}
		if strings.Contains(trimmed, "iptables") {
			t.Fatalf("survival.sh no debe ejecutar iptables: %q", line)
		}
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
