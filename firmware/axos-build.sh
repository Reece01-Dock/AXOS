#!/usr/bin/env bash
# axos-build.sh: a thin controller around build.sh giving the hot-deploy
# workflow's "build/resume/status/rebuild/clean" vocabulary for the
# firmware build too (see docs/development.md for the same idea applied to
# axosd itself, and docs/build-environment.md "Incremental builds" for why
# this exists and what it does and doesn't guarantee).
#
# This does not reimplement incremental building — that's build.sh calling
# into the vendor Make tree, whose own dependency tracking is what actually
# decides what recompiles (see firmware/patches/0003-*.patch, which is what
# makes that tracking trustworthy across retries in the first place). This
# script adds: state you can ask "what happened last time" about, logs you
# can inspect after the fact, a lock so two builds can't corrupt the same
# tree at once, and mechanical rebuild/clean of one component by removing
# just its compiled outputs and letting Make figure out the rest.
#
# Usage:
#   ./axos-build.sh build                  # normal incremental build
#   ./axos-build.sh resume                 # retry after failure/interrupt
#   ./axos-build.sh status                 # what happened last time
#   ./axos-build.sh rebuild <component>    # invalidate + rebuild one component
#   ./axos-build.sh clean <component>      # invalidate one component only
#   ./axos-build.sh clean-all              # wipe all build state (not source)
#   ./axos-build.sh explain-rebuild        # why would `make gt-ax6000` do work
#
# Environment (same as build.sh, which this calls into):
#   APPLY_PATCHES=1  BUILD_JOBS=N  IMAGE_TAG=...  OUT_DIR=...  SRC_DIR=...
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="${SRC_DIR:-$HERE/src}"
MERLIN_SRC="$SRC_DIR/asuswrt-merlin.ng"
TOOLCHAINS_SRC="$SRC_DIR/am-toolchains"
BUILD_SUBDIR="release/src-rt-5.04axhnd.675x"
BUILD_ROOT="$MERLIN_SRC/$BUILD_SUBDIR"
APPLY_PATCHES="${APPLY_PATCHES:-0}"

STATE_DIR="$HERE/.build-state"
LOCK_FILE="$STATE_DIR/lock"
LAST_RUN_FILE="$STATE_DIR/last-run.json"
LOGS_DIR="$STATE_DIR/logs"
mkdir -p "$STATE_DIR" "$LOGS_DIR"

usage() {
  cat <<'EOF'
axos-build.sh - incremental-build controller for the GT-AX6000 firmware

Commands:
  build                   Normal incremental build (== build.sh, plus state/logging)
  resume                  Retry the last failed/interrupted build, after
                           rechecking the config it was run with hasn't changed
  status                  Show the last run's outcome, log path, and current lock state
  rebuild <component>     Remove one component's compiled outputs, then build
                           (component name = a directory under router-sysdep.gt-ax6000/
                           or release/src/router/, e.g. "bcm_util", "rc", "shared")
  clean <component>       Remove one component's compiled outputs only (no rebuild)
  clean-all               Remove ALL build state (router-sysdep/, .o/.d/.a/.so
                           outputs, fs.install staging) — NOT the source checkout
  explain-rebuild         Ask Make itself why `make gt-ax6000` would do work
                           right now (make -n, filtered) — nothing is built

Environment: same as build.sh (APPLY_PATCHES, BUILD_JOBS, IMAGE_TAG, OUT_DIR, SRC_DIR).
EOF
}

require_sources() {
  if [ ! -d "$MERLIN_SRC" ] || [ ! -d "$TOOLCHAINS_SRC" ]; then
    echo "error: sources not found under $SRC_DIR — run ./setup-sources.sh first" >&2
    exit 1
  fi
}

# --- locking -----------------------------------------------------------
#
# Prevents two `build`/`resume`/`rebuild` invocations from running against
# the same MERLIN_SRC tree concurrently, which would corrupt it (two `make`
# processes writing the same generated files, two `docker run`s racing on
# the same bind-mounted source). flock is standard on the Ubuntu-family
# hosts this project targets; the lock is released automatically when the
# holding process exits, so a crashed build doesn't leave it stuck.
with_lock() {
  exec 9>"$LOCK_FILE"
  if ! flock -n 9; then
    echo "error: another axos-build.sh (build/resume/rebuild/clean) is already" >&2
    echo "       running against $MERLIN_SRC — wait for it or check:" >&2
    echo "       fuser $LOCK_FILE" >&2
    exit 1
  fi
}

