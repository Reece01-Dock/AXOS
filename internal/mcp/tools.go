package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/reece01-dock/axos/internal/rollback"
)

func decodeArgs(raw json.RawMessage, v interface{}) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func handleSystemInfo(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.Info(ctx)
}

func handleSystemResources(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.Resources(ctx)
}

type shellExecArgs struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func handleShellExec(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a shellExecArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.Command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if a.TimeoutSeconds <= 0 {
		a.TimeoutSeconds = 30
	}
	return s.Backend.ShellExec(ctx, a.Command, a.TimeoutSeconds)
}

func handleInterfaces(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.Interfaces(ctx)
}

type routesArgs struct {
	Table string `json:"table"`
}

func handleRoutes(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a routesArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	return s.Backend.Routes(ctx, a.Table)
}

func handleClients(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.Clients(ctx)
}

func handleWiFiStatus(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.WiFiStatus(ctx)
}

func handleFirewallRules(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.FirewallRules(ctx)
}

func handleVPNStatus(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.VPNStatus(ctx)
}

func handleServices(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.Services(ctx)
}

func handleNVRAMDump(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.NVRAMDump(ctx)
}

type rollbackArmArgs struct {
	TimeoutSeconds int    `json:"timeout_seconds"`
	Reason         string `json:"reason"`
}

type rollbackArmResult struct {
	ID       string `json:"id"`
	Deadline string `json:"deadline"`
}

func handleRollbackArm(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a rollbackArmArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.TimeoutSeconds <= 0 {
		return nil, fmt.Errorf("timeout_seconds must be positive")
	}

	// The snapshot-then-arm composition lives in rollbackctl now (Local, or
	// the HTTP-forwarding client talking to axosd) — this handler is just a
	// thin JSON-args translation over whichever Controller this Server was
	// built with. See internal/rollbackctl's package doc for why.
	id, deadline, err := s.Rollback.Arm(ctx, a.TimeoutSeconds, a.Reason)
	if err != nil {
		if errors.Is(err, rollback.ErrBusy) {
			return nil, fmt.Errorf("rollback_busy: a transaction is already armed; confirm or wait for it before arming another")
		}
		return nil, err
	}

	return rollbackArmResult{ID: id, Deadline: deadline.Format("2006-01-02T15:04:05Z07:00")}, nil
}

type rollbackConfirmArgs struct {
	ID string `json:"id"`
}

func handleRollbackConfirm(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a rollbackConfirmArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if err := s.Rollback.Confirm(ctx, a.ID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "confirmed"}, nil
}

func handleRollbackStatus(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Rollback.Status(ctx)
}

type configBackupArgs struct {
	Reason string `json:"reason"`
}

func handleConfigBackup(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a configBackupArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.Reason == "" {
		a.Reason = "manual"
	}
	return s.Backend.Backup(ctx, a.Reason)
}

type configRestoreArgs struct {
	BackupID string `json:"backup_id"`
}

func handleConfigRestore(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a configRestoreArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.BackupID == "" {
		return nil, fmt.Errorf("backup_id is required")
	}
	if err := s.Backend.Restore(ctx, a.BackupID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "restored", "backup_id": a.BackupID}, nil
}

func handleListBackups(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.ListBackups(ctx)
}
