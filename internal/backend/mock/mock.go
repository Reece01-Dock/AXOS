// Package mock provides an in-memory RouterBackend implementation for
// development and unit testing, with no dependency on real router hardware
// or Asuswrt/Merlin tooling. All state is mutable at runtime (see
// state.go's setters) so tests and `axosd --backend mock` sessions can
// simulate a client joining, a VPN peer handshaking, a service crashing,
// etc., without touching a real router.
package mock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/reece01-dock/axos/internal/backend"
)

// Backend is a fake RouterBackend holding all state in memory, guarded by a
// single mutex. Simple over clever: this is a development/test tool, not a
// performance-sensitive path.
type Backend struct {
	mu sync.Mutex

	bootTime time.Time
	nvram    map[string]string
	ifaces   map[string]backend.Interface // keyed by name
	ifOrder  []string                     // insertion order, for deterministic listing
	routes   []backend.Route
	clients  map[string]backend.Client // keyed by MAC
	wifi     map[string]backend.WiFiRadio
	wifiOrd  []string
	services map[string]backend.ServiceStatus
	svcOrder []string
	firewall []backend.FirewallRule
	vpn      map[string]backend.VPNTunnel
	vpnOrder []string
	res      backend.Resources

	backups  map[string]backend.BackupInfo
	restored []string
	nextID   int
}

// New returns a mock backend pre-populated with a plausible GT-AX6000-shaped
// fixture: two 2.5GbE ports, four 1GbE LAN ports, both Wi-Fi radios, a
// couple of clients, dnsmasq/httpd services, and no VPN configured (matching
// a stock router with nothing set up yet). Every field can be changed
// afterwards via the setters in state.go.
func New() *Backend {
	b := &Backend{
		bootTime: time.Now().Add(-2 * time.Hour),
		nvram: map[string]string{
			"productid": "GT-AX6000",
			"buildno":   "3004.388.9",
			"extendno":  "axos-mock",
			// A representative secret, so tests exercising capture/backup
			// redaction have something real to redact.
			"wl1_wpa_psk": "MockWifiPassphrase123",
			"http_passwd": "mock-admin-password",
		},
		ifaces:   make(map[string]backend.Interface),
		clients:  make(map[string]backend.Client),
		wifi:     make(map[string]backend.WiFiRadio),
		services: make(map[string]backend.ServiceStatus),
		vpn:      make(map[string]backend.VPNTunnel),
		backups:  make(map[string]backend.BackupInfo),
		res: backend.Resources{
			CPULoad1: 0.12, CPULoad5: 0.18, CPULoad15: 0.15,
			MemTotalKB: 1024 * 1024, MemUsedKB: 412 * 1024, MemFreeKB: 612 * 1024,
			TemperaturesC: map[string]float64{"cpu": 54.5, "5g": 48.2, "2g": 42.0},
		},
	}

	b.setInterfaceLocked(backend.Interface{
		Name: "eth0", Type: "ethernet", Role: "wan", State: backend.IfaceUp, LinkSpeed: "2.5Gbps",
		MAC: "AC:DE:48:00:11:01", Addresses: []string{"203.0.113.42/24"}, RxBytes: 1_200_000_000, TxBytes: 300_000_000,
	})
	b.setInterfaceLocked(backend.Interface{
		Name: "eth5", Type: "ethernet", Role: "lan", State: backend.IfaceUp, LinkSpeed: "2.5Gbps",
		MAC: "AC:DE:48:00:11:02", Addresses: []string{"192.168.1.1/24"},
	})
	b.setInterfaceLocked(backend.Interface{
		Name: "br0", Type: "bridge", Role: "lan", State: backend.IfaceUp,
		MAC: "AC:DE:48:00:11:00", Addresses: []string{"192.168.1.1/24"},
	})
	b.setInterfaceLocked(backend.Interface{Name: "wl0", Type: "wifi", Role: "lan", State: backend.IfaceUp, MAC: "AC:DE:48:00:11:10"})
	b.setInterfaceLocked(backend.Interface{Name: "wl1", Type: "wifi", Role: "lan", State: backend.IfaceUp, MAC: "AC:DE:48:00:11:11"})

	b.routes = []backend.Route{
		{Table: "main", Destination: "default", Gateway: "203.0.113.1", Interface: "eth0", Metric: 0},
		{Table: "main", Destination: "192.168.1.0/24", Interface: "br0", Metric: 0},
	}

	rssi := -52
	rate := 866.7
	b.setClientLocked(backend.Client{MAC: "11:22:33:44:55:66", IP: "192.168.1.50", Hostname: "gaming-pc", Interface: "eth1", LastSeen: time.Now()})
	b.setClientLocked(backend.Client{MAC: "11:22:33:44:55:77", IP: "192.168.1.51", Hostname: "living-room-tv", Interface: "wl1",
		Wireless: true, RSSI: &rssi, PHYRateMbps: &rate, LastSeen: time.Now()})

	b.setWiFiRadioLocked(backend.WiFiRadio{Interface: "wl0", Band: "2.4GHz", Enabled: true, SSID: "AXOS-Lab", Channel: 6, ChannelWidthMHz: 40, ClientCount: 1})
	b.setWiFiRadioLocked(backend.WiFiRadio{Interface: "wl1", Band: "5GHz", Enabled: true, SSID: "AXOS-Lab", Channel: 149, ChannelWidthMHz: 80, ClientCount: 1})

	b.setServiceLocked(backend.ServiceStatus{Name: "dnsmasq", Running: true, PID: 512})
	b.setServiceLocked(backend.ServiceStatus{Name: "httpd", Running: true, PID: 498})
	b.setServiceLocked(backend.ServiceStatus{Name: "wireguard", Running: false})
	b.setServiceLocked(backend.ServiceStatus{Name: "openvpn", Running: false})

	b.firewall = []backend.FirewallRule{
		{Table: "filter", Chain: "INPUT", Rule: "-i eth0 -j DROP"},
		{Table: "filter", Chain: "FORWARD", Rule: "-i br0 -o eth0 -j ACCEPT"},
		{Table: "nat", Chain: "POSTROUTING", Rule: "-o eth0 -j MASQUERADE"},
	}

	return b
}