# --- config identity (for resume's "did anything relevant change" check) ---
#
# A commit ID alone isn't enough (per this task's own requirement): local
# uncommitted changes in the submodule, which model/patches were requested,
# and which toolchain commit is mounted all matter too. This is deliberately
# not a hash of the whole repository — router-sysdep.gt-ax6000/ alone is
# 32MB/817 files, and hashing that on every `status`/`resume` call would
# itself become the slow part. Instead: commit + dirty-or-not (a cheap
# `git status --porcelain` bool, not a diff) + the inputs that actually
# change which Make target/flags get invoked.
current_identity() {
  local merlin_commit merlin_dirty toolchains_commit
  merlin_commit="$(git -C "$MERLIN_SRC" rev-parse HEAD 2>/dev/null || echo unknown)"
  if [ -n "$(git -C "$MERLIN_SRC" status --porcelain 2>/dev/null)" ]; then
    merlin_dirty=true
  else
    merlin_dirty=false
  fi
  toolchains_commit="$(git -C "$TOOLCHAINS_SRC" rev-parse HEAD 2>/dev/null || echo unknown)"
  printf '{"merlin_commit":"%s","merlin_dirty":%s,"toolchains_commit":"%s","apply_patches":%s,"model":"gt-ax6000"}' \
    "$merlin_commit" "$merlin_dirty" "$toolchains_commit" \
    "$([ "$APPLY_PATCHES" = "1" ] && echo true || echo false)"
}

# --- state recording (atomic: write to a temp file, then rename) -------
record_run() {
  local status="$1" log_path="$2" started="$3" finished="$4"
  local identity tmp
  identity="$(current_identity)"
  tmp="$(mktemp "$STATE_DIR/.last-run.XXXXXX")"
  cat > "$tmp" <<EOF
{
  "status": "$status",
  "started_at": "$started",
  "finished_at": "$finished",
  "log": "$log_path",
  "identity": $identity
}
EOF
  mv -f "$tmp" "$LAST_RUN_FILE"
}

# --- build / resume ------------------------------------------------------
do_build() {
  local label="$1"
  require_sources
  with_lock
  local ts log started
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  log="$LOGS_DIR/$ts.log"
  started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  echo "==> [$label] logging to $log"
  set +e
  ( cd "$HERE" && APPLY_PATCHES="$APPLY_PATCHES" ./build.sh ) 2>&1 | tee "$log"
  # Preserve the real exit code through the tee pipe, not tee's own (per
  # this task's own "preserve exit codes through logging pipelines").
  build_status=${PIPESTATUS[0]}
  set -e

  local finished
  finished="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  if [ "$build_status" -eq 0 ]; then
    record_run "success" "$log" "$started" "$finished"
    echo "==> [$label] succeeded — see $log"
  else
    record_run "failed" "$log" "$started" "$finished"
    echo "==> [$label] FAILED (exit $build_status) — see $log" >&2
  fi
  return "$build_status"
}

cmd_build() { do_build "build"; }

cmd_resume() {
  if [ ! -f "$LAST_RUN_FILE" ]; then
    echo "No previous run recorded — running a normal build."
    do_build "resume"
    return $?
  fi
  local prev_status prev_identity cur_identity
  prev_status="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["status"])' "$LAST_RUN_FILE" 2>/dev/null || echo unknown)"
  prev_identity="$(python3 -c 'import json,sys; print(json.dumps(json.load(open(sys.argv[1]))["identity"], sort_keys=True))' "$LAST_RUN_FILE" 2>/dev/null || echo '{}')"
  cur_identity="$(current_identity | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin), sort_keys=True))' 2>/dev/null || current_identity)"

  if [ "$prev_status" = "success" ]; then
    echo "Last run already succeeded ($LAST_RUN_FILE). Nothing to resume."
    echo "Use './axos-build.sh build' to build again (incrementally), or"
    echo "'./axos-build.sh rebuild <component>' to force one component."
    return 0
  fi

  if [ "$prev_identity" != "$cur_identity" ]; then
    echo "warning: the model/patches/toolchain/source identity has changed"
    echo "         since the last (failed) run — a plain resume would be"
    echo "         retrying under different conditions than the failure"
    echo "         happened under. Proceeding anyway: Make's own dependency"
    echo "         tracking will still only rebuild what's actually stale,"
    echo "         but re-read the diff below before trusting the result."
    echo "  was: $prev_identity"
    echo "  now: $cur_identity"
  fi

  echo "==> Resuming after previous status: $prev_status"
  do_build "resume"
}

