// Package vpnbench ranks VPN/WAN endpoints by from-router ping latency.
// Selection/auto-apply to a profile is intentionally not done here yet.
package vpnbench

import (
	"regexp"
	"sort"
	"strconv"
)

var rttRe = regexp.MustCompile(`(?i)(?:round-trip|rtt)[^=]*=\s*([\d.]+)/([\d.]+)/([\d.]+)`)

// Sample is one host's benchmark result.
type Sample struct {
	Host  string   `json:"host"`
	OK    bool     `json:"ok"`
	AvgMs *float64 `json:"avg_ms,omitempty"`
	Raw   string   `json:"raw,omitempty"`
}

// ParseAvgMs extracts the average RTT in milliseconds from ping output.
// Returns ok=false when the summary line is missing.
func ParseAvgMs(output string) (avg float64, ok bool) {
	m := rttRe.FindStringSubmatch(output)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// Rank sorts samples best-first: lowest avg_ms, then failures last (stable by host).
func Rank(samples []Sample) []Sample {
	out := append([]Sample(nil), samples...)
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := out[i].AvgMs, out[j].AvgMs
		if ai == nil && aj == nil {
			return out[i].Host < out[j].Host
		}
		if ai == nil {
			return false
		}
		if aj == nil {
			return true
		}
		if *ai != *aj {
			return *ai < *aj
		}
		return out[i].Host < out[j].Host
	})
	return out
}
