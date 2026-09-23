#!/usr/bin/env bash
# Runs inside the build container (see ../build.sh's docker run invocation,
# which bind-mounts this file rather than embedding it as an inline
# `bash -lc '...'` string — that used to be inline here, but a long-enough
# explanatory comment containing a plain apostrophe breaks that quoting in
# a way `bash -n` can't catch (closes the quote early, splices the rest into
# literal outer-shell syntax) and it happened twice. A real file has no such
# trap.
set -euo pipefail

echo "== toolchain check =="
ls /opt/toolchains || { echo "toolchain symlink missing/broken"; exit 1; }

# CCACHE_DIR is a host bind mount (see ../build.sh) so the cache survives
# across `docker run --rm` invocations. If the host directory's ownership
# doesn't match this container's builder UID (a common bind-mount mismatch
# when the host user's UID isn't 1000), ccache would silently fail to write
# to it — fix that defensively rather than losing the cache benefit quietly.
if [ -n "${CCACHE_DIR:-}" ]; then
  mkdir -p "$CCACHE_DIR"
  if [ ! -w "$CCACHE_DIR" ]; then
    sudo chown -R "$(id -u):$(id -g)" "$CCACHE_DIR"
  fi
fi

# /etc/ld.so.conf.d/am-toolchains.conf (see ../docker/Dockerfile) names the
# crosstools lib dirs, but they only exist now that this volume is mounted —
# rebuild the ldconfig cache against the real, now-present directories so
# cc1 and friends can find their bundled libisl/libmpc/libmpfr/libgmp
# (their baked-in RPATH points at the original build machine, not here —
# see the Dockerfile comment for the full story).
echo "== refreshing ldconfig cache for the mounted toolchains =="
sudo ldconfig

# --- ccache: wrap the cross-compilers, not the whole toolchain -------------
#
# CROSS_COMPILE in this SDK (release/src-rt-5.04axhnd.675x/make.common) is
# built as an *absolute path* into /opt/toolchains/<name>/bin/ or usr/bin/
# (`CROSS_COMPILE = $(TOOLCHAIN)/bin/$(TOOLCHAIN_PREFIX)-`), never a bare
# `gcc` resolved via PATH. That means the usual ccache trick — put a
# same-named symlink to ccache earlier in PATH than the real compiler — does
# not apply here: PATH is never consulted for an already-absolute command.
#
# Standard fix for exactly this situation (documented ccache use case for
# prebuilt/relocated cross-toolchains): build a parallel directory that
# mirrors each real toolchain directory via symlinks for everything except
# the compiler driver binaries, which become small wrapper scripts that
# call `ccache <real compiler> "$@"`. Then point /opt/toolchains at the
# mirror instead of the real toolchain location. Every existing absolute
# path the vendor Makefiles construct (.../bin/aarch64-...-gcc etc.) keeps
# working unchanged; it just now resolves through the wrapper first.
#
# This runs on every container start (cheap: only symlinks + a handful of
# tiny wrapper scripts, no file copies) rather than being baked into the
# image, because /opt/am-toolchains is bind-mounted at `docker run` time —
# it does not exist yet when the image itself is built.
CCACHE_TOOLCHAINS_ROOT=/tmp/toolchains-ccache
echo "== building ccache-wrapped toolchain mirror at $CCACHE_TOOLCHAINS_ROOT =="
rm -rf "$CCACHE_TOOLCHAINS_ROOT"
mkdir -p "$CCACHE_TOOLCHAINS_ROOT"

