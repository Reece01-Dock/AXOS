// Package api implements the AXOS Core API: an HTTP/JSON surface over
// RouterBackend + the rollback engine + the audit log, hosted by axosd.
// This is what makes independent-process frontends possible — axos-mcp and
// axosctl talk to this over HTTP (via internal/backend/httpclient) instead
// of embedding a RouterBackend directly, so restarting either of them never
// touches axosd's in-flight state (an armed rollback timer, the audit log
// file handle, ...). See docs/development.md "Shared API".
//
// Security posture (see docs/security.md): this API carries the same
// authority as axosd itself — including full nvram (secrets) and
// system.shell_exec. It defaults to binding 127.0.0.1 only (see
// cmd/axosd's -api-addr flag); widening that bind address is a deliberate,
// separately-considered choice, not this package's default.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/footprint"
	"github.com/reece01-dock/axos/internal/rollback"
	"github.com/reece01-dock/axos/internal/rollbackctl"
	"github.com/reece01-dock/axos/internal/supervisor"
)

// Server hosts the AXOS Core API. Build one with NewServer and pass its
// Handler() to an http.Server (cmd/axosd does this) — Server itself doesn't
// open a socket, keeping it trivially testable with httptest.
type Server struct {
	Backend  backend.RouterBackend
	Rollback *rollback.Engine
	Audit    *audit.Logger
	// Supervisor is optional (nil by default): axosd only manages
	// hot-deployable sibling services once there are real ones worth
	// supervising (see cmd/axosd's -services-config flag). When nil, the
	// /v1/supervisor/services/* routes respond 501 rather than pretending —
	// no service is silently faked as "running".
	Supervisor *supervisor.Supervisor
	// Footprint is optional (nil by default): set by cmd/axosd once it has
	// opened its RAM footprint log (see internal/footprint). When nil, the
	// /v1/footprint* routes respond 501, matching the Supervisor convention
	// above.
	Footprint *footprint.Store
	// uiFS, when set (see SetUI), serves the Phase 7 web UI at GET / and
	// GET /ui/*. Nil means those routes 404 — /v1/* and /healthz are
	// unaffected either way.
	uiFS fs.FS
	// UITokenFile, when set, enables LAN access to the Core API for the
	// Merlin-embedded UI (non-loopback clients must send X-Axos-UI-Token).
	// Loopback (MCP / SSH tunnel) never requires the token.
	UITokenFile string
	// DataDir is the AXOS state root (e.g. /jffs/axos). Used for client
	// groups and other small JSON state under DataDir/run/.
	DataDir string

	local *rollbackctl.Local // reuses the same snapshot-then-arm composition Local implements
	mux   *http.ServeMux
}

// NewServer wires up all routes. Supervisor is optional — pass nil if this
// axosd instance has no hot-deployable sibling services to manage yet (see
// the Supervisor field's doc comment); set it directly on the returned
// *Server before serving if you do.
func NewServer(be backend.RouterBackend, rb *rollback.Engine, al *audit.Logger) *Server {
	s := &Server{
		Backend:  be,
		Rollback: rb,
		Audit:    al,
		local:    &rollbackctl.Local{Engine: rb, Backend: be},
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns the http.Handler to serve — pass to http.Server.Handler.
// When UITokenFile is set, the handler is wrapped with LANAuth (CORS +
// token gate for non-loopback callers).
func (s *Server) Handler() http.Handler {
	h := http.Handler(s.mux)
	if s.UITokenFile != "" {
		h = &LANAuth{Inner: h, TokenFile: s.UITokenFile}
	}
	return h
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)

	s.mux.HandleFunc("GET /v1/info", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.Info(ctx) }))
	s.mux.HandleFunc("GET /v1/resources", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.Resources(ctx) }))
	s.mux.HandleFunc("GET /v1/interfaces", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.Interfaces(ctx) }))
	s.mux.HandleFunc("GET /v1/clients", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.Clients(ctx) }))
	s.mux.HandleFunc("GET /v1/wifi", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.WiFiStatus(ctx) }))
	s.mux.HandleFunc("GET /v1/services", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.Services(ctx) }))
	s.mux.HandleFunc("GET /v1/firewall", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.FirewallRules(ctx) }))
	s.mux.HandleFunc("GET /v1/vpn", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.VPNStatus(ctx) }))
	// nvram carries secrets — same trust boundary as the rest of this API
	// (docs/security.md "who can reach axosd"), not a separate exposure.
	s.mux.HandleFunc("GET /v1/nvram", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.NVRAMDump(ctx) }))
	s.mux.HandleFunc("GET /v1/routes", s.handleRoutes)
	s.mux.HandleFunc("GET /v1/backups", s.readHandler(func(ctx context.Context) (interface{}, error) { return s.Backend.ListBackups(ctx) }))

	s.mux.HandleFunc("GET /v1/footprint", s.handleFootprint)
	s.mux.HandleFunc("GET /v1/footprint/history", s.handleFootprintHistory)
	s.mux.HandleFunc("POST /v1/footprint/snapshot", s.handleFootprintSnapshot)

	s.mux.HandleFunc("POST /v1/shell_exec", s.handleShellExec)
	s.mux.HandleFunc("POST /v1/backup", s.handleBackup)
	s.mux.HandleFunc("POST /v1/restore", s.handleRestore)

	s.mux.HandleFunc("POST /v1/rollback/arm", s.handleRollbackArm)
	s.mux.HandleFunc("POST /v1/rollback/confirm", s.handleRollbackConfirm)
	s.mux.HandleFunc("GET /v1/rollback/status", s.handleRollbackStatus)

	// Note the /v1/supervisor/ prefix, distinct from /v1/services above:
	// that route is RouterBackend.Services() (router-native OS services
	// like dnsmasq/httpd); these are AXOS's own hot-deployable sibling
	// processes (axos-mcp, axos-monitor, ...) managed by internal/supervisor
	// — a different concept that happens to share the word "services".
	s.mux.HandleFunc("GET /v1/supervisor/services", s.handleServicesStatus)
	s.mux.HandleFunc("GET /v1/supervisor/services/{name}/status", s.handleServiceStatus)
	s.mux.HandleFunc("GET /v1/supervisor/services/{name}/health", s.handleServiceHealth)
	s.mux.HandleFunc("POST /v1/supervisor/services/{name}/start", s.handleServiceStart)
	s.mux.HandleFunc("POST /v1/supervisor/services/{name}/stop", s.handleServiceStop)
	s.mux.HandleFunc("POST /v1/supervisor/services/{name}/restart", s.handleServiceRestart)
	s.mux.HandleFunc("GET /v1/supervisor/services/{name}/logs", s.handleServiceLogs)

	s.registerM3Routes()
	s.registerUIRoutes()
}

