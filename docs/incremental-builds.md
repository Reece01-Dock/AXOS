# Incremental firmware builds

Why retries used to look like they started over, what was fixed, how to use
the result, and what's still unverified because this session's sandbox
cannot run Docker (see `docs/ROADMAP.md` — confirmed, not assumed: Docker
Hub's CDN is network-blocked here). Every claim below is either something
verified directly against the real pinned source (`firmware/src/asuswrt-merlin.ng`,
tag `3004.388.9`), or explicitly marked as needing your verification on a
real build machine.

## The root cause

`release/src-rt/Makefile` (symlinked in as `release/src-rt-5.04axhnd.675x/Makefile`)
has two device-name pattern rules matching `gt-ax6000`/`GT-AX6000`. Both did,
on **every single invocation** of `make gt-ax6000`, success or retry, changed
or unchanged:

```make
rm -fr router-sysdep
cp -ar router-sysdep.$(lowercase_B)/ router-sysdep
```

`router-sysdep.gt-ax6000/` is 817 files / 32MB spanning ~90 components
(`bcm_util`, `wlan`, `wlconf`, `wlcsm`, `ethctl`, `fcctl`, and more). Critically,
`router-sysdep` (no suffix) isn't just a source copy — it's also where these
components get **compiled**: object/library outputs land inside
`router-sysdep/<component>/` alongside the copied source. Deleting the whole
tree before every build destroyed every previously-compiled output in it,
forcing a full rebuild of this entire subtree regardless of whether anything
actually changed. Confirmed directly against a real failing build log:
`bcm_ethswutils.o` was compiling inside `router-sysdep/bcm_util/`, which only
exists via this copy step.

**Fix**: `firmware/patches/0014-incremental-router-sysdep-sync.patch` replaces
the wipe+copy with `rsync -a --delete`, which only touches files that
actually changed (preserving existing `.o`/`.so` outputs for unchanged
sources, correctly seen as up to date by Make's own mtime-based dependency
tracking) while `--delete` still removes anything no longer present in the
source. This is the single biggest fix here — everything else is smaller.
**Requires `APPLY_PATCHES=1`** (see `firmware/patches/README.md` — this
model's stock source doesn't build without patches applied at all).
`0002`-`0013` fix thirteen separate real compile bugs found while driving
this build to completion (missing symbol guards, a getpid/fromfile
multiple-definition clash, a real parallel-build race, sqlite's autotools
scaffolding, and more — see `firmware/patches/README.md` for the full
per-patch trace); `0014` is this rsync fix, renumbered from `0003` when
that patch set and this incremental-build work were merged together.

Other suspicious `rm -rf` calls in the same Makefile were checked and ruled
out: Realtek-only (irrelevant — this is a Broadcom HND build), inside the
`clean` target itself (not the normal build path), or a cheap symlink
re-point (not real content).

## Everything else that was added

- **ccache**, wired around the actual cross-compilers. `CROSS_COMPILE` in
  this SDK is always an absolute path
  (`$(TOOLCHAIN)/bin/$(TOOLCHAIN_PREFIX)-`), never resolved via `PATH`, so
  the usual "put a same-named symlink to ccache earlier in PATH" trick
  doesn't apply. `firmware/docker/entrypoint.sh` instead builds a mirror of
  each toolchain directory at container start (symlinks for everything,
  wrapper scripts calling `ccache <real compiler>` for the `*-gcc`/`*-g++`/
  `*-cc` binaries specifically) and points `/opt/toolchains` at the mirror.
  Verified the *construction logic* directly against the real toolchain
  checkout here (96 wrapper scripts created for actual compiler drivers,
  1887 correct plain symlinks for binutils/gcc-ar/gcc-nm/etc. and
  everything else) — **not verified**: real cache hit rates, since that
  needs an actual build run. `ccache -s` prints before and after the build
  in the log; compare them.
- **Persistent cache**: `firmware/.ccache/` on the host, bind-mounted into
  the container, so hits survive across `docker run --rm` invocations
  (which would otherwise throw the container's own filesystem away every
  time). `CCACHE_COMPILERCHECK=content` for safety against a moved
  toolchain pin not being caught by an mtime-only check.
- **Bounded parallelism**: `BUILD_JOBS` (default `1`, strictly serial) passed
  as `make -j`. Real builds on real hardware hit two vendor parallel-build
  races under `-j>1`: `release/src/router/Makefile`'s clean-build vs.
  `fsbuild/`'s `$(obj-y)` (fixed by `firmware/patches/0008-*.patch`'s
  `.NOTPARALLEL`) and `router-sysdep/wlan/scripts` needing `nvramUpdate`
  from the sibling `nvram/` target before it's built ("No rule to make
  target `nvramUpdate`", not yet patched). This vendor tree was evidently
  never validated under `-j>1` in general, so the default is serial —
  override `BUILD_JOBS` only once you've dealt with both races or are
  prepared to hit the second. Known, confirmed limitation regardless: the
  kernel build phase hardcodes its own `make -j 9` internally
  (`release/src-rt/Makefile` ~line 1226-1227) — that one phase's
  parallelism isn't governed by `BUILD_JOBS` either way.