real_toolchains_root="$(readlink -f /opt/toolchains)"
for tc_dir in "$real_toolchains_root"/crosstools-*; do
  [ -d "$tc_dir" ] || continue
  tc_name="$(basename "$tc_dir")"
  shadow_dir="$CCACHE_TOOLCHAINS_ROOT/$tc_name"
  mkdir -p "$shadow_dir"

  for entry in "$tc_dir"/*; do
    entry_name="$(basename "$entry")"
    if [ "$entry_name" = "bin" ] || [ "$entry_name" = "usr" ]; then
      # bin/ (most toolchains) or usr/ (the two gcc-5.3 toolchains, whose
      # compiler binaries live under usr/bin/) — descend one level so we
      # can wrap just the compiler drivers and symlink everything else.
      if [ "$entry_name" = "usr" ]; then
        [ -d "$entry/bin" ] || { ln -sfn "$entry" "$shadow_dir/$entry_name"; continue; }
        mkdir -p "$shadow_dir/usr"
        bin_src="$entry/bin"
        bin_dst="$shadow_dir/usr/bin"
        # Anything else under usr/ besides bin/ — symlink as a whole.
        for usr_entry in "$entry"/*; do
          ue_name="$(basename "$usr_entry")"
          [ "$ue_name" = "bin" ] || ln -sfn "$usr_entry" "$shadow_dir/usr/$ue_name"
        done
      else
        bin_src="$entry"
        bin_dst="$shadow_dir/bin"
      fi
      mkdir -p "$bin_dst"
      for bin_file in "$bin_src"/*; do
        [ -f "$bin_file" ] || { ln -sfn "$bin_file" "$bin_dst/$(basename "$bin_file")"; continue; }
        bf_name="$(basename "$bin_file")"
        case "$bf_name" in
          *-gcc|*-g++|*-cc)
            cat > "$bin_dst/$bf_name" <<WRAPPER
#!/bin/sh
exec ccache "$bin_file" "\$@"
WRAPPER
            chmod +x "$bin_dst/$bf_name"
            ;;
          *)
            ln -sfn "$bin_file" "$bin_dst/$bf_name"
            ;;
        esac
      done
    else
      ln -sfn "$entry" "$shadow_dir/$entry_name"
    fi
  done
done

sudo ln -sfn "$CCACHE_TOOLCHAINS_ROOT" /opt/toolchains
echo "== toolchain mirror ready; ccache stats before build =="
ccache -s || true

echo "== building GT-AX6000 (jobs: ${BUILD_JOBS:-4}) =="
# Target name confirmed against the upstream repo's own multi-model build
# automation (tools/build-all: build_fw() does exactly
# `cd release/src-rt-5.04axhnd.675x && make "$FWMODEL"` with
# FWMODEL="gt-ax6000" — note the dash; "gtax6000" (no dash) is not a valid
# target and was an earlier, unverified guess in this script).
#
# RTCONFIG_UUPLUGIN/RTCONFIG_GEARUPPLUGIN/RTCONFIG_TPVPN/RTCONFIG_AMAS_ADTBW/
# RTCONFIG_PRELINK/RTCONFIG_BRCM_HOSTAPD/RTCONFIG_RGBLED/RTCONFIG_BT_CONN=n:
# six confirmed prebuild/*.o gaps this Merlin release genuinely doesn't ship
# for GT-AX6000, plus two hardware features this model doesn't have (RGB
# LEDs, Bluetooth) — see docs/ROADMAP.md and firmware/patches/README.md for
# the full trace of each. All eight default off in config.in/config_base
# with GT-AX6000's own fragment never overriding any of them on; forcing
# them off on the command line wins over whatever internal Kconfig/.config
# state is otherwise enabling some of them, confirmed safe (no `override`
# directive anywhere in the relevant Makefiles).
#
# -j "$BUILD_JOBS": this SDK's own kernel-build step hardcodes its own
# `make -j 9` internally (release/src-rt/Makefile ~line 1226-1227) —
# confirmed in source, not assumed — so that one phase's parallelism isn't
# actually governed by this flag; everything else in the tree that responds
# to -j is bounded by it.
make -j "${BUILD_JOBS:-4}" gt-ax6000 \
  RTCONFIG_UUPLUGIN=n RTCONFIG_GEARUPPLUGIN=n \
  RTCONFIG_TPVPN=n RTCONFIG_AMAS_ADTBW=n RTCONFIG_PRELINK=n \
  RTCONFIG_BRCM_HOSTAPD=n RTCONFIG_RGBLED=n RTCONFIG_BT_CONN=n

echo "== ccache stats after build (compare hit rate against the 'before' run above) =="
ccache -s || true
