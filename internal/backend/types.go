// Package backend defines the RouterBackend interface: the single typed API
// that every AXOS frontend (MCP, web UI, CLI) is built on. No frontend talks
// to nvram/iptables/wl directly — only a backend implementation does.
package backend

import "time"

// SystemInfo describes the router itself.
type SystemInfo struct {
	Model        string        `json:"model"`
	FirmwareVer  string        `json:"firmware_version"`  // e.g. "3004.388.9"
	FirmwareRev  string        `json:"firmware_revision"` // AXOS-specific build tag, if any
	Serial       string        `json:"serial,omitempty"`
	Uptime       time.Duration `json:"uptime"`
	BootTime     time.Time     `json:"boot_time"`
	HardwareRevA string        `json:"hardware_rev,omitempty"`
}

// Resources holds live system resource usage.
type Resources struct {
	CPULoad1      float64            `json:"cpu_load_1m"`
	CPULoad5      float64            `json:"cpu_load_5m"`
	CPULoad15     float64            `json:"cpu_load_15m"`
	MemTotalKB    uint64             `json:"mem_total_kb"`
	MemUsedKB     uint64             `json:"mem_used_kb"`
	MemFreeKB     uint64             `json:"mem_free_kb"`
	TemperaturesC map[string]float64 `json:"temperatures_c"` // zone name -> °C
}

// InterfaceState is the operational state of a network interface.
type InterfaceState string

const (
	IfaceUp      InterfaceState = "up"
	IfaceDown    InterfaceState = "down"
	IfaceUnknown InterfaceState = "unknown"
)

// Interface describes one network interface (physical, VLAN, bridge, tunnel).
// Traffic-statistics simulation/capture is deliberately not a separate
// method — the Rx/Tx counters here are the traffic stats; a dedicated
// TrafficStats() would just duplicate this data under another name.
type Interface struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"`           // "ethernet", "wifi", "bridge", "vpn", "vlan"
	Role      string         `json:"role,omitempty"` // "wan", "lan", "guest", "unknown" — best-effort
	State     InterfaceState `json:"state"`
	MAC       string         `json:"mac,omitempty"`
	Addresses []string       `json:"addresses,omitempty"`  // CIDR notation
	LinkSpeed string         `json:"link_speed,omitempty"` // e.g. "2.5Gbps", "1Gbps"
	Duplex    string         `json:"duplex,omitempty"`
	RxBytes   uint64         `json:"rx_bytes"`
	TxBytes   uint64         `json:"tx_bytes"`
	RxErrors  uint64         `json:"rx_errors"`
	TxErrors  uint64         `json:"tx_errors"`
	RxDropped uint64         `json:"rx_dropped"`
	TxDropped uint64         `json:"tx_dropped"`
}

// Route is a single routing table entry.
type Route struct {
	Table       string `json:"table"`
	Destination string `json:"destination"` // CIDR, or "default"
	Gateway     string `json:"gateway,omitempty"`
	Interface   string `json:"interface"`
	Metric      int    `json:"metric"`
}

// Client is a device the router has seen on the network.
type Client struct {
	MAC          string    `json:"mac"`
	IP           string    `json:"ip,omitempty"`
	Hostname     string    `json:"hostname,omitempty"`
	Interface    string    `json:"interface,omitempty"` // which LAN/Wi-Fi interface
	Wireless     bool      `json:"wireless"`
	RSSI         *int      `json:"rssi_dbm,omitempty"`
	PHYRateMbps  *float64  `json:"phy_rate_mbps,omitempty"`
	LastSeen     time.Time `json:"last_seen"`
	LeaseExpires time.Time `json:"lease_expires,omitempty"`
}

// WiFiRadio describes one radio's configuration and state.
type WiFiRadio struct {
	Interface       string `json:"interface"`
	Band            string `json:"band"` // "2.4GHz", "5GHz", "6GHz"
	Enabled         bool   `json:"enabled"`
	SSID            string `json:"ssid"`
	Channel         int    `json:"channel"`
	ChannelWidthMHz int    `json:"channel_width_mhz"`
	TxPowerPct      int    `json:"tx_power_pct,omitempty"`
	ClientCount     int    `json:"client_count"`
}

// ShellResult is the outcome of a system.shell_exec call.
type ShellResult struct {
	Command  string        `json:"command"`
	ExitCode int           `json:"exit_code"`
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	Duration time.Duration `json:"duration"`
	TimedOut bool          `json:"timed_out"`
}

// BackupInfo describes a stored configuration snapshot.
type BackupInfo struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
	Reason    string    `json:"reason,omitempty"` // e.g. "pre-rollback", "manual", "scheduled"
}

// ServiceStatus is the running state of one router-managed service
// (dnsmasq, httpd, wireguard, openvpn, ...).
type ServiceStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
}

