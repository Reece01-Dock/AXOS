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

Stock wifi6 builds and runs on GT-AX6000 with **no patches**. Active AXOS
diffs are the Milestone 1 login-title marker and Phase 9 JFFS bootstrap.

## Active patches

| Patch | Role |
|---|---|
| `0001-axos-login-title-marker.patch` | Appends ` (AXOS)` to the web UI login tab title |
| `0002-axos-jffs-bootstrap.patch` | Adds `release/src/router/others/axos-bootstrap` (Phase 9); installs to `/usr/sbin/axos-bootstrap` when `others/Makefile` is hooked via `gen-0002.sh` |

```sh
APPLY_PATCHES=1 ./build.sh
```

Verification once flashed: browser tab reads `ASUS Login (AXOS)`.

### Phase 9 bootstrap (`0002`)

Source of truth: `firmware/patches/axos-bootstrap/axos-bootstrap.sh` (kept in
sync with `scripts/router/axos-bootstrap.sh`). The checked-in `0002-*.patch`
adds the script under Merlin's `release/src/router/others/`.

To regenerate (or add the Makefile install rule + optional services-start
hook) against a checked-out submodule:

```sh
git submodule update --init firmware/src/asuswrt-merlin.ng
./firmware/patches/gen-0002.sh
cd firmware/src/asuswrt-merlin.ng && git apply --check ../../patches/0002-axos-jffs-bootstrap.patch
```

On a **running** squashfs image we cannot install into `/usr/sbin` — JFFS
`services-start` (see `scripts/router/install-axosd.sh`) remains the primary
boot hook. The firmware bake-in is for the **next** image build so
`/usr/sbin/axos-bootstrap` exists and can be invoked from post-mount /
services paths.

## Retired patches (`retired-3004.388.9/`)

All former build-fix / symbol-guard patches from the old `3004.388.9` pin
(and wifi6 `apply --check` holdovers). **Not** applied by `build.sh` (it only
globs `patches/*.patch`). Do not re-apply `0010` on wifi6 — it `#if 0`'d the
privacy CGI routes and caused first-time-setup 404s.
