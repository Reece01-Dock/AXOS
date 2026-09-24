package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/reece01-dock/axos/internal/backend"
)

// Policy (VPN Director) routes. Every write goes through
// backend.MergePolicyRoutes so that:
//   - a change never silently overwrites an existing rule for the same
//     device — callers get 409 + the conflicting rules unless they pass
//     replace=true;
//   - a bulk change is one ReplacePolicyRoutes call (one nvram commit, one
//     routing restart), so it either lands whole or not at all.

func (s *Server) registerPolicyRoutes() {
	s.mux.HandleFunc("POST /v1/policy", s.handleSetPolicyRoute)
	s.mux.HandleFunc("PUT /v1/policy", s.handleReplacePolicyRoutes)
	s.mux.HandleFunc("POST /v1/policy/bulk", s.handlePolicyBulk)
	s.mux.HandleFunc("POST /v1/policy/bulk/remove", s.handlePolicyBulkRemove)
	s.mux.HandleFunc("DELETE /v1/policy/{id}", s.handleDeletePolicyRoute)
	s.mux.HandleFunc("POST /v1/vpn/client-groups/{id}/apply", s.handleApplyClientGroup)
}

type policyConflictBody struct {
	Error     string                   `json:"error"`
	Conflicts []backend.PolicyConflict `json:"conflicts"`
}

func writePolicyConflict(w http.ResponseWriter, conflicts []backend.PolicyConflict) {
	srcs := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		srcs = append(srcs, c.Existing.Source+" → "+c.Existing.Interface)
	}
	writeJSON(w, http.StatusConflict, policyConflictBody{
		Error:     "existing VPN Director rules would be replaced (" + strings.Join(srcs, ", ") + "); resend with replace=true to overwrite them",
		Conflicts: conflicts,
	})
}

func (s *Server) handleSetPolicyRoute(w http.ResponseWriter, r *http.Request) {
	var route backend.PolicyRoute
	if err := readJSON(r, &route); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if route.Source == "" || route.Interface == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("source and interface are required"))
		return
	}
	resolved, ok := s.resolveSources(w, r, []string{route.Source})
	if !ok {
		return
	}
	route.Source = resolved[0]
	route.Interface = backend.NormalizeDirectorIface(route.Interface)
	args := map[string]interface{}{
		"id": route.ID, "source": route.Source, "remote": route.Remote,
		"interface": route.Interface, "kill_switch": route.KillSwitch, "enabled": route.Enabled,
	}
	ctx := r.Context()
	if route.ID != "" {
		err := s.Backend.SetPolicyRoute(ctx, route)
		s.audit(s.actor(r), "route.policy.set", args, "", err)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	existing, err := s.Backend.PolicyRoutes(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	res := backend.MergePolicyRoutes(existing, []backend.PolicyRoute{route}, queryBool(r, "replace"))
	if len(res.Conflicts) > 0 && res.Replaced == 0 {
		writePolicyConflict(w, res.Conflicts)
		return
	}
	if res.Added+res.Replaced > 0 {
		err = s.Backend.ReplacePolicyRoutes(ctx, res.Routes)
		s.audit(s.actor(r), "route.policy.set", args, "", err)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, policyResult(res))
}

func (s *Server) handleReplacePolicyRoutes(w http.ResponseWriter, r *http.Request) {
	var routes []backend.PolicyRoute
	if err := readJSON(r, &routes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if routes == nil {
		routes = []backend.PolicyRoute{}
	}
	if err := backend.ValidatePolicyRoutes(routes); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	err := s.Backend.ReplacePolicyRoutes(r.Context(), routes)
	s.audit(s.actor(r), "route.policy.replace", map[string]interface{}{"count": len(routes)}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "count": len(routes)})
}

func (s *Server) handleDeletePolicyRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("id is required"))
		return
	}
	err := s.Backend.DeletePolicyRoute(r.Context(), id)
	s.audit(s.actor(r), "route.policy.delete", map[string]interface{}{"id": id}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type policyBulkRequest struct {
	Interface   string   `json:"interface"` // WAN, WGC5, OVPN1, ...
	Description string   `json:"description"`
	Sources     []string `json:"sources"` // MAC or IP per client
	Enabled     *bool    `json:"enabled,omitempty"`
	Replace     bool     `json:"replace"`
}

func (s *Server) handlePolicyBulk(w http.ResponseWriter, r *http.Request) {
	var req policyBulkRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.applyPolicyBulk(w, r, req, "route.policy.bulk", nil)
}

// applyPolicyBulk merges one rule per source into the Director list and
// writes it in a single backend call.
func (s *Server) applyPolicyBulk(w http.ResponseWriter, r *http.Request, req policyBulkRequest, action string, extra map[string]interface{}) {
	iface := backend.NormalizeDirectorIface(req.Interface)
	if iface == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("interface is required"))
		return
	}
	ctx := r.Context()
	sources, ok := s.resolveSources(w, r, req.Sources)
	if !ok {
		return
	}
	enabled := req.Enabled == nil || *req.Enabled
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = "axos"
	}
	updates := make([]backend.PolicyRoute, 0, len(sources))
	for _, src := range sources {
		updates = append(updates, backend.PolicyRoute{
			Source: src, Interface: iface, Description: desc, Enabled: enabled,
		})
	}
	existing, err := s.Backend.PolicyRoutes(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	res := backend.MergePolicyRoutes(existing, updates, req.Replace)
	if len(res.Conflicts) > 0 && !req.Replace {
		writePolicyConflict(w, res.Conflicts)
		return
	}
	if res.Added+res.Replaced > 0 {
		args := map[string]interface{}{
			"interface": iface, "sources": sources, "replace": req.Replace,
			"added": res.Added, "replaced": res.Replaced,
		}
		for k, v := range extra {
			args[k] = v
		}
		err = s.Backend.ReplacePolicyRoutes(ctx, res.Routes)
		s.audit(s.actor(r), action, args, "", err)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, policyResult(res))
}