// FirewallRule is one rule from the router's packet-filtering configuration.
// Deliberately a thin, mostly-opaque representation (raw table/chain/rule
// text) rather than a fully parsed/structured rule — full iptables/nftables
// modeling is Milestone 3 scope (docs/ROADMAP.md); this is enough for
// read-only status/capture/health-check purposes today.
type FirewallRule struct {
	Table string `json:"table"` // e.g. "filter", "nat"
	Chain string `json:"chain"` // e.g. "FORWARD", "INPUT", "PREROUTING"
	Rule  string `json:"rule"`  // raw rule text, implementation-specific
}

// VPNPeer is one configured peer of a VPN tunnel. Never carries a private
// key — only identifying/connection-state fields that are safe to log,
// capture, and display. See docs/security.md "Secrets at rest".
type VPNPeer struct {
	Name          string    `json:"name,omitempty"`
	PublicKey     string    `json:"public_key,omitempty"`
	Endpoint      string    `json:"endpoint,omitempty"`
	AllowedIPs    []string  `json:"allowed_ips,omitempty"`
	LastHandshake time.Time `json:"last_handshake,omitempty"`
	RxBytes       uint64    `json:"rx_bytes"`
	TxBytes       uint64    `json:"tx_bytes"`
}

// VPNTunnel is one configured VPN tunnel (WireGuard/OpenVPN/WARP) and its peers.
type VPNTunnel struct {
	Name         string    `json:"name"` // e.g. "wg0", "warp0", "ovpnc1"
	Type         string    `json:"type"` // "wireguard", "openvpn", "warp"
	Interface    string    `json:"interface"`
	Up           bool      `json:"up"`
	LocalAddress string    `json:"local_address,omitempty"`
	Peers        []VPNPeer `json:"peers,omitempty"`
}

// DiagResult is the typed output of a from-router diagnostic command.
type DiagResult struct {
	Command  string        `json:"command"`
	OK       bool          `json:"ok"`
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration"`
}

// IperfOpts configures a from-router iperf3 run.
type IperfOpts struct {
	Mode    string `json:"mode"` // "client" or "server"
	Target  string `json:"target,omitempty"`
	Port    int    `json:"port,omitempty"`
	Seconds int    `json:"seconds,omitempty"`
	Reverse bool   `json:"reverse,omitempty"`
	UDP     bool   `json:"udp,omitempty"`
}

// PerfResult summarises a bandwidth / latency measurement.
type PerfResult struct {
	Tool     string        `json:"tool"`
	OK       bool          `json:"ok"`
	Summary  string        `json:"summary"`
	Output   string        `json:"output,omitempty"`
	Duration time.Duration `json:"duration"`
}

// DNSInfo is the router's DNS configuration (upstreams + optional DoT).
type DNSInfo struct {
	WANUpstreams []string `json:"wan_upstreams,omitempty"`
	LANUpstreams []string `json:"lan_upstreams,omitempty"`
	DoTEnabled   bool     `json:"dot_enabled"`
	DoTProfile   string   `json:"dot_profile,omitempty"`
	DoTRules     string   `json:"dot_rules,omitempty"`
}

// DHCPReservation is a static DHCP host mapping.
type DHCPReservation struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
}

// QoSInfo is a summary of Adaptive QoS / Cake state.
type QoSInfo struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode,omitempty"`
	Method  int    `json:"method,omitempty"`
	ObwKbps string `json:"obw_kbps,omitempty"`
	IbwKbps string `json:"ibw_kbps,omitempty"`
}

// VPNProfile is a configured VPN client slot (no secrets).
type VPNProfile struct {
	Name        string `json:"name"` // e.g. "wgc1", "ovpnc1"
	Type        string `json:"type"` // "wireguard", "openvpn"
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	Endpoint    string `json:"endpoint,omitempty"`
	KillSwitch  bool   `json:"kill_switch,omitempty"`
}

// WireGuardImport is the input for creating/updating a WG client slot.
// PrivateKey is accepted for import then stored only in the dedicated
// secrets path / nvram — never returned by VPNProfiles / VPNStatus.
type WireGuardImport struct {
	Unit          int      `json:"unit"` // 1..5
	Description   string   `json:"description,omitempty"`
	PrivateKey    string   `json:"private_key"`
	PeerPublicKey string   `json:"peer_public_key"`
	PresharedKey  string   `json:"preshared_key,omitempty"`
	Endpoint      string   `json:"endpoint"`
	EndpointPort  int      `json:"endpoint_port"`
	Address       string   `json:"address"` // tunnel local CIDR
	AllowedIPs    []string `json:"allowed_ips"`
	DNS           string   `json:"dns,omitempty"`
	MTU           int      `json:"mtu,omitempty"`
	Keepalive     int      `json:"keepalive,omitempty"`
	Nat           bool     `json:"nat"`
	KillSwitch    bool     `json:"kill_switch"`
}

// PolicyRoute steers a device (or subnet) to WAN or a VPN client.
type PolicyRoute struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`           // IP or CIDR (VPN Director "Local IP")
	Remote      string `json:"remote,omitempty"` // destination IP/CIDR; empty = any
	Interface   string `json:"interface"`        // "wan", "wgc1", "ovpnc1", ...
	KillSwitch  bool   `json:"kill_switch,omitempty"`
	Enabled     bool   `json:"enabled"`
}
