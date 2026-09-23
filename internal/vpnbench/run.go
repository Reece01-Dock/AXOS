package vpnbench

import (
	"context"
	"fmt"
	"strings"

	"github.com/reece01-dock/axos/internal/backend"
)

// Pinger is the subset of RouterBackend needed to rank endpoints.
type Pinger interface {
	Ping(ctx context.Context, host string, count int) (backend.DiagResult, error)
}

// Result is a ranked benchmark run.
type Result struct {
	Results []Sample `json:"results"`
	Best    string   `json:"best,omitempty"`
}

// Run pings each host and returns samples ranked best-first.
// At most 16 unique non-empty hosts; count defaults to 3.
func Run(ctx context.Context, p Pinger, hosts []string, count int) (Result, error) {
	uniq := make([]string, 0, len(hosts))
	seen := map[string]bool{}
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		uniq = append(uniq, h)
	}
	if len(uniq) == 0 {
		return Result{}, fmt.Errorf("hosts is required")
	}
	if len(uniq) > 16 {
		return Result{}, fmt.Errorf("at most 16 hosts")
	}
	if count <= 0 {
		count = 3
	}
	samples := make([]Sample, 0, len(uniq))
	for _, h := range uniq {
		res, err := p.Ping(ctx, h, count)
		s := Sample{Host: h, OK: err == nil && res.OK, Raw: res.Output}
		if err != nil {
			s.OK = false
			s.Raw = err.Error()
		} else if avg, ok := ParseAvgMs(res.Output); ok {
			s.AvgMs = &avg
		}
		samples = append(samples, s)
	}
	ranked := Rank(samples)
	out := Result{Results: ranked}
	if len(ranked) > 0 && ranked[0].AvgMs != nil {
		out.Best = ranked[0].Host
	}
	return out, nil
}
