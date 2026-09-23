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
model's stock source doesn't build without patches applied at all; `0002`
fixes a separate real compile bug found earlier).

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
- **Bounded parallelism**: `BUILD_JOBS` (default `min(nproc, 4)`) passed as
  `make -j`. Known, confirmed limitation: the kernel build phase hardcodes
  its own `make -j 9` internally (`release/src-rt/Makefile` ~line
  1226-1227) — that one phase's parallelism isn't governed by `BUILD_JOBS`.
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

- `firmware/patches/0003-*.patch`: `git apply --check` + a full
  apply-then-revert cycle against the actual pinned checkout, confirmed the
  fix lands correctly at both occurrences.
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

**Not verified — needs a real build machine** (this sandbox cannot run
Docker; see `docs/ROADMAP.md`):

- That `0003`'s fix actually produces a materially faster second build in
  practice, and by how much.
- Real ccache hit rates (`ccache -s` before/after is printed by
  `entrypoint.sh`; nothing here fabricates a number).
- Every acceptance scenario a full incremental-build reliability review
  should cover (unchanged rebuild does no unnecessary compilation,
  interrupt-and-resume preserves completed work, a userspace-only change
  doesn't rebuild the kernel, a packaging-only retry reuses compiled
  components, etc.) — these need to actually run.

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
