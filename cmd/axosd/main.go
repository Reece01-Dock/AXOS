// Command axosd is the AXOS core service daemon: it owns the RouterBackend,
// the rollback engine, and the audit log, and exposes them either directly
// over MCP (single-process mode) or over the HTTP Core API that axos-mcp
// and axosctl talk to (see docs/architecture.md, docs/development.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reece01-dock/axos/internal/api"
	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backendselect"
	"github.com/reece01-dock/axos/internal/mcp"
	"github.com/reece01-dock/axos/internal/rollback"
	"github.com/reece01-dock/axos/internal/rollbackctl"
	"github.com/reece01-dock/axos/internal/supervisor"
	"github.com/reece01-dock/axos/internal/svcconfig"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "mcp":
		runMCP(os.Args[2:])
	case "version":
		fmt.Println("axosd 0.2.0-hot-deploy")
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `axosd - AXOS core service daemon

Usage:
  axosd serve [flags]     Run the HTTP Core API (the normal way to run axosd —
                           axos-mcp and axosctl talk to this over the network)
  axosd mcp [flags]       Run the MCP server directly over stdio, in this same
                           process (single-process/legacy mode; no axos-mcp
                           needed, but MCP can't be restarted independently)
  axosd version            Print version

Common flags:
  -backend string    "mock", "replay", or "asuswrt" (default "mock")
  -fixture string     replay: path to a fixture directory
  -host string         asuswrt: run over SSH against this host instead of
                        locally (Live Development Mode)
  -audit string        Path to the append-only audit log (default "./axosd-audit.jsonl")
  -actor string          Identity recorded in audit log entries (mcp mode only —
                          serve mode reads it per-request from X-Axos-Actor)

'serve'-only flags:
  -api-addr string        Address the Core API listens on (default "127.0.0.1:9090" —
                           see docs/security.md before widening this)
  -services-config string  Path to a JSON file registering hot-deployable
                            sibling services for axosd to supervise (optional —
                            see internal/svcconfig; empty/missing is normal)
  -service-log-dir string   Directory for supervised services' stdout/stderr logs`)
}

func backendFlags(fs *flag.FlagSet) *backendselect.Options {
	o := &backendselect.Options{}
	fs.StringVar(&o.Name, "backend", "mock", `Backend implementation: "mock", "replay", or "asuswrt"`)
	fs.StringVar(&o.Fixture, "fixture", "", "replay: path to a fixture directory")
	fs.StringVar(&o.Host, "host", "", "asuswrt: run over SSH against this host instead of locally")
	return o
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	beOpts := backendFlags(fs)
	auditPath := fs.String("audit", "./axosd-audit.jsonl", "Path to the append-only audit log")
	apiAddr := fs.String("api-addr", "127.0.0.1:9090", "Address the Core API listens on")
	servicesConfig := fs.String("services-config", "", "Path to a JSON file registering supervised sibling services (optional)")
	serviceLogDir := fs.String("service-log-dir", "", "Directory for supervised services' logs (optional)")
	_ = fs.Parse(args)

	be, err := backendselect.New(*beOpts)
	if err != nil {
		log.Fatalf("axosd: %v", err)
	}

	al, err := audit.Open(*auditPath)
	if err != nil {
		log.Fatalf("axosd: %v", err)
	}
	defer al.Close()

	rb := newLoggingRollbackEngine(al)
	server := api.NewServer(be, rb, al)

	sup := supervisor.New(*serviceLogDir)
	specs, err := svcconfig.Load(*servicesConfig)
	if err != nil {
		log.Fatalf("axosd: %v", err)
	}
	for _, spec := range specs {
		sup.Register(spec)
		// Auto-start on registration: a supervisor that registers a
		// service but leaves it stopped until someone remembers to call
		// "restart" isn't really supervising it. A start failure here is
		// logged, not fatal — one misconfigured sibling service shouldn't
		// take down axosd's own Core API.
		if err := sup.Start(context.Background(), spec.Name); err != nil {
			log.Printf("axosd: WARNING: failed to start supervised service %q: %v", spec.Name, err)
		} else {
			log.Printf("axosd: started supervised service %q", spec.Name)
		}
	}
	server.Supervisor = sup

	ln, err := net.Listen("tcp", *apiAddr)
	if err != nil {
		log.Fatalf("axosd: listening on %s: %v", *apiAddr, err)
	}
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("axosd: Core API listening on %s (backend=%s, audit=%s)", *apiAddr, beOpts.Name, *auditPath)
	if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatalf("axosd: API server exited with error: %v", err)
	}
}

func runMCP(args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	beOpts := backendFlags(fs)
	auditPath := fs.String("audit", "./axosd-audit.jsonl", "Path to the append-only audit log")
	actor := fs.String("actor", "mcp:unknown", "Identity recorded in audit log entries")
	_ = fs.Parse(args)

	be, err := backendselect.New(*beOpts)
	if err != nil {
		log.Fatalf("axosd: %v", err)
	}

	al, err := audit.Open(*auditPath)
	if err != nil {
		log.Fatalf("axosd: %v", err)
	}
	defer al.Close()

	rb := newLoggingRollbackEngine(al)
	server := mcp.NewServer(be, &rollbackctl.Local{Engine: rb, Backend: be}, al, *actor)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("axosd: MCP server starting in-process (backend=%s, audit=%s)", beOpts.Name, *auditPath)
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("axosd: MCP server exited with error: %v", err)
	}
}

// newLoggingRollbackEngine builds a rollback.Engine wired to record every
// arm/confirm/expire event to the audit log — shared by both run modes so
// that wiring can't drift between them.
func newLoggingRollbackEngine(al *audit.Logger) *rollback.Engine {
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
	return rb
}
