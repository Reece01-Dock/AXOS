package main

import (
	"context"
	"flag"
	"fmt"
)

// cmdFootprint shows AXOS's own RAM usage against its recorded baseline —
// "how much has this addon grown since it was first installed" (see
// internal/footprint). With no subcommand it shows current vs. baseline;
// "history" lists every recorded snapshot; "snapshot" forces a new one now.
func cmdFootprint(ctx context.Context, args []string) error {
	sub := ""
	rest := args
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		sub, rest = args[0], args[1:]
	}

	switch sub {
	case "", "show":
		return cmdFootprintShow(ctx, rest)
	case "history":
		return cmdFootprintHistory(ctx, rest)
	case "snapshot":
		return cmdFootprintSnapshot(ctx, rest)
	default:
		return fmt.Errorf("unknown footprint subcommand %q (want show, history, or snapshot)", sub)
	}
}

func cmdFootprintShow(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("footprint show", flag.ExitOnError)
	apiURL, actor := addAPIFlags(fs)
	_ = fs.Parse(args)

	client := newAPIClient(apiURL, actor)
	fp, err := client.Footprint(ctx)
	if err != nil {
		return fmt.Errorf("fetching footprint: %w", err)
	}

	cur := fp.Current
	fmt.Printf("Process RSS:       %d KB\n", cur.ProcessRSSKB)
	fmt.Printf("Go heap alloc:     %d KB\n", cur.GoHeapAllocKB)
	fmt.Printf("Go runtime sys:    %d KB\n", cur.GoSysKB)
	fmt.Printf("Goroutines:        %d\n", cur.NumGoroutine)
	if cur.SystemTotalKB > 0 {
		fmt.Printf("System memory:     %d / %d KB used (%d KB free)\n", cur.SystemUsedKB, cur.SystemTotalKB, cur.SystemFreeKB)
	}

	if fp.Baseline == nil {
		fmt.Println("\nNo baseline recorded yet.")
		return nil
	}
	base := *fp.Baseline
	fmt.Printf("\nBaseline (recorded %s, label=%q):\n", base.Timestamp.Format("2006-01-02 15:04:05"), base.Label)
	fmt.Printf("  Process RSS:     %d KB\n", base.ProcessRSSKB)
	delta := int64(cur.ProcessRSSKB) - int64(base.ProcessRSSKB)
	sign := "+"
	if delta < 0 {
		sign = ""
	}
	fmt.Printf("\nGrowth since baseline: %s%d KB\n", sign, delta)
	return nil
}

func cmdFootprintHistory(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("footprint history", flag.ExitOnError)
	apiURL, actor := addAPIFlags(fs)
	_ = fs.Parse(args)

	client := newAPIClient(apiURL, actor)
	hist, err := client.FootprintHistory(ctx)
	if err != nil {
		return fmt.Errorf("fetching footprint history: %w", err)
	}
	if len(hist) == 0 {
		fmt.Println("No footprint snapshots recorded yet.")
		return nil
	}

	baseline := hist[0].ProcessRSSKB
	for _, snap := range hist {
		if snap.Label == "baseline" {
			baseline = snap.ProcessRSSKB
			break
		}
	}

	fmt.Printf("%-20s %-20s %-12s %-12s %s\n", "TIME", "LABEL", "RSS (KB)", "DELTA (KB)", "RELEASE")
	for _, snap := range hist {
		delta := int64(snap.ProcessRSSKB) - int64(baseline)
		release := snap.ReleaseID
		if release == "" {
			release = "-"
		}
		fmt.Printf("%-20s %-20s %-12d %-+12d %s\n",
			snap.Timestamp.Format("2006-01-02 15:04:05"), snap.Label, snap.ProcessRSSKB, delta, release)
	}
	return nil
}

func cmdFootprintSnapshot(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("footprint snapshot", flag.ExitOnError)
	apiURL, actor := addAPIFlags(fs)
	label := fs.String("label", "manual", "Label to record this snapshot under")
	releaseID := fs.String("release", "", "Release id this snapshot corresponds to (optional)")
	_ = fs.Parse(args)

	client := newAPIClient(apiURL, actor)
	snap, err := client.FootprintSnapshot(ctx, *label, *releaseID)
	if err != nil {
		return fmt.Errorf("recording footprint snapshot: %w", err)
	}
	fmt.Printf("Recorded snapshot %q: process RSS = %d KB\n", snap.Label, snap.ProcessRSSKB)
	return nil
}
