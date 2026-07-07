package sip

import "testing"

func TestExtractSIPUser(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`"Doorbell" <sip:c300x@2617372.bs.iotleg.com>;tag=abc123`, "c300x"},
		{`<sip:webrtc@example.com>`, "webrtc"},
		{`sip:alice@10.0.0.1;transport=tcp`, "alice"},
		{`sip:noatsign`, "noatsign"},
		{`Bob <sip:bob@host>`, "bob"},
		{`plain text no uri`, "plain text no uri"},
	}
	for _, c := range cases {
		if got := extractSIPUser(c.in); got != c.want {
			t.Errorf("extractSIPUser(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
