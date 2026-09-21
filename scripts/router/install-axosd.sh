#!/bin/sh
# Bootstrap install of axosd/axos-mcp/axosctl on the router from USB storage,
# and register axosd to start at boot via Merlin's services-start user
# script. This is the FIRST install only — once axosctl itself is on the
# router, prefer `axosctl deploy`/`axosctl rollback` (docs/development.md)
# for subsequent updates: atomic, checksum-verified, instantly reversible,
# which a plain file copy is not.
#
# STATUS: written against documented Merlin conventions, NOT YET RUN on real
# hardware (docs/ROADMAP.md Milestone 2). Verify USB_ROOT and the
# services-start mechanism against the actual router before relying on this.
#
# Usage (run ON the router, as root, over SSH):
#   ./install-axosd.sh /path/to/build/dir
# where the build dir contains linux/arm64 binaries named axosd, axos-mcp,
# axosctl (e.g. the output of `axosctl deploy -goos linux -goarch arm64`'s
# staging directory, or a manual `go build` of each into one directory).
set -eu

BUILD_DIR="${1:?usage: install-axosd.sh <path-to-build-dir-containing-axosd,axos-mcp,axosctl>}"

# (verify) USB mount point/label on this router — Merlin commonly mounts
# USB storage under /tmp/mnt/<label> or /mnt/<label>; confirm with `mount`.
USB_ROOT="${USB_ROOT:-/tmp/mnt/usb1}"
AXOS_DIR="$USB_ROOT/axos"
BIN_DIR="$AXOS_DIR/bin"
# Bound to loopback only — see docs/security.md "MCP transport & access
# control". Do not widen this without reading that section first.
API_ADDR="${API_ADDR:-127.0.0.1:9090}"

if [ ! -d "$USB_ROOT" ]; then
    echo "error: USB root $USB_ROOT does not exist — set USB_ROOT to the correct mount point" >&2
    exit 1
fi

mkdir -p "$BIN_DIR" "$AXOS_DIR/backups" "$AXOS_DIR/logs"
# Backups contain plaintext secrets (Wi-Fi passphrase, admin password, later
# VPN keys) and the audit log records full shell_exec commands/output —
# both must be owner-only. axosd enforces this itself on every write too
# (docs/security.md "Secrets at rest"), but set it here as well so the
# directories are never briefly world-readable between creation and first use.
chmod 700 "$AXOS_DIR" "$AXOS_DIR/backups" "$AXOS_DIR/logs"

for name in axosd axos-mcp axosctl; do
    if [ -f "$BUILD_DIR/$name" ]; then
        echo "==> Installing $name to $BIN_DIR/$name"
        cp "$BUILD_DIR/$name" "$BIN_DIR/$name"
        chmod 755 "$BIN_DIR/$name"
    else
        echo "==> warning: $BUILD_DIR/$name not found — skipping"
    fi
done

# services-start is Merlin's user-customizable boot hook
# (https://github.com/RMerl/asuswrt-merlin.ng/wiki/User-scripts). It must be
# executable and JFFS custom scripts support must be enabled
# (Administration > System > Enable JFFS custom scripts and configs).
SERVICES_START=/jffs/scripts/services-start

if [ ! -f "$SERVICES_START" ]; then
    echo "#!/bin/sh" > "$SERVICES_START"
fi

MARKER="# AXOS: axosd Core API (managed by install-axosd.sh)"
if ! grep -qF "$MARKER" "$SERVICES_START" 2>/dev/null; then
    {
        echo ""
        echo "$MARKER"
        echo "$BIN_DIR/axosd serve -backend=asuswrt -api-addr=$API_ADDR -audit=$AXOS_DIR/logs/audit.jsonl -service-log-dir=$AXOS_DIR/logs >> $AXOS_DIR/logs/axosd.log 2>&1 &"
    } >> "$SERVICES_START"
    echo "==> Registered axosd in $SERVICES_START"
else
    echo "==> $SERVICES_START already references axosd — leaving it as-is"
fi

chmod 755 "$SERVICES_START"

echo "==> Done. axosd will start on next boot (Core API on $API_ADDR), or start it now with:"
echo "    $BIN_DIR/axosd serve -backend=asuswrt -api-addr=$API_ADDR -audit=$AXOS_DIR/logs/audit.jsonl -service-log-dir=$AXOS_DIR/logs &"
echo ""
echo "axos-mcp is NOT started here — it's invoked per SSH session by whatever"
echo "connects (e.g. \`ssh router $BIN_DIR/axos-mcp\`), since it talks over"
echo "stdio to its caller, not as a background daemon. See docs/development.md."
echo ""
echo "axosctl works locally on the router (\$BIN_DIR/axosctl -api http://\$API_ADDR ...)"
echo "or from a dev machine over an SSH-tunneled API port — never by widening"
echo "-api-addr beyond loopback (docs/security.md)."
