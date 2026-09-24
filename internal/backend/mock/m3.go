package mock

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/reece01-dock/axos/internal/backend"
)

// --- Diagnostics -----------------------------------------------------------

func (b *Backend) Ping(_ context.Context, host string, count int) (backend.DiagResult, error) {
	if count <= 0 {
		count = 4
	}
	// Deterministic fake RTT from host length so /v1/vpn/benchmark sorts stably in tests.
	avg := float64(len(host)) + 0.5
	cmd := fmt.Sprintf("ping -c %d %s", count, host)
	out := fmt.Sprintf("[mock] PING %s: %d packets transmitted, %d received\nrtt min/avg/max/mdev = 1.0/%.1f/9.0/0.1 ms", host, count, count, avg)
	return backend.DiagResult{
		Command:  cmd,
		OK:       true,
		Output:   out,
		Duration: 12 * time.Millisecond,
	}, nil
}

func (b *Backend) Traceroute(_ context.Context, host string, maxHops int) (backend.DiagResult, error) {
	cmd := "traceroute"
	if maxHops > 0 {
		cmd = fmt.Sprintf("traceroute -m %d %s", maxHops, host)
	} else {
		cmd = fmt.Sprintf("traceroute %s", host)
	}
	return backend.DiagResult{
		Command:  cmd,
		OK:       true,
		Output:   fmt.Sprintf("[mock] traceroute to %s\n 1  192.168.1.1  1.2 ms\n 2  %s  12.0 ms", host, host),
		Duration: 25 * time.Millisecond,
	}, nil
}

func (b *Backend) DNSLookup(_ context.Context, name string) (backend.DiagResult, error) {
	return backend.DiagResult{
		Command:  "nslookup " + name,
		OK:       true,
		Output:   fmt.Sprintf("[mock] Name: %s\nAddress: 203.0.113.10", name),
		Duration: 8 * time.Millisecond,
	}, nil
}

func (b *Backend) PortCheck(_ context.Context, host string, port int) (backend.DiagResult, error) {
	return backend.DiagResult{
		Command:  fmt.Sprintf("nc -z -w 3 %s %d", host, port),
		OK:       true,
		Output:   fmt.Sprintf("[mock] Connection to %s %d port [tcp/*] succeeded!", host, port),
		Duration: 5 * time.Millisecond,
	}, nil
}

func (b *Backend) Iperf3(_ context.Context, opts backend.IperfOpts) (backend.PerfResult, error) {
	mode := opts.Mode
	if mode == "" {
		mode = "client"
	}
	summary := fmt.Sprintf("[mock] iperf3 %s OK ~940 Mbits/sec", mode)
	return backend.PerfResult{
		Tool:     "iperf3",
		OK:       true,
		Summary:  summary,
		Output:   summary,
		Duration: time.Duration(opts.Seconds)*time.Second + 50*time.Millisecond,
	}, nil
}

// --- DNS -------------------------------------------------------------------

func (b *Backend) DNSConfig(_ context.Context) (backend.DNSInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneDNS(b.dns), nil
}

func (b *Backend) SetDNSConfig(_ context.Context, cfg backend.DNSInfo) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dns = cloneDNS(cfg)
	b.dns.WANDNSAuto = len(cfg.WANUpstreams) == 0
	return nil
}

func cloneDNS(d backend.DNSInfo) backend.DNSInfo {
	out := d
	if d.WANUpstreams != nil {
		out.WANUpstreams = append([]string(nil), d.WANUpstreams...)
	}
	if d.LANUpstreams != nil {
		out.LANUpstreams = append([]string(nil), d.LANUpstreams...)
	}
	return out
}

// --- DHCP ------------------------------------------------------------------

func (b *Backend) DHCPReservations(_ context.Context) ([]backend.DHCPReservation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.DHCPReservation, len(b.dhcp))
	copy(out, b.dhcp)
	return out, nil
}

func (b *Backend) SetDHCPReservation(_ context.Context, r backend.DHCPReservation) error {
	mac := normalizeMAC(r.MAC)
	if mac == "" || strings.TrimSpace(r.IP) == "" {
		return fmt.Errorf("backend/mock: dhcp reservation requires mac and ip")
	}
	r.MAC = mac
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.dhcp {
		if normalizeMAC(b.dhcp[i].MAC) == mac {
			b.dhcp[i] = r
			return nil
		}
	}
	b.dhcp = append(b.dhcp, r)
	return nil
}

func (b *Backend) DeleteDHCPReservation(_ context.Context, mac string) error {
	mac = normalizeMAC(mac)
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.dhcp[:0]
	for _, r := range b.dhcp {
		if normalizeMAC(r.MAC) != mac {
			out = append(out, r)
		}
	}
	b.dhcp = out
	return nil
}

func normalizeMAC(mac string) string {
	mac = strings.TrimSpace(mac)
	mac = strings.ReplaceAll(mac, "-", ":")
	return strings.ToUpper(mac)
}

