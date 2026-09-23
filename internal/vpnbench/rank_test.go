package vpnbench

import "testing"

func TestParseAvgMs(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"round-trip min/avg/max = 1.2/3.4/5.6 ms", 3.4, true},
		{"rtt min/avg/max/mdev = 10.0/20.5/30.0/1.0 ms", 20.5, true},
		{"no stats here", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseAvgMs(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseAvgMs(%q) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestRank(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	in := []Sample{
		{Host: "c.example", OK: false},
		{Host: "b.example", OK: true, AvgMs: f(20)},
		{Host: "a.example", OK: true, AvgMs: f(5)},
		{Host: "d.example", OK: true, AvgMs: f(5)},
	}
	got := Rank(in)
	want := []string{"a.example", "d.example", "b.example", "c.example"}
	for i, h := range want {
		if got[i].Host != h {
			t.Fatalf("rank[%d]=%q want %q (full=%v)", i, got[i].Host, h, got)
		}
	}
	// input not mutated
	if in[0].Host != "c.example" {
		t.Fatal("Rank mutated input")
	}
}
