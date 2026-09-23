// Command axosd is the AXOS core service daemon: it owns the RouterBackend,
// the rollback engine, and the audit log, and exposes them either directly
// over MCP (single-process mode) or over the HTTP Core API that axos-mcp
// and axosctl talk to (see docs/architecture.md, docs/development.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reece01-dock/axos/internal/api"
	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/backendselect"
	"github.com/reece01-dock/axos/internal/footprint"
	"github.com/reece01-dock/axos/internal/mcp"
	"github.com/reece01-dock/axos/internal/rollback"
	"github.com/reece01-dock/axos/internal/rollbackctl"
	"github.com/reece01-dock/axos/internal/supervisor"
	"github.com/reece01-dock/axos/internal/svcconfig"
	"github.com/reece01-dock/axos/web"
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
  -backup-dir string   asuswrt: config.backup storage dir (default
                        /jffs/axos/backups)
  -audit string        Path to the append-only audit log (default "./axosd-audit.jsonl")
  -actor string          Identity recorded in audit log entries (mcp mode only —
                          serve mode reads it per-request from X-Axos-Actor)

'serve'-only flags:
  -api-addr string        Address the Core API listens on (default "127.0.0.1:9090" —
                           see docs/security.md before widening this)
  -ui-dir string          Directory of web UI static assets (index.html, styles.css,
                            app.js). If set and the directory exists, serve from disk
                            (hot-deploy under <axos-root>/www/). Otherwise use the
                            assets embedded in this binary.
  -ui-token-file string   Non-loopback API clients must send X-Axos-UI-Token
                            (Merlin UI embed). Loopback never requires it.
  -services-config string  Path to a JSON file registering hot-deployable
                            sibling services for axosd to supervise (optional —
                            see internal/svcconfig; empty/missing is normal)
  -service-log-dir string   Directory for supervised services' stdout/stderr logs
  -footprint string        Path to the append-only RAM footprint log (default
                            "./axosd-footprint.jsonl") — see internal/footprint`)
}

func backendFlags(fs *flag.FlagSet) *backendselect.Options {
	o := &backendselect.Options{}
	fs.StringVar(&o.Name, "backend", "mock", `Backend implementation: "mock", "replay", or "asuswrt"`)
	fs.StringVar(&o.Fixture, "fixture", "", "replay: path to a fixture directory")
	fs.StringVar(&o.Host, "host", "", "asuswrt: run over SSH against this host instead of locally")
	fs.StringVar(&o.BackupDir, "backup-dir", "", "asuswrt: directory for config.backup snapshots (default /jffs/axos/backups)")
	return o
}

func runServe(args []string) {
	flagSet := flag.NewFlagSet("serve", flag.ExitOnError)
	beOpts := backendFlags(flagSet)
	auditPath := flagSet.String("audit", "./axosd-audit.jsonl", "Path to the append-only audit log")
	apiAddr := flagSet.String("api-addr", "127.0.0.1:9090", "Address the Core API listens on")
	uiDir := flagSet.String("ui-dir", "", "Directory of web UI static assets; if set and exists, serve from disk, else embedded web.FS")
	uiTokenFile := flagSet.String("ui-token-file", "", "If set, non-loopback API clients must send X-Axos-UI-Token (Merlin UI); loopback never requires it")
	servicesConfig := flagSet.String("services-config", "", "Path to a JSON file registering supervised sibling services (optional)")
	serviceLogDir := flagSet.String("service-log-dir", "", "Directory for supervised services' logs (optional)")
	footprintPath := flagSet.String("footprint", "./axosd-footprint.jsonl", "Path to the append-only RAM footprint log")
	_ = flagSet.Parse(args)

	be, err := backendselect.New(*beOpts)
	if err != nil {
		log.Fatalf("axosd: %v", err)
	}

	al, err := audit.Open(*auditPath)
	if err != nil {
		log.Fatalf("axosd: %v", err)
	}
	defer al.Close()

	fp, err := footprint.Open(*footprintPath)
	if err != nil {
		log.Fatalf("axosd: %v", err)
	}
	defer fp.Close()
	recordFootprintSnapshot(fp, be, "startup")

	rb := newLoggingRollbackEngine(al)
	server := api.NewServer(be, rb, al)
	server.Footprint = fp
	server.SetUI(resolveUIFS(*uiDir))
	server.UITokenFile = *uiTokenFile

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

// resolveUIFS returns a disk FS when uiDir is set and exists as a directory;
// otherwise the embedded web.FS fallback (Phase 7 hot-deploy vs bake-in).
func resolveUIFS(uiDir string) fs.FS {
	if uiDir != "" {
		if st, err := os.Stat(uiDir); err == nil && st.IsDir() {
			log.Printf("axosd: serving web UI from %s", uiDir)
			return os.DirFS(uiDir)
		}
		log.Printf("axosd: -ui-dir=%s not found or not a directory — using embedded web UI", uiDir)
	}
	return web.FS
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

// recordFootprintSnapshot measures and records one footprint.Snapshot,
// labelling it "baseline" instead of label if this is the very first
// snapshot this install has ever recorded (see internal/footprint's package
// doc) — so a fresh install's first-ever axosd start captures the
// before-any-feature baseline automatically, with no separate manual step.
// System-wide memory (from be.Resources) is attached best-effort: a backend
// error here shouldn't block startup or lose the process-level measurement.
func recordFootprintSnapshot(fp *footprint.Store, be backend.RouterBackend, label string) {
	if fp.IsEmpty() {
		label = "baseline"
	}
	snap := footprint.Measure(label, "")
	if res, err := be.Resources(context.Background()); err == nil {
		snap.SystemTotalKB, snap.SystemUsedKB, snap.SystemFreeKB = res.MemTotalKB, res.MemUsedKB, res.MemFreeKB
	}
	if err := fp.Record(snap); err != nil {
		log.Printf("axosd: WARNING: failed to record footprint snapshot: %v", err)
		return
	}
	log.Printf("axosd: recorded %q footprint snapshot (process_rss=%dKB, system_used=%dKB/%dKB)",
		label, snap.ProcessRSSKB, snap.SystemUsedKB, snap.SystemTotalKB)
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
