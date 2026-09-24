#!/bin/sh
# axos-merlin-ui.sh — hot-plug AXOS into stock Merlin menus without a firmware rebuild.
#
# Merlin's state.js matches menu entries by ASP *basename* (===). Each AXOS
# panel therefore overlays a different unused stock ASP under /www/.
#
# Tabs (tabName "AXOS") are injected into the Merlin sections where those
# features already live — VPN, LAN, WAN, Firewall, QoS, Wireless, Network
# Tools — plus the full panel under Administration (after Firmware Upgrade).
set -eu

STATE_DIR="${STATE_DIR:-/jffs/axos}"
UI_DIR="$STATE_DIR/merlin-ui"
TOKEN_FILE="$STATE_DIR/run/ui.token"
MENU_SRC="/www/require/modules/menuTree.js"
MENU_DST="$UI_DIR/menuTree.js"
MENU_STOCK="$UI_DIR/menuTree.stock.js"
TEMPLATE="$UI_DIR/Axos_Content.asp"

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

# --- Build per-menu ASP overlays from the shared template --------------------
# page|section|title|desc|menu-anchor-asp
# Anchors are stock tabs we insert *after* in menuTree.js.
gen_page() {
  page="$1"
  section="$2"
  title="$3"
  desc="$4"
  out="$UI_DIR/$page"
  if [ ! -f "$TEMPLATE" ]; then
    echo "axos-merlin-ui: missing template $TEMPLATE" >&2
    return 1
  fi
  # Escape & for sed replacement carefully — titles/descs are plain ASCII.
  sed -e "s|__AXOS_PAGE__|$page|g" \
      -e "s|__AXOS_SECTION__|$section|g" \
      -e "s|__AXOS_TITLE__|$title|g" \
      -e "s|__AXOS_DESC__|$desc|g" \
      "$TEMPLATE" >"$out"
  chmod 644 "$out"
  # Bind over stock /www page (must already exist on squashfs).
  if [ -f "/www/$page" ]; then
    umount "/www/$page" 2>/dev/null || true
    mount --bind "$out" "/www/$page"
  else
    echo "axos-merlin-ui: WARN — /www/$page missing, skip bind" >&2
  fi
}

if [ -f "$TEMPLATE" ]; then
  gen_page "Main_GameServer_Content.asp" "all" "Overview" \
    "AXOS status, system, interfaces and backups."
  gen_page "Advanced_VPN_PPTP.asp" "vpn" "VPN" \
    "Route LAN clients through VPN tunnels (VPN Director), import WireGuard, test endpoints."
  gen_page "Advanced_APPList_Content.asp" "lan" "LAN" \
    "DHCP reservations and LAN clients."
  gen_page "WAN_info.asp" "dns" "DNS" \
    "WAN and LAN DNS servers, DNS-over-TLS."
  gen_page "Advanced_VPN_IPSec.asp" "firewall" "Firewall" \
    "Live iptables rules."
  gen_page "Advanced_AiDisk_webdav.asp" "qos" "QoS" \
    "QoS on/off and live throughput."
  gen_page "WiFi_Insight.asp" "wifi" "Wi-Fi" \
    "Radios and wireless client signal."
  gen_page "Guest_network.asp" "diag" "Tools" \
    "Ping, traceroute, nslookup, port check, iperf3."
fi

# --- Patch menuTree: AXOS tab in each relevant Merlin section ----------------
umount "$MENU_SRC" 2>/dev/null || true
if [ -f "$MENU_SRC" ]; then
  # After umount, MENU_SRC is the real squashfs file — always refresh stock.
  cp -a "$MENU_SRC" "$MENU_STOCK"
  cp -a "$MENU_STOCK" "$MENU_DST"

  awk '
    function emit(page) {
      print "{url: \"" page "\", tabName: \"AXOS\"},"
    }
    {
      print
      # Administration — full panel after Firmware Upgrade
      if ($0 ~ /Advanced_FirmwareUpgrade_Content\.asp/ && $0 ~ /tabName/ && !a_admin) {
        emit("Main_GameServer_Content.asp"); a_admin=1
      }
      # VPN — after VPN Director
      if ($0 ~ /Advanced_VPNDirector\.asp/ && $0 ~ /tabName/ && !a_vpn) {
        emit("Advanced_VPN_PPTP.asp"); a_vpn=1
      }
      # LAN — after DHCP
      if ($0 ~ /Advanced_DHCP_Content\.asp/ && $0 ~ /tabName/ && !a_lan) {
        emit("Advanced_APPList_Content.asp"); a_lan=1
      }
      # WAN — after ASUS DDNS (DNS-adjacent)
      if ($0 ~ /Advanced_ASUSDDNS_Content\.asp/ && $0 ~ /tabName/ && !a_wan) {
        emit("WAN_info.asp"); a_wan=1
      }
      # Firewall — after Basic Firewall
      if ($0 ~ /Advanced_BasicFirewall_Content\.asp/ && $0 ~ /tabName/ && !a_fw) {
        emit("Advanced_VPN_IPSec.asp"); a_fw=1
      }
      # QoS / Bandwidth Monitor — after EZQoS
      if ($0 ~ /QoS_EZQoS\.asp/ && $0 ~ /tabName/ && !a_qos) {
        emit("Advanced_AiDisk_webdav.asp"); a_qos=1
      }
      # Wireless — after main Wireless tab
      if ($0 ~ /Advanced_Wireless_Content\.asp/ && $0 ~ /tabName/ && !a_wifi) {
        emit("WiFi_Insight.asp"); a_wifi=1
      }
      # Network Tools — after Analysis
      if ($0 ~ /Main_Analysis_Content\.asp/ && $0 ~ /tabName/ && !a_diag) {
        emit("Guest_network.asp"); a_diag=1
      }
    }
    END {
      if (!a_admin) exit 11
      if (!a_vpn)   exit 12
      if (!a_lan)   exit 13
      if (!a_wan)   exit 14
      if (!a_fw)    exit 15
      if (!a_qos)   exit 16
      if (!a_wifi)  exit 17
      if (!a_diag)  exit 18
    }
  ' "$MENU_DST" >"$MENU_DST.tmp" && mv "$MENU_DST.tmp" "$MENU_DST"

  axos_tabs=$(grep -c 'tabName: "AXOS"' "$MENU_DST" || true)
  if [ "$axos_tabs" -lt 8 ]; then
    echo "axos-merlin-ui: ERROR — expected 8 AXOS tabs, found $axos_tabs" >&2
    exit 1
  fi

  mount --bind "$MENU_DST" "$MENU_SRC"
fi

echo "axos-merlin-ui: ready — AXOS tabs in VPN / LAN / WAN / Firewall / QoS / Wireless / Tools / Administration"
echo "axos-merlin-ui: log out and back in (or private window) if Merlin Session menu cache is stale"