// --- QoS -------------------------------------------------------------------

func (b *Backend) QoSStatus(_ context.Context) (backend.QoSInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.qos, nil
}

func (b *Backend) SetQoSEnable(_ context.Context, enabled bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.qos.Enabled = enabled
	return nil
}

// --- VPN -------------------------------------------------------------------

func (b *Backend) VPNProfiles(_ context.Context) ([]backend.VPNProfile, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.VPNProfile, 0, len(b.vpnProfOrd))
	for _, name := range b.vpnProfOrd {
		out = append(out, b.vpnProfiles[name])
	}
	return out, nil
}

func (b *Backend) ImportWireGuard(_ context.Context, p backend.WireGuardImport) error {
	if p.Unit < 1 || p.Unit > 5 {
		return fmt.Errorf("backend/mock: wireguard unit must be 1..5, got %d", p.Unit)
	}
	name := fmt.Sprintf("wgc%d", p.Unit)
	port := p.EndpointPort
	if port <= 0 {
		port = 51820
	}
	profile := backend.VPNProfile{
		Name:        name,
		Type:        "wireguard",
		Description: p.Description,
		Enabled:     false,
		Endpoint:    fmt.Sprintf("%s:%d", p.Endpoint, port),
		KillSwitch:  p.KillSwitch,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.vpnProfiles[name]; !exists {
		b.vpnProfOrd = append(b.vpnProfOrd, name)
	}
	b.vpnProfiles[name] = profile
	b.vpnSecrets[name] = p.PrivateKey
	return nil
}

func (b *Backend) VPNUp(_ context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.vpnProfiles[name]
	if !ok {
		return fmt.Errorf("backend/mock: unknown vpn profile %q", name)
	}
	p.Enabled = true
	b.vpnProfiles[name] = p
	if _, exists := b.vpn[name]; !exists {
		b.vpnOrder = append(b.vpnOrder, name)
	}
	b.vpn[name] = backend.VPNTunnel{
		Name: name, Type: p.Type, Interface: name, Up: true,
	}
	return nil
}

func (b *Backend) VPNDown(_ context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.vpnProfiles[name]
	if !ok {
		return fmt.Errorf("backend/mock: unknown vpn profile %q", name)
	}
	p.Enabled = false
	b.vpnProfiles[name] = p
	if t, ok := b.vpn[name]; ok {
		t.Up = false
		b.vpn[name] = t
	}
	return nil
}

// --- Firewall mutations ----------------------------------------------------

func (b *Backend) FirewallApply(_ context.Context, rule backend.FirewallRule) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if rule.Table == "" {
		rule.Table = "filter"
	}
	b.firewall = append(b.firewall, rule)
	return nil
}

func (b *Backend) FirewallDelete(_ context.Context, rule backend.FirewallRule) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if rule.Table == "" {
		rule.Table = "filter"
	}
	out := b.firewall[:0]
	removed := false
	for _, r := range b.firewall {
		if !removed && r.Table == rule.Table && r.Chain == rule.Chain && r.Rule == rule.Rule {
			removed = true
			continue
		}
		out = append(out, r)
	}
	b.firewall = out
	if !removed {
		return fmt.Errorf("backend/mock: firewall rule not found")
	}
	return nil
}

// --- Policy routes ---------------------------------------------------------

func (b *Backend) PolicyRoutes(_ context.Context) ([]backend.PolicyRoute, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.PolicyRoute, len(b.policy))
	copy(out, b.policy)
	return out, nil
}

func (b *Backend) SetPolicyRoute(_ context.Context, r backend.PolicyRoute) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.ID == "" {
		r.ID = strconv.Itoa(len(b.policy) + 1)
		b.policy = append(b.policy, r)
		return nil
	}
	for i := range b.policy {
		if b.policy[i].ID == r.ID {
			b.policy[i] = r
			return nil
		}
	}
	return fmt.Errorf("mock: set policy route: no rule with id %q", r.ID)
}

func (b *Backend) ReplacePolicyRoutes(_ context.Context, routes []backend.PolicyRoute) error {
	if err := backend.ValidatePolicyRoutes(routes); err != nil {
		return fmt.Errorf("mock: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.policy = make([]backend.PolicyRoute, len(routes))
	copy(b.policy, routes)
	for i := range b.policy {
		b.policy[i].ID = strconv.Itoa(i + 1)
	}
	return nil
}

func (b *Backend) DeletePolicyRoute(_ context.Context, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]backend.PolicyRoute, 0, len(b.policy))
	for _, r := range b.policy {
		if r.ID != id {
			out = append(out, r)
		}
	}
	if len(out) == len(b.policy) {
		return fmt.Errorf("mock: delete policy route: no rule with id %q", id)
	}
	for i := range out {
		out[i].ID = strconv.Itoa(i + 1)
	}
	b.policy = out
	return nil
}
