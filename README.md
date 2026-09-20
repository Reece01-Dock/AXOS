# AXOS

**AXOS** is a custom router firmware platform for the **ASUS ROG Rapture GT-AX6000**,
built as a fork of [Asuswrt-Merlin](https://github.com/RMerl/asuswrt-merlin.ng).

The end goal: a fully programmable, AI-controlled router. An MCP-connected AI has
full administrator control — VPN management, per-device policy routing, firewall,
DNS, Wi-Fi/Ethernet optimisation, monitoring, diagnostics — protected by
snapshot/rollback safety mechanisms so a bad change can never permanently brick
connectivity.

## Project layout

| Path | Purpose |
|---|---|
| `docs/` | Roadmap, architecture, build guide, flashing & recovery, MCP API, safety design, security model |
| `firmware/` | Reproducible Merlin build environment (Docker), source fetch + build scripts, patches |
| `axosd/` | AXOS core service daemon (Go): shared router API, MCP server, rollback engine, audit log |
| `scripts/router/` | Helpers that run on the router itself (install, service hooks) |

## Architecture (short version)

```
GT-AX6000 hardware
  └─ Broadcom binary blobs (Wi-Fi, flow acceleration — kept, never replaced)
      └─ Forked Asuswrt-Merlin firmware
          └─ axosd — AXOS core services (single static Go binary)
              └─ shared RouterBackend API
                  └─ MCP server / Web UI / CLI  (one backend, three frontends)
```

There is exactly **one** control plane: `axosd`. The MCP server, the future web UI
and the CLI are all thin frontends over the same `RouterBackend` interface. The
`AsuswrtBackend` implementation talks to nvram, rc services, iptables, `wl`, and
Broadcom tools; a `MockBackend` exists for development and tests; other backends
(OpenWrt, GL.iNet) can be added later without touching the frontends.

See [`docs/architecture.md`](docs/architecture.md).

## Current status

**Milestone 1 (build & flash proof) — not started on real hardware.** Nothing
here has been flashed to a router yet. The firmware build scripts
(`firmware/setup-sources.sh`, `firmware/build.sh`) are written but have not been
run end-to-end on a build machine, and no image has been verified on a real
GT-AX6000 — recovery mode must be tested first regardless (see
`docs/flashing-and-recovery.md`).

**Milestone 2 (axosd core service) — code complete against the mock backend,
untested on hardware.** `axosd` (`axosd/`) builds cleanly for both the host
and `GOOS=linux GOARCH=arm64` (a static binary with no runtime dependencies),
and its full test suite (17 tests: rollback arm/confirm/auto-revert including
the "AI never confirms → automatic restore" property, audit logging, and the
MCP tool surface end-to-end) passes against the mock backend. The `asuswrt`
backend (`axosd/internal/backend/asuswrt`) is implemented against documented
Asuswrt-Merlin conventions but every hardware-specific assumption in it —
nvram key names, interface naming, `wl` output parsing — is marked
`(verify)` and has not been checked against a real router.

See [`docs/ROADMAP.md`](docs/ROADMAP.md) for the milestone checklists — that file
is the source of truth for what is actually verified vs. merely written.

## Development principles

1. **Prove the build/flash/recover loop before anything else.** No AXOS features
   ship until we can build stock Merlin, flash it, and recover from a bad flash.
2. **Never break recovery paths.** The bootloader rescue mode and factory reset
   must always work.
3. **Keep Broadcom blobs.** Wi-Fi drivers, flow cache / runner acceleration stay
   as shipped. We build around them.
4. **Measure, don't guess.** Every "optimisation" is a benchmarked before/after
   with automatic revert on regression.
5. **Transactional changes.** Dangerous config changes go through
   snapshot → arm rollback → apply → health-check → confirm-or-revert.
6. **No fake functionality.** A feature is either implemented and verified, or
   documented as not done. Docs mark hardware-unverified code explicitly.
7. **The security boundary is "who can reach `axosd`," not "what the AI is
   allowed to do."** The AI has full root by design — see
   [`docs/security.md`](docs/security.md) for the threat model this implies,
   what's actually protected today (owner-only permissions on every
   secret-bearing file, checksum-verified restores), and what's explicitly
   still a gap (encryption at rest, network-transport auth).

## Licensing note

Asuswrt-Merlin is a mix of GPL code and proprietary ASUS/Broadcom components.
The Merlin fork (when created under `firmware/`) inherits those licences and their
redistribution restrictions — in particular, **firmware images containing Broadcom
proprietary components must not be redistributed**; they are for our own devices.
Original AXOS code in this repository (`axosd/`, `scripts/`) is ours.
