// Package capture queries a RouterBackend (real or mock) and records its
// state as sanitized JSON fixtures, so development can proceed against
// realistic data without repeatedly connecting to a real router (see
// internal/backend/replay, which reads these fixtures back).
//
// Capture works against ANY backend.RouterBackend — it doesn't know or care
// whether it's talking to mock.Backend or a real router. This is
// deliberate: it means the capture -> sanitize -> write -> replay pipeline
// is fully exercisable (and tested) without hardware, by capturing a
// MockBackend and replaying it.
package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/reece01-dock/axos/internal/backend"
)

// Snapshot is everything captured from one RouterBackend at one point in
// time. Field names match the fixture files a Snapshot is written to (see
// WriteFixtures) minus the .json extension.
type Snapshot struct {
	CapturedAt time.Time `json:"captured_at"`
	Source     string    `json:"source"` // free-text: what produced this, e.g. "mock", "gt-ax6000 (asuswrt, live)"

	System     backend.SystemInfo      `json:"system"`
	Resources  backend.Resources       `json:"resources"`
	NVRAM      map[string]string       `json:"nvram"`
	Interfaces []backend.Interface     `json:"interfaces"`
	Routes     []backend.Route         `json:"routes"`
	Clients    []backend.Client        `json:"clients"`
	WiFi       []backend.WiFiRadio     `json:"wifi"`
	Services   []backend.ServiceStatus `json:"services"`
	Firewall   []backend.FirewallRule  `json:"firewall"`
	VPN        []backend.VPNTunnel     `json:"vpn"`

	// Sanitized records whether Sanitize() has been run on this snapshot.
	// WriteFixtures refuses to write an unsanitized snapshot — see there.
	Sanitized bool `json:"sanitized"`

	// Errors collects per-field capture failures without aborting the whole
	// capture — a backend that doesn't support e.g. VPNStatus yet still
	// produces a useful partial snapshot for everything else.
	Errors []string `json:"errors,omitempty"`
}

// Capture queries every RouterBackend read method and assembles a Snapshot.
// Individual method failures are recorded in Snapshot.Errors rather than
// aborting the capture, so a backend that only partially implements the
// interface (or a real router where one subsystem is temporarily
// unreachable) still yields a useful result.
func Capture(ctx context.Context, be backend.RouterBackend, source string) (*Snapshot, error) {
	s := &Snapshot{CapturedAt: time.Now(), Source: source}

	record := func(field string, err error) {
		if err != nil {
			s.Errors = append(s.Errors, fmt.Sprintf("%s: %v", field, err))
		}
	}

	var err error
	if s.System, err = be.Info(ctx); err != nil {
		record("system", err)
	}
	if s.Resources, err = be.Resources(ctx); err != nil {
		record("resources", err)
	}
	if s.NVRAM, err = be.NVRAMDump(ctx); err != nil {
		record("nvram", err)
	}
	if s.Interfaces, err = be.Interfaces(ctx); err != nil {
		record("interfaces", err)
	}
	if s.Routes, err = be.Routes(ctx, ""); err != nil {
		record("routes", err)
	}
	if s.Clients, err = be.Clients(ctx); err != nil {
		record("clients", err)
	}
	if s.WiFi, err = be.WiFiStatus(ctx); err != nil {
		record("wifi", err)
	}
	if s.Services, err = be.Services(ctx); err != nil {
		record("services", err)
	}
	if s.Firewall, err = be.FirewallRules(ctx); err != nil {
		record("firewall", err)
	}
	if s.VPN, err = be.VPNStatus(ctx); err != nil {
		record("vpn", err)
	}

	if len(s.Errors) == len(errorableFields) {
		return s, fmt.Errorf("capture: every field failed to capture, first error: %s", s.Errors[0])
	}
	return s, nil
}

// errorableFields is used only to size-check "did everything fail" above —
// kept as a literal count rather than reflection for simplicity.
var errorableFields = [10]struct{}{}

