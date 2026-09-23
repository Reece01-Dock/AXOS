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
CCACHE_DIR_HOST="${CCACHE_DIR_HOST:-$HERE/.ccache}"
# Real build evidence (firmware/patches/0008-gt-ax6000-router-makefile-notparallel.patch,
# docs/incremental-builds.md) found two vendor parallel-build races in this
# tree under -j>1: release/src/router/Makefile's clean-build vs. fsbuild/
# $(obj-y) (fixed by 0008's .NOTPARALLEL) and router-sysdep/wlan/scripts
# needing nvramUpdate from the sibling nvram/ target before it's built
# ("No rule to make target 'nvramUpdate'", not yet patched). This vendor
# tree was evidently never validated under -j>1 in general. Default to
# strictly serial (BUILD_JOBS=1) since that's what has actually completed
# an end-to-end build; override explicitly (BUILD_JOBS=N) only once you've
# either patched the remaining race yourself or are prepared to hit it.
BUILD_JOBS="${BUILD_JOBS:-1}"

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

# The HND GT-AX6000 build directory within the Merlin tree. Confirmed
# against a real checkout of the pinned tag (3004.388.9): this is the SDK
# dir containing release/src-rt-5.04axhnd.675x/router-sysdep.gt-ax6000/ and
# chip_profile.mak's "GT-AX6000_CHIP_PROFILE=4912" line.
BUILD_SUBDIR="release/src-rt-5.04axhnd.675x"

mkdir -p "$CCACHE_DIR_HOST"

echo "==> Running build inside container (this can take 45-90+ minutes, longer still single-threaded)"
docker run --rm \
  -v "$MERLIN_SRC":/build/asuswrt-merlin.ng \
  -v "$TOOLCHAINS_SRC":/opt/am-toolchains \
  -v "$OUT_DIR":/build/out \
  -v "$CCACHE_DIR_HOST":/home/builder/.ccache \
  -v "$HERE/docker/entrypoint.sh":/entrypoint.sh:ro \
  -e CCACHE_DIR=/home/builder/.ccache \
  -e CCACHE_COMPILERCHECK=content \
  -e BUILD_JOBS="$BUILD_JOBS" \
  -w /build/asuswrt-merlin.ng/release/src-rt-5.04axhnd.675x \
  "$IMAGE_TAG" \
  bash /entrypoint.sh

echo "==> Locating build output"
# tools/build-all looks specifically in image/ and matches
# *_nand_squashfs.pkgtb for this model (confirmed against upstream's own
# build_fw() function) — search there first, then fall back to a broader
# scan in case the layout differs for the pinned ref actually checked out.
found=$(find "$MERLIN_SRC/$BUILD_SUBDIR/image" -maxdepth 1 -iname "*_nand_squashfs.pkgtb" -print -quit 2>/dev/null || true)
if [ -z "$found" ]; then
  found=$(find "$MERLIN_SRC" -maxdepth 6 -iname "*GT-AX6000*" \( -iname "*.w" -o -iname "*.pkgtb" -o -iname "*.trx" \) -print -quit || true)
fi
if [ -n "$found" ]; then
  image_name="$(basename "$found")"
  cp -v "$found" "$OUT_DIR/"
  echo "==> Output copied to $OUT_DIR/$image_name"

  echo "==> Hashing artifact"
  image_sha256="$(sha256sum "$OUT_DIR/$image_name" | awk '{print $1}')"
  echo "    sha256: $image_sha256"

  echo "==> Writing build manifest"
  pinned_file="$HERE/.sources-pinned"
  merlin_ref="unknown"; merlin_commit="unknown"
  toolchains_ref="unknown"; toolchains_commit="unknown"
  if [ -f "$pinned_file" ]; then
    # shellcheck disable=SC1090
    . "$pinned_file"
    merlin_ref="${MERLIN_REF:-unknown}"; merlin_commit="${MERLIN_COMMIT:-unknown}"
    toolchains_ref="${TOOLCHAINS_REF:-unknown}"; toolchains_commit="${TOOLCHAINS_COMMIT:-unknown}"
  else
    echo "    warning: $pinned_file not found (did you run setup-sources.sh?) — manifest refs will read 'unknown'"
  fi
  manifest_path="$OUT_DIR/$image_name.manifest.json"
  cat > "$manifest_path" <<EOF
{
  "image": "$image_name",
  "sha256": "$image_sha256",
  "built_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "apply_patches": $([ "$APPLY_PATCHES" = "1" ] && echo true || echo false),
  "merlin_ref": "$merlin_ref",
  "merlin_commit": "$merlin_commit",
  "toolchains_ref": "$toolchains_ref",
  "toolchains_commit": "$toolchains_commit"
}
EOF
  echo "==> Manifest written to $manifest_path"
else
  echo "warning: could not locate a firmware image automatically — inspect the"
  echo "         build tree's release/ or image/ output directory by hand and"
  echo "         update this script's search once the real path is known."
  echo "         (no hash or manifest was generated, since there is no artifact yet)"
fi

echo "==> Build script finished. Record the resulting refs in docs/ROADMAP.md / flashing-and-recovery.md before flashing."
