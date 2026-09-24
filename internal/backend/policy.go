package backend

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// MaxPolicyRoutes is Merlin's VPN Director rule limit.
const MaxPolicyRoutes = 199

// NormalizeDirectorIface maps API interface names (wan, wgc5, ovpnc1, OVPN1)
// to Merlin VPN Director tokens (WAN, WGC5, OVPN1). Unknown names are
// upper-cased and passed through; an empty name stays empty.
func NormalizeDirectorIface(iface string) string {
	up := strings.ToUpper(strings.TrimSpace(iface))
	switch {
	case up == "":
		return ""
	case strings.HasPrefix(up, "OVPNC") && len(up) > len("OVPNC"):
		return "OVPN" + strings.TrimPrefix(up, "OVPNC")
	}
	return up
}

// PolicySourceKey is the comparison key for a policy route source: MACs,
// IPs and CIDRs compare case- and whitespace-insensitively.
func PolicySourceKey(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}

// policyKey identifies one rule slot: VPN Director may legitimately hold
// several rules for one source that differ only by remote (destination).
func policyKey(r PolicyRoute) string {
	return PolicySourceKey(r.Source) + ">" + PolicySourceKey(r.Remote)
}

// PolicyConflict describes an existing rule that a policy change would
// overwrite with a different interface or description.
type PolicyConflict struct {
	Existing PolicyRoute `json:"existing"`
	Proposed PolicyRoute `json:"proposed"`
}

// PolicyMergeResult is the outcome of MergePolicyRoutes.
type PolicyMergeResult struct {
	Routes    []PolicyRoute    `json:"-"`
	Added     int              `json:"added"`
	Replaced  int              `json:"replaced"`
	Unchanged int              `json:"unchanged"`
	Conflicts []PolicyConflict `json:"conflicts,omitempty"`
}

// MergePolicyRoutes upserts updates into existing by (source, remote), one
// rule per pair. A source that already has a rule with a different interface,
// description, enabled state or kill switch is a conflict: it is reported,
// and it is only overwritten when replace is true. The returned Routes are
// renumbered 1..N (the Merlin rule list is positional).
func MergePolicyRoutes(existing, updates []PolicyRoute, replace bool) PolicyMergeResult {
	out := make([]PolicyRoute, len(existing))
	copy(out, existing)
	bySource := make(map[string]int, len(out))
	for i := range out {
		if PolicySourceKey(out[i].Source) != "" {
			k := policyKey(out[i])
			if _, dup := bySource[k]; !dup {
				bySource[k] = i
			}
		}
	}
	var res PolicyMergeResult
	for _, u := range updates {
		u.Interface = NormalizeDirectorIface(u.Interface)
		u.Source = strings.TrimSpace(u.Source)
		u.Remote = strings.TrimSpace(u.Remote)
		k := policyKey(u)
		i, ok := bySource[k]
		if !ok {
			bySource[k] = len(out)
			out = append(out, u)
			res.Added++
			continue
		}
		cur := out[i]
		if samePolicy(cur, u) {
			res.Unchanged++
			continue
		}
		u.ID = cur.ID
		res.Conflicts = append(res.Conflicts, PolicyConflict{Existing: cur, Proposed: u})
		if replace {
			out[i] = u
			res.Replaced++
		}
	}
	for i := range out {
		out[i].ID = strconv.Itoa(i + 1)
	}
	res.Routes = out
	return res
}

// RemovePolicySources drops every rule whose source matches one of sources
// (whatever its remote) and renumbers the rest. It returns the new list and how many were removed.
func RemovePolicySources(existing []PolicyRoute, sources []string) ([]PolicyRoute, int) {
	drop := make(map[string]bool, len(sources))
	for _, s := range sources {
		if k := PolicySourceKey(s); k != "" {
			drop[k] = true
		}
	}
	out := make([]PolicyRoute, 0, len(existing))
	for _, r := range existing {
		if !drop[PolicySourceKey(r.Source)] {
			out = append(out, r)
		}
	}
	for i := range out {
		out[i].ID = strconv.Itoa(i + 1)
	}
	return out, len(existing) - len(out)
}

// ValidatePolicyRoutes checks a full rule list before it is written: every
// rule needs an IP/CIDR source (VPN Director matches on "Local IP" — it
// cannot match a MAC; see ResolvePolicySources) and an interface, the
// remote must be empty or an IP/CIDR, and no (source, remote) pair may
// appear twice (VPN Director is first-match, so a duplicate silently
// shadows).
func ValidatePolicyRoutes(routes []PolicyRoute) error {
	if len(routes) > MaxPolicyRoutes {
		return fmt.Errorf("VPN Director holds at most %d rules, got %d", MaxPolicyRoutes, len(routes))
	}
	seen := make(map[string]bool, len(routes))
	for i, r := range routes {
		if PolicySourceKey(r.Source) == "" {
			return fmt.Errorf("policy rule %d: source is required", i+1)
		}
		if !isIPOrCIDR(r.Source) {
			return fmt.Errorf("policy rule %d: source %q must be an IP address or CIDR", i+1, r.Source)
		}
		if strings.TrimSpace(r.Remote) != "" && !isIPOrCIDR(r.Remote) {
			return fmt.Errorf("policy rule %d: remote %q must be an IP address or CIDR", i+1, r.Remote)
		}
		if NormalizeDirectorIface(r.Interface) == "" {
			return fmt.Errorf("policy rule %d (%s): interface is required", i+1, r.Source)
		}
		if strings.ContainsAny(r.Source+r.Remote+r.Description+r.Interface, "<>") {
			return fmt.Errorf("policy rule %d (%s): '<' and '>' are not allowed", i+1, r.Source)
		}
		k := policyKey(r)
		if seen[k] {
			return fmt.Errorf("policy rule %d: duplicate source %s", i+1, r.Source)
		}
		seen[k] = true
	}
	return nil
}

func samePolicy(a, b PolicyRoute) bool {
	return NormalizeDirectorIface(a.Interface) == NormalizeDirectorIface(b.Interface) &&
		a.Description == b.Description &&
		a.Enabled == b.Enabled &&
		a.KillSwitch == b.KillSwitch
}

func isIPOrCIDR(s string) bool {
	s = strings.TrimSpace(s)
	if net.ParseIP(s) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// ResolvePolicySources turns each source into something VPN Director can
// match: IPs and CIDRs pass through, MAC addresses are looked up in clients
// and replaced by that device's current IP. A MAC with no known IP is an
// error (the device is offline or unknown), so no rule is silently dropped.
func ResolvePolicySources(sources []string, clients []Client) ([]string, error) {
	byMAC := make(map[string]string, len(clients))
	for _, c := range clients {
		if hw, err := net.ParseMAC(strings.TrimSpace(c.MAC)); err == nil && c.IP != "" {
			byMAC[hw.String()] = c.IP
		}
	}
	out := make([]string, 0, len(sources))
	var missing []string
	for _, src := range sources {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		if isIPOrCIDR(src) {
			out = append(out, src)
			continue
		}
		hw, err := net.ParseMAC(src)
		if err != nil {
			return nil, fmt.Errorf("source %q is not an IP, CIDR or MAC address", src)
		}
		ip, ok := byMAC[hw.String()]
		if !ok {
			missing = append(missing, src)
			continue
		}
		out = append(out, ip)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("no current IP for %s (device offline or unknown) — VPN Director rules need an IP", strings.Join(missing, ", "))
	}
	return out, nil
}
