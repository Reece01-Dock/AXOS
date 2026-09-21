package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend/mock"
	"github.com/reece01-dock/axos/internal/rollback"
)

func newTestAPIServer(t *testing.T) (*httptest.Server, *bytes.Buffer) {
	t.Helper()
	mb := mock.New()
	rb := rollback.New()
	var auditBuf bytes.Buffer
	al := audit.New(&auditBuf)
	rb.OnEvent(func(ev rollback.Event) {
		if ev.Kind == rollback.EventExpiredRestored || ev.Kind == rollback.EventExpiredRestoreFailed {
			_ = al.Reverted("rollback", ev.Detail, ev.ID)
		}
	})
	s := NewServer(mb, rb, al)
	return httptest.NewServer(s.Handler()), &auditBuf
}

func TestHealthz(t *testing.T) {
	srv, _ := newTestAPIServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestInfo_ReturnsMockData(t *testing.T) {
	srv, _ := newTestAPIServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/info")
	if err != nil {
		t.Fatalf("GET /v1/info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestShellExec_IsAudited(t *testing.T) {
	srv, auditBuf := newTestAPIServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/shell_exec", bytes.NewBufferString(`{"command":"reboot"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Axos-Actor", "test:actor")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/shell_exec: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if !bytes.Contains(auditBuf.Bytes(), []byte(`"actor":"test:actor"`)) {
		t.Errorf("audit log missing actor from X-Axos-Actor header:\n%s", auditBuf.String())
	}
	if !bytes.Contains(auditBuf.Bytes(), []byte(`"action":"system.shell_exec"`)) {
		t.Errorf("audit log missing system.shell_exec entry:\n%s", auditBuf.String())
	}
}

func TestRollbackArm_MissingBody(t *testing.T) {
	srv, _ := newTestAPIServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/rollback/arm", "application/json", bytes.NewBufferString(`{"timeout_seconds":0}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("arm with timeout_seconds=0 should fail, not succeed")
	}
}

func TestRollbackArmConfirm_RoundTrip(t *testing.T) {
	srv, _ := newTestAPIServer(t)
	defer srv.Close()

	armResp, err := http.Post(srv.URL+"/v1/rollback/arm", "application/json", bytes.NewBufferString(`{"timeout_seconds":60,"reason":"test"}`))
	if err != nil {
		t.Fatalf("POST arm: %v", err)
	}
	defer armResp.Body.Close()
	if armResp.StatusCode != http.StatusOK {
		t.Fatalf("arm status = %d, want 200", armResp.StatusCode)
	}

	statusResp, err := http.Get(srv.URL + "/v1/rollback/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", statusResp.StatusCode)
	}
}

func TestRollbackArm_TwiceReturnsConflict(t *testing.T) {
	srv, _ := newTestAPIServer(t)
	defer srv.Close()

	body := `{"timeout_seconds":60,"reason":"test"}`
	resp1, err := http.Post(srv.URL+"/v1/rollback/arm", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("first arm: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first arm status = %d, want 200", resp1.StatusCode)
	}

	resp2, err := http.Post(srv.URL+"/v1/rollback/arm", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("second arm: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("second arm status = %d, want 409 Conflict", resp2.StatusCode)
	}
}