func (s *Server) requireSupervisor(w http.ResponseWriter) bool {
	if s.Supervisor == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("this axosd instance has no supervised services configured"))
		return false
	}
	return true
}

func (s *Server) requireFootprint(w http.ResponseWriter) bool {
	if s.Footprint == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("this axosd instance has no footprint log configured"))
		return false
	}
	return true
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readHandler adapts a no-argument RouterBackend read call into an
// http.HandlerFunc. Read calls aren't audited individually (see
// docs/mcp-api.md — only mutating calls are) and take no request body.
func (s *Server) readHandler(fn func(ctx context.Context) (interface{}, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := fn(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	routes, err := s.Backend.Routes(r.Context(), table)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, routes)
}

func (s *Server) actor(r *http.Request) string {
	if a := r.Header.Get("X-Axos-Actor"); a != "" {
		return a
	}
	return "api:unknown"
}

type shellExecRequest struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (s *Server) handleShellExec(w http.ResponseWriter, r *http.Request) {
	var req shellExecRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("command is required"))
		return
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 30
	}

	result, err := s.Backend.ShellExec(r.Context(), req.Command, req.TimeoutSeconds)
	s.audit(s.actor(r), "system.shell_exec", map[string]interface{}{"command": req.Command}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type backupRequest struct {
	Reason string `json:"reason"`
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	var req backupRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Reason == "" {
		req.Reason = "manual"
	}
	info, err := s.Backend.Backup(r.Context(), req.Reason)
	s.audit(s.actor(r), "config.backup", map[string]interface{}{"reason": req.Reason}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

type restoreRequest struct {
	BackupID string `json:"backup_id"`
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	var req restoreRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.BackupID == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("backup_id is required"))
		return
	}
	err := s.Backend.Restore(r.Context(), req.BackupID)
	s.audit(s.actor(r), "config.restore", map[string]interface{}{"backup_id": req.BackupID}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "backup_id": req.BackupID})
}

type rollbackArmRequest struct {
	TimeoutSeconds int    `json:"timeout_seconds"`
	Reason         string `json:"reason"`
}

type rollbackArmResponse struct {
	ID       string    `json:"id"`
	Deadline time.Time `json:"deadline"`
}

