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
| `docs/` | Roadmap, architecture, hot-deploy dev workflow, build guide, flashing & recovery, MCP API, safety design, security model |
| `firmware/` | Reproducible Merlin build environment (Docker), source fetch + build scripts, patches |
| `cmd/axosd/` | Core service daemon: owns the RouterBackend, rollback engine, and audit log; exposes them via `serve` (HTTP Core API — the normal way to run it) or `mcp` (single-process MCP, no separate axos-mcp needed) |
| `cmd/axos-mcp/` | MCP frontend as an **independent process** — a thin client of axosd's Core API, restartable without touching axosd's state (armed rollback timers, audit log) |
| `cmd/axosctl/` | The control CLI: `status`, `health`, `deploy`, `rollback`, `restart`, `logs`, `capture`, `transaction`, and the `dev deploy/watch/restart/test` hot-development loop |
| `internal/backend/` | `RouterBackend` interface + implementations: `mock` (in-memory, dev/test), `replay` (serves captured fixtures), `asuswrt` (real router, local or SSH), `httpclient` (talks to axosd's Core API) |
| `internal/api/` | The Core API itself (HTTP/JSON) — what `axos-mcp` and `axosctl` actually talk to |
| `internal/deploy/` | Atomic release layout: `staging/` → `releases/NNNNNN/` → `current`/`previous` symlinks, checksum-verified |
| `internal/supervisor/` | Process supervision for hot-deployable services: start/stop/restart, health checks, crash-restart with backoff |
| `internal/capture/`, `internal/rollback/`, `internal/rollbackctl/` | Router-state capture/sanitization; the arm/confirm/auto-revert safety engine and its frontend-facing interface |
| `scripts/deploy-router.sh` | Build+test locally, deploy to a real router over SSH (untested against real hardware — see the script's own header) |
| `scripts/build-firmware.sh` | Thin wrapper for the firmware build (delegates to `firmware/build.sh`) |
| `scripts/router/` | Helpers that run on the router itself (install, service hooks) |

## Architecture (short version)

```
GT-AX6000 hardware
  └─ Broadcom binary blobs (Wi-Fi, flow acceleration — kept, never replaced)
      └─ Forked Asuswrt-Merlin firmware   (release/integration track)
          └─ axosd — AXOS Core API (HTTP/JSON), owns RouterBackend + rollback + audit
              └─ axos-mcp / axosctl / (future) Web UI — independent processes,
                 each a thin client of the Core API, hot-restartable on their own
```

There is exactly **one** control plane: `axosd`'s Core API. `axos-mcp`, `axosctl`,
and the future web UI are all thin frontends over the same `RouterBackend`
interface, reached over HTTP rather than embedded directly — which is what lets
any of them be rebuilt and restarted without disturbing axosd's own in-flight
state. See [`docs/development.md`](docs/development.md) for the day-to-day
hot-deploy workflow this enables, and [`docs/architecture.md`](docs/architecture.md)
for the full design.

## Current status

**Milestone 1 (build & flash proof) — not started on real hardware.** Nothing
here has been flashed to a router yet. The firmware build scripts
(`firmware/setup-sources.sh`, `firmware/build.sh`) are written but have not been
run end-to-end on a build machine, and no image has been verified on a real
GT-AX6000 — recovery mode must be tested first regardless (see
`docs/flashing-and-recovery.md`).

**Milestone 2 (core service) — code complete and verified end-to-end against
the mock/replay backends and real compiled binaries; untested against real
hardware.** The whole module builds cleanly for the host and for
`GOOS=linux GOARCH=arm64` (static binaries, no runtime dependencies), with
99 automated tests passing. Beyond the unit tests, the actual compiled
`axosd`/`axos-mcp`/`axosctl` binaries have been run against each other as
real, separate OS processes and confirmed to: serve the Core API and answer
router-status calls; run `axos-mcp` as a genuinely independent process
against it (including proving an armed rollback transaction survives
`axos-mcp` restarting — the whole reason the Core API exists); stage, deploy,
and roll back real release directories with `axosctl`; and capture a
sanitized fixture set from the mock backend with the Wi-Fi passphrase
confirmed redacted on disk. The `asuswrt` backend
(`internal/backend/asuswrt`) is implemented against documented
Asuswrt-Merlin conventions, including an SSH transport for driving a remote
router, but every hardware-specific assumption in it — nvram key names,
interface naming, `wl`/`iptables`/`wg` output parsing — is marked
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
8. **Firmware flashing is a release step, not the development loop.** Editing
   MCP tools, API logic, VPN/firewall/routing/DNS handling, or anything else
   that lives in `axosd`/`axos-mcp`/`axosctl` should never require a reboot —
   see [`docs/development.md`](docs/development.md) for the
   edit → build → test → deploy → restart-one-service → health-check loop
   this repo is built around.

## Licensing note

Asuswrt-Merlin is a mix of GPL code and proprietary ASUS/Broadcom components.
The Merlin fork (when created under `firmware/`) inherits those licences and their
redistribution restrictions — in particular, **firmware images containing Broadcom
proprietary components must not be redistributed**; they are for our own devices.
Original AXOS code in this repository (`cmd/`, `internal/`, `scripts/`) is ours.
