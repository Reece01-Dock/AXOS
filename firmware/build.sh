#!/usr/bin/env bash
# Build the GT-AX6000 Asuswrt-Merlin firmware image inside a pinned Docker
# build environment. Requires ./setup-sources.sh to have been run first.
#
# Usage:
#   ./build.sh                 # build stock (no AXOS patches)
#   APPLY_PATCHES=1 ./build.sh # also apply firmware/patches/*.patch first
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="${SRC_DIR:-$HERE/src}"
MERLIN_SRC="$SRC_DIR/asuswrt-merlin.ng"
TOOLCHAINS_SRC="$SRC_DIR/am-toolchains"
OUT_DIR="${OUT_DIR:-$HERE/out}"
IMAGE_TAG="${IMAGE_TAG:-axos-merlin-build}"
APPLY_PATCHES="${APPLY_PATCHES:-0}"

if [ ! -d "$MERLIN_SRC" ] || [ ! -d "$TOOLCHAINS_SRC" ]; then
  echo "error: sources not found under $SRC_DIR — run ./setup-sources.sh first" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

echo "==> Building Docker image ($IMAGE_TAG)"
docker build -t "$IMAGE_TAG" "$HERE/docker"

if [ "$APPLY_PATCHES" = "1" ]; then
  echo "==> Applying firmware/patches/*.patch to $MERLIN_SRC"
  shopt -s nullglob
  patches=("$HERE"/patches/*.patch)
  shopt -u nullglob
  if [ ${#patches[@]} -eq 0 ]; then
    echo "    (no patches found in firmware/patches/)"
  fi
  for p in "${patches[@]}"; do
    echo "    applying $(basename "$p")"
    git -C "$MERLIN_SRC" apply --check "$p"
    git -C "$MERLIN_SRC" apply "$p"
  done
else
  echo "==> APPLY_PATCHES=0 — building unmodified upstream (Milestone 1 default)"
fi

# The HND GT-AX6000 build directory within the Merlin tree. Verify this path
# against the checked-out tag if the build fails with "no such directory" —
# HND source layouts have moved between SDK revisions.
BUILD_SUBDIR="release/src-rt-5.04axhnd.675x"

echo "==> Running build inside container (this can take 45-90+ minutes)"
docker run --rm \
  -v "$MERLIN_SRC":/build/asuswrt-merlin.ng \
  -v "$TOOLCHAINS_SRC":/opt/am-toolchains \
  -v "$OUT_DIR":/build/out \
  -w /build/asuswrt-merlin.ng/release/src-rt-5.04axhnd.675x \
  "$IMAGE_TAG" \
  bash -lc '
    set -euo pipefail
    echo "== toolchain check =="
    ls /opt/toolchains || { echo "toolchain symlink missing/broken"; exit 1; }
    echo "== building GT-AX6000 =="
    # The exact target name/make invocation for this model; confirm against
    # asuswrt-merlin.ng/release/src-rt-5.04axhnd.675x/target.mak or the repo
    # README for the correct GT-AX6000 profile before the first real run.
    make gtax6000
  '

echo "==> Locating build output"
found=$(find "$MERLIN_SRC" -maxdepth 6 -iname "*GT-AX6000*" \( -iname "*.w" -o -iname "*.pkgtb" -o -iname "*.trx" \) -print -quit || true)
if [ -n "$found" ]; then
  cp -v "$found" "$OUT_DIR/"
  echo "==> Output copied to $OUT_DIR/$(basename "$found")"
else
  echo "warning: could not locate a firmware image automatically — inspect the"
  echo "         build tree's release/ or image/ output directory by hand and"
  echo "         update this script's search once the real path is known."
fi

echo "==> Build script finished. Record the resulting refs in docs/ROADMAP.md / flashing-and-recovery.md before flashing."
