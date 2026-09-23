package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/reece01-dock/axos/internal/backend"
	"github.com/reece01-dock/axos/internal/vpnbench"
)

// registerM3Routes wires Milestone 3 DNS / DHCP / QoS / VPN / firewall /
// policy / diag / perf endpoints. Mutating handlers audit like
// backup/shell_exec (X-Axos-Actor); reads use readHandler.
func (s *Server) registerM3Routes() {
	s.mux.HandleFunc("GET /v1/dns", s.readHandler(func(ctx context.Context) (interface{}, error) {
		return s.Backend.DNSConfig(ctx)
	}))
	s.mux.HandleFunc("POST /v1/dns", s.handleSetDNS)

	s.mux.HandleFunc("GET /v1/dhcp/reservations", s.readHandler(func(ctx context.Context) (interface{}, error) {
		return s.Backend.DHCPReservations(ctx)
	}))
	s.mux.HandleFunc("POST /v1/dhcp/reservations", s.handleSetDHCPReservation)
	s.mux.HandleFunc("DELETE /v1/dhcp/reservations/{mac}", s.handleDeleteDHCPReservation)

	s.mux.HandleFunc("GET /v1/qos", s.readHandler(func(ctx context.Context) (interface{}, error) {
		return s.Backend.QoSStatus(ctx)
	}))
	s.mux.HandleFunc("POST /v1/qos", s.handleSetQoS)

	s.mux.HandleFunc("GET /v1/vpn/profiles", s.readHandler(func(ctx context.Context) (interface{}, error) {
		return s.Backend.VPNProfiles(ctx)
	}))
	s.mux.HandleFunc("POST /v1/vpn/wireguard/import", s.handleImportWireGuard)
	s.mux.HandleFunc("POST /v1/vpn/benchmark", s.handleVPNBenchmark)
	s.mux.HandleFunc("POST /v1/vpn/{name}/up", s.handleVPNUp)
	s.mux.HandleFunc("POST /v1/vpn/{name}/down", s.handleVPNDown)

	s.mux.HandleFunc("POST /v1/firewall/apply", s.handleFirewallApply)
	s.mux.HandleFunc("POST /v1/firewall/delete", s.handleFirewallDelete)

	s.mux.HandleFunc("GET /v1/policy", s.readHandler(func(ctx context.Context) (interface{}, error) {
		return s.Backend.PolicyRoutes(ctx)
	}))
	s.mux.HandleFunc("POST /v1/policy", s.handleSetPolicyRoute)
	s.mux.HandleFunc("DELETE /v1/policy/{id}", s.handleDeletePolicyRoute)

	s.mux.HandleFunc("POST /v1/diag/ping", s.handleDiagPing)
	s.mux.HandleFunc("POST /v1/diag/traceroute", s.handleDiagTraceroute)
	s.mux.HandleFunc("POST /v1/diag/dns", s.handleDiagDNS)
	s.mux.HandleFunc("POST /v1/diag/port", s.handleDiagPort)
	s.mux.HandleFunc("POST /v1/perf/iperf3", s.handleIperf3)
}