- **`firmware/axos-build.sh`**: a controller adding `build`/`resume`/
  `status`/`rebuild <component>`/`clean <component>`/`clean-all`/
  `explain-rebuild` on top of `build.sh` (see below).

### What was deliberately *not* changed

- **Config/header regeneration**: `chip_profile.tmp` was checked and is
  already correctly skip-if-exists (`ifeq (,$(wildcard ./chip_profile.tmp))`),
  not a bug. A full audit of every config-generation script in this ~300k+
  line vendor tree for the same pattern was not done — the router-sysdep fix
  above is by far the dominant win found; treat "other regeneration exists
  somewhere" as a real possibility, not ruled out.
- **`fs.install` staging rootfs**: whether the vendor build already handles
  stale-file cleanup here correctly wasn't fully traced (would need running
  the actual build to observe). Rather than guess and risk breaking an
  assumption the vendor build depends on, `axos-build.sh clean-all`
  explicitly wipes it (safe — it's copied-from-already-built-binaries
  staging, not compiled output) instead of doing so unconditionally on
  every build. If you remove/disable a package, run `clean-all` (or at
  least the fs.install portion) before your next build to be sure nothing
  stale survives into the image, until this is verified more precisely.

## Using it

```sh
cd firmware
./setup-sources.sh                          # once
APPLY_PATCHES=1 ./axos-build.sh build        # normal incremental build
APPLY_PATCHES=1 ./axos-build.sh resume       # retry after a failure/interrupt
./axos-build.sh status                       # what happened last time
APPLY_PATCHES=1 ./axos-build.sh rebuild bcm_util   # invalidate + rebuild one component
./axos-build.sh clean bcm_util                # invalidate only, no rebuild
./axos-build.sh clean-all                     # wipe all build state (not source)
./axos-build.sh explain-rebuild               # why would a build do work right now
```

`APPLY_PATCHES=1` is required for a working build on this model (see
above) — pass it on every `build`/`resume`/`rebuild` call, or `export` it
for the session.

