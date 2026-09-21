package main

import (
	"context"
	"flag"
	"fmt"
)

// cmdRestart restarts one supervised service via axosd's Core API. This is
// the "restart only the changed AXOS service" step of the hot-deploy loop —
// it does not touch anything else and never reboots the router.
func cmdRestart(ctx context.Context, args []string) error {
	name, rest := popPositional(args)

	fs := flag.NewFlagSet("restart", flag.ExitOnError)
	apiURL, actor := addAPIFlags(fs)
	_ = fs.Parse(rest)

	if name == "" {
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: axosctl restart <service>")
		}
		name = fs.Arg(0)
	}

	client := newAPIClient(apiURL, actor)
	if err := client.ServiceRestart(ctx, name); err != nil {
		return fmt.Errorf("restarting %s: %w", name, err)
	}
	fmt.Printf("%s: restarted\n", name)
	return nil
}

func cmdLogs(ctx context.Context, args []string) error {
	name, rest := popPositional(args)

	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	apiURL, actor := addAPIFlags(fs)
	lines := fs.Int("lines", 200, "Number of trailing lines to show")
	_ = fs.Parse(rest)

	if name == "" {
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: axosctl logs <service> [--lines N]")
		}
		name = fs.Arg(0)
	}

	client := newAPIClient(apiURL, actor)
	log, err := client.ServiceLogs(ctx, name, *lines)
	if err != nil {
		return fmt.Errorf("fetching logs for %s: %w", name, err)
	}
	fmt.Println(log)
	return nil
}

// cmdTransaction is a thin CLI over the existing router-config rollback
// engine (internal/rollbackctl, via axosd's API) — NOT a separate
// mechanism. "transaction begin/confirm/status" is just friendlier CLI
// vocabulary for "rollback.arm/confirm/status", per the brief's own
// example (`axosctl transaction begin --rollback-after 120`). Reusing the
// one existing engine here is deliberate: docs/architecture.md's rule
// against two control systems doing the same job applies just as much
// within a single binary as it does across MCP/CLI/web UI.
func cmdTransaction(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: axosctl transaction <begin|confirm|status> [flags]")
	}
	sub, rest := args[0], args[1:]

	fs := flag.NewFlagSet("transaction "+sub, flag.ExitOnError)
	apiURL, actor := addAPIFlags(fs)

	switch sub {
	case "begin":
		rollbackAfter := fs.Int("rollback-after", 120, "Seconds before automatic revert if not confirmed")
		reason := fs.String("reason", "", "Human-readable reason, for the audit log")
		_ = fs.Parse(rest)

		client := newAPIClient(apiURL, actor)
		id, deadline, err := client.Arm(ctx, *rollbackAfter, *reason)
		if err != nil {
			return fmt.Errorf("arming rollback: %w", err)
		}
		fmt.Printf("Armed transaction %s — automatic revert at %s unless confirmed\n", id, deadline.Format("15:04:05"))
		fmt.Printf("Confirm with: axosctl transaction confirm --id %s\n", id)
		return nil

	case "confirm":
		id := fs.String("id", "", "Transaction id from 'transaction begin'")
		_ = fs.Parse(rest)
		if *id == "" {
			return fmt.Errorf("usage: axosctl transaction confirm --id <id>")
		}
		client := newAPIClient(apiURL, actor)
		if err := client.Confirm(ctx, *id); err != nil {
			return fmt.Errorf("confirming %s: %w", *id, err)
		}
		fmt.Printf("%s: confirmed (change kept)\n", *id)
		return nil

	case "status":
		_ = fs.Parse(rest)
		client := newAPIClient(apiURL, actor)
		st, err := client.Status(ctx)
		if err != nil {
			return fmt.Errorf("fetching rollback status: %w", err)
		}
		if !st.Pending {
			fmt.Println("No transaction armed.")
			return nil
		}
		fmt.Printf("Armed: id=%s reason=%q armed_at=%s deadline=%s\n",
			st.ID, st.Reason, st.ArmedAt.Format("15:04:05"), st.Deadline.Format("15:04:05"))
		return nil

	default:
		return fmt.Errorf("unknown transaction subcommand %q (want begin, confirm, or status)", sub)
	}
}
