package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/reece01-dock/axos/internal/backendselect"
	"github.com/reece01-dock/axos/internal/capture"
)

// cmdCapture queries a backend (mock, an existing replay fixture, or a real
// router over SSH) and writes a sanitized fixture set — see internal/capture.
// Running it with --backend mock (the default) needs no router at all and
// is a good way to see the fixture format; running it against a real
// GT-AX6000 (--backend asuswrt --host <router>) is how you'd actually
// produce testdata/gt-ax6000/ for the first time (not done from this
// sandbox — see docs/development.md).
func cmdCapture(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("capture", flag.ExitOnError)
	beOpts := &backendselect.Options{}
	fs.StringVar(&beOpts.Name, "backend", "mock", `"mock", "replay", or "asuswrt"`)
	fs.StringVar(&beOpts.Fixture, "fixture", "", "replay: source fixture directory to re-capture")
	fs.StringVar(&beOpts.Host, "host", "", "asuswrt: SSH host to capture from")
	out := fs.String("out", "testdata/capture", "Directory to write the sanitized fixture set to")
	source := fs.String("source", "", "Label recorded in capabilities.json (default: derived from --backend/--host)")
	_ = fs.Parse(args)

	be, err := backendselect.New(*beOpts)
	if err != nil {
		return err
	}

	label := *source
	if label == "" {
		label = beOpts.Name
		if beOpts.Host != "" {
			label += " (" + beOpts.Host + ")"
		}
	}

	fmt.Printf("Capturing from backend=%s...\n", label)
	snap, err := capture.Capture(ctx, be, label)
	if err != nil {
		return fmt.Errorf("capture failed: %w", err)
	}
	if len(snap.Errors) > 0 {
		fmt.Println("Partial capture — some fields failed:")
		for _, e := range snap.Errors {
			fmt.Println("  " + e)
		}
	}

	capture.Sanitize(snap)
	if err := capture.WriteFixtures(*out, snap); err != nil {
		return fmt.Errorf("writing fixtures: %w", err)
	}

	fmt.Printf("Wrote sanitized fixtures to %s\n", *out)
	fmt.Printf("  interfaces: %d, routes: %d, clients: %d, wifi radios: %d, services: %d, firewall rules: %d, vpn tunnels: %d, nvram keys: %d\n",
		len(snap.Interfaces), len(snap.Routes), len(snap.Clients), len(snap.WiFi),
		len(snap.Services), len(snap.Firewall), len(snap.VPN), len(snap.NVRAM))
	fmt.Println("\nReplay it with: axosd serve --backend replay --fixture " + *out)
	return nil
}
