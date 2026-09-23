# Merlin UI embed (hot-deploy, no rebuild)

AXOS appears as a native **AXOS → Control** entry in the stock Asuswrt-Merlin
web UI. The pages live on JFFS and are bind-mounted into httpd at boot — so
you iterate with `deploy-router.sh`, not a firmware flash.

## How it works

| Piece | Where | Rebuild? |
|---|---|---|
| Menu entry + pages | `/jffs/axos/merlin-ui/` → bind `/www/userRpm` + patched `menuTree.js` | **No** — hot |
| Core API | `axosd` on `0.0.0.0:9090` | **No** — hot |
| LAN auth | `/jffs/axos/run/ui.token` → `X-Axos-UI-Token` (loopback exempt) | **No** |
| Optional squashfs bake-in | `0002` bootstrap only | Rare |

`scripts/router/axos-merlin-ui.sh` (called from `axos-bootstrap`):

1. Ensures a UI token and writes `merlin-ui/token.js`
2. `mount --bind /jffs/axos/merlin-ui /www/userRpm`
3. Injects `menu_AXOS` into a JFFS copy of `menuTree.js` and bind-mounts it

Source UI files: `web/merlin/` (staged into each release as `merlin-ui/`).

## Where to find it in the Merlin UI

- **Administration** → tab **AXOS** (immediately after **Firmware Upgrade**)
- Direct URL (while logged in):
  `http://www.asusrouter.com/Main_GameServer_Content.asp`
- From Firmware Upgrade:
  `http://www.asusrouter.com/Advanced_FirmwareUpgrade_Content.asp`
  then click the **AXOS** tab in the same strip

  (That stock filename is an unused page we overlay — Merlin’s menu code
  only matches basenames, so the page has to live at `/www/*.asp`.)

Merlin caches the menu in the browser Session. If you do not see AXOS after
install/upgrade: **log out and log back in** (or use a private window), then
hard-refresh.

## Day-to-day workflow

```sh
# edit web/merlin/Axos_Content.asp (or axos-embed.js / .css)
./scripts/deploy-router.sh Reece@192.168.50.1
ssh router '/jffs/axos/bin/axos-merlin-ui.sh'   # refresh binds
# hard-refresh the Merlin UI tab (or re-login)
```

No `firmware/build.sh`, no flash.

## Security

- Merlin httpd still requires an admin login before `userRpm/Axos_Content.asp` loads.
- The browser then calls `:9090` with the token from `token.js` (only served after that login).
- Loopback clients (MCP over SSH, on-router `axosctl`) do **not** need the token.
- Do not forward WAN port 9090.

See `docs/security.md` and `internal/api/lan_auth.go`.
