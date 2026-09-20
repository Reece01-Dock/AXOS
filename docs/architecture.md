# AXOS architecture

## Layers

```
┌──────────────────────────────────────────────┐
│  Frontends: MCP server │ Web UI │ CLI        │   thin, no logic
├──────────────────────────────────────────────┤
│  axosd core services                         │
│   · RouterBackend API (typed interface)      │
│   · transaction/rollback engine              │
│   · audit log                                │
│   · monitoring collector                     │
│   · policy routing engine (M3)               │
│   · VPN manager (M3)                         │
│   · optimisers (M4)                          │
├──────────────────────────────────────────────┤
│  Backend implementations                     │
│   · AsuswrtBackend  (nvram, rc, wl, iptables,│
│     ip, fc/archer, ASUS services)            │
│   · MockBackend     (dev & tests)            │
│   · OpenWrtBackend  (future)                 │
├──────────────────────────────────────────────┤
│  Forked Asuswrt-Merlin firmware              │
├──────────────────────────────────────────────┤
│  Broadcom proprietary blobs (kept as-is)     │
├──────────────────────────────────────────────┤
│  GT-AX6000 hardware                          │
└──────────────────────────────────────────────┘
```

**One control plane.** MCP, web UI and CLI all call the same `RouterBackend`
methods inside `axosd`. Nothing user-facing shells out on its own; if a frontend
needs a capability, it gets added to the backend interface first. The single
deliberate escape hatch is `system.shell_exec`, which is part of the API and is
audited like everything else.

## axosd (Go)

A single static Go binary (`GOOS=linux GOARCH=arm64`), because:

- one file to deploy, no interpreter or shared-lib dependencies on the router;
- cross-compiles instantly from any dev machine;
- goroutines fit the monitoring/health-check/rollback-timer workload;
- typed interfaces (`RouterBackend`) are a core project requirement.

Layout (`axosd/`):

```
cmd/axosd/            main: flags, backend selection, serve MCP
internal/backend/     RouterBackend interface + shared types
internal/backend/mock/     in-memory fake, used by unit tests and --backend=mock
internal/backend/asuswrt/  real implementation (hardware-unverified until M2)
internal/mcp/         minimal MCP server: JSON-RPC 2.0 over stdio, tools/*
internal/rollback/    arm/confirm/auto-revert engine (unit-tested)
internal/audit/       append-only JSONL audit log
```

### Deployment model (phased)

1. **M2 — USB sideload**: `axosd` lives on the USB SSD
   (`/mnt/<label>/axos/axosd`), started by Merlin's `services-start` user script.
   Zero firmware changes needed; iterate fast.
2. **Later — firmware-integrated**: `axosd` built into the image via
   `firmware/patches/`, still storing mutable data on USB.

### MCP transport

`axosd mcp` speaks MCP (JSON-RPC 2.0) over **stdio** first — trivially bridged to
an AI client over SSH (`ssh router /path/axosd mcp`), which gives us
authentication and encryption for free via SSH keys. A network transport
(HTTP/SSE on the LAN with token auth) comes later; stdio-over-SSH is the
security-conservative default. See `docs/security.md` ("MCP transport &
access control") for why this makes SSH key management the actual security
perimeter of the whole platform, not an incidental detail.

## RouterBackend interface (v0, Milestone 2 scope)

Defined in `axosd/internal/backend/backend.go`. Summary:

- `Info()` — model, firmware, uptime, serial
- `Resources()` — CPU load, memory, temperatures
- `Interfaces()` — name, type, state, MACs, IPs, counters, link speed
- `Routes(table)` — routing table entries
- `Clients()` — connected devices (DHCP + ARP + Wi-Fi assoc merged)
- `WiFiStatus()` — radios, channel, width, power, clients with RSSI/PHY rate
- `ShellExec(cmd, timeout)` — root shell, audited
- `Backup(dest)` / `Restore(src)` — config snapshot/restore
- Milestone 3 grows this: VPN, PolicyRoute, DNS, Firewall, QoS, Diagnostics.

Rules for the interface:

- **Typed results**, never raw command output (raw output is available via
  `ShellExec` when a human/AI wants it).
- **No ASUS-isms leak through**: `WiFiSetChannel(radio, ch, width)` — not
  "set nvram wl1_chanspec". The Asuswrt backend owns the nvram knowledge.
- Every **mutating** method goes through the audit log, and the dangerous ones
  through the rollback engine (see `docs/safety-rollback.md`).

## Data placement

| Data | Location |
|---|---|
| axosd binary + assets | USB SSD (M2), firmware image (later) |
| Config snapshots/backups | USB SSD, rotated |
| Audit log | USB SSD, append-only JSONL |
| Metrics history | USB SSD (sqlite or similar) |
| Small persistent flags | JFFS (tiny, low write rate) |
| Never | chatty writes to internal NAND |
