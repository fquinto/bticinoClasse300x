#!/usr/bin/env bash
# Guard: fails if a tracked (or new, not-ignored) text file contains a
# secret-SHAPED value.
# It only knows generic shapes — this script holds no real value:
#   - bare 32-hex tokens (HA1 / MD5 digests), except all-0 / all-f examples
#   - <numeric gateway id>.bs.iotleg.com
#   - cloud SIP users: <name>-gmx.com-<hex…> / <name>-gmail.com-<hex…>
# Real credentials belong in configs/secrets.yaml (gitignored). See docs/SECRETS.md.
#
# Usage: scripts/check-no-secrets.sh [path...]     (default: whole bticino_bridge tree)
set -u
cd "$(dirname "$0")/.." || exit 2
[ "$#" -eq 0 ] && set -- .

PATTERN='\b[0-9a-f]{32}\b|\b[0-9]{6,8}\.bs\.iotleg\.com|-(gmx|gmail)\.com-[0-9A-Fa-f]{8}'
# --untracked: also scan new files not yet added (ignored files are skipped).
raw=$(git grep --untracked -nIE "$PATTERN" -- "$@")
rc=$?
if [ "$rc" -gt 1 ]; then
  echo "ERROR: git grep failed (rc=$rc) — not a git checkout? Nothing was verified." >&2
  exit 2
fi
hits=$(printf '%s\n' "$raw" | grep -vE '\b(0{32}|f{32})\b' | grep .)
if [ -n "$hits" ]; then
  echo "$hits" | cut -c1-200 >&2
  echo "ERROR: secret-shaped values in tracked files (see above). Move them to configs/secrets.yaml or use placeholders." >&2
  exit 1
fi
echo "ok: no secret-shaped values"