// capabilities is a small derived summary written alongside a snapshot's
// full data, matching the "capabilities.json" fixture the design calls for:
// a quick answer to "what did this capture actually contain" without
// parsing every other file.
type capabilities struct {
	CapturedAt     time.Time `json:"captured_at"`
	Source         string    `json:"source"`
	Sanitized      bool      `json:"sanitized"`
	InterfaceCount int       `json:"interface_count"`
	RouteCount     int       `json:"route_count"`
	ClientCount    int       `json:"client_count"`
	WiFiRadioCount int       `json:"wifi_radio_count"`
	ServiceCount   int       `json:"service_count"`
	FirewallCount  int       `json:"firewall_rule_count"`
	VPNTunnelCount int       `json:"vpn_tunnel_count"`
	NVRAMKeyCount  int       `json:"nvram_key_count"`
	CaptureErrors  []string  `json:"capture_errors,omitempty"`
}

func (s *Snapshot) capabilities() capabilities {
	return capabilities{
		CapturedAt:     s.CapturedAt,
		Source:         s.Source,
		Sanitized:      s.Sanitized,
		InterfaceCount: len(s.Interfaces),
		RouteCount:     len(s.Routes),
		ClientCount:    len(s.Clients),
		WiFiRadioCount: len(s.WiFi),
		ServiceCount:   len(s.Services),
		FirewallCount:  len(s.Firewall),
		VPNTunnelCount: len(s.VPN),
		NVRAMKeyCount:  len(s.NVRAM),
		CaptureErrors:  s.Errors,
	}
}

// fixtureFiles maps each on-disk fixture filename to the accessor/setter
// pair used by WriteFixtures/ReadFixtures. Keeping this table-driven means
// there is exactly one place that knows the file layout.
type systemFixture struct {
	Info      backend.SystemInfo `json:"info"`
	Resources backend.Resources  `json:"resources"`
}

// WriteFixtures writes a sanitized Snapshot to dir as the set of JSON files
// described in docs/development.md ("Router Capture System"): system.json,
// nvram.json, interfaces.json, routes.json, wifi.json, clients.json,
// firewall.json, vpn.json, services.json, capabilities.json.
//
// Refuses to write a snapshot that hasn't been through Sanitize() —
// unsanitized nvram data contains plaintext secrets (see docs/security.md)
// and must never reach disk under testdata/, which is committed to git.
func WriteFixtures(dir string, s *Snapshot) error {
	if !s.Sanitized {
		return fmt.Errorf("capture: refusing to write an unsanitized snapshot to %s (call Sanitize first)", dir)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("capture: creating fixture dir %s: %w", dir, err)
	}

	files := map[string]interface{}{
		"system.json":       systemFixture{Info: s.System, Resources: s.Resources},
		"nvram.json":        s.NVRAM,
		"interfaces.json":   s.Interfaces,
		"routes.json":       s.Routes,
		"clients.json":      s.Clients,
		"wifi.json":         s.WiFi,
		"services.json":     s.Services,
		"firewall.json":     s.Firewall,
		"vpn.json":          s.VPN,
		"capabilities.json": s.capabilities(),
	}
	for name, data := range files {
		if err := writeJSONFile(filepath.Join(dir, name), data); err != nil {
			return fmt.Errorf("capture: writing %s: %w", name, err)
		}
	}
	return nil
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}

// ReadFixtures reads a fixture directory written by WriteFixtures back into
// a Snapshot. Used by internal/backend/replay and by round-trip tests.
func ReadFixtures(dir string) (*Snapshot, error) {
	var sf systemFixture
	if err := readJSONFile(filepath.Join(dir, "system.json"), &sf); err != nil {
		return nil, fmt.Errorf("capture: reading system.json: %w", err)
	}

	s := &Snapshot{System: sf.Info, Resources: sf.Resources, Sanitized: true}

	readers := []struct {
		file string
		dst  interface{}
	}{
		{"nvram.json", &s.NVRAM},
		{"interfaces.json", &s.Interfaces},
		{"routes.json", &s.Routes},
		{"clients.json", &s.Clients},
		{"wifi.json", &s.WiFi},
		{"services.json", &s.Services},
		{"firewall.json", &s.Firewall},
		{"vpn.json", &s.VPN},
	}
	for _, r := range readers {
		if err := readJSONFile(filepath.Join(dir, r.file), r.dst); err != nil {
			return nil, fmt.Errorf("capture: reading %s: %w", r.file, err)
		}
	}

	var caps capabilities
	if err := readJSONFile(filepath.Join(dir, "capabilities.json"), &caps); err == nil {
		s.CapturedAt = caps.CapturedAt
		s.Source = caps.Source
		s.Errors = caps.CaptureErrors
	}

	return s, nil
}

func readJSONFile(path string, dst interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
