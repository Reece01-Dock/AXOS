package asuswrt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/reece01-dock/axos/internal/backend"
)

// --- Diagnostics -----------------------------------------------------------

func (b *Backend) Ping(ctx context.Context, host string, count int) (backend.DiagResult, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return backend.DiagResult{}, fmt.Errorf("asuswrt: ping: host is required")
	}
	if count <= 0 {
		count = 4
	}
	return b.runDiag(ctx, time.Duration(count+5)*time.Second, "ping", "-c", strconv.Itoa(count), host)
}

func (b *Backend) Traceroute(ctx context.Context, host string, maxHops int) (backend.DiagResult, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return backend.DiagResult{}, fmt.Errorf("asuswrt: traceroute: host is required")
	}
	args := []string{}
	if maxHops > 0 {
		args = append(args, "-m", strconv.Itoa(maxHops))
	}
	args = append(args, host)
	timeout := 60 * time.Second
	if maxHops > 0 {
		timeout = time.Duration(maxHops*3+10) * time.Second
	}
	return b.runDiag(ctx, timeout, "traceroute", args...)
}

func (b *Backend) DNSLookup(ctx context.Context, name string) (backend.DiagResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return backend.DiagResult{}, fmt.Errorf("asuswrt: dnslookup: name is required")
	}
	return b.runDiag(ctx, 15*time.Second, "nslookup", name)
}

func (b *Backend) PortCheck(ctx context.Context, host string, port int) (backend.DiagResult, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return backend.DiagResult{}, fmt.Errorf("asuswrt: portcheck: host is required")
	}
	if port <= 0 || port > 65535 {
		return backend.DiagResult{}, fmt.Errorf("asuswrt: portcheck: invalid port %d", port)
	}
	portStr := strconv.Itoa(port)
	start := time.Now()
	// BusyBox nc has no -z; a successful connect to IPADDR PORT exits 0.
	cmd := fmt.Sprintf("nc -w 3 %s %s </dev/null", host, portStr)
	stdout, stderr, code, timedOut, err := b.runner.run(ctx, 8*time.Second, "sh", "-c", cmd)

	out := strings.TrimSpace(stdout)
	if se := strings.TrimSpace(stderr); se != "" {
		if out != "" {
			out += "\n"
		}
		out += se
	}
	res := backend.DiagResult{
		Command:  cmd,
		OK:       err == nil && !timedOut && code == 0,
		Output:   out,
		Duration: time.Since(start),
	}
	return res, nil
}

func (b *Backend) runDiag(ctx context.Context, timeout time.Duration, name string, args ...string) (backend.DiagResult, error) {
	start := time.Now()
	cmd := strings.TrimSpace(name + " " + strings.Join(args, " "))
	stdout, stderr, code, timedOut, err := b.runner.run(ctx, timeout, name, args...)
	out := strings.TrimSpace(stdout)
	if se := strings.TrimSpace(stderr); se != "" {
		if out != "" {
			out += "\n"
		}
		out += se
	}
	res := backend.DiagResult{
		Command:  cmd,
		OK:       err == nil && !timedOut && code == 0,
		Output:   out,
		Duration: time.Since(start),
	}
	if err != nil && !timedOut {
		return res, nil
	}
	return res, nil
}

func (b *Backend) Iperf3(ctx context.Context, opts backend.IperfOpts) (backend.PerfResult, error) {
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = "client"
	}
	args := []string{}
	switch mode {
	case "client":
		if strings.TrimSpace(opts.Target) == "" {
			return backend.PerfResult{}, fmt.Errorf("asuswrt: iperf3 client: target is required")
		}
		args = append(args, "-c", opts.Target)
	case "server":
		args = append(args, "-s", "-1") // one-off accept then exit when supported
	default:
		return backend.PerfResult{}, fmt.Errorf("asuswrt: iperf3: unknown mode %q (want client or server)", opts.Mode)
	}
	if opts.Port > 0 {
		args = append(args, "-p", strconv.Itoa(opts.Port))
	}
	secs := opts.Seconds
	if secs <= 0 {
		secs = 5
	}
	if mode == "client" {
		args = append(args, "-t", strconv.Itoa(secs))
	}
	if opts.Reverse {
		args = append(args, "-R")
	}
	if opts.UDP {
		args = append(args, "-u")
	}

	start := time.Now()
	timeout := time.Duration(secs+30) * time.Second
	stdout, stderr, code, timedOut, err := b.runner.run(ctx, timeout, "iperf3", args...)
	out := strings.TrimSpace(stdout)
	if se := strings.TrimSpace(stderr); se != "" {
		if out != "" {
			out += "\n"
		}
		out += se
	}
	summary := firstNonEmptyLine(out)
	if summary == "" {
		summary = "iperf3 finished"
	}
	res := backend.PerfResult{
		Tool:     "iperf3",
		OK:       err == nil && !timedOut && code == 0,
		Summary:  summary,
		Output:   out,
		Duration: time.Since(start),
	}
	if err != nil && !timedOut {
		return res, nil
	}
	return res, nil
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// --- DNS -------------------------------------------------------------------

