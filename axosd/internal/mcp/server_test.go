package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/reece01-dock/axos/axosd/internal/audit"
	mockbackend "github.com/reece01-dock/axos/axosd/internal/backend/mock"
	"github.com/reece01-dock/axos/axosd/internal/rollback"
)

func newTestServer(t *testing.T) (*Server, *bytes.Buffer) {
	t.Helper()
	b := mockbackend.New()
	rb := rollback.New()
	var auditBuf bytes.Buffer
	al := audit.New(&auditBuf)
	rb.OnEvent(func(ev rollback.Event) {
		if ev.Kind == rollback.EventExpiredRestored || ev.Kind == rollback.EventExpiredRestoreFailed {
			_ = al.Reverted("rollback", ev.Detail, ev.ID)
		}
	})
	s := NewServer(b, rb, al, "test:actor")
	return s, &auditBuf
}

// call sends one JSON-RPC request line through Serve and returns the parsed response.
func call(t *testing.T, s *Server, req Request) Response {
	t.Helper()
	reqBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var out bytes.Buffer
	if err := s.Serve(context.Background(), bytes.NewReader(append(reqBytes, '\n')), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", out.String(), err)
	}
	return resp
}

func toolCallRequest(t *testing.T, id string, name string, args interface{}) Request {
	t.Helper()
	var rawArgs json.RawMessage
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			t.Fatalf("marshal args: %v", err)
		}
		rawArgs = b
	}
	params, err := json.Marshal(ToolCallParams{Name: name, Arguments: rawArgs})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return Request{JSONRPC: "2.0", ID: json.RawMessage(`"` + id + `"`), Method: "tools/call", Params: params}
}

func decodeToolResult(t *testing.T, resp Response) ToolCallResult {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %+v", resp.Error)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("re-marshal result: %v", err)
	}
	var tr ToolCallResult
	if err := json.Unmarshal(b, &tr); err != nil {
		t.Fatalf("unmarshal ToolCallResult: %v", err)
	}
	return tr
}

