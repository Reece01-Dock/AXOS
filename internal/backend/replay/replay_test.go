package replay

import (
	"context"
	"testing"

	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/backend/mock"
	"github.com/reece01-dock/axos/internal/capture"
)

// buildFixture captures a MockBackend (with a couple of distinguishing
// mutations applied) and returns a ReplayBackend serving that capture —
// the full local pipeline the acceptance criteria call for: "AXOS runs
// locally with MockBackend" + "Router state can be captured safely" +
// "Captures can be replayed", with no real router involved anywhere.
func buildFixture(t *testing.T) *Backend {
	t.Helper()
	mb := mock.New()
	mb.SetClient(backend.Client{MAC: "AA:BB:CC:DD:EE:FF", IP: "192.168.1.77", Hostname: "replay-test-device"})
	mb.SetVPNTunnel(backend.VPNTunnel{Name: "wg0", Type: "wireguard", Interface: "wg0", Up: true})

	snap, err := capture.Capture(context.Background(), mb, "mock")
	if err != nil {
		t.Fatalf("capture.Capture: %v", err)
	}
	capture.Sanitize(snap)

	dir := t.TempDir()
	if err := capture.WriteFixtures(dir, snap); err != nil {
		t.Fatalf("capture.WriteFixtures: %v", err)
	}

	rb, err := New(dir)
	if err != nil {
		t.Fatalf("replay.New: %v", err)
	}
	return rb
}

func TestReplayBackend_ServesCapturedData(t *testing.T) {
	rb := buildFixture(t)
	ctx := context.Background()

	info, err := rb.Info(ctx)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Model != "GT-AX6000" {
		t.Errorf("Info().Model = %q, want GT-AX6000", info.Model)
	}

	clients, err := rb.Clients(ctx)
	if err != nil {
		t.Fatalf("Clients: %v", err)
	}
	found := false
	for _, c := range clients {
		if c.MAC == "AA:BB:CC:DD:EE:FF" {
			found = true
		}
	}
	if !found {
		t.Error("ReplayBackend did not serve the client that was present at capture time")
	}

	vpns, err := rb.VPNStatus(ctx)
	if err != nil {
		t.Fatalf("VPNStatus: %v", err)
	}
	if len(vpns) != 1 || vpns[0].Name != "wg0" {
		t.Errorf("VPNStatus = %+v, want the wg0 tunnel captured from mock", vpns)
	}

	ifaces, err := rb.Interfaces(ctx)
	if err != nil || len(ifaces) == 0 {
		t.Fatalf("Interfaces() = %v, %v; want the mock's default interfaces", ifaces, err)
	}
}

func TestReplayBackend_NVRAMDumpIsSanitized(t *testing.T) {
	rb := buildFixture(t)
	nv, err := rb.NVRAMDump(context.Background())
	if err != nil {
		t.Fatalf("NVRAMDump: %v", err)
	}
	// mock.New() seeds wl1_wpa_psk; Sanitize should have redacted it before
	// it ever reached the fixture ReplayBackend loaded.
	if nv["wl1_wpa_psk"] != "[REDACTED]" {
		t.Errorf("NVRAMDump()[wl1_wpa_psk] = %q, want [REDACTED] — replay must never surface a real secret", nv["wl1_wpa_psk"])
	}
}

func TestReplayBackend_ShellExecIsRefused(t *testing.T) {
	rb := buildFixture(t)
	_, err := rb.ShellExec(context.Background(), "echo hi", 5)
	if err == nil {
		t.Fatal("ShellExec against a replayed fixture should fail clearly, not fake a result")
	}
}

func TestReplayBackend_BackupAndRestore(t *testing.T) {
	rb := buildFixture(t)
	ctx := context.Background()

	info, err := rb.Backup(ctx, "test")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if info.ID == "" {
		t.Fatal("Backup did not return an id")
	}

	if err := rb.Restore(ctx, info.ID); err != nil {
		t.Fatalf("Restore of a known backup id should succeed, got: %v", err)
	}
	if err := rb.Restore(ctx, "does-not-exist"); err == nil {
		t.Fatal("Restore of an unknown backup id should fail")
	}

	backups, err := rb.ListBackups(ctx)
	if err != nil || len(backups) != 1 {
		t.Fatalf("ListBackups() = %v, %v; want exactly the one backup created above", backups, err)
	}
}

func TestNew_UnknownFixtureDir(t *testing.T) {
	if _, err := New(t.TempDir()); err == nil {
		t.Fatal("New against an empty directory (no fixtures) should fail")
	}
}

var _ backend.RouterBackend = (*Backend)(nil)