func (b *Backend) DNSConfig(ctx context.Context) (backend.DNSInfo, error) {
	return dnsInfoFromNVRAM(func(key string) string {
		v, _ := b.nvramGet(ctx, key)
		return v
	}), nil
}

func (b *Backend) SetDNSConfig(ctx context.Context, cfg backend.DNSInfo) error {
	wan := strings.Join(cfg.WANUpstreams, " ")
	if err := b.nvramSet(ctx, "wan0_dns", wan); err != nil {
		return err
	}
	if err := b.nvramSet(ctx, "wan_dns", wan); err != nil {
		return err
	}
	lan1, lan2 := "", ""
	if len(cfg.LANUpstreams) > 0 {
		lan1 = cfg.LANUpstreams[0]
	}
	if len(cfg.LANUpstreams) > 1 {
		lan2 = cfg.LANUpstreams[1]
	}
	for _, kv := range [][2]string{
		{"dhcp_dns1_x", lan1},
		{"dhcp_dns2_x", lan2},
		{"lan_dns1_x", lan1},
		{"lan_dns2_x", lan2},
	} {
		if err := b.nvramSet(ctx, kv[0], kv[1]); err != nil {
			return err
		}
	}
	dot := "0"
	if cfg.DoTEnabled {
		dot = "1"
	}
	if err := b.nvramSet(ctx, "dnspriv_enable", dot); err != nil {
		return err
	}
	if err := b.nvramSet(ctx, "dnspriv_profile", cfg.DoTProfile); err != nil {
		return err
	}
	if err := b.nvramSet(ctx, "dnspriv_rulelist", cfg.DoTRules); err != nil {
		return err
	}
	return b.nvramCommit(ctx)
}

func dnsInfoFromNVRAM(get func(string) string) backend.DNSInfo {
	wan := get("wan0_dns")
	if wan == "" {
		wan = get("wan_dns")
	}
	info := backend.DNSInfo{
		WANUpstreams: splitDNSList(wan),
		DoTEnabled:   get("dnspriv_enable") == "1",
		DoTProfile:   get("dnspriv_profile"),
		DoTRules:     get("dnspriv_rulelist"),
	}
	for _, k := range []string{"dhcp_dns1_x", "lan_dns1_x", "dhcp_dns2_x", "lan_dns2_x"} {
		if v := strings.TrimSpace(get(k)); v != "" {
			info.LANUpstreams = appendUnique(info.LANUpstreams, v)
		}
	}
	return info
}

