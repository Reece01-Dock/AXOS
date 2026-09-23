# Merlin UI embed (hot-deploy, no rebuild)

AXOS appears as an **AXOS** tab inside the Merlin menus where those features
already live — not only under Administration:

| Merlin menu | AXOS focus | Overlay page (basename) |
|---|---|---|
| **VPN** | profiles, Director, endpoint ping | `Advanced_VPN_PPTP.asp` |
| **LAN** | DHCP reservations + clients | `Advanced_APPList_Content.asp` |
| **WAN** | DNS upstreams / DoT | `WAN_info.asp` |
| **Firewall** | filter preview | `Advanced_VPN_IPSec.asp` |
| **Adaptive QoS** | QoS toggle | `Advanced_AiDisk_webdav.asp` |
| **Wireless** | radios + clients | `WiFi_Insight.asp` |
| **Network Tools** | diagnostics | `Guest_network.asp` |
| **Administration** | full control panel | `Main_GameServer_Content.asp` |

(After **Firmware Upgrade** in the Administration tab strip.)

Each overlay is an unused stock ASP bind-mounted from JFFS. Merlin’s menu code
matches **basenames** only, so every panel needs its own `/www/*.asp` name.

## Control panel

Shared FormTable UI (`web/merlin/Axos_Content.asp` + `axos-embed.js`), filtered
by `window.AXOS_SECTION`. Mutating actions use rollback arm/confirm.

Focused panels link back to the full Administration AXOS page.

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
# edit web/merlin/Axos_Content.asp (or axos-embed.js / .css)
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
