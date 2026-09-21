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
	"net/http"
	"time"

	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/rollback"
	"github.com/reece01-dock/axos/internal/rollbackctl"
)

// Server hosts the AXOS Core API. Build one with NewServer and pass its
// Handler() to an http.Server (cmd/axosd does this) — Server itself doesn't
// open a socket, keeping it trivially testable with httptest.
type Server struct {
	Backend  backend.RouterBackend
	Rollback *rollback.Engine
	Audit    *audit.Logger

	local *rollbackctl.Local // reuses the same snapshot-then-arm composition Local implements
	mux   *http.ServeMux
}

// NewServer wires up all routes.
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
func (s *Server) Handler() http.Handler { return s.mux }

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

	s.mux.HandleFunc("POST /v1/shell_exec", s.handleShellExec)
	s.mux.HandleFunc("POST /v1/backup", s.handleBackup)
	s.mux.HandleFunc("POST /v1/restore", s.handleRestore)

	s.mux.HandleFunc("POST /v1/rollback/arm", s.handleRollbackArm)
	s.mux.HandleFunc("POST /v1/rollback/confirm", s.handleRollbackConfirm)
	s.mux.HandleFunc("GET /v1/rollback/status", s.handleRollbackStatus)
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
