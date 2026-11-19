package mqtt

import "testing"

func TestEntityDecision(t *testing.T) {
	allow := map[string]string{"bticino_door": "Front door", "bticino_doorbell": ""}
	cases := []struct {
		allow   map[string]string
		id      string
		publish bool
		name    string
	}{
		{nil, "bticino_door", true, ""},
		{map[string]string{}, "bticino_door", true, ""},
		{allow, "bticino_door", true, "Front door"},
		{allow, "bticino_doorbell", true, ""},
		{allow, "bticino_display", false, ""},
	}
	for _, c := range cases {
		p, n := entityDecision(c.allow, c.id)
		if p != c.publish || n != c.name {
			t.Errorf("%v %s: got (%v,%q) want (%v,%q)", c.allow, c.id, p, n, c.publish, c.name)
		}
	}
}
