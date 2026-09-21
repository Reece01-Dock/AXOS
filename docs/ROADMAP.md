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

- [ ] 1. Build Asuswrt-Merlin from source for GT-AX6000 *(scripted: `firmware/`)*
- [ ] 2. Produce a firmware image (`.w` / `.pkgtb`)
- [ ] 3. Flash it via the stock ASUS web UI
- [ ] 4. Router boots
- [ ] 5. 1GbE Ethernet works
- [ ] 6. Both 2.5GbE ports work
- [ ] 7. 2.4 GHz Wi-Fi works
- [ ] 8. 5 GHz Wi-Fi works
- [ ] 9. ASUS web UI works
- [ ] 10. SSH works
- [ ] 11. Recovery mode verified (bootloader rescue + firmware restoration — do this
       **before** flashing anything custom; see `docs/flashing-and-recovery.md`)
- [ ] 12. Create and store a settings + JFFS backup
- [ ] 13. Make one harmless, visible source modification *(procedure documented in
       `firmware/patches/README.md`)*
- [ ] 14. Rebuild
- [ ] 15. Flash again
- [ ] 16. Verify the modification is visible on the router

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