func splitDNSList(s string) []string {
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

// --- DHCP reservations -----------------------------------------------------

func (b *Backend) DHCPReservations(ctx context.Context) ([]backend.DHCPReservation, error) {
	raw, err := b.nvramGet(ctx, "dhcp_staticlist")
	if err != nil {
		return nil, err
	}
	return parseDHCPStaticList(raw), nil
}

func (b *Backend) SetDHCPReservation(ctx context.Context, r backend.DHCPReservation) error {
	mac := normalizeMAC(r.MAC)
	if mac == "" || strings.TrimSpace(r.IP) == "" {
		return fmt.Errorf("asuswrt: dhcp reservation requires mac and ip")
	}
	r.MAC = mac
	list, err := b.DHCPReservations(ctx)
	if err != nil {
		return err
	}
	found := false
	for i := range list {
		if normalizeMAC(list[i].MAC) == mac {
			list[i] = r
			found = true
			break
		}
	}
	if !found {
		list = append(list, r)
	}
	if err := b.nvramSet(ctx, "dhcp_staticlist", formatDHCPStaticList(list)); err != nil {
		return err
	}
	return b.nvramCommit(ctx)
}

func (b *Backend) DeleteDHCPReservation(ctx context.Context, mac string) error {
	mac = normalizeMAC(mac)
	if mac == "" {
		return fmt.Errorf("asuswrt: delete dhcp reservation: mac is required")
	}
	list, err := b.DHCPReservations(ctx)
	if err != nil {
		return err
	}
	out := list[:0]
	for _, r := range list {
		if normalizeMAC(r.MAC) != mac {
			out = append(out, r)
		}
	}
	if err := b.nvramSet(ctx, "dhcp_staticlist", formatDHCPStaticList(out)); err != nil {
		return err
	}
	return b.nvramCommit(ctx)
}

// parseDHCPStaticList parses Merlin dhcp_staticlist:
//
//	<MAC>IP>hostname<MAC>IP>hostname
func parseDHCPStaticList(raw string) []backend.DHCPReservation {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []backend.DHCPReservation{}
	}
	out := make([]backend.DHCPReservation, 0)
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
			MAC: normalizeMAC(fields[0]),
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

func formatDHCPStaticList(rs []backend.DHCPReservation) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteByte('<')
		b.WriteString(normalizeMAC(r.MAC))
		b.WriteByte('>')
		b.WriteString(strings.TrimSpace(r.IP))
		b.WriteByte('>')
		b.WriteString(strings.TrimSpace(r.Hostname))
	}
	return b.String()
}

func normalizeMAC(mac string) string {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return ""
	}
	mac = strings.ReplaceAll(mac, "-", ":")
	return strings.ToUpper(mac)
}

// --- QoS -------------------------------------------------------------------

func (b *Backend) QoSStatus(ctx context.Context) (backend.QoSInfo, error) {
	return qosInfoFromNVRAM(func(key string) string {
		v, _ := b.nvramGet(ctx, key)
		return v
	}), nil
}

func (b *Backend) SetQoSEnable(ctx context.Context, enabled bool) error {
	v := "0"
	if enabled {
		v = "1"
	}
	if err := b.nvramSet(ctx, "qos_enable", v); err != nil {
		return err
	}
	return b.nvramCommit(ctx)
}

func qosInfoFromNVRAM(get func(string) string) backend.QoSInfo {
	info := backend.QoSInfo{
		Enabled: get("qos_enable") == "1",
		ObwKbps: get("qos_obw"),
		IbwKbps: get("qos_ibw"),
	}
	if m := get("qos_method"); m != "" {
		if n, err := strconv.Atoi(m); err == nil {
			info.Method = n
		}
		info.Mode = m
	}
	return info
}

// --- VPN profiles ----------------------------------------------------------

func (b *Backend) VPNProfiles(ctx context.Context) ([]backend.VPNProfile, error) {
	return vpnProfilesFromNVRAM(func(key string) string {
		v, _ := b.nvramGet(ctx, key)
		return v
	}), nil
}

