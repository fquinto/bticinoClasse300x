package mqtt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"bticino_bridge/pkg/config"
)

// discoveryNames parses bridge.go and returns object_id -> hard-coded name for
// every b.discovery("comp", "id", {"name": "..."}) call and every LED table row.
func discoveryNames(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bridge.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	str := func(e ast.Expr) (string, bool) {
		l, ok := e.(*ast.BasicLit)
		if !ok || l.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(l.Value)
		return s, err == nil
	}
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || len(c.Args) != 3 {
			return true
		}
		if sel, ok := c.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "discovery" {
			return true
		}
		id, ok := str(c.Args[1])
		if !ok {
			return true // dynamic id (LED/GPIO/lock loops) handled elsewhere
		}
		lit, ok := c.Args[2].(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, el := range lit.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if k, _ := str(kv.Key); k == "name" {
				if name, ok := str(kv.Value); ok {
					out[id] = name
				}
			}
		}
		return true
	})
	raw, err := os.ReadFile("bridge.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`\{"(bticino_led_\w+)", "([^"]+)"`).FindAllStringSubmatch(string(raw), -1) {
		out[m[1]] = m[2]
	}
	if len(out) < 50 {
		t.Fatalf("parsed only %d discovery names; parser out of sync with bridge.go", len(out))
	}
	return out
}

func TestEveryDiscoveryEntityHasNameInEveryLanguage(t *testing.T) {
	names := discoveryNames(t)
	for lang, cat := range entityNames {
		for id := range names {
			if cat[id] == "" {
				t.Errorf("language %q: no name for object_id %q", lang, id)
			}
		}
		for key, n := range cat {
			if _, ok := names[key]; !ok && !strings.HasPrefix(key, "_") {
				t.Errorf("language %q: catalog key %q matches no discovery entity", lang, key)
			}
			if strings.HasPrefix(key, "_") && !strings.Contains(n, "%d") {
				t.Errorf("language %q: template %q lacks %%d: %q", lang, key, n)
			}
		}
	}
}

// "es" must stay identical to the names hard-coded at the call sites: that is
// the backward-compatibility guarantee of the default language.
func TestSpanishCatalogEqualsHardcodedNames(t *testing.T) {
	for id, hard := range discoveryNames(t) {
		if got := entityNames["es"][id]; got != hard {
			t.Errorf("%s: es catalog %q != hard-coded %q", id, got, hard)
		}
	}
	if entityNames["es"][nameKeyGPIO] != "GPIO %d" || entityNames["es"][nameKeyAdditionalLock] != "Cerradura %d" {
		t.Error("es templates changed")
	}
}

func TestEntityNameLanguageAndFallback(t *testing.T) {
	cases := []struct{ lang, id, def, want string }{
		{"", "bticino_doorbell", "x", "Timbre"},
		{"es", "bticino_doorbell", "x", "Timbre"},
		{"en", "bticino_doorbell", "x", "Doorbell"},
		{"IT", "bticino_door_open", "x", "Apri porta"},
		{"it-IT", "bticino_call_state", "x", "Stato chiamata"},
		{"xx", "bticino_doorbell", "x", "Timbre"},     // unknown language -> es
		{"en", "bticino_unknown", "Custom", "Custom"}, // not in catalog -> call-site name
	}
	for _, c := range cases {
		b := &MQTTBridge{config: &config.Config{MQTT: config.MQTTConfig{Language: c.lang}}}
		if got := b.entityName(c.id, c.def); got != c.want {
			t.Errorf("%q/%s: got %q want %q", c.lang, c.id, got, c.want)
		}
	}
	b := &MQTTBridge{config: &config.Config{MQTT: config.MQTTConfig{Language: "it"}}}
	if got := b.entityNamef(nameKeyGPIO, "GPIO %d", 12); got != "GPIO 12" {
		t.Errorf("gpio: %q", got)
	}
	if got := b.entityNamef(nameKeyAdditionalLock, "Cerradura %d", 2); got != "Serratura aggiuntiva 2" {
		t.Errorf("lock: %q", got)
	}
}
