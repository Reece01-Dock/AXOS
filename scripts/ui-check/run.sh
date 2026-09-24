#!/bin/sh
# ui-check: build axosd, run it on the mock backend, and drive both web UIs
# (the AXOS tabs inside a Merlin page shell, and the standalone :9090 page)
# in headless Chromium. Fails on any JS error or broken feature.
#
#   scripts/ui-check/run.sh [screenshot-dir]
#
# Needs: go, python3, node with Playwright (+ Chromium). Merlin's own
# stylesheets/images are fetched from RMerl/asuswrt-merlin.ng into a temp
# dir (cached in $UI_CHECK_CACHE) — they are not vendored into this repo.
set -eu
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/../.." && pwd)
WORK=$(mktemp -d)
CACHE=${UI_CHECK_CACHE:-${TMPDIR:-/tmp}/axos-ui-check-merlin}
SHOTS=${1:-$WORK/shots}
PW=${PW:-$(npm root -g)/playwright}
export PW
AXOSD_PID= PROXY_PID=
cleanup() { [ -n "$AXOSD_PID" ] && kill "$AXOSD_PID" 2>/dev/null; [ -n "$PROXY_PID" ] && kill "$PROXY_PID" 2>/dev/null; rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# --- Merlin assets (cached) ---
BASE=https://raw.githubusercontent.com/RMerl/asuswrt-merlin.ng/master/release/src/router/www
mkdir -p "$CACHE"
for f in index_style.css form_style.css images/New_ui/accountadd.png images/New_ui/accountdelete.png \
         images/New_ui/accountedit.png images/New_ui/enable.svg images/New_ui/disable.svg \
         images/New_ui/helpicon.png images/InternetScan.gif; do
  [ -s "$CACHE/$f" ] && continue
  mkdir -p "$CACHE/$(dirname "$f")"
  curl -fsS -m 30 -o "$CACHE/$f" "$BASE/$f" || echo "ui-check: WARN could not fetch $f (pages still load, less styled)" >&2
done

# --- harness root: Merlin shell + AXOS pages generated like axos-merlin-ui.sh ---
H=$WORK/harness
mkdir -p "$H/userRpm" "$H/js" "$SHOTS"
cp -R "$CACHE/." "$H/"
cp "$HERE/state.js" "$H/state.js"
: >"$H/js/jquery.js"; : >"$H/general.js"; : >"$H/popup.js"; : >"$H/help.js"
cp "$REPO/web/axos-ui.js" "$REPO/web/merlin/axos-embed.css" "$H/userRpm/"
echo 'window.AXOS_UI_TOKEN="ui-check"; window.AXOS_API_BASE="";' >"$H/userRpm/token.js"
gen() {
  sed -e "s|__AXOS_PAGE__|$1|g" -e "s|__AXOS_SECTION__|$2|g" -e "s|__AXOS_TITLE__|$3|g" -e "s|__AXOS_DESC__||g" \
      -e 's|<% nvram_get("[a-z_]*"); %>||g' -e 's|<#Web_Title#>|ASUS Wireless Router|g' \
      "$REPO/web/merlin/Axos_Content.asp" >"$H/$1"
}
gen Main_GameServer_Content.asp all Overview
gen Advanced_VPN_PPTP.asp vpn VPN
gen Advanced_APPList_Content.asp lan LAN
gen WAN_info.asp dns DNS
gen Advanced_VPN_IPSec.asp firewall Firewall
gen Advanced_AiDisk_webdav.asp qos QoS
gen WiFi_Insight.asp wifi Wi-Fi
gen Guest_network.asp diag Tools

# --- axosd on the mock backend + proxy ---
(cd "$REPO" && go build -o "$WORK/axosd" ./cmd/axosd)
mkdir -p "$WORK/state/axos/backups"
start_axosd() {
  (cd "$WORK/state" && exec "$WORK/axosd" serve -backend mock -backup-dir "$WORK/state/axos/backups" \
     -ui-dir "$REPO/web" -audit "$WORK/state/audit.jsonl" -footprint "$WORK/state/fp.jsonl") >"$WORK/axosd.log" 2>&1 &
  AXOSD_PID=$!
  for _ in 1 2 3 4 5 6 7 8 9 10; do curl -fs http://127.0.0.1:9090/healthz >/dev/null 2>&1 && return 0; sleep 0.3; done
  echo "ui-check: axosd did not start" >&2; cat "$WORK/axosd.log" >&2; exit 1
}
seed() {
  B=http://127.0.0.1:9090
  c() { curl -fsS -XPOST "$B$1" -d "$2" >/dev/null; }
  c /v1/vpn/wireguard/import '{"unit":5,"description":"Cloudflare WARP","private_key":"cHJpdmF0ZWtleQ==","peer_public_key":"cGVlcg==","endpoint":"engage.cloudflareclient.com","endpoint_port":2408,"address":"172.16.0.2/32","allowed_ips":["0.0.0.0/0"],"nat":true}'
  c /v1/vpn/wgc5/up '{}'
  c /v1/vpn/wireguard/import '{"unit":1,"description":"Mullvad UK","private_key":"eA==","peer_public_key":"eQ==","endpoint":"gb-lon-wg-001.relays.mullvad.net","endpoint_port":51820,"address":"10.64.0.2/32","allowed_ips":["0.0.0.0/0"]}'
  c /v1/policy '{"source":"192.168.1.51","interface":"WGC5","description":"TV","enabled":true}'
  c /v1/policy '{"source":"192.168.1.60","remote":"10.0.0.0/8","interface":"WAN","description":"work-bypass","enabled":true}'
  c /v1/dhcp/reservations '{"mac":"11:22:33:44:55:66","ip":"192.168.1.50","hostname":"gaming-pc"}'
}
start_axosd
seed
python3 "$HERE/proxy.py" "$H" >"$WORK/proxy.log" 2>&1 &
PROXY_PID=$!
sleep 0.5

echo "==> screenshots + JS error scan"
node "$HERE/shoot.js" "$SHOTS"
echo "==> end-to-end feature test"
node "$HERE/e2e.js"
echo "ui-check: OK (screenshots in $SHOTS)"
