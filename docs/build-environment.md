# Build environment (reproducible)

Goal: anyone (human or AI) can produce a byte-identical-in-practice GT-AX6000
firmware image from a clean machine with two commands. We use Docker to pin the
distro and package set the Merlin HND toolchain expects.

> **Authoritative reference**: the Asuswrt-Merlin build documentation in the
> upstream wiki (`https://github.com/RMerl/asuswrt-merlin.ng/wiki`). If anything
> below conflicts with the wiki for the firmware branch we're on, the wiki wins —
> update this doc and the scripts.

## Requirements

- x86_64 Linux host (or VM) with Docker
- **~20 GB free disk for source alone, ~60 GB recommended overall** (source
  tree + toolchains + build output). Measured, not estimated: a shallow
  (`--depth 1`) checkout of `asuswrt-merlin.ng` at the pinned commit is
  **~11 GB**, and `am-toolchains` at its pinned commit is **~3.1 GB** — so
  source alone is ~14 GB. Add headroom for Docker image layers and the
  actual build's object files/output (a full embedded Linux + userspace
  build historically runs several more GB) and ~60 GB total is the safe
  target. A 30 GB disk allowance was tight but source fetch alone fit fine
  in one; the build step itself has not been attempted end-to-end due to
  that remaining headroom being uncertain — see `docs/ROADMAP.md` Milestone 1.
- Decent CPU; a full HND build takes on the order of an hour on 8 cores

## Layout

Everything lives under `firmware/`:

```
firmware/
├── docker/Dockerfile        # pinned Ubuntu build image
├── setup-sources.sh         # fetches the pinned submodules below
├── build.sh                 # runs the GT-AX6000 build inside the container
├── patches/                 # our source modifications, as git patches
└── src/
    ├── go.mod               # module boundary only — see below, never built
    ├── asuswrt-merlin.ng/   # git submodule — pinned to an exact upstream commit
    └── am-toolchains/       # git submodule — pinned to an exact upstream commit
```

`firmware/src/go.mod` (and the sibling `firmware/src-stock/go.mod` for the
local stock Merlin clone) exist only so `go build/vet/test ./...` run from
the AXOS repo root doesn't sweep in stray Go source that ships inside the
vendored trees (e.g. `asuswrt-merlin.ng`'s `wireguard-tools/contrib/external-tests`,
which has unmet third-party dependencies and isn't meant to build as part of
AXOS). They mark those directories as separate Go modules with no dependents —
Go's `./...` pattern skips subtrees below a nested `go.mod` automatically.
Don't remove them without re-checking `go build ./...` from the repo root.

`firmware/src/asuswrt-merlin.ng` and `firmware/src/am-toolchains` are **git
submodules** (see `.gitmodules` at the repo root), each pinned to one exact
upstream commit — verified to actually exist and, for Merlin, to contain the
expected GT-AX6000 build profile (`docs/ROADMAP.md` Milestone 1 has the
specifics). This means the pin is **recorded in this repo's own git
history** — visible and clickable on GitHub (a submodule link jumps straight
to that exact commit on the upstream repo), diffable with normal `git log`,
and requires no separate "trust this shell variable" step. Nothing from
either upstream tree is vendored/copied into this repo — only the commit
reference is, which is why cloning `axos` itself stays lightweight even
though the pinned sources are ~14 GB once fetched.

We keep this as submodule references rather than a full hosted fork for now:
the diff we actually maintain against Merlin (`firmware/patches/`) is small.
If/when it grows large enough that patch maintenance becomes painful, switch
to a proper hosted fork with an `axos` branch and repoint `.gitmodules` at it.

## Usage

```sh
cd firmware
./setup-sources.sh            # fetch the pinned submodules (git submodule update --init)
./build.sh                    # build docker image, run 'make gt-ax6000' inside it
```

Or, from the repo root, the standard git way works too:
`git submodule update --init --depth 1 -- firmware/src/asuswrt-merlin.ng firmware/src/am-toolchains`.

Output image lands in `firmware/out/` (a `GT-AX6000_*_nand_squashfs.pkgtb`
file — confirmed against upstream's own build automation, see
`docs/ROADMAP.md`).

## Version pinning

The pin lives in this repo's git history as the submodules' gitlink commits,
not in shell script variables. To move it:

```sh
cd firmware
MERLIN_REF=<new-tag> ./setup-sources.sh --repin
# review: git -C src/asuswrt-merlin.ng log -1 ; git diff --cached
git add firmware/src/asuswrt-merlin.ng
git commit -m "firmware: repin asuswrt-merlin.ng to <new-tag>"
```

Pin to a **released tag**, not a branch — `master` may contain
unreleased/broken state. `am-toolchains` has no release tags upstream; it's
pinned to a specific commit on `master` instead (same `--repin` mechanism,
via `TOOLCHAINS_REF=<commit>`).

The pin isn't real for anyone else until that gitlink-update commit is
pushed — a local `--repin` alone only changes your own checkout.

## Known build facts / gotchas (HND 5.04 platform)

- The build expects the toolchains at **`/opt/toolchains`** (the Dockerfile
  symlinks the mounted `am-toolchains` repo there) and several environment
  variables / PATH entries — handled in `docker/Dockerfile` and `build.sh`.
- Upstream historically targets **Ubuntu LTS (20.04 era)** for HND builds; the
  Dockerfile pins that. Don't "upgrade" the base image casually — toolchain
  binaries are prebuilt and picky about host libs.
- The build must run as a **non-root user in some steps** and needs
  case-sensitive filesystem, plenty of file handles, and `python2`+`python3`
  available. All handled in the image.
- First build should be **completely unmodified upstream** (Milestone 1 steps
  1–12). Only after that do we apply `firmware/patches/`.

## Where AXOS eventually hooks into the firmware build

Later milestones (not now for full rootfs integration):

- Install `axosd` into the rootfs and register an init hook.
- Add busybox/tool config we need.
- Web UI additions under the ASUS httpd's pages.

**Phase 9 (bootstrap bake-in)** is started: `firmware/patches/0002-axos-jffs-bootstrap.patch`
adds `/usr/sbin/axos-bootstrap` (via `release/src/router/others/axos-bootstrap`)
so a flashed image can start a JFFS-deployed `axosd` without relying solely on
hand-edited `services-start`. Source of truth:
`firmware/patches/axos-bootstrap/axos-bootstrap.sh` (mirrored at
`scripts/router/axos-bootstrap.sh`). Regenerate with
`firmware/patches/gen-0002.sh` when the Merlin submodule is checked out.
Until that image is flashed, `scripts/router/install-axosd.sh` + JFFS
`services-start` remain the primary boot path — squashfs cannot be mutated
live to drop files into `/usr/sbin`.

All of that arrives as reviewed patches in `firmware/patches/`, one logical
change per patch, applied in order by `build.sh`.
