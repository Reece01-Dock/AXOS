# MCP API specification

The AI is trusted as a full router administrator. Tools are namespaced, typed,
and all map onto the same `RouterBackend` used by the web UI and CLI.

Transport (M2): MCP over stdio (`axosd mcp`), typically bridged via SSH.
Protocol: MCP (JSON-RPC 2.0; `initialize`, `tools/list`, `tools/call`).

## Tool catalog

Status: ✅ implemented in axosd (mock-tested, hardware-unverified) ·
🔜 Milestone 3 · 🔮 Milestone 4+. Every mutating tool is audited; tools marked
**[danger]** require/arm a rollback transaction (see `docs/safety-rollback.md`).

### System

| Tool | Status | Notes |
|---|---|---|
| `system.info` | ✅ | model, firmware, uptime, serial |
| `system.resources` | ✅ | CPU load, memory, temperatures |
| `system.shell_exec` | ✅ | **[danger-lite]** unrestricted root shell with timeout; the escape hatch when no dedicated tool exists; fully audited (command, exit code, output hash) |
| `system.services` | 🔜 | list/restart rc services |
| `system.update` | 🔮 | firmware update flow (human confirmation required) |
| `system.packages` | 🔮 | optional module/package management |

### Network

| Tool | Status | Notes |
|---|---|---|
| `network.interfaces` | ✅ | state, addresses, counters, link speed |
| `network.routes` | ✅ | per routing table |
| `network.clients` | ✅ | DHCP + ARP + Wi-Fi assoc merged |
| `network.diag.ping` / `.traceroute` / `.dns_lookup` | 🔜 | from-router diagnostics |
| `network.perf.iperf3` / `.speedtest` / `.loaded_latency` | 🔜 | benchmarking |

### Wi-Fi

| Tool | Status | Notes |
|---|---|---|
| `wifi.status` | ✅ | radios, channels, clients, RSSI, PHY rates |
| `wifi.scan` | 🔜 | nearby APs, channel utilisation |
| `wifi.set_channel` | 🔜 | **[danger]** channel/width per radio |
| `wifi.set_txpower` | 🔜 | **[danger]** where supported |

### VPN (Milestone 3)

`vpn.list`, `vpn.profile.create/import/delete`, `vpn.up/down`,
`vpn.health`, `vpn.benchmark_endpoints`, `vpn.select_best_endpoint` —
WireGuard, OpenVPN, WARP (WireGuard profile via WARP registration), later
Tailscale. All **[danger]** where they touch routing.

### Policy routing (Milestone 3)

`route.policy.list`, `route.policy.set` (device/MAC/IP/subnet/port → wan|vpnX,
kill_switch bool), `route.policy.delete`. **[danger]**. This is what implements
"Put the TV and Xbox through WARP, keep the gaming PC on WAN".

### Firewall / DNS / DHCP / QoS (Milestone 3)

`firewall.rules.list/set/delete` **[danger]**, `dns.get/set` **[danger]**,
`dhcp.leases/reservations.set`, `qos.status/set`.

### Safety & state

| Tool | Status | Notes |
|---|---|---|
| `rollback.arm` | ✅ | `{timeout_seconds}` → transaction id |
| `rollback.confirm` | ✅ | `{id}` |
| `rollback.status` | ✅ | pending transaction, deadline |
| `config.backup` | ✅ | snapshot to USB, returns path + checksum |
| `config.restore` | ✅ | **[danger]** restore a named snapshot |
| `logs.read` | 🔜 | syslog / axosd audit log access |
| `metrics.query` | 🔮 | historical monitoring data |

## Conventions

- **Inputs/outputs are JSON schemas** declared in `tools/list`; no free-text
  parsing of router command output by the AI unless it chose `system.shell_exec`.
- **Errors** are structured: `{code, message, hint}` — e.g. arming while a
  transaction is pending returns `rollback_busy` with the pending id.
- **Audit**: every `tools/call` that mutates state logs
  `{ts, tool, args, caller, result, txn_id}` to the append-only audit log.
- **Danger enforcement**: [danger] tools fail with `rollback_required` unless a
  transaction is armed (or `arm_timeout_seconds` is passed to arm implicitly).
