# AXOS firmware patches

Every AXOS modification to the Merlin source tree lives here as an ordered
`git apply`-able patch, applied by `build.sh` when `APPLY_PATCHES=1`. We never
edit `firmware/src/asuswrt-merlin.ng` and commit the tree itself — the patch
files are the record of our diff. `firmware/src/asuswrt-merlin.ng` and
`firmware/src/am-toolchains` are git submodules (see `.gitmodules` and
`docs/build-environment.md`) — this repo tracks exactly which upstream
commit they're pinned to, but not their (multi-GB) content.

## Why patches instead of a vendored fork (for now)

Milestone 1 requires proving stock builds/flashes work before any modification.
Keeping the diff as small, reviewable patches:

- keeps this repo lightweight (no multi-GB vendor tree to commit),
- makes every AXOS change to Merlin auditable in a code review,
- makes it trivial to rebase onto a newer Merlin release,
- matches how the Merlin project itself expects downstream forks to work
  during early bring-up.

If/when the AXOS diff grows large enough that patch maintenance becomes painful,
switch to a real hosted fork of `RMerl/asuswrt-merlin.ng` with an `axos` branch,
and update `firmware/setup-sources.sh` to point `MERLIN_REPO`/`MERLIN_REF` at it.
The patch files here become the seed commits for that branch.

## Creating a patch

```sh
cd firmware/src/asuswrt-merlin.ng
# make your change
git diff > ../../patches/0001-short-description.patch
```

Number patches so `build.sh` applies them in a defined order (it currently
globs `*.patch` — keep filenames sortable: `0001-`, `0002-`, ...).

## Milestone 1, step 13: "one harmless, visible source-code modification"

