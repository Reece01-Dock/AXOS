#!/usr/bin/env bash
# Deploy a local build to a real router over SSH: build+test locally
# (exactly axosctl deploy's own pipeline — see docs/development.md),
# rsync the resulting release to the router's staging dir, then run
# `axosctl deploy` remotely to promote it atomically.
#
# STATUS: the local half (build+test+stage+promote-locally) is the same
# code path exercised by cmd/axosctl's own tests and manual verification.
# The rsync-to-a-real-router-then-remote-promote half is NOT run from this
# sandbox (no reachable GT-AX6000) and should be treated as unverified
# until it is.
#
# Usage:
#   ./scripts/deploy-router.sh user@router [components] [remote-axos-root]
#
# Requires axosctl already installed on the router (see
# scripts/router/install-axosd.sh for the first-ever install).
set -euo pipefail

ROUTER="${1:?usage: deploy-router.sh <user@router> [components] [remote-axos-root]}"
COMPONENTS="${2:-axosd,axos-mcp,axosctl}"
# (verify) USB mount path on the real router — see docs/hardware.md and
# scripts/router/install-axosd.sh's own USB_ROOT note.
REMOTE_ROOT="${3:-/tmp/mnt/usb1/axos}"
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
ssh $SSH_OPTS "$ROUTER" "mkdir -p '$REMOTE_ROOT/staging'"
rsync -az --delete -e "ssh $SSH_OPTS" "$RELEASE_DIR/" "$ROUTER:$REMOTE_ROOT/staging/"

echo "==> Promoting on $ROUTER (checksum-verified there too — see internal/deploy)"
ssh $SSH_OPTS "$ROUTER" "'$REMOTE_ROOT/bin/axosctl' deploy -root '$REMOTE_ROOT' -skip-tests -skip-build -components '$COMPONENTS'"

echo ""
echo "==> Deployed. Nothing was restarted automatically — restart what changed:"
echo "    ssh $ROUTER '$REMOTE_ROOT/bin/axosctl' restart <service>"
echo "then check:"
echo "    ssh $ROUTER '$REMOTE_ROOT/bin/axosctl' health"
echo "or roll back if unhealthy:"
echo "    ssh $ROUTER '$REMOTE_ROOT/bin/axosctl' rollback -root '$REMOTE_ROOT'"
