// Command axosd is the AXOS core service daemon: a single static binary
// exposing the RouterBackend API over MCP (see docs/architecture.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/reece01-dock/axos/axosd/internal/audit"
	"github.com/reece01-dock/axos/axosd/internal/backend"
	"github.com/reece01-dock/axos/axosd/internal/backend/asuswrt"
	"github.com/reece01-dock/axos/axosd/internal/backend/mock"
	"github.com/reece01-dock/axos/axosd/internal/mcp"
	"github.com/reece01-dock/axos/axosd/internal/rollback"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "mcp":
		runMCP(os.Args[2:])
	case "version":
		fmt.Println("axosd 0.1.0-milestone2")
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `axosd - AXOS core service daemon

Usage:
  axosd mcp [flags]      Run the MCP server over stdio
  axosd version           Print version

Flags for 'mcp':
  -backend string   "mock" or "asuswrt" (default "mock")
  -audit string      Path to the audit log file (default "./axosd-audit.jsonl")
  -actor string       Identity recorded in audit entries (default "mcp:unknown")`)
}

func runMCP(args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	backendName := fs.String("backend", "mock", `Backend implementation: "mock" or "asuswrt"`)
	auditPath := fs.String("audit", "./axosd-audit.jsonl", "Path to the append-only audit log")
	actor := fs.String("actor", "mcp:unknown", "Identity recorded in audit log entries")
	_ = fs.Parse(args)

	var be backend.RouterBackend
	switch *backendName {
	case "mock":
		be = mock.New()
	case "asuswrt":
		var err error
		be, err = asuswrt.New()
		if err != nil {
			log.Fatalf("axosd: failed to initialize asuswrt backend: %v", err)
		}
	default:
		log.Fatalf("axosd: unknown backend %q (want \"mock\" or \"asuswrt\")", *backendName)
	}

	al, err := audit.Open(*auditPath)
	if err != nil {
		log.Fatalf("axosd: %v", err)
	}
	defer al.Close()

	rb := rollback.New()
	rb.OnEvent(func(ev rollback.Event) {
		switch ev.Kind {
		case rollback.EventExpiredRestored:
			_ = al.Reverted("rollback.auto_restore", ev.Detail, ev.ID)
			log.Printf("axosd: transaction %s expired without confirmation — automatically reverted (%s)", ev.ID, ev.Detail)
		case rollback.EventExpiredRestoreFailed:
			_ = al.Reverted("rollback.auto_restore_failed", ev.Detail, ev.ID)
			log.Printf("axosd: CRITICAL: transaction %s expired and automatic restore FAILED: %s", ev.ID, ev.Detail)
		}
	})

	server := mcp.NewServer(be, rb, al, *actor)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("axosd: MCP server starting (backend=%s, audit=%s)", *backendName, *auditPath)
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("axosd: MCP server exited with error: %v", err)
	}
}