`0001-axos-login-title-marker.patch` is ready to use — not a suggestion, an
actual patch: it appends " (AXOS)" to the browser tab title on the web UI
login page (`release/src/router/www/Main_Login.asp`'s `<title>` tag). Small,
cosmetic, and nowhere near networking/auth code, so a mistake here can't
break connectivity.

**Verified against real source**, not written speculatively: generated from
and `git apply --check`-confirmed against an actual checkout of the pinned
tag (`3004.388.9`, the exact ref `firmware/setup-sources.sh` fetches by
default) — see the commit that added this patch for the session that did
that verification. It has **not** been verified past that point: nobody has
built it into an image or flashed it to a router yet, since that requires
the Docker toolchain build (`firmware/build.sh`) and physical hardware,
neither available in the environment that generated the patch.

To use it: `APPLY_PATCHES=1 ./build.sh` applies everything under this
directory before building (see `firmware/build.sh`).

Verification once flashed: load the web UI login screen and confirm the
browser tab reads "ASUS Login (AXOS)". Record the result in
`docs/ROADMAP.md` (step 16) and `docs/flashing-and-recovery.md`'s flash log.

## `0002`: a required build fix, not an AXOS feature

`0002-gt-ax6000-bcm-util-bcmnet-include.patch` is a different kind of
patch from `0001`: it's not an AXOS change, it's a fix for a real bug in
this Merlin release's own `router-sysdep.gt-ax6000/bcm_util/Makefile` —
`bcm_ethswutils.c` needs `bcmnet.h`, but the Makefile only adds that
include path for the `BCM4906_504` chip family, not GT-AX6000's actual
chip (`4912`), even though the rest of this codebase already treats both
chips the same way everywhere else it matters. Without it, `make
gt-ax6000` cannot complete at all — confirmed via a real build attempt
that failed with `bcmnet.h: No such file or directory`, then fixed and
`git apply --check`-verified against the pinned tag.

This means **Milestone 1's "unmodified upstream" build isn't actually
buildable as-is for this model** — `APPLY_PATCHES=1 ./build.sh` is
required to get a working image, not optional the way `0001`'s cosmetic
marker is. Not yet verified past `git apply --check` (no environment that
generated this patch could complete a full build — see
`docs/build-environment.md` and `docs/ROADMAP.md`).

## `0003`-`0012`: more of the same

Ten more real, build-attempt-confirmed upstream bugs, found and fixed the
same way as `0002` — see each patch's own header for its specific root
cause. Required together with `0002` for `make gt-ax6000` to get past
userspace linking.

## `0013`: sqlite's missing autotools output

`0013-gt-ax6000-sqlite-pregenerated-autotools.patch` fixes a different
*kind* of bug from the others: `sqlite/` is the only autotools subdir in
this tree that tries to run `autoreconf -i -f` at build time instead of
shipping pre-generated output (`missing`, `aclocal.m4`, `configure`,
etc.) the way every other one does (`flac`, `libogg`, `strace-4.5.20`,
...). Confirmed via two real build attempts that the recursive
`make -C sqlite all` loses those auxiliary files moments after
`autoreconf` successfully creates them, failing with `./missing: No such
file or directory`. Rather than keep chasing why, this patch brings
`sqlite` in line with the rest of the tree: ships the generated output as
tracked files (produced once via this project's own Docker
image/toolchain) and drops the `autoreconf -i -f` call.

**This is the patch that got a build all the way through** — with
`0001`-`0013` all applied, `APPLY_PATCHES=1 ./build.sh` completed with
exit code 0 and produced a real, signed
`firmware/out/GT-AX6000_3004_388.9_0_nand_squashfs.pkgtb`. See
`docs/ROADMAP.md`'s Milestone 1 section for the full story and the
image's hash/manifest.

**Follow-up, found during real incremental-rebuild testing**: dropping
the explicit `autoreconf -i -f` call isn't sufficient on its own for
idempotency. Without `--disable-maintainer-mode` on the `$(CONFIGURE)`
call, sqlite's own *generated* Makefile still carries automake's normal
auto-remake rules and can regenerate `Makefile.in` on a plain `make`
invocation, with no explicit `autoreconf` call needed — confirmed via a
real build: `sqlite/Makefile.in`'s `am__DIST_COMMON` gained an `INSTALL`
entry it didn't have right after the patch was first applied, purely
from running a normal build afterward. That permanently drifted the
shipped file away from what `build.sh`'s patch-apply idempotency check
(added alongside `0014`) expects, breaking `resume`/repeat `build` calls
for this patch specifically. Fixed by adding `--disable-maintainer-mode`
to the same `$(CONFIGURE)` call `0013` already modifies.

**Second follow-up**: `--disable-maintainer-mode` turned out to be a
no-op for sqlite specifically — confirmed via `grep AM_MAINTAINER_MODE
sqlite/configure.ac`, which matches nothing. Without that macro,
automake's default `Makefile.in: Makefile.am configure.ac ...`
auto-remake rule is unconditionally active regardless of the configure
flag; it only *fires* on an actual mtime race between the shipped
scaffolding and the derived `Makefile`, but a real one was confirmed
(`git apply`'s write order isn't guaranteed relative to a fresh
submodule checkout's, and this repo's own accidental whole-tree
`git checkout --` during testing made it worse). Fixed for real by
pinning explicit mtimes in the `sqlite/stamp-h1` recipe itself —
`touch -d 2020-01-01` on the scaffolding before configuring, `touch -d
2020-01-02` on the resulting `Makefile`/`config.status` after — so
Make's own staleness check can never see the scaffolding as newer than
the Makefile it produced, independent of wall-clock timing or automake
version behavior. Verified directly: with the shipped `./configure`
run and these touches applied, `make -n Makefile.in` reports "up to
date" and a full `make -n` goes straight to real compiler invocations
with zero autoreconf/automake output.

## `0014`: incremental builds, not a compile-bug fix

`0014-incremental-router-sysdep-sync.patch` (originally drafted as `0003`,
renumbered to `0014` when this patch set and the incremental-build work
merged) is a different kind of patch again: `make gt-ax6000` already
completes without it (see `0013` above). It fixes `release/src-rt/Makefile`
unconditionally `rm -fr`-ing and recopying `router-sysdep/` on *every*
invocation, which destroyed all previously-compiled output in that ~90
component subtree even when nothing had changed — the dominant reason a
retried build looked like it started over. See `docs/incremental-builds.md`
for the full root-cause trace and how to use the resulting `axos-build.sh`
controller.

## `0016`: lighttpd's own genuine upstream configure.ac bug

`0016-gt-ax6000-lighttpd-libunwind-placeholder.patch` — `src/Makefile.am`
references `$(LIBUNWIND_CFLAGS)`/`$(LIBUNWIND_LIBS)` unconditionally, but
`configure.ac` only calls `PKG_CHECK_MODULES(LIBUNWIND, libunwind)` —
what actually registers those as `AC_SUBST` substitutions — inside an
`if test "$WITH_LIBUNWIND" != "no"` block, and `--with-libunwind` is
never passed (defaults to `no`) by `preconfigure-script-hnd`. Automake
still emits `@LIBUNWIND_CFLAGS@`/`@LIBUNWIND_LIBS@` placeholders into the
generated Makefile expecting configure to fill them in, but since
configure never registered them, the literal placeholder text survives
into the real Makefile — confirmed via a real build: gcc tried to open a
file literally named `LIBUNWIND_CFLAGS@`. This is a genuine, always-
reproducible upstream `configure.ac` bug (nothing to do with any
`autoreconf`/mtime fragility this project already worked around for
`0013`/`0015`) — fixed by stripping the leftover placeholders from the
generated `src/Makefile` with `sed` right after `preconfigure-script-hnd`
runs configure, rather than touching `configure.ac` itself and re-opening
the same regeneration-fragility questions `0013`/`0015` already had to
work around.

## `0017`: same prebuild/ gap pattern as UUPLUGIN/TPVPN/etc, but a path bug

`0017-gt-ax6000-lighttpd-prebuild-path.patch` — lighttpd's `copy-prebuild:`
target (its substitute for source files this vendor release doesn't ship,
e.g. `mod_smbdav.c`) copies from a flat `prebuild/mod_smbdav.so.l` path.
That path doesn't exist for any model — the real prebuilt binaries live
under a per-model subdirectory (`prebuild/GT-AX6000/`, alongside
`prebuild/RT-AX88U/`, `prebuild/GT-AX11000/`, etc.) with a `.so`
extension, not `.so.l`. Confirmed directly: `ls prebuild/GT-AX6000/`
shows all six needed files (`mod_smbdav.so`, `mod_aidisk_access.so`,
`mod_aicloud_sharelink.so`, `mod_aicloud_auth.so`, `mod_aicloud_invite.so`,
`mod_query_field_json.so`) sitting right there — the Makefile just never
looks in the right place. Same "gap" shape as the `RTCONFIG_UUPLUGIN`/
`RTCONFIG_TPVPN`/etc. fixes earlier in this project, except those were
missing files and this is a wrong path to files that do exist. Fixed by
pointing `copy-prebuild:` at `prebuild/GT-AX6000/<name>.so` directly
(hardcoded to this model, matching how the rest of this patch set is
scoped — `Makefile.in` doesn't use a `$(BUILD_NAME)`-style variable here
to make it generic).

## Later patches (Milestone 2+)

- Install hook for `axosd` (a `services-start` addition, or an `/etc/init.d`
  style entry depending on what this Merlin release supports) — added once
  `axosd` itself is built and tested via USB sideload first (no firmware
  change needed for that).
- Any web UI additions for the AXOS section, once the design is settled.

## `0015`: the same maintainer-mode gap, everywhere else it exists

`0015-gt-ax6000-disable-maintainer-mode.patch` extends `0013`'s fix
(`--disable-maintainer-mode`) to every other component in this tree that
calls `autoreconf`/`autogen.sh` at build time and hadn't already been
given it: `wget`, `libogg`, `nano`, `haveged`, `tor`, `openvpn`,
`lldpd-0.9.8`, `lldpd-1.0.11`, `onig-6.9.9`, and `lighttpd-1.4.39` (via
its `preconfigure-script-hnd`). Found the same way `0013` was — a real
build, not a guess: after `0013` shipped, a real incremental-rebuild test
hit `libogg`'s own generated Makefile trying to self-regenerate
`Makefile.in` via its built-in maintainer-mode auto-remake rule
(`missing automake-1.16 --foreign` → a hard version-mismatch error
against the checked-in `aclocal.m4`, generated with automake 1.15). Since
every one of these components shares the identical mechanism (confirmed:
none of them passed `--disable-maintainer-mode` before this patch,
`grep -c disable-maintainer-mode` was 0 across the whole Makefile),
fixed all of them together rather than rediscovering each one serially
across further build attempts. Verified: applies cleanly through the
full patch sequence against the real pinned checkout, and a reverse-check
confirms it as idempotent.

Keep each patch focused on one logical change and documented with a one-line
summary at the top of the patch file (a comment above the `diff --git` line is
fine and ignored by `git apply`).
