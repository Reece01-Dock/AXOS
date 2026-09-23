#!/usr/bin/env bash
# Build an unmodified Asuswrt-Merlin GT-AX6000 image (no AXOS patches).
#
# Uses a separate pristine tree under firmware/src-stock/ and writes artifacts
# to firmware/out-stock/ so previously patched builds cannot leak in.
#
# Usage:
#   ./build-stock.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STOCK_SRC="${STOCK_SRC:-$HERE/src-stock}"
MERLIN_SRC="$STOCK_SRC/asuswrt-merlin.ng"
TOOLCHAINS_SRC="${TOOLCHAINS_SRC:-$STOCK_SRC/am-toolchains}"
OUT_DIR="${OUT_DIR:-$HERE/out-stock}"
LOG_DIR="${LOG_DIR:-$HERE/.build-state/stock-logs}"
IMAGE_TAG="${IMAGE_TAG:-axos-merlin-build}"
ENTRYPOINT="$HERE/docker/entrypoint-stock.sh"
BUILD_SUBDIR="release/src-rt-5.04axhnd.675x"
# Upstream tools/build-all uses BRANCH_GTAX6000=3006.102-wifi6 for this model.
# The multi-model release tag 3006.102.8_4 lacks GT-AX6000 prebuilds (e.g. uu_utils.o).
EXPECTED_BRANCH_HINT="3006.102-wifi6"
EXPECTED_COMMIT_FILE="${EXPECTED_COMMIT_FILE:-}"  # optional pin override

if [ ! -d "$MERLIN_SRC/.git" ]; then
  echo "error: pristine Merlin tree missing at $MERLIN_SRC" >&2
  echo "       clone branch 3006.102-wifi6 there first" >&2
  exit 1
fi
if [ ! -d "$TOOLCHAINS_SRC" ]; then
  echo "error: toolchains missing at $TOOLCHAINS_SRC" >&2
  exit 1
fi
if [ ! -f "$ENTRYPOINT" ]; then
  echo "error: stock entrypoint missing: $ENTRYPOINT" >&2
  exit 1
fi

mkdir -p "$OUT_DIR" "$LOG_DIR"
LOG="$LOG_DIR/$(date -u +%Y%m%dT%H%M%SZ).log"

# Refuse to proceed if any AXOS patch application could sneak in.
if [ "${APPLY_PATCHES:-0}" = "1" ]; then
  echo "error: APPLY_PATCHES=1 is forbidden for stock builds" >&2
  exit 1
fi

echo "==> Stock Merlin identity (pre-build)" | tee "$LOG"
{
  echo "merlin_path=$MERLIN_SRC"
  echo "merlin_commit=$(git -C "$MERLIN_SRC" rev-parse HEAD)"
  echo "merlin_describe=$(git -C "$MERLIN_SRC" describe --tags --always)"
  echo "merlin_branch_hint=3006.102-wifi6"
  echo "merlin_dirty_count=$(git -C "$MERLIN_SRC" status --porcelain | wc -l)"
  echo "toolchains_commit=$(git -C "$TOOLCHAINS_SRC" rev-parse HEAD)"
  echo "apply_patches=0"
  echo "entrypoint=$(basename "$ENTRYPOINT")"
  echo "out_dir=$OUT_DIR"
} | tee -a "$LOG"

merlin_commit="$(git -C "$MERLIN_SRC" rev-parse HEAD)"
merlin_desc="$(git -C "$MERLIN_SRC" describe --tags --always)"
# Require the GT-AX6000 uu_utils prebuild that only exists on 3006.102-wifi6.
if [ ! -f "$MERLIN_SRC/release/src/router/shared/prebuild/GT-AX6000/uu_utils.o" ]; then
  echo "error: missing shared/prebuild/GT-AX6000/uu_utils.o — checkout $EXPECTED_BRANCH_HINT" >&2
  exit 1
fi
if [ "${ALLOW_DIRTY:-0}" != "1" ] && [ "$(git -C "$MERLIN_SRC" status --porcelain | wc -l)" -ne 0 ]; then
  echo "error: Merlin tree is dirty; stock build aborted (ALLOW_DIRTY=1 to resume)" >&2
  git -C "$MERLIN_SRC" status --porcelain | head -50 >&2
  exit 1
