# AXOS Roadmap

This file is the source of truth for project progress. A box is checked `[x]`
only when the step has been **verified on real hardware or a real build
machine** — not when the code/scripts for it exist.

Legend: `[ ]` not done · `[x]` done and verified on real router hardware ·
`[s]` built and verified **in this development sandbox** — passing automated
tests, and for the parts that matter most, the actual compiled binaries run
against each other as real, separate OS processes (not just `go test`) —
but **not yet run against real router hardware** · `(scripted)` tooling
exists but hasn't been executed/verified at all yet.

`[s]` is a real, meaningful bar (see `docs/development.md` "Status: what's
actually verified" for exactly what was run), but it is not `[x]` — nothing
in this repo has touched a GT-AX6000 yet, and every hardware-specific
assumption in `internal/backend/asuswrt` remains explicitly `(verify)`
until one does.

## Milestone 1 — Prove the build → flash → recover loop

The single most important milestone. No AXOS functionality before this is green.

**Hard blocker, stated plainly:** steps 3–12 and 14–16 require a physical
GT-AX6000 — flashing it, watching it boot, confirming Wi-Fi actually
broadcasts on both bands, checking real Ethernet link speeds, and pressing
its physical recovery-mode button are not things any amount of code or
sandboxed automation can substitute for. No environment this project has had
access to so far includes one. What follows is exactly what *was* checked —
real verification against the actual upstream source, not speculation —
and exactly where it stops.

- [ ] 1. Build Asuswrt-Merlin from source for GT-AX6000 *(scripted:
      `firmware/`; **partially verified** — see below)*
- [ ] 2. Produce a firmware image (`.w` / `.pkgtb`) — blocked on disk (see below)
- [ ] 3. Flash it via the stock ASUS web UI — blocked on physical hardware
- [ ] 4. Router boots — blocked on physical hardware
- [ ] 5. 1GbE Ethernet works — blocked on physical hardware
- [ ] 6. Both 2.5GbE ports work — blocked on physical hardware
- [ ] 7. 2.4 GHz Wi-Fi works — blocked on physical hardware
- [ ] 8. 5 GHz Wi-Fi works — blocked on physical hardware
- [ ] 9. ASUS web UI works — blocked on physical hardware
- [ ] 10. SSH works — blocked on physical hardware
- [ ] 11. Recovery mode verified (bootloader rescue + firmware restoration — do this
       **before** flashing anything custom; see `docs/flashing-and-recovery.md`)
       — blocked on physical hardware
- [ ] 12. Create and store a settings + JFFS backup — blocked on physical hardware
- [ ] 13. Make one harmless, visible source modification — **patch written and
      verified**: `firmware/patches/0001-axos-login-title-marker.patch`
      (adds " (AXOS)" to the web UI login page's browser-tab title),
      generated against and `git apply --check`-confirmed on an actual
      checkout of the pinned tag `3004.388.9`. Not yet built into an image
      or flashed — see step 2.
- [ ] 14. Rebuild — blocked on step 2
- [ ] 15. Flash again — blocked on physical hardware
- [ ] 16. Verify the modification is visible on the router — blocked on physical hardware

### What was actually verified this session (real, against real upstream source)

No shortcuts here — every item below was checked against a real, network-fetched
clone of `RMerl/asuswrt-merlin.ng` (cleaned up afterward; none of this is
committed as vendored source — see `firmware/patches/README.md` for why the
Merlin tree itself isn't vendored into this repo):

- **Network access confirmed.** `git ls-remote`/`git clone`/`git fetch` to
  `github.com` all work from this environment (through the pre-configured
  egress proxy) — a plain `curl` HEAD request gets blocked, but git's own
  protocol goes through fine.
- **Pinned tag confirmed to exist**: `3004.388.9` (resolves to commit
  `bab3cb030c`, "Bumped revision to 3004.388.9 final").
- **Found and fixed a real bug in `firmware/build.sh`**: the build target
  was `make gtax6000` (no dash) — invalid. The correct target is
  `make gt-ax6000` (confirmed two ways: `chip_profile.mak` defines
  `GT-AX6000_CHIP_PROFILE=4912`, matched via the Makefile's
  `$(MAKECMDGOALS)_CHIP_PROFILE` mechanism only when the goal is literally
  `gt-ax6000`; and the upstream project's own multi-model build automation,
  `tools/build-all`, calls exactly `cd release/src-rt-5.04axhnd.675x && make
  "$FWMODEL"` with `FWMODEL="gt-ax6000"` for this device). Fixed in
  `firmware/build.sh`.
- **Confirmed the build subdirectory** (`release/src-rt-5.04axhnd.675x`) and
  **the built-image naming pattern** (`*_nand_squashfs.pkgtb` in that
  directory's `image/` subdir) against the same `tools/build-all` script —
  `firmware/build.sh`'s output-detection now checks there first.
- **Measured real disk requirements** instead of guessing: `asuswrt-merlin.ng`
  at the pinned commit is ~11 GB (shallow), `am-toolchains` at its pinned
  commit is ~3.1 GB (shallow) — ~14 GB for source alone, both fetched for
  real, inspected, then cleaned up locally (nothing vendored — see below).
  That comfortably fit this environment's 30 GB disk allowance with ~16 GB
  left over for the Docker build itself, which has *not* been attempted —
  whether that headroom is enough for the actual build's object files and
  output is still unverified. `docs/build-environment.md` now states ~20 GB
  for source (measured) and ~60 GB recommended overall (build headroom,
  still an estimate).
- **Wrote and verified the step-13 patch** against the real pinned-tag
  source (see step 13 above).
- **Pinned both sources as git submodules**, not left as "a script will
  clone them": `firmware/src/asuswrt-merlin.ng` and
  `firmware/src/am-toolchains` are gitlinks in this repo's own tree,
  pointing at the exact verified commits above (`.gitmodules` at the repo
  root). This is a real, visible link — GitHub renders a submodule entry as
  a clickable jump straight to that commit on the upstream repo — not a
  promise inside a shell script's default variable. Creating a full fork or
  a brand-new mirror repository was attempted first and is **not possible
  with this session's GitHub permissions** (the GitHub App installation
  used here can modify `reece01-dock/axos` but cannot fork a
  differently-owned repo or create new repositories — both attempts
  returned explicit permission errors, not silently skipped). The submodule
  pin achieves the same practical goal — an exact, GitHub-visible,
  version-controlled reference to the real source — without needing either.
  `firmware/setup-sources.sh` now just runs `git submodule update --init`
  against these pins rather than maintaining its own separate clone/ref logic.

**Not done, and not possible from this environment:** the actual Docker
build step — and now confirmed *why*, not just "disk headroom is
unverified": this session's network egress policy blocks
`production.cloudfront.docker.com`, the CDN Docker Hub redirects to for
actual image layer blobs. `docker pull ubuntu:20.04` (the very first line
of `build.sh`, before anything AXOS-specific even runs) fails with a 403.
Confirmed twice — once via a direct `dockerd` (no proxy env vars, got a
403 straight from CloudFront) and once with `dockerd` explicitly pointed
at this session's egress proxy (the proxy's own status log recorded
`connect_rejected ... policy denial` for that exact host) — so this isn't
a retry-able transient failure or something a Dockerfile/build.sh change
can route around; it's a hard block on this environment. Per this
environment's own proxy documentation, a 403/407 policy denial is
something to report, not bypass.

### First real build attempt outside this sandbox (on a real VM, real internet)

The user ran `firmware/setup-sources.sh && ./firmware/build.sh` on an actual
Ubuntu Server VM with unrestricted internet — confirming the sandbox's
Docker Hub block above doesn't apply to a normal machine. The build got
substantially further than anything run in this project's own sandboxed
environment ever could: past `docker build`, past mounting the toolchains,
into the actual HND SDK build (compiled BusyBox, ran the CFE/wireless
firmware sysdeps copy steps for the GT-AX6000 profile), before hitting a
real bug:

```
ERROR: /bin/sh does not invoke bash shell
make[2]: *** [.../make.common:3450: prebuild_checks] Error 1
```

**Root cause, confirmed against the real error output**: Ubuntu's `/bin/sh`
is `dash` by default — installing the `bash` package (which
`firmware/docker/Dockerfile` already did) doesn't change that. The HND
SDK's `make.common` has an explicit `prebuild_checks` target that requires
`/bin/sh` to actually invoke bash and fails the whole build otherwise.
**Fixed** in `firmware/docker/Dockerfile`: `RUN ln -sf /bin/bash /bin/sh`
forces the symlink.

That fix got the next attempt past `prebuild_checks`' shell check into its
header/library checks, where it hit a second real bug:

```
fatal error: lzo/lzo1x.h: No such file or directory
ERROR: lzo/lzo1x.h development library is required for build
```

**Fixed** the same way, `liblzo2-dev` added to the Dockerfile's package
list. This time verified against the actual `prebuild_checks` target
source (`make.common`, checked out for real in `firmware/src/`) rather than
just the one failing header: read the whole target end to end and confirmed
every other header/pkg-config check it makes (`uuid/uuid.h`, `pkg-config
zlib`, `pkg-config uuid`) is already covered by packages already in the
Dockerfile (`uuid-dev`, `zlib1g-dev`) — so this should be the last
host-package gap in `prebuild_checks` specifically, though the checks after
it (kernel build, per-profile object build) are a different category and
unverified.

That fix revealed a third, different-*kind* of bug on the next attempt:

```
Package zlib was not found in the pkg-config search path.
ERROR: pkg-config zlib failed
```

— despite `zlib1g-dev` being installed. **Root cause wasn't a missing
package at all**: `firmware/docker/Dockerfile` put the cross-toolchain's
`usr/bin` directories ahead of system `PATH`, and both toolchains bundle
their *own* `pkg-config` binary (confirmed directly:
`crosstools-{aarch64,arm}-gcc-5.5-.../usr/bin/pkg-config` both exist) —
scoped to their own sysroot, so `pkg-config --exists zlib` was silently
running the wrong binary and never saw the system's zlib.pc. **Fixed** by
reordering `PATH` so system tools resolve first (`$PATH:<toolchain
dirs>` instead of `<toolchain dirs>:$PATH`) — confirmed safe by checking
`make.common`: the actual cross-compiler is always invoked via
`$(TOOLCHAIN_TOP)/bin/$(TOOLCHAIN_PREFIX)-gcc`, a fully-qualified path,
never a bare `gcc`/etc. resolved from `PATH`, so this reorder can't affect
which compiler the build actually uses.

That fix got the next attempt substantially further — past `prebuild_checks`
entirely and into the real kernel build (`olddefconfig` against the actual
4.19 kernel tree for this profile) — before hitting a fourth real bug:

```
.../crosstools-aarch64-gcc-9.2-.../cc1: error while loading shared
libraries: libisl.so.15: cannot open shared object file
```

**Root cause, confirmed directly against the real toolchain files**: the
crosstools bundle their own `libisl`/`libmpc`/`libmpfr`/`libgmp`/etc. under
each toolchain's own `lib/` dir — `libisl.so.15` genuinely exists right
there — but `cc1`'s baked-in `RPATH` (`readelf -d cc1`) points at the
*original build machine's* path
(`/home/defjovi/temp3/toolchain/crosstools-.../lib`), not wherever we
mount the toolchain, so the dynamic linker never finds it. **Fixed**: a
static `/etc/ld.so.conf.d/am-toolchains.conf` (added in
`firmware/docker/Dockerfile`) listing every `crosstools-*` lib dir in the
pinned `am-toolchains` commit (enumerated directly from the real checkout,
not guessed — includes the two gcc-5.3 toolchains' `usr/lib` layout,
different from the rest), plus a `sudo ldconfig` re-run in
`firmware/build.sh` right after the toolchains volume is mounted (the
`.conf` file's paths don't exist yet at `docker build` time, only at
`docker run` time, so the cache has to be rebuilt then, not baked into the
image). Verified the Dockerfile itself still parses/builds correctly up to
the same known network-blocked base-image pull from earlier entries in
this log — the RUN steps added here have correct syntax, at minimum.

That fix got the next attempt into the real kernel build proper (HOSTCC of
kernel build scripts, `kernel/bounds.s`) before a fifth, much simpler bug:

```
/bin/sh: bc: command not found
make[5]: *** [Kbuild:42: include/generated/timeconst.h] Error 127
```

Plain missing package — `bc` is a standard Linux kernel build dependency
(used to compute `include/generated/timeconst.h`) that was never in the
Dockerfile's list. **Fixed**: added `bc`.

That fix got the next attempt past the kernel build entirely and into
router userspace — linking `libshared.so` — before a sixth bug, a
different kind again:

```
arm-buildroot-linux-gnueabi-gcc.br_real: error: prebuild/uu_utils.o: No such file or directory
make[5]: *** [Makefile:481: libshared.so] Error 1
```

**Traced in the real source**: `release/src/router/shared/Makefile:472`
pulls in `prebuild/uu_utils.o` (a proprietary ASUS "UU/GearUp cloud
plugin" binary blob, unrelated to core networking) whenever
`RTCONFIG_UUPLUGIN` or `RTCONFIG_GEARUPPLUGIN` is `y`. Confirmed: this
Merlin release's `release/src/router/shared/prebuild/GT-AX6000/` directory
genuinely does not ship `uu_utils.o` (it has 16 *other* prebuilt objects —
that one exists only for RT-AX86U/RT-AX58U/RT-AX68U/RT-AX88U/GT-AX11000).
Also confirmed both Kconfig sources of truth
(`release/src/router/config/config.in`, `config_base`) default this
feature **off**, and GT-AX6000's own config fragment
(`targets/94912GW/94912GW.GT-AX6000`) never turns it on — so something
deeper in the generated `.config` state (not staticly traceable from the
source alone, possibly leftover state from an earlier attempt, since the
build tree persists across retries) is enabling a feature whose binary
this model's source doesn't even ship.

**Fixed**: `firmware/build.sh` now passes `RTCONFIG_UUPLUGIN=n
RTCONFIG_GEARUPPLUGIN=n` as `make` command-line overrides — confirmed no
`override` directive anywhere in the relevant Makefiles that would defeat
this, so it reliably wins regardless of the exact root cause. Not yet
re-verified end to end.

Everything requiring physical hardware is separately blocked as before.
The concrete next step is to run
`firmware/setup-sources.sh && APPLY_PATCHES=1 ./firmware/build.sh` on a
**real machine outside this sandbox** (a Linux box or VM with normal,
unrestricted internet access — the disk-space math above still applies:
~20GB for source, ~60GB recommended overall) — the build-target fix above
should make the build itself succeed once it can actually pull its base
image, then work through steps 3–16 with the physical router.

## Milestone 2 — First AXOS control service (`axosd`) + MCP

Everything in this milestone is `[s]` (sandbox-verified — see the legend
above): code complete, 99 automated tests, and manually driven as real
compiled binaries against each other. Checking `[x]` requires the same
things confirmed **on the router**.

- [s] `axosd`/`axos-mcp`/`axosctl` cross-compile cleanly for aarch64
- [ ] Runs on the router from USB; starts at boot via `services-start` hook
      *(scripted: `scripts/router/`)*
- [s] System info (model, firmware, uptime)
- [s] CPU / RAM / temperature
- [s] Interfaces (state, role, addresses, counters)
- [s] Routing tables
- [s] Connected clients (DHCP leases, ARP, Wi-Fi assoc list)
- [s] Wi-Fi information (radios, channels, clients, RSSI)
- [s] Router-native services, firewall rules, VPN tunnel status, full nvram
      dump — added beyond the original milestone scope, needed for capture/replay
- [s] `system.shell_exec` (root shell, audited)
- [s] Config backup / restore
- [s] Audit logging of every mutating call
- [s] Rollback engine: `rollback.arm(seconds)` / `rollback.confirm()` with
      automatic restore on timeout, confirmed to survive `axos-mcp`
      restarting mid-transaction (the reason the Core API architecture
      exists at all — see `docs/development.md`)
- [s] All of the above exposed over MCP and driven end-to-end (real
      `axos-mcp`/`axosctl` binaries against a real `axosd`, not just tests)
- [x] Audit log and backup files/dirs are owner-only (0600/0700), enforced on
      every write, not just at creation (see `docs/security.md` "Secrets at
      rest") — filesystem-permission behavior, not hardware-dependent
- [x] Backup restore refuses a checksum-mismatched (tampered/corrupted)
      backup — same reasoning, not hardware-dependent
- [ ] SSH password auth disabled on the router, key-only access confirmed
      (`docs/security.md` "MCP transport & access control" — this is the
      actual authentication perimeter for the whole platform)
- [ ] `internal/backend/asuswrt`'s `(verify)`-marked assumptions checked
      against a real router's actual output (nvram keys, interface names,
      `wl`/`iptables`/`wg` formats) — the concrete next step once a
      GT-AX6000 is reachable; `axosctl capture --backend asuswrt --host
      <router>` is what would surface the mismatches

### Hot-deployable development platform (the "no reboot for normal dev" requirement)

Detailed status of the phases behind Milestone 2's `[s]` marks above — see
`docs/development.md` for the full write-up of each.

- [s] **Phase 1** — `RouterBackend` abstraction, `MockBackend` (mutable),
      CLI (`axosctl`), basic daemon (`axosd`)
- [s] **Phase 2** — Capture tooling (`internal/capture`, secret redaction
      enforced in code), `ReplayBackend`, sanitized fixtures (round-tripped
      through mock in tests; a real `testdata/gt-ax6000/` capture is not
      done — no reachable router)
- [s] **Phase 3** — `AsuswrtBackend` — nvram/interface/route discovery,
      service control, SSH transport (`WithHost`) *(all `(verify)` against
      real hardware, per above)*
- [s] **Phase 4** — Atomic releases (`internal/deploy`): staging → 
      releases/NNNNNN → current/previous, checksum-verified, instant rollback
- [s] **Phase 5** — Cross-compilation + deploy pipeline (`axosctl deploy`),
      supervisor-mediated restart (`internal/supervisor`), health checks
- [s] **Phase 6** — MCP as an independent process (`axos-mcp`), hot
      restart proven to preserve rollback state, local dev mode (point
      `axos-mcp`/`axosctl` at a router over an SSH-tunneled API port)
- [ ] **Phase 7** — Web UI dev server + hot static asset deployment — not
      started; no web UI code exists yet (see "Later / continuous" below)
- [s] **Phase 8** — Network transaction layer + timed rollback watchdog —
      this is `internal/rollback` + `internal/rollbackctl`, built in
      Milestone 2 rather than as a separate later phase, since the MCP
      tool surface already needed it from the start
- [ ] **Phase 9** — Integrate the stable AXOS bootstrap into the Merlin
      firmware fork — blocked on Milestone 1 (a working build/flash loop)
      existing first

## Milestone 3 — VPN, routing, firewall, DNS, QoS

- [ ] WireGuard profile management (create/import, up/down, health check)
- [ ] Cloudflare WARP (WireGuard profile via warp registration)
- [ ] OpenVPN client management
- [ ] Policy routing engine: per-device (MAC/IP) → WAN/VPN table, kill switches
- [ ] Bypass rules (device stays on WAN)
- [ ] VPN endpoint latency benchmarking + automatic endpoint selection
- [ ] DNS configuration (upstreams, per-device DNS, DoT)
- [ ] DHCP reservations / options
- [ ] Firewall rule management (audited, transactional)
- [ ] QoS inspection and control
- [ ] Diagnostics: ping/traceroute/DNS lookup/port check from the router
- [ ] Performance testing: iperf3 server/client, WAN speed test, loaded latency
- [ ] Dedicated VPN secrets store, separate from general config backups
      (`docs/security.md` "Secrets at rest")

## Milestone 4 — Optimisation loops

Every optimiser follows: OBSERVE → BASELINE → CHANGE ONE THING → TEST → COMPARE →
KEEP OR REVERT → CONTINUE. No unbenchmarked "tuning".

- [ ] Ethernet optimiser (IRQ/CPU affinity, buffers, conntrack, offload state —
      never disabling Broadcom flow acceleration accidentally)
- [ ] Wi-Fi optimiser (channel scan, utilisation, candidate configs, A/B compare)
- [ ] VPN optimiser (endpoint selection, MTU, fast paths)
- [ ] Latency optimiser (loaded-latency driven)
- [ ] AXOS Performance Mode (orchestrates the above with rollback arming)

## Later / continuous

- [ ] Custom AXOS web UI sections (same backend as MCP — never a second control system;
      security requirements checklist in `docs/security.md` "Future: web UI security requirements")
- [ ] Package/module system for optional functionality
- [ ] Historical metrics on USB storage (never internal flash)
- [ ] Backup encryption at rest, once a key-management approach is decided
      (`docs/security.md` "Secrets at rest")
- [ ] Firmware image signing / verified `system.update` before flashing
      anything AI-selected (`docs/security.md` "Firmware & build integrity")
- [ ] Firmware-integrated axosd (built into the image rather than USB-installed)
- [ ] `OpenWrtBackend` / other router support via the `RouterBackend` interface
