#!/usr/bin/env bash
# Generate firmware/patches/0002-axos-jffs-bootstrap.patch against a
# checked-out Merlin tree (firmware/src/asuswrt-merlin.ng).
#
# Usage (from repo root, after submodule init+update):
#   ./firmware/patches/gen-0002.sh
#
# The script copies firmware/patches/axos-bootstrap/axos-bootstrap.sh into
# release/src/router/others/axos-bootstrap, appends an install rule to
# others/Makefile when present, and writes a git-format patch. It does NOT
# commit into the submodule.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MERLIN="$HERE/firmware/src/asuswrt-merlin.ng"
SRC="$HERE/firmware/patches/axos-bootstrap/axos-bootstrap.sh"
OUT="$HERE/firmware/patches/0002-axos-jffs-bootstrap.patch"

if [ ! -d "$MERLIN/release/src/router" ]; then
  echo "error: Merlin tree not checked out at $MERLIN" >&2
  echo "  git submodule update --init firmware/src/asuswrt-merlin.ng" >&2
  exit 1
fi

OTHERS="$MERLIN/release/src/router/others"
mkdir -p "$OTHERS"
cp "$SRC" "$OTHERS/axos-bootstrap"
chmod 755 "$OTHERS/axos-bootstrap"

MAKEFILE="$OTHERS/Makefile"
if [ -f "$MAKEFILE" ] && ! grep -qF 'axos-bootstrap' "$MAKEFILE"; then
  {
    echo ""
    echo "# AXOS Phase 9 — install JFFS bootstrap helper"
    echo "install: install-axos-bootstrap"
    echo "install-axos-bootstrap:"
    echo "	install -D -m 755 axos-bootstrap \$(INSTALLDIR)/usr/sbin/axos-bootstrap"
  } >> "$MAKEFILE"
  echo "==> Appended install-axos-bootstrap rule to others/Makefile"
fi

# Optional: if a stock services-start template exists under rom/, append a
# one-line call. Most Merlin trees leave services-start to JFFS only — skip
# silently when no template is found.
for candidate in \
  "$MERLIN/release/src/router/rom/etc/services-start" \
  "$MERLIN/release/src/router/others/services-start"; do
  if [ -f "$candidate" ] && ! grep -qF axos-bootstrap "$candidate"; then
    echo "/usr/sbin/axos-bootstrap &" >> "$candidate"
    echo "==> Hooked axos-bootstrap from $candidate"
  fi
done

cd "$MERLIN"
git add -N release/src/router/others/axos-bootstrap 2>/dev/null || true
git diff --no-ext-diff -- release/src/router/others/axos-bootstrap \
  release/src/router/others/Makefile \
  release/src/router/rom/etc/services-start \
  release/src/router/others/services-start \
  > "$OUT" || true

# Prefer a complete tracked diff if files were already added:
if [ ! -s "$OUT" ]; then
  git diff --no-ext-diff HEAD -- release/src/router/others/ \
    > "$OUT" || true
fi

if [ ! -s "$OUT" ]; then
  echo "error: empty patch — ensure others/ paths exist and differ from HEAD" >&2
  exit 1
fi

echo "==> Wrote $OUT ($(wc -l < "$OUT") lines)"
echo "    Verify: cd firmware/src/asuswrt-merlin.ng && git apply --check ../../patches/0002-axos-jffs-bootstrap.patch"
echo "    Reset submodule dirty files when done: git -C firmware/src/asuswrt-merlin.ng checkout -- . && git -C ... clean -fd"
