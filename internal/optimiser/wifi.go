package optimiser

import (
	"context"
	"fmt"
)

// WiFiOptimiser is a Milestone 4 skeleton for channel/utilisation A/B loops.
// It does not scan radios or change Merlin wl settings.
type WiFiOptimiser struct{}

// NewWiFi returns a no-op WiFiOptimiser.
func NewWiFi() *WiFiOptimiser { return &WiFiOptimiser{} }

func (w *WiFiOptimiser) Observe(_ context.Context) (Observation, error) {
	return Observation{
		Subsystem: "wifi",
		Notes:     "noop skeleton — not hardware-verified",
		Metrics:   map[string]float64{},
	}, nil
}

func (w *WiFiOptimiser) Baseline(_ context.Context) (Baseline, error) {
	return Baseline{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (w *WiFiOptimiser) Propose(_ context.Context, _ Observation, _ Baseline) (Proposal, error) {
	return Proposal{
		ID:          "noop",
		Description: "no change proposed (skeleton)",
	}, nil
}

func (w *WiFiOptimiser) Apply(_ context.Context, p Proposal) error {
	if p.ID == "" {
		return fmt.Errorf("optimiser: proposal id is required")
	}
	return nil
}

func (w *WiFiOptimiser) Measure(_ context.Context) (Measurement, error) {
	return Measurement{Label: "noop", Metrics: map[string]float64{}}, nil
}

func (w *WiFiOptimiser) Decide(_ context.Context, _ Baseline, _ Measurement) (Decision, error) {
	return Decision{Keep: false, Reason: "noop skeleton — nothing applied"}, nil
}
