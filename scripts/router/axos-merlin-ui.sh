#!/bin/sh
# axos-merlin-ui.sh — hot-plug AXOS into the stock Merlin httpd UI without a
# firmware rebuild.
#
# Merlin's state.js sets current_url to the *basename* of the path and matches
# menu entries with ===. So the AXOS page must live at /www/Axos_Content.asp
# (we bind-mount over an unused stock ASP), not only under /userRpm/.
set -eu

STATE_DIR="${STATE_DIR:-/jffs/axos}"
UI_DIR="$STATE_DIR/merlin-ui"
TOKEN_FILE="$STATE_DIR/run/ui.token"
MENU_SRC="/www/require/modules/menuTree.js"
MENU_DST="$UI_DIR/menuTree.js"
MENU_STOCK="$UI_DIR/menuTree.stock.js"
MARKER="menu_AXOS"
# Unused stock page we overlay — not referenced in menuTree on GT-AX6000.
AXOS_WWW_PAGE="Main_GameServer_Content.asp"
AXOS_WWW_PATH="/www/$AXOS_WWW_PAGE"

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

# --- Static assets under /userRpm (css/js/token) ------------------------------
if [ -d /www/userRpm ]; then
  umount /www/userRpm 2>/dev/null || true
  mount --bind "$UI_DIR" /www/userRpm
fi

# --- Page at www root (basename match for state.js) ---------------------------
if [ -f "$AXOS_WWW_PATH" ] && [ -f "$UI_DIR/Axos_Content.asp" ]; then
  umount "$AXOS_WWW_PATH" 2>/dev/null || true
  mount --bind "$UI_DIR/Axos_Content.asp" "$AXOS_WWW_PATH"
fi

# --- Patch menuTree from clean stock copy ------------------------------------
umount "$MENU_SRC" 2>/dev/null || true
if [ -f "$MENU_SRC" ]; then
  cp -a "$MENU_SRC" "$MENU_STOCK"
  cp -a "$MENU_STOCK" "$MENU_DST"

  if ! grep -q "$MARKER" "$MENU_DST" 2>/dev/null; then
    awk -v mark="$MARKER" -v page="$AXOS_WWW_PAGE" '
      BEGIN { done=0 }
      {
        print
        if (!done && $0 ~ /list:[[:space:]]*\[/) {
          print "\t{"
          print "\t\tmenuName: \"AXOS\","
          print "\t\tindex: \"" mark "\","
          print "\t\ttab: ["
          print "\t\t\t{url: \"" page "\", tabName: \"Control\"},"
          print "\t\t\t{url: \"NULL\", tabName: \"__INHERIT__\"}"
          print "\t\t]"
          print "\t},"
          done=1
        }
      }
    ' "$MENU_DST" >"$MENU_DST.tmp" && mv "$MENU_DST.tmp" "$MENU_DST"
  fi

  if ! grep -q "tabName: \"AXOS\"" "$MENU_DST" 2>/dev/null; then
    awk -v page="$AXOS_WWW_PAGE" '
      {
        print
        if ($0 ~ /Advanced_System_Content\.asp/ && $0 ~ /tabName/ && !done) {
          print "{url: \"" page "\", tabName: \"AXOS\"},"
          done=1
        }
      }
    ' "$MENU_DST" >"$MENU_DST.tmp" && mv "$MENU_DST.tmp" "$MENU_DST"
  fi

  mount --bind "$MENU_DST" "$MENU_SRC"
fi

echo "axos-merlin-ui: ready — open /$AXOS_WWW_PAGE (Administration → AXOS)"
echo "axos-merlin-ui: log out/in if the left menu is stale"