func (b *Backend) Info(_ context.Context) (backend.SystemInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return backend.SystemInfo{
		Model:       b.nvram["productid"],
		FirmwareVer: b.nvram["buildno"],
		FirmwareRev: b.nvram["extendno"],
		Uptime:      time.Since(b.bootTime),
		BootTime:    b.bootTime,
	}, nil
}

func (b *Backend) Resources(_ context.Context) (backend.Resources, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.res, nil
}

func (b *Backend) Interfaces(_ context.Context) ([]backend.Interface, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.Interface, 0, len(b.ifOrder))
	for _, name := range b.ifOrder {
		out = append(out, b.ifaces[name])
	}
	return out, nil
}

func (b *Backend) Routes(_ context.Context, table string) ([]backend.Route, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if table == "" {
		table = "main"
	}
	out := make([]backend.Route, 0, len(b.routes))
	for _, r := range b.routes {
		if r.Table == table {
			out = append(out, r)
		}
	}
	return out, nil
}

func (b *Backend) Clients(_ context.Context) ([]backend.Client, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.Client, 0, len(b.clients))
	for _, c := range b.clients {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out, nil
}

func (b *Backend) WiFiStatus(_ context.Context) ([]backend.WiFiRadio, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.WiFiRadio, 0, len(b.wifiOrd))
	for _, name := range b.wifiOrd {
		out = append(out, b.wifi[name])
	}
	return out, nil
}

func (b *Backend) Services(_ context.Context) ([]backend.ServiceStatus, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.ServiceStatus, 0, len(b.svcOrder))
	for _, name := range b.svcOrder {
		out = append(out, b.services[name])
	}
	return out, nil
}

func (b *Backend) FirewallRules(_ context.Context) ([]backend.FirewallRule, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.FirewallRule, len(b.firewall))
	copy(out, b.firewall)
	return out, nil
}

func (b *Backend) VPNStatus(_ context.Context) ([]backend.VPNTunnel, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.VPNTunnel, 0, len(b.vpnOrder))
	for _, name := range b.vpnOrder {
		out = append(out, b.vpn[name])
	}
	return out, nil
}

func (b *Backend) NVRAMDump(_ context.Context) (map[string]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]string, len(b.nvram))
	for k, v := range b.nvram {
		out[k] = v
	}
	return out, nil
}

func (b *Backend) ShellExec(_ context.Context, command string, timeoutSeconds int) (backend.ShellResult, error) {
	// The mock never actually executes anything — it fakes a result so
	// higher layers (audit logging, MCP tool plumbing) can be exercised in
	// tests without touching a real shell.
	return backend.ShellResult{
		Command:  command,
		ExitCode: 0,
		Stdout:   fmt.Sprintf("[mock] would run: %s", command),
		Duration: 5 * time.Millisecond,
	}, nil
}

func (b *Backend) Backup(_ context.Context, reason string) (backend.BackupInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	id := fmt.Sprintf("backup-%04d", b.nextID)
	content := fmt.Sprintf("mock-nvram-export:%s:%d", reason, b.nextID)
	sum := sha256.Sum256([]byte(content))

	info := backend.BackupInfo{
		ID:        id,
		Path:      "/opt/axos/backups/" + id + ".tar.gz",
		SizeBytes: int64(len(content)),
		SHA256:    hex.EncodeToString(sum[:]),
		CreatedAt: time.Now(),
		Reason:    reason,
	}
	b.backups[id] = info
	return info, nil
}

func (b *Backend) Restore(_ context.Context, backupID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.backups[backupID]; !ok {
		return fmt.Errorf("backend/mock: unknown backup id %q", backupID)
	}
	b.restored = append(b.restored, backupID)
	return nil
}

func (b *Backend) ListBackups(_ context.Context) ([]backend.BackupInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]backend.BackupInfo, 0, len(b.backups))
	for _, info := range b.backups {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// RestoredIDs returns the history of Restore calls, most recent last. Test-only helper.
func (b *Backend) RestoredIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.restored))
	copy(out, b.restored)
	return out
}

// Ensure interface compliance at compile time.
var _ backend.RouterBackend = (*Backend)(nil)