func (s *Server) handlePolicyBulkRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sources []string `json:"sources"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sources, ok := s.resolveSources(w, r, req.Sources)
	if !ok {
		return
	}
	ctx := r.Context()
	existing, err := s.Backend.PolicyRoutes(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	out, removed := backend.RemovePolicySources(existing, sources)
	if removed > 0 {
		err = s.Backend.ReplacePolicyRoutes(ctx, out)
		s.audit(s.actor(r), "route.policy.bulk_remove", map[string]interface{}{
			"sources": sources, "removed": removed,
		}, "", err)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "removed": removed})
}

// resolveSources de-duplicates sources and maps MACs to the device's current
// IP (VPN Director matches on IP). On failure it writes the response and
// returns ok=false.
func (s *Server) resolveSources(w http.ResponseWriter, r *http.Request, raw []string) ([]string, bool) {
	sources := dedupSources(raw)
	if len(sources) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("sources is required"))
		return nil, false
	}
	clients, err := s.Backend.Clients(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return nil, false
	}
	resolved, err := backend.ResolvePolicySources(sources, clients)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return nil, false
	}
	return dedupSources(resolved), true
}

func policyResult(res backend.PolicyMergeResult) map[string]interface{} {
	return map[string]interface{}{
		"status":    "ok",
		"added":     res.Added,
		"replaced":  res.Replaced,
		"unchanged": res.Unchanged,
		"applied":   res.Added + res.Replaced,
	}
}

func dedupSources(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, src := range in {
		src = strings.TrimSpace(src)
		k := backend.PolicySourceKey(src)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, src)
	}
	return out
}

func queryBool(r *http.Request, name string) bool {
	switch strings.ToLower(r.URL.Query().Get(name)) {
	case "1", "true", "yes":
		return true
	}
	return false
}