func (b *Backend) ImportWireGuard(ctx context.Context, p backend.WireGuardImport) error {
	if p.Unit < 1 || p.Unit > 5 {
		return fmt.Errorf("asuswrt: wireguard unit must be 1..5, got %d", p.Unit)
	}
	if strings.TrimSpace(p.PrivateKey) == "" || strings.TrimSpace(p.PeerPublicKey) == "" {
		return fmt.Errorf("asuswrt: wireguard import requires private_key and peer_public_key")
	}
	if strings.TrimSpace(p.Endpoint) == "" {
		return fmt.Errorf("asuswrt: wireguard import requires endpoint")
	}
	if err := b.writeVPNSecret(ctx, p.Unit, p.PrivateKey); err != nil {
		return err
	}

	prefix := fmt.Sprintf("wgc%d_", p.Unit)
	port := p.EndpointPort
	if port <= 0 {
		port = 51820
	}
	alive := p.Keepalive
	sets := [][2]string{
		{prefix + "desc", p.Description},
		{prefix + "priv", p.PrivateKey},
		{prefix + "ppub", p.PeerPublicKey},
		{prefix + "psk", p.PresharedKey},
		{prefix + "ep_addr", p.Endpoint},
		{prefix + "ep_port", strconv.Itoa(port)},
		{prefix + "addr", p.Address},
		{prefix + "aips", strings.Join(p.AllowedIPs, ",")},
		{prefix + "dns", p.DNS},
		{prefix + "alive", strconv.Itoa(alive)},
		{prefix + "nat", bool01(p.Nat)},
		{prefix + "fw", bool01(p.KillSwitch)},
	}
	if p.MTU > 0 {
		sets = append(sets, [2]string{prefix + "mtu", strconv.Itoa(p.MTU)})
	}
	for _, kv := range sets {
		if err := b.nvramSet(ctx, kv[0], kv[1]); err != nil {
			return err
		}
	}
	// Also keep wgc_unit pointing at this slot (Merlin UI convention).
	if err := b.nvramSet(ctx, "wgc_unit", strconv.Itoa(p.Unit)); err != nil {
		return err
	}
	return b.nvramCommit(ctx)
}

func (b *Backend) VPNUp(ctx context.Context, name string) error {
	kind, unit, err := parseVPNName(name)
	if err != nil {
		return err
	}
	switch kind {
	case "wgc":
		prefix := fmt.Sprintf("wgc%d_", unit)
		if err := b.nvramSet(ctx, prefix+"enable", "1"); err != nil {
			return err
		}
		if err := b.nvramCommit(ctx); err != nil {
			return err
		}
		// Merlin: service restart_wgc <unit>
		if _, err := b.run(ctx, 60*time.Second, "service", "restart_wgc", strconv.Itoa(unit)); err != nil {
			return fmt.Errorf("asuswrt: restart_wgc %d: %w", unit, err)
		}
		return nil
	case "ovpnc":
		if err := b.nvramSet(ctx, fmt.Sprintf("vpn_client%d_state", unit), "2"); err != nil {
			return err
		}
		if err := b.nvramCommit(ctx); err != nil {
			return err
		}
		action := fmt.Sprintf("start_vpnclient%d", unit)
		if _, err := b.run(ctx, 60*time.Second, "service", action); err != nil {
			return fmt.Errorf("asuswrt: %s: %w", action, err)
		}
		return nil
	default:
		return fmt.Errorf("asuswrt: vpn up: unsupported profile %q", name)
	}
}

func (b *Backend) VPNDown(ctx context.Context, name string) error {
	kind, unit, err := parseVPNName(name)
	if err != nil {
		return err
	}
	switch kind {
	case "wgc":
		prefix := fmt.Sprintf("wgc%d_", unit)
		if err := b.nvramSet(ctx, prefix+"enable", "0"); err != nil {
			return err
		}
		if err := b.nvramCommit(ctx); err != nil {
			return err
		}
		if _, err := b.run(ctx, 60*time.Second, "service", "stop_wgc", strconv.Itoa(unit)); err != nil {
			return fmt.Errorf("asuswrt: stop_wgc %d: %w", unit, err)
		}
		return nil
	case "ovpnc":
		action := fmt.Sprintf("stop_vpnclient%d", unit)
		if _, err := b.run(ctx, 60*time.Second, "service", action); err != nil {
			return fmt.Errorf("asuswrt: %s: %w", action, err)
		}
		return nil
	default:
		return fmt.Errorf("asuswrt: vpn down: unsupported profile %q", name)
	}
}

func parseVPNName(name string) (kind string, unit int, err error) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(name, "wgc"):
		unit, err = strconv.Atoi(strings.TrimPrefix(name, "wgc"))
		if err != nil || unit < 1 || unit > 5 {
			return "", 0, fmt.Errorf("asuswrt: invalid wireguard profile %q (want wgc1..wgc5)", name)
		}
		return "wgc", unit, nil
	case strings.HasPrefix(name, "ovpnc"):
		unit, err = strconv.Atoi(strings.TrimPrefix(name, "ovpnc"))
		if err != nil || unit < 1 || unit > 5 {
			return "", 0, fmt.Errorf("asuswrt: invalid openvpn profile %q (want ovpnc1..ovpnc5)", name)
		}
		return "ovpnc", unit, nil
	case strings.HasPrefix(name, "vpnclient"):
		unit, err = strconv.Atoi(strings.TrimPrefix(name, "vpnclient"))
		if err != nil || unit < 1 || unit > 5 {
			return "", 0, fmt.Errorf("asuswrt: invalid openvpn profile %q", name)
		}
		return "ovpnc", unit, nil
	default:
		return "", 0, fmt.Errorf("asuswrt: unrecognized vpn profile %q (want wgcN or ovpncN)", name)
	}
}