func (s *Server) handleRollbackArm(w http.ResponseWriter, r *http.Request) {
	var req rollbackArmRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	id, deadline, err := s.local.Arm(r.Context(), req.TimeoutSeconds, req.Reason)
	s.audit(s.actor(r), "rollback.arm", map[string]interface{}{"timeout_seconds": req.TimeoutSeconds, "reason": req.Reason}, id, err)
	if err != nil {
		status := http.StatusBadGateway
		if err == rollback.ErrBusy {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, rollbackArmResponse{ID: id, Deadline: deadline})
}

type rollbackConfirmRequest struct {
	ID string `json:"id"`
}

func (s *Server) handleRollbackConfirm(w http.ResponseWriter, r *http.Request) {
	var req rollbackConfirmRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	err := s.local.Confirm(r.Context(), req.ID)
	s.audit(s.actor(r), "rollback.confirm", map[string]interface{}{"id": req.ID}, req.ID, err)
	if err != nil {
		status := http.StatusBadGateway
		if err == rollback.ErrNotFound {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

func (s *Server) handleRollbackStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.local.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// audit is a thin wrapper so every handler's audit call reads the same way;
// a nil Audit logger (not expected in production, but convenient in tests)
// is a silent no-op rather than a panic.
func (s *Server) audit(actor, action string, args map[string]interface{}, txnID string, callErr error) {
	if s.Audit == nil {
		return
	}
	if callErr != nil {
		_ = s.Audit.Failure(actor, action, args, txnID, callErr)
	} else {
		_ = s.Audit.Success(actor, action, args, txnID)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorBody{Error: err.Error()})
}

func readJSON(r *http.Request, dst interface{}) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil // empty body is fine — every request struct's fields are optional-with-defaults or validated by the handler
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

// --- Footprint (AXOS's own RAM usage over time) ------------------------

// footprintResponse pairs a fresh live measurement with the install's
// baseline, so a caller can see the growth delta without a second request.
type footprintResponse struct {
	Current  footprint.Snapshot  `json:"current"`
	Baseline *footprint.Snapshot `json:"baseline,omitempty"`
}

// withSystemMemory attaches the router's current whole-system memory
// figures to snap, best-effort: a Resources() failure just leaves them
// zero rather than failing the footprint measurement itself, since the
// process-level numbers are the ones that actually matter here.
func (s *Server) withSystemMemory(ctx context.Context, snap footprint.Snapshot) footprint.Snapshot {
	if res, err := s.Backend.Resources(ctx); err == nil {
		snap.SystemTotalKB, snap.SystemUsedKB, snap.SystemFreeKB = res.MemTotalKB, res.MemUsedKB, res.MemFreeKB
	}
	return snap
}

func (s *Server) handleFootprint(w http.ResponseWriter, r *http.Request) {
	if !s.requireFootprint(w) {
		return
	}
	resp := footprintResponse{Current: s.withSystemMemory(r.Context(), footprint.Measure("live", ""))}
	if base, ok, err := s.Footprint.Baseline(); err == nil && ok {
		resp.Baseline = &base
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleFootprintHistory(w http.ResponseWriter, r *http.Request) {
	if !s.requireFootprint(w) {
		return
	}
	hist, err := s.Footprint.History()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

type footprintSnapshotRequest struct {
	Label     string `json:"label"`
	ReleaseID string `json:"release_id"`
}

func (s *Server) handleFootprintSnapshot(w http.ResponseWriter, r *http.Request) {
	if !s.requireFootprint(w) {
		return
	}
	var req footprintSnapshotRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Label == "" {
		req.Label = "manual"
	}
	snap := s.withSystemMemory(r.Context(), footprint.Measure(req.Label, req.ReleaseID))
	err := s.Footprint.Record(snap)
	s.audit(s.actor(r), "footprint.snapshot", map[string]interface{}{"label": req.Label, "release_id": req.ReleaseID}, "", err)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// --- Supervisor (hot-deployable sibling services) ---------------------
//
// All routes here 501 if Supervisor is nil (see requireSupervisor) — axosd
// doesn't pretend to manage services it isn't actually configured to.

func (s *Server) handleServicesStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireSupervisor(w) {
		return
	}
	writeJSON(w, http.StatusOK, s.Supervisor.StatusAll())
}

func (s *Server) handleServiceStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireSupervisor(w) {
		return
	}
	st, err := s.Supervisor.Status(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleServiceHealth(w http.ResponseWriter, r *http.Request) {
	if !s.requireSupervisor(w) {
		return
	}
	h, err := s.Supervisor.HealthCheck(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleServiceStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireSupervisor(w) {
		return
	}
	name := r.PathValue("name")
	err := s.Supervisor.Start(r.Context(), name)
	s.audit(s.actor(r), "service.start", map[string]interface{}{"name": name}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleServiceStop(w http.ResponseWriter, r *http.Request) {
	if !s.requireSupervisor(w) {
		return
	}
	name := r.PathValue("name")
	err := s.Supervisor.Stop(r.Context(), name)
	s.audit(s.actor(r), "service.stop", map[string]interface{}{"name": name}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	if !s.requireSupervisor(w) {
		return
	}
	name := r.PathValue("name")
	err := s.Supervisor.Restart(r.Context(), name)
	s.audit(s.actor(r), "service.restart", map[string]interface{}{"name": name}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

// handleServiceLogs returns the tail of a service's log file (see
// Supervisor.LogDir). Not audited — reading logs isn't a mutating action.
func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireSupervisor(w) {
		return
	}
	name := r.PathValue("name")
	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			lines = n
		}
	}

	if s.Supervisor.LogDir == "" {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("this axosd instance has no service log directory configured"))
		return
	}
	data, err := os.ReadFile(filepath.Join(s.Supervisor.LogDir, name+".log"))
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("no log file for service %q: %w", name, err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"log": tailLines(string(data), lines)})
}

// tailLines returns at most the last n lines of s.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
