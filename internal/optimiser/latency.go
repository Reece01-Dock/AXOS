package optimiser

import (
	"context"
	"fmt"
)

// LatencyOptimiser is a Milestone 4 skeleton for loaded-latency driven loops.
type LatencyOptimiser struct{}

// NewLatency returns a no-op LatencyOptimiser.
func NewLatency() *LatencyOptimiser { return &LatencyOptimiser{} }

func (l *LatencyOptimiser) Observe(_ context.Context) (Observation, error) {
	return Observation{
		Subsystem: "latency",
		Notes:     "noop skeleton — not hardware-verified",
		Metrics:   map[string]float64{},
	}, nil
}

func (l *LatencyOptimiser) Baseline(_ context.Context) (Baseline, error) {
	return Baseline{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (l *LatencyOptimiser) Propose(_ context.Context, _ Observation, _ Baseline) (Proposal, error) {
	return Proposal{
		ID:          "noop",
		Description: "no change proposed (skeleton)",
	}, nil
}

func (l *LatencyOptimiser) Apply(_ context.Context, p Proposal) error {
	if p.ID == "" {
		return fmt.Errorf("optimiser: proposal id is required")
	}
	return nil
}

func (l *LatencyOptimiser) Measure(_ context.Context) (Measurement, error) {
	return Measurement{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (l *LatencyOptimiser) Decide(_ context.Context, _ Baseline, _ Measurement) (Decision, error) {
	return Decision{Keep: false, Reason: "noop skeleton — nothing applied"}, nil
}
