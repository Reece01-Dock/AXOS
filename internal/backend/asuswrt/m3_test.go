package asuswrt

import (
	"context"
	"strings"
	"testing"

	"github.com/reece01-dock/axos/internal/backend"
)

func TestParseDHCPStaticList(t *testing.T) {
	got := parseDHCPStaticList("<11:22:33:44:55:66>192.168.1.50>gaming-pc<AA:BB:CC:DD:EE:FF>192.168.1.51>")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2; got %+v", len(got), got)
	}
	if got[0].MAC != "11:22:33:44:55:66" || got[0].IP != "192.168.1.50" || got[0].Hostname != "gaming-pc" {
		t.Errorf("entry0 = %+v", got[0])
	}
	if got[1].MAC != "AA:BB:CC:DD:EE:FF" || got[1].IP != "192.168.1.51" || got[1].Hostname != "" {
		t.Errorf("entry1 = %+v", got[1])
	}
}

func TestParseDHCPStaticList_Empty(t *testing.T) {
	got := parseDHCPStaticList("")
	if got == nil || len(got) != 0 {
		t.Fatalf("empty = %v, want non-nil empty slice", got)
	}
}

func TestFormatDHCPStaticList_RoundTrip(t *testing.T) {
	in := []backend.DHCPReservation{
		{MAC: "aa-bb-cc-dd-ee-ff", IP: "192.168.1.10", Hostname: "nas"},
		{MAC: "11:22:33:44:55:66", IP: "192.168.1.20"},
	}
	raw := formatDHCPStaticList(in)
	got := parseDHCPStaticList(raw)
	if len(got) != 2 {
		t.Fatalf("roundtrip len = %d; raw=%q", len(got), raw)
	}
	if got[0].MAC != "AA:BB:CC:DD:EE:FF" || got[0].Hostname != "nas" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].MAC != "11:22:33:44:55:66" || got[1].IP != "192.168.1.20" {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestParseVPNDirectorRuleList(t *testing.T) {
	got := parseVPNDirectorRuleList("<1>Chromecast>192.168.1.50>>WGC1<0>Phone>192.168.1.51>>WAN")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2; got %+v", len(got), got)
	}
	if !got[0].Enabled || got[0].Description != "Chromecast" || got[0].Source != "192.168.1.50" || got[0].Interface != "WGC1" {
		t.Errorf("entry0 = %+v", got[0])
	}
	if got[0].ID != "1" || got[1].ID != "2" {
		t.Errorf("ids = %q/%q, want 1/2", got[0].ID, got[1].ID)
	}
	if got[1].Enabled {
		t.Errorf("entry1 should be disabled: %+v", got[1])
	}
}

func TestFormatVPNDirectorRuleList_RoundTrip(t *testing.T) {
	in := []backend.PolicyRoute{
		{Enabled: true, Description: "TV", Source: "192.168.1.9", Interface: "WGC1"},
	}
	got := parseVPNDirectorRuleList(formatVPNDirectorRuleList(in))
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if !got[0].Enabled || got[0].Interface != "WGC1" || got[0].Source != "192.168.1.9" {
		t.Errorf("got = %+v", got[0])
	}
}

func TestVPNDirectorRuleList_PreservesRemote(t *testing.T) {
	raw := "<1>Work>192.168.1.9>10.0.0.0/8>OVPN1<1>TV>192.168.1.10>>WGC5"
	got := parseVPNDirectorRuleList(raw)
	if len(got) != 2 || got[0].Remote != "10.0.0.0/8" || got[1].Remote != "" {
		t.Fatalf("got %+v", got)
	}
	if back := formatVPNDirectorRuleList(got); back != raw {
		t.Errorf("round trip = %q, want %q", back, raw)
	}
}

func TestDNSInfoFromNVRAM(t *testing.T) {
	nv := map[string]string{
		"wan0_dns":        "1.1.1.1 1.0.0.1",
		"dhcp_dns1_x":     "192.168.1.1",
		"dnspriv_enable":  "1",
		"dnspriv_profile": "cloudflare",
	}
	info := dnsInfoFromNVRAM(func(k string) string { return nv[k] })
	if len(info.WANUpstreams) != 2 || info.WANUpstreams[0] != "1.1.1.1" {
		t.Errorf("WAN = %v", info.WANUpstreams)
	}
	if !info.DoTEnabled || info.DoTProfile != "cloudflare" {
		t.Errorf("DoT = %+v", info)
	}
	if len(info.LANUpstreams) != 1 || info.LANUpstreams[0] != "192.168.1.1" {
		t.Errorf("LAN = %v", info.LANUpstreams)
	}
}

