package main

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"bticino_bridge/pkg/config"
	"bticino_bridge/pkg/health"
	"bticino_bridge/pkg/mqtt"
	"bticino_bridge/pkg/sip"
)

const (
	problemAnswererRoute = "answerer_route"
	problemFlexisip      = "flexisip"

	flexisipCheckInterval = 60 * time.Second
	sshCheckInterval      = 60 * time.Second
	sshRetryBackoff       = 10 * time.Minute
	sshMinGrace           = time.Minute
)

// sshWatchdogSettings aplica los valores por defecto del vigilante de SSH.
func sshWatchdogSettings(c config.SSHWatchdogConfig) (port int, grace time.Duration, startCmd string) {
	port, grace, startCmd = c.Port, c.Grace, c.StartCommand
	if port <= 0 {
		port = 22
	}
	if grace == 0 {
		grace = 5 * time.Minute
	}
	if grace < sshMinGrace {
		grace = sshMinGrace
	}
	if startCmd == "" {
		startCmd = health.DefaultSSHStartCommand
	}
	return port, grace, startCmd
}

func flexisipAttrs(s health.FlexisipState) map[string]interface{} {
	return map[string]interface{}{
		"running":             s.Running,
		"sip_port_listening":  s.SIPPortListening,
		"tls_port_listening":  s.TLSPortListening,
		"passphrase_file_arg": s.PassphraseArg,
	}
}

// installSSHStarter deja disponible en HA el botón "Arrancar SSH" (lo publica el
// bridge MQTT solo si mqtt.enable_system_buttons está activo). Llamar antes de
// mqttBridge.Start.
func installSSHStarter(cfg *config.Config, mqttBridge *mqtt.MQTTBridge) {
	if mqttBridge == nil {
		return
	}
	_, _, startCmd := sshWatchdogSettings(cfg.Health.SSHWatchdog)
	mqttBridge.SetSSHStarter(func() error { return health.RunCommand(startCmd) })
}

// startHealthWatchers arranca las comprobaciones periódicas de solo lectura
// (ruta del contestador, Flexisip) y, si está habilitado, el vigilante de SSH.
// Devuelve la función que las detiene.
func startHealthWatchers(cfg *config.Config, mqttBridge *mqtt.MQTTBridge, logger *logrus.Logger) (stop func()) {
	var stops []func()
	proc := health.Proc{}

	if cfg.SIP.Answerer.Enabled {
		if mqttBridge != nil {
			mqttBridge.RegisterProblemSensor(mqtt.ProblemSensor{
				Key: problemAnswererRoute, Name: "Contestador sin ruta", Icon: "mdi:phone-alert"})
		}
		w := sip.NewRouteWatcher(cfg.SIP.Answerer.RouteFile, cfg.SIP.Answerer.User, cfg.SIP.Domain,
			sip.DefaultRouteCheckInterval, func(s sip.RouteState) {
				if s.OK {
					logger.Infof("[ANSWERER] ruta del contestador presente en %s", s.File)
				} else {
					logger.Warnf("[ANSWERER] el contestador no recibirá llamadas: %s (¿setup_answerer_route.sh?)", s.Detail)
				}
				if mqttBridge != nil {
					mqttBridge.PublishProblem(problemAnswererRoute, !s.OK, s.Detail, map[string]interface{}{"file": s.File})
				}
			})
		w.Start()
		stops = append(stops, w.Stop)
	}

	if cfg.SIP.Enabled {
		if mqttBridge != nil {
			mqttBridge.RegisterProblemSensor(mqtt.ProblemSensor{
				Key: problemFlexisip, Name: "Flexisip (llamadas y nube)", Icon: "mdi:cloud-alert"})
		}
		sipPort := cfg.SIP.ServerPort
		if sipPort <= 0 {
			sipPort = 5060
		}
		stops = append(stops, health.Watch(flexisipCheckInterval,
			func() health.FlexisipState { return health.CheckFlexisip(proc, sipPort) },
			func(s health.FlexisipState) {
				if s.Problem {
					logger.Warnf("[HEALTH] %s", s.Detail)
				} else {
					logger.Infof("[HEALTH] %s (TLS 5061: %v)", s.Detail, s.TLSPortListening)
				}
				if mqttBridge != nil {
					mqttBridge.PublishProblem(problemFlexisip, s.Problem, s.Detail, flexisipAttrs(s))
				}
			}))
	}

	if cfg.Health.SSHWatchdog.Enabled {
		port, grace, startCmd := sshWatchdogSettings(cfg.Health.SSHWatchdog)
		wd := health.NewSSHWatchdog(grace, sshRetryBackoff, health.PortListening(proc, port),
			func() error { return health.RunCommand(startCmd) })
		logger.Infof("[HEALTH] vigilante de SSH activo (puerto %d, espera %s)", port, grace)
		stops = append(stops, health.Watch(sshCheckInterval, func() int {
			if tried, err := wd.Tick(); tried {
				if err != nil {
					logger.WithError(err).Error("[HEALTH] SSH caído; no se pudo arrancar")
				} else {
					logger.Warnf("[HEALTH] SSH llevaba más de %s sin escuchar: arrancado", grace)
				}
			}
			return 0
		}, func(int) {}))
	}

	return func() {
		for _, s := range stops {
			s()
		}
	}
}

// retryUntilRead llama a try hasta que devuelve nil, como mucho `attempts`
// veces separadas por `interval`. Devuelve true si llegó a tener éxito.
func retryUntilRead(ctx context.Context, interval time.Duration, attempts int, try func() error) bool {
	for i := 0; i < attempts; i++ {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(interval):
		}
		if try() == nil {
			return true
		}
	}
	return false
}