fi
tag="$merlin_desc"

# Ensure no leftover image from a previous run is mistaken for success:
# record pre-existing artifacts, then require a newer file after the build.
pre_images="$(find "$OUT_DIR" -maxdepth 1 -name '*_nand_squashfs.pkgtb' -printf '%f\n' 2>/dev/null | sort || true)"
echo "==> Pre-existing out-stock images:" | tee -a "$LOG"
echo "${pre_images:-"(none)"}" | tee -a "$LOG"

echo "==> Ensuring Docker image ($IMAGE_TAG)" | tee -a "$LOG"
docker build -t "$IMAGE_TAG" "$HERE/docker" 2>&1 | tee -a "$LOG"

echo "==> Running stock build inside container (full log: $LOG)" | tee -a "$LOG"
set +e
docker run --rm \
  -v "$MERLIN_SRC":/build/asuswrt-merlin.ng \
  -v "$TOOLCHAINS_SRC":/opt/am-toolchains \
  -v "$OUT_DIR":/build/out \
  -v "$ENTRYPOINT":/entrypoint-stock.sh:ro \
  -e ALLOW_DIRTY="${ALLOW_DIRTY:-0}" \
  -w /build/asuswrt-merlin.ng/"$BUILD_SUBDIR" \
  "$IMAGE_TAG" \
  bash /entrypoint-stock.sh 2>&1 | tee -a "$LOG"
build_rc=${PIPESTATUS[0]}
set -e

echo "==> docker/make exit status: $build_rc" | tee -a "$LOG"

# Locate newly produced image inside the stock tree only (never the old patched out/).
found=""
if [ -d "$MERLIN_SRC/$BUILD_SUBDIR/image" ]; then
  found=$(find "$MERLIN_SRC/$BUILD_SUBDIR/image" -maxdepth 1 -iname '*_nand_squashfs.pkgtb' -printf '%T@ %p\n' 2>/dev/null | sort -nr | head -1 | cut -d' ' -f2- || true)
fi

if [ "$build_rc" -ne 0 ]; then
  echo "error: stock build failed with exit $build_rc — see $LOG" >&2
  # Still write a failure marker for the report.
  cat > "$OUT_DIR/STOCK_BUILD_FAILED.json" <<EOF
{
  "status": "failed",
  "exit_code": $build_rc,
  "log": "$LOG",
  "merlin_tag": "$tag",
  "merlin_commit": "$(git -C "$MERLIN_SRC" rev-parse HEAD)",
  "finished_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "apply_patches": false
}
EOF
  exit "$build_rc"
fi

if [ -z "$found" ] || [ ! -f "$found" ]; then
  echo "error: build exited 0 but no *_nand_squashfs.pkgtb found under image/" >&2
  exit 1
fi

image_name="$(basename "$found")"
cp -v "$found" "$OUT_DIR/"
image_sha256="$(sha256sum "$OUT_DIR/$image_name" | awk '{print $1}')"
image_size="$(stat -c%s "$OUT_DIR/$image_name")"

cat > "$OUT_DIR/$image_name.manifest.json" <<EOF
{
  "image": "$image_name",
  "path": "$OUT_DIR/$image_name",
  "size_bytes": $image_size,
  "sha256": "$image_sha256",
  "built_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "apply_patches": false,
  "axos_patches": "none",
  "merlin_ref": "$tag",
  "merlin_commit": "$(git -C "$MERLIN_SRC" rev-parse HEAD)",
  "toolchains_commit": "$(git -C "$TOOLCHAINS_SRC" rev-parse HEAD)",
  "build_command": "make gt-ax6000",
  "entrypoint": "docker/entrypoint-stock.sh",
  "log": "$LOG",
  "source_tree": "$MERLIN_SRC"
}
EOF

echo "==> STOCK IMAGE READY" | tee -a "$LOG"
echo "    path: $OUT_DIR/$image_name" | tee -a "$LOG"
echo "    size: $image_size" | tee -a "$LOG"
echo "    sha256: $image_sha256" | tee -a "$LOG"
echo "    manifest: $OUT_DIR/$image_name.manifest.json" | tee -a "$LOG"