cmd_status() {
  if [ -f "$LOCK_FILE" ] && ! flock -n -x "$LOCK_FILE" -c true 2>/dev/null; then
    echo "Build currently IN PROGRESS (lock held: $LOCK_FILE)"
  else
    echo "No build currently in progress."
  fi
  echo
  if [ -f "$LAST_RUN_FILE" ]; then
    echo "Last run:"
    python3 -m json.tool "$LAST_RUN_FILE" 2>/dev/null || cat "$LAST_RUN_FILE"
  else
    echo "No run recorded yet — nothing built through axos-build.sh so far."
  fi
}

# --- component rebuild/clean ---------------------------------------------
#
# Finds a component's compiled *outputs*. Confirmed by testing against the
# real checkout (not assumed) that this needed two fixes beyond the first
# draft:
#
#   1. router-sysdep.gt-ax6000/<name> and release/src/router/<name> can
#      both legitimately exist under the *same* name for *different*
#      components — e.g. release/src/router/bcm_util is a real, separate,
#      much smaller legacy component (just bcm_crc.c/.h) for non-HND
#      profiles, unrelated to router-sysdep.gt-ax6000/bcm_util (the actual
#      GT-AX6000 one, with bcm_ethswutils.c etc.). A name-only fallback
#      silently invalidated the *wrong* component. Fixed: router-sysdep
#      locations are always checked first and preferred; release/src/router
#      is only used as a fallback for names that don't exist under
#      router-sysdep.gt-ax6000 at all.
#   2. Every one of the 46 "prebuild/" directories audited earlier in this
#      project (firmware/patches/README.md, docs/ROADMAP.md) contains
#      vendor-shipped prebuilt .o blobs checked into git — source inputs,
#      never this build's own output. A first version of this find had no
#      exclusion for them and deleted 608 tracked files out of
#      release/src/router/rc/prebuild/ on the very first real test run
#      (caught immediately by `git status`, restored via `git checkout --`
#      before anything else touched it). Fixed: -not -path '*/prebuild/*'
#      on every invalidation, unconditionally.
#
# Given real compiled output only starts existing under router-sysdep/
# (the synced copy — router-sysdep.gt-ax6000/ is pure vendor source, never
# a build target) after at least one build/sync has happened, a name that's
# only valid before any build ever ran (nothing to clean yet, correctly) is
# distinguished from a name that isn't valid at all.
component_dir() {
  local name="$1"
  if [ -d "$BUILD_ROOT/router-sysdep/$name" ]; then
    echo "$BUILD_ROOT/router-sysdep/$name"
    return 0
  fi
  if [ -d "$BUILD_ROOT/router-sysdep.gt-ax6000/$name" ]; then
    return 2 # valid component name, but never synced/built yet — nothing to clean
  fi
  if [ -d "$MERLIN_SRC/release/src/router/$name" ]; then
    echo "$MERLIN_SRC/release/src/router/$name"
    return 0
  fi
  return 1
}

