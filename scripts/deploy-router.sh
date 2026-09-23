#!/usr/bin/env bash
# Deploy a local build to a real router over SSH: build+test locally
# (exactly axosctl deploy's own pipeline — see docs/development.md),
# rsync the resulting release to the router's staging dir, then run
# `axosctl deploy` remotely to promote it atomically.
#
# The staged release includes bin/{axosd,axos-mcp,axosctl} and www/
# (Phase 7 web UI assets from repo web/*.html|css|js). On the router,
# axosd serve -ui-dir=$AXOS_DIR/www picks up the hot-deployed assets.
#
# STATUS: verified 2026-09-23 end-to-end against a GT-AX6000 at /jffs/axos
# (first install via install-axosd.sh, then this script with key-based SSH;
# tar-over-ssh used when rsync is unavailable).
#
# Usage:
#   ./scripts/deploy-router.sh user@router [components] [remote-axos-root]
#
# Requires axosctl already installed on the router (see
# scripts/router/install-axosd.sh for the first-ever install).
set -euo pipefail

ROUTER="${1:?usage: deploy-router.sh <user@router> [components] [remote-axos-root]}"
COMPONENTS="${2:-axosd,axos-mcp,axosctl}"
REMOTE_ROOT="${3:-/jffs/axos}"
SSH_OPTS="${SSH_OPTS:--o BatchMode=yes}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$HERE"

LOCAL_ROOT="$(mktemp -d)"
trap 'rm -rf "$LOCAL_ROOT"' EXIT

echo "==> Building + testing locally (staging under $LOCAL_ROOT), targeting linux/arm64"
go run ./cmd/axosctl deploy -root "$LOCAL_ROOT" -components "$COMPONENTS" -goos linux -goarch arm64

RELEASE_DIR="$(readlink -f "$LOCAL_ROOT/current")"
echo "==> Built release: $(basename "$RELEASE_DIR")"

echo "==> Syncing to $ROUTER:$REMOTE_ROOT/staging"
ssh $SSH_OPTS "$ROUTER" "rm -rf '$REMOTE_ROOT/staging' && mkdir -p '$REMOTE_ROOT/staging'"
if command -v rsync >/dev/null 2>&1; then
  rsync -az --delete -e "ssh $SSH_OPTS" "$RELEASE_DIR/" "$ROUTER:$REMOTE_ROOT/staging/"
else
  # Dev environments without rsync: stream the release tree over ssh.
  # Same end state as rsync (staging replaced atomically-enough for promote).
  tar -C "$RELEASE_DIR" -czf - . | ssh $SSH_OPTS "$ROUTER" "tar -C '$REMOTE_ROOT/staging' -xzf -"
fi

echo "==> Promoting on $ROUTER (checksum-verified there too — see internal/deploy)"
# Use the bootstrap axosctl under bin/ (from install-axosd.sh). After the
# first promote, releases live under current/; bin/ remains the CLI that
# drives subsequent promotes (don't run axosctl out of staging — Deploy
# renames staging away).
ssh $SSH_OPTS "$ROUTER" "'$REMOTE_ROOT/bin/axosctl' deploy -root '$REMOTE_ROOT' -skip-tests -skip-build -components '$COMPONENTS'"

# Keep bin/ lean on JFFS: point bootstrap CLI at current release and drop
# duplicate fat copies of the three binaries (44MB JFFS is tight).
ssh $SSH_OPTS "$ROUTER" "sh -s" <<EOF
set -e
ROOT='$REMOTE_ROOT'
mkdir -p "\$ROOT/bin" "\$ROOT/merlin-ui" "\$ROOT/www" "\$ROOT/run"
if [ -x "\$ROOT/current/bin/axosctl" ]; then
  cp -a "\$ROOT/current/bin/axosctl" "\$ROOT/bin/axosctl"
fi
rm -f "\$ROOT/bin/axosd" "\$ROOT/bin/axos-mcp"
# Hot-deploy Merlin UI + helper scripts (no firmware flash).
if [ -d "\$ROOT/current/merlin-ui" ]; then
  cp -a "\$ROOT/current/merlin-ui/." "\$ROOT/merlin-ui/"
fi
if [ -d "\$ROOT/current/www" ]; then
  cp -a "\$ROOT/current/www/." "\$ROOT/www/"
fi
# Helper scripts always from the repo copy pushed beside this script? Prefer
# keeping them under bin/ from the host deploy (see below).
df -h /jffs | tail -1
EOF

# Refresh bootstrap + Merlin UI helper on the router (tiny shell scripts).
scp $SSH_OPTS "$HERE/scripts/router/axos-bootstrap.sh" "$HERE/scripts/router/axos-merlin-ui.sh" \
  "${ROUTER}:$REMOTE_ROOT/bin/" 2>/dev/null || {
  # scp may be missing OpenSSH -O; fall back to tar pipe
  tar -C "$HERE/scripts/router" -cf - axos-bootstrap.sh axos-merlin-ui.sh \
    | ssh $SSH_OPTS "$ROUTER" "tar -C '$REMOTE_ROOT/bin' -xf - && chmod 755 '$REMOTE_ROOT/bin/axos-bootstrap.sh' '$REMOTE_ROOT/bin/axos-merlin-ui.sh'"
}
ssh $SSH_OPTS "$ROUTER" "chmod 755 '$REMOTE_ROOT/bin/axos-bootstrap.sh' '$REMOTE_ROOT/bin/axos-merlin-ui.sh' 2>/dev/null; ln -sf axos-bootstrap.sh '$REMOTE_ROOT/bin/axos-bootstrap' 2>/dev/null || cp -a '$REMOTE_ROOT/bin/axos-bootstrap.sh' '$REMOTE_ROOT/bin/axos-bootstrap'"

echo ""
echo "==> Deployed. Nothing was restarted automatically — restart what changed:"
echo "    ssh $ROUTER '$REMOTE_ROOT/bin/axosctl' restart <service>"
echo "then check:"
echo "    ssh $ROUTER '$REMOTE_ROOT/bin/axosctl' health"
echo "or roll back if unhealthy:"
echo "    ssh $ROUTER '$REMOTE_ROOT/bin/axosctl' rollback -root '$REMOTE_ROOT'"
