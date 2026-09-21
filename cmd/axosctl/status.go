package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/reece01-dock/axos/internal/backendselect"
	"github.com/reece01-dock/axos/internal/deploy"
)

func rootFlag(fs *flag.FlagSet) *string {
	return fs.String("root", "/opt/axos", "Local AXOS release root (releases/, current, previous, staging)")
}

// cmdStatus prints the release currently deployed at --root and, if axosd's
// API is reachable, every supervised service's status. Release state is a
// local filesystem fact (internal/deploy) — no network needed for that
// half even if axosd isn't reachable.
func cmdStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	root := rootFlag(fs)
	apiURL, actor := addAPIFlags(fs)
	_ = fs.Parse(args)

	d := deploy.New(*root)
	cur, _ := d.CurrentRelease()
	prev, _ := d.PreviousRelease()
	fmt.Printf("Release root:     %s\n", *root)
	if cur == "" {
		fmt.Println("Current release:   (none deployed yet)")
	} else {
		fmt.Printf("Current release:   %s\n", cur)
	}
	if prev != "" {
		fmt.Printf("Previous release:  %s\n", prev)
	}

	client := newAPIClient(apiURL, actor)
	info, err := client.Info(ctx)
	if err != nil {
		fmt.Printf("\naxosd Core API:    UNREACHABLE at %s (%v)\n", *apiURL, err)
		return nil // status is informational — a down API isn't a CLI failure
	}
	fmt.Printf("\naxosd Core API:    reachable at %s\n", *apiURL)
	fmt.Printf("Router model:      %s (firmware %s)\n", info.Model, info.FirmwareVer)

	st, err := client.Status(ctx)
	if err == nil && st.Pending {
		fmt.Printf("Rollback:          ARMED (id=%s, reason=%q, deadline=%s)\n", st.ID, st.Reason, st.Deadline.Format("15:04:05"))
	} else {
		fmt.Println("Rollback:          not armed")
	}

	services, err := client.ServicesStatus(ctx)
	if err != nil {
		fmt.Println("Supervised services: (none configured on this axosd instance)")
		return nil
	}
	if len(services) == 0 {
		fmt.Println("Supervised services: (none registered)")
		return nil
	}
	fmt.Println("Supervised services:")
	for _, s := range services {
		state := "stopped"
		if s.Running {
			state = fmt.Sprintf("running (pid %d)", s.PID)
		}
		if s.GaveUpAfterRestarts {
			state = "CRASHED (gave up after repeated restarts)"
		}
		fmt.Printf("  %-20s %s  (restarts: %d)\n", s.Name, state, s.Restarts)
	}
	return nil
}

// cmdHealth is the pass/fail version of status: reachability + every
// supervised service's health check. Exits non-zero (via a returned error)
// if anything is unhealthy, so it's usable as a script/CI gate.
func cmdHealth(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	apiURL, actor := addAPIFlags(fs)
	_ = fs.Parse(args)

	client := newAPIClient(apiURL, actor)

	if _, err := client.Info(ctx); err != nil {
		return fmt.Errorf("axosd Core API unreachable at %s: %w", *apiURL, err)
	}
	fmt.Println("axosd Core API:      OK")

	services, err := client.ServicesStatus(ctx)
	if err != nil {
		fmt.Println("Supervised services: (none configured — nothing further to check)")
		return nil
	}

	allHealthy := true
	for _, s := range services {
		h, err := client.ServiceHealth(ctx, s.Name)
		status := "OK"
		if err != nil || !h.Healthy {
			status = "UNHEALTHY"
			if h.Detail != "" {
				status += ": " + h.Detail
			}
			allHealthy = false
		}
		fmt.Printf("%-20s   %s\n", s.Name, status)
	}
	if !allHealthy {
		return fmt.Errorf("one or more supervised services are unhealthy")
	}
	return nil
}

// cmdBackendInfo is a small connectivity sanity check: resolve the backend
// the given flags describe and print what it reports for Info(), without
// needing axosd's API at all (it talks to the backend directly, the same
// way axosd itself would).
func cmdBackendInfo(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("backend-info", flag.ExitOnError)
	beOpts := &backendselect.Options{}
	fs.StringVar(&beOpts.Name, "backend", "mock", `"mock", "replay", or "asuswrt"`)
	fs.StringVar(&beOpts.Fixture, "fixture", "", "replay: fixture directory")
	fs.StringVar(&beOpts.Host, "host", "", "asuswrt: SSH host")
	_ = fs.Parse(args)

	be, err := backendselect.New(*beOpts)
	if err != nil {
		return err
	}
	info, err := be.Info(ctx)
	if err != nil {
		return fmt.Errorf("backend %q did not respond: %w", beOpts.Name, err)
	}
	fmt.Printf("backend:   %s\n", beOpts.Name)
	if beOpts.Host != "" {
		fmt.Printf("host:      %s\n", beOpts.Host)
	}
	if beOpts.Fixture != "" {
		fmt.Printf("fixture:   %s\n", beOpts.Fixture)
	}
	fmt.Printf("model:     %s\n", info.Model)
	fmt.Printf("firmware:  %s\n", info.FirmwareVer)
	return nil
}