invalidate_component() {
  local name="$1" dir rc
  require_sources
  # set -e would otherwise abort the whole script right here on
  # component_dir's non-zero return (a classic pitfall: `var=$(cmd)` under
  # `set -e` aborts immediately on failure, before the next line even runs
  # — confirmed by testing: an earlier version of this silently exited
  # with no output at all instead of reaching the case statement below).
  set +e
  dir="$(component_dir "$name")"
  rc=$?
  set -e
  case "$rc" in
    0) : ;;
    2)
      echo "==> '$name' is a real component (router-sysdep.gt-ax6000/$name) but"
      echo "    has never been synced/built yet — nothing to clean."
      return 3
      ;;
    *)
      echo "error: no component directory found for '$name'" >&2
      echo "       checked: router-sysdep/$name, router-sysdep.gt-ax6000/$name, release/src/router/$name" >&2
      exit 1
      ;;
  esac
  echo "==> Removing compiled outputs under $dir (excluding prebuild/ — vendor source, never touched)"
  # Only compiled outputs — .c/.h/Makefile source is never touched, and
  # anything under a prebuild/ subdirectory is always vendor-shipped
  # source, never this build's own output (see comment above).
  find "$dir" -type f \
    \( -name '*.o' -o -name '*.d' -o -name '*.a' -o -name '*.so' -o -name '*.so.*' \) \
    -not -path '*/prebuild/*' \
    -print -delete
}

cmd_clean() {
  local name="${1:-}" rc
  [ -n "$name" ] || { echo "usage: axos-build.sh clean <component>" >&2; exit 1; }
  with_lock
  set +e
  invalidate_component "$name"
  rc=$?
  set -e
  if [ "$rc" -eq 0 ]; then
    echo "==> $name invalidated. Not rebuilding (clean, not rebuild)."
  elif [ "$rc" -eq 3 ]; then
    : # invalidate_component already explained there was nothing to do
  else
    exit "$rc"
  fi
}

cmd_rebuild() {
  local name="${1:-}" rc
  [ -n "$name" ] || { echo "usage: axos-build.sh rebuild <component>" >&2; exit 1; }
  # invalidate_component can return 3 ("nothing to clean yet" — a real,
  # non-error outcome, e.g. this component has never been built before) —
  # rebuild should still proceed to build in that case, so this must not
  # be a bare unguarded call under set -e (same pitfall fixed in cmd_clean).
  set +e
  invalidate_component "$name"
  rc=$?
  set -e
  if [ "$rc" -ne 0 ] && [ "$rc" -ne 3 ]; then
    exit "$rc"
  fi
  do_build "rebuild:$name"
}

cmd_clean_all() {
  require_sources
  with_lock
  echo "==> Removing ALL build state under $BUILD_ROOT (source checkout untouched)"
  rm -rf "$BUILD_ROOT/router-sysdep"
  # fs.install: the staging rootfs — see docs/build-environment.md
  # "Filesystem assembly" for why this is a separate, explicit operation
  # rather than something the default `build`/`resume` path touches.
  find "$BUILD_ROOT/targets" -maxdepth 1 -iname "fs.install" -exec rm -rf {} + 2>/dev/null || true
  find "$BUILD_ROOT" -maxdepth 1 -iname "chip_profile.tmp" -delete 2>/dev/null || true
  rm -rf "$STATE_DIR/last-run.json"
  echo "==> Done. Next build will be a full build."
}

cmd_explain_rebuild() {
  require_sources
  echo "==> Asking Make why 'gt-ax6000' would do work right now (dry run; nothing is built)"
  echo "    This runs make -n inside the same container/toolchain context as a real build,"
  echo "    filtered to the lines that explain staleness."
  docker run --rm \
    -v "$MERLIN_SRC":/build/asuswrt-merlin.ng \
    -v "$TOOLCHAINS_SRC":/opt/am-toolchains \
    -w /build/asuswrt-merlin.ng/release/src-rt-5.04axhnd.675x \
    "${IMAGE_TAG:-axos-merlin-build}" \
    bash -c 'sudo ldconfig >/dev/null 2>&1; make -n --debug=b gt-ax6000 2>&1' \
    | grep -iE "must remake|newer than|does not exist|considering target|no need to remake" \
    | head -200
  echo "==> (showing at most 200 matching lines; full dry-run output is what make -n printed above the filter)"
}

# --- entry point -----------------------------------------------------------
cmd="${1:-}"
shift || true
case "$cmd" in
  build) cmd_build ;;
  resume) cmd_resume ;;
  status) cmd_status ;;
  rebuild) cmd_rebuild "${1:-}" ;;
  clean) cmd_clean "${1:-}" ;;
  clean-all) cmd_clean_all ;;
  explain-rebuild) cmd_explain_rebuild ;;
  -h|--help|help|"") usage ;;
  *) echo "unknown command: $cmd" >&2; usage; exit 2 ;;
esac
