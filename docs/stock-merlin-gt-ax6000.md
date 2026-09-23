# Stock Merlin GT-AX6000 baseline

Status document for the unmodified Asuswrt-Merlin build used as the
hardware-test baseline. This is **not** an AXOS-patched image.

## Pin

| Item | Value |
|---|---|
| Upstream branch (official `tools/build-all`) | `3006.102-wifi6` (`BRANCH_GTAX6000`) |
| Checkout commit | `d832d71c8b6d32cc5ae57c0cfbbe4d5249592fea` |
| SDK directory | `release/src-rt-5.04axhnd.675x` |
| Make target | `gt-ax6000` (no extra make variables) |
| Toolchains | `am-toolchains` @ `d1af80e6b6686a4edc680386c09a8361453dd5c1` |
| Source tree | `firmware/src-stock/asuswrt-merlin.ng` (separate from AXOS submodule) |
| Output dir | `firmware/out-stock/` |

**Do not build GT-AX6000 from multi-model tag `3006.102.8_4` alone.** That tag
lacks model prebuilds required for this router (e.g.
`shared/prebuild/GT-AX6000/uu_utils.o` and the privacy CGI objects in
`httpd/prebuild/GT-AX6000/web_hook.o`). Upstream’s own automation checks out
`3006.102-wifi6` for this model.

Matching official prebuilt release (SourceForge):
`GT-AX6000_3006_102.8_4_nand_squashfs.pkgtb`
SHA-256 `9497ab9a5956da9b6c9b86a1c0172e0eef9d8e02c9531598332f0a911a92e925`.

## Backup of prior AXOS work

Before resetting/separating the build tree, everything recoverable was copied to:

`/home/reece/AXOS-backups/stock-baseline-prep-20260923T174613Z/`

Contents include: all `firmware/patches/*.patch`, the failed patched image +
manifest, Merlin dirty-tree diff/status, and build scripts as they were.

## How to reproduce

```sh
cd firmware
# pristine tree must already be branch 3006.102-wifi6 under src-stock/
./build-stock.sh
```

Guarantees enforced by `build-stock.sh` / `docker/entrypoint-stock.sh`:

- `APPLY_PATCHES=1` is rejected
- `firmware/patches/` is never mounted into the container
- stock entrypoint runs plain `make gt-ax6000` (no RTCONFIG overrides)
- tree must be git-clean and must contain `uu_utils.o` for GT-AX6000
- artifacts go only to `firmware/out-stock/`

## Privacy / first-time setup (source evidence on wifi6)

On `3006.102-wifi6`, the handlers that were `#if 0`’d by AXOS patch `0010` are
present as GT-AX6000 prebuilds:

| Symbol | Object |
|---|---|
| `do_set_ASUS_privacy_policy_cgi` / `do_get_ASUS_privacy_policy_cgi` | `httpd/prebuild/GT-AX6000/web_hook.o` |
| `get_ASUS_privacy_policy_state` | `shared/prebuild/GT-AX6000/spwenc.o` |
| `set_ASUS_NEW_EULA` / EULA helpers | `libwebapi/prebuild/GT-AX6000/priv_webapi.o` |

Official SF `httpd` also contains `set_ASUS_privacy_policy.cgi*` /
`do_set_ASUS_privacy_policy_cgi` strings and UI calls in `www/js/httpApi.js`.

## Relation to the previous setup failure

Hypothesis confirmed by source/history: AXOS patch
`0010-gt-ax6000-httpd-missing-symbol-guards.patch` deliberately removed the
privacy CGI routes from `mime_handlers[]` to work around missing symbols on
the old `3004.388.9` pin — producing the observed HTTP 404. Stock wifi6
source retains those routes and ships the missing objects.

## Validation ledger

Filled in when the stock build finishes; hardware items remain NOT TESTED
until explicitly authorized.

### Pre-flash (2026-09-23 local build)

| Check | Result |
|---|---|
| `make gt-ax6000` exit 0 | PASS |
| No AXOS patches applied | PASS |
| No AXOS branding in `Main_Login.asp` | PASS |
| Image: `firmware/out-stock/GT-AX6000_3006_102.9_beta1_nand_squashfs.pkgtb` | PASS |
| Size / SHA-256 | 70,443,084 / `2d8c403ea2252e13bbe857450fc10778894b9c17b0d7c156a54621f2d500b85b` |
| Upstream commit | `d832d71c8b6d32cc5ae57c0cfbbe4d5249592fea` (`3006.102-wifi6`) |
| `httpd` contains `set_ASUS_privacy_policy.cgi*` + `do_set_ASUS_privacy_policy_cgi` | PASS (extracted rootfs) |
| Matches SF `3006.102.8_4` byte-identical | FAIL (expected — wifi6 tip is `3006.102.9_beta1`) |
| Live httpd / first-time setup on router | PASS (privacy CGI strings present in on-device `httpd`; UI reachable) |
| Hardware flash / boot / Wi-Fi / WAN | PASS — see `docs/flashing-and-recovery.md` flash log |

See also: `firmware/out-stock/*.manifest.json` and the backup directory’s
`STOCK_PIN.md` / build logs under `firmware/.build-state/stock-logs/`.
