// Package udpproxy implementa un proxy UDP simple que reenvía paquetes entre dos puertos.
// Esto permite que clientes externos (como la app móvil BTicino o scrcpy) se comuniquen
// con el dispositivo a través del bridge, reenviando paquetes del puerto 40004 al 4000.
//
// Basado en la implementación de referencia de slyoldfox/c300x-controller (lib/udp-proxy.js).
package udpproxy

import (
	"fmt"
	"net"
	"sync"

	"github.com/sirupsen/logrus"
)

// UDPProxy reenvía paquetes UDP de un puerto de escucha a un puerto destino en localhost.
type UDPProxy struct {
	listenPort int
	targetPort int
	logger     *logrus.Logger

	conn    *net.UDPConn
	mu      sync.Mutex
	running bool
	done    chan struct{}
}

// New crea una nueva instancia de UDPProxy.
func New(listenPort, targetPort int, logger *logrus.Logger) *UDPProxy {
	return &UDPProxy{
		listenPort: listenPort,
		targetPort: targetPort,
		logger:     logger,
		done:       make(chan struct{}),
	}
}

// Start inicia el proxy UDP en una goroutine.
func (p *UDPProxy) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("UDP proxy ya esta en ejecucion")
	}

	listenAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("0.0.0.0:%d", p.listenPort))
	if err != nil {
		return fmt.Errorf("error resolviendo direccion de escucha: %w", err)
	}

	p.conn, err = net.ListenUDP("udp4", listenAddr)
	if err != nil {
		return fmt.Errorf("error abriendo socket UDP en puerto %d: %w", p.listenPort, err)
	}

	p.running = true

	p.logger.Infof("UDP Proxy escuchando en 0.0.0.0:%d -> reenviando a 127.0.0.1:%d", p.listenPort, p.targetPort)

	go p.forwardLoop()

	return nil
}

// forwardLoop lee paquetes del socket de escucha y los reenvía al puerto destino.
func (p *UDPProxy) forwardLoop() {
	defer func() {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
		close(p.done)
	}()

	buf := make([]byte, 65535)

	targetAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", p.targetPort))
	if err != nil {
		p.logger.WithError(err).Error("Error resolviendo direccion destino UDP")
		return
	}

	for {
		n, _, err := p.conn.ReadFromUDP(buf)
		if err != nil {
			// Comprobar si fue cerrado intencionalmente
			p.mu.Lock()
			running := p.running
			p.mu.Unlock()
			if !running {
				p.logger.Info("UDP Proxy: socket cerrado, deteniendo")
				return
			}
			p.logger.WithError(err).Warn("Error leyendo paquete UDP")
			continue
		}

		if n == 0 {
			continue
		}

		// Crear socket temporal para reenviar al destino
		fwdConn, err := net.DialUDP("udp4", nil, targetAddr)
		if err != nil {
			p.logger.WithError(err).Warn("Error conectando al destino UDP")
			continue
		}

		_, err = fwdConn.Write(buf[:n])
		fwdConn.Close()
		if err != nil {
			p.logger.WithError(err).Warn("Error reenviando paquete UDP")
			continue
		}

		p.logger.WithField("bytes", n).Debug("Paquete UDP reenviado")
	}
}

// Stop detiene el proxy UDP.
func (p *UDPProxy) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false
	p.mu.Unlock()

	if p.conn != nil {
		p.conn.Close()
	}

	// Esperar a que la goroutine termine
	<-p.done

	p.logger.Info("UDP Proxy detenido")
	return nil
}

// IsRunning devuelve si el proxy está en ejecución.
func (p *UDPProxy) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}
