# AXOS Roadmap

This file is the source of truth for project progress. A box is checked `[x]`
only when the step has been **verified on real hardware or a real build
machine** — not when the code/scripts for it exist.

Legend: `[ ]` not done · `[x]` done and verified on real router hardware ·
`[s]` built and verified **in this development sandbox** — passing automated
tests, and for the parts that matter most, the actual compiled binaries run
against each other as real, separate OS processes (not just `go test`) —
but **not yet run against real router hardware** · `(scripted)` tooling
exists but hasn't been executed/verified at all yet.

`[s]` is a real, meaningful bar (see `docs/development.md` "Status: what's
actually verified" for exactly what was run), but it is not `[x]`. As of
2026-09-23:

- **Milestone 1** is complete on a physical GT-AX6000 (stock Merlin
  `3006.102-wifi6` + AXOS login-title marker flashed and verified — see
  `docs/flashing-and-recovery.md`). Optional: 2.5GbE cable retest (step 6).
- **Milestone 2** is complete on hardware including Phase 7 web UI (served
  from `axosd` at `/` + `/ui/*`, hot-deployed under `/jffs/axos/www`) and
  Phase 9 JFFS bootstrap (`axos-bootstrap` + `0002` firmware patch ready
  for the next image bake; live hook is `/jffs/scripts/services-start`).
- **Milestone 3** Core API + MCP tools are live on the router (diagnostics,
  DNS/DHCP/QoS, WireGuard import, OpenVPN slots, VPN Director policy,
  firewall apply/delete, secrets dir). Cloudflare WARP registration and
  full auto endpoint-selection loops remain thinner (WG-import path / ping).
- **Milestone 4** has `internal/optimiser` skeletons for ethernet / Wi-Fi /
  VPN / latency / Performance Mode (`[s]`); live optimisation loops are
  not hardware-verified yet.

## Milestone 1 — Prove the build → flash → recover loop

The single most important milestone. No AXOS functionality before this is green.

**Hardware status (2026-09-23):** Milestone 1 complete on a physical GT-AX6000
at `192.168.50.1` — stock Merlin wifi6 flashed and verified, then AXOS
login-title marker rebuilt/flashed/verified (`ASUS Login (AXOS)`). Remaining
optional: cable-linked 2.5GbE retest (step 6).

- [x] 1. Build Asuswrt-Merlin from source for GT-AX6000 — stock tree
      `firmware/src-stock/` on branch `3006.102-wifi6`
      (`d832d71c8b6d32cc5ae57c0cfbbe4d5249592fea`), plain `make gt-ax6000`,
      exit 0. (Earlier sandbox work on `3004.388.9` + AXOS patches is
      superseded; that pin lacks GT-AX6000 prebuilds and is not the
      upstream build-all branch for this model.)
- [x] 2. Produce a firmware image (`.w` / `.pkgtb`) —
      `firmware/out-stock/GT-AX6000_3006_102.9_beta1_nand_squashfs.pkgtb`,
      70,443,084 bytes, sha256
      `2d8c403ea2252e13bbe857450fc10778894b9c17b0d7c156a54621f2d500b85b`.
- [x] 3. Flash it via the stock ASUS web UI — uploaded over
      `http://192.168.50.1/upgrade.cgi` from ASUS stock
      `3.0.0.6.102_37436`; router rebooted and came back.
- [x] 4. Router boots — web UI + SSH after flash.
- [x] 5. 1GbE Ethernet works — `eth1` up at 1G; LAN at `192.168.50.1`.
- [ ] 6. Both 2.5GbE ports work — PHY present (`eth5` advertises 2.5G);
       **no cable linked** at check time (link down). Retest with cable.
- [x] 7. 2.4 GHz Wi-Fi works — `wl` on eth6: SSID `Reece-Net`, ch 2.
- [x] 8. 5 GHz Wi-Fi works — `wl` on eth7: SSID `Reece-Net`, ch 36/80.
- [x] 9. ASUS web UI works — login + authenticated `appGet.cgi`.
- [x] 10. SSH works — `sshd_enable=1`, port 22; initially password login as
       `Reece`, then hardened to **key-only** (`sshd_pass=0`) in Milestone 2
       — see `docs/ssh-access.md`. `uname` reports `ASUSWRT-Merlin`.
- [x] 11. Recovery mode verified (bootloader rescue + firmware restoration —
       **before** this Merlin flash the unit was recovered from a bad
       custom image via ASUS Firmware Restoration — recorded by operator).
       Formal re-document of LED/button procedure still optional polish.
- [x] 12. Create and store a settings + JFFS backup —
      `/home/reece/AXOS-backups/pre-axos-marker-20260923T194612Z/`
      (`nvram-full.txt`, `settings.cfg` via `nvram save`, `jffs.tar.gz`).
- [x] 13. Make one harmless, visible source modification —
      **only** `0001-axos-login-title-marker.patch` on `3006.102-wifi6`.
- [x] 14. Rebuild — `APPLY_PATCHES=1` image
      `firmware/out/GT-AX6000_3006_102.9_beta1_nand_squashfs.pkgtb`,
      70,443,084 bytes, sha256
      `193be109f44987fb52486613cb4c0b1fa0ebc607290005c8d822386df1ca84e6`
      (differs from stock `2d8c403e…`).
- [x] 15. Flash again — web UI `upgrade.cgi` from running Merlin stock
      wifi6; router rebooted.
- [x] 16. Verify the modification is visible on the router —
      `Main_Login.asp` `<title>` is **`ASUS Login (AXOS)`**; SSH up;
      `uname` build time `Wed Sep 23 19:46:41 UTC 2026`.

### What was actually verified this session (real, against real upstream source)

No shortcuts here — every item below was checked against a real, network-fetched
clone of `RMerl/asuswrt-merlin.ng` (cleaned up afterward; none of this is
committed as vendored source — see `firmware/patches/README.md` for why the
Merlin tree itself isn't vendored into this repo):

- **Network access confirmed.** `git ls-remote`/`git clone`/`git fetch` to
  `github.com` all work from this environment (through the pre-configured
  egress proxy) — a plain `curl` HEAD request gets blocked, but git's own
  protocol goes through fine.
- **Pinned tag confirmed to exist**: `3004.388.9` (resolves to commit
  `bab3cb030c`, "Bumped revision to 3004.388.9 final").
- **Found and fixed a real bug in `firmware/build.sh`**: the build target
  was `make gtax6000` (no dash) — invalid. The correct target is
  `make gt-ax6000` (confirmed two ways: `chip_profile.mak` defines
  `GT-AX6000_CHIP_PROFILE=4912`, matched via the Makefile's
  `$(MAKECMDGOALS)_CHIP_PROFILE` mechanism only when the goal is literally
  `gt-ax6000`; and the upstream project's own multi-model build automation,
  `tools/build-all`, calls exactly `cd release/src-rt-5.04axhnd.675x && make
  "$FWMODEL"` with `FWMODEL="gt-ax6000"` for this device). Fixed in
  `firmware/build.sh`.
- **Confirmed the build subdirectory** (`release/src-rt-5.04axhnd.675x`) and
  **the built-image naming pattern** (`*_nand_squashfs.pkgtb` in that
  directory's `image/` subdir) against the same `tools/build-all` script —
  `firmware/build.sh`'s output-detection now checks there first.
- **Measured real disk requirements** instead of guessing: `asuswrt-merlin.ng`
  at the pinned commit is ~11 GB (shallow), `am-toolchains` at its pinned
  commit is ~3.1 GB (shallow) — ~14 GB for source alone, both fetched for
  real, inspected, then cleaned up locally (nothing vendored — see below).
  That comfortably fit this environment's 30 GB disk allowance with ~16 GB
  left over for the Docker build itself, which has *not* been attempted —
  whether that headroom is enough for the actual build's object files and
  output is still unverified. `docs/build-environment.md` now states ~20 GB
  for source (measured) and ~60 GB recommended overall (build headroom,
  still an estimate).
- **Wrote and verified the step-13 patch** against the real pinned-tag
  source (see step 13 above).
- **Pinned both sources as git submodules**, not left as "a script will
  clone them": `firmware/src/asuswrt-merlin.ng` and
  `firmware/src/am-toolchains` are gitlinks in this repo's own tree,
  pointing at the exact verified commits above (`.gitmodules` at the repo
  root). This is a real, visible link — GitHub renders a submodule entry as
  a clickable jump straight to that commit on the upstream repo — not a
  promise inside a shell script's default variable. Creating a full fork or
  a brand-new mirror repository was attempted first and is **not possible
  with this session's GitHub permissions** (the GitHub App installation
  used here can modify `reece01-dock/axos` but cannot fork a
  differently-owned repo or create new repositories — both attempts
  returned explicit permission errors, not silently skipped). The submodule
  pin achieves the same practical goal — an exact, GitHub-visible,
  version-controlled reference to the real source — without needing either.
  `firmware/setup-sources.sh` now just runs `git submodule update --init`
  against these pins rather than maintaining its own separate clone/ref logic.

**Not done, and not possible from this environment:** the actual Docker
build step — and now confirmed *why*, not just "disk headroom is
unverified": this session's network egress policy blocks
`production.cloudfront.docker.com`, the CDN Docker Hub redirects to for
actual image layer blobs. `docker pull ubuntu:20.04` (the very first line
of `build.sh`, before anything AXOS-specific even runs) fails with a 403.
Confirmed twice — once via a direct `dockerd` (no proxy env vars, got a
403 straight from CloudFront) and once with `dockerd` explicitly pointed
at this session's egress proxy (the proxy's own status log recorded
`connect_rejected ... policy denial` for that exact host) — so this isn't
a retry-able transient failure or something a Dockerfile/build.sh change
can route around; it's a hard block on this environment. Per this
environment's own proxy documentation, a 403/407 policy denial is
something to report, not bypass.

### First real build attempt outside this sandbox (on a real VM, real internet)

The user ran `firmware/setup-sources.sh && ./firmware/build.sh` on an actual
Ubuntu Server VM with unrestricted internet — confirming the sandbox's
Docker Hub block above doesn't apply to a normal machine. The build got
substantially further than anything run in this project's own sandboxed
environment ever could: past `docker build`, past mounting the toolchains,
into the actual HND SDK build (compiled BusyBox, ran the CFE/wireless
firmware sysdeps copy steps for the GT-AX6000 profile), before hitting a
real bug:

```
ERROR: /bin/sh does not invoke bash shell
make[2]: *** [.../make.common:3450: prebuild_checks] Error 1
```

**Root cause, confirmed against the real error output**: Ubuntu's `/bin/sh`
is `dash` by default — installing the `bash` package (which
`firmware/docker/Dockerfile` already did) doesn't change that. The HND
SDK's `make.common` has an explicit `prebuild_checks` target that requires
`/bin/sh` to actually invoke bash and fails the whole build otherwise.
**Fixed** in `firmware/docker/Dockerfile`: `RUN ln -sf /bin/bash /bin/sh`
forces the symlink.

That fix got the next attempt past `prebuild_checks`' shell check into its
header/library checks, where it hit a second real bug:

```
fatal error: lzo/lzo1x.h: No such file or directory
ERROR: lzo/lzo1x.h development library is required for build
```

**Fixed** the same way, `liblzo2-dev` added to the Dockerfile's package
list. This time verified against the actual `prebuild_checks` target
source (`make.common`, checked out for real in `firmware/src/`) rather than
just the one failing header: read the whole target end to end and confirmed
every other header/pkg-config check it makes (`uuid/uuid.h`, `pkg-config
zlib`, `pkg-config uuid`) is already covered by packages already in the
Dockerfile (`uuid-dev`, `zlib1g-dev`) — so this should be the last
host-package gap in `prebuild_checks` specifically, though the checks after
it (kernel build, per-profile object build) are a different category and
unverified.

That fix revealed a third, different-*kind* of bug on the next attempt:

```
Package zlib was not found in the pkg-config search path.
ERROR: pkg-config zlib failed
```

— despite `zlib1g-dev` being installed. **Root cause wasn't a missing
package at all**: `firmware/docker/Dockerfile` put the cross-toolchain's
`usr/bin` directories ahead of system `PATH`, and both toolchains bundle
their *own* `pkg-config` binary (confirmed directly:
`crosstools-{aarch64,arm}-gcc-5.5-.../usr/bin/pkg-config` both exist) —
scoped to their own sysroot, so `pkg-config --exists zlib` was silently
running the wrong binary and never saw the system's zlib.pc. **Fixed** by
reordering `PATH` so system tools resolve first (`$PATH:<toolchain
dirs>` instead of `<toolchain dirs>:$PATH`) — confirmed safe by checking
`make.common`: the actual cross-compiler is always invoked via
`$(TOOLCHAIN_TOP)/bin/$(TOOLCHAIN_PREFIX)-gcc`, a fully-qualified path,
never a bare `gcc`/etc. resolved from `PATH`, so this reorder can't affect
which compiler the build actually uses.

That fix got the next attempt substantially further — past `prebuild_checks`
entirely and into the real kernel build (`olddefconfig` against the actual
4.19 kernel tree for this profile) — before hitting a fourth real bug:

```
.../crosstools-aarch64-gcc-9.2-.../cc1: error while loading shared
libraries: libisl.so.15: cannot open shared object file
```

**Root cause, confirmed directly against the real toolchain files**: the
crosstools bundle their own `libisl`/`libmpc`/`libmpfr`/`libgmp`/etc. under
each toolchain's own `lib/` dir — `libisl.so.15` genuinely exists right
there — but `cc1`'s baked-in `RPATH` (`readelf -d cc1`) points at the
*original build machine's* path
(`/home/defjovi/temp3/toolchain/crosstools-.../lib`), not wherever we
mount the toolchain, so the dynamic linker never finds it. **Fixed**: a
static `/etc/ld.so.conf.d/am-toolchains.conf` (added in
`firmware/docker/Dockerfile`) listing every `crosstools-*` lib dir in the
pinned `am-toolchains` commit (enumerated directly from the real checkout,
not guessed — includes the two gcc-5.3 toolchains' `usr/lib` layout,
different from the rest), plus a `sudo ldconfig` re-run in
`firmware/build.sh` right after the toolchains volume is mounted (the
`.conf` file's paths don't exist yet at `docker build` time, only at
`docker run` time, so the cache has to be rebuilt then, not baked into the
image). Verified the Dockerfile itself still parses/builds correctly up to
the same known network-blocked base-image pull from earlier entries in
this log — the RUN steps added here have correct syntax, at minimum.

That fix got the next attempt into the real kernel build proper (HOSTCC of
kernel build scripts, `kernel/bounds.s`) before a fifth, much simpler bug:

```
/bin/sh: bc: command not found
make[5]: *** [Kbuild:42: include/generated/timeconst.h] Error 127
```

Plain missing package — `bc` is a standard Linux kernel build dependency
(used to compute `include/generated/timeconst.h`) that was never in the
Dockerfile's list. **Fixed**: added `bc`.

That fix got the next attempt past the kernel build entirely and into
router userspace — linking `libshared.so` — before a sixth bug, a
different kind again:

```
arm-buildroot-linux-gnueabi-gcc.br_real: error: prebuild/uu_utils.o: No such file or directory
make[5]: *** [Makefile:481: libshared.so] Error 1
```

**Traced in the real source**: `release/src/router/shared/Makefile:472`
pulls in `prebuild/uu_utils.o` (a proprietary ASUS "UU/GearUp cloud
plugin" binary blob, unrelated to core networking) whenever
`RTCONFIG_UUPLUGIN` or `RTCONFIG_GEARUPPLUGIN` is `y`. Confirmed: this
Merlin release's `release/src/router/shared/prebuild/GT-AX6000/` directory
genuinely does not ship `uu_utils.o` (it has 16 *other* prebuilt objects —
that one exists only for RT-AX86U/RT-AX58U/RT-AX68U/RT-AX88U/GT-AX11000).
Also confirmed both Kconfig sources of truth
(`release/src/router/config/config.in`, `config_base`) default this
feature **off**, and GT-AX6000's own config fragment
(`targets/94912GW/94912GW.GT-AX6000`) never turns it on — so something
deeper in the generated `.config` state (not staticly traceable from the
source alone, possibly leftover state from an earlier attempt, since the
build tree persists across retries) is enabling a feature whose binary
this model's source doesn't even ship.

**Fixed**: `firmware/build.sh` now passes `RTCONFIG_UUPLUGIN=n
RTCONFIG_GEARUPPLUGIN=n` as `make` command-line overrides — confirmed no
`override` directive anywhere in the relevant Makefiles that would defeat
this, so it reliably wins regardless of the exact root cause.

That fix hit the exact same failure shape twice more (`RTCONFIG_TPVPN`
pulling in `prebuild/tpvpn.o`), which prompted a full audit instead of
continuing to fix these reactively one at a time: every `prebuild/`
directory in the tree (46 of them) was enumerated, GT-AX6000's file set
compared against every sibling model's for each, and every gap traced to
its consuming Makefile. Findings:

- **Real, fixed**: `RTCONFIG_TPVPN`, `RTCONFIG_AMAS_ADTBW`,
  `RTCONFIG_PRELINK`, `RTCONFIG_BRCM_HOSTAPD` — same shape as UUPLUGIN
  exactly (an unconditional `OBJS +=` in `release/src/router/rc/Makefile`
  for a prebuilt object with no source-file fallback), all four default
  off in `config.in`/`config_base`, GT-AX6000's own fragment
  (`targets/94912GW/94912GW.GT-AX6000`) never overrides any of them, and
  GT-AX6000's `prebuild/` is missing every corresponding object
  (`amas-adtbw-broadcom.o`, `amas_adtbw.o`, `amas_prelink.o`,
  `hostapd_config.o`, `tpvpn.o`, `wps_pbcd.o`).
- **Real, fixed preemptively**: `RTCONFIG_RGBLED` (gates the `aura_sw`
  subdirectory) and `RTCONFIG_BT_CONN` (gates `bluez-5.56`/`btconfig`) —
  GT-AX6000 has no `prebuild/` entry for either *at all* (not even a
  partial one), both default off with no GT-AX6000 override, consistent
  with this model having neither RGB LEDs nor a Bluetooth radio. Not yet
  hit as a live failure, but same shape as everything else that has —
  fixed ahead of time rather than waiting for the build to reach it.
- **Investigated, not a risk**: `dns_dpi_check.o` (also missing from
  GT-AX6000's `rc/prebuild`) is referenced nowhere in tracked source at
  all (`git grep` finds only the orphaned binary itself) — nothing
  consumes it. `asd2.1` (also has no GT-AX6000 `prebuild/` entry) copies
  its prebuilt files with a leading `-` in the Makefile recipe (make's
  ignore-errors-on-this-line prefix) — a missing prebuilt binary there is
  a silent, designed-in no-op, not a hard failure like the `OBJS+=`
  pattern above.

**Fixed**: extended the same command-line-override approach to all six
confirmed flags (`firmware/build.sh`) — confirmed no `override` directive
anywhere in these Makefiles for any of them either.

That fix also hit a real snag of its own: the long explanatory comment
added to `build.sh`'s single-quoted `bash -lc '...'` container script
contained several raw, unescaped apostrophes instead of this file's
established `'\''`-escape idiom — closing the quote early and splicing
the rest into literal outer-shell syntax (quote count still balanced by
EOF, so `bash -n` didn't catch it; the semantic grouping was just wrong).
This is why `make` briefly appeared to run on the host instead of inside
the container. Fixed all 8 raw apostrophes and verified properly this
time — not just `bash -n`, but a stand-in `docker` function confirming
the container script arrives as one clean, unmangled argument.

With that actually fixed, the next attempt got past `bc`/kernel build/
router userspace linking entirely and into a real compile failure:

```
bcm_ethswutils.c:47:10: fatal error: bcmnet.h: No such file or directory
make[5]: *** [<builtin>: bcm_ethswutils.o] Error 1
```

**Traced to a genuine upstream Makefile bug, not a config-flag gap**:
`bcmnet.h` exists in the pinned tree at
`bcmdrivers/opensource/include/bcm963xx/bcmnet.h`, but
`router-sysdep.gt-ax6000/bcm_util/Makefile` only adds that directory to
the include path when `BCM4906_504` is set (a different Broadcom chip
family — GT-AX6000 is chip `4912`, confirmed via
`chip_profile.mak`/`BCM_CHIP`). Checked whether forcing `BCM4906_504=y`
was a safe workaround first (matching the pattern of every other fix so
far) — it is not: that variable also drives real chip-specific `#define`s
(`-DBCM4906_504 -DSUPPORT_MLD`) consumed throughout `rc.c`, `usb.c`,
`shared.h`, `boardapi.c`, and `broadcom.c` to select genuinely different
hardware behavior, so forcing it on for chip 4912 hardware would risk
silently wrong runtime behavior, not just a build fix. Every one of those
same C-level checks already ORs `BCM4912` in alongside `BCM4906_504`
except this one Makefile, which was simply never updated to match.
Also checked the two other files with the identical `BCM4906_504`
conditional pattern (`wlan/Makefile`, `wlan/nvram/Makefile`) — both are
genuine, intentional chip-4906-specific tuning (restricting
`WLANAPP_DIRS`, setting `WL_DEFAULT_NUM_SSID=16`), not bugs, and were
left untouched.

**Fixed as `firmware/patches/0002-gt-ax6000-bcm-util-bcmnet-include.patch`**
— a real source patch, not a `build.sh` override, since this needed an
actual Makefile logic change (OR in `$(filter 4912,$(BCM_CHIP))`
alongside the existing `BCM4906_504` check) that a command-line variable
couldn't safely express. Verified `git apply`/apply-then-revert against
the real pinned checkout, not just `--check`. This means Milestone 1's
"unmodified upstream" build is **not actually buildable as-is** for this
model — `APPLY_PATCHES=1 ./build.sh` is now required, not optional (see
`firmware/patches/README.md`). Not yet verified past `git apply --check`
+ a clean apply/revert cycle — no environment that generated this patch
could run a full build.

Everything requiring physical hardware is separately blocked as before.

### First complete build (this sandbox, disk expanded to 57GB)

The sandbox's disk was expanded (`lvextend` + `resize2fs` on the host,
done by the user — not something this session's own permissions can do)
from 29GB to 57GB, and Docker Hub pulls that were blocked in an earlier
session's environment worked fine in this one (`docker pull ubuntu:20.04`
succeeded directly) — neither of the two blockers recorded above turned
out to still apply here. That unblocked a real, full build attempt
in-sandbox, which is how everything below was found and fixed.

Ten more real, build-attempt-confirmed bugs were found and fixed the same
way as `0002` — patches `0003` through `0012` (see each patch file's own
header for its specific root cause; shapes matched what the `0002`/`prebuild`
audit above already predicted: more `RTCONFIG_*`-gated `OBJS +=` gaps,
missing symbol guards, a `getpid`/`fromfile` multiple-definition clash, a
real parallel-build race fixed with `.NOTPARALLEL` on `router/Makefile`
itself once `-j1` alone wasn't enough headroom on a constrained host, and
the `asd2.1-install` leniency gap predicted but not yet hit before).

That got the build past userspace linking and into `sqlite`, where it hit
an eleventh, differently-shaped bug: `sqlite/`'s `stamp-h1` recipe is the
*only* place in this tree that runs `autoreconf -i -f` at build time
instead of shipping pre-generated autotools output the way every other
autotools subdir here does (`flac`, `libogg`, `strace-4.5.20`, etc. all
carry a tracked `missing`/`aclocal.m4`/`configure`). Confirmed
identically on two separate real build attempts: the recursive
`make -C sqlite all` fails immediately with `./missing: No such file or
directory` trying to rebuild `aclocal.m4`, even though `autoreconf`
had just run successfully moments earlier in the same build and even
produced a working `sqlite3` binary — its auxiliary helper files don't
survive to when the submake re-checks its own Makefile's freshness.
Verified in isolation that `autoreconf -i -f -v` against the pinned
`sqlite/configure.ac`/`Makefile.am` reliably works — the tool isn't
broken, something about the real build environment's recursive submake
is. Rather than keep chasing that mechanism, **`firmware/patches/0013-gt-ax6000-sqlite-pregenerated-autotools.patch`**
converts `sqlite` to the same already-proven pattern every other subdir
uses: ships `configure`/`aclocal.m4`/`missing`/`install-sh`/`Makefile.in`/
`compile`/`depcomp`/`config.guess`/`config.sub` as tracked files
(generated once via this project's own Docker image/toolchain) and drops
the `autoreconf -i -f` call from the recipe.

With all 13 patches applied, a full `APPLY_PATCHES=1 ./firmware/build.sh`
run from a pristine checkout **completed successfully — exit code 0**:
past `sqlite`, through the rest of userspace, through the full 4.19
kernel build, through `strongSwan`/`ncurses`/bootloader image generation,
producing a real signed FIT image and
`firmware/out/GT-AX6000_3004_388.9_0_nand_squashfs.pkgtb` (63,261,708
bytes, sha256 `8bcae611633e1fafcd2fdf9d3356ed92ffc383c7a170c749552865fcb4bb00b3`,
manifest alongside it with `apply_patches: true`). This is the first time
anything in this project has produced a real firmware image.

Two things surfaced along the way that are **not** believed to be real
problems, recorded so they aren't re-investigated from scratch later if
seen again: (1) a `udb/tmcfg_udb.h: No such file or directory` compiling
`bwdpi_source`'s TrendMicro sample code, which fires during the
pre-build `clean` pass and does not stop or reappear in the real build —
never confirmed *why* it's non-fatal, just confirmed twice that it is;
(2) dozens of `pjproject` sample-binary link failures (`Error 127
(ignored)`) — explicitly marked `(ignored)` by `make` itself, i.e. the
vendor's own Makefile already expects and tolerates these.

**Not done, and still genuinely blocked on physical hardware**: steps
3–12 and 14–16 exactly as stated above. The image now exists to flash —
see `docs/flashing-and-recovery.md` before doing that on a real router.

## Milestone 2 — First AXOS control service (`axosd`) + MCP

Everything that was `[s]` for M2 runtime is now hardware-verified `[x]`
below. Phase 7 (web UI) and Phase 9 (bootstrap) are also done — see the
hot-deploy phases list.

- [x] `axosd`/`axos-mcp`/`axosctl` cross-compile cleanly for aarch64 —
      static `linux/arm64` binaries (~18 MB total) built 2026-09-23
- [x] Runs on the router from **JFFS** (`/jffs/axos`; no USB present);
      starts at boot via `/jffs/scripts/services-start` with
      `jffs2_scripts=1`. Verified live: Core API on `127.0.0.1:9090`.
- [x] System info (model, firmware, uptime) — `/v1/info` → GT-AX6000 / 102.9
- [x] CPU / RAM / temperature — `/v1/resources` (broadcomThermalDrv ~68°C)
- [x] Interfaces (state, role, addresses, counters) — `/v1/interfaces`
- [x] Routing tables — `/v1/routes` (default via `ppp0`)
- [x] Connected clients (DHCP leases, ARP, Wi-Fi assoc list) — `/v1/clients`
- [x] Wi-Fi information (radios, channels, clients, RSSI) — `/v1/wifi`
      (`wl0`/`wl1`, SSID `Reece-Net`)
- [x] Router-native services, firewall rules, VPN tunnel status, full nvram
      dump — `/v1/services`, `/v1/firewall`, `/v1/vpn`, capture `nvram.json`
- [x] `system.shell_exec` (root shell, audited) — `uname -a` via
      `POST /v1/shell_exec`; audit `result=ok`
- [x] Config backup / restore — backup to `/jffs/axos/backups` (0600/0700);
      restore of `backup-20260923-202632` → `status=restored` (nvram commit
      timeout raised to 60s; bare `nvram commit` ~16s on this unit)
- [x] Audit logging of every mutating call — `/jffs/axos/logs/audit.jsonl`
      records shell_exec / backup / restore / rollback.arm / rollback.confirm
- [x] Rollback engine — `rollback.arm` (`txn-1`) + `rollback.confirm` OK
- [x] All of the above exposed over MCP and driven end-to-end — on-router
      `axos-mcp` stdio: `initialize`, `tools/list`, `tools/call system.info`
      → GT-AX6000 / 102.9
- [x] Audit log and backup files/dirs are owner-only (0600/0700)
- [x] Backup restore refuses a checksum-mismatched backup
- [x] SSH password auth disabled on the router, key-only access confirmed —
      `sshd_pass=0`, ed25519 pubkey in `sshd_authkeys`; password rejected
      with `Permission denied (publickey)`
- [x] `internal/backend/asuswrt`'s assumptions checked via live API +
      `axosctl capture -backend asuswrt` → `testdata/gt-ax6000/`
      (sanitized; secrets → `[REDACTED]`)

### Hot-deployable development platform (the "no reboot for normal dev" requirement)

Detailed status of the phases behind Milestone 2 — see
`docs/development.md` for the full write-up of each.

- [x] **Phase 1** — `RouterBackend` abstraction, `MockBackend`, CLI, daemon
      — verified on hardware via live Core API
- [x] **Phase 2** — Capture tooling + real `testdata/gt-ax6000/` fixture
      set captured from the router 2026-09-23
- [x] **Phase 3** — `AsuswrtBackend` live on GT-AX6000 (info/resources/
      interfaces/routes/clients/wifi/services/firewall/nvram)
- [x] **Phase 4** — Atomic releases (`internal/deploy`) — first
      `deploy-router.sh` promote on hardware created
      `/jffs/axos/releases/000001` + `current` symlink
- [x] **Phase 5** — Cross-compilation + deploy pipeline —
      `scripts/deploy-router.sh Reece@192.168.50.1` (tar fallback when
      rsync absent); promote + health OK
- [x] **Phase 6** — MCP as independent process — stdio `axos-mcp` on the
      router against live Core API (not a supervised sibling; by design —
      see `docs/development.md`)
- [x] **Phase 7** — Web UI + hot static asset deployment — `web/` embedded
      + `-ui-dir=/jffs/axos/www`; `GET /` and `/ui/*` verified on router;
      `axosctl deploy` stages `www/`
- [x] **Phase 8** — Network transaction layer + timed rollback watchdog —
      arm/confirm verified on hardware
- [x] **Phase 9** — AXOS bootstrap — `/jffs/axos/bin/axos-bootstrap` is the
      live boot hook; `firmware/patches/0002-axos-jffs-bootstrap.patch`
      applies cleanly (`git apply --check` on Merlin tree) for baking
      `/usr/sbin/axos-bootstrap` into the next firmware image (not yet
      re-flashed; JFFS hook covers runtime today)

## Milestone 3 — VPN, routing, firewall, DNS, QoS

- [x] WireGuard profile management — `GET /v1/vpn/profiles`,
      `POST /v1/vpn/wireguard/import` (slot `wgc5` import verified; secrets
      at `/jffs/axos/secrets/vpn/`), `POST /v1/vpn/{name}/up|down` wired to
      Merlin `restart_wgc` / `stop_wgc`
- [s] Cloudflare WARP — no separate registration client; import a WARP
      WireGuard config via `vpn.wireguard.import` after obtaining keys
      externally (same path as any WG profile)
- [x] OpenVPN client management — profiles listed from `vpn_clientN_*`;
      up/down via `service start_vpnclientN` / `stop_vpnclientN`
- [x] Policy routing engine — VPN Director `vpndirector_rulelist` via
      `GET|POST /v1/policy` + `DELETE /v1/policy/{id}` (round-trip verified)
- [x] Bypass rules (device stays on WAN) — policy route with
      `interface=wan` (same API)
- [s] VPN endpoint latency benchmarking + automatic endpoint selection —
      Merlin AXOS tab **VPN endpoint ping** ranks hosts via
      `POST /v1/diag/ping`; no auto-apply to a profile yet
- [x] DNS configuration — `GET|POST /v1/dns` (WAN upstreams + DoT flags
      from nvram; live read verified)
- [x] DHCP reservations — `GET|POST /v1/dhcp/reservations` + DELETE by MAC
      (round-trip verified)
- [x] Firewall rule management — `POST /v1/firewall/apply|delete` audited;
      custom-chain round-trip verified under rollback
- [x] QoS inspection and control — `GET|POST /v1/qos` (`qos_enable`
      toggle verified)
- [x] Diagnostics — ping / DNS lookup / port check verified on-router;
      traceroute wired (`traceroute -m N`)
- [s] Performance testing — `POST /v1/perf/iperf3` implemented; needs a
      reachable iperf3 peer for a full hardware pass; WAN Ookla / loaded
      latency loops not built yet
- [x] Dedicated VPN secrets store — `/jffs/axos/secrets/vpn/` (0700/0600);
      `wgc5.key` written on import

## Milestone 4 — Optimisation loops

Every optimiser follows: OBSERVE → BASELINE → CHANGE ONE THING → TEST → COMPARE →
KEEP OR REVERT → CONTINUE. No unbenchmarked "tuning".

- [s] Ethernet optimiser — `internal/optimiser` Observe/Baseline/Propose/
      Apply/Measure/Decide skeleton + unit test (not run against hardware)
- [s] Wi-Fi optimiser (channel scan, utilisation, candidate configs, A/B compare)
      — interface skeleton + unit test; no channel scan / apply yet
- [s] VPN optimiser (endpoint selection, MTU, fast paths) — skeleton + unit
      test; Merlin UI ranks endpoints by ping (manual)
- [s] Latency optimiser (loaded-latency driven) — skeleton + unit test
- [s] AXOS Performance Mode (orchestrates the above with rollback arming) —
      `PerformanceMode` skeleton wires subsystem optimisers; no live loop

## Later / continuous

- [x] AXOS web UI (Phase 7) — thin Core API client at `/` (same backend as
      MCP/CLI).
- [x] Custom AXOS sections inside the stock ASUS httpd UI — **Administration →
      AXOS** tab (after Firmware Upgrade) via JFFS bind-mount
      (`docs/merlin-ui.md`). Control panel (not JSON dumps): system/resources/
      Wi-Fi/clients, DNS/QoS apply, diagnostics, VPN up/down + endpoint ping,
      VPN Director policy, firewall preview, DHCP reservations, backups.
      Session-gated Merlin page + LAN API token (`X-Axos-UI-Token`).
      Iterate with `deploy-router.sh` / copy to `merlin-ui/`, no firmware rebuild.
- [ ] Package/module system for optional functionality
- [ ] Historical metrics on USB storage (never internal flash)
- [ ] Backup encryption at rest, once a key-management approach is decided
      (`docs/security.md` "Secrets at rest")
- [ ] Firmware image signing / verified `system.update` before flashing
      anything AI-selected (`docs/security.md` "Firmware & build integrity")
- [s] Firmware-integrated bootstrap (`0002-axos-jffs-bootstrap.patch`) —
      applies; awaiting next full image flash to land `/usr/sbin/axos-bootstrap`
      in squashfs (JFFS hook already live)
- [ ] `OpenWrtBackend` / other router support via the `RouterBackend` interface
