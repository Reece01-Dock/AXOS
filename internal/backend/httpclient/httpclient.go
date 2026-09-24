// Package httpclient implements backend.RouterBackend and
// rollbackctl.Controller by calling axosd's HTTP API (internal/api) over
// the network. This is what lets axos-mcp and axosctl run as separate
// processes from axosd — possibly on a different machine entirely (the dev
// PC, per docs/development.md's "MCP Development" — while axosd runs on the
// router) — without embedding router-control logic themselves. Every
// backend.RouterBackend caller (in particular internal/mcp.Server) works
// identically whether it holds a local backend or one of these; that's the
// entire point of the interface.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/rollback"
	"github.com/reece01-dock/axos/internal/rollbackctl"
)

// Client talks to one axosd instance's HTTP API.
type Client struct {
	// BaseURL is e.g. "http://127.0.0.1:9090" or "http://router.lan:9090" —
	// no trailing slash.
	BaseURL string
	// Actor is sent as X-Axos-Actor on every request, for audit attribution
	// on axosd's side (see docs/security.md — this is a label, not auth;
	// the transport/network path is what actually gates access).
	Actor string
	// HTTPClient is used for every request; defaults to a client with a
	// sane timeout if left nil (see New).
	HTTPClient *http.Client
}

// New returns a Client with a default timeout, ready to use.
func New(baseURL, actor string) *Client {
	return &Client{
		BaseURL:    baseURL,
		Actor:      actor,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("httpclient: marshaling request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("httpclient: building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Actor != "" {
		req.Header.Set("X-Axos-Actor", c.Actor)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("httpclient: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("httpclient: reading response from %s %s: %w", method, path, err)
	}

	if resp.StatusCode >= 300 {
		var eb struct {
			Error string `json:"error"`
		}
		if jsonErr := json.Unmarshal(respBody, &eb); jsonErr == nil && eb.Error != "" {
			return apiError{status: resp.StatusCode, message: eb.Error}
		}
		return apiError{status: resp.StatusCode, message: string(respBody)}
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("httpclient: decoding response from %s %s: %w", method, path, err)
	}
	return nil
}

// apiError carries the HTTP status through so callers that switch on
// specific rollback errors (rollback.ErrBusy, rollback.ErrNotFound) can
// still do so — see Is() below.
type apiError struct {
	status  int
	message string
}

func (e apiError) Error() string { return e.message }

// Is lets errors.Is(err, rollback.ErrBusy) / rollback.ErrNotFound work
// across the HTTP boundary, matched by status code (axosd's API sets 409
// for ErrBusy and 404 for ErrNotFound — see internal/api/server.go) rather
// than by parsing the message string, which is not a stable contract.
func (e apiError) Is(target error) bool {
	switch target {
	case rollback.ErrBusy:
		return e.status == http.StatusConflict
	case rollback.ErrNotFound:
		return e.status == http.StatusNotFound
	}
	return false
}

// --- backend.RouterBackend ---------------------------------------------

func (c *Client) Info(ctx context.Context) (backend.SystemInfo, error) {
	var v backend.SystemInfo
	err := c.do(ctx, http.MethodGet, "/v1/info", nil, &v)
	return v, err
}

func (c *Client) Resources(ctx context.Context) (backend.Resources, error) {
	var v backend.Resources
	err := c.do(ctx, http.MethodGet, "/v1/resources", nil, &v)
	return v, err
}

func (c *Client) Interfaces(ctx context.Context) ([]backend.Interface, error) {
	var v []backend.Interface
	err := c.do(ctx, http.MethodGet, "/v1/interfaces", nil, &v)
	return v, err
}

func (c *Client) Routes(ctx context.Context, table string) ([]backend.Route, error) {
	path := "/v1/routes"
	if table != "" {
		path += "?table=" + table
	}
	var v []backend.Route
	err := c.do(ctx, http.MethodGet, path, nil, &v)
	return v, err
}

func (c *Client) Clients(ctx context.Context) ([]backend.Client, error) {
	var v []backend.Client
	err := c.do(ctx, http.MethodGet, "/v1/clients", nil, &v)
	return v, err
}

func (c *Client) WiFiStatus(ctx context.Context) ([]backend.WiFiRadio, error) {
	var v []backend.WiFiRadio
	err := c.do(ctx, http.MethodGet, "/v1/wifi", nil, &v)
	return v, err
}

func (c *Client) Services(ctx context.Context) ([]backend.ServiceStatus, error) {
	var v []backend.ServiceStatus
	err := c.do(ctx, http.MethodGet, "/v1/services", nil, &v)
	return v, err
}

func (c *Client) FirewallRules(ctx context.Context) ([]backend.FirewallRule, error) {
	var v []backend.FirewallRule
	err := c.do(ctx, http.MethodGet, "/v1/firewall", nil, &v)
	return v, err
}

func (c *Client) VPNStatus(ctx context.Context) ([]backend.VPNTunnel, error) {
	var v []backend.VPNTunnel
	err := c.do(ctx, http.MethodGet, "/v1/vpn", nil, &v)
	return v, err
}

func (c *Client) NVRAMDump(ctx context.Context) (map[string]string, error) {
	var v map[string]string
	err := c.do(ctx, http.MethodGet, "/v1/nvram", nil, &v)
	return v, err
}

func (c *Client) ShellExec(ctx context.Context, command string, timeoutSeconds int) (backend.ShellResult, error) {
	var v backend.ShellResult
	err := c.do(ctx, http.MethodPost, "/v1/shell_exec", map[string]interface{}{
		"command": command, "timeout_seconds": timeoutSeconds,
	}, &v)
	return v, err
}

func (c *Client) Backup(ctx context.Context, reason string) (backend.BackupInfo, error) {
	var v backend.BackupInfo
	err := c.do(ctx, http.MethodPost, "/v1/backup", map[string]interface{}{"reason": reason}, &v)
	return v, err
}

func (c *Client) Restore(ctx context.Context, backupID string) error {
	return c.do(ctx, http.MethodPost, "/v1/restore", map[string]interface{}{"backup_id": backupID}, nil)
}

func (c *Client) ListBackups(ctx context.Context) ([]backend.BackupInfo, error) {
	var v []backend.BackupInfo
	err := c.do(ctx, http.MethodGet, "/v1/backups", nil, &v)
	return v, err
}

// --- Milestone 3 -------------------------------------------------------

func (c *Client) Ping(ctx context.Context, host string, count int) (backend.DiagResult, error) {
	var v backend.DiagResult
	err := c.do(ctx, http.MethodPost, "/v1/diag/ping", map[string]interface{}{
		"host": host, "count": count,
	}, &v)
	return v, err
}

func (c *Client) Traceroute(ctx context.Context, host string, maxHops int) (backend.DiagResult, error) {
	var v backend.DiagResult
	err := c.do(ctx, http.MethodPost, "/v1/diag/traceroute", map[string]interface{}{
		"host": host, "max_hops": maxHops,
	}, &v)
	return v, err
}

func (c *Client) DNSLookup(ctx context.Context, name string) (backend.DiagResult, error) {
	var v backend.DiagResult
	err := c.do(ctx, http.MethodPost, "/v1/diag/dns", map[string]interface{}{"name": name}, &v)
	return v, err
}

func (c *Client) PortCheck(ctx context.Context, host string, port int) (backend.DiagResult, error) {
	var v backend.DiagResult
	err := c.do(ctx, http.MethodPost, "/v1/diag/port", map[string]interface{}{
		"host": host, "port": port,
	}, &v)
	return v, err
}

func (c *Client) Iperf3(ctx context.Context, opts backend.IperfOpts) (backend.PerfResult, error) {
	var v backend.PerfResult
	err := c.do(ctx, http.MethodPost, "/v1/perf/iperf3", opts, &v)
	return v, err
}

func (c *Client) DNSConfig(ctx context.Context) (backend.DNSInfo, error) {
	var v backend.DNSInfo
	err := c.do(ctx, http.MethodGet, "/v1/dns", nil, &v)
	return v, err
}

func (c *Client) SetDNSConfig(ctx context.Context, cfg backend.DNSInfo) error {
	return c.do(ctx, http.MethodPost, "/v1/dns", cfg, nil)
}

func (c *Client) DHCPReservations(ctx context.Context) ([]backend.DHCPReservation, error) {
	var v []backend.DHCPReservation
	err := c.do(ctx, http.MethodGet, "/v1/dhcp/reservations", nil, &v)
	return v, err
}

func (c *Client) SetDHCPReservation(ctx context.Context, r backend.DHCPReservation) error {
	return c.do(ctx, http.MethodPost, "/v1/dhcp/reservations", r, nil)
}

func (c *Client) DeleteDHCPReservation(ctx context.Context, mac string) error {
	return c.do(ctx, http.MethodDelete, "/v1/dhcp/reservations/"+url.PathEscape(mac), nil, nil)
}

func (c *Client) QoSStatus(ctx context.Context) (backend.QoSInfo, error) {
	var v backend.QoSInfo
	err := c.do(ctx, http.MethodGet, "/v1/qos", nil, &v)
	return v, err
}

func (c *Client) SetQoSEnable(ctx context.Context, enabled bool) error {
	return c.do(ctx, http.MethodPost, "/v1/qos", map[string]interface{}{"enabled": enabled}, nil)
}

func (c *Client) VPNProfiles(ctx context.Context) ([]backend.VPNProfile, error) {
	var v []backend.VPNProfile
	err := c.do(ctx, http.MethodGet, "/v1/vpn/profiles", nil, &v)
	return v, err
}

func (c *Client) ImportWireGuard(ctx context.Context, p backend.WireGuardImport) error {
	return c.do(ctx, http.MethodPost, "/v1/vpn/wireguard/import", p, nil)
}

func (c *Client) VPNUp(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/v1/vpn/"+url.PathEscape(name)+"/up", nil, nil)
}

func (c *Client) VPNDown(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/v1/vpn/"+url.PathEscape(name)+"/down", nil, nil)
}

// VPNBenchmark ranks hosts via the Core API (not part of RouterBackend).
func (c *Client) VPNBenchmark(ctx context.Context, hosts []string, count int) (map[string]interface{}, error) {
	var v map[string]interface{}
	err := c.do(ctx, http.MethodPost, "/v1/vpn/benchmark", map[string]interface{}{
		"hosts": hosts, "count": count,
	}, &v)
	return v, err
}

func (c *Client) FirewallApply(ctx context.Context, rule backend.FirewallRule) error {
	return c.do(ctx, http.MethodPost, "/v1/firewall/apply", rule, nil)
}

func (c *Client) FirewallDelete(ctx context.Context, rule backend.FirewallRule) error {
	return c.do(ctx, http.MethodPost, "/v1/firewall/delete", rule, nil)
}

func (c *Client) PolicyRoutes(ctx context.Context) ([]backend.PolicyRoute, error) {
	var v []backend.PolicyRoute
	err := c.do(ctx, http.MethodGet, "/v1/policy", nil, &v)
	return v, err
}

func (c *Client) SetPolicyRoute(ctx context.Context, r backend.PolicyRoute) error {
	return c.do(ctx, http.MethodPost, "/v1/policy", r, nil)
}

func (c *Client) ReplacePolicyRoutes(ctx context.Context, routes []backend.PolicyRoute) error {
	return c.do(ctx, http.MethodPut, "/v1/policy", routes, nil)
}

func (c *Client) DeletePolicyRoute(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/policy/"+url.PathEscape(id), nil, nil)
}

var _ backend.RouterBackend = (*Client)(nil)

// --- rollbackctl.Controller ----------------------------------------------

func (c *Client) Arm(ctx context.Context, timeoutSeconds int, reason string) (string, time.Time, error) {
	var v struct {
		ID       string    `json:"id"`
		Deadline time.Time `json:"deadline"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/rollback/arm", map[string]interface{}{
		"timeout_seconds": timeoutSeconds, "reason": reason,
	}, &v)
	return v.ID, v.Deadline, err
}

func (c *Client) Confirm(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/v1/rollback/confirm", map[string]interface{}{"id": id}, nil)
}

func (c *Client) Status(ctx context.Context) (rollback.Status, error) {
	var v rollback.Status
	err := c.do(ctx, http.MethodGet, "/v1/rollback/status", nil, &v)
	return v, err
}

var _ rollbackctl.Controller = (*Client)(nil)
