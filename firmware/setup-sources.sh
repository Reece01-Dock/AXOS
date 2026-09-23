#!/usr/bin/env bash
# Fetch the Asuswrt-Merlin and am-toolchains source trees this repo already
# has pinned as git submodules (see ../.gitmodules) — normal usage is just
# running this with no arguments.
#
# The pin itself lives in the AXOS repo's own git history now, not in shell
# variables here: firmware/src/asuswrt-merlin.ng and firmware/src/am-toolchains
# are gitlinks pointing at exact upstream commits (visible on GitHub as
# submodule links, diffable with normal `git log`/`git show` on this repo).
# Currently pinned to asuswrt-merlin.ng branch 3006.102-wifi6 (GT-AX6000
# official build-all branch; commit recorded in the submodule gitlink) and
# am-toolchains' master as of the verified stock baseline — see
# docs/stock-merlin-gt-ax6000.md and docs/ROADMAP.md Milestone 1.
#
# Usage:
#   ./setup-sources.sh                          # fetch the pinned commits
#   MERLIN_REF=<tag-or-branch> ./setup-sources.sh --repin
#                                                # move the pin to a new ref
#                                                # (checks it out, but you
#                                                # still `git add` + commit
#                                                # the updated gitlink)
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/.." && pwd)"

MERLIN_PATH="firmware/src/asuswrt-merlin.ng"
TOOLCHAINS_PATH="firmware/src/am-toolchains"
# Human-readable pin for .sources-pinned / docs. Prefer branch name for
# wifi6 (not a single multi-model release tag).
MERLIN_PINNED_REF="3006.102-wifi6"

if [ "${1:-}" = "--repin" ]; then
  : "${MERLIN_REF:=}"
  : "${TOOLCHAINS_REF:=}"
  if [ -z "$MERLIN_REF" ] && [ -z "$TOOLCHAINS_REF" ]; then
    echo "error: --repin needs MERLIN_REF and/or TOOLCHAINS_REF set" >&2
    echo "  e.g. MERLIN_REF=3006.102-wifi6 ./setup-sources.sh --repin" >&2
    exit 1
  fi

  cd "$REPO_ROOT"
  git submodule update --init --depth 1 -- "$MERLIN_PATH" "$TOOLCHAINS_PATH"

  if [ -n "$MERLIN_REF" ]; then
    echo "==> Re-pinning $MERLIN_PATH to $MERLIN_REF"
    # Accept branch or tag: try branch first (wifi6), then tag.
    if ! git -C "$MERLIN_PATH" fetch --depth 1 origin "$MERLIN_REF" 2>&1; then
      git -C "$MERLIN_PATH" fetch --depth 1 origin "tag" "$MERLIN_REF" 2>&1 \
        || git -C "$MERLIN_PATH" fetch --unshallow origin
    fi
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

# Shallow submodule update fetches only the pinned commit. Refresh the
# branch tip ref when possible so describe/ref reporting is useful.
git -C "$MERLIN_PATH" fetch --depth 1 origin "$MERLIN_PINNED_REF" >/dev/null 2>&1 || true

merlin_ref="$(git -C "$MERLIN_PATH" describe --tags --exact-match 2>/dev/null || true)"
if [ -z "$merlin_ref" ]; then
  merlin_ref="$(git -C "$MERLIN_PATH" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
fi
if [ -z "$merlin_ref" ] || [ "$merlin_ref" = "HEAD" ]; then
  merlin_ref="$MERLIN_PINNED_REF"
fi

cat > "$HERE/.sources-pinned" <<EOF
# Written by setup-sources.sh — record of what was fetched. The pin itself
# lives in this repo's git history (.gitmodules + the gitlink commits for
# $MERLIN_PATH / $TOOLCHAINS_PATH) — this file is just a
# convenience snapshot for firmware/build.sh's build manifest.
MERLIN_REPO=https://github.com/RMerl/asuswrt-merlin.ng.git
MERLIN_REF=$merlin_ref
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
echo "      Milestone 1 marker: APPLY_PATCHES=1 ./build.sh"
