package mock

import "github.com/reece01-dock/axos/internal/backend"

// This file holds Backend's state-mutation API. Every exported method here
// locks, mutates, and unlocks; the unexported *Locked helpers assume the
// caller already holds b.mu (or, for the ones called from New, that no
// other goroutine can see b yet).
//
// These let tests and interactive `axosd --backend mock` sessions simulate
// a changing network — a client joining, a VPN peer handshaking, a service
// crashing — without a real router. See docs/development.md "MockBackend".

func (b *Backend) setInterfaceLocked(iface backend.Interface) {
	if _, exists := b.ifaces[iface.Name]; !exists {
		b.ifOrder = append(b.ifOrder, iface.Name)
	}
	b.ifaces[iface.Name] = iface
}

// SetInterface adds or replaces an interface by name.
func (b *Backend) SetInterface(iface backend.Interface) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.setInterfaceLocked(iface)
}

// RemoveInterface deletes an interface by name, if present.
func (b *Backend) RemoveInterface(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.ifaces[name]; !exists {
		return
	}
	delete(b.ifaces, name)
	for i, n := range b.ifOrder {
		if n == name {
			b.ifOrder = append(b.ifOrder[:i], b.ifOrder[i+1:]...)
			break
		}
	}
}

// SetRoutes replaces the full route table (all tables).
func (b *Backend) SetRoutes(routes []backend.Route) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.routes = append([]backend.Route(nil), routes...)
}

func (b *Backend) setClientLocked(c backend.Client) {
	b.clients[c.MAC] = c
}

// SetClient adds or replaces a client by MAC — simulates a device joining
// or its state changing (e.g. RSSI, IP renewal).
func (b *Backend) SetClient(c backend.Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.setClientLocked(c)
}

// RemoveClient deletes a client by MAC — simulates a device leaving.
func (b *Backend) RemoveClient(mac string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, mac)
}

func (b *Backend) setWiFiRadioLocked(r backend.WiFiRadio) {
	if _, exists := b.wifi[r.Interface]; !exists {
		b.wifiOrd = append(b.wifiOrd, r.Interface)
	}
	b.wifi[r.Interface] = r
}

// SetWiFiRadio adds or replaces a radio's state by interface name.
func (b *Backend) SetWiFiRadio(r backend.WiFiRadio) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.setWiFiRadioLocked(r)
}

func (b *Backend) setServiceLocked(s backend.ServiceStatus) {
	if _, exists := b.services[s.Name]; !exists {
		b.svcOrder = append(b.svcOrder, s.Name)
	}
	b.services[s.Name] = s
}

// SetService adds or replaces a service's running state by name — e.g.
// SetService(ServiceStatus{Name: "wireguard", Running: true}) to simulate
// starting a service, or Running: false to simulate a crash.
func (b *Backend) SetService(s backend.ServiceStatus) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.setServiceLocked(s)
}

// SetFirewallRules replaces the full simulated rule set.
func (b *Backend) SetFirewallRules(rules []backend.FirewallRule) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.firewall = append([]backend.FirewallRule(nil), rules...)
}

// SetVPNTunnel adds or replaces a VPN tunnel (and its peers) by name —
// simulates configuring a WireGuard/OpenVPN/WARP tunnel, or a peer
// handshaking (by re-setting with updated LastHandshake/Rx/TxBytes).
func (b *Backend) SetVPNTunnel(t backend.VPNTunnel) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.vpn[t.Name]; !exists {
		b.vpnOrder = append(b.vpnOrder, t.Name)
	}
	b.vpn[t.Name] = t
}

// RemoveVPNTunnel deletes a VPN tunnel by name.
func (b *Backend) RemoveVPNTunnel(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.vpn[name]; !exists {
		return
	}
	delete(b.vpn, name)
	for i, n := range b.vpnOrder {
		if n == name {
			b.vpnOrder = append(b.vpnOrder[:i], b.vpnOrder[i+1:]...)
			break
		}
	}
}

// SetResources replaces the simulated CPU/memory/temperature readings —
// useful for testing monitoring/health-check thresholds (e.g. simulate a
// thermal warning) without real hardware.
func (b *Backend) SetResources(r backend.Resources) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.res = r
}

// SetNVRAM sets or overwrites a single nvram key.
func (b *Backend) SetNVRAM(key, value string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nvram[key] = value
}

// DeleteNVRAM removes an nvram key.
func (b *Backend) DeleteNVRAM(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.nvram, key)
}
