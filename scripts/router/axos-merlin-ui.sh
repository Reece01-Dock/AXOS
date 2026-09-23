#!/bin/sh
# axos-merlin-ui.sh — hot-plug AXOS into the stock Merlin httpd UI without a
# firmware rebuild.
#
# Strategy (Merlin-addon style):
#   1. Bind-mount /jffs/axos/merlin-ui  →  /www/userRpm   (empty dir on squashfs)
#   2. Inject an AXOS menu entry into a JFFS copy of menuTree.js and bind-mount
#      that over /www/require/modules/menuTree.js
#   3. Ensure a UI token exists for LAN Core API access (axosd -ui-token-file)
#
# After this, edit web/merlin/* in the repo, `./scripts/deploy-router.sh`, and
# re-run this script (or reboot) — no image rebuild, no flash.
#
# Safe to run repeatedly. Called from axos-bootstrap when merlin-ui/ is present.
set -eu

STATE_DIR="${STATE_DIR:-/jffs/axos}"
UI_DIR="$STATE_DIR/merlin-ui"
TOKEN_FILE="$STATE_DIR/run/ui.token"
MENU_SRC="/www/require/modules/menuTree.js"
MENU_DST="$UI_DIR/menuTree.js"
MARKER="menu_AXOS"

mkdir -p "$UI_DIR" "$STATE_DIR/run"

# --- UI token (LAN API gate) -------------------------------------------------
if [ ! -s "$TOKEN_FILE" ]; then
  # BusyBox-friendly random token
  if [ -r /proc/sys/kernel/random/uuid ]; then
    tr -d '-' </proc/sys/kernel/random/uuid >"$TOKEN_FILE"
  else
    date +%s%N | md5sum | awk '{print $1}' >"$TOKEN_FILE"
  fi
  chmod 600 "$TOKEN_FILE"
fi
printf 'window.AXOS_UI_TOKEN="' >"$UI_DIR/token.js"
sed 's/\\/\\\\/g; s/"/\\"/g' "$TOKEN_FILE" | tr -d '\n' >>"$UI_DIR/token.js"
printf '";\n' >>"$UI_DIR/token.js"
chmod 644 "$UI_DIR/token.js"

# --- Bind-mount UI tree over /www/userRpm -----------------------------------
if [ -d /www/userRpm ]; then
  # Unmount prior bind if present (ignore errors).
  umount /www/userRpm 2>/dev/null || true
  mount --bind "$UI_DIR" /www/userRpm
fi

# --- Inject AXOS into menuTree.js -------------------------------------------
if [ -f "$MENU_SRC" ]; then
  # Refresh working copy from stock whenever our marker is missing, so Merlin
  # firmware upgrades don't leave a stale patched tree forever.
  if [ ! -f "$MENU_DST" ] || ! grep -q "$MARKER" "$MENU_DST" 2>/dev/null; then
    cp -a "$MENU_SRC" "$MENU_DST"
  fi
  if ! grep -q "$MARKER" "$MENU_DST" 2>/dev/null; then
    # Insert a top-level menu block immediately after `list: [` .
    # Use a temp file for BusyBox sed portability.
    awk -v mark="$MARKER" '
      BEGIN { done=0 }
      {
        print
        if (!done && $0 ~ /list:[[:space:]]*\[/) {
          print "\t{"
          print "\t\tmenuName: \"AXOS\","
          print "\t\tindex: \"" mark "\","
          print "\t\ttab: ["
          print "\t\t\t{url: \"userRpm/Axos_Content.asp\", tabName: \"Control\"},"
          print "\t\t\t{url: \"NULL\", tabName: \"__INHERIT__\"}"
          print "\t\t]"
          print "\t},"
          done=1
        }
      }
    ' "$MENU_DST" >"$MENU_DST.tmp" && mv "$MENU_DST.tmp" "$MENU_DST"
  fi
  umount "$MENU_SRC" 2>/dev/null || true
  mount --bind "$MENU_DST" "$MENU_SRC"
fi

echo "axos-merlin-ui: mounted $UI_DIR → /www/userRpm; menu AXOS injected"
