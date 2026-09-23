package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestVPNBenchmark_RanksByMockRTT(t *testing.T) {
	srv, _ := newTestAPIServer(t)
	defer srv.Close()

	body := []byte(`{"hosts":["zzz.long.example","a.io","mid.example"],"count":2}`)
	resp, err := http.Post(srv.URL+"/v1/vpn/benchmark", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var out struct {
		Best    string `json:"best"`
		Results []struct {
			Host  string   `json:"host"`
			AvgMs *float64 `json:"avg_ms"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// mock avg = len(host)+0.5 → a.io (4) < mid.example (11) < zzz.long.example (16)
	if out.Best != "a.io" {
		t.Fatalf("best=%q want a.io; results=%v", out.Best, out.Results)
	}
	if len(out.Results) != 3 || out.Results[0].Host != "a.io" {
		t.Fatalf("results=%v", out.Results)
	}
}

func TestVPNBenchmark_RequiresHosts(t *testing.T) {
	srv, _ := newTestAPIServer(t)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/v1/vpn/benchmark", "application/json", bytes.NewBufferString(`{"hosts":[]}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", resp.StatusCode)
	}
}
