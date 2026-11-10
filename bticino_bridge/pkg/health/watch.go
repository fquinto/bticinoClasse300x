package health

import (
	"sync"
	"time"
)

// Watch ejecuta check cada interval y llama a onChange con el valor inicial y,
// después, solo cuando cambia. Devuelve una función de parada idempotente que
// espera a que el bucle termine.
func Watch[T comparable](interval time.Duration, check func() T, onChange func(T)) (stop func()) {
	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		last := check()
		onChange(last)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-quit:
				return
			case <-t.C:
				if cur := check(); cur != last {
					last = cur
					onChange(cur)
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(quit) })
		<-done
	}
}
