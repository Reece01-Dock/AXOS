package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/reece01-dock/axos/internal/backend"
)

// ClientGroup is a named set of device MACs for VPN Director steering.
type ClientGroup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Members     []string `json:"members"` // MACs
	Interface   string   `json:"interface,omitempty"` // default tunnel e.g. WGC5
	Description string   `json:"description,omitempty"`
}

type clientGroupsFile struct {
	Groups []ClientGroup `json:"groups"`
}

var clientGroupsMu sync.Mutex

func (s *Server) clientGroupsPath() string {
	if s.DataDir == "" {
		return ""
	}
	return filepath.Join(s.DataDir, "run", "client-groups.json")
}

func (s *Server) handleGetClientGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.loadClientGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"groups": groups})
}

func (s *Server) handlePutClientGroups(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Groups []ClientGroup `json:"groups"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Groups == nil {
		body.Groups = []ClientGroup{}
	}
	for i := range body.Groups {
		if body.Groups[i].ID == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("group id is required"))
			return
		}
		if body.Groups[i].Name == "" {
			body.Groups[i].Name = body.Groups[i].ID
		}
		if body.Groups[i].Members == nil {
			body.Groups[i].Members = []string{}
		}
	}
	if err := s.saveClientGroups(body.Groups); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.audit(s.actor(r), "vpn.client_groups.set", map[string]interface{}{
		"count": len(body.Groups),
	}, "", nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"groups": body.Groups})
}

func (s *Server) loadClientGroups() ([]ClientGroup, error) {
	path := s.clientGroupsPath()
	if path == "" {
		return []ClientGroup{}, nil
	}
	clientGroupsMu.Lock()
	defer clientGroupsMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ClientGroup{}, nil
		}
		return nil, err
	}
	var f clientGroupsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Groups == nil {
		return []ClientGroup{}, nil
	}
	return f.Groups, nil
}

func (s *Server) saveClientGroups(groups []ClientGroup) error {
	path := s.clientGroupsPath()
	if path == "" {
		return fmt.Errorf("client groups require DataDir (set -backup-dir so parent /jffs/axos is known)")
	}
	clientGroupsMu.Lock()
	defer clientGroupsMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(clientGroupsFile{Groups: groups}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type policyBulkRequest struct {
	Interface   string   `json:"interface"` // WAN, WGC5, ...
	Description string   `json:"description"`
	Sources     []string `json:"sources"` // MAC or IP per client
	Enabled     *bool    `json:"enabled,omitempty"`
}

func (s *Server) handlePolicyBulk(w http.ResponseWriter, r *http.Request) {
	var req policyBulkRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Interface == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("interface is required"))
		return
	}
	sources := make([]string, 0, len(req.Sources))
	seen := map[string]bool{}
	for _, src := range req.Sources {
		src = stringsTrim(src)
		if src == "" || seen[stringsToLower(src)] {
			continue
		}
		seen[stringsToLower(src)] = true
		sources = append(sources, src)
	}
	if len(sources) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("sources is required"))
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	desc := req.Description
	if desc == "" {
		desc = "axos"
	}
	applied := 0
	for _, src := range sources {
		route := backend.PolicyRoute{
			Source:      src,
			Interface:   req.Interface,
			Description: desc,
			Enabled:     enabled,
		}
		err := s.Backend.SetPolicyRoute(r.Context(), route)
		s.audit(s.actor(r), "route.policy.set", map[string]interface{}{
			"source": src, "interface": req.Interface, "bulk": true,
		}, "", err)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		applied++
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"applied": applied,
	})
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func stringsToLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