func (s *Server) handleSetDNS(w http.ResponseWriter, r *http.Request) {
	var cfg backend.DNSInfo
	if err := readJSON(r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	err := s.Backend.SetDNSConfig(r.Context(), cfg)
	s.audit(s.actor(r), "dns.set", map[string]interface{}{
		"wan_upstreams": cfg.WANUpstreams, "lan_upstreams": cfg.LANUpstreams,
		"dot_enabled": cfg.DoTEnabled, "dot_profile": cfg.DoTProfile,
	}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSetDHCPReservation(w http.ResponseWriter, r *http.Request) {
	var res backend.DHCPReservation
	if err := readJSON(r, &res); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if res.MAC == "" || res.IP == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("mac and ip are required"))
		return
	}
	err := s.Backend.SetDHCPReservation(r.Context(), res)
	s.audit(s.actor(r), "dhcp.reservations.set", map[string]interface{}{
		"mac": res.MAC, "ip": res.IP, "hostname": res.Hostname,
	}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteDHCPReservation(w http.ResponseWriter, r *http.Request) {
	mac := r.PathValue("mac")
	if mac == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("mac is required"))
		return
	}
	err := s.Backend.DeleteDHCPReservation(r.Context(), mac)
	s.audit(s.actor(r), "dhcp.reservations.delete", map[string]interface{}{"mac": mac}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type qosEnableRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleSetQoS(w http.ResponseWriter, r *http.Request) {
	var req qosEnableRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	err := s.Backend.SetQoSEnable(r.Context(), req.Enabled)
	s.audit(s.actor(r), "qos.set", map[string]interface{}{"enabled": req.Enabled}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleImportWireGuard(w http.ResponseWriter, r *http.Request) {
	var p backend.WireGuardImport
	if err := readJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if p.Unit < 1 || p.PrivateKey == "" || p.PeerPublicKey == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unit, private_key, and peer_public_key are required"))
		return
	}
	err := s.Backend.ImportWireGuard(r.Context(), p)
	// Never audit private keys — only identifying fields.
	s.audit(s.actor(r), "vpn.wireguard.import", map[string]interface{}{
		"unit": p.Unit, "endpoint": p.Endpoint, "description": p.Description,
	}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVPNUp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	err := s.Backend.VPNUp(r.Context(), name)
	s.audit(s.actor(r), "vpn.up", map[string]interface{}{"name": name}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "up", "name": name})
}

func (s *Server) handleVPNDown(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	err := s.Backend.VPNDown(r.Context(), name)
	s.audit(s.actor(r), "vpn.down", map[string]interface{}{"name": name}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "down", "name": name})
}

type vpnBenchmarkRequest struct {
	Hosts []string `json:"hosts"`
	Count int      `json:"count"`
}

func (s *Server) handleVPNBenchmark(w http.ResponseWriter, r *http.Request) {
	var req vpnBenchmarkRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := vpnbench.Run(r.Context(), s.Backend, req.Hosts, req.Count)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleFirewallApply(w http.ResponseWriter, r *http.Request) {
	var rule backend.FirewallRule
	if err := readJSON(r, &rule); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	err := s.Backend.FirewallApply(r.Context(), rule)
	s.audit(s.actor(r), "firewall.rules.set", map[string]interface{}{
		"table": rule.Table, "chain": rule.Chain, "rule": rule.Rule,
	}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleFirewallDelete(w http.ResponseWriter, r *http.Request) {
	var rule backend.FirewallRule
	if err := readJSON(r, &rule); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	err := s.Backend.FirewallDelete(r.Context(), rule)
	s.audit(s.actor(r), "firewall.rules.delete", map[string]interface{}{
		"table": rule.Table, "chain": rule.Chain, "rule": rule.Rule,
	}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	err := s.Backend.SetPolicyRoute(r.Context(), route)
	s.audit(s.actor(r), "route.policy.set", map[string]interface{}{
		"id": route.ID, "source": route.Source, "interface": route.Interface,
		"kill_switch": route.KillSwitch, "enabled": route.Enabled,
	}, "", err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

type diagPingRequest struct {
	Host  string `json:"host"`
	Count int    `json:"count"`
}

func (s *Server) handleDiagPing(w http.ResponseWriter, r *http.Request) {
	var req diagPingRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Host == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("host is required"))
		return
	}
	if req.Count <= 0 {
		req.Count = 4
	}
	result, err := s.Backend.Ping(r.Context(), req.Host, req.Count)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type diagTracerouteRequest struct {
	Host    string `json:"host"`
	MaxHops int    `json:"max_hops"`
}

func (s *Server) handleDiagTraceroute(w http.ResponseWriter, r *http.Request) {
	var req diagTracerouteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Host == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("host is required"))
		return
	}
	if req.MaxHops <= 0 {
		req.MaxHops = 30
	}
	result, err := s.Backend.Traceroute(r.Context(), req.Host, req.MaxHops)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type diagDNSRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleDiagDNS(w http.ResponseWriter, r *http.Request) {
	var req diagDNSRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	result, err := s.Backend.DNSLookup(r.Context(), req.Name)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type diagPortRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (s *Server) handleDiagPort(w http.ResponseWriter, r *http.Request) {
	var req diagPortRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Host == "" || req.Port <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("host and port are required"))
		return
	}
	result, err := s.Backend.PortCheck(r.Context(), req.Host, req.Port)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleIperf3(w http.ResponseWriter, r *http.Request) {
	var opts backend.IperfOpts
	if err := readJSON(r, &opts); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if opts.Mode == "" {
		opts.Mode = "client"
	}
	result, err := s.Backend.Iperf3(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
