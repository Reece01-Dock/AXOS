package optimiser

import (
	"context"
	"testing"
)

func runNoopLoop(t *testing.T, name string, opt Interface, wantSubsystem string) {
	t.Helper()
	ctx := context.Background()

	obs, err := opt.Observe(ctx)
	if err != nil {
		t.Fatalf("%s Observe: %v", name, err)
	}
	if obs.Subsystem != wantSubsystem {
		t.Fatalf("%s subsystem = %q, want %q", name, obs.Subsystem, wantSubsystem)
	}

	base, err := opt.Baseline(ctx)
	if err != nil {
		t.Fatalf("%s Baseline: %v", name, err)
	}
	prop, err := opt.Propose(ctx, obs, base)
	if err != nil {
		t.Fatalf("%s Propose: %v", name, err)
	}
	if err := opt.Apply(ctx, prop); err != nil {
		t.Fatalf("%s Apply: %v", name, err)
	}
	m, err := opt.Measure(ctx)
	if err != nil {
		t.Fatalf("%s Measure: %v", name, err)
	}
	dec, err := opt.Decide(ctx, base, m)
	if err != nil {
		t.Fatalf("%s Decide: %v", name, err)
	}
	if dec.Keep {
		t.Fatalf("%s noop skeleton should not Keep", name)
	}
	if err := opt.Apply(ctx, Proposal{}); err == nil {
		t.Fatalf("%s Apply with empty id should fail", name)
	}
}

func TestWiFiVPNLatencyPerformance_NoopLoops(t *testing.T) {
	runNoopLoop(t, "wifi", NewWiFi(), "wifi")
	runNoopLoop(t, "vpn", NewVPN(), "vpn")
	runNoopLoop(t, "latency", NewLatency(), "latency")
	runNoopLoop(t, "performance", NewPerformanceMode(), "performance")
}