Component names for `rebuild`/`clean` are directory names under
`router-sysdep.gt-ax6000/` (e.g. `bcm_util`, `wlan`, `ethctl`) or
`release/src/router/` (e.g. `rc`, `shared`, `httpd`) — whichever the
component actually lives under. `rebuild`/`clean` were tested directly
against the real checkout (see "What was verified" below) and correctly
refuse to touch anything under a `prebuild/` subdirectory (vendor-shipped
binary blobs, never this build's own output) or a same-named-but-different
legacy component (`release/src/router/bcm_util` is a real, separate,
much smaller non-HND component, unrelated to the GT-AX6000 one under
`router-sysdep.gt-ax6000/bcm_util`).

State lives in `firmware/.build-state/` (gitignored): `last-run.json`
(status, timestamps, log path, and the identity — source/toolchain commits,
dirty-or-not, patches-or-not — the run happened under), per-run logs under
`logs/`, and a `flock`-based lock preventing two builds from corrupting the
same source tree concurrently.

## What was verified, and how

Real, direct verification performed in this session (no Docker needed for
these):

- `firmware/patches/0014-*.patch`: `git apply --check` + a full
  apply-then-revert cycle against the actual pinned checkout, confirmed the
  fix lands correctly at both occurrences.
- `firmware/patches/0002-0013`, `firmware/build.sh`'s pre-ccache shape, and
  the full 8-flag RTCONFIG fix: driven to a **real, complete, successful
  build** on real hardware — `GT-AX6000_3004_388.9_0_nand_squashfs.pkgtb`
  (63,261,708 bytes, sha256 `8bcae611633e1fafcd2fdf9d3356ed92ffc383c7a170c749552865fcb4bb00b3`).
  This is strong evidence the patch set and RTCONFIG-flag fix are correct.
  It predates this incremental-build work being merged in, though: `ccache`,
  the `entrypoint.sh` extraction, and `axos-build.sh` didn't exist yet at
  the time of that build, so the *merged* pipeline (this doc, "What to run
  to gather that evidence" below) still needs its own end-to-end run to
  confirm nothing in the merge broke that result.
- ccache toolchain-mirror construction logic: run directly against the real
  `firmware/src/am-toolchains` checkout (not a mockup) — confirmed correct
  wrapper/symlink classification across all 9 toolchain variants.
- `axos-build.sh clean`/`rebuild`: run directly against the real pinned
  Merlin checkout. **This caught a real bug before it could do damage**: the
  first version's `find -delete` had no exclusion for `prebuild/`
  directories and deleted 608 tracked vendor-blob files from
  `release/src/router/rc/prebuild/` on the very first test — caught
  immediately via `git status`, restored via `git checkout --` (these are
  tracked files in the submodule's own git history, so nothing was
  permanently lost), and fixed before being used again. A second bug
  (`release/src/router/bcm_util` being silently used as a stand-in for the
  unrelated `router-sysdep.gt-ax6000/bcm_util`) was found and fixed the
  same way. Both fixes were then re-tested and confirmed correct, including
  the positive case (a real non-prebuild `.o` file is actually removed).
- All scripts pass `bash -n`.

## Real evidence from an actual build machine

The merged pipeline (all patches through `0017`, ccache, `entrypoint.sh`,
`axos-build.sh`) completed a full real build end to end for the first
time on 2026-09-23, via `APPLY_PATCHES=1 ./axos-build.sh resume`:

- **Exit code 0**, real signed image:
  `GT-AX6000_3004_388.9_0_nand_squashfs.pkgtb`,
  sha256 `dcef62e44be632ea613db0fe32347e0ab6b17a5767363d863e5562047776443f`.
- **Wall time: 8m40s** (`real 8m40.899s`) — this was a `resume` on top of
  substantial already-compiled state from earlier failed attempts
  (router-sysdep, most of userspace, all of the kernel build already
  done), not a from-clean-source baseline, but it's real, direct
  evidence that a resume after fixing a real failure reuses previously
  completed work rather than rebuilding it — the whole point of `0014`'s
  fix. A from-clean-source baseline timing is still open (see below).
- **Real ccache stats**, printed by `entrypoint.sh`, not fabricated:
  `cache hit rate 55.00%` (3936 direct hits + 148 preprocessed hits out
  of 7425 total lookups), 8236 files in cache, 123.9 MB cache size. This
  is the first real confirmation the toolchain-mirror wrapper construction
  (verified earlier only as *construction logic*, never against a real
  compile) actually produces cache hits during real cross-compilation.

This also, incidentally, validated every one of the 17 patches applying
and reapplying correctly together in sequence across many real
build/resume cycles on real hardware — see `firmware/patches/README.md`
for the full trace of `0013` through `0017`, several of which were only
found and fixed *because* this incremental-build work forced repeated
real rebuild/resume cycles that a single one-shot build would never have
exercised (sqlite's and libogg's automake auto-remake mtime fragility,
lighttpd's `LIBUNWIND_CFLAGS` and `copy-prebuild` path bugs).

**Still open — needs a real build machine**:

- A from-clean-source baseline timing (`clean-all` then a full build),
  to compare against the 8m40s resume above and quantify the
  router-sysdep fix's actual speedup, not just its qualitative effect.
- Controlled interrupt-and-resume (Ctrl-C mid-build, then `resume`).
- A single userspace source-file touch + `rebuild <component>`,
  confirming only that component recompiles.
- Web-only change vs. no kernel recompilation.
- Packaging-only failure/retry reusing compiled components.
- Package removal leaving no stale files in the assembled image.

## What to run to gather that evidence

On a real build machine, from a clean state:

```sh
cd firmware
APPLY_PATCHES=1 time ./axos-build.sh build          # baseline — record wall time
APPLY_PATCHES=1 time ./axos-build.sh build           # immediate rebuild — should be much faster
                                                       # if router-sysdep/*.o survived correctly
# interrupt a build (Ctrl-C) partway through, then:
APPLY_PATCHES=1 ./axos-build.sh resume
# touch one .c file in a userspace component, rebuild, confirm only that
# component (and whatever depends on it) recompiles:
touch src/asuswrt-merlin.ng/release/src-rt-5.04axhnd.675x/router-sysdep.gt-ax6000/bcm_util/bcm_ethswutils.c
APPLY_PATCHES=1 ./axos-build.sh rebuild bcm_util
```

Paste the output back and it can be diagnosed the same way every build
error in this project has been so far — against the real source, not
guesses.
