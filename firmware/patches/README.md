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

## Later patches (Milestone 2+)

- Install hook for `axosd` (a `services-start` addition, or an `/etc/init.d`
  style entry depending on what this Merlin release supports) — added once
  `axosd` itself is built and tested via USB sideload first (no firmware
  change needed for that).
- Any web UI additions for the AXOS section, once the design is settled.

Keep each patch focused on one logical change and documented with a one-line
summary at the top of the patch file (a comment above the `diff --git` line is
fine and ignored by `git apply`).
