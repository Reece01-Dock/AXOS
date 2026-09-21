// Package svcconfig loads a JSON file describing the services axosd's
// Supervisor should manage, so operators (or a future milestone shipping a
// real daemon like axos-monitor) can register them without a code change.
// No services exist to pre-register as of this milestone — see
// docs/development.md "Shared API" — so an empty/missing config file is
// the normal, honest default, not a workaround.
package svcconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/reece01-dock/axos/internal/supervisor"
)

// entry mirrors supervisor.ServiceSpec but with human-writable string
// durations ("1s", "30s") instead of raw nanosecond integers.
type entry struct {
	Name                         string   `json:"name"`
	Command                      string   `json:"command"`
	Args                         []string `json:"args"`
	Env                          []string `json:"env"`
	HealthURL                    string   `json:"health_url"`
	HealthTimeout                string   `json:"health_timeout"`
	RestartBackoffBase           string   `json:"restart_backoff_base"`
	RestartBackoffMax            string   `json:"restart_backoff_max"`
	MaxConsecutiveRestarts       int      `json:"max_consecutive_restarts"`
	MinUptimeToResetRestartCount string   `json:"min_uptime_to_reset_restart_count"`
}

// Load reads path (a JSON array of entry) and returns the corresponding
// ServiceSpecs. A missing file is not an error — it returns (nil, nil), so
// callers can pass an optional, possibly-nonexistent path without special
// casing "not configured yet".
func Load(path string) ([]supervisor.ServiceSpec, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("svcconfig: reading %s: %w", path, err)
	}

	var entries []entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("svcconfig: parsing %s: %w", path, err)
	}

	specs := make([]supervisor.ServiceSpec, 0, len(entries))
	for _, e := range entries {
		if e.Name == "" || e.Command == "" {
			return nil, fmt.Errorf("svcconfig: %s: every entry needs at least \"name\" and \"command\"", path)
		}
		spec := supervisor.ServiceSpec{
			Name: e.Name, Command: e.Command, Args: e.Args, Env: e.Env,
			HealthURL:              e.HealthURL,
			MaxConsecutiveRestarts: e.MaxConsecutiveRestarts,
		}
		var parseErr error
		spec.HealthTimeout, parseErr = parseDurationOrZero(e.HealthTimeout)
		if parseErr != nil {
			return nil, fmt.Errorf("svcconfig: %s: entry %q: health_timeout: %w", path, e.Name, parseErr)
		}
		spec.RestartBackoffBase, parseErr = parseDurationOrZero(e.RestartBackoffBase)
		if parseErr != nil {
			return nil, fmt.Errorf("svcconfig: %s: entry %q: restart_backoff_base: %w", path, e.Name, parseErr)
		}
		spec.RestartBackoffMax, parseErr = parseDurationOrZero(e.RestartBackoffMax)
		if parseErr != nil {
			return nil, fmt.Errorf("svcconfig: %s: entry %q: restart_backoff_max: %w", path, e.Name, parseErr)
		}
		spec.MinUptimeToResetRestartCount, parseErr = parseDurationOrZero(e.MinUptimeToResetRestartCount)
		if parseErr != nil {
			return nil, fmt.Errorf("svcconfig: %s: entry %q: min_uptime_to_reset_restart_count: %w", path, e.Name, parseErr)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func parseDurationOrZero(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}
