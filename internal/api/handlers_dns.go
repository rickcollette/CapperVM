package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	capperdns "capper/internal/dns"
)

func (s *Server) handleListDNSZones(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "dns:zone:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	networkID := r.URL.Query().Get("networkId")
	zones, err := capperdns.NewManager(s.ctrl.Store.DNS).ListZones(networkID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, zones, nil)
}

func (s *Server) handleGetDNSZone(w http.ResponseWriter, r *http.Request) {
	zone := r.PathValue("zone")
	if err := s.authorize(r, "dns:zone:inspect", "dns/"+zone); err != nil {
		writeForbidden(w, err)
		return
	}
	networkID := r.URL.Query().Get("networkId")
	z, err := capperdns.NewManager(s.ctrl.Store.DNS).GetZone(zone, networkID)
	if err != nil {
		writeNotFound(w, "zone not found")
		return
	}
	records, _ := capperdns.NewManager(s.ctrl.Store.DNS).ListRecords(zone, networkID)
	writeData(w, map[string]any{"zone": z, "records": records}, nil)
}

func (s *Server) handleCreateDNSZone(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "dns:zone:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name        string `json:"name"`
		NetworkID   string `json:"networkId,omitempty"`
		Type        string `json:"type,omitempty"`
		DefaultTTL  int    `json:"defaultTtl,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.NetworkID != "" {
		if _, err := s.ctrl.Store.VPC.GetSubnetByID(req.NetworkID); err != nil {
			writeBadRequest(w, fmt.Errorf("networkId must be a vpc subnet id"))
			return
		}
	}
	z, err := capperdns.NewManager(s.ctrl.Store.DNS).CreateZone(req.Name, req.Type, req.NetworkID, req.DefaultTTL, req.Description)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: z})
}

func (s *Server) handleDeleteDNSZone(w http.ResponseWriter, r *http.Request) {
	zone := r.PathValue("zone")
	networkID := r.URL.Query().Get("networkId")
	if err := s.authorize(r, "dns:zone:delete", "dns/"+zone); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := capperdns.NewManager(s.ctrl.Store.DNS).DeleteZone(zone, networkID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateDNSRecord(w http.ResponseWriter, r *http.Request) {
	zone := r.PathValue("zone")
	if err := s.authorize(r, "dns:record:create", "dns/"+zone); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		NetworkID string   `json:"networkId,omitempty"`
		Name      string   `json:"name"`
		Type      string   `json:"type"`
		Values    []string `json:"values"`
		TTL       int      `json:"ttl,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.NetworkID != "" {
		if _, err := s.ctrl.Store.VPC.GetSubnetByID(req.NetworkID); err != nil {
			writeBadRequest(w, fmt.Errorf("networkId must be a vpc subnet id"))
			return
		}
	}
	rec, err := capperdns.NewManager(s.ctrl.Store.DNS).CreateRecord(zone, req.NetworkID, req.Name, req.Type, req.Values, req.TTL)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: rec})
}

func (s *Server) handleDeleteDNSRecord(w http.ResponseWriter, r *http.Request) {
	zone := r.PathValue("zone")
	id := r.PathValue("id")
	networkID := r.URL.Query().Get("networkId")
	if err := s.authorize(r, "dns:record:delete", "dns/"+zone); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := capperdns.NewManager(s.ctrl.Store.DNS).DeleteRecord(zone, networkID, id); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDNSQuery(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "dns:query", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		FQDN string `json:"fqdn"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	resolver := capperdns.NewResolver(s.ctrl.Store.DNS, nil, nil)
	rrs, err := resolver.Query(req.FQDN, req.Type)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, map[string]any{
		"fqdn":    req.FQDN,
		"type":    req.Type,
		"records": rrs,
		"text":    capperdns.FormatRRs(rrs),
	}, nil)
}
