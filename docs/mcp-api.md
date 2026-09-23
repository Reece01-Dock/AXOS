# MCP API specification

The AI is trusted as a full router administrator. Tools are namespaced, typed,
and all map onto the same `RouterBackend` used by the web UI and CLI.

Transport: `axos-mcp` (a standalone process — see `docs/development.md`)
speaks MCP over stdio and is itself a thin client of `axosd serve`'s Core
API over HTTP/loopback (or an SSH tunnel to it for dev-PC-to-router use —
see `docs/security.md` "MCP transport & access control"). `axosd mcp`
(single-process mode) speaks MCP directly over stdio with no separate
`axos-mcp` process, useful for quick manual testing. Protocol: MCP
(JSON-RPC 2.0; `initialize`, `tools/list`, `tools/call`).

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
| `system.services` | ✅ | running state of known router-managed services (read-only; *restart* individual native services is 🔜 Milestone 3 — not to be confused with `axosctl restart`, which restarts AXOS's own hot-deployable processes, see `docs/development.md`) |
| `system.nvram_dump` | ✅ | full nvram key/value dump; contains secrets, same trust boundary as the rest of this API (`docs/security.md`); not audited on read, matching the "only mutating calls are audited" convention |
| `system.update` | 🔮 | firmware update flow (human confirmation required) |
| `system.packages` | 🔮 | optional module/package management (`internal/pkgmod` registry skeleton) |

### Network

| Tool | Status | Notes |
|---|---|---|
| `network.interfaces` | ✅ | state, role (wan/lan/...), addresses, counters, link speed |
| `network.routes` | ✅ | per routing table |
| `network.clients` | ✅ | DHCP + ARP + Wi-Fi assoc merged |
| `network.firewall_rules` | ✅ | current packet-filter rules (read-only; mutation via `firewall.rules.set` / `.delete`) |
| `network.vpn_status` | ✅ | configured VPN tunnels + peers, read-only (never includes private keys); see `vpn.*` for mutation |
| `network.diag.ping` / `.traceroute` / `.dns_lookup` / `.port_check` | ✅ | from-router diagnostics |
| `network.perf.iperf3` | ✅ | iperf3 client/server from the router |
| `network.perf.speedtest` / `.loaded_latency` | 🔜 | additional benchmarking |

### Wi-Fi

| Tool | Status | Notes |
|---|---|---|
| `wifi.status` | ✅ | radios, channels, clients, RSSI, PHY rates |
| `wifi.scan` | 🔜 | nearby APs, channel utilisation |
| `wifi.set_channel` | 🔜 | **[danger]** channel/width per radio |
| `wifi.set_txpower` | 🔜 | **[danger]** where supported |

### VPN (Milestone 3)

| Tool | Status | Notes |
|---|---|---|
| `vpn.list` | ✅ | configured profile slots (no private keys) |
| `vpn.wireguard.import` | ✅ | **[danger]** import WG client into a Merlin slot |
| `vpn.up` / `vpn.down` | ✅ | **[danger]** bring a named profile up/down |
| `vpn.benchmark_endpoints` | ✅ | rank hosts by from-router ping (`POST /v1/vpn/benchmark`); no profile mutation |
| `vpn.profile.create/delete`, `vpn.health`, `vpn.select_best_endpoint` | 🔜 | remaining VPN surface |

### Policy routing (Milestone 3)

| Tool | Status | Notes |
|---|---|---|
| `route.policy.list` | ✅ | device → WAN/VPN steering |
| `route.policy.set` | ✅ | **[danger]** |
| `route.policy.delete` | ✅ | **[danger]** |

### Firewall / DNS / DHCP / QoS (Milestone 3)

| Tool | Status | Notes |
|---|---|---|
| `firewall.rules.set` / `.delete` | ✅ | **[danger]** map to FirewallApply / FirewallDelete |
| `dns.get` | ✅ | WAN/LAN upstreams + DoT |
| `dns.set` | ✅ | **[danger]** |
| `dhcp.reservations` | ✅ | list static mappings |
| `dhcp.reservations.set` / `.delete` | ✅ | **[danger]** |
| `dhcp.leases` | 🔜 | live lease table (clients cover much of this today) |
| `qos.status` | ✅ | Adaptive QoS / Cake summary |
| `qos.set` | ✅ | **[danger]** enable/disable |

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
