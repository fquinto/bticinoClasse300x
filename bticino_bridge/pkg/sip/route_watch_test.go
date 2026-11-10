package sip

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func writeRoute(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Flexisip lee route.conf, un enlace que el equipo apunta a route_ext.conf o
// route_int.conf: hay que comprobar el fichero al que apunta ahora.
func TestActiveRouteFile_FollowsRouteConfSymlink(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(dir, "route_ext.conf")
	in := filepath.Join(dir, "route_int.conf")
	writeRoute(t, ext, "x")
	writeRoute(t, in, "y")
	if err := os.Symlink(in, filepath.Join(dir, "route.conf")); err != nil {
		t.Skip("sin symlinks:", err)
	}
	got := ActiveRouteFile(ext)
	want, _ := filepath.EvalSymlinks(in)
	if got != want {
		t.Errorf("ActiveRouteFile = %s, want %s", got, want)
	}
}

func TestActiveRouteFile_NoSymlinkKeepsConfigured(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(dir, "route_ext.conf")
	writeRoute(t, ext, "x")
	if got := ActiveRouteFile(ext); got != ext {
		t.Errorf("sin route.conf: got %s, want %s", got, ext)
	}
	// Enlace roto: tampoco debe inventarse otra ruta.
	if err := os.Symlink(filepath.Join(dir, "no-existe"), filepath.Join(dir, "route.conf")); err == nil {
		if got := ActiveRouteFile(ext); got != ext {
			t.Errorf("enlace roto: got %s, want %s", got, ext)
		}
	}
}

type routeLog struct {
	mu  sync.Mutex
	got []RouteState
}

func (l *routeLog) add(s RouteState) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.got = append(l.got, s)
}

func (l *routeLog) snapshot() []RouteState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]RouteState(nil), l.got...)
}

func waitRoute(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout esperando: %s", what)
}

func TestRouteWatcher_ReportsInitialStateThenOnlyChanges(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(dir, "route_ext.conf")
	const dom = "example.invalid"
	withAOR := "<sip:alluser@" + dom + "> <sip:a@" + dom + ">, <sip:bridge@" + dom + ">\n"
	withoutAOR := "<sip:alluser@" + dom + "> <sip:a@" + dom + ">\n"
	writeRoute(t, ext, withAOR)

	l := &routeLog{}
	w := NewRouteWatcher(ext, "bridge", dom, 10*time.Millisecond, l.add)
	w.Start()
	defer w.Stop()

	waitRoute(t, "estado inicial", func() bool { return len(l.snapshot()) == 1 })
	if s := l.snapshot()[0]; !s.OK || s.File != ext {
		t.Fatalf("inicial: %+v", s)
	}
	time.Sleep(60 * time.Millisecond) // varias pasadas sin cambios: no repite
	if n := len(l.snapshot()); n != 1 {
		t.Fatalf("sin cambios emitió %d estados", n)
	}

	writeRoute(t, ext, withoutAOR) // p. ej. logout/login de la app reescribe el fichero
	waitRoute(t, "ruta perdida", func() bool { return len(l.snapshot()) == 2 })
	if s := l.snapshot()[1]; s.OK || s.Detail == "" {
		t.Fatalf("ruta perdida: %+v", s)
	}

	writeRoute(t, ext, withAOR)
	waitRoute(t, "ruta recuperada", func() bool { return len(l.snapshot()) == 3 })
	if s := l.snapshot()[2]; !s.OK {
		t.Fatalf("recuperada: %+v", s)
	}
}

func TestRouteWatcher_UnreadableFileIsAProblem(t *testing.T) {
	l := &routeLog{}
	w := NewRouteWatcher(filepath.Join(t.TempDir(), "nope.conf"), "bridge", "example.invalid", time.Hour, l.add)
	w.Start()
	defer w.Stop()
	waitRoute(t, "estado inicial", func() bool { return len(l.snapshot()) == 1 })
	if s := l.snapshot()[0]; s.OK || s.Detail == "" {
		t.Fatalf("fichero ilegible: %+v", s)
	}
}

func TestRouteWatcher_StopIsIdempotent(t *testing.T) {
	w := NewRouteWatcher(filepath.Join(t.TempDir(), "x"), "u", "d", time.Hour, func(RouteState) {})
	w.Start()
	w.Stop()
	w.Stop()
}

func TestRouteWatcher_StopWithoutStartDoesNotBlock(t *testing.T) {
	w := NewRouteWatcher(filepath.Join(t.TempDir(), "x"), "u", "d", time.Hour, func(RouteState) {})
	done := make(chan struct{})
	go func() { w.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop sin Start se quedó bloqueado")
	}
}
