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
	"strconv"
	"strings"
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

// --- Milestone 3: diagnostics / mutations are not supported in replay -----

func (b *Backend) Ping(_ context.Context, host string, _ int) (backend.DiagResult, error) {
	return backend.DiagResult{}, fmt.Errorf("replay: ping is not supported against a replayed fixture (no real shell to reach %q — use --backend mock or asuswrt)", host)
}

func (b *Backend) Traceroute(_ context.Context, host string, _ int) (backend.DiagResult, error) {
	return backend.DiagResult{}, fmt.Errorf("replay: traceroute is not supported against a replayed fixture (no real shell to reach %q — use --backend mock or asuswrt)", host)
}

func (b *Backend) DNSLookup(_ context.Context, name string) (backend.DiagResult, error) {
	return backend.DiagResult{}, fmt.Errorf("replay: dnslookup is not supported against a replayed fixture (no real resolver for %q — use --backend mock or asuswrt)", name)
}

func (b *Backend) PortCheck(_ context.Context, host string, port int) (backend.DiagResult, error) {
	return backend.DiagResult{}, fmt.Errorf("replay: portcheck is not supported against a replayed fixture (no real socket to %s:%d — use --backend mock or asuswrt)", host, port)
}

func (b *Backend) Iperf3(_ context.Context, _ backend.IperfOpts) (backend.PerfResult, error) {
	return backend.PerfResult{}, fmt.Errorf("replay: iperf3 is not supported against a replayed fixture (use --backend mock or asuswrt)")
}

func (b *Backend) SetDNSConfig(_ context.Context, _ backend.DNSInfo) error {
	return fmt.Errorf("replay: set dns config is not supported against a replayed fixture (immutable snapshot)")
}

func (b *Backend) SetDHCPReservation(_ context.Context, _ backend.DHCPReservation) error {
	return fmt.Errorf("replay: set dhcp reservation is not supported against a replayed fixture (immutable snapshot)")
}

func (b *Backend) DeleteDHCPReservation(_ context.Context, _ string) error {
	return fmt.Errorf("replay: delete dhcp reservation is not supported against a replayed fixture (immutable snapshot)")
}

func (b *Backend) SetQoSEnable(_ context.Context, _ bool) error {
	return fmt.Errorf("replay: set qos enable is not supported against a replayed fixture (immutable snapshot)")
}

func (b *Backend) ImportWireGuard(_ context.Context, _ backend.WireGuardImport) error {
	return fmt.Errorf("replay: import wireguard is not supported against a replayed fixture (immutable snapshot)")
}

func (b *Backend) VPNUp(_ context.Context, name string) error {
	return fmt.Errorf("replay: vpn up %q is not supported against a replayed fixture (immutable snapshot)", name)
}

func (b *Backend) VPNDown(_ context.Context, name string) error {
	return fmt.Errorf("replay: vpn down %q is not supported against a replayed fixture (immutable snapshot)", name)
}

func (b *Backend) FirewallApply(_ context.Context, _ backend.FirewallRule) error {
	return fmt.Errorf("replay: firewall apply is not supported against a replayed fixture (immutable snapshot)")
}

func (b *Backend) FirewallDelete(_ context.Context, _ backend.FirewallRule) error {
	return fmt.Errorf("replay: firewall delete is not supported against a replayed fixture (immutable snapshot)")
}

func (b *Backend) SetPolicyRoute(_ context.Context, _ backend.PolicyRoute) error {
	return fmt.Errorf("replay: set policy route is not supported against a replayed fixture (immutable snapshot)")
}

func (b *Backend) DeletePolicyRoute(_ context.Context, _ string) error {
	return fmt.Errorf("replay: delete policy route is not supported against a replayed fixture (immutable snapshot)")
}

// --- Milestone 3: read methods derived from snapshot NVRAM -----------------

func (b *Backend) DNSConfig(_ context.Context) (backend.DNSInfo, error) {
	nv := b.snapshot.NVRAM
	if nv == nil {
		return backend.DNSInfo{}, nil
	}
	wan := nv["wan0_dns"]
	if wan == "" {
		wan = nv["wan_dns"]
	}
	info := backend.DNSInfo{
		WANUpstreams: fieldsOrNil(wan),
		DoTEnabled:   nv["dnspriv_enable"] == "1",
		DoTProfile:   nv["dnspriv_profile"],
		DoTRules:     nv["dnspriv_rulelist"],
	}
	for _, k := range []string{"dhcp_dns1_x", "lan_dns1_x", "dhcp_dns2_x", "lan_dns2_x"} {
		if v := strings.TrimSpace(nv[k]); v != "" {
			info.LANUpstreams = appendUnique(info.LANUpstreams, v)
		}
	}
	return info, nil
}

