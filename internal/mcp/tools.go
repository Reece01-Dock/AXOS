package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/rollback"
	"github.com/reece01-dock/axos/internal/vpnbench"
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

// --- Milestone 3 -------------------------------------------------------

type diagPingArgs struct {
	Host  string `json:"host"`
	Count int    `json:"count"`
}

func handleDiagPing(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a diagPingArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if a.Count <= 0 {
		a.Count = 4
	}
	return s.Backend.Ping(ctx, a.Host, a.Count)
}

type diagTracerouteArgs struct {
	Host    string `json:"host"`
	MaxHops int    `json:"max_hops"`
}

func handleDiagTraceroute(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a diagTracerouteArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if a.MaxHops <= 0 {
		a.MaxHops = 30
	}
	return s.Backend.Traceroute(ctx, a.Host, a.MaxHops)
}

type diagDNSArgs struct {
	Name string `json:"name"`
}

func handleDiagDNSLookup(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a diagDNSArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return s.Backend.DNSLookup(ctx, a.Name)
}

type diagPortArgs struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func handleDiagPortCheck(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a diagPortArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.Host == "" || a.Port <= 0 {
		return nil, fmt.Errorf("host and port are required")
	}
	return s.Backend.PortCheck(ctx, a.Host, a.Port)
}

func handleIperf3(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var opts backend.IperfOpts
	if err := decodeArgs(raw, &opts); err != nil {
		return nil, err
	}
	return s.Backend.Iperf3(ctx, opts)
}

func handleDNSGet(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.DNSConfig(ctx)
}

func handleDNSSet(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var cfg backend.DNSInfo
	if err := decodeArgs(raw, &cfg); err != nil {
		return nil, err
	}
	if err := s.Backend.SetDNSConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

func handleDHCPReservations(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.DHCPReservations(ctx)
}

func handleDHCPReservationsSet(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var r backend.DHCPReservation
	if err := decodeArgs(raw, &r); err != nil {
		return nil, err
	}
	if r.MAC == "" || r.IP == "" {
		return nil, fmt.Errorf("mac and ip are required")
	}
	if err := s.Backend.SetDHCPReservation(ctx, r); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

type dhcpDeleteArgs struct {
	MAC string `json:"mac"`
}

func handleDHCPReservationsDelete(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a dhcpDeleteArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.MAC == "" {
		return nil, fmt.Errorf("mac is required")
	}
	if err := s.Backend.DeleteDHCPReservation(ctx, a.MAC); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

func handleQoSStatus(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.QoSStatus(ctx)
}

type qosSetArgs struct {
	Enabled bool `json:"enabled"`
}

func handleQoSSet(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a qosSetArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if err := s.Backend.SetQoSEnable(ctx, a.Enabled); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

func handleVPNList(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.VPNProfiles(ctx)
}

type vpnBenchmarkArgs struct {
	Hosts []string `json:"hosts"`
	Count int      `json:"count"`
}

func handleVPNBenchmark(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a vpnBenchmarkArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	return vpnbench.Run(ctx, s.Backend, a.Hosts, a.Count)
}

func handleVPNWireGuardImport(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var p backend.WireGuardImport
	if err := decodeArgs(raw, &p); err != nil {
		return nil, err
	}
	if p.Unit < 1 || p.PrivateKey == "" || p.PeerPublicKey == "" {
		return nil, fmt.Errorf("unit, private_key, and peer_public_key are required")
	}
	if err := s.Backend.ImportWireGuard(ctx, p); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

type vpnNameArgs struct {
	Name string `json:"name"`
}

func handleVPNUp(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a vpnNameArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.Backend.VPNUp(ctx, a.Name); err != nil {
		return nil, err
	}
	return map[string]string{"status": "up", "name": a.Name}, nil
}

func handleVPNDown(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a vpnNameArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.Backend.VPNDown(ctx, a.Name); err != nil {
		return nil, err
	}
	return map[string]string{"status": "down", "name": a.Name}, nil
}

func handleFirewallRulesSet(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var rule backend.FirewallRule
	if err := decodeArgs(raw, &rule); err != nil {
		return nil, err
	}
	if err := s.Backend.FirewallApply(ctx, rule); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

func handleFirewallRulesDelete(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var rule backend.FirewallRule
	if err := decodeArgs(raw, &rule); err != nil {
		return nil, err
	}
	if err := s.Backend.FirewallDelete(ctx, rule); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

func handlePolicyList(ctx context.Context, s *Server, _ json.RawMessage) (interface{}, error) {
	return s.Backend.PolicyRoutes(ctx)
}

func handlePolicySet(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var route backend.PolicyRoute
	if err := decodeArgs(raw, &route); err != nil {
		return nil, err
	}
	if route.Source == "" || route.Interface == "" {
		return nil, fmt.Errorf("source and interface are required")
	}
	if err := s.Backend.SetPolicyRoute(ctx, route); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

type policyDeleteArgs struct {
	ID string `json:"id"`
}

func handlePolicyDelete(ctx context.Context, s *Server, raw json.RawMessage) (interface{}, error) {
	var a policyDeleteArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	if a.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if err := s.Backend.DeletePolicyRoute(ctx, a.ID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}
