package sip

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultRouteCheckInterval es cada cuánto se re-comprueba la ruta del contestador.
const DefaultRouteCheckInterval = 60 * time.Second

// ActiveRouteFile devuelve el fichero de rutas que Flexisip usa de verdad.
// Flexisip lee route.conf, un enlace que el equipo apunta a route_ext.conf o
// route_int.conf; si ese enlace existe junto al fichero configurado y resuelve,
// se devuelve su destino. Si no, el fichero configurado.
func ActiveRouteFile(configured string) string {
	link := filepath.Join(filepath.Dir(configured), "route.conf")
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return configured
	}
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		return configured
	}
	return target
}

// RouteState es el resultado de una comprobación de la ruta del contestador.
type RouteState struct {
	OK     bool   // el AOR del bridge está en la línea alluser del fichero activo
	File   string // fichero comprobado
	Detail string // motivo cuando OK=false
}

// RouteWatcher re-comprueba periódicamente (solo lectura) que el AOR del
// contestador sigue en el fichero de rutas activo y avisa cuando el estado
// cambia. El fichero lo reescribe el propio equipo (p. ej. al cerrar/abrir
// sesión en la app), así que una comprobación única al arrancar no basta.
type RouteWatcher struct {
	routeFile, user, domain string
	interval                time.Duration
	onChange                func(RouteState)

	startOnce sync.Once
	stopOnce  sync.Once
	stop      chan struct{}
	done      chan struct{}
}

// NewRouteWatcher crea el vigilante; onChange recibe el estado inicial y
// después solo los cambios.
func NewRouteWatcher(routeFile, user, domain string, interval time.Duration, onChange func(RouteState)) *RouteWatcher {
	if interval <= 0 {
		interval = DefaultRouteCheckInterval
	}
	return &RouteWatcher{
		routeFile: routeFile, user: user, domain: domain,
		interval: interval, onChange: onChange,
		stop: make(chan struct{}), done: make(chan struct{}),
	}
}

// Check hace una comprobación puntual.
func (w *RouteWatcher) Check() RouteState {
	file := ActiveRouteFile(w.routeFile)
	ok, err := CheckAnswererRoute(file, w.user, w.domain)
	switch {
	case err != nil:
		return RouteState{File: file, Detail: fmt.Sprintf("no se pudo leer el fichero de rutas: %v", err)}
	case !ok:
		return RouteState{File: file, Detail: fmt.Sprintf("el usuario %q no está en la línea alluser de %s", w.user, filepath.Base(file))}
	}
	return RouteState{OK: true, File: file}
}

// Start lanza el bucle de comprobación.
func (w *RouteWatcher) Start() {
	w.startOnce.Do(w.run)
}

func (w *RouteWatcher) run() {
	go func() {
		defer close(w.done)
		last := w.Check()
		w.onChange(last)
		t := time.NewTicker(w.interval)
		defer t.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-t.C:
				if cur := w.Check(); cur != last {
					last = cur
					w.onChange(cur)
				}
			}
		}
	}()
}

// Stop detiene el bucle y espera a que termine. Idempotente, y seguro aunque
// Start no se haya llamado.
func (w *RouteWatcher) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	w.startOnce.Do(func() { close(w.done) })
	<-w.done
}
