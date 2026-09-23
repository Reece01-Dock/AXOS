package optimiser

import (
	"context"
	"fmt"
)

// EthernetOptimiser is a Milestone 4 skeleton: no-op observe/propose/apply
// that compiles and is unit-testable. It does not touch hardware or claim
// verification of IRQ affinity, buffers, conntrack, or Broadcom flow offload.
type EthernetOptimiser struct{}

// NewEthernet returns a no-op EthernetOptimiser.
func NewEthernet() *EthernetOptimiser { return &EthernetOptimiser{} }

func (e *EthernetOptimiser) Observe(_ context.Context) (Observation, error) {
	return Observation{
		Subsystem: "ethernet",
		Notes:     "noop skeleton — not hardware-verified",
		Metrics:   map[string]float64{},
	}, nil
}

func (e *EthernetOptimiser) Baseline(_ context.Context) (Baseline, error) {
	return Baseline{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (e *EthernetOptimiser) Propose(_ context.Context, _ Observation, _ Baseline) (Proposal, error) {
	return Proposal{
		ID:          "noop",
		Description: "no change proposed (skeleton)",
	}, nil
}

func (e *EthernetOptimiser) Apply(_ context.Context, p Proposal) error {
	if p.ID == "" {
		return fmt.Errorf("optimiser: proposal id is required")
	}
	return nil
}

func (e *EthernetOptimiser) Measure(_ context.Context) (Measurement, error) {
	return Measurement{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (e *EthernetOptimiser) Decide(_ context.Context, _ Baseline, _ Measurement) (Decision, error) {
	return Decision{Keep: false, Reason: "noop skeleton — nothing applied"}, nil
}
