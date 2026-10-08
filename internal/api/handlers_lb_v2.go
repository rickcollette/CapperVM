package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"capper/internal/ipam"
	"capper/internal/lb"
	"capper/internal/networking"
)

func (s *Server) lbVIPPlacer() *lb.VIPPlacer {
	return &lb.VIPPlacer{
		IPAM: ipam.NewManager(s.ctrl.Store.IPAM),
		VPC:  s.ctrl.Store.VPC.Store(),
		LB:   s.ctrl.Store.LB.Store(),
	}
}

func (s *Server) handleSubnetAvailableIPs(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	subnetID := r.PathValue("id")
	ips, err := lb.ListAvailableSubnetIPs(s.ctrl.Store.VPC.Store(), s.ctrl.Store.LB.Store(), subnetID, 50)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, ips, nil)
}

func (s *Server) handleGetLBTargetGroups(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "lb:inspect", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	lbObj, err := s.ctrl.Store.LB.Get(name, s.project)
	if err != nil {
		writeNotFound(w, "lb not found")
		return
	}
	tgs, err := s.ctrl.Store.LB.Store().ListTargetGroupsForLB(lbObj.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, tgs, nil)
}

func (s *Server) handleCreateLBTargetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "lb:update", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		TGName     string `json:"name"`
		Protocol   string `json:"protocol"`
		Port       int    `json:"port"`
		HealthPath string `json:"healthPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	tg, err := s.ctrl.Store.LB.CreateTargetGroupForLB(name, s.project, req.TGName, req.Protocol, req.Port, req.HealthPath)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: tg})
}

func (s *Server) handleDeleteLBTargetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tgID := r.PathValue("tgId")
	if err := s.authorize(r, "lb:update", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.LB.DeleteTargetGroup(name, s.project, tgID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetLBListener(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	lstID := r.PathValue("id")
	if err := s.authorize(r, "lb:inspect", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	lbObj, err := s.ctrl.Store.LB.Get(name, s.project)
	if err != nil {
		writeNotFound(w, "lb not found")
		return
	}
	lst, err := s.ctrl.Store.LB.Store().GetListener(lstID)
	if err != nil || lst.LoadBalancerID != lbObj.ID {
		writeNotFound(w, "listener not found")
		return
	}
	writeData(w, lst, nil)
}

func (s *Server) handleUpdateLBListener(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	lstID := r.PathValue("id")
	if err := s.authorize(r, "lb:update", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	lbObj, err := s.ctrl.Store.LB.Get(name, s.project)
	if err != nil {
		writeNotFound(w, "lb not found")
		return
	}
	var req struct {
		Port          int    `json:"port"`
		Protocol      string `json:"protocol"`
		CertificateID string `json:"certificateId"`
		TargetGroupID string `json:"targetGroupId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	lst, err := s.ctrl.Store.LB.Store().GetListener(lstID)
	if err != nil || lst.LoadBalancerID != lbObj.ID {
		writeNotFound(w, "listener not found")
		return
	}
	port := req.Port
	if port == 0 {
		port = lst.Port
	}
	proto := req.Protocol
	if proto == "" {
		proto = lst.Protocol
	}
	tgID := req.TargetGroupID
	if tgID == "" {
		tgID = lst.TargetGroupID
	}
	certID := req.CertificateID
	if certID == "" {
		certID = lst.CertificateID
	}
	if err := s.ctrl.Store.LB.Store().UpdateListener(lstID, port, proto, certID, tgID); err != nil {
		writeBadRequest(w, err)
		return
	}
	s.ctrl.Store.LB.Store().GetListener(lstID) // refresh not needed for response
	updated, _ := s.ctrl.Store.LB.Store().GetListener(lstID)
	writeData(w, updated, nil)
}

