// Package replay implements backend.RouterBackend by replaying a
// previously captured fixture directory (see internal/capture) instead of
// talking to a real router. This lets development proceed against
// realistic data — real interface names, real client counts, whatever a
// real capture recorded — without repeatedly connecting to hardware.
//
// ReplayBackend's data is immutable: it reflects one moment the fixture was
// captured. Backup/Restore are implemented (so code exercising those tools
// doesn't need special-casing for replay mode) but Restore is necessarily a
// no-op against the static fixture — see their doc comments.
package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/capture"
)

// Backend serves RouterBackend calls from a loaded capture.Snapshot.
type Backend struct {
	mu       sync.Mutex
	snapshot *capture.Snapshot
	backups  map[string]backend.BackupInfo
	nextID   int
}

// New loads a fixture directory (as written by capture.WriteFixtures) and
// returns a ReplayBackend serving it.
func New(fixtureDir string) (*Backend, error) {
	snap, err := capture.ReadFixtures(fixtureDir)
	if err != nil {
		return nil, fmt.Errorf("replay: loading fixtures from %s: %w", fixtureDir, err)
	}
	return &Backend{snapshot: snap, backups: make(map[string]backend.BackupInfo)}, nil
}

// NewFromSnapshot builds a ReplayBackend directly from an in-memory
// Snapshot, skipping the filesystem — used by tests and by any future
// in-process "capture then immediately replay" workflow.
func NewFromSnapshot(snap *capture.Snapshot) *Backend {
	return &Backend{snapshot: snap, backups: make(map[string]backend.BackupInfo)}
}

func (b *Backend) Info(_ context.Context) (backend.SystemInfo, error) {
	return b.snapshot.System, nil
}

func (b *Backend) Resources(_ context.Context) (backend.Resources, error) {
	return b.snapshot.Resources, nil
}

func (b *Backend) Interfaces(_ context.Context) ([]backend.Interface, error) {
	return b.snapshot.Interfaces, nil
}

func (b *Backend) Routes(_ context.Context, table string) ([]backend.Route, error) {
	if table == "" {
		table = "main"
	}
	out := make([]backend.Route, 0, len(b.snapshot.Routes))
	for _, r := range b.snapshot.Routes {
		if r.Table == table {
			out = append(out, r)
		}
	}
	return out, nil
}

func (b *Backend) Clients(_ context.Context) ([]backend.Client, error) {
	return b.snapshot.Clients, nil
}

func (b *Backend) WiFiStatus(_ context.Context) ([]backend.WiFiRadio, error) {
	return b.snapshot.WiFi, nil
}

func (b *Backend) Services(_ context.Context) ([]backend.ServiceStatus, error) {
	return b.snapshot.Services, nil
}

func (b *Backend) FirewallRules(_ context.Context) ([]backend.FirewallRule, error) {
	return b.snapshot.Firewall, nil
}

func (b *Backend) VPNStatus(_ context.Context) ([]backend.VPNTunnel, error) {
	return b.snapshot.VPN, nil
}

func (b *Backend) NVRAMDump(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(b.snapshot.NVRAM))
	for k, v := range b.snapshot.NVRAM {
		out[k] = v
	}
	return out, nil
}

// ShellExec is not supported in replay mode: there is no real shell behind
// a static fixture, and faking arbitrary command output would violate the
// project rule against faking router responses outside explicit mock
// fixtures (docs/ROADMAP.md's own development philosophy). Fails clearly
// rather than pretending to execute anything.
func (b *Backend) ShellExec(_ context.Context, command string, _ int) (backend.ShellResult, error) {
	return backend.ShellResult{}, fmt.Errorf("replay: system.shell_exec is not supported against a replayed fixture (no real shell to run %q against — use --backend mock or asuswrt)", command)
}

// Backup records a snapshot of the replayed fixture's (already-sanitized)
// nvram data as a new in-memory backup entry. Useful for exercising
// backup-listing/UI code against realistic data.
func (b *Backend) Backup(_ context.Context, reason string) (backend.BackupInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	id := fmt.Sprintf("replay-backup-%04d", b.nextID)
	data, _ := json.Marshal(b.snapshot.NVRAM)
	sum := sha256.Sum256(data)

	info := backend.BackupInfo{
		ID:        id,
		Path:      "(replay: in-memory, not written to disk)",
		SizeBytes: int64(len(data)),
		SHA256:    hex.EncodeToString(sum[:]),
		CreatedAt: time.Now(),
		Reason:    reason,
	}
	b.backups[id] = info
	return info, nil
}

// Restore succeeds as a no-op if backupID is known. There is nothing to
// actually restore *to* — replayed fixture data is a fixed snapshot of one
// moment, not a mutable router state — but the call still validates the ID
// and errors on an unknown one, so code paths that check for that (e.g. the
// rollback engine's error handling) get exercised realistically.
func (b *Backend) Restore(_ context.Context, backupID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.backups[backupID]; !ok {
		return fmt.Errorf("replay: unknown backup id %q", backupID)
	}
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

var _ backend.RouterBackend = (*Backend)(nil)
