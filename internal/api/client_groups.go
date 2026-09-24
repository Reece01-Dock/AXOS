package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/reece01-dock/axos/internal/backend"
)

// ClientGroup is a named set of device MACs for VPN Director steering.
type ClientGroup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Members     []string `json:"members"`             // MACs, AA:BB:CC:DD:EE:FF
	Interface   string   `json:"interface,omitempty"` // default tunnel e.g. WGC5
	Description string   `json:"description,omitempty"`
}

type clientGroupsFile struct {
	Groups []ClientGroup `json:"groups"`
}

const (
	maxClientGroups       = 64
	maxClientGroupMembers = 256
	maxClientGroupName    = 32
)

var (
	clientGroupsMu sync.Mutex
	clientGroupID  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

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
	groups, err := normalizeClientGroups(body.Groups)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.saveClientGroups(groups); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.audit(s.actor(r), "vpn.client_groups.set", map[string]interface{}{
		"count": len(groups),
	}, "", nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"groups": groups})
}

// normalizeClientGroups validates a full group list: ids are unique slugs,
// names are short, members are well-formed MACs (canonicalised to
// upper-case colon form and de-duplicated), interfaces use Director tokens.
func normalizeClientGroups(in []ClientGroup) ([]ClientGroup, error) {
	if len(in) > maxClientGroups {
		return nil, fmt.Errorf("at most %d groups are allowed", maxClientGroups)
	}
	out := make([]ClientGroup, 0, len(in))
	ids := make(map[string]bool, len(in))
	for i, g := range in {
		g.ID = strings.TrimSpace(g.ID)
		if !clientGroupID.MatchString(g.ID) {
			return nil, fmt.Errorf("group %d: id %q must be 1-32 chars of a-z, 0-9, '-' or '_'", i+1, g.ID)
		}
		if ids[g.ID] {
			return nil, fmt.Errorf("group %d: duplicate id %q", i+1, g.ID)
		}
		ids[g.ID] = true
		g.Name = strings.TrimSpace(g.Name)
		if g.Name == "" {
			g.Name = g.ID
		}
		if len(g.Name) > maxClientGroupName {
			return nil, fmt.Errorf("group %q: name is longer than %d characters", g.ID, maxClientGroupName)
		}
		if len(g.Members) > maxClientGroupMembers {
			return nil, fmt.Errorf("group %q: at most %d members are allowed", g.ID, maxClientGroupMembers)
		}
		members := make([]string, 0, len(g.Members))
		seen := make(map[string]bool, len(g.Members))
		for _, m := range g.Members {
			mac, err := canonicalMAC(m)
			if err != nil {
				return nil, fmt.Errorf("group %q: %w", g.ID, err)
			}
			if !seen[mac] {
				seen[mac] = true
				members = append(members, mac)
			}
		}
		g.Members = members
		g.Interface = backend.NormalizeDirectorIface(g.Interface)
		g.Description = strings.TrimSpace(g.Description)
		out = append(out, g)
	}
	return out, nil
}

func canonicalMAC(s string) (string, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(s))
	if err != nil || len(hw) != 6 {
		return "", fmt.Errorf("invalid MAC address %q", s)
	}
	return strings.ToUpper(hw.String()), nil
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

type clientGroupApplyRequest struct {
	Interface   string `json:"interface,omitempty"` // defaults to the group's own
	Description string `json:"description,omitempty"`
	Replace     bool   `json:"replace"`
}

// handleApplyClientGroup steers every member of a group to an interface in
// one VPN Director write — the server-side twin of the UI's group preset.
func (s *Server) handleApplyClientGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req clientGroupApplyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	groups, err := s.loadClientGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var group *ClientGroup
	for i := range groups {
		if groups[i].ID == id {
			group = &groups[i]
			break
		}
	}
	if group == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("no client group %q", id))
		return
	}
	iface := req.Interface
	if iface == "" {
		iface = group.Interface
	}
	desc := req.Description
	if desc == "" {
		desc = group.Name
	}
	s.applyPolicyBulk(w, r, policyBulkRequest{
		Interface:   iface,
		Description: desc,
		Sources:     group.Members,
		Replace:     req.Replace,
	}, "vpn.client_groups.apply", map[string]interface{}{"group": id})
}
