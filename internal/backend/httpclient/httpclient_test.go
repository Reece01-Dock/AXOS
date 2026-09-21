package httpclient

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/reece01-dock/axos/internal/api"
	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend/mock"
	"github.com/reece01-dock/axos/internal/rollback"
)

// newTestClient starts a real internal/api.Server backed by mock.Backend,
// wraps it in an httptest.Server, and returns a Client pointed at it. This
// is the whole point of httpclient: nothing here is faked or mocked at the
// HTTP layer — real JSON goes over a real (loopback) HTTP connection.
func newTestClient(t *testing.T) (*Client, *bytes.Buffer, func()) {
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
	apiServer := api.NewServer(mb, rb, al)
	httpSrv := httptest.NewServer(apiServer.Handler())

	c := New(httpSrv.URL, "test:actor")
	return c, &auditBuf, httpSrv.Close
}

func TestClient_Info(t *testing.T) {
	c, _, closeFn := newTestClient(t)
	defer closeFn()

	info, err := c.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Model != "GT-AX6000" {
		t.Errorf("Info().Model = %q, want GT-AX6000", info.Model)
	}
}

func TestClient_AllReadMethods(t *testing.T) {
	c, _, closeFn := newTestClient(t)
	defer closeFn()
	ctx := context.Background()

	if _, err := c.Resources(ctx); err != nil {
		t.Errorf("Resources: %v", err)
	}
	if _, err := c.Interfaces(ctx); err != nil {
		t.Errorf("Interfaces: %v", err)
	}
	if _, err := c.Routes(ctx, ""); err != nil {
		t.Errorf("Routes: %v", err)
	}
	if _, err := c.Clients(ctx); err != nil {
		t.Errorf("Clients: %v", err)
	}
	if _, err := c.WiFiStatus(ctx); err != nil {
		t.Errorf("WiFiStatus: %v", err)
	}
	if _, err := c.Services(ctx); err != nil {
		t.Errorf("Services: %v", err)
	}
	if _, err := c.FirewallRules(ctx); err != nil {
		t.Errorf("FirewallRules: %v", err)
	}
	if _, err := c.VPNStatus(ctx); err != nil {
		t.Errorf("VPNStatus: %v", err)
	}
	if _, err := c.NVRAMDump(ctx); err != nil {
		t.Errorf("NVRAMDump: %v", err)
	}
	if _, err := c.ListBackups(ctx); err != nil {
		t.Errorf("ListBackups: %v", err)
	}
}

func TestClient_ShellExecAndBackupRestore(t *testing.T) {
	c, auditBuf, closeFn := newTestClient(t)
	defer closeFn()
	ctx := context.Background()

	res, err := c.ShellExec(ctx, "echo hi", 5)
	if err != nil {
		t.Fatalf("ShellExec: %v", err)
	}
	if res.Command != "echo hi" {
		t.Errorf("ShellExec result.Command = %q, want %q", res.Command, "echo hi")
	}

	info, err := c.Backup(ctx, "test")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if info.ID == "" {
		t.Fatal("Backup did not return an id")
	}

	if err := c.Restore(ctx, info.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if !bytes.Contains(auditBuf.Bytes(), []byte(`"actor":"test:actor"`)) {
		t.Error("audit log should attribute actions to the client's configured Actor")
	}
}

// TestClient_RollbackSurvivesAcrossIndependentClients is the core proof for
// acceptance criterion 16 ("MCP can restart independently"): the rollback
// engine lives in the api.Server (standing in for axosd), not in the
// Client. Arming via one Client instance and checking status via a brand
// new Client instance (simulating axos-mcp being restarted) must see the
// same in-flight transaction, because neither Client holds any state of
// its own — the server does.
func TestClient_RollbackSurvivesAcrossIndependentClients(t *testing.T) {
	c1, _, closeFn := newTestClient(t)
	defer closeFn()

	id, deadline, err := c1.Arm(context.Background(), 60, "simulated restart test")
	if err != nil {
		t.Fatalf("Arm: %v", err)
	}
	if id == "" || deadline.IsZero() {
		t.Fatalf("Arm returned id=%q deadline=%v, want both set", id, deadline)
	}

	// A fresh Client, same URL — stands in for axos-mcp restarting: a new
	// process, zero local state, pointed at the same axosd.
	c2 := New(c1.BaseURL, "test:actor-after-restart")
	st, err := c2.Status(context.Background())
	if err != nil {
		t.Fatalf("Status from second client: %v", err)
	}
	if !st.Pending || st.ID != id {
		t.Fatalf("Status from second client = %+v, want the transaction armed by the first client (id=%s) still pending", st, id)
	}

	if err := c2.Confirm(context.Background(), id); err != nil {
		t.Fatalf("Confirm from second client: %v", err)
	}
}

func TestClient_ErrorsIsMapsRollbackSentinels(t *testing.T) {
	c, _, closeFn := newTestClient(t)
	defer closeFn()
	ctx := context.Background()

	if _, _, err := c.Arm(ctx, 60, "first"); err != nil {
		t.Fatalf("first Arm: %v", err)
	}
	_, _, err := c.Arm(ctx, 60, "second")
	if !errors.Is(err, rollback.ErrBusy) {
		t.Errorf("second Arm error = %v, want errors.Is(err, rollback.ErrBusy) to be true across the HTTP boundary", err)
	}

	err = c.Confirm(ctx, "does-not-exist")
	if !errors.Is(err, rollback.ErrNotFound) {
		t.Errorf("Confirm(unknown) error = %v, want errors.Is(err, rollback.ErrNotFound) to be true across the HTTP boundary", err)
	}
}

func TestClient_ExpiredTransactionAutoRestoresOnServerSide(t *testing.T) {
	c, _, closeFn := newTestClient(t)
	defer closeFn()
	ctx := context.Background()

	if _, _, err := c.Arm(ctx, 1, "will expire"); err != nil {
		t.Fatalf("Arm: %v", err)
	}

	time.Sleep(2 * time.Second)

	st, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Pending {
		t.Fatalf("Status after expiry = %+v, want not pending (server should have auto-restored)", st)
	}
}
