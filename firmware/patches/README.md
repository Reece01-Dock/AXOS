# AXOS firmware patches

Every AXOS modification to the Merlin source tree lives here as an ordered
`git apply`-able patch, applied by `build.sh` when `APPLY_PATCHES=1`. We never
edit `firmware/src/asuswrt-merlin.ng` and commit the tree itself — the patch
files are the record of our diff, and `firmware/src/` stays gitignored.

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

The recommended first patch — small, safe, and easy to verify on-device without
touching networking, so a mistake here can't break connectivity:

- Add an AXOS marker to the web UI login page or the `About` panel (e.g. append
  " (AXOS)" to the firmware version string shown in
  `router_webui`/`www/...`), **or**
- Append a line to `/etc/motd` (or equivalent build-time template) so it shows
  on SSH login.

Verification: after flashing, either check the web UI login screen / About page
for the marker, or SSH in and confirm the MOTD/banner text. Record the result
in `docs/ROADMAP.md` (step 16) and `docs/flashing-and-recovery.md`'s flash log.

## Later patches (Milestone 2+)

- Install hook for `axosd` (a `services-start` addition, or an `/etc/init.d`
  style entry depending on what this Merlin release supports) — added once
  `axosd` itself is built and tested via USB sideload first (no firmware
  change needed for that).
- Any web UI additions for the AXOS section, once the design is settled.

Keep each patch focused on one logical change and documented with a one-line
summary at the top of the patch file (a comment above the `diff --git` line is
fine and ignored by `git apply`).
