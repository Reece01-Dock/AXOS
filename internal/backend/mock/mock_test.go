package mock

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/reece01-dock/axos/internal/backend"
)

func TestNew_HasPlausibleDefaults(t *testing.T) {
	b := New()
	ctx := context.Background()

	info, err := b.Info(ctx)
	if err != nil || info.Model != "GT-AX6000" {
		t.Fatalf("Info() = %+v, %v; want Model GT-AX6000", info, err)
	}

	ifaces, err := b.Interfaces(ctx)
	if err != nil || len(ifaces) == 0 {
		t.Fatalf("Interfaces() = %v, %v; want at least one", ifaces, err)
	}

	svcs, err := b.Services(ctx)
	if err != nil || len(svcs) == 0 {
		t.Fatalf("Services() = %v, %v; want at least one", svcs, err)
	}

	vpns, err := b.VPNStatus(ctx)
	if err != nil {
		t.Fatalf("VPNStatus() error = %v", err)
	}
	if len(vpns) != 0 {
		t.Fatalf("VPNStatus() = %v, want empty (stock router has nothing configured)", vpns)
	}

	rules, err := b.FirewallRules(ctx)
	if err != nil || len(rules) == 0 {
		t.Fatalf("FirewallRules() = %v, %v; want at least one", rules, err)
	}

	nv, err := b.NVRAMDump(ctx)
	if err != nil {
		t.Fatalf("NVRAMDump() error = %v", err)
	}
	if nv["productid"] != "GT-AX6000" {
		t.Fatalf("NVRAMDump()[productid] = %q, want GT-AX6000", nv["productid"])
	}
	if _, hasSecret := nv["wl1_wpa_psk"]; !hasSecret {
		t.Fatal("NVRAMDump() should include a secret-shaped key for capture-redaction tests to exercise")
	}
}

func TestMutators_AreVisibleThroughInterfaceMethods(t *testing.T) {
	b := New()
	ctx := context.Background()

	// Simulate a new client joining.
	b.SetClient(backend.Client{MAC: "AA:BB:CC:DD:EE:FF", IP: "192.168.1.99", Hostname: "new-device"})
	clients, err := b.Clients(ctx)
	if err != nil {
		t.Fatalf("Clients() error = %v", err)
	}
	found := false
	for _, c := range clients {
		if c.MAC == "AA:BB:CC:DD:EE:FF" {
			found = true
		}
	}
	if !found {
		t.Fatal("SetClient did not make the new client visible via Clients()")
	}

	b.RemoveClient("AA:BB:CC:DD:EE:FF")
	clients, _ = b.Clients(ctx)
	for _, c := range clients {
		if c.MAC == "AA:BB:CC:DD:EE:FF" {
			t.Fatal("RemoveClient did not remove the client")
		}
	}

	// Simulate configuring a WireGuard tunnel with a peer handshaking.
	b.SetVPNTunnel(backend.VPNTunnel{
		Name: "wg0", Type: "wireguard", Interface: "wg0", Up: true,
		Peers: []backend.VPNPeer{{PublicKey: "abc123", RxBytes: 1000, TxBytes: 2000}},
	})
	vpns, err := b.VPNStatus(ctx)
	if err != nil {
		t.Fatalf("VPNStatus() error = %v", err)
	}
	if len(vpns) != 1 || !vpns[0].Up || len(vpns[0].Peers) != 1 {
		t.Fatalf("VPNStatus() = %+v, want one up tunnel with one peer", vpns)
	}

	b.RemoveVPNTunnel("wg0")
	vpns, _ = b.VPNStatus(ctx)
	if len(vpns) != 0 {
		t.Fatalf("VPNStatus() after RemoveVPNTunnel = %v, want empty", vpns)
	}

	// Simulate a service crashing.
	b.SetService(backend.ServiceStatus{Name: "dnsmasq", Running: false})
	svcs, _ := b.Services(ctx)
	for _, s := range svcs {
		if s.Name == "dnsmasq" && s.Running {
			t.Fatal("SetService did not update dnsmasq's running state")
		}
	}

	// Simulate a thermal spike.
	b.SetResources(backend.Resources{TemperaturesC: map[string]float64{"cpu": 95.0}})
	res, _ := b.Resources(ctx)
	if res.TemperaturesC["cpu"] != 95.0 {
		t.Fatalf("Resources() after SetResources = %+v, want cpu=95.0", res)
	}

	// nvram mutation.
	b.SetNVRAM("test_key", "test_value")
	nv, _ := b.NVRAMDump(ctx)
	if nv["test_key"] != "test_value" {
		t.Fatal("SetNVRAM did not update NVRAMDump")
	}
	b.DeleteNVRAM("test_key")
	nv, _ = b.NVRAMDump(ctx)
	if _, ok := nv["test_key"]; ok {
		t.Fatal("DeleteNVRAM did not remove the key")
	}
}

