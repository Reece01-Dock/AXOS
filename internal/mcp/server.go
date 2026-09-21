package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/rollbackctl"
)

const serverVersion = "0.1.0-milestone2"

// handlerFunc implements one MCP tool. args is the raw JSON arguments object
// (may be nil/empty for tools that take none).
type handlerFunc func(ctx context.Context, s *Server, args json.RawMessage) (interface{}, error)

type registeredTool struct {
	def     Tool
	handler handlerFunc
	// dangerous tools refuse to run unless a rollback transaction is
	// currently armed (docs/mcp-api.md "Danger enforcement").
	dangerous bool
}

// Server wires a RouterBackend, a rollback controller, and an audit logger
// together and exposes them as MCP tools. The MCP layer owns audit logging
// and danger enforcement so no individual backend method has to.
//
// Rollback is a rollbackctl.Controller, not a concrete *rollback.Engine —
// this Server has no idea (and doesn't need one) whether it's running
// in-process alongside the real engine (axosd's own "axosd mcp" mode) or as
// a separate axos-mcp process talking to axosd's HTTP API. See
// internal/rollbackctl's package doc for why that distinction matters.
type Server struct {
	Backend  backend.RouterBackend
	Rollback rollbackctl.Controller
	Audit    *audit.Logger
	// Actor identifies who is driving this server instance, for audit
	// entries (e.g. "mcp:ai"). Defaults to "mcp:unknown" if empty.
	Actor string

	tools map[string]registeredTool
}

// NewServer builds a Server with all Milestone-2 tools registered.
func NewServer(b backend.RouterBackend, rb rollbackctl.Controller, al *audit.Logger, actor string) *Server {
	if actor == "" {
		actor = "mcp:unknown"
	}
	s := &Server{Backend: b, Rollback: rb, Audit: al, Actor: actor, tools: make(map[string]registeredTool)}
	s.registerTools()
	return s
}

func schema(props string) json.RawMessage {
	if props == "" {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return json.RawMessage(fmt.Sprintf(`{"type":"object","properties":{%s}}`, props))
}

func (s *Server) register(name, description string, inputSchema json.RawMessage, dangerous bool, h handlerFunc) {
	s.tools[name] = registeredTool{
		def:       Tool{Name: name, Description: description, InputSchema: inputSchema},
		handler:   h,
		dangerous: dangerous,
	}
}

func (s *Server) registerTools() {
	s.register("system.info", "Router model, firmware version, uptime.", schema(""), false, handleSystemInfo)
	s.register("system.resources", "CPU load, memory usage, temperatures.", schema(""), false, handleSystemResources)
	s.register("system.shell_exec",
		"Run an arbitrary command as root with a timeout. Escape hatch for when no dedicated tool exists. Fully audited.",
		schema(`"command":{"type":"string"},"timeout_seconds":{"type":"integer","default":30}`),
		false, handleShellExec)

	s.register("network.interfaces", "List network interfaces and their state/counters.", schema(""), false, handleInterfaces)
	s.register("network.routes", "List routing table entries.", schema(`"table":{"type":"string"}`), false, handleRoutes)
	s.register("network.clients", "List known connected devices (DHCP/ARP/Wi-Fi merged).", schema(""), false, handleClients)
	s.register("network.firewall_rules", "List current packet-filtering rules.", schema(""), false, handleFirewallRules)
	s.register("network.vpn_status", "List configured VPN tunnels and their peers (never includes private keys).", schema(""), false, handleVPNStatus)

	s.register("wifi.status", "Wi-Fi radio state: channel, width, clients, SSID.", schema(""), false, handleWiFiStatus)

	s.register("system.services", "Running state of known router-managed services.", schema(""), false, handleServices)
	s.register("system.nvram_dump",
		"Full nvram key/value dump. Contains secrets (Wi-Fi passphrase, admin password) — same trust boundary as this whole API, not a new exposure (docs/security.md). Not audited on read, matching this project's convention that only mutating calls are audited.",
		schema(""), false, handleNVRAMDump)

	s.register("rollback.arm",
		"Arm automatic rollback before a risky change: if rollback.confirm isn't called within timeout_seconds, the snapshot is restored automatically.",
		schema(`"timeout_seconds":{"type":"integer"},"reason":{"type":"string"}`), false, handleRollbackArm)
	s.register("rollback.confirm", "Confirm the armed change, keeping it.", schema(`"id":{"type":"string"}`), false, handleRollbackConfirm)
	s.register("rollback.status", "Current armed transaction, if any.", schema(""), false, handleRollbackStatus)

	s.register("config.backup", "Create a configuration snapshot.", schema(`"reason":{"type":"string"}`), false, handleConfigBackup)
	// config.restore is intentionally NOT gated behind an armed rollback
	// transaction: restoring a backup is itself the recovery action an
	// operator (human or AI) reaches for, often precisely because something
	// else already went wrong — requiring a *second* armed transaction to
	// perform the fix would be circular. It is still fully audited.
	s.register("config.restore", "Restore a named backup by id.", schema(`"backup_id":{"type":"string"}`), false, handleConfigRestore)
	s.register("config.list_backups", "List known backups, newest first.", schema(""), false, handleListBackups)
}

// Serve runs the JSON-RPC loop reading requests from r and writing responses
// to w, one JSON object per line, until r is exhausted or ctx is cancelled.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(w)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(Response{JSONRPC: "2.0", Error: &RPCError{Code: ErrParse, Message: err.Error()}})
			continue
		}
		resp := s.handle(ctx, req)
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("mcp: encode response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp: read request: %w", err)
	}
	return nil
}

