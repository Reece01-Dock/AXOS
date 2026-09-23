#!/bin/sh
# axos-merlin-ui.sh — hot-plug AXOS into the stock Merlin httpd UI without a
# firmware rebuild.
#
# Strategy (Merlin-addon style):
#   1. Bind-mount /jffs/axos/merlin-ui  →  /www/userRpm
#   2. Patch a fresh copy of menuTree.js:
#        - top-level "AXOS" menu
#        - "AXOS" tab under Administration (menu_Setting), next to System
#      then bind-mount over /www/require/modules/menuTree.js
#   3. Ensure a UI token exists for LAN Core API access
#
# After editing web/merlin/*, run deploy-router.sh then this script.
# Browser: log out of Merlin and log back in (menuList is Session-cached).
set -eu

STATE_DIR="${STATE_DIR:-/jffs/axos}"
UI_DIR="$STATE_DIR/merlin-ui"
TOKEN_FILE="$STATE_DIR/run/ui.token"
MENU_SRC="/www/require/modules/menuTree.js"
MENU_DST="$UI_DIR/menuTree.js"
MENU_STOCK="$UI_DIR/menuTree.stock.js"
MARKER="menu_AXOS"
TAB_MARK="userRpm/Axos_Content.asp"

mkdir -p "$UI_DIR" "$STATE_DIR/run"

# --- UI token ----------------------------------------------------------------
if [ ! -s "$TOKEN_FILE" ]; then
  if [ -r /proc/sys/kernel/random/uuid ]; then
    tr -d '-' </proc/sys/kernel/random/uuid >"$TOKEN_FILE"
  else
    date +%s | md5sum | awk '{print $1}' >"$TOKEN_FILE"
  fi
  chmod 600 "$TOKEN_FILE"
fi
printf 'window.AXOS_UI_TOKEN="' >"$UI_DIR/token.js"
sed 's/\\/\\\\/g; s/"/\\"/g' "$TOKEN_FILE" | tr -d '\n' >>"$UI_DIR/token.js"
printf '";\n' >>"$UI_DIR/token.js"
chmod 644 "$UI_DIR/token.js"

# --- Bind-mount UI tree ------------------------------------------------------
if [ -d /www/userRpm ]; then
  umount /www/userRpm 2>/dev/null || true
  mount --bind "$UI_DIR" /www/userRpm
fi

# --- Patch menuTree from a clean stock copy every run ------------------------
# Drop our previous bind so we read the real squashfs menuTree as the base.
umount "$MENU_SRC" 2>/dev/null || true
if [ -f "$MENU_SRC" ]; then
  cp -a "$MENU_SRC" "$MENU_STOCK"
  cp -a "$MENU_STOCK" "$MENU_DST"

  # 1) Top-level AXOS menu (after list: [)
  if ! grep -q "$MARKER" "$MENU_DST" 2>/dev/null; then
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

  # 2) Tab under Administration / menu_Setting (next to System)
  if ! grep -q "$TAB_MARK" "$MENU_DST" 2>/dev/null || ! grep -q 'tabName: "AXOS"' "$MENU_DST" 2>/dev/null; then
    # Insert after Advanced_System_Content.asp line inside menu_Setting.
    awk '
      {
        print
        if ($0 ~ /Advanced_System_Content\.asp/ && $0 ~ /tabName/ && !done) {
          print "{url: \"userRpm/Axos_Content.asp\", tabName: \"AXOS\"},"
          done=1
        }
      }
    ' "$MENU_DST" >"$MENU_DST.tmp" && mv "$MENU_DST.tmp" "$MENU_DST"
  fi

  mount --bind "$MENU_DST" "$MENU_SRC"
fi

echo "axos-merlin-ui: /www/userRpm + menuTree ready"
echo "axos-merlin-ui: open http://<router>/userRpm/Axos_Content.asp"
echo "axos-merlin-ui: or Administration → AXOS tab (log out/in if menu cached)"
