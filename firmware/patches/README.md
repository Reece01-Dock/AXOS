# AXOS firmware patches

Every AXOS modification to the Merlin source tree lives here as an ordered
`git apply`-able patch, applied by `build.sh` when `APPLY_PATCHES=1`. We never
edit `firmware/src/asuswrt-merlin.ng` and commit the tree itself — the patch
files are the record of our diff.

## Current Merlin pin

| Item | Value |
|---|---|
| Upstream branch | `3006.102-wifi6` |
| Commit | `d832d71c8b6d32cc5ae57c0cfbbe4d5249592fea` |
| Stock baseline | `docs/stock-merlin-gt-ax6000.md` |

Stock wifi6 builds and runs on GT-AX6000 with **no patches**. The only active
AXOS diff is the Milestone 1 login-title marker.

## Active patches

| Patch | Role |
|---|---|
| `0001-axos-login-title-marker.patch` | Appends ` (AXOS)` to the web UI login tab title |

```sh
APPLY_PATCHES=1 ./build.sh
```

Verification once flashed: browser tab reads `ASUS Login (AXOS)`.

## Retired patches (`retired-3004.388.9/`)

All former build-fix / symbol-guard patches from the old `3004.388.9` pin
(and wifi6 `apply --check` holdovers). **Not** applied by `build.sh` (it only
globs `patches/*.patch`). Do not re-apply `0010` on wifi6 — it `#if 0`'d the
privacy CGI routes and caused first-time-setup 404s.
