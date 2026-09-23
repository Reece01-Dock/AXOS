package optimiser

import (
	"context"
	"testing"
)

func TestEthernetOptimiser_NoopLoop(t *testing.T) {
	opt := NewEthernet()
	ctx := context.Background()

	obs, err := opt.Observe(ctx)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.Subsystem != "ethernet" {
		t.Fatalf("subsystem = %q, want ethernet", obs.Subsystem)
	}

	base, err := opt.Baseline(ctx)
	if err != nil {
		t.Fatalf("Baseline: %v", err)
	}

	prop, err := opt.Propose(ctx, obs, base)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if err := opt.Apply(ctx, prop); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	m, err := opt.Measure(ctx)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	dec, err := opt.Decide(ctx, base, m)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dec.Keep {
		t.Fatal("noop skeleton should not Keep")
	}
}

func TestEthernetOptimiser_ApplyRequiresID(t *testing.T) {
	opt := NewEthernet()
	if err := opt.Apply(context.Background(), Proposal{}); err == nil {
		t.Fatal("Apply with empty id should fail")
	}
}
