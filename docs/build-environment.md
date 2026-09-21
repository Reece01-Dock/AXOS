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
- **~60 GB free disk** (source tree + toolchains + build output). Measured,
  not estimated: a shallow (`--depth 1`) clone of just `asuswrt-merlin.ng`
  alone is **~11 GB**; the full pinned-tag checkout plus `am-toolchains`
  (prebuilt cross-compilers) plus build objects/output will exceed that. A
  30 GB disk allowance is **not enough** — confirmed by attempting exactly
  that and running out of headroom before the toolchains or build step. Use
  a real build machine or VM with the full ~60 GB, not a constrained sandbox.
- Decent CPU; a full HND build takes on the order of an hour on 8 cores

## Layout

Everything lives under `firmware/` and is gitignored except the scripts:

```
firmware/
├── docker/Dockerfile        # pinned Ubuntu build image
├── setup-sources.sh         # clones asuswrt-merlin.ng + am-toolchains
├── build.sh                 # runs the GT-AX6000 build inside the container
├── patches/                 # our source modifications, as git patches
└── src/                     # (gitignored) asuswrt-merlin.ng checkout
    └── ../am-toolchains/    # (gitignored) prebuilt toolchains
```

We deliberately keep the Merlin tree as a **separate clone** (not vendored into
this repo) for now: it is multi-GB and mostly not ours. AXOS changes to Merlin
are maintained as patches in `firmware/patches/` applied on top of a pinned
upstream tag. If/when our diff grows large, we switch to a proper hosted fork of
`RMerl/asuswrt-merlin.ng` and pin a branch instead — the scripts already support
overriding the clone URL.

## Usage

```sh
cd firmware
./setup-sources.sh            # clone sources + toolchains, pin versions
./build.sh                    # build docker image, run 'make gt-ax6000' inside it
```

Output image lands in `firmware/out/` (a `GT-AX6000_*.w` / `.pkgtb` file — the
exact artifact name/extension comes from the Merlin build; the script prints it).

## Version pinning

`setup-sources.sh` pins:

- the `asuswrt-merlin.ng` **tag/branch** (default: the current 3004.388 release
  tag — set `MERLIN_REF` to override). Pin to a **released tag**, not `master`:
  master may contain unreleased/broken state.
- the `am-toolchains` commit matching that release.

Record the exact refs used for any image you flash in `docs/ROADMAP.md` notes or
the flash log — reproducibility is worthless if we don't know what we built.

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

Later milestones (not now):

- Install `axosd` into the rootfs and register an init hook.
- Add busybox/tool config we need.
- Web UI additions under the ASUS httpd's pages.

All of that arrives as reviewed patches in `firmware/patches/`, one logical
change per patch, applied in order by `build.sh`.
