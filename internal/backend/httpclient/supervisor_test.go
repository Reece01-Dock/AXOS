package httpclient

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/reece01-dock/axos/internal/api"
	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend/mock"
	"github.com/reece01-dock/axos/internal/rollback"
	"github.com/reece01-dock/axos/internal/supervisor"
)

func newTestClientWithSupervisor(t *testing.T) (*Client, func()) {
	t.Helper()
	mb := mock.New()
	rb := rollback.New()
	al := audit.New(&bytes.Buffer{})
	apiServer := api.NewServer(mb, rb, al)

	sup := supervisor.New(t.TempDir())
	sup.Register(supervisor.ServiceSpec{
		Name: "sleeper", Command: "sh", Args: []string{"-c", "echo booted && sleep 5"},
		RestartBackoffBase: 20 * time.Millisecond, RestartBackoffMax: 50 * time.Millisecond,
	})
	apiServer.Supervisor = sup

	httpSrv := httptest.NewServer(apiServer.Handler())
	c := New(httpSrv.URL, "test:actor")
	return c, func() { sup.Stop(context.Background(), "sleeper"); httpSrv.Close() }
}

func TestClient_ServiceLifecycle(t *testing.T) {
	c, closeFn := newTestClientWithSupervisor(t)
	defer closeFn()
	ctx := context.Background()

	if err := c.ServiceStart(ctx, "sleeper"); err != nil {
		t.Fatalf("ServiceStart: %v", err)
	}

	var st supervisor.State
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		st, err = c.ServiceStatus(ctx, "sleeper")
		if err != nil {
			t.Fatalf("ServiceStatus: %v", err)
		}
		if st.Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !st.Running {
		t.Fatal("service never reported running")
	}

	h, err := c.ServiceHealth(ctx, "sleeper")
	if err != nil {
		t.Fatalf("ServiceHealth: %v", err)
	}
	if !h.Healthy {
		t.Errorf("ServiceHealth = %+v, want healthy", h)
	}

	if err := c.ServiceRestart(ctx, "sleeper"); err != nil {
		t.Fatalf("ServiceRestart: %v", err)
	}

	all, err := c.ServicesStatus(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("ServicesStatus() = %v, %v; want exactly one entry", all, err)
	}

	// Log tailing: give the (re)started process a moment to have written
	// its "booted" line, then check it shows up.
	var logs string
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs, err = c.ServiceLogs(ctx, "sleeper", 10)
		if err == nil && logs != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("ServiceLogs: %v", err)
	}
	if logs == "" {
		t.Error("ServiceLogs returned empty output, want at least the \"booted\" line")
	}

	if err := c.ServiceStop(ctx, "sleeper"); err != nil {
		t.Fatalf("ServiceStop: %v", err)
	}
}

func TestClient_ServiceStatus_UnknownService(t *testing.T) {
	c, closeFn := newTestClientWithSupervisor(t)
	defer closeFn()

	if _, err := c.ServiceStatus(context.Background(), "does-not-exist"); err == nil {
		t.Fatal("ServiceStatus for an unknown service should error")
	}
}
