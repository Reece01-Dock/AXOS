# Safety & rollback design

Full root control for the AI is the point of AXOS — so the platform, not the AI's
judgement, must guarantee that a bad change cannot permanently sever management
access. This is a **fundamental platform feature**, not an add-on.

## The transaction pattern

Every dangerous change (firewall, routing, VPN, interface, Wi-Fi radio, DNS)
follows:

```
1. snapshot current state          (config backup + the specific subsystem state)
2. arm rollback                     rollback.arm(timeout_seconds)
3. apply the change
4. health checks:
     · management reachable (the arming client can still talk to axosd)
     · LAN forwarding OK
     · WAN reachable (ping gateway + external anchor)
     · DNS resolves
     · routing sane (default route present, no blackhole of br0)
5a. healthy → rollback.confirm()    keep the change, disarm
5b. unhealthy or no confirm before timeout → automatic restore of the snapshot
```

The critical property: **the confirm must come from outside the change**. If the
AI firewalled itself out, it *cannot* confirm, so the timer restores the snapshot
and management access returns. Losing connectivity is therefore always a
~`timeout`-bounded outage, never a lockout.

## Engine (implemented in `axosd/internal/rollback`)

- `Arm(id, timeout, restoreFn)` — stores a pending transaction with a restore
  function and starts a monotonic timer. Arming while another transaction is
  pending is **rejected** (one dangerous change at a time — "change one thing").
- `Confirm(id)` — cancels the timer, discards the snapshot, logs success.
- Timer expiry — runs `restoreFn`, logs an automatic revert with the reason.
- Everything is written to the audit log: arm, confirm, expiry, restore result.

### Surviving axosd crashes and reboots (M2 hardening, required before M3)

A timer inside a daemon dies with the daemon. Before AXOS is allowed to touch
firewall/routing on real hardware, the armed state must also exist **outside**
axosd:

- On `Arm`, axosd writes the snapshot + deadline to disk (USB) **and** installs a
  watchdog: a cron/`cru` entry (or init hook after reboot) that runs a tiny
  standalone restore script if the deadline passes and no confirm marker exists.
- `Confirm` removes the marker and the watchdog entry.
- If the router **reboots** mid-transaction, the init-time check finds the stale
  armed marker and restores the snapshot before services finish starting.

This mirrors how Merlin users protect themselves manually today
(schedule a `service restart_firewall` before touching rules), but automated and
audited.

### Change classes

| Class | Examples | Requirement |
|---|---|---|
| Read-only | status, metrics, scans | none |
| Reversible-soft | Wi-Fi channel, QoS params, DNS upstream | audit log; rollback recommended |
| Dangerous | firewall, routes, VPN policy, interface config | **must** arm rollback; refuse to apply if arming fails |
| Destructive | factory reset, firmware flash | explicit human confirmation + full backup verified readable |

The MCP tool layer enforces the class: tools that perform dangerous changes take
the transaction id from a prior `rollback.arm` (or arm implicitly) — there is no
code path that applies a dangerous change with no armed rollback.

## Snapshots & backups

- **Settings snapshot**: nvram export (same data as the web UI settings backup).
- **JFFS snapshot**: tarball of /jffs.
- **Subsystem snapshots** for fast restore: iptables-save output, `ip route/rule`
  dump, relevant nvram subset — restoring just these is seconds, vs. a full
  nvram restore + reboot.
- Rotation on USB: keep N most recent + one per day; never on internal flash.
- Periodic automatic backup (cron) independent of transactions.

## Firmware-level safety

- Recovery mode is sacred: nothing AXOS does may touch the bootloader.
- Before any firmware flash: settings + JFFS backup verified, stock image on
  hand, recovery procedure already exercised (`docs/flashing-and-recovery.md`).
- Later (nice to have): investigate the bootloader's dual-image behaviour on
  BCM4912 — if a fallback slot exists, integrate flash operations with it.
