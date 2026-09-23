package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/reece01-dock/axos/internal/audit"
	"github.com/reece01-dock/axos/internal/backend/mock"
	"github.com/reece01-dock/axos/internal/rollback"
)

func TestClientGroupsAndPolicyBulk(t *testing.T) {
	mb := mock.New()
	rb := rollback.New()
	var auditBuf bytes.Buffer
	al := audit.New(&auditBuf)
	s := NewServer(mb, rb, al)
	s.DataDir = t.TempDir()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	body := []byte(`{"groups":[{"id":"warp","name":"Cloudflare","members":["AA:BB:CC:DD:EE:FF"],"interface":"WGC5"}]}`)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/vpn/client-groups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put groups status=%d", resp.StatusCode)
	}
	path := filepath.Join(s.DataDir, "run", "client-groups.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("groups file: %v", err)
	}

	get, err := http.Get(ts.URL + "/v1/vpn/client-groups")
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var got struct {
		Groups []ClientGroup `json:"groups"`
	}
	if err := json.NewDecoder(get.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || got.Groups[0].ID != "warp" {
		t.Fatalf("groups=%v", got.Groups)
	}

	bulk := []byte(`{"interface":"WGC5","description":"warp","sources":["AA:BB:CC:DD:EE:FF","11:22:33:44:55:66"]}`)
	bres, err := http.Post(ts.URL+"/v1/policy/bulk", "application/json", bytes.NewReader(bulk))
	if err != nil {
		t.Fatal(err)
	}
	defer bres.Body.Close()
	if bres.StatusCode != http.StatusOK {
		t.Fatalf("bulk status=%d", bres.StatusCode)
	}
	routes, err := mb.PolicyRoutes(bres.Request.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) < 2 {
		t.Fatalf("routes=%v", routes)
	}
}
