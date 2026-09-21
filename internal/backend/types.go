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
type Interface struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"` // "ethernet", "wifi", "bridge", "vpn", "vlan"
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
