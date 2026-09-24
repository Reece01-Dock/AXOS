package api

import (
	"bytes"
	"context"
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

	// MACs resolve to the mock clients' current IPs.
	bulk := []byte(`{"interface":"WGC5","description":"warp","sources":["11:22:33:44:55:66","11:22:33:44:55:77"]}`)
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
	if len(routes) != 2 || routes[0].Source != "192.168.1.50" || routes[1].Source != "192.168.1.51" {
		t.Fatalf("routes=%v", routes)
	}

	// An unknown MAC is refused rather than written as an unmatchable rule.
	unknown := []byte(`{"interface":"WGC5","sources":["AA:BB:CC:DD:EE:FF"]}`)
	ures, err := http.Post(ts.URL+"/v1/policy/bulk", "application/json", bytes.NewReader(unknown))
	if err != nil {
		t.Fatal(err)
	}
	ures.Body.Close()
	if ures.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown MAC status=%d, want 400", ures.StatusCode)
	}
}

func doJSON(t *testing.T, method, url, body string) (int, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestPolicyBulk_ConflictReplaceAndRemove(t *testing.T) {
	mb := mock.New()
	s := NewServer(mb, rollback.New(), audit.New(&bytes.Buffer{}))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// A rule a person made by hand in VPN Director.
	code, _ := doJSON(t, http.MethodPost, ts.URL+"/v1/policy",
		`{"source":"192.168.50.9","interface":"OVPN1","description":"mine","enabled":true}`)
	if code != http.StatusOK {
		t.Fatalf("seed status=%d", code)
	}
	// Adding a second rule for the same device without an id is refused.
	code, _ = doJSON(t, http.MethodPost, ts.URL+"/v1/policy",
		`{"source":"192.168.50.9","interface":"WGC5","enabled":true}`)
	if code != http.StatusConflict {
		t.Fatalf("duplicate single set status=%d, want 409", code)
	}

	code, body := doJSON(t, http.MethodPost, ts.URL+"/v1/policy/bulk",
		`{"interface":"wgc5","sources":["192.168.50.9","192.168.50.10"]}`)
	if code != http.StatusConflict {
		t.Fatalf("bulk without replace status=%d, want 409", code)
	}
	if c, _ := body["conflicts"].([]interface{}); len(c) != 1 {
		t.Fatalf("conflicts=%v", body["conflicts"])
	}
	routes, _ := mb.PolicyRoutes(context.Background())
	if len(routes) != 1 || routes[0].Interface != "OVPN1" {
		t.Fatalf("refused bulk must not write anything: %+v", routes)
	}

	code, body = doJSON(t, http.MethodPost, ts.URL+"/v1/policy/bulk",
		`{"interface":"wgc5","sources":["192.168.50.9","192.168.50.10"],"replace":true}`)
	if code != http.StatusOK || body["added"].(float64) != 1 || body["replaced"].(float64) != 1 {
		t.Fatalf("bulk replace status=%d body=%v", code, body)
	}
	routes, _ = mb.PolicyRoutes(context.Background())
	if len(routes) != 2 || routes[0].Interface != "WGC5" || routes[1].Interface != "WGC5" {
		t.Fatalf("routes=%+v", routes)
	}

	code, body = doJSON(t, http.MethodPost, ts.URL+"/v1/policy/bulk/remove", `{"sources":["192.168.50.9"]}`)
	if code != http.StatusOK || body["removed"].(float64) != 1 {
		t.Fatalf("remove status=%d body=%v", code, body)
	}
	routes, _ = mb.PolicyRoutes(context.Background())
	if len(routes) != 1 || routes[0].Source != "192.168.50.10" || routes[0].ID != "1" {
		t.Fatalf("after remove routes=%+v", routes)
	}
}

func TestClientGroups_ValidationAndApply(t *testing.T) {
	mb := mock.New()
	s := NewServer(mb, rollback.New(), audit.New(&bytes.Buffer{}))
	s.DataDir = t.TempDir()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	for _, bad := range []string{
		`{"groups":[{"id":"a","members":["not-a-mac"]}]}`,
		`{"groups":[{"id":"a"},{"id":"a"}]}`,
		`{"groups":[{"id":"Bad Id"}]}`,
		`{"groups":[{"id":""}]}`,
	} {
		if code, _ := doJSON(t, http.MethodPut, ts.URL+"/v1/vpn/client-groups", bad); code != http.StatusBadRequest {
			t.Errorf("PUT %s status=%d, want 400", bad, code)
		}
	}

	code, body := doJSON(t, http.MethodPut, ts.URL+"/v1/vpn/client-groups",
		`{"groups":[{"id":"warp","name":"Cloudflare","interface":"wgc5","members":["11-22-33-44-55-77","11:22:33:44:55:77","11:22:33:44:55:66"]}]}`)
	if code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%v", code, body)
	}
	g := body["groups"].([]interface{})[0].(map[string]interface{})
	if m := g["members"].([]interface{}); len(m) != 2 || m[0] != "11:22:33:44:55:77" || g["interface"] != "WGC5" {
		t.Fatalf("group not normalised: %v", g)
	}

	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/v1/vpn/client-groups/nope/apply", `{}`); code != http.StatusNotFound {
		t.Fatalf("apply unknown group status=%d", code)
	}
	code, body = doJSON(t, http.MethodPost, ts.URL+"/v1/vpn/client-groups/warp/apply", `{}`)
	if code != http.StatusOK || body["added"].(float64) != 2 {
		t.Fatalf("apply status=%d body=%v", code, body)
	}
	routes, _ := mb.PolicyRoutes(context.Background())
	if len(routes) != 2 || routes[0].Interface != "WGC5" || routes[0].Description != "Cloudflare" {
		t.Fatalf("routes=%+v", routes)
	}
}
