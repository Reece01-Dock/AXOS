package optimiser

import "context"

// Interface is the Milestone 4 optimisation loop contract:
// OBSERVE → BASELINE → PROPOSE → APPLY → MEASURE → DECIDE.
// Implementations must never claim hardware verification from unit tests alone.
type Interface interface {
	Observe(ctx context.Context) (Observation, error)
	Baseline(ctx context.Context) (Baseline, error)
	Propose(ctx context.Context, obs Observation, base Baseline) (Proposal, error)
	Apply(ctx context.Context, p Proposal) error
	Measure(ctx context.Context) (Measurement, error)
	Decide(ctx context.Context, base Baseline, m Measurement) (Decision, error)
}

// Observation is a point-in-time view of the subsystem under optimisation.
type Observation struct {
	Subsystem string                 `json:"subsystem"`
	Notes     string                 `json:"notes,omitempty"`
	Metrics   map[string]float64     `json:"metrics,omitempty"`
	Raw       map[string]interface{} `json:"raw,omitempty"`
}

// Baseline is the pre-change reference measurement set.
type Baseline struct {
	Label   string             `json:"label"`
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

// Proposal is a single candidate change (one thing at a time).
type Proposal struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	Params      map[string]interface{} `json:"params,omitempty"`
}

// Measurement is a post-change (or re-check) metric set.
type Measurement struct {
	Label   string             `json:"label"`
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

// Decision is keep vs revert for the applied proposal.
type Decision struct {
	Keep   bool   `json:"keep"`
	Reason string `json:"reason"`
}

// Ensure EthernetOptimiser satisfies Interface at compile time.
var (
	_ Interface = (*EthernetOptimiser)(nil)
	_ Interface = (*WiFiOptimiser)(nil)
	_ Interface = (*VPNOptimiser)(nil)
	_ Interface = (*LatencyOptimiser)(nil)
	_ Interface = (*PerformanceMode)(nil)
)