func TestVPNProfilesFromNVRAM_OmitsPrivateKeys(t *testing.T) {
	nv := map[string]string{
		"wgc1_enable":       "1",
		"wgc1_ep_addr":      "vpn.example.com",
		"wgc1_ep_port":      "51820",
		"wgc1_priv":         "SECRETPRIVATEKEY",
		"wgc1_ppub":         "peerpub",
		"wgc1_desc":         "Home",
		"wgc1_fw":           "1",
		"vpn_client2_desc":  "Work",
		"vpn_client2_addr":  "ovpn.example.com",
		"vpn_client2_state": "2",
	}
	profiles := vpnProfilesFromNVRAM(func(k string) string { return nv[k] })
	if len(profiles) != 2 {
		t.Fatalf("profiles = %+v, want 2", profiles)
	}
	if profiles[0].Name != "wgc1" || profiles[0].Endpoint != "vpn.example.com:51820" || !profiles[0].KillSwitch {
		t.Errorf("wg = %+v", profiles[0])
	}
	if profiles[1].Name != "ovpnc2" || profiles[1].Type != "openvpn" {
		t.Errorf("ovpn = %+v", profiles[1])
	}
	for _, p := range profiles {
		blob := p.Name + p.Type + p.Description + p.Endpoint
		if strings.Contains(blob, "SECRETPRIVATEKEY") {
			t.Fatal("VPNProfiles must never surface private keys")
		}
	}
}

func TestParseVPNName(t *testing.T) {
	kind, unit, err := parseVPNName("wgc3")
	if err != nil || kind != "wgc" || unit != 3 {
		t.Errorf("wgc3 -> %s %d %v", kind, unit, err)
	}
	kind, unit, err = parseVPNName("ovpnc1")
	if err != nil || kind != "ovpnc" || unit != 1 {
		t.Errorf("ovpnc1 -> %s %d %v", kind, unit, err)
	}
	if _, _, err := parseVPNName("wg0"); err == nil {
		t.Fatal("wg0 should be rejected")
	}
}

func TestReplacePolicyRoutes_SingleCommitAndRestart(t *testing.T) {
	r := &fakeRunner{}
	b := newTestBackend(t, r)
	routes := []backend.PolicyRoute{
		{Enabled: true, Description: "a", Source: "192.168.50.11", Interface: "wgc5"},
		{Enabled: true, Description: "b", Source: "192.168.50.12", Interface: "ovpnc1"},
		{Enabled: true, Description: "c", Source: "192.168.50.0/28", Interface: "WAN"},
	}
	if err := b.ReplacePolicyRoutes(context.Background(), routes); err != nil {
		t.Fatalf("ReplacePolicyRoutes: %v", err)
	}
	var sets, commits, restarts int
	for _, c := range r.calls {
		switch {
		case strings.HasPrefix(c, "nvram set vpndirector_rulelist="):
			sets++
			if !strings.Contains(c, ">WGC5<") || !strings.Contains(c, ">OVPN1<") {
				t.Errorf("interfaces not normalised: %s", c)
			}
		case c == "nvram commit":
			commits++
		case c == "service restart_vpnrouting0":
			restarts++
		}
	}
	if sets != 1 || commits != 1 || restarts != 1 {
		t.Fatalf("sets=%d commits=%d restarts=%d, want 1/1/1; calls=%v", sets, commits, restarts, r.calls)
	}
}

func TestReplacePolicyRoutes_RejectsDuplicateSource(t *testing.T) {
	r := &fakeRunner{}
	b := newTestBackend(t, r)
	err := b.ReplacePolicyRoutes(context.Background(), []backend.PolicyRoute{
		{Source: "192.168.50.9", Interface: "WGC1"},
		{Source: "192.168.50.9", Interface: "WAN"},
	})
	if err == nil {
		t.Fatal("want duplicate-source error")
	}
	if len(r.calls) != 0 {
		t.Fatalf("nothing should be written on validation failure, got %v", r.calls)
	}
}

func TestSetPolicyRoute_UnknownIDIsError(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, string, int) {
		return "<1>TV>192.168.50.9>>WGC1", "", 0
	}}
	b := newTestBackend(t, r)
	if err := b.SetPolicyRoute(context.Background(), backend.PolicyRoute{ID: "7", Source: "x", Interface: "WAN"}); err == nil {
		t.Fatal("want error for unknown id")
	}
}
