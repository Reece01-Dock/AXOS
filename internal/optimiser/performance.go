package optimiser

import (
	"context"
	"fmt"
)

// PerformanceMode orchestrates Ethernet/Wi-Fi/VPN/Latency optimisers under
// a single OBSERVE→…→DECIDE pass. Skeleton only — does not arm rollback or
// apply subsystem changes.
type PerformanceMode struct {
	Ethernet *EthernetOptimiser
	WiFi     *WiFiOptimiser
	VPN      *VPNOptimiser
	Latency  *LatencyOptimiser
}

// NewPerformanceMode wires default no-op subsystem optimisers.
func NewPerformanceMode() *PerformanceMode {
	return &PerformanceMode{
		Ethernet: NewEthernet(),
		WiFi:     NewWiFi(),
		VPN:      NewVPN(),
		Latency:  NewLatency(),
	}
}

func (p *PerformanceMode) Observe(ctx context.Context) (Observation, error) {
	_ = ctx
	return Observation{
		Subsystem: "performance",
		Notes:     "noop skeleton — orchestrates ethernet/wifi/vpn/latency; not hardware-verified",
		Metrics:   map[string]float64{},
	}, nil
}

func (p *PerformanceMode) Baseline(_ context.Context) (Baseline, error) {
	return Baseline{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (p *PerformanceMode) Propose(_ context.Context, _ Observation, _ Baseline) (Proposal, error) {
	return Proposal{
		ID:          "noop",
		Description: "no change proposed (skeleton)",
	}, nil
}

func (p *PerformanceMode) Apply(_ context.Context, prop Proposal) error {
	if prop.ID == "" {
		return fmt.Errorf("optimiser: proposal id is required")
	}
	return nil
}

func (p *PerformanceMode) Measure(_ context.Context) (Measurement, error) {
	return Measurement{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (p *PerformanceMode) Decide(_ context.Context, _ Baseline, _ Measurement) (Decision, error) {
	return Decision{Keep: false, Reason: "noop skeleton — nothing applied"}, nil
}
