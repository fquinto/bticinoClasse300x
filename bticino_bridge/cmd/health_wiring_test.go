package main

import (
	"context"
	"os"
	"testing"
	"time"

	"bticino_bridge/pkg/config"
	"bticino_bridge/pkg/health"
)

func TestSSHWatchdogSettings_Defaults(t *testing.T) {
	port, grace, cmd := sshWatchdogSettings(config.SSHWatchdogConfig{})
	if port != 22 || grace != 5*time.Minute || cmd != health.DefaultSSHStartCommand {
		t.Errorf("defaults: %d %v %q", port, grace, cmd)
	}
}

func TestSSHWatchdogSettings_Overrides(t *testing.T) {
	port, grace, cmd := sshWatchdogSettings(config.SSHWatchdogConfig{Port: 2222, Grace: time.Minute, StartCommand: "/x start"})
	if port != 2222 || grace != time.Minute || cmd != "/x start" {
		t.Errorf("overrides: %d %v %q", port, grace, cmd)
	}
	// Una espera absurda por lo corta se sube al mínimo: el vigilante no debe
	// pelearse con el arranque normal del equipo.
	if _, grace, _ := sshWatchdogSettings(config.SSHWatchdogConfig{Grace: time.Second}); grace != time.Minute {
		t.Errorf("grace mínima: %v", grace)
	}
}

func TestFlexisipAttrs(t *testing.T) {
	a := flexisipAttrs(health.FlexisipState{Running: true, SIPPortListening: true})
	if a["running"] != true || a["sip_port_listening"] != true || a["tls_port_listening"] != false || a["passphrase_file_arg"] != false {
		t.Errorf("attrs: %v", a)
	}
}

func TestRetryUntilRead(t *testing.T) {
	calls := 0
	ok := retryUntilRead(context.Background(), time.Millisecond, 10, func() error {
		calls++
		if calls < 3 {
			return os.ErrNotExist
		}
		return nil
	})
	if !ok || calls != 3 {
		t.Fatalf("ok=%v calls=%d", ok, calls)
	}
	calls = 0
	if retryUntilRead(context.Background(), time.Millisecond, 4, func() error { calls++; return os.ErrNotExist }) || calls != 4 {
		t.Fatalf("debía rendirse tras 4 intentos, calls=%d", calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if retryUntilRead(ctx, time.Hour, 4, func() error { return os.ErrNotExist }) {
		t.Fatal("cancelado debe devolver false")
	}
}