func TestNVRAMDump_ReturnsACopy(t *testing.T) {
	b := New()
	ctx := context.Background()

	dump, err := b.NVRAMDump(ctx)
	if err != nil {
		t.Fatalf("NVRAMDump: %v", err)
	}
	dump["productid"] = "TAMPERED"

	dump2, _ := b.NVRAMDump(ctx)
	if dump2["productid"] != "GT-AX6000" {
		t.Fatal("mutating a returned NVRAMDump map affected the backend's internal state — it should return a copy")
	}
}

var _ backend.RouterBackend = (*Backend)(nil)

func TestSetDHCPReservation_RoundTrip(t *testing.T) {
	b := New()
	ctx := context.Background()

	r := backend.DHCPReservation{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.99", Hostname: "nas"}
	if err := b.SetDHCPReservation(ctx, r); err != nil {
		t.Fatalf("SetDHCPReservation: %v", err)
	}
	list, err := b.DHCPReservations(ctx)
	if err != nil {
		t.Fatalf("DHCPReservations: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %+v, want 1 entry", list)
	}
	if list[0].MAC != "AA:BB:CC:DD:EE:FF" || list[0].IP != "192.168.1.99" || list[0].Hostname != "nas" {
		t.Errorf("got %+v", list[0])
	}

	// Update in place by MAC.
	if err := b.SetDHCPReservation(ctx, backend.DHCPReservation{MAC: "AA:BB:CC:DD:EE:FF", IP: "192.168.1.100", Hostname: "nas2"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	list, _ = b.DHCPReservations(ctx)
	if len(list) != 1 || list[0].IP != "192.168.1.100" || list[0].Hostname != "nas2" {
		t.Fatalf("after update = %+v", list)
	}

	if err := b.DeleteDHCPReservation(ctx, "aa:bb:cc:dd:ee:ff"); err != nil {
		t.Fatalf("DeleteDHCPReservation: %v", err)
	}
	list, _ = b.DHCPReservations(ctx)
	if len(list) != 0 {
		t.Fatalf("after delete = %+v, want empty", list)
	}
}

func TestImportWireGuard_NoPrivateKeyInProfiles(t *testing.T) {
	b := New()
	ctx := context.Background()
	err := b.ImportWireGuard(ctx, backend.WireGuardImport{
		Unit: 1, PrivateKey: "SECRETKEY", PeerPublicKey: "peer",
		Endpoint: "vpn.example.com", EndpointPort: 51820, Address: "10.6.0.2/32",
		AllowedIPs: []string{"0.0.0.0/0"}, Description: "test",
	})
	if err != nil {
		t.Fatalf("ImportWireGuard: %v", err)
	}
	profiles, err := b.VPNProfiles(ctx)
	if err != nil {
		t.Fatalf("VPNProfiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "wgc1" {
		t.Fatalf("profiles = %+v", profiles)
	}
	if strings.Contains(fmt.Sprintf("%+v", profiles), "SECRETKEY") {
		t.Fatal("private key must not appear in VPNProfiles")
	}
}
