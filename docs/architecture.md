# AXOS architecture

## Layers

```
┌────────────────────────────────────────────────────┐
│  Frontends: axos-mcp │ axosctl │ (future) Web UI    │   thin HTTP clients,
├────────────────────────────────────────────────────┤   no logic of their own
│  axosd — the Core API (internal/api, HTTP/JSON)     │
│   · RouterBackend (typed interface)                 │
│   · rollback engine + rollbackctl.Local composition  │
│   · audit log                                       │
│   · supervisor (hot-deployable sibling services)    │
│   · policy routing engine (M3) / VPN manager (M3)    │
│   · optimisers (M4)                                  │
├────────────────────────────────────────────────────┤
│  Backend implementations                             │
│   · AsuswrtBackend (nvram, rc, wl, iptables, wg —    │
│     local exec or SSH via WithHost)                  │
│   · MockBackend    (dev & tests, mutable)            │
│   · ReplayBackend  (serves a captured fixture set)   │
│   · httpclient     (talks to another axosd's API)    │
├────────────────────────────────────────────────────┤
│  Forked Asuswrt-Merlin firmware                       │
├────────────────────────────────────────────────────┤
│  Broadcom proprietary blobs (kept as-is)              │
├────────────────────────────────────────────────────┤
│  GT-AX6000 hardware                                   │
└────────────────────────────────────────────────────┘
```

**One control plane.** `axos-mcp`, `axosctl`, and the future web UI all call
the same `RouterBackend` methods, reached through `axosd`'s Core API rather
than embedded directly. Nothing user-facing shells out on its own; if a
frontend needs a capability, it gets added to the backend interface (and the
API surface) first. The single deliberate escape hatch is
`system.shell_exec`, which is part of the API and is audited like
everything else.

**Why HTTP instead of a shared in-process `RouterBackend`:** so any frontend
can be killed and restarted — to pick up a rebuild, or just because it
crashed — without disturbing `axosd`'s own state: an armed rollback timer,
the audit log's file handle, any supervised child processes. See
`docs/development.md` and `internal/rollbackctl`'s package doc for the full
reasoning; `internal/mcp/integration_httpclient_test.go` is the automated
proof that a rollback transaction armed by one `axos-mcp` process survives
that process being discarded and a fresh one built against the same
`axosd`.

## The binaries (Go)

Static binaries (`GOOS=linux GOARCH=arm64` for the router; cross-compiles
instantly from any dev machine), because:

- one file to deploy per component, no interpreter or shared-lib
  dependencies on the router;
- goroutines fit the monitoring/health-check/rollback-timer/supervisor
  workload;
- typed interfaces (`RouterBackend`, `rollbackctl.Controller`) are a core
  project requirement, and Go's interfaces make the "same code, different
  transport" pattern (in-process vs. over HTTP) cheap to keep honest.

```
cmd/axosd/       serve (HTTP Core API — the normal way to run it) or
                 mcp (single-process MCP, no axos-mcp needed — legacy/simple mode)
cmd/axos-mcp/    MCP over stdio, as an independent process — a thin
                 httpclient.Client wrapped in internal/mcp.Server
cmd/axosctl/     the control CLI (status/health/deploy/rollback/restart/
                 logs/capture/transaction/dev watch)

internal/backend/            RouterBackend interface + shared types
internal/backend/mock/       in-memory fake, mutable at runtime (state.go)
internal/backend/replay/     serves a captured fixture directory
internal/backend/asuswrt/    real implementation — local exec or SSH (ssh.go)
internal/backend/httpclient/ RouterBackend + rollbackctl.Controller over HTTP
internal/backendselect/      the one place "--backend X" flags become a backend

internal/api/          the Core API itself: RouterBackend's methods as
                        routes, plus rollback and supervised-service control
internal/rollback/      arm/confirm/auto-revert engine (the one real
                         instance lives in whichever process calls
                         internal/api.NewServer — normally axosd)
internal/rollbackctl/   the Controller interface frontends use instead of
                         touching rollback.Engine directly — Local (in-process
                         composition) or httpclient.Client (remote) implement it
internal/audit/          append-only JSONL audit log

internal/deploy/         atomic release layout: staging/ -> releases/NNNNNN/
                          -> current/previous symlinks, checksum-verified
internal/supervisor/     process supervision for hot-deployable services
internal/svcconfig/      loads which services axosd should supervise

internal/capture/        query any RouterBackend, sanitize, write fixtures
```

