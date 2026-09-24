package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

const uiTokenHeader = "X-Axos-UI-Token"

// LANAuth wraps a handler so that:
//   - loopback clients (MCP, SSH tunnels, axosctl on-router) pass through
//   - non-loopback clients must present X-Axos-UI-Token matching the token
//     file (Merlin UI embed). When no token file is configured/readable,
//     non-loopback requests are rejected (fail closed).
//
// Also attaches CORS headers so the Merlin httpd origin (port 80/443) can
// call the Core API on :9090 from browser JavaScript.
type LANAuth struct {
	Inner     http.Handler
	TokenFile string

	mu sync.Mutex
}

func (a *LANAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if originOK(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Axos-Actor, "+uiTokenHeader)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Vary", "Origin")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if !isLoopbackRequest(r) {
		tok, ok := a.loadToken()
		if !ok || tok == "" {
			http.Error(w, `{"error":"lan api disabled (no ui token)"}`, http.StatusForbidden)
			return
		}
		got := r.Header.Get(uiTokenHeader)
		if subtle.ConstantTimeCompare([]byte(got), []byte(tok)) != 1 {
			http.Error(w, `{"error":"missing or invalid UI token"}`, http.StatusUnauthorized)
			return
		}
	}

	a.Inner.ServeHTTP(w, r)
}

func (a *LANAuth) loadToken() (string, bool) {
	if a.TokenFile == "" {
		return "", false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	data, err := os.ReadFile(a.TokenFile)
	if err != nil {
		return "", false
	}
	tok := strings.TrimSpace(string(data))
	return tok, tok != ""
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func originOK(origin string) bool {
	if origin == "" {
		return false
	}
	origin = strings.TrimSpace(origin)
	if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
		return false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	host, _, err := net.SplitHostPort(rest)
	if err != nil {
		host = rest
	}
	host = strings.Trim(host, "[]")
	hostLower := strings.ToLower(host)

	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}

	// Merlin admin hostnames (www.asusrouter.com, router.asus.com, …)
	if hostLower == "asusrouter.com" ||
		strings.HasSuffix(hostLower, ".asusrouter.com") ||
		hostLower == "router.asus.com" ||
		strings.HasSuffix(hostLower, ".asus.com") {
		return true
	}
	return false
}
