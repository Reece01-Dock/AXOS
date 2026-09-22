#!/usr/bin/env bash
# Fetch the Asuswrt-Merlin and am-toolchains source trees this repo already
# has pinned as git submodules (see ../.gitmodules) — normal usage is just
# running this with no arguments.
#
# The pin itself lives in the AXOS repo's own git history now, not in shell
# variables here: firmware/src/asuswrt-merlin.ng and firmware/src/am-toolchains
# are gitlinks pointing at exact upstream commits (visible on GitHub as
# submodule links, diffable with normal `git log`/`git show` on this repo).
# Currently pinned to asuswrt-merlin.ng tag 3004.388.9 and am-toolchains'
# master as of 2024-05-06 — verified to exist upstream and to contain the
# expected GT-AX6000 build profile (see docs/ROADMAP.md Milestone 1 for how
# that was checked).
#
# Usage:
#   ./setup-sources.sh                          # fetch the pinned commits
#   MERLIN_REF=<tag> ./setup-sources.sh --repin # move the pin to a new ref
#                                                # (checks it out, but you
#                                                # still `git add` + commit
#                                                # the updated gitlink)
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/.." && pwd)"

MERLIN_PATH="firmware/src/asuswrt-merlin.ng"
TOOLCHAINS_PATH="firmware/src/am-toolchains"

if [ "${1:-}" = "--repin" ]; then
  : "${MERLIN_REF:=}"
  : "${TOOLCHAINS_REF:=}"
  if [ -z "$MERLIN_REF" ] && [ -z "$TOOLCHAINS_REF" ]; then
    echo "error: --repin needs MERLIN_REF and/or TOOLCHAINS_REF set" >&2
    echo "  e.g. MERLIN_REF=3004.388.12_2 ./setup-sources.sh --repin" >&2
    exit 1
  fi

  cd "$REPO_ROOT"
  git submodule update --init --depth 1 -- "$MERLIN_PATH" "$TOOLCHAINS_PATH"

  if [ -n "$MERLIN_REF" ]; then
    echo "==> Re-pinning $MERLIN_PATH to $MERLIN_REF"
    git -C "$MERLIN_PATH" fetch --depth 1 origin "tag" "$MERLIN_REF" 2>&1 \
      || git -C "$MERLIN_PATH" fetch --unshallow origin
    git -C "$MERLIN_PATH" checkout "$MERLIN_REF"
    git add "$MERLIN_PATH"
  fi
  if [ -n "$TOOLCHAINS_REF" ]; then
    echo "==> Re-pinning $TOOLCHAINS_PATH to $TOOLCHAINS_REF"
    git -C "$TOOLCHAINS_PATH" fetch --depth 1 origin "$TOOLCHAINS_REF" 2>&1 \
      || git -C "$TOOLCHAINS_PATH" fetch --unshallow origin
    git -C "$TOOLCHAINS_PATH" checkout "$TOOLCHAINS_REF"
    git add "$TOOLCHAINS_PATH"
  fi

  echo "==> Done. Review with 'git diff --cached' and commit the updated pin(s)."
  echo "    (this only moved the local checkout + staged the new gitlink — it"
  echo "     is not pinned for anyone else until that commit is pushed)"
  exit 0
fi

echo "==> Fetching pinned submodules (shallow — this repo's exact pins, not 'latest')"
cd "$REPO_ROOT"
git submodule update --init --depth 1 -- "$MERLIN_PATH" "$TOOLCHAINS_PATH"

# firmware/build.sh reads sources directly from $MERLIN_PATH/$TOOLCHAINS_PATH
# (both already firmware/src/<name>, exactly where `git submodule update`
# checks them out) — no extra symlink needed. An earlier version of this
# script symlinked $SRC_DIR/<name> to that same path, i.e. onto itself; on
# this environment that self-referential `ln -sfn` wiped the am-toolchains
# checkout it was supposedly linking to. Do not reintroduce it.

# A shallow `submodule update --init --depth 1` fetches only the pinned
# commit, not tag refs, so `git describe --tags` finds nothing to name it
# with. Fetch the known pinned tag explicitly so .sources-pinned can record
# the human-readable ref instead of falling back to "unknown".
MERLIN_PINNED_TAG="3004.388.9"
git -C "$MERLIN_PATH" fetch --depth 1 origin "tag" "$MERLIN_PINNED_TAG" >/dev/null 2>&1 || true

cat > "$HERE/.sources-pinned" <<EOF
# Written by setup-sources.sh — record of what was fetched. The pin itself
# lives in this repo's git history (.gitmodules + the gitlink commits for
# $MERLIN_PATH / $TOOLCHAINS_PATH) — this file is just a
# convenience snapshot for firmware/build.sh's build manifest.
MERLIN_REPO=https://github.com/RMerl/asuswrt-merlin.ng.git
MERLIN_REF=$(git -C "$MERLIN_PATH" describe --tags --exact-match 2>/dev/null || echo "unknown")
MERLIN_COMMIT=$(git -C "$MERLIN_PATH" rev-parse HEAD)
TOOLCHAINS_REPO=https://github.com/RMerl/am-toolchains.git
TOOLCHAINS_REF=master
TOOLCHAINS_COMMIT=$(git -C "$TOOLCHAINS_PATH" rev-parse HEAD)
FETCHED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

echo "==> Done. Pinned refs recorded in firmware/.sources-pinned:"
cat "$HERE/.sources-pinned"
echo
echo "Next: cd firmware && ./build.sh"
