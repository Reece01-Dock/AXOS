#!/usr/bin/env bash
# Clone the Asuswrt-Merlin source tree and matching am-toolchains at pinned
# refs. Run this once (or again with FORCE=1 to re-pin/re-clone).
#
# Usage:
#   ./setup-sources.sh
#   MERLIN_REF=<tag> TOOLCHAINS_REF=<commit> ./setup-sources.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="${SRC_DIR:-$HERE/src}"

MERLIN_REPO="${MERLIN_REPO:-https://github.com/RMerl/asuswrt-merlin.ng.git}"
# Pin to a released tag, not a branch. Override with MERLIN_REF=<tag> for a
# newer/older release; check the upstream repo's tags for GT-AX6000 support
# (3004.388.x line) before changing this default.
MERLIN_REF="${MERLIN_REF:-3004.388.9}"

TOOLCHAINS_REPO="${TOOLCHAINS_REPO:-https://github.com/RMerl/am-toolchains.git}"
# Should match the toolchain commit Merlin's own build docs/README specify for
# MERLIN_REF. Verify against asuswrt-merlin.ng/README.md for that tag before
# building; default here tracks that repo's master as a starting point.
TOOLCHAINS_REF="${TOOLCHAINS_REF:-master}"

FORCE="${FORCE:-0}"

clone_or_update() {
  local repo="$1" ref="$2" dir="$3"
  if [ -d "$dir/.git" ] && [ "$FORCE" != "1" ]; then
    echo "==> $dir already exists (set FORCE=1 to re-fetch/re-pin) — skipping clone"
  else
    rm -rf "$dir"
    echo "==> Cloning $repo -> $dir"
    git clone "$repo" "$dir"
  fi
  echo "==> Pinning $dir to $ref"
  git -C "$dir" fetch --tags origin
  git -C "$dir" checkout "$ref"
}

mkdir -p "$SRC_DIR"
clone_or_update "$MERLIN_REPO" "$MERLIN_REF" "$SRC_DIR/asuswrt-merlin.ng"
clone_or_update "$TOOLCHAINS_REPO" "$TOOLCHAINS_REF" "$SRC_DIR/am-toolchains"

cat > "$HERE/.sources-pinned" <<EOF
# Written by setup-sources.sh — record of what was fetched.
MERLIN_REPO=$MERLIN_REPO
MERLIN_REF=$MERLIN_REF
MERLIN_COMMIT=$(git -C "$SRC_DIR/asuswrt-merlin.ng" rev-parse HEAD)
TOOLCHAINS_REPO=$TOOLCHAINS_REPO
TOOLCHAINS_REF=$TOOLCHAINS_REF
TOOLCHAINS_COMMIT=$(git -C "$SRC_DIR/am-toolchains" rev-parse HEAD)
FETCHED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "==> Done. Pinned refs recorded in firmware/.sources-pinned:"
cat "$HERE/.sources-pinned"
echo
echo "Next: cd firmware && ./build.sh"
