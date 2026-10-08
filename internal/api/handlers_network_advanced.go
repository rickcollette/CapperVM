package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"capper/internal/lb"
	"capper/internal/networking"
)

func (s *Server) handleAnalyzeReachability(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req networking.ReachabilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	var sgIDs []string
	if req.SourceType == "instance" && req.SourceID != "" {
		if inst, err := s.ctrl.Store.ResolveInstance(req.SourceID); err == nil {
			sgIDs = inst.SecurityGroupIDs
		}
	}
	result := networking.AnalyzeReachabilityWithVPC(req, s.ctrl.Store.VPC, sgIDs, req.Port)
	writeData(w, result, nil)
}

func (s *Server) handleListVpcEndpoints(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	eps, err := s.ctrl.Store.VPC.ListVPCEndpoints(vpcID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, eps, nil)
}

func (s *Server) handleCreateVpcEndpoint(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		VPCID        string   `json:"vpcId"`
		Name         string   `json:"name"`
		ServiceName  string   `json:"serviceName"`
		EndpointType string   `json:"endpointType"`
		SubnetIDs    []string `json:"subnetIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	ep, err := s.ctrl.Store.VPC.CreateVPCEndpoint(req.VPCID, req.Name, req.ServiceName, req.EndpointType, req.SubnetIDs)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: ep})
}

func (s *Server) handleListVpcPeerings(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	peerings, err := s.ctrl.Store.VPC.ListVPCPeerings(vpcID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, peerings, nil)
}

func (s *Server) handleCreateVpcPeering(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		RequesterVPCID string `json:"requesterVpcId"`
		AccepterVPCID  string `json:"accepterVpcId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	p, err := s.ctrl.Store.VPC.CreateVPCPeering(req.RequesterVPCID, req.AccepterVPCID)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: p})
}

func (s *Server) handleListFlowLogs(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	resourceID := r.URL.Query().Get("resourceId")
	logs, err := s.ctrl.Store.VPC.ListFlowLogs(resourceID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, logs, nil)
}

func (s *Server) handleCreateFlowLog(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		ResourceType string `json:"resourceType"`
		ResourceID   string `json:"resourceId"`
		Destination  string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	fl, err := s.ctrl.Store.VPC.CreateFlowLog(req.ResourceType, req.ResourceID, req.Destination)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: fl})
}

func (s *Server) handleNetworkTopologyGraph(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	graph, err := networking.BuildTopologyGraph(s.netSvc(), s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, graph, nil)
}

func (s *Server) handleNetworkingDashboard(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	dash, err := networking.BuildDashboard(s.netSvc(), s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, dash, nil)
}

func (s *Server) handleNetworkingDrift(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcRef := r.URL.Query().Get("vpcId")
	if vpcRef == "" {
		writeBadRequest(w, errMissing("vpcId"))
		return
	}
	reports, err := networking.ListVPCDrift(s.netSvc(), s.project, vpcRef)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	writeData(w, reports, nil)
}

func (s *Server) handleListTargetGroups(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "lb:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	tgs, err := s.ctrl.Store.LB.ListTargetGroups(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, tgs, nil)
}

func (s *Server) handleCreateTargetGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "lb:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name           string `json:"name"`
		VPCID          string `json:"vpcId"`
		LoadBalancerID string `json:"loadBalancerId"`
		Protocol       string `json:"protocol"`
		Port           int    `json:"port"`
		HealthPath     string `json:"healthPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.LoadBalancerID == "" {
		writeError(w, http.StatusBadRequest, "loadBalancerId is required")
		return
	}
	lbObj, err := s.ctrl.Store.LB.Get(req.LoadBalancerID, s.project)
	if err != nil {
		writeNotFound(w, "load balancer not found")
		return
	}
	tg, err := s.ctrl.Store.LB.CreateTargetGroupForLB(lbObj.Name, s.project, req.Name, req.Protocol, req.Port, req.HealthPath)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	_ = req.VPCID
	writeJSON(w, http.StatusCreated, Envelope{Data: tg})
}

func (s *Server) handleListLBListeners(w http.ResponseWriter, r *http.Request) {
	lbRef := r.PathValue("name")
	if err := s.authorize(r, "lb:inspect", "lb/"+lbRef); err != nil {
		writeForbidden(w, err)
		return
	}
	lbObj, err := s.ctrl.Store.LB.Get(lbRef, s.project)
	if err != nil {
		writeNotFound(w, "load balancer not found")
		return
	}
	listeners, err := s.ctrl.Store.LB.ListListeners(lbObj.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, listeners, nil)
}

func (s *Server) handleCreateLBListener(w http.ResponseWriter, r *http.Request) {
	lbRef := r.PathValue("name")
	if err := s.authorize(r, "lb:update", "lb/"+lbRef); err != nil {
		writeForbidden(w, err)
		return
	}
	lbObj, err := s.ctrl.Store.LB.Get(lbRef, s.project)
	if err != nil {
		writeNotFound(w, "load balancer not found")
		return
	}
	var req struct {
		TargetGroupID string `json:"targetGroupId"`
		Protocol      string `json:"protocol"`
		Port          int    `json:"port"`
		CertificateID string `json:"certificateId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	l, err := s.ctrl.Store.LB.CreateListenerForLB(lbRef, s.project, req.TargetGroupID, req.Protocol, req.Port, req.CertificateID)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	_ = lbObj
	writeJSON(w, http.StatusCreated, Envelope{Data: l})
}