func (b *Backend) DHCPReservations(_ context.Context) ([]backend.DHCPReservation, error) {
	if b.snapshot.NVRAM == nil {
		return nil, nil
	}
	return parseDHCPStaticList(b.snapshot.NVRAM["dhcp_staticlist"]), nil
}

func (b *Backend) QoSStatus(_ context.Context) (backend.QoSInfo, error) {
	nv := b.snapshot.NVRAM
	if nv == nil {
		return backend.QoSInfo{}, nil
	}
	info := backend.QoSInfo{
		Enabled: nv["qos_enable"] == "1",
		ObwKbps: nv["qos_obw"],
		IbwKbps: nv["qos_ibw"],
	}
	if m := nv["qos_method"]; m != "" {
		info.Mode = m
		if n, err := strconv.Atoi(m); err == nil {
			info.Method = n
		}
	}
	return info, nil
}

func (b *Backend) VPNProfiles(_ context.Context) ([]backend.VPNProfile, error) {
	nv := b.snapshot.NVRAM
	if nv == nil {
		return nil, nil
	}
	var out []backend.VPNProfile
	for i := 1; i <= 5; i++ {
		prefix := fmt.Sprintf("wgc%d_", i)
		ep := nv[prefix+"ep_addr"]
		if ep == "" && nv[prefix+"priv"] == "" && nv[prefix+"ppub"] == "" && nv[prefix+"enable"] == "" {
			continue
		}
		endpoint := ep
		if port := nv[prefix+"ep_port"]; port != "" && port != "0" {
			endpoint = ep + ":" + port
		}
		out = append(out, backend.VPNProfile{
			Name:        fmt.Sprintf("wgc%d", i),
			Type:        "wireguard",
			Description: nv[prefix+"desc"],
			Enabled:     nv[prefix+"enable"] == "1",
			Endpoint:    endpoint,
			KillSwitch:  nv[prefix+"fw"] == "1",
		})
	}
	for i := 1; i <= 5; i++ {
		desc := nv[fmt.Sprintf("vpn_client%d_desc", i)]
		addr := nv[fmt.Sprintf("vpn_client%d_addr", i)]
		state := nv[fmt.Sprintf("vpn_client%d_state", i)]
		if desc == "" && addr == "" && state == "" {
			continue
		}
		out = append(out, backend.VPNProfile{
			Name:        fmt.Sprintf("ovpnc%d", i),
			Type:        "openvpn",
			Description: desc,
			Enabled:     state == "2" || state == "1",
			Endpoint:    addr,
			KillSwitch:  nv[fmt.Sprintf("vpn_client%d_enforce", i)] == "1",
		})
	}
	return out, nil
}

func (b *Backend) PolicyRoutes(_ context.Context) ([]backend.PolicyRoute, error) {
	if b.snapshot.NVRAM == nil {
		return nil, nil
	}
	return parseVPNDirectorRuleList(b.snapshot.NVRAM["vpndirector_rulelist"]), nil
}

func fieldsOrNil(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

func appendUnique(slice []string, v string) []string {
	for _, x := range slice {
		if x == v {
			return slice
		}
	}
	return append(slice, v)
}

func parseDHCPStaticList(raw string) []backend.DHCPReservation {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []backend.DHCPReservation
	for _, part := range strings.Split(raw, "<") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, ">")
		if len(fields) < 2 {
			continue
		}
		r := backend.DHCPReservation{
			MAC: strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(fields[0]), "-", ":")),
			IP:  strings.TrimSpace(fields[1]),
		}
		if len(fields) > 2 {
			r.Hostname = strings.TrimSpace(fields[2])
		}
		if r.MAC == "" || r.IP == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

func parseVPNDirectorRuleList(raw string) []backend.PolicyRoute {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []backend.PolicyRoute
	idx := 0
	for _, part := range strings.Split(raw, "<") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, ">")
		if len(fields) < 5 {
			continue
		}
		idx++
		out = append(out, backend.PolicyRoute{
			ID:          strconv.Itoa(idx),
			Enabled:     fields[0] == "1",
			Description: fields[1],
			Source:      fields[2],
			Interface:   fields[4],
		})
	}
	return out
}

var _ backend.RouterBackend = (*Backend)(nil)
