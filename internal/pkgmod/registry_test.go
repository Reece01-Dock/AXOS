package pkgmod

import "testing"

func TestMemoryRegistry(t *testing.T) {
	r := NewMemory()
	r.Seed(Info{ID: "demo", Name: "Demo", Version: "0.1.0", Description: "stub"})

	list := r.List()
	if len(list) != 1 || list[0].Enabled {
		t.Fatalf("list=%v", list)
	}
	if err := r.Enable("demo"); err != nil {
		t.Fatal(err)
	}
	p, ok := r.Get("demo")
	if !ok || !p.Enabled {
		t.Fatalf("get=%v ok=%v", p, ok)
	}
	if err := r.Disable("demo"); err != nil {
		t.Fatal(err)
	}
	p, _ = r.Get("demo")
	if p.Enabled {
		t.Fatal("still enabled")
	}
	if err := r.Enable("missing"); err == nil {
		t.Fatal("want error for unknown id")
	}
}
