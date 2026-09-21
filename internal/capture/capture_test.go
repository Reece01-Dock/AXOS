package capture

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/backend/mock"
)

func TestCapture_AgainstMockBackend(t *testing.T) {
	mb := mock.New()
	snap, err := Capture(context.Background(), mb, "mock")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if snap.System.Model != "GT-AX6000" {
		t.Errorf("System.Model = %q, want GT-AX6000", snap.System.Model)
	}
	if len(snap.Interfaces) == 0 {
		t.Error("Interfaces is empty, want at least the mock's default set")
	}
	if len(snap.Errors) != 0 {
		t.Errorf("Capture against a fully-implemented backend should have no errors, got: %v", snap.Errors)
	}
	if snap.Sanitized {
		t.Error("a freshly captured snapshot must not report Sanitized until Sanitize() runs")
	}
}

func TestSanitize_RedactsSecrets(t *testing.T) {
	mb := mock.New()
	mb.SetNVRAM("wl1_wpa_psk", "TotallyRealWifiPassword")
	mb.SetNVRAM("http_passwd", "admin123")
	mb.SetNVRAM("some_secret_token", "abcdef")
	mb.SetNVRAM("productid", "GT-AX6000") // not a secret — must survive

	snap, err := Capture(context.Background(), mb, "mock")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	Sanitize(snap)

	if !snap.Sanitized {
		t.Fatal("Sanitize() did not set Sanitized = true")
	}

	cases := map[string]string{
		"wl1_wpa_psk":       redacted,
		"http_passwd":       redacted,
		"some_secret_token": redacted,
		"productid":         "GT-AX6000",
	}
	for key, want := range cases {
		if got := snap.NVRAM[key]; got != want {
			t.Errorf("NVRAM[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestWriteFixtures_RefusesUnsanitizedSnapshot(t *testing.T) {
	mb := mock.New()
	snap, err := Capture(context.Background(), mb, "mock")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if err := WriteFixtures(t.TempDir(), snap); err == nil {
		t.Fatal("WriteFixtures should refuse an unsanitized snapshot")
	}
}

func TestWriteFixtures_NeverWritesRawSecretsToDisk(t *testing.T) {
	mb := mock.New()
	mb.SetNVRAM("wl1_wpa_psk", "TotallyRealWifiPassword-CanaryValue")

	snap, err := Capture(context.Background(), mb, "mock")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	Sanitize(snap)

	dir := t.TempDir()
	if err := WriteFixtures(dir, snap); err != nil {
		t.Fatalf("WriteFixtures: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		if strings.Contains(string(data), "TotallyRealWifiPassword-CanaryValue") {
			t.Fatalf("%s contains the raw secret value — sanitization was bypassed", e.Name())
		}
	}
}

func TestWriteFixtures_ProducesExpectedFileSet(t *testing.T) {
	mb := mock.New()
	snap, err := Capture(context.Background(), mb, "mock")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	Sanitize(snap)

	dir := t.TempDir()
	if err := WriteFixtures(dir, snap); err != nil {
		t.Fatalf("WriteFixtures: %v", err)
	}

	want := []string{
		"system.json", "nvram.json", "interfaces.json", "routes.json",
		"clients.json", "wifi.json", "services.json", "firewall.json",
		"vpn.json", "capabilities.json",
	}
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected fixture file %s: %v", name, err)
		}
	}
}

func TestRoundTrip_CaptureWriteReadMatchesOriginal(t *testing.T) {
	mb := mock.New()
	mb.SetClient(backend.Client{MAC: "AA:BB:CC:DD:EE:FF", IP: "192.168.1.77", Hostname: "roundtrip-test-device"})

	snap, err := Capture(context.Background(), mb, "mock")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	Sanitize(snap)

	dir := t.TempDir()
	if err := WriteFixtures(dir, snap); err != nil {
		t.Fatalf("WriteFixtures: %v", err)
	}

	reloaded, err := ReadFixtures(dir)
	if err != nil {
		t.Fatalf("ReadFixtures: %v", err)
	}

	if reloaded.System.Model != snap.System.Model {
		t.Errorf("reloaded System.Model = %q, want %q", reloaded.System.Model, snap.System.Model)
	}
	if len(reloaded.Interfaces) != len(snap.Interfaces) {
		t.Errorf("reloaded has %d interfaces, want %d", len(reloaded.Interfaces), len(snap.Interfaces))
	}

	found := false
	for _, c := range reloaded.Clients {
		if c.MAC == "AA:BB:CC:DD:EE:FF" && c.Hostname == "roundtrip-test-device" {
			found = true
		}
	}
	if !found {
		t.Error("reloaded fixture is missing the client added before capture")
	}
}
