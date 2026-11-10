package deviceconfig

import (
	"bticino_bridge/pkg/bticino"
	"os"
	"path/filepath"
	"testing"
)

const sampleVDE = `<?xml version="1.0" encoding="utf-8"?>
<configuratore>
  <setup>
    <vdes>
      <communication>
        <vde_enabled>1</vde_enabled>
        <mode>0</mode>
        <dev>1</dev>
        <address>6</address>
        <p_default>
          <dev>2</dev>
          <address>0</address>
        </p_default>
      </communication>
      <alarmlogger>
        <enabled>0</enabled>
        <address>1</address>
      </alarmlogger>
    </vdes>
  </setup>
</configuratore>`

func TestParseVDEAddresses_Sample(t *testing.T) {
	got := ParseVDEAddresses([]byte(sampleVDE))
	if got.Own != "16" {
		t.Errorf("Own = %q, want 16", got.Own)
	}
	if got.Panel != "20" {
		t.Errorf("Panel = %q, want 20", got.Panel)
	}
}

// La <address> de alarmlogger (u otra sección) no debe confundirse con la propia.
func TestParseVDEAddresses_IgnoresOtherSections(t *testing.T) {
	xml := `<configuratore><setup><vdes>
	  <alarmlogger><address>1</address></alarmlogger>
	  <communication><p_default><dev>2</dev><address>3</address></p_default></communication>
	</vdes></setup></configuratore>`
	got := ParseVDEAddresses([]byte(xml))
	if got.Own != "" {
		t.Errorf("Own = %q, want empty (no dev/address propios)", got.Own)
	}
	if got.Panel != "23" {
		t.Errorf("Panel = %q, want 23", got.Panel)
	}
}

func TestParseVDEAddresses_RejectsGarbage(t *testing.T) {
	cases := map[string]string{
		"vacío":           ``,
		"no xml":          `esto no es xml <<<`,
		"no numérico":     `<configuratore><setup><vdes><communication><dev>x</dev><address>6</address><p_default><dev>2</dev><address>;rm</address></p_default></communication></vdes></setup></configuratore>`,
		"medio vacío":     `<configuratore><setup><vdes><communication><dev>1</dev><address></address><p_default><dev></dev><address>0</address></p_default></communication></vdes></setup></configuratore>`,
		"demasiado largo": `<configuratore><setup><vdes><communication><dev>1</dev><address>123456</address></communication></vdes></setup></configuratore>`,
	}
	for name, xml := range cases {
		got := ParseVDEAddresses([]byte(xml))
		if got.Own != "" || got.Panel != "" {
			t.Errorf("%s: got %+v, want vacío", name, got)
		}
	}
}

func TestReadVDEAddresses_File(t *testing.T) {
	p := filepath.Join(t.TempDir(), "conf.xml")
	if err := os.WriteFile(p, []byte(sampleVDE), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadVDEAddresses(p)
	if err != nil || got.Own != "16" || got.Panel != "20" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if _, err := ReadVDEAddresses(filepath.Join(t.TempDir(), "nope.xml")); err == nil {
		t.Error("fichero inexistente debe dar error")
	}
}

func TestApplyBusAddresses(t *testing.T) {
	defer bticino.ResetAddresses()
	dir := t.TempDir()
	p := filepath.Join(dir, "conf.xml")
	other := `<configuratore><setup><vdes><communication><dev>1</dev><address>3</address><p_default><dev>2</dev><address>1</address></p_default></communication></vdes></setup></configuratore>`
	if err := os.WriteFile(p, []byte(other), 0o600); err != nil {
		t.Fatal(err)
	}

	// Autodescubrimiento: conf.xml manda.
	r := ApplyBusAddresses("", "", p)
	if r.Door != "21" || r.DoorSource != "conf.xml" || r.Light != "13" || r.LightSource != "conf.xml" {
		t.Fatalf("autodescubierto: %+v", r)
	}
	if bticino.DoorOpenPressCmd() != "*8*19*21##" || bticino.LightOnPressCmd() != "*8*21*13##" {
		t.Fatalf("no aplicado: %s %s", bticino.DoorOpenPressCmd(), bticino.LightOnPressCmd())
	}

	// La configuración fuerza por encima del conf.xml.
	r = ApplyBusAddresses("22", "", p)
	if r.Door != "22" || r.DoorSource != "config" || r.Light != "13" {
		t.Fatalf("forzado: %+v", r)
	}

	// Sin conf.xml: valores históricos, y los comandos vuelven a ellos.
	r = ApplyBusAddresses("", "", filepath.Join(dir, "nope.xml"))
	if r.Door != "20" || r.DoorSource != "default" || r.Light != "16" || r.LightSource != "default" {
		t.Fatalf("respaldo: %+v", r)
	}
	if bticino.DoorOpenPressCmd() != bticino.CmdDoorOpenPress {
		t.Fatalf("respaldo no aplicado: %s", bticino.DoorOpenPressCmd())
	}
}

// Un conf.xml a medio escribir no debe dar una dirección "válida" pero falsa
// (p. ej. <address>1 cortado de un 10).
func TestParseVDEAddresses_TruncatedFileYieldsNothing(t *testing.T) {
	for _, cut := range []string{
		`<configuratore><setup><vdes><communication><dev>1</dev><address>6</address><p_default><dev>2</dev><address>1`,
		`<configuratore><setup><vdes><communication><dev>1</dev><address>6</address><p_default><dev>2</dev><address>0</address></p_default></communication>`,
	} {
		if got := ParseVDEAddresses([]byte(cut)); got.Own != "" || got.Panel != "" {
			t.Errorf("truncado %q: got %+v, want vacío", cut[len(cut)-20:], got)
		}
	}
}