func vpnProfilesFromNVRAM(get func(string) string) []backend.VPNProfile {
	var out []backend.VPNProfile
	for i := 1; i <= 5; i++ {
		prefix := fmt.Sprintf("wgc%d_", i)
		ep := get(prefix + "ep_addr")
		priv := get(prefix + "priv")
		ppub := get(prefix + "ppub")
		enable := get(prefix + "enable")
		if ep == "" && priv == "" && ppub == "" && enable == "" {
			continue
		}
		port := get(prefix + "ep_port")
		endpoint := ep
		if port != "" && port != "0" {
			endpoint = ep + ":" + port
		}
		out = append(out, backend.VPNProfile{
			Name:        fmt.Sprintf("wgc%d", i),
			Type:        "wireguard",
			Description: get(prefix + "desc"),
			Enabled:     enable == "1",
			Endpoint:    endpoint,
			KillSwitch:  get(prefix+"fw") == "1",
		})
	}
	for i := 1; i <= 5; i++ {
		desc := get(fmt.Sprintf("vpn_client%d_desc", i))
		addr := get(fmt.Sprintf("vpn_client%d_addr", i))
		enforce := get(fmt.Sprintf("vpn_client%d_enforce", i))
		state := get(fmt.Sprintf("vpn_client%d_state", i))
		if desc == "" && addr == "" && state == "" {
			continue
		}
		out = append(out, backend.VPNProfile{
			Name:        fmt.Sprintf("ovpnc%d", i),
			Type:        "openvpn",
			Description: desc,
			Enabled:     state == "2" || state == "1",
			Endpoint:    addr,
			KillSwitch:  enforce == "1",
		})
	}
	return out
}

func bool01(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func (b *Backend) writeVPNSecret(ctx context.Context, unit int, privKey string) error {
	dir := filepath.Join(b.SecretsDir, "vpn")
	path := filepath.Join(dir, fmt.Sprintf("wgc%d.key", unit))
	if b.Host == "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("asuswrt: creating secrets dir %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return fmt.Errorf("asuswrt: chmod secrets dir %s: %w", dir, err)
		}
		if err := writeOwnerOnlyFile(path, []byte(privKey+"\n")); err != nil {
			return fmt.Errorf("asuswrt: writing vpn secret %s: %w", path, err)
		}
		return nil
	}
	// Remote (SSH) path: write via shell so Live Development Mode works.
	script := fmt.Sprintf(
		`umask 077; mkdir -p %q && chmod 700 %q && cat > %q <<'AXOS_WG_EOF'
%s
AXOS_WG_EOF
chmod 600 %q`,
		dir, dir, path, privKey, path,
	)
	if _, err := b.run(ctx, 10*time.Second, "sh", "-c", script); err != nil {
		return fmt.Errorf("asuswrt: writing remote vpn secret: %w", err)
	}
	return nil
}

// --- Firewall mutations ----------------------------------------------------

func (b *Backend) FirewallApply(ctx context.Context, rule backend.FirewallRule) error {
	return b.firewallMutate(ctx, "-A", rule)
}

func (b *Backend) FirewallDelete(ctx context.Context, rule backend.FirewallRule) error {
	return b.firewallMutate(ctx, "-D", rule)
}

func (b *Backend) firewallMutate(ctx context.Context, op string, rule backend.FirewallRule) error {
	table := rule.Table
	if table == "" {
		table = "filter"
	}
	if strings.TrimSpace(rule.Chain) == "" {
		return fmt.Errorf("asuswrt: firewall %s: chain is required", op)
	}
	args := []string{"-t", table, op, rule.Chain}
	args = append(args, strings.Fields(rule.Rule)...)
	if _, err := b.run(ctx, 5*time.Second, "iptables", args...); err != nil {
		return fmt.Errorf("asuswrt: iptables %s: %w", op, err)
	}
	return nil
}

