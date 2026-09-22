package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend/mock"
	"github.com/reece01-dock/axos/internal/footprint"
	"github.com/reece01-dock/axos/internal/rollback"
)

func TestFootprintRoutes_501WhenUnconfigured(t *testing.T) {
	srv, _ := newTestAPIServer(t) // no Footprint set
	defer srv.Close()

	for _, req := range []struct {
		method, path string
	}{
		{http.MethodGet, "/v1/footprint"},
		{http.MethodGet, "/v1/footprint/history"},
		{http.MethodPost, "/v1/footprint/snapshot"},
	} {
		httpReq, err := http.NewRequest(req.method, srv.URL+req.path, nil)
		if err != nil {
			t.Fatalf("building request: %v", err)
		}
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatalf("%s %s: %v", req.method, req.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s %s status = %d, want 501", req.method, req.path, resp.StatusCode)
		}
	}
}

// newTestAPIServerWithFootprint mirrors newTestAPIServerWithSupervisor in
// supervisor_routes_test.go: build the *Server directly so an optional
// field can be set before wrapping it in an httptest.Server.
func newTestAPIServerWithFootprint(t *testing.T) *httptest.Server {
	t.Helper()
	mb := mock.New()
	rb := rollback.New()
	al := audit.New(io.Discard)
	apiServer := NewServer(mb, rb, al)

	fp, err := footprint.Open(filepath.Join(t.TempDir(), "footprint.jsonl"))
	if err != nil {
		t.Fatalf("footprint.Open: %v", err)
	}
	t.Cleanup(func() { fp.Close() })
	apiServer.Footprint = fp

	return httptest.NewServer(apiServer.Handler())
}

func TestFootprintRoutes_CurrentWithoutBaseline(t *testing.T) {
	srv := newTestAPIServerWithFootprint(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/footprint")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got footprintResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.Current.Timestamp.IsZero() {
		t.Error("Current.Timestamp should be set")
	}
	if got.Baseline != nil {
		t.Error("Baseline should be nil when no snapshot has ever been recorded")
	}
}

func TestFootprintRoutes_SnapshotThenHistoryAndBaseline(t *testing.T) {
	srv := newTestAPIServerWithFootprint(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/footprint/snapshot", "application/json", nil)
	if err != nil {
		t.Fatalf("POST snapshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("snapshot status = %d, want 200", resp.StatusCode)
	}
	var snap footprint.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decoding snapshot response: %v", err)
	}
	if snap.Label != "manual" {
		t.Errorf("snapshot Label = %q, want manual (default)", snap.Label)
	}

	histResp, err := http.Get(srv.URL + "/v1/footprint/history")
	if err != nil {
		t.Fatalf("GET history: %v", err)
	}
	defer histResp.Body.Close()
	var hist []footprint.Snapshot
	if err := json.NewDecoder(histResp.Body).Decode(&hist); err != nil {
		t.Fatalf("decoding history response: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history has %d entries, want 1", len(hist))
	}

	// A second read of "current" should now be able to report the one
	// snapshot recorded above as the baseline.
	curResp, err := http.Get(srv.URL + "/v1/footprint")
	if err != nil {
		t.Fatalf("GET current: %v", err)
	}
	defer curResp.Body.Close()
	var cur footprintResponse
	if err := json.NewDecoder(curResp.Body).Decode(&cur); err != nil {
		t.Fatalf("decoding current response: %v", err)
	}
	if cur.Baseline == nil {
		t.Fatal("Baseline should be set once a snapshot exists")
	}
}

func TestFootprintRoutes_SnapshotHonorsGivenLabel(t *testing.T) {
	srv := newTestAPIServerWithFootprint(t)
	defer srv.Close()

	body, err := json.Marshal(map[string]string{"label": "post-vpn-feature", "release_id": "000007"})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	resp, err := http.Post(srv.URL+"/v1/footprint/snapshot", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST snapshot: %v", err)
	}
	defer resp.Body.Close()
	var snap footprint.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decoding snapshot response: %v", err)
	}
	if snap.Label != "post-vpn-feature" {
		t.Errorf("Label = %q, want post-vpn-feature", snap.Label)
	}
	if snap.ReleaseID != "000007" {
		t.Errorf("ReleaseID = %q, want 000007", snap.ReleaseID)
	}
}
