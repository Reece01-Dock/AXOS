#!/usr/bin/env bash
# Build the GT-AX6000 Asuswrt-Merlin firmware image inside a pinned Docker
# build environment. Requires ./setup-sources.sh to have been run first.
#
# Usage:
#   ./build.sh                 # build stock (no AXOS patches)
#   APPLY_PATCHES=1 ./build.sh # also apply firmware/patches/*.patch first
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="${SRC_DIR:-$HERE/src}"
MERLIN_SRC="$SRC_DIR/asuswrt-merlin.ng"
TOOLCHAINS_SRC="$SRC_DIR/am-toolchains"
OUT_DIR="${OUT_DIR:-$HERE/out}"
IMAGE_TAG="${IMAGE_TAG:-axos-merlin-build}"
APPLY_PATCHES="${APPLY_PATCHES:-0}"

if [ ! -d "$MERLIN_SRC" ] || [ ! -d "$TOOLCHAINS_SRC" ]; then
  echo "error: sources not found under $SRC_DIR — run ./setup-sources.sh first" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

echo "==> Building Docker image ($IMAGE_TAG)"
docker build -t "$IMAGE_TAG" "$HERE/docker"

if [ "$APPLY_PATCHES" = "1" ]; then
  echo "==> Applying firmware/patches/*.patch to $MERLIN_SRC"
  shopt -s nullglob
  patches=("$HERE"/patches/*.patch)
  shopt -u nullglob
  if [ ${#patches[@]} -eq 0 ]; then
    echo "    (no patches found in firmware/patches/)"
  fi
  for p in "${patches[@]}"; do
    echo "    applying $(basename "$p")"
    git -C "$MERLIN_SRC" apply --check "$p"
    git -C "$MERLIN_SRC" apply "$p"
  done
else
  echo "==> APPLY_PATCHES=0 — building unmodified upstream (Milestone 1 default)"
fi

# The HND GT-AX6000 build directory within the Merlin tree. Confirmed
# against a real checkout of the pinned tag (3004.388.9): this is the SDK
# dir containing release/src-rt-5.04axhnd.675x/router-sysdep.gt-ax6000/ and
# chip_profile.mak's "GT-AX6000_CHIP_PROFILE=4912" line.
BUILD_SUBDIR="release/src-rt-5.04axhnd.675x"

echo "==> Running build inside container (this can take 45-90+ minutes)"
docker run --rm \
  -v "$MERLIN_SRC":/build/asuswrt-merlin.ng \
  -v "$TOOLCHAINS_SRC":/opt/am-toolchains \
  -v "$OUT_DIR":/build/out \
  -w /build/asuswrt-merlin.ng/release/src-rt-5.04axhnd.675x \
  "$IMAGE_TAG" \
  bash -lc '
    set -euo pipefail
    echo "== toolchain check =="
    ls /opt/toolchains || { echo "toolchain symlink missing/broken"; exit 1; }
    # /etc/ld.so.conf.d/am-toolchains.conf (see Dockerfile) names the
    # crosstools lib dirs, but they only exist now that this volume is
    # mounted — rebuild the ldconfig cache against the real, now-present
    # directories so cc1 and friends can find their bundled libisl/libmpc/
    # libmpfr/libgmp (their baked-in RPATH points at the original build
    # machine, not here — see the Dockerfile comment for the full story).
    echo "== refreshing ldconfig cache for the mounted toolchains =="
    sudo ldconfig
    echo "== building GT-AX6000 =="
    # Target name confirmed against the upstream repo'\''s own multi-model
    # build automation (tools/build-all: build_fw() does exactly
    # `cd release/src-rt-5.04axhnd.675x && make "$FWMODEL"` with
    # FWMODEL="gt-ax6000" — note the dash; "gtax6000" (no dash) is not a
    # valid target and was an earlier, unverified guess in this script).
    #
    # RTCONFIG_UUPLUGIN/RTCONFIG_GEARUPPLUGIN=n: these ASUS cloud-account
    # plugin features (unrelated to core networking) default OFF in both
    # release/src/router/config/config.in and config_base, and GT-AX6000'\''s
    # own config fragment (targets/94912GW/94912GW.GT-AX6000) never turns
    # them on — yet release/src/router/shared/Makefile'\''s OBJS list still
    # pulled in prebuild/uu_utils.o, which this Merlin release genuinely
    # does not ship for GT-AX6000 (confirmed: prebuild/GT-AX6000/ has 16
    # other prebuilt .o files, not this one — only present for
    # RT-AX86U/RT-AX58U/RT-AX68U/RT-AX88U/GT-AX11000).
    #
    # After that fix hit the exact same failure shape twice more
    # (RTCONFIG_TPVPN pulling in prebuild/tpvpn.o), a full audit was done
    # across every "prebuild/" directory in the tree (46 of them) rather
    # than continuing to fix these one crash at a time — comparing
    # GT-AX6000's file set against every sibling model's, then tracing
    # each gap's consuming Makefile:
    #
    #   - RTCONFIG_TPVPN, RTCONFIG_AMAS_ADTBW, RTCONFIG_PRELINK,
    #     RTCONFIG_BRCM_HOSTAPD: same shape as UUPLUGIN — each gates an
    #     OBJS += prebuild/*.o in release/src/router/rc/Makefile with no
    #     source-file fallback, each defaults off in config.in/config_base,
    #     and GT-AX6000's own fragment never turns any of them on either.
    #     GT-AX6000's prebuild/ is missing every one of these objects
    #     (amas-adtbw-broadcom.o, amas_adtbw.o, amas_prelink.o,
    #     hostapd_config.o, tpvpn.o, wps_pbcd.o) — this model's own source
    #     release plainly never intended these built.
    #   - RTCONFIG_RGBLED, RTCONFIG_BT_CONN: gate whole subdirectories
    #     (aura_sw; bluez-5.56 and btconfig) that have no GT-AX6000
    #     prebuild/ entry *at all* (unlike the others, not even a
    #     partial/missing-file case — no per-model prebuilt anything
    #     exists for this hardware). Both also default off with no
    #     GT-AX6000 override, consistent with this model having neither
    #     RGB LEDs nor a Bluetooth radio.
    #   - dns_dpi_check.o (also missing from GT-AX6000's rc/prebuild) is
    #     NOT a risk: confirmed via `git grep` it is not referenced by name
    #     anywhere in the tracked source, in any Makefile or .c file — an
    #     orphaned prebuilt artifact nothing actually consumes.
    #   - asd2.1 (also has no GT-AX6000 prebuild/ entry) is NOT a risk
    #     either: its own Makefile copies prebuild/$(BUILD_NAME)/* with a
    #     leading "-" (make's ignore-errors-on-this-line prefix), so a
    #     missing prebuilt binary there is a silent, designed-in no-op,
    #     not a hard failure — unlike the OBJS+= pattern above.
    #
    # All six flags below default off in both
    # release/src/router/config/config.in and config_base, and
    # targets/94912GW/94912GW.GT-AX6000 never overrides any of them on —
    # forcing them off on the command line (which wins over whatever
    # internal Kconfig/.config state is otherwise enabling them; confirmed
    # no `override` directive anywhere in these Makefiles that would defeat
    # it) is safe regardless of the exact cause, and matches what the
    # shipped source for this model clearly intends.
    make gt-ax6000 \
      RTCONFIG_UUPLUGIN=n RTCONFIG_GEARUPPLUGIN=n \
      RTCONFIG_TPVPN=n RTCONFIG_AMAS_ADTBW=n RTCONFIG_PRELINK=n \
      RTCONFIG_BRCM_HOSTAPD=n RTCONFIG_RGBLED=n RTCONFIG_BT_CONN=n
  '

echo "==> Locating build output"
# tools/build-all looks specifically in image/ and matches
# *_nand_squashfs.pkgtb for this model (confirmed against upstream's own
# build_fw() function) — search there first, then fall back to a broader
# scan in case the layout differs for the pinned ref actually checked out.
found=$(find "$MERLIN_SRC/$BUILD_SUBDIR/image" -maxdepth 1 -iname "*_nand_squashfs.pkgtb" -print -quit 2>/dev/null || true)
if [ -z "$found" ]; then
  found=$(find "$MERLIN_SRC" -maxdepth 6 -iname "*GT-AX6000*" \( -iname "*.w" -o -iname "*.pkgtb" -o -iname "*.trx" \) -print -quit || true)
fi
if [ -n "$found" ]; then
  image_name="$(basename "$found")"
  cp -v "$found" "$OUT_DIR/"
  echo "==> Output copied to $OUT_DIR/$image_name"

  echo "==> Hashing artifact"
  image_sha256="$(sha256sum "$OUT_DIR/$image_name" | awk '{print $1}')"
  echo "    sha256: $image_sha256"

  echo "==> Writing build manifest"
  pinned_file="$HERE/.sources-pinned"
  merlin_ref="unknown"; merlin_commit="unknown"
  toolchains_ref="unknown"; toolchains_commit="unknown"
  if [ -f "$pinned_file" ]; then
    # shellcheck disable=SC1090
    . "$pinned_file"
    merlin_ref="${MERLIN_REF:-unknown}"; merlin_commit="${MERLIN_COMMIT:-unknown}"
    toolchains_ref="${TOOLCHAINS_REF:-unknown}"; toolchains_commit="${TOOLCHAINS_COMMIT:-unknown}"
  else
    echo "    warning: $pinned_file not found (did you run setup-sources.sh?) — manifest refs will read 'unknown'"
  fi
  manifest_path="$OUT_DIR/$image_name.manifest.json"
  cat > "$manifest_path" <<EOF
{
  "image": "$image_name",
  "sha256": "$image_sha256",
  "built_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "apply_patches": $([ "$APPLY_PATCHES" = "1" ] && echo true || echo false),
  "merlin_ref": "$merlin_ref",
  "merlin_commit": "$merlin_commit",
  "toolchains_ref": "$toolchains_ref",
  "toolchains_commit": "$toolchains_commit"
}
EOF
  echo "==> Manifest written to $manifest_path"
else
  echo "warning: could not locate a firmware image automatically — inspect the"
  echo "         build tree's release/ or image/ output directory by hand and"
  echo "         update this script's search once the real path is known."
  echo "         (no hash or manifest was generated, since there is no artifact yet)"
fi

echo "==> Build script finished. Record the resulting refs in docs/ROADMAP.md / flashing-and-recovery.md before flashing."
