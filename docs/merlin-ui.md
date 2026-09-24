# Merlin UI embed (hot-deploy, no rebuild)

AXOS appears as an **AXOS** tab inside the Merlin menus where those features
already live — not only under Administration:

| Merlin menu | AXOS focus | Overlay page (basename) |
|---|---|---|
| **VPN** | VPN client status + connect, route LAN clients to a tunnel / WAN, client groups, staged VPN Director rule editor, WireGuard import (incl. `.conf`), endpoint latency | `Advanced_VPN_PPTP.asp` |
| **LAN** | DHCP reservations (pick a client or type), client list with one-click reserve | `Advanced_APPList_Content.asp` |
| **WAN** | WAN DNS (auto/manual), DNS-over-TLS servers + presets, LAN DNS, WAN status, lookup test | `WAN_info.asp` |
| **Firewall** | live iptables rules (filter/search), add / delete | `Advanced_VPN_IPSec.asp` |
| **Adaptive QoS** | QoS on/off, bandwidth, live throughput | `Advanced_AiDisk_webdav.asp` |
| **Wireless** | radios, wireless clients by signal | `WiFi_Insight.asp` |
| **Network Tools** | ping, traceroute, nslookup, port check, iperf3 | `Guest_network.asp` |
| **Administration** | overview: AXOS status, rollback, system, interfaces, services, backups/restore | `Main_GameServer_Content.asp` |

(After **Firmware Upgrade** in the Administration tab strip.)

Each overlay is an unused stock ASP bind-mounted from JFFS. Merlin’s menu code
matches **basenames** only, so every panel needs its own `/www/*.asp` name.

## UI code

One renderer, two frontends: `web/axos-ui.js` builds every section with
Merlin's own markup (`FormTable`, `FormTable_table` + `list_table`,
`add_btn`/`remove_btn`/`edit_btn`, `apply_gen`, Yes/No radios), so inside
Merlin it is styled entirely by the stock `form_style.css`
(`web/merlin/axos-embed.css` only adds what Merlin keeps in per-page
`<style>` blocks). The standalone page on `:9090` (`web/index.html`) uses
the same renderer with `web/styles.css`, a copy of the Merlin look.

Every change is wrapped in a 2-minute safety rollback (arm → change →
confirm; a failed change is left armed so the router restores itself).
VPN Director edits are staged like Merlin's page and written in one Apply.

Test both UIs in headless Chromium against the mock backend:

```sh
scripts/ui-check/run.sh [screenshot-dir]
```

## How it works

| Piece | Where | Rebuild? |
|---|---|---|
| Menu entries + pages | `/jffs/axos/merlin-ui/` → bind `/www/userRpm` + per-page `/www/*.asp` + patched `menuTree.js` | **No** — hot |
| Core API | `axosd` on `0.0.0.0:9090` | **No** — hot |
| LAN auth | `/jffs/axos/run/ui.token` → `X-Axos-UI-Token` (loopback exempt); CORS allows `www.asusrouter.com` | **No** |
| Optional squashfs bake-in | `0002` bootstrap only | Rare |

`scripts/router/axos-merlin-ui.sh` (called from `axos-bootstrap`):

1. Ensures a UI token and writes `merlin-ui/token.js`
2. `mount --bind` merlin-ui → `/www/userRpm`
3. Generates section ASP files from `Axos_Content.asp` and bind-mounts each over its stock `/www` target
4. Injects `{url, tabName: "AXOS"}` into each relevant `menuTree.js` section

Source UI files: `web/merlin/` (staged into each release as `merlin-ui/`).

## Where to find it

Examples (logged in):

- VPN → **AXOS**: `http://www.asusrouter.com/Advanced_VPN_PPTP.asp`
- Administration → **AXOS**: `http://www.asusrouter.com/Main_GameServer_Content.asp`
- From Firmware Upgrade, click the **AXOS** tab in the same strip

Merlin caches the menu in the browser Session. After install/upgrade: **log out
and log back in** (or private window), then hard-refresh.

## Day-to-day workflow

```sh
# edit web/axos-ui.js (or web/merlin/Axos_Content.asp / axos-embed.css)
scripts/ui-check/run.sh
./scripts/deploy-router.sh Reece@192.168.50.1
# or hot-copy + re-run inject:
#   cat web/merlin/Axos_Content.asp | ssh router 'cat > /jffs/axos/merlin-ui/Axos_Content.asp'
ssh router '/jffs/axos/bin/axos-merlin-ui.sh'
# log out/in, hard-refresh
```

No `firmware/build.sh`, no flash.

## Security

- Merlin httpd still requires an admin login before any AXOS ASP loads.
- The browser then calls `:9090` with the token from `token.js` (only served after that login).
- Loopback clients (MCP over SSH, on-router `axosctl`) do **not** need the token.
- Do not forward WAN port 9090.

See `docs/security.md` and `internal/api/lan_auth.go`.
