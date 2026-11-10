#!/usr/bin/env bash
# Añade/quita el AOR del bridge a la línea `alluser` de route_ext.conf (fork del timbre; fichero ACTIVO via symlink route.conf).
# Reversible: --add crea backup; --restore recupera el backup más reciente.
set -euo pipefail

DEFAULT_REMOTE='/etc/flexisip/users/route_ext.conf'
MODE=""
FILE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --add|--restore|--check) MODE="${1#--}";;
    --file) FILE="$2"; shift;;
    *) echo "uso: $0 [--add|--restore|--check] [--file <path>]" >&2; exit 2;;
  esac
  shift
done
[ -n "$MODE" ] || { echo "falta modo" >&2; exit 2; }
FILE="${FILE:-$DEFAULT_REMOTE}"
[ -f "$FILE" ] || { echo "no existe: $FILE" >&2; exit 1; }

# Dominio SIP (<gateway-id>.bs.iotleg.com): no se guarda en el repo. Se toma de
# SIP_DOMAIN o, si no está, de la propia línea `alluser` del fichero. Solo lo
# necesitan --check y --add: --restore debe funcionar con el fichero dañado.
resolve_domain() {
  DOMAIN="${SIP_DOMAIN:-$(sed -nE 's/^<sip:alluser@([^>]+)>.*/\1/p' "$FILE" | head -1)}"
  [ -n "$DOMAIN" ] || { echo "no se pudo determinar el dominio SIP: define SIP_DOMAIN" >&2; exit 1; }
  DOMAIN_RE="${DOMAIN//./\\.}"
  AOR="<sip:bridge@${DOMAIN}>"
}

case "$MODE" in
  check)
    resolve_domain
    grep -qF "sip:bridge@${DOMAIN}" "$FILE"
    ;;
  add)
    resolve_domain
    if grep -qF "sip:bridge@${DOMAIN}" "$FILE"; then
      echo "ya presente, nada que hacer"; exit 0
    fi
    cp -a "$FILE" "$FILE.bak.$(date +%Y%m%d-%H%M%S)"
    # Insertar el AOR al final de la lista de contactos de la línea alluser
    sed -i -E "/<sip:alluser@${DOMAIN_RE}>/ s#\$#,${AOR}#" "$FILE"
    grep -qF "sip:bridge@${DOMAIN}" "$FILE" || { echo "no insertó" >&2; exit 1; }
    echo "añadido bridge a alluser"
    ;;
  restore)
    LAST="$(ls -1t "$FILE".bak.* 2>/dev/null | head -1 || true)"
    [ -n "$LAST" ] || { echo "no hay backup" >&2; exit 1; }
    cp -a "$LAST" "$FILE"
    echo "restaurado desde $LAST"
    ;;
esac
