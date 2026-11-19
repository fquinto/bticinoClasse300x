package sip

import (
	"bufio"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"

	"bticino_bridge/pkg/events"
)

// fakeFlexisip accepts TCP connections, answers every REGISTER with 200 (like the
// unit's Flexisip with trusted-hosts=127.0.0.1) and records the registered users.
func fakeFlexisip(t *testing.T) (addr string, users func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	var seen []string
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				r := bufio.NewReader(conn)
				var req []string
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimRight(line, "\r\n")
					if line != "" {
						req = append(req, line)
						continue
					}
					if len(req) > 0 && strings.HasPrefix(req[0], "REGISTER ") {
						var callID, cseq string
						for _, h := range req {
							if v, ok := strings.CutPrefix(h, "Call-ID: "); ok {
								callID = v
							}
							if v, ok := strings.CutPrefix(h, "CSeq: "); ok {
								cseq = v
							}
							if v, ok := strings.CutPrefix(h, "From: "); ok {
								mu.Lock()
								seen = append(seen, extractSIPUser(v))
								mu.Unlock()
							}
						}
						conn.Write([]byte(sipRegisterOK(callID, cseq)))
					}
					req = nil
				}
			}(conn)
		}
	}()
	return ln.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

// Start registers c300x (the unit's own identity) only with RegisterUnitIdentity:
// otherwise the bridge takes over the unit's Flexisip binding and the official
// app can no longer open the camera.
func TestStartRegistersUnitIdentityOnlyWhenEnabled(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		want    string
	}{
		{false, "webrtc"},
		{true, "webrtc,c300x"},
	} {
		addr, users := fakeFlexisip(t)
		logger := logrus.New()
		logger.SetOutput(io.Discard)
		c := NewBTicinoSIPClient(&SIPConfig{ServerAddr: addr, Domain: "example.com", Username: "webrtc",
			LocalIP: "127.0.0.1", Transport: "tcp", RegisterUnitIdentity: tc.enabled},
			events.NewEventBus(10, logger), logger)
		if err := c.Start(); err != nil {
			t.Fatalf("enabled=%t: Start: %v", tc.enabled, err)
		}
		got := strings.Join(users(), ",")
		dual := c.c300xID != nil
		c.Stop()
		if got != tc.want {
			t.Errorf("enabled=%t: registered %q, want %q", tc.enabled, got, tc.want)
		}
		if dual != tc.enabled {
			t.Errorf("enabled=%t: c300x identity active=%t", tc.enabled, dual)
		}
	}
}
