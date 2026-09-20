# AXOS Roadmap

This file is the source of truth for project progress. A box is checked only when
the step has been **verified on real hardware or a real build machine** — not when
the code/scripts for it exist.

Legend: `[ ]` not done · `[x]` done and verified · `(scripted)` tooling exists in
this repo but has not been executed/verified yet.

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

Code for most of this exists in `axosd/` with unit tests against the mock backend.
Checkboxes refer to verification **on the router**.

- [ ] `axosd` cross-compiled for aarch64, runs on the router from USB
- [ ] Starts at boot via `services-start` hook *(scripted: `scripts/router/`)*
- [ ] System info (model, firmware, uptime)
- [ ] CPU / RAM / temperature
- [ ] Interfaces (state, addresses, counters)
- [ ] Routing tables
- [ ] Connected clients (DHCP leases, ARP, Wi-Fi assoc list)
- [ ] Wi-Fi information (radios, channels, clients, RSSI)
- [ ] `system.shell_exec` (root shell, audited)
- [ ] Config backup / restore
- [ ] Audit logging of every mutating call
- [ ] Rollback engine: `rollback.arm(seconds)` / `rollback.confirm()` with
      automatic restore on timeout *(implemented + unit-tested in `axosd/`)*
- [ ] All of the above exposed over MCP and driven end-to-end from an AI client
- [ ] Audit log and backup files/dirs are owner-only (0600/0700), enforced on
      every write, not just at creation *(implemented + unit-tested in
      `axosd/`; see `docs/security.md` "Secrets at rest")*
- [ ] Backup restore refuses a checksum-mismatched (tampered/corrupted)
      backup *(implemented + unit-tested in `axosd/`)*
- [ ] SSH password auth disabled on the router, key-only access confirmed
      (`docs/security.md` "MCP transport & access control" — this is the
      actual authentication perimeter for the whole platform)

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
