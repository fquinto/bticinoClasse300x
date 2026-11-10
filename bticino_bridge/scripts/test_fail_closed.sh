#!/usr/bin/env bash
# Comprueba el gate fail-closed de secretos del bridge SIN dispositivo:
#   1) subsistemas activados sin secretos  -> aborta nombrando las claves
#   2) subsistemas desactivados            -> no exige sus secretos
#   3) secrets.yaml completo pero 0644     -> arranca y avisa de permisos
# Uso: bash scripts/test_fail_closed.sh   (desde bticino_bridge/)
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
FAILED=0
fail() { echo "FAIL: $*"; FAILED=1; }

( cd "$ROOT" && go build -o "$T/bridge" ./cmd ) || { echo "FAIL: build"; exit 1; }

write_config() { # $1 = mqtt enabled (true|false)
  cat > "$T/config.yaml" <<YAML
sip:
  enabled: true
  server_host: "127.0.0.1"
  domain: ""
  username: "webrtc"
  password: ""
mqtt:
  enabled: $1
  host: "127.0.0.1"
  port: 1
  username: ""
  password: ""
homekit:
  enabled: true
  port: "18081"
  pin: ""
  storage_path: "$T/homekit_data"
web:
  enabled: false
YAML
}

run() { # resto = flags extra; deja salida en $T/out y código en RC
  ( cd "$T" && timeout 6 ./bridge -test -enable-openwebnet=false -config "$T/config.yaml" "$@" ) >"$T/out" 2>&1
  RC=$?
}

# --- caso 1: todo activado, sin secretos -> aborta ---
write_config true
run
[ "$RC" -ne 0 ] && [ "$RC" -ne 124 ] || fail "case 1: debía abortar (rc=$RC)"
for k in sip.domain sip.password mqtt.password homekit.pin secrets.yaml; do
  grep -q "$k" "$T/out" || fail "case 1: la salida no nombra $k"
done

# --- caso 2: subsistemas desactivados -> no exige secretos ---
write_config false
run -enable-video=false -enable-homekit=false
grep -q "missing required secrets" "$T/out" && fail "case 2: exigió secretos de subsistemas desactivados"

# --- caso 3: secrets.yaml completo pero legible por otros -> arranca y avisa ---
write_config true
cat > "$T/secrets.yaml" <<YAML
sip:
  domain: "gw.example.test"
  password: "0123456789abcdef"
mqtt:
  password: "example-pass"
homekit:
  pin: "11122333"
YAML
chmod 644 "$T/secrets.yaml"
run
grep -q "missing required secrets" "$T/out" && fail "case 3: secrets.yaml completo y aun así faltan secretos"
grep -q "0600" "$T/out" || fail "case 3: no avisó de permisos demasiado abiertos"

# --- caso 4: secrets.yaml roto -> aborta culpando al fichero de secretos, sin caer a defaults ---
write_config true
printf ':\n\t- [\n' > "$T/secrets.yaml"; chmod 600 "$T/secrets.yaml"
run
[ "$RC" -ne 0 ] && [ "$RC" -ne 124 ] || fail "case 4: debía abortar (rc=$RC)"
grep -q "invalid secrets file" "$T/out" || fail "case 4: no culpa al fichero de secretos"
grep -q "using defaults" "$T/out" && fail "case 4: cayó a los defaults con un secrets.yaml roto"

# --- caso 5: secrets.yaml vacío -> equivale a no tener claves (no es un fichero roto) ---
write_config false
: > "$T/secrets.yaml"; chmod 600 "$T/secrets.yaml"
run -enable-video=false -enable-homekit=false
grep -qE "invalid secrets file|using defaults" "$T/out" && fail "case 5: un secrets.yaml vacío se trató como roto"

# --- caso 6: config.yaml con credenciales y sin secrets.yaml (device sin migrar) -> arranca y avisa ---
rm -f "$T/secrets.yaml"
write_config false
sed -i 's/^  password: ""$/  password: "inline-example"/' "$T/config.yaml"
run -enable-video=false -enable-homekit=false
grep -q "docs/SECRETS.md" "$T/out" || fail "case 6: no avisó de la migración pendiente"

[ "$FAILED" -eq 0 ] && echo "PASS" || { echo "--- última salida ---"; tail -15 "$T/out"; exit 1; }
