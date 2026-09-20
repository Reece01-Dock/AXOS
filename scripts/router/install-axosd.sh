#!/bin/sh
# Install/update axosd on the router from USB storage and register it to
# start at boot via Merlin's services-start user script.
#
# STATUS: written against documented Merlin conventions, NOT YET RUN on real
# hardware (docs/ROADMAP.md Milestone 2). Verify USB_ROOT and the
# services-start mechanism against the actual router before relying on this.
#
# Usage (run ON the router, as root, over SSH):
#   ./install-axosd.sh /path/to/axosd-linux-arm64
set -eu

BIN_SRC="${1:?usage: install-axosd.sh <path-to-axosd-binary>}"

# (verify) USB mount point/label on this router — Merlin commonly mounts
# USB storage under /tmp/mnt/<label> or /mnt/<label>; confirm with `mount`.
USB_ROOT="${USB_ROOT:-/tmp/mnt/usb1}"
AXOS_DIR="$USB_ROOT/axos"
BIN_DEST="$AXOS_DIR/axosd"

if [ ! -d "$USB_ROOT" ]; then
    echo "error: USB root $USB_ROOT does not exist — set USB_ROOT to the correct mount point" >&2
    exit 1
fi

mkdir -p "$AXOS_DIR/backups" "$AXOS_DIR/logs"
# Backups contain plaintext secrets (Wi-Fi passphrase, admin password, later
# VPN keys) and the audit log records full shell_exec commands/output —
# both must be owner-only. axosd enforces this itself on every write too
# (docs/security.md "Secrets at rest"), but set it here as well so the
# directories are never briefly world-readable between creation and first use.
chmod 700 "$AXOS_DIR" "$AXOS_DIR/backups" "$AXOS_DIR/logs"

echo "==> Installing axosd to $BIN_DEST"
cp "$BIN_SRC" "$BIN_DEST"
chmod 755 "$BIN_DEST"

# services-start is Merlin's user-customizable boot hook
# (https://github.com/RMerl/asuswrt-merlin.ng/wiki/User-scripts). It must be
# executable and JFFS custom scripts support must be enabled
# (Administration > System > Enable JFFS custom scripts and configs).
SERVICES_START=/jffs/scripts/services-start

if [ ! -f "$SERVICES_START" ]; then
    echo "#!/bin/sh" > "$SERVICES_START"
fi

MARKER="# AXOS: axosd MCP daemon (managed by install-axosd.sh)"
if ! grep -qF "$MARKER" "$SERVICES_START" 2>/dev/null; then
    {
        echo ""
        echo "$MARKER"
        echo "$BIN_DEST mcp -backend=asuswrt -audit=$AXOS_DIR/logs/audit.jsonl -actor=mcp:ai >> $AXOS_DIR/logs/axosd.log 2>&1 &"
    } >> "$SERVICES_START"
    echo "==> Registered axosd in $SERVICES_START"
else
    echo "==> $SERVICES_START already references axosd — leaving it as-is"
fi

chmod 755 "$SERVICES_START"

echo "==> Done. axosd will start on next boot, or start it now with:"
echo "    $BIN_DEST mcp -backend=asuswrt -audit=$AXOS_DIR/logs/audit.jsonl -actor=mcp:ai &"
