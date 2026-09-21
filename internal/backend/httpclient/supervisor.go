package httpclient

import (
	"context"
	"net/http"
	"strconv"

	"github.com/reece01-dock/axos/internal/supervisor"
)

// This file is httpclient.Client's other half: calls to axosd's
// /v1/supervisor/services/* routes (internal/api's supervisor control
// surface), for axosctl's restart/logs/status/health commands. Same
// Client, same transport/auth as the RouterBackend and rollbackctl.Controller
// methods in httpclient.go — one client for everything axosd's API exposes.

func (c *Client) ServicesStatus(ctx context.Context) ([]supervisor.State, error) {
	var v []supervisor.State
	err := c.do(ctx, http.MethodGet, "/v1/supervisor/services", nil, &v)
	return v, err
}

func (c *Client) ServiceStatus(ctx context.Context, name string) (supervisor.State, error) {
	var v supervisor.State
	err := c.do(ctx, http.MethodGet, "/v1/supervisor/services/"+name+"/status", nil, &v)
	return v, err
}

func (c *Client) ServiceHealth(ctx context.Context, name string) (supervisor.Health, error) {
	var v supervisor.Health
	err := c.do(ctx, http.MethodGet, "/v1/supervisor/services/"+name+"/health", nil, &v)
	return v, err
}

func (c *Client) ServiceStart(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/v1/supervisor/services/"+name+"/start", nil, nil)
}

func (c *Client) ServiceStop(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/v1/supervisor/services/"+name+"/stop", nil, nil)
}

func (c *Client) ServiceRestart(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/v1/supervisor/services/"+name+"/restart", nil, nil)
}

func (c *Client) ServiceLogs(ctx context.Context, name string, lines int) (string, error) {
	path := "/v1/supervisor/services/" + name + "/logs"
	if lines > 0 {
		path += "?lines=" + strconv.Itoa(lines)
	}
	var v struct {
		Log string `json:"log"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &v)
	return v.Log, err
}
