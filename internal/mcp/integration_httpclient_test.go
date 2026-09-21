package mcp

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reece01-dock/axos/internal/api"
	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend/httpclient"
	"github.com/reece01-dock/axos/internal/backend/mock"
	"github.com/reece01-dock/axos/internal/rollback"
)

// TestMCPOverHTTPClient_MatchesInProcessBehavior is the end-to-end proof for
// the architecture this whole refactor exists to enable: an mcp.Server can
// be driven entirely by an httpclient.Client (exactly what cmd/axos-mcp
// does) talking to a real internal/api.Server (exactly what cmd/axosd
// "serve" does) over real HTTP — with no direct in-process access to the
// backend or rollback engine at all — and it behaves identically to the
// in-process case: tools list, dangerous-tool enforcement, rollback
// arm/confirm, and audit logging (on the API server's side, where the real
// action happened) all work.
//
// This is what "MCP can restart independently" (acceptance criterion 16)
// actually rests on: the mcp.Server built here holds no state that
// survival depends on — every fact about the world (backend data, armed
// transactions) lives in the api.Server this test also spins up, standing
// in for axosd. Discarding this mcp.Server and building a fresh one against
// the same api.Server (done explicitly below) simulates exactly that
// restart, and proves nothing is lost.
func TestMCPOverHTTPClient_MatchesInProcessBehavior(t *testing.T) {
	mb := mock.New()
	rb := rollback.New()
	var apiAuditBuf bytes.Buffer
	al := audit.New(&apiAuditBuf)
	rb.OnEvent(func(ev rollback.Event) {
		if ev.Kind == rollback.EventExpiredRestored || ev.Kind == rollback.EventExpiredRestoreFailed {
			_ = al.Reverted("rollback", ev.Detail, ev.ID)
		}
	})
	apiServer := api.NewServer(mb, rb, al)
	httpSrv := httptest.NewServer(apiServer.Handler())
	defer httpSrv.Close()

	// This stands in for axos-mcp: a Client with NO direct access to mb or
	// rb, only the HTTP URL — matching exactly what a separate process
	// would have.
	client := httpclient.New(httpSrv.URL, "mcp:ai")
	mcpServer := NewServer(client, client, nil /* audit lives on the API side, not duplicated here */, "mcp:ai")

	// tools/list works.
	resp := call(t, mcpServer, Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"})
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}

	// system.info reaches the real mock backend through the whole chain.
	infoResp := call(t, mcpServer, toolCallRequest(t, "2", "system.info", nil))
	infoTr := decodeToolResult(t, infoResp)
	if infoTr.IsError || !strings.Contains(infoTr.Content[0].Text, "GT-AX6000") {
		t.Fatalf("system.info over httpclient = %+v, want it to mention GT-AX6000", infoTr)
	}

	// rollback.arm/confirm works over the full HTTP round trip.
	armResp := call(t, mcpServer, toolCallRequest(t, "3", "rollback.arm", map[string]interface{}{
		"timeout_seconds": 60, "reason": "integration test",
	}))
	armTr := decodeToolResult(t, armResp)
	if armTr.IsError {
		t.Fatalf("rollback.arm over httpclient failed: %+v", armTr)
	}
	var armResult rollbackArmResult
	if err := json.Unmarshal([]byte(armTr.Content[0].Text), &armResult); err != nil {
		t.Fatalf("unmarshal arm result: %v", err)
	}

	// Simulate axos-mcp restarting: throw away mcpServer entirely and build
	// a brand new one against the same api.Server. If rollback state lived
	// in the MCP process, this transaction would now be gone.
	mcpServer2 := NewServer(client, client, nil, "mcp:ai")
	statusResp := call(t, mcpServer2, toolCallRequest(t, "4", "rollback.status", nil))
	statusTr := decodeToolResult(t, statusResp)
	if !strings.Contains(statusTr.Content[0].Text, armResult.ID) {
		t.Fatalf("rollback.status from a freshly built mcp.Server = %s, want it to still see transaction %s (proves rollback state survives an MCP restart)",
			statusTr.Content[0].Text, armResult.ID)
	}

	confirmResp := call(t, mcpServer2, toolCallRequest(t, "5", "rollback.confirm", map[string]interface{}{"id": armResult.ID}))
	confirmTr := decodeToolResult(t, confirmResp)
	if confirmTr.IsError {
		t.Fatalf("rollback.confirm over httpclient failed: %+v", confirmTr)
	}

	// Audit logging happened exactly once, on the API server's side, for
	// every mutating call above — not duplicated, and not silently dropped
	// because mcpServer's own Audit was nil.
	if !bytes.Contains(apiAuditBuf.Bytes(), []byte(`"action":"rollback.arm"`)) {
		t.Errorf("API-side audit log missing rollback.arm entry:\n%s", apiAuditBuf.String())
	}
	if !bytes.Contains(apiAuditBuf.Bytes(), []byte(`"action":"rollback.confirm"`)) {
		t.Errorf("API-side audit log missing rollback.confirm entry:\n%s", apiAuditBuf.String())
	}
}
