// Package mock provides an in-memory RouterBackend implementation for
// development and unit testing, with no dependency on real router hardware
// or Asuswrt/Merlin tooling.
package mock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/reece01-dock/axos/axosd/internal/backend"
)

// Backend is a fake RouterBackend holding all state in memory.
type Backend struct {
	mu       sync.Mutex
	bootTime time.Time
	backups  map[string]backend.BackupInfo
	restored []string // history of restored backup IDs, for test assertions
	nextID   int
}

// New returns a mock backend pre-populated with plausible fake data.
func New() *Backend {
	return &Backend{
		bootTime: time.Now().Add(-2 * time.Hour),
		backups:  make(map[string]backend.BackupInfo),
	}
}

func (b *Backend) Info(_ context.Context) (backend.SystemInfo, error) {
	return backend.SystemInfo{
		Model:       "GT-AX6000",
		FirmwareVer: "3004.388.9",
		FirmwareRev: "axos-mock",
		Uptime:      time.Since(b.bootTime),
		BootTime:    b.bootTime,
	}, nil
}

func (b *Backend) Resources(_ context.Context) (backend.Resources, error) {
	return backend.Resources{
		CPULoad1:   0.12,
		CPULoad5:   0.18,
		CPULoad15:  0.15,
		MemTotalKB: 1024 * 1024,
		MemUsedKB:  412 * 1024,
		MemFreeKB:  612 * 1024,
		TemperaturesC: map[string]float64{
			"cpu": 54.5,
			"5g":  48.2,
			"2g":  42.0,
		},
	}, nil
}

func (b *Backend) Interfaces(_ context.Context) ([]backend.Interface, error) {
	return []backend.Interface{
		{Name: "eth0", Type: "ethernet", State: backend.IfaceUp, LinkSpeed: "2.5Gbps",
			MAC: "AC:DE:48:00:11:01", Addresses: []string{"0.0.0.0/0"}, RxBytes: 1_200_000_000, TxBytes: 300_000_000},
		{Name: "eth5", Type: "ethernet", State: backend.IfaceUp, LinkSpeed: "2.5Gbps",
			MAC: "AC:DE:48:00:11:02", Addresses: []string{"192.168.1.1/24"}},
		{Name: "br0", Type: "bridge", State: backend.IfaceUp,
			MAC: "AC:DE:48:00:11:00", Addresses: []string{"192.168.1.1/24"}},
		{Name: "wl0", Type: "wifi", State: backend.IfaceUp, MAC: "AC:DE:48:00:11:10"},
		{Name: "wl1", Type: "wifi", State: backend.IfaceUp, MAC: "AC:DE:48:00:11:11"},
	}, nil
}

func (b *Backend) Routes(_ context.Context, table string) ([]backend.Route, error) {
	if table == "" {
		table = "main"
	}
	return []backend.Route{
		{Table: table, Destination: "default", Gateway: "203.0.113.1", Interface: "eth0", Metric: 0},
		{Table: table, Destination: "192.168.1.0/24", Interface: "br0", Metric: 0},
	}, nil
}

func (b *Backend) Clients(_ context.Context) ([]backend.Client, error) {
	rssi := -52
	rate := 866.7
	return []backend.Client{
		{MAC: "11:22:33:44:55:66", IP: "192.168.1.50", Hostname: "gaming-pc", Interface: "eth1", LastSeen: time.Now()},
		{MAC: "11:22:33:44:55:77", IP: "192.168.1.51", Hostname: "living-room-tv", Interface: "wl1",
			Wireless: true, RSSI: &rssi, PHYRateMbps: &rate, LastSeen: time.Now()},
	}, nil
}

func (b *Backend) WiFiStatus(_ context.Context) ([]backend.WiFiRadio, error) {
	return []backend.WiFiRadio{
		{Interface: "wl0", Band: "2.4GHz", Enabled: true, SSID: "AXOS-Lab", Channel: 6, ChannelWidthMHz: 40, ClientCount: 3},
		{Interface: "wl1", Band: "5GHz", Enabled: true, SSID: "AXOS-Lab", Channel: 149, ChannelWidthMHz: 80, ClientCount: 5},
	}, nil
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
		Path:      "/mnt/usb1/axos/backups/" + id + ".tar.gz",
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
