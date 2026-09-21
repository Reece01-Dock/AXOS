// Command axos-mcp is the MCP frontend as an independent process: it holds
// no RouterBackend, no rollback engine, and no audit log of its own — it
// is entirely a thin client of an axosd Core API (see internal/api,
// internal/backend/httpclient), which is what lets it be restarted freely
// (to pick up a new build during development, or just because it crashed)
// without disturbing axosd's own state: an in-flight rollback timer, open
// audit log file, or anything else.
//
// It can run on the same machine as axosd (pointed at 127.0.0.1) or, during
// early development, on the developer's own PC pointed at the router over
// the network — see docs/development.md "MCP Development". It's the exact
// same MCP tool implementation (internal/mcp) either way; only where the
// process runs and what URL it points at changes.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/reece01-dock/axos/internal/backend/httpclient"
	"github.com/reece01-dock/axos/internal/mcp"
)

func main() {
	fs := flag.NewFlagSet("axos-mcp", flag.ExitOnError)
	apiURL := fs.String("api", "http://127.0.0.1:9090", "Base URL of the axosd Core API (axosd's -api-addr, with a scheme)")
	actor := fs.String("actor", "mcp:ai", "Identity sent as X-Axos-Actor on every request, for axosd's audit log")
	showVersion := fs.Bool("version", false, "Print version and exit")
	_ = fs.Parse(os.Args[1:])

	if *showVersion {
		fmt.Println("axos-mcp 0.1.0")
		return
	}

	client := httpclient.New(*apiURL, *actor)

	// No local audit log: every mutating call this makes goes through
	// axosd's API, which is where it's actually performed and where it's
	// audited (internal/api/server.go). Auditing it again here would
	// duplicate the same entries under a different process's log, not add
	// information — see internal/mcp/integration_httpclient_test.go for the
	// test that pins this behavior down.
	server := mcp.NewServer(client, client, nil, *actor)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("axos-mcp: starting, talking to axosd Core API at %s", *apiURL)
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("axos-mcp: MCP server exited with error: %v", err)
	}
}