### Deployment model (phased)

1. **M2 — USB sideload**: `axosd` (and, once real Milestone-3+ daemons
   exist, whatever it supervises) lives on the USB SSD
   (`/mnt/<label>/axos/...`), started by Merlin's `services-start` user
   script. Zero firmware changes needed; iterate via `axosctl deploy` (see
   `docs/development.md`), never a reflash.
2. **Later — firmware-integrated**: a minimal, stable bootstrap that starts
   `axosd` gets folded into `firmware/patches/`, while the actual binaries
   and their mutable data keep living on USB and getting updated via the
   same hot-deploy path — the firmware-side piece should rarely need to
   change once it exists.

### Transport

`axosd serve` exposes the Core API over HTTP/JSON, bound to `127.0.0.1` by
default (`docs/security.md` "Network exposure defaults" — enforced as the
flag default, not just documented). `axos-mcp` and `axosctl` reach it
either locally (same machine, typical when both run on the router) or over
the network during development (dev PC → router, `docs/development.md`
"Live Development Mode"); either way the traffic should stay inside a
trusted LAN/SSH-tunnel boundary, not be exposed to the WAN.

`axosd mcp` (the single-process mode) still speaks MCP directly over
**stdio**, trivially bridged to an AI client over SSH
(`ssh router axosd mcp`) — useful for quick manual testing, but `axos-mcp`
talking to `axosd serve` is the normal path since it's the only one where
MCP can be restarted independently.

## RouterBackend interface

Defined in `internal/backend/backend.go`. Summary:

- `Info()` — model, firmware, uptime, serial
- `Resources()` — CPU load, memory, temperatures
- `Interfaces()` — name, type, role (wan/lan/...), state, MACs, IPs,
  counters, link speed
- `Routes(table)` — routing table entries
- `Clients()` — connected devices (DHCP + ARP + Wi-Fi assoc merged)
- `WiFiStatus()` — radios, channel, width, power, clients with RSSI/PHY rate
- `NVRAMDump()` — full nvram key/value set (the one place secrets live —
  see `docs/security.md`)
- `Services()` — known router-managed service running-state
- `FirewallRules()` — current packet-filter rules
- `VPNStatus()` — configured VPN tunnels and peers (never private keys)
- `ShellExec(cmd, timeout)` — root shell, audited
- `Backup(dest)` / `Restore(src)` / `ListBackups()` — config snapshot/restore
- Milestone 3 grows this further: policy routing, DNS, QoS, diagnostics.

Rules for the interface:

- **Typed results**, never raw command output (raw output is available via
  `ShellExec` when a human/AI wants it).
- **No ASUS-isms leak through**: `WiFiSetChannel(radio, ch, width)` — not
  "set nvram wl1_chanspec". The Asuswrt backend owns the nvram knowledge.
- Every **mutating** method goes through the audit log, and the dangerous ones
  through the rollback engine (see `docs/safety-rollback.md`).
- Every method must be implementable (even if only as a clear "not
  supported" error) by `mock`, `replay`, and `asuswrt` alike — see
  `docs/development.md`'s "no fake functionality" examples (`ShellExec`
  fails clearly in replay mode rather than fabricating output).

## Data placement

| Data | Location |
|---|---|
| axosd/axos-mcp/axosctl binaries, releases | USB SSD (M2), firmware image (later) |
| Config snapshots/backups | USB SSD, rotated |
| Audit log | USB SSD, append-only JSONL |
| Supervised-service logs | USB SSD (`-service-log-dir`) |
| Metrics history | USB SSD (sqlite or similar) |
| Small persistent flags | JFFS (tiny, low write rate) |
| Never | chatty writes to internal NAND |
