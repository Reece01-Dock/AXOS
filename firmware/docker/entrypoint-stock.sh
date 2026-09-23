#!/usr/bin/env bash
# Stock Merlin entrypoint: official `make gt-ax6000` only.
# No AXOS patches, no RTCONFIG overrides, no feature-flag injection.
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "$REPO_ROOT" ]; then
  # When -w is the SDK dir, walk up to the clone root.
  REPO_ROOT="$(cd ../../.. && pwd)"
fi

echo "== toolchain check =="
ls /opt/toolchains || { echo "toolchain symlink missing/broken"; exit 1; }

echo "== refreshing ldconfig cache for the mounted toolchains =="
sudo ldconfig

echo "== source cleanliness (stock tree) =="
git -C "$REPO_ROOT" rev-parse HEAD
git -C "$REPO_ROOT" describe --tags --always
git -C "$REPO_ROOT" status -sb > /tmp/stock-git-status.txt || true
head -20 /tmp/stock-git-status.txt
dirty_count="$(git -C "$REPO_ROOT" status --porcelain | wc -l)"
tracked_dirty="$(git -C "$REPO_ROOT" diff --name-only | wc -l)"
echo "dirty_paths=$dirty_count tracked_dirty=$tracked_dirty"
# Untracked build outputs are expected after a first/partial make. Tracked
# diffs are also common (autoconf remakes). Refuse only when ALLOW_DIRTY=0
# (default on a fresh tree). Resume sets ALLOW_DIRTY=1.
if [ "${ALLOW_DIRTY:-0}" != "1" ] && [ "$dirty_count" -ne 0 ]; then
  echo "ERROR: stock build requires a clean Merlin tree; aborting (set ALLOW_DIRTY=1 to resume)" >&2
  git -C "$REPO_ROOT" status --porcelain | head -50 >&2
  exit 1
fi
# Hard guarantee: no AXOS branding in the login page.
if grep -q 'AXOS' "$REPO_ROOT/release/src/router/www/Main_Login.asp" 2>/dev/null; then
  echo "ERROR: AXOS branding detected in Main_Login.asp — not a stock tree" >&2
  exit 1
fi
# GT-AX6000 stock builds require the wifi6 branch prebuilds (uu_utils.o).
if [ ! -f "$REPO_ROOT/release/src/router/shared/prebuild/GT-AX6000/uu_utils.o" ]; then
  echo "ERROR: missing GT-AX6000 uu_utils.o prebuild — use branch 3006.102-wifi6" >&2
  exit 1
fi
echo "merlin_describe=$(git -C "$REPO_ROOT" describe --tags --always)"
echo "allow_dirty=${ALLOW_DIRTY:-0}"

echo "== building GT-AX6000 (stock, unmodified make target) =="
# Matches upstream tools/build-all: `make gt-ax6000` with no extra make vars.
# CWD is expected to be release/src-rt-5.04axhnd.675x (set by docker -w).
make gt-ax6000

echo "== stock build command finished =="