// --- Policy routes (VPN Director) ------------------------------------------

func (b *Backend) PolicyRoutes(ctx context.Context) ([]backend.PolicyRoute, error) {
	raw, err := b.nvramGet(ctx, "vpndirector_rulelist")
	if err != nil {
		return nil, err
	}
	return parseVPNDirectorRuleList(raw), nil
}

func (b *Backend) SetPolicyRoute(ctx context.Context, r backend.PolicyRoute) error {
	list, err := b.PolicyRoutes(ctx)
	if err != nil {
		return err
	}
	if r.ID == "" {
		list = append(list, r)
	} else {
		found := false
		for i := range list {
			if list[i].ID == r.ID {
				list[i] = r
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("asuswrt: set policy route: no rule with id %q", r.ID)
		}
	}
	return b.ReplacePolicyRoutes(ctx, list)
}

// maxVPNDirectorRuleListLen mirrors Advanced_VPNDirector.asp's own check.
const maxVPNDirectorRuleListLen = 7999

// ReplacePolicyRoutes writes the whole VPN Director rule list with a single
// nvram commit and a single restart_vpnrouting0, so a bulk change costs one
// routing reload instead of one per device.
func (b *Backend) ReplacePolicyRoutes(ctx context.Context, routes []backend.PolicyRoute) error {
	list := make([]backend.PolicyRoute, len(routes))
	copy(list, routes)
	for i := range list {
		list[i].Interface = backend.NormalizeDirectorIface(list[i].Interface)
		list[i].ID = strconv.Itoa(i + 1)
	}
	if err := backend.ValidatePolicyRoutes(list); err != nil {
		return fmt.Errorf("asuswrt: %w", err)
	}
	value := formatVPNDirectorRuleList(list)
	if len(value) > maxVPNDirectorRuleListLen {
		return fmt.Errorf("asuswrt: VPN Director rule list is %d characters, over Merlin's %d limit — remove rules or shorten descriptions", len(value), maxVPNDirectorRuleListLen)
	}
	if err := b.nvramSet(ctx, "vpndirector_rulelist", value); err != nil {
		return err
	}
	if err := b.nvramCommit(ctx); err != nil {
		return err
	}
	return b.restartVPNRouting(ctx)
}

func (b *Backend) DeletePolicyRoute(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("asuswrt: delete policy route: id is required")
	}
	list, err := b.PolicyRoutes(ctx)
	if err != nil {
		return err
	}
	out := make([]backend.PolicyRoute, 0, len(list))
	for _, r := range list {
		if r.ID != id {
			out = append(out, r)
		}
	}
	if len(out) == len(list) {
		return fmt.Errorf("asuswrt: delete policy route: no rule with id %q", id)
	}
	return b.ReplacePolicyRoutes(ctx, out)
}

func (b *Backend) restartVPNRouting(ctx context.Context) error {
	// Merlin VPN Director applies via restart_vpnrouting0 (see Advanced_VPNDirector.asp).
	if _, err := b.run(ctx, 60*time.Second, "service", "restart_vpnrouting0"); err != nil {
		return fmt.Errorf("asuswrt: restart_vpnrouting0: %w", err)
	}
	return nil
}

// parseVPNDirectorRuleList parses Merlin vpndirector_rulelist:
//
//	<1>Description>192.168.1.50>>WGC1
//
// Fields: enabled, description, source, remote (often empty), interface.
func parseVPNDirectorRuleList(raw string) []backend.PolicyRoute {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []backend.PolicyRoute{}
	}
	out := make([]backend.PolicyRoute, 0)
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
			Remote:      fields[3],
			Interface:   fields[4],
		})
	}
	return out
}

func formatVPNDirectorRuleList(rs []backend.PolicyRoute) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteByte('<')
		b.WriteString(bool01(r.Enabled))
		b.WriteByte('>')
		b.WriteString(r.Description)
		b.WriteByte('>')
		b.WriteString(r.Source)
		b.WriteByte('>')
		b.WriteString(r.Remote)
		b.WriteByte('>')
		b.WriteString(r.Interface)
	}
	return b.String()
}