func (s *Server) handleAssociateDNSZoneVPC(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "dns:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	zoneRef := r.PathValue("zone")
	var req struct {
		VPCID string `json:"vpcId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := s.ctrl.Store.DNS.AssociateZoneVPC(zoneRef, req.VPCID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDisassociateDNSZoneVPC(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "dns:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	zoneRef := r.PathValue("zone")
	vpcID := r.URL.Query().Get("vpcId")
	if vpcID == "" {
		writeBadRequest(w, errMissing("vpcId"))
		return
	}
	if err := s.ctrl.Store.DNS.DisassociateZoneVPC(zoneRef, vpcID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListDNSZoneVPCs(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "dns:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	zoneRef := r.PathValue("zone")
	assocs, err := s.ctrl.Store.DNS.ListZoneVPCs(zoneRef)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, assocs, nil)
}

// errMissing returns a simple missing-parameter error.
func errMissing(field string) error {
	return &missingParamError{field: field}
}

type missingParamError struct{ field string }

func (e *missingParamError) Error() string { return e.field + " is required" }

// Ensure lb package types are referenced for swagger/docgen.
var _ = lb.TargetGroup{}

func (s *Server) handleListLaunchTemplateVersions(w http.ResponseWriter, r *http.Request) {
	if _, err := s.ctrl.Store.VPC.GetLaunchTemplate(s.project, r.PathValue("templateId")); err != nil {
		writeNotFound(w, "launch template not found")
		return
	}
	versions, err := s.ctrl.Store.VPC.ListLaunchTemplateVersions(s.project, r.PathValue("templateId"))
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, versions, nil)
}

func (s *Server) handleCreateLaunchTemplateVersion(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "launch-template:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Config map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	v, err := s.ctrl.Store.VPC.CreateLaunchTemplateVersion(s.project, r.PathValue("templateId"), req.Config)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: v})
}

func mergeLaunchTemplateIntoRequest(s *Server, project string, req *createInstanceRequest) error {
	if req.LaunchTemplateID == "" {
		return nil
	}
	overrides := map[string]any{
		"image": req.Image, "name": req.Name, "instanceType": req.InstanceType,
		"network": req.Network, "vpcId": req.VPCID, "subnetId": req.SubnetID,
		"keyName": req.KeyName, "publicIpBehavior": req.PublicIPBehavior,
	}
	cfg, err := s.ctrl.Store.VPC.ResolveLaunchConfig(project, req.LaunchTemplateID, req.LaunchTemplateVersion, overrides)
	if err != nil {
		return err
	}
	if v, ok := cfg["image"].(string); ok && v != "" && req.Image == "" {
		req.Image = v
	}
	if v, ok := cfg["name"].(string); ok && v != "" && req.Name == "" {
		req.Name = v
	}
	if v, ok := cfg["instanceType"].(string); ok && v != "" && req.InstanceType == "" {
		req.InstanceType = v
	}
	if v, ok := cfg["vpcId"].(string); ok && v != "" && req.VPCID == "" {
		req.VPCID = v
	}
	if v, ok := cfg["subnetId"].(string); ok && v != "" && req.SubnetID == "" {
		req.SubnetID = v
	}
	if v, ok := cfg["network"].(string); ok && v != "" {
		if req.SubnetID == "" {
			req.SubnetID = v
		}
		// legacy template field maps to subnetId; do not set Network (create rejects it)
	}
	if v, ok := cfg["keyName"].(string); ok && v != "" && req.KeyName == "" {
		req.KeyName = v
	}
	if v, ok := cfg["publicIpBehavior"].(string); ok && v != "" && req.PublicIPBehavior == "" {
		req.PublicIPBehavior = v
	}
	if sg, ok := cfg["securityGroupIds"].([]any); ok && len(req.SecurityGroupIDs) == 0 {
		for _, item := range sg {
			if s, ok := item.(string); ok {
				req.SecurityGroupIDs = append(req.SecurityGroupIDs, s)
			}
		}
	}
	return nil
}

func parseLaunchTemplateVersion(s string) int {
	if s == "" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}