func TestToolsList_IncludesCoreTools(t *testing.T) {
	s, _ := newTestServer(t)
	resp := call(t, s, Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	b, _ := json.Marshal(resp.Result)
	var lr ToolsListResult
	if err := json.Unmarshal(b, &lr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := []string{"system.info", "system.shell_exec", "network.interfaces", "wifi.status", "rollback.arm", "rollback.confirm", "config.backup"}
	got := map[string]bool{}
	for _, tool := range lr.Tools {
		got[tool.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("tools/list missing %q", name)
		}
	}
}

func TestToolsCall_SystemInfo(t *testing.T) {
	s, _ := newTestServer(t)
	resp := call(t, s, toolCallRequest(t, "1", "system.info", nil))
	tr := decodeToolResult(t, resp)
	if tr.IsError {
		t.Fatalf("system.info returned error: %+v", tr)
	}
	if len(tr.Content) != 1 || !strings.Contains(tr.Content[0].Text, "GT-AX6000") {
		t.Fatalf("system.info content = %+v, want it to mention GT-AX6000", tr.Content)
	}
}

func TestToolsCall_UnknownTool(t *testing.T) {
	s, _ := newTestServer(t)
	resp := call(t, s, toolCallRequest(t, "1", "nonexistent.tool", nil))
	if resp.Error == nil || resp.Error.Code != ErrMethodNotFound {
		t.Fatalf("resp.Error = %+v, want ErrMethodNotFound", resp.Error)
	}
}

func TestRollback_ArmConfirmFlow(t *testing.T) {
	s, auditBuf := newTestServer(t)

	armResp := call(t, s, toolCallRequest(t, "1", "rollback.arm", map[string]interface{}{
		"timeout_seconds": 60,
		"reason":          "test change",
	}))
	tr := decodeToolResult(t, armResp)
	if tr.IsError {
		t.Fatalf("rollback.arm error: %+v", tr)
	}
	var armResult rollbackArmResult
	if err := json.Unmarshal([]byte(tr.Content[0].Text), &armResult); err != nil {
		t.Fatalf("unmarshal arm result: %v", err)
	}
	if armResult.ID == "" {
		t.Fatal("rollback.arm did not return an id")
	}

	statusResp := call(t, s, toolCallRequest(t, "2", "rollback.status", nil))
	statusTr := decodeToolResult(t, statusResp)
	if !strings.Contains(statusTr.Content[0].Text, armResult.ID) {
		t.Fatalf("rollback.status = %s, want it to mention %s", statusTr.Content[0].Text, armResult.ID)
	}

	confirmResp := call(t, s, toolCallRequest(t, "3", "rollback.confirm", map[string]interface{}{"id": armResult.ID}))
	confirmTr := decodeToolResult(t, confirmResp)
	if confirmTr.IsError {
		t.Fatalf("rollback.confirm error: %+v", confirmTr)
	}

	statusResp2 := call(t, s, toolCallRequest(t, "4", "rollback.status", nil))
	statusTr2 := decodeToolResult(t, statusResp2)
	if strings.Contains(statusTr2.Content[0].Text, `"pending":true`) {
		t.Fatalf("rollback.status after confirm = %s, want pending:false", statusTr2.Content[0].Text)
	}

	// Every call above should have produced an audit line.
	lines := countLines(auditBuf.String())
	if lines < 4 {
		t.Fatalf("audit log has %d lines, want at least 4", lines)
	}
}

func TestRollback_ArmTwiceRejected(t *testing.T) {
	s, _ := newTestServer(t)

	resp1 := call(t, s, toolCallRequest(t, "1", "rollback.arm", map[string]interface{}{"timeout_seconds": 60, "reason": "first"}))
	tr1 := decodeToolResult(t, resp1)
	if tr1.IsError {
		t.Fatalf("first arm failed: %+v", tr1)
	}

	resp2 := call(t, s, toolCallRequest(t, "2", "rollback.arm", map[string]interface{}{"timeout_seconds": 60, "reason": "second"}))
	tr2 := decodeToolResult(t, resp2)
	if !tr2.IsError {
		t.Fatalf("second concurrent arm should have failed, got: %+v", tr2)
	}
	if !strings.Contains(tr2.Content[0].Text, "rollback_busy") {
		t.Fatalf("second arm error = %q, want rollback_busy", tr2.Content[0].Text)
	}
}

func TestRollback_ExpiryRestoresViaBackend(t *testing.T) {
	s, _ := newTestServer(t)

	resp := call(t, s, toolCallRequest(t, "1", "rollback.arm", map[string]interface{}{"timeout_seconds": 1, "reason": "will expire"}))
	tr := decodeToolResult(t, resp)
	if tr.IsError {
		t.Fatalf("arm failed: %+v", tr)
	}

	// Give the rollback engine's timer time to fire and call Backend.Restore.
	time.Sleep(2 * time.Second)

	mb := s.Backend.(*mockbackend.Backend)
	restored := mb.RestoredIDs()
	if len(restored) != 1 {
		t.Fatalf("RestoredIDs = %v, want exactly one automatic restore", restored)
	}

	st := s.Rollback.Status()
	if st.Pending {
		t.Fatalf("rollback status still pending after expiry: %+v", st)
	}
}

func TestConfigBackupAndRestore(t *testing.T) {
	s, _ := newTestServer(t)

	backupResp := call(t, s, toolCallRequest(t, "1", "config.backup", map[string]interface{}{"reason": "manual test"}))
	tr := decodeToolResult(t, backupResp)
	if tr.IsError {
		t.Fatalf("config.backup error: %+v", tr)
	}
	var info struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(tr.Content[0].Text), &info); err != nil {
		t.Fatalf("unmarshal backup info: %v", err)
	}

	restoreResp := call(t, s, toolCallRequest(t, "2", "config.restore", map[string]interface{}{"backup_id": info.ID}))
	rtr := decodeToolResult(t, restoreResp)
	if rtr.IsError {
		t.Fatalf("config.restore error: %+v", rtr)
	}

	badResp := call(t, s, toolCallRequest(t, "3", "config.restore", map[string]interface{}{"backup_id": "nope"}))
	btr := decodeToolResult(t, badResp)
	if !btr.IsError {
		t.Fatal("config.restore with unknown id should have errored")
	}
}

func TestShellExec_RequiresCommand(t *testing.T) {
	s, _ := newTestServer(t)
	resp := call(t, s, toolCallRequest(t, "1", "system.shell_exec", map[string]interface{}{}))
	tr := decodeToolResult(t, resp)
	if !tr.IsError {
		t.Fatal("system.shell_exec with no command should error")
	}
}

func TestShellExec_AuditsCommand(t *testing.T) {
	s, auditBuf := newTestServer(t)
	resp := call(t, s, toolCallRequest(t, "1", "system.shell_exec", map[string]interface{}{"command": "reboot"}))
	tr := decodeToolResult(t, resp)
	if tr.IsError {
		t.Fatalf("system.shell_exec error: %+v", tr)
	}

	found := false
	sc := bufio.NewScanner(strings.NewReader(auditBuf.String()))
	for sc.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("bad audit line: %v", err)
		}
		if e.Action == "system.shell_exec" && e.Args["command"] == "reboot" {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit log missing system.shell_exec(reboot) entry:\n%s", auditBuf.String())
	}
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(strings.TrimRight(s, "\n"), "\n"))
}
