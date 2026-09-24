package backend

import "context"

// RouterBackend is the single typed interface every AXOS frontend (MCP, web
// UI, CLI) is built on. Implementations translate these calls into whatever
// the underlying platform needs (nvram, rc services, iptables, wl, ...) —
// callers never need to know which platform they're talking to.
//
// This is the Milestone 2 scope (docs/mcp-api.md "✅" rows). Milestone 3 adds
// VPN/routing/firewall/DNS/QoS/diagnostics methods to this same interface.
//
// Every implementation must be safe for concurrent use.
type RouterBackend interface {
	// Info returns static/semi-static identity information about the router.
	Info(ctx context.Context) (SystemInfo, error)

	// Resources returns live CPU/memory/temperature readings.
	Resources(ctx context.Context) (Resources, error)

	// Interfaces returns all known network interfaces and their state.
	Interfaces(ctx context.Context) ([]Interface, error)

	// Routes returns routing table entries. table == "" means the main table.
	Routes(ctx context.Context, table string) ([]Route, error)

	// Clients returns devices the router currently knows about (DHCP + ARP +
	// Wi-Fi association, merged).
	Clients(ctx context.Context) ([]Client, error)

	// WiFiStatus returns the state of all Wi-Fi radios.
	WiFiStatus(ctx context.Context) ([]WiFiRadio, error)

	// ShellExec runs an arbitrary command with root privileges and a timeout.
	// This is the unrestricted escape hatch (system.shell_exec) for cases with
	// no dedicated method — callers (the MCP layer) are responsible for
	// audit-logging every invocation.
	ShellExec(ctx context.Context, command string, timeoutSeconds int) (ShellResult, error)

	// Backup creates a configuration snapshot (settings + relevant subsystem
	// state) and returns metadata about it. reason is a short free-text label
	// (e.g. "pre-rollback", "manual", "scheduled").
	Backup(ctx context.Context, reason string) (BackupInfo, error)

	// Restore restores a previously created backup by its ID.
	Restore(ctx context.Context, backupID string) error

	// ListBackups returns known backups, newest first.
	ListBackups(ctx context.Context) ([]BackupInfo, error)

	// NVRAMDump returns the full nvram key/value set. This is the one place
	// secrets can appear (Wi-Fi passphrase, admin password, ...) — callers
	// that persist or transmit the result (internal/capture in particular)
	// must sanitize it. See docs/security.md "Secrets at rest".
	NVRAMDump(ctx context.Context) (map[string]string, error)

	// Services returns the running state of known router-managed services.
	Services(ctx context.Context) ([]ServiceStatus, error)

	// FirewallRules returns the current packet-filtering rule set.
	FirewallRules(ctx context.Context) ([]FirewallRule, error)

	// VPNStatus returns configured VPN tunnels and their peers (WireGuard,
	// OpenVPN, WARP). Never includes private keys — see VPNPeer.
	VPNStatus(ctx context.Context) ([]VPNTunnel, error)

	// --- Milestone 3 -------------------------------------------------------

	// Ping / Traceroute / DNSLookup / PortCheck run from-router diagnostics.
	Ping(ctx context.Context, host string, count int) (DiagResult, error)
	Traceroute(ctx context.Context, host string, maxHops int) (DiagResult, error)
	DNSLookup(ctx context.Context, name string) (DiagResult, error)
	PortCheck(ctx context.Context, host string, port int) (DiagResult, error)

	// Iperf3 runs a short bandwidth test (server or client mode).
	Iperf3(ctx context.Context, opts IperfOpts) (PerfResult, error)

	// DNSConfig / SetDNSConfig read and update DNS upstreams / DoT.
	DNSConfig(ctx context.Context) (DNSInfo, error)
	SetDNSConfig(ctx context.Context, cfg DNSInfo) error

	// DHCPReservations / SetDHCPReservation manage static DHCP mappings.
	DHCPReservations(ctx context.Context) ([]DHCPReservation, error)
	SetDHCPReservation(ctx context.Context, r DHCPReservation) error
	DeleteDHCPReservation(ctx context.Context, mac string) error

	// QoSStatus / SetQoSEnable inspect and toggle QoS.
	QoSStatus(ctx context.Context) (QoSInfo, error)
	SetQoSEnable(ctx context.Context, enabled bool) error

	// VPNProfiles lists configured profile slots (no private keys).
	VPNProfiles(ctx context.Context) ([]VPNProfile, error)
	// ImportWireGuard writes a WireGuard client profile into a Merlin slot.
	ImportWireGuard(ctx context.Context, p WireGuardImport) error
	// VPNUp / VPNDown bring a named profile (e.g. "wgc1", "ovpnc1") up/down.
	VPNUp(ctx context.Context, name string) error
	VPNDown(ctx context.Context, name string) error

	// FirewallApply appends a raw iptables-style rule; FirewallDelete removes
	// a matching rule. Both are [danger] at the MCP layer.
	FirewallApply(ctx context.Context, rule FirewallRule) error
	FirewallDelete(ctx context.Context, rule FirewallRule) error

	// PolicyRoutes / SetPolicyRoute / DeletePolicyRoute manage per-device
	// WAN/VPN steering (VPN Director / Fusion style). SetPolicyRoute
	// updates the rule with r.ID, or appends when r.ID is empty.
	// ReplacePolicyRoutes writes a whole rule list in one step (one apply /
	// routing restart) — callers merge with MergePolicyRoutes first.
	PolicyRoutes(ctx context.Context) ([]PolicyRoute, error)
	SetPolicyRoute(ctx context.Context, r PolicyRoute) error
	ReplacePolicyRoutes(ctx context.Context, routes []PolicyRoute) error
	DeletePolicyRoute(ctx context.Context, id string) error
}
