package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

	for _, origin := range []string{"http://192.168.50.1", "http://www.asusrouter.com"} {
		req2 := httptest.NewRequest(http.MethodOptions, "/v1/info", nil)
		req2.RemoteAddr = "192.168.50.10:9999"
		req2.Header.Set("Origin", origin)
		rr2 := httptest.NewRecorder()
		h.ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusNoContent {
			t.Fatalf("OPTIONS %s status = %d", origin, rr2.Code)
		}
		if rr2.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatalf("OPTIONS %s missing CORS, got %q", origin, rr2.Header().Get("Access-Control-Allow-Origin"))
		}
		// The Merlin UI saves client groups with PUT.
		if m := rr2.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(m, "PUT") {
			t.Fatalf("OPTIONS %s Allow-Methods = %q, want PUT", origin, m)
		}

		req3 := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
		req3.RemoteAddr = "192.168.50.10:9999"
		req3.Header.Set(uiTokenHeader, "secret-token")
		req3.Header.Set("Origin", origin)
		rr3 := httptest.NewRecorder()
		h.ServeHTTP(rr3, req3)
		if rr3.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", origin, rr3.Code)
		}
	}
}

func TestLANAuth_StaticUIWithoutToken(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "ui.token")
	if err := os.WriteFile(tokenFile, []byte("secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &LANAuth{Inner: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), TokenFile: tokenFile}
	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/", http.StatusOK},
		{http.MethodGet, "/ui/axos-ui.js", http.StatusOK},
		{http.MethodPost, "/ui/axos-ui.js", http.StatusUnauthorized},
		{http.MethodGet, "/v1/info", http.StatusUnauthorized},
		{http.MethodGet, "/v1/nvram", http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		req.RemoteAddr = "192.168.50.10:9999"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != c.want {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, rr.Code, c.want)
		}
	}
}