func (s *Server) handleDeleteLBListener(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	lstID := r.PathValue("id")
	if err := s.authorize(r, "lb:update", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.LB.DeleteListener(name, s.project, lstID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListLBTargets(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tgID := r.PathValue("tgId")
	if err := s.authorize(r, "lb:inspect", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	lbObj, err := s.ctrl.Store.LB.Get(name, s.project)
	if err != nil {
		writeNotFound(w, "lb not found")
		return
	}
	tg, err := s.ctrl.Store.LB.Store().GetTargetGroup(tgID)
	if err != nil || tg.LoadBalancerID != lbObj.ID {
		writeNotFound(w, "target group not found")
		return
	}
	targets, err := s.ctrl.Store.LB.Store().ListTargets(tgID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, targets, nil)
}

func (s *Server) handleAddLBTarget(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tgID := r.PathValue("tgId")
	if err := s.authorize(r, "lb:update", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Address string `json:"address"`
		Weight  int    `json:"weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	t, err := s.ctrl.Store.LB.AddTarget(name, s.project, tgID, req.Address)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: t})
}

func (s *Server) handleRemoveLBTarget(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tgID := r.PathValue("tgId")
	targetID := r.PathValue("targetId")
	if err := s.authorize(r, "lb:update", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.LB.RemoveTarget(name, s.project, tgID, targetID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAttachListenerCert(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	lbName := r.PathValue("name")
	lstID := r.PathValue("id")
	if err := s.authorize(r, "lb:update", "lb/"+lbName); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		CertID   string `json:"certId"`
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	lbRec, err := s.ctrl.Store.LB.Get(lbName, s.project)
	if err != nil {
		writeNotFound(w, "lb not found")
		return
	}
	lst, err := s.ctrl.Store.LB.Store().GetListener(lstID)
	if err != nil || lst.LoadBalancerID != lbRec.ID {
		writeNotFound(w, "listener not found")
		return
	}
	if err := s.ctrl.CertMgr.AttachToLoadBalancer(r.Context(), req.CertID, lbRec.ID, req.Hostname); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := s.ctrl.Store.LB.SetListenerCertificate(lbName, s.project, lstID, req.CertID); err != nil {
		writeInternal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDetachListenerCert(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	lbName := r.PathValue("name")
	lstID := r.PathValue("id")
	certID := r.URL.Query().Get("certId")
	if certID == "" {
		certID = r.PathValue("certId")
	}
	if err := s.authorize(r, "lb:update", "lb/"+lbName); err != nil {
		writeForbidden(w, err)
		return
	}
	lbRec, err := s.ctrl.Store.LB.Get(lbName, s.project)
	if err != nil {
		writeNotFound(w, "lb not found")
		return
	}
	if err := s.ctrl.CertMgr.DetachFromLoadBalancer(r.Context(), certID, lbRec.ID, ""); err != nil {
		writeInternal(w, err)
		return
	}
	if err := s.ctrl.Store.LB.SetListenerCertificate(lbName, s.project, lstID, ""); err != nil {
		writeInternal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createLBFromRequest handles both legacy and v2 LB create payloads.
func (s *Server) createLBFromRequest(w http.ResponseWriter, r *http.Request, req struct {
	Name              string        `json:"name"`
	Scheme            lb.LBScheme   `json:"scheme"`
	Type              lb.LBType     `json:"type"`
	VPCID             string        `json:"vpcId"`
	SubnetID          string        `json:"subnetId"`
	NetworkID         string        `json:"networkId,omitempty"`
	PoolID            string        `json:"poolId"`
	VIP               string        `json:"vip"`
	AutoVIP           bool          `json:"autoVip"`
	ListenAddr        string        `json:"listenAddr"`
	Mode              lb.LBMode     `json:"mode"`
	Algorithm         lb.LBAlgorithm `json:"algorithm,omitempty"`
	Selector          string        `json:"selector,omitempty"`
	TLSCertID         string        `json:"tlsCertId,omitempty"`
	ListenerProtocol  string        `json:"listenerProtocol"`
	ListenerPort      int           `json:"listenerPort"`
	ListenerCertID    string        `json:"listenerCertId"`
	TargetGroupName   string        `json:"targetGroupName"`
	TargetGroupPort   int           `json:"targetGroupPort"`
	InitialTargetAddr string        `json:"initialTargetAddr"`
}) {
	subnetID := req.SubnetID
	if subnetID == "" {
		subnetID = req.NetworkID
	}
	if subnetID == "" {
		writeBadRequest(w, fmt.Errorf("subnetId is required"))
		return
	}
	sub, err := networking.ResolveSubnetForLaunch(s.ctrl.Store.VPC, subnetID, req.VPCID)
	if err != nil {
		writeBadRequest(w, fmt.Errorf("subnet: %w", err))
		return
	}
	vpcID := sub.VPCID
	if req.VPCID != "" {
		vpcID = req.VPCID
	}

	// Legacy path: listenAddr + mode without scheme
	if req.Scheme == "" && req.ListenAddr != "" {
		if req.Mode == "" {
			req.Mode = lb.ModeTCP
		}
		result, err := s.ctrl.Store.LB.Create(req.Name, s.project, subnetID, req.ListenAddr, req.Mode)
		if err != nil {
			writeBadRequest(w, err)
			return
		}
		if req.Algorithm != "" || req.Selector != "" || req.TLSCertID != "" {
			algo := req.Algorithm
			if algo == "" {
				algo = lb.AlgoRoundRobin
			}
			_ = s.ctrl.Store.LB.SetMeta(result.Name, s.project, req.Selector, req.TLSCertID, algo)
			result, _ = s.ctrl.Store.LB.Get(result.Name, s.project)
		}
		writeJSON(w, http.StatusCreated, Envelope{Data: result})
		return
	}

	if req.Scheme == "" {
		req.Scheme = lb.SchemeInternal
	}
	if req.Type == "" {
		req.Type = lb.TypeApplication
	}

	placer := s.lbVIPPlacer()
	vip := req.VIP
	routableIPID := ""
	if req.AutoVIP || vip == "" {
		vip, routableIPID, err = placer.AllocateVIP(req.Scheme, s.project, req.Name, subnetID, req.PoolID, vip)
		if err != nil {
			writeBadRequest(w, fmt.Errorf("vip: %w", err))
			return
		}
	} else if req.Scheme == lb.SchemeInternetFacing && req.PoolID != "" {
		vip, routableIPID, err = placer.AllocateVIP(req.Scheme, s.project, req.Name, subnetID, req.PoolID, vip)
		if err != nil {
			writeBadRequest(w, fmt.Errorf("vip: %w", err))
			return
		}
	}

	opts := lb.CreateOptions{
		Name:              req.Name,
		Project:           s.project,
		Scheme:            req.Scheme,
		Type:              req.Type,
		VPCID:             vpcID,
		SubnetID:          subnetID,
		VIPAddress:        vip,
		RoutableIPID:      routableIPID,
		Algorithm:         req.Algorithm,
		Selector:          req.Selector,
		ListenerProtocol:  req.ListenerProtocol,
		ListenerPort:      req.ListenerPort,
		ListenerCertID:    req.ListenerCertID,
		TargetGroupName:   req.TargetGroupName,
		TargetGroupPort:   req.TargetGroupPort,
		InitialTargetAddr: req.InitialTargetAddr,
	}
	if opts.ListenerCertID == "" {
		opts.ListenerCertID = req.TLSCertID
	}
	detail, err := s.ctrl.Store.LB.CreateExtended(opts)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	if routableIPID != "" {
		_ = placer.AttachRoutableIP(routableIPID, detail.LoadBalancer.ID)
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: detail})
}
