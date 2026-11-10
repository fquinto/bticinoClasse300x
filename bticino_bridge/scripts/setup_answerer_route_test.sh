#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/setup_answerer_route.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
FIX="$TMP/route_int.conf"

# Fixture: línea alluser con user1..5 (formato flexisip static-record)
cat > "$FIX" <<'EOF'
<sip:alluser@YOUR_GATEWAY_ID.bs.iotleg.com> <sip:user1@YOUR_GATEWAY_ID.bs.iotleg.com>,<sip:user2@YOUR_GATEWAY_ID.bs.iotleg.com>,<sip:user3@YOUR_GATEWAY_ID.bs.iotleg.com>,<sip:user4@YOUR_GATEWAY_ID.bs.iotleg.com>,<sip:user5@YOUR_GATEWAY_ID.bs.iotleg.com>
EOF

# 1) --add inserta el AOR bridge y crea backup
"$SCRIPT" --add --file "$FIX"
grep -q 'sip:bridge@YOUR_GATEWAY_ID.bs.iotleg.com' "$FIX" || { echo "FAIL: no añadió bridge"; exit 1; }
ls "$FIX".bak.* >/dev/null 2>&1 || { echo "FAIL: no creó backup"; exit 1; }
grep -q 'sip:user5@YOUR_GATEWAY_ID.bs.iotleg.com' "$FIX" || { echo "FAIL: perdió user5"; exit 1; }

# 2) --add es idempotente (no duplica)
"$SCRIPT" --add --file "$FIX"
[ "$(grep -o 'sip:bridge@YOUR_GATEWAY_ID' "$FIX" | wc -l)" -eq 1 ] || { echo "FAIL: duplicó bridge"; exit 1; }

# 3) --check devuelve 0 cuando está, !=0 cuando no
"$SCRIPT" --check --file "$FIX" || { echo "FAIL: --check debería ser 0"; exit 1; }

# 4) --restore recupera el backup más reciente (sin bridge)
"$SCRIPT" --restore --file "$FIX"
grep -q 'sip:bridge@YOUR_GATEWAY_ID' "$FIX" && { echo "FAIL: restore no quitó bridge"; exit 1; }
grep -q 'sip:user1@YOUR_GATEWAY_ID' "$FIX" || { echo "FAIL: restore corrompió"; exit 1; }

# 5) --restore funciona aunque el fichero actual esté dañado (sin línea alluser) y sin SIP_DOMAIN
"$SCRIPT" --add --file "$FIX"
echo "corrupto" > "$FIX"
( unset SIP_DOMAIN; "$SCRIPT" --restore --file "$FIX" ) || { echo "FAIL: restore exige dominio"; exit 1; }
grep -q 'sip:user1@' "$FIX" || { echo "FAIL: restore no recuperó el backup"; exit 1; }

# 6) SIP_DOMAIN explícito manda sobre la autodetección
rm -f "$FIX".bak.*
echo '<sip:alluser@other.example.test> <sip:user1@other.example.test>' > "$FIX"
SIP_DOMAIN=other.example.test "$SCRIPT" --add --file "$FIX"
grep -q 'sip:bridge@other.example.test' "$FIX" || { echo "FAIL: SIP_DOMAIN no se usó"; exit 1; }

echo "OK setup_answerer_route: 6/6"