func (s *Server) handle(ctx context.Context, req Request) Response {
	base := Response{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		return withResult(base, InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      ServerInfo{Name: "axosd", Version: serverVersion},
		})

	case "tools/list":
		tools := make([]Tool, 0, len(s.tools))
		for _, rt := range s.tools {
			tools = append(tools, rt.def)
		}
		return withResult(base, ToolsListResult{Tools: tools})

	case "tools/call":
		var p ToolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return withError(base, ErrInvalidParams, "invalid tools/call params: "+err.Error())
		}
		return s.callTool(ctx, base, p)

	default:
		return withError(base, ErrMethodNotFound, "unknown method: "+req.Method)
	}
}

func (s *Server) callTool(ctx context.Context, base Response, p ToolCallParams) Response {
	rt, ok := s.tools[p.Name]
	if !ok {
		return withError(base, ErrMethodNotFound, "unknown tool: "+p.Name)
	}

	argsForAudit := rawArgsToMap(p.Arguments)

	if rt.dangerous {
		st, err := s.Rollback.Status(ctx)
		if err != nil {
			// Fail safe: if we can't even verify whether a rollback
			// transaction is armed (e.g. axos-mcp can't reach axosd's
			// API right now), refuse the dangerous action rather than
			// guessing. See docs/mcp-api.md "Danger enforcement".
			s.logAudit(p.Name, argsForAudit, "", fmt.Errorf("rollback_status_unavailable: %w", err))
			return withError(base, ErrRollbackRequired, "could not verify rollback status: "+err.Error())
		}
		if !st.Pending {
			s.logAudit(p.Name, argsForAudit, "", fmt.Errorf("rollback_required"))
			return withError(base, ErrRollbackRequired, "this action requires an armed rollback transaction (call rollback.arm first)")
		}
	}

	result, err := rt.handler(ctx, s, p.Arguments)

	txnID := ""
	if rt.dangerous {
		if st, err := s.Rollback.Status(ctx); err == nil {
			txnID = st.ID
		}
	}

	if err != nil {
		s.logAudit(p.Name, argsForAudit, txnID, err)
		return withResult(base, ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		})
	}

	s.logAudit(p.Name, argsForAudit, txnID, nil)

	payload, mErr := json.Marshal(result)
	if mErr != nil {
		return withError(base, ErrInternal, "marshal result: "+mErr.Error())
	}
	return withResult(base, ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(payload)}}})
}

func (s *Server) logAudit(action string, args map[string]interface{}, txnID string, callErr error) {
	if s.Audit == nil {
		return
	}
	var err error
	if callErr != nil {
		err = s.Audit.Failure(s.Actor, action, args, txnID, callErr)
	} else {
		err = s.Audit.Success(s.Actor, action, args, txnID)
	}
	if err != nil {
		log.Printf("mcp: audit log write failed: %v", err)
	}
}

func rawArgsToMap(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]interface{}{"_raw": string(raw)}
	}
	return m
}

func withResult(base Response, result interface{}) Response {
	base.Result = result
	return base
}

func withError(base Response, code int, msg string) Response {
	base.Error = &RPCError{Code: code, Message: msg}
	return base
}
