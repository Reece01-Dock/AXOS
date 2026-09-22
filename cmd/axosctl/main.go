// Command axosctl is the AXOS control CLI: the same operations available
// over MCP, but from a terminal — status, health, deploy, rollback,
// restart, logs, capture, and the "dev" workflow commands
// (deploy/watch/restart/test) that make up the hot-development loop (see
// docs/development.md).
//
// axosctl talks to axosd's Core API (internal/api, via
// internal/backend/httpclient) for anything about the live router or
// supervised services, and operates directly on the filesystem (via
// internal/deploy) for release management — see each subcommand's own
// comment for which.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/reece01-dock/axos/internal/backend/httpclient"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "status":
		err = cmdStatus(ctx, os.Args[2:])
	case "health":
		err = cmdHealth(ctx, os.Args[2:])
	case "backend-info":
		err = cmdBackendInfo(ctx, os.Args[2:])
	case "capture":
		err = cmdCapture(ctx, os.Args[2:])
	case "test":
		err = cmdTest(ctx, os.Args[2:])
	case "deploy":
		err = cmdDeploy(ctx, os.Args[2:])
	case "rollback":
		err = cmdRollback(ctx, os.Args[2:])
	case "history":
		err = cmdHistory(ctx, os.Args[2:])
	case "restart":
		err = cmdRestart(ctx, os.Args[2:])
	case "footprint":
		err = cmdFootprint(ctx, os.Args[2:])
	case "logs":
		err = cmdLogs(ctx, os.Args[2:])
	case "transaction":
		err = cmdTransaction(ctx, os.Args[2:])
	case "dev":
		err = cmdDev(ctx, os.Args[2:])
	case "version":
		fmt.Println("axosctl 0.1.0")
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "axosctl: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "axosctl: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `axosctl - AXOS control CLI

Router status (talks to axosd's Core API, -api flag; default http://127.0.0.1:9090):
  axosctl status                    Release + supervised-service summary
  axosctl health                    Aggregate health (API reachable, services, rollback state)
  axosctl backend-info               Which backend a given --backend/--fixture/--host resolves to
  axosctl restart <service>          Restart a supervised sibling service
  axosctl logs <service> [--lines N] Tail a supervised service's log
  axosctl footprint [show]           AXOS's own RAM usage vs. its recorded baseline
  axosctl footprint history          Every recorded footprint snapshot, with deltas
  axosctl footprint snapshot [--label L] [--release ID]
                                      Force a footprint snapshot now
  axosctl transaction begin --rollback-after <seconds> [--reason ...]
  axosctl transaction confirm --id <id>
  axosctl transaction status

Development (local, no router needed for mock/replay):
  axosctl capture [--backend mock|replay|asuswrt] [--host H] [--out DIR]
  axosctl test                       Run this repo's Go test suite

Release management (operates on --root, a local /opt/axos-shaped directory):
  axosctl deploy [--root DIR] [--components axosd,axos-mcp,axosctl]
                 [--goos linux] [--goarch arm64] [--skip-tests]
  axosctl rollback [--root DIR] [--version ID]
  axosctl history [--root DIR]

Hot-development loop:
  axosctl dev deploy [same flags as deploy]
  axosctl dev restart <service>
  axosctl dev test
  axosctl dev watch [--watch-dir .] [--service axosd] [--interval 2s]
                     [same flags as deploy] — edit -> test -> deploy -> restart
                     -> health check, repeating on every change, never on a
                     failed test or a failed health check. No router reboot,
                     ever, for anything this command does.

Common flags:
  -api string    Base URL of axosd's Core API (default "http://127.0.0.1:9090")
  -actor string  Identity sent as X-Axos-Actor for axosd's audit log (default "cli:$USER")
`)
}

func defaultActor() string {
	if u := os.Getenv("USER"); u != "" {
		return "cli:" + u
	}
	return "cli:unknown"
}

// addAPIFlags registers the common -api/-actor flags on fs, returning
// pointers populated once fs.Parse has run. Call newAPIClient with them
// afterward to build the actual client.
func addAPIFlags(fs *flag.FlagSet) (apiURL, actor *string) {
	apiURL = fs.String("api", "http://127.0.0.1:9090", "Base URL of axosd's Core API")
	actor = fs.String("actor", defaultActor(), "Identity sent as X-Axos-Actor")
	return apiURL, actor
}

func newAPIClient(apiURL, actor *string) *httpclient.Client {
	return httpclient.New(*apiURL, *actor)
}

// popPositional pulls a single leading positional argument (one that
// doesn't start with "-") off the front of args, if present, returning it
// separately from the rest. Go's flag package stops parsing at the first
// non-flag argument and dumps everything from there into Args() — so
// "axosctl logs echoer -lines 10" would otherwise see NArg()==3, not the
// single positional the usage text promises. Commands with exactly one
// leading positional (restart, logs) call this before fs.Parse so flags
// can appear in either order around it.
func popPositional(args []string) (positional string, rest []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}
