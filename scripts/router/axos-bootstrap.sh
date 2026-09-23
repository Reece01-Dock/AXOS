#!/bin/sh
# axos-bootstrap — idempotent start of axosd from JFFS (Phase 9 firmware bake-in
# companion; also usable from Merlin services-start on a running image).
#
# Looks for a deployed axosd under /jffs/axos/current/bin or /jffs/axos/bin
# (install-axosd.sh / axosctl deploy layouts). If found and not already
# running, starts it with the same flags as install-axosd.sh.
#
# Also hot-plugs the Merlin UI embed (axos-merlin-ui.sh) when
# /jffs/axos/merlin-ui is present — no firmware rebuild required to iterate
# on that UI (bind-mount over /www/userRpm + menuTree.js).
#
# This script is the source of truth copied into firmware via
# firmware/patches/axos-bootstrap/ and 0002-axos-jffs-bootstrap.patch.
set -eu

# LAN bind so the Merlin-embedded UI (browser → :9090) can reach the API.
# Non-loopback callers require -ui-token-file (see internal/api.LANAuth).
# Loopback (MCP / SSH tunnel) stays token-free.
API_ADDR="${API_ADDR:-0.0.0.0:9090}"

resolve_axos_dir() {
  if [ -x /jffs/axos/current/bin/axosd ]; then
    echo /jffs/axos/current
    return
  fi
  if [ -x /jffs/axos/bin/axosd ]; then
    echo /jffs/axos
    return
  fi
  return 1
}

if ! AXOS_ROOT="$(resolve_axos_dir)"; then
  exit 0
fi

AXOSD="$AXOS_ROOT/bin/axosd"
if [ -d /jffs/axos/backups ] || [ -d /jffs/axos ]; then
  STATE_DIR=/jffs/axos
else
  STATE_DIR="$AXOS_ROOT"
fi

mkdir -p "$STATE_DIR/backups" "$STATE_DIR/logs" "$STATE_DIR/www" "$STATE_DIR/merlin-ui" "$STATE_DIR/run"
chmod 700 "$STATE_DIR/backups" "$STATE_DIR/logs" "$STATE_DIR/run" 2>/dev/null || true

# Hot-plug Merlin UI (bind mounts) before starting axosd so token.js exists.
if [ -x "$STATE_DIR/bin/axos-merlin-ui.sh" ]; then
  STATE_DIR="$STATE_DIR" "$STATE_DIR/bin/axos-merlin-ui.sh" || true
elif [ -x /jffs/axos/bin/axos-merlin-ui.sh ]; then
  STATE_DIR="$STATE_DIR" /jffs/axos/bin/axos-merlin-ui.sh || true
fi

if pidof axosd >/dev/null 2>&1; then
  exit 0
fi

UI_FLAG=""
if [ -d "$STATE_DIR/www" ]; then
  UI_FLAG="-ui-dir=$STATE_DIR/www"
fi

TOKEN_FLAG=""
if [ -f "$STATE_DIR/run/ui.token" ]; then
  TOKEN_FLAG="-ui-token-file=$STATE_DIR/run/ui.token"
fi

# shellcheck disable=SC2086
"$AXOSD" serve \
  -backend=asuswrt \
  -api-addr="$API_ADDR" \
  -backup-dir="$STATE_DIR/backups" \
  -audit="$STATE_DIR/logs/audit.jsonl" \
  -service-log-dir="$STATE_DIR/logs" \
  $UI_FLAG \
  $TOKEN_FLAG \
  >>"$STATE_DIR/logs/axosd.log" 2>&1 &
