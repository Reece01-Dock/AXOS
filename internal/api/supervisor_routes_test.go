package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend/mock"
	"github.com/reece01-dock/axos/internal/rollback"
	"github.com/reece01-dock/axos/internal/supervisor"
)

func TestServiceRoutes_501WhenSupervisorUnconfigured(t *testing.T) {
	srv, _ := newTestAPIServer(t) // no Supervisor set
	defer srv.Close()

	for _, path := range []string{"/v1/supervisor/services", "/v1/supervisor/services/foo/status", "/v1/supervisor/services/foo/health", "/v1/supervisor/services/foo/logs"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("GET %s status = %d, want 501", path, resp.StatusCode)
		}
	}
}

func newTestAPIServerWithSupervisor(t *testing.T) (*httptest.Server, *supervisor.Supervisor) {
	t.Helper()
	mb := mock.New()
	rb := rollback.New()
	al := audit.New(io.Discard) // no assertions on audit content in these tests
	apiServer := NewServer(mb, rb, al)

	sup := supervisor.New("")
	sup.Register(supervisor.ServiceSpec{
		Name: "sleeper", Command: "sh", Args: []string{"-c", "sleep 5"},
		RestartBackoffBase: 20 * time.Millisecond, RestartBackoffMax: 50 * time.Millisecond,
	})
	apiServer.Supervisor = sup

	return httptest.NewServer(apiServer.Handler()), sup
}

func TestServiceRoutes_StartStatusStopRestart(t *testing.T) {
	srv, sup := newTestAPIServerWithSupervisor(t)
	defer srv.Close()
	defer sup.Stop(context.Background(), "sleeper")

	resp, err := http.Post(srv.URL+"/v1/supervisor/services/sleeper/start", "application/json", nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start status = %d, want 200", resp.StatusCode)
	}

	waitForHTTPCondition(t, srv.URL+"/v1/supervisor/services/sleeper/status", func(body []byte) bool {
		return contains(body, `"running":true`)
	})

	resp, err = http.Post(srv.URL+"/v1/supervisor/services/sleeper/restart", "application/json", nil)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restart status = %d, want 200", resp.StatusCode)
	}

	resp, err = http.Post(srv.URL+"/v1/supervisor/services/sleeper/stop", "application/json", nil)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stop status = %d, want 200", resp.StatusCode)
	}
}

func TestServiceRoutes_UnknownServiceIs404(t *testing.T) {
	srv, _ := newTestAPIServerWithSupervisor(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/supervisor/services/does-not-exist/status")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServiceRoutes_All(t *testing.T) {
	srv, _ := newTestAPIServerWithSupervisor(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/supervisor/services")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func waitForHTTPCondition(t *testing.T, url string, cond func(body []byte) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			buf := make([]byte, 4096)
			n, _ := resp.Body.Read(buf)
			resp.Body.Close()
			if cond(buf[:n]) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for condition at %s", url)
}

func contains(haystack []byte, needle string) bool {
	return strings.Contains(string(haystack), needle)
}
