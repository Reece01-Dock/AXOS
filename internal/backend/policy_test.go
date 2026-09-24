package backend

import "testing"

func TestNormalizeDirectorIface(t *testing.T) {
	cases := map[string]string{
		"": "", " wan ": "WAN", "WAN": "WAN",
		"wgc5": "WGC5", "WGC5": "WGC5",
		"ovpnc1": "OVPN1", "OVPN1": "OVPN1", "ovpn2": "OVPN2",
	}
	for in, want := range cases {
		if got := NormalizeDirectorIface(in); got != want {
			t.Errorf("NormalizeDirectorIface(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMergePolicyRoutes(t *testing.T) {
	existing := []PolicyRoute{
		{ID: "1", Enabled: true, Description: "mine", Source: "192.168.50.9", Interface: "OVPN1"},
		{ID: "2", Enabled: true, Description: "tv", Source: "aa:bb:cc:dd:ee:ff", Interface: "WGC5"},
		{ID: "3", Enabled: true, Description: "work", Source: "192.168.50.20", Remote: "10.0.0.0/8", Interface: "OVPN1"},
	}
	updates := []PolicyRoute{
		{Enabled: true, Description: "tv", Source: "AA:BB:CC:DD:EE:FF", Interface: "wgc5"}, // unchanged
		{Enabled: true, Description: "axos", Source: "192.168.50.9", Interface: "WGC5"},    // conflict
		{Enabled: true, Description: "axos", Source: "192.168.50.20", Interface: "WGC5"},   // new (other remote)
	}

	res := MergePolicyRoutes(existing, updates, false)
	if res.Unchanged != 1 || res.Added != 1 || res.Replaced != 0 || len(res.Conflicts) != 1 {
		t.Fatalf("no-replace result = %+v", res)
	}
	if res.Routes[0].Interface != "OVPN1" {
		t.Errorf("conflicting rule was overwritten without replace: %+v", res.Routes[0])
	}
	if len(res.Routes) != 4 || res.Routes[3].ID != "4" || res.Routes[2].Remote != "10.0.0.0/8" {
		t.Errorf("routes = %+v", res.Routes)
	}

	res = MergePolicyRoutes(existing, updates, true)
	if res.Replaced != 1 || res.Routes[0].Interface != "WGC5" || res.Routes[0].ID != "1" {
		t.Fatalf("replace result = %+v", res)
	}
	if existing[0].Interface != "OVPN1" {
		t.Error("MergePolicyRoutes mutated its input")
	}
}

func TestRemovePolicySources(t *testing.T) {
	existing := []PolicyRoute{
		{ID: "1", Source: "A"}, {ID: "2", Source: "b"}, {ID: "3", Source: "B", Remote: "1.2.3.4"}, {ID: "4", Source: "c"},
	}
	out, n := RemovePolicySources(existing, []string{" b "})
	if n != 2 || len(out) != 2 || out[1].Source != "c" || out[1].ID != "2" {
		t.Fatalf("out=%+v n=%d", out, n)
	}
}

func TestValidatePolicyRoutes(t *testing.T) {
	ok := []PolicyRoute{
		{Source: "192.168.50.9", Interface: "WAN"},
		{Source: "192.168.50.9", Remote: "10.0.0.0/8", Interface: "OVPN1"},
	}
	if err := ValidatePolicyRoutes(ok); err != nil {
		t.Fatalf("valid list rejected: %v", err)
	}
	bad := [][]PolicyRoute{
		{{Source: "", Interface: "WAN"}},
		{{Source: "10.0.0.1", Interface: ""}},
		{{Source: "10.0.0.1", Interface: "WAN", Description: "a<b"}},
		{{Source: "10.0.0.1", Interface: "WAN"}, {Source: " 10.0.0.1", Interface: "WGC1"}},
		{{Source: "AA:BB:CC:DD:EE:FF", Interface: "WAN"}},
		{{Source: "10.0.0.1", Remote: "example.com", Interface: "WAN"}},
	}
	for i, b := range bad {
		if err := ValidatePolicyRoutes(b); err == nil {
			t.Errorf("case %d: want error", i)
		}
	}
}

func TestResolvePolicySources(t *testing.T) {
	clients := []Client{{MAC: "11:22:33:44:55:66", IP: "192.168.50.20"}}
	got, err := ResolvePolicySources([]string{"11-22-33-44-55-66", "192.168.50.0/24", "10.0.0.1"}, clients)
	if err != nil || len(got) != 3 || got[0] != "192.168.50.20" {
		t.Fatalf("got %v err %v", got, err)
	}
	if _, err := ResolvePolicySources([]string{"AA:BB:CC:DD:EE:FF"}, clients); err == nil {
		t.Fatal("unknown MAC should be an error")
	}
	if _, err := ResolvePolicySources([]string{"nonsense"}, clients); err == nil {
		t.Fatal("garbage source should be an error")
	}
}
