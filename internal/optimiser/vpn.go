package optimiser

import (
	"context"
	"fmt"
)

// VPNOptimiser is a Milestone 4 skeleton for endpoint/MTU selection loops.
// Endpoint ranking today is manual via Merlin UI / POST /v1/diag/ping.
type VPNOptimiser struct{}

// NewVPN returns a no-op VPNOptimiser.
func NewVPN() *VPNOptimiser { return &VPNOptimiser{} }

func (v *VPNOptimiser) Observe(_ context.Context) (Observation, error) {
	return Observation{
		Subsystem: "vpn",
		Notes:     "noop skeleton — not hardware-verified",
		Metrics:   map[string]float64{},
	}, nil
}

func (v *VPNOptimiser) Baseline(_ context.Context) (Baseline, error) {
	return Baseline{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (v *VPNOptimiser) Propose(_ context.Context, _ Observation, _ Baseline) (Proposal, error) {
	return Proposal{
		ID:          "noop",
		Description: "no change proposed (skeleton)",
	}, nil
}

func (v *VPNOptimiser) Apply(_ context.Context, p Proposal) error {
	if p.ID == "" {
		return fmt.Errorf("optimiser: proposal id is required")
	}
	return nil
}

func (v *VPNOptimiser) Measure(_ context.Context) (Measurement, error) {
	return Measurement{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (v *VPNOptimiser) Decide(_ context.Context, _ Baseline, _ Measurement) (Decision, error) {
	return Decision{Keep: false, Reason: "noop skeleton — nothing applied"}, nil
}
