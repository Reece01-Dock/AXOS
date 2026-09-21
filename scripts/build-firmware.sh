#!/usr/bin/env bash
# Thin wrapper at the path the project's firmware workflow convention
# expects (scripts/build-firmware.sh). The actual implementation lives in
# firmware/build.sh (validate environment, sync/build, hash the artifact,
# write a build manifest) — see docs/build-environment.md. Kept as a single
# implementation rather than duplicated logic; this just forwards args and
# runs setup-sources.sh first if the source tree isn't there yet.
#
# Does NOT flash the router — see docs/flashing-and-recovery.md. Flashing
# stays an explicit, separate, manual step.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ ! -d "$HERE/firmware/src/asuswrt-merlin.ng" ]; then
  echo "==> No source checkout found — running setup-sources.sh first"
  "$HERE/firmware/setup-sources.sh"
fi

exec "$HERE/firmware/build.sh" "$@"
