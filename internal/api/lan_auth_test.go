package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLANAuth_LoopbackNoToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h := &LANAuth{Inner: inner, TokenFile: "/nonexistent"}
	req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("loopback status = %d, want 200", rr.Code)
	}
}

func TestLANAuth_LANRequiresToken(t *testing.T) {
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "ui.token")
	if err := os.WriteFile(tokPath, []byte("secret-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := &LANAuth{Inner: inner, TokenFile: tokPath}

	req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	req.RemoteAddr = "192.168.50.10:9999"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no token status = %d, want 401", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	req2.RemoteAddr = "192.168.50.10:9999"
	req2.Header.Set(uiTokenHeader, "secret-token")
	req2.Header.Set("Origin", "http://192.168.50.1")
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("with token status = %d, want 200", rr2.Code)
	}
	if rr2.Header().Get("Access-Control-Allow-Origin") != "http://192.168.50.1" {
		t.Fatalf("missing CORS origin, got %q", rr2.Header().Get("Access-Control-Allow-Origin"))
	}
}
