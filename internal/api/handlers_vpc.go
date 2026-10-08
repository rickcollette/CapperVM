package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"capper/internal/networking"
	"capper/internal/vpc"
)

func (s *Server) netSvc() *networking.Service {
	return s.ctrl.Store.Networking
}

// ---- Canonical VPC handlers (unified model) ---------------------------------

func (s *Server) handleListVPCsUnified(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcs, err := s.netSvc().ListVPCs(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, vpcs, nil)
}

func (s *Server) handleCreateVPCUnified(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name                  string            `json:"name"`
		Slug                  string            `json:"slug"`
		CIDR                  string            `json:"cidr"`
		PrimaryIPv4CIDR       string            `json:"primaryIpv4Cidr"`
		RealmID               string            `json:"realmId"`
		HomeRegionID          string            `json:"homeRegionId"`
		Description           string            `json:"description"`
		DNSDomain             string            `json:"dnsDomain"`
		MobilityPolicy        string            `json:"mobilityPolicy"`
		EnableFlowLogs        bool              `json:"enableFlowLogs"`
		Labels                map[string]string `json:"labels"`
		AttachInternetGateway bool              `json:"attachInternetGateway"`
		InitialSubnets        []struct {
			Name               string         `json:"name"`
			Slug               string         `json:"slug"`
			CIDR               string         `json:"cidr"`
			ZoneID             string         `json:"zoneId"`
			Kind               vpc.SubnetKind `json:"kind"`
			SubnetType         vpc.SubnetKind `json:"subnetType"`
			AutoAssignPublicIP bool           `json:"autoAssignPublicIp"`
		} `json:"initialSubnets"`
		NATGateway *struct {
			SubnetID   string `json:"subnetId"`
			SubnetCIDR string `json:"subnetCidr"`
			Name       string `json:"name"`
			PublicIP   string `json:"publicIp"`
		} `json:"natGateway"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	cidr := req.CIDR
	if cidr == "" {
		cidr = req.PrimaryIPv4CIDR
	}
	in := networking.CreateVPCInput{
		Project:               s.project,
		Name:                  req.Name,
		Slug:                  req.Slug,
		CIDR:                  cidr,
		RealmID:               req.RealmID,
		HomeRegionID:          req.HomeRegionID,
		Description:           req.Description,
		DNSDomain:             req.DNSDomain,
		MobilityPolicy:        req.MobilityPolicy,
		EnableFlowLogs:        req.EnableFlowLogs,
		Labels:                req.Labels,
		AttachInternetGateway: req.AttachInternetGateway,
	}
	for _, sub := range req.InitialSubnets {
		kind := sub.SubnetType
		if kind == "" {
			kind = sub.Kind
		}
		in.InitialSubnets = append(in.InitialSubnets, networking.InitialSubnetInput{
			Name:               sub.Name,
			Slug:               sub.Slug,
			CIDR:               sub.CIDR,
			ZoneID:             sub.ZoneID,
			SubnetType:         kind,
			AutoAssignPublicIP: sub.AutoAssignPublicIP,
		})
	}
	if req.NATGateway != nil {
		in.NATGateway = &networking.NATGatewayInput{
			SubnetID:   req.NATGateway.SubnetID,
			SubnetCIDR: req.NATGateway.SubnetCIDR,
			Name:       req.NATGateway.Name,
			PublicIP:   req.NATGateway.PublicIP,
		}
	}
	detail, err := s.netSvc().CreateVPC(in)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: detail})
}

func (s *Server) handleGetVPCUnified(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	if err := s.authorize(r, "vpc:get", "vpc/"+ref); err != nil {
		writeForbidden(w, err)
		return
	}
	v, err := s.netSvc().GetVPC(s.project, ref)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	writeData(w, v, nil)
}

func (s *Server) handlePatchVPCUnified(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	if err := s.authorize(r, "vpc:update", "vpc/"+ref); err != nil {
		writeForbidden(w, err)
		return
	}
	var body struct {
		Name           string            `json:"name"`
		Description    string            `json:"description"`
		MobilityPolicy string            `json:"mobilityPolicy"`
		Labels         map[string]string `json:"labels"`
		DNSDomain      string            `json:"dnsDomain"`
		DNSSupport     *bool             `json:"dnsSupport"`
		DNSHostnames   *bool             `json:"dnsHostnames"`
		EnableFlowLogs *bool             `json:"enableFlowLogs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, err)
		return
	}
	v, err := s.netSvc().UpdateVPC(s.project, ref, networking.VPCPatch{
		Name: body.Name, Description: body.Description, MobilityPolicy: body.MobilityPolicy,
		Labels: body.Labels, DNSDomain: body.DNSDomain,
		DNSSupport: body.DNSSupport, DNSHostnames: body.DNSHostnames, EnableFlowLogs: body.EnableFlowLogs,
	})
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, v, nil)
}

func (s *Server) handleDeleteVPCUnified(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	if err := s.authorize(r, "vpc:delete", "vpc/"+ref); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.netSvc().DeleteVPC(s.project, ref); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleVPCSummary(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	sum, err := s.netSvc().VPCSummary(s.project, ref)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	writeData(w, sum, nil)
}

func (s *Server) handleVPCDetail(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	if err := s.authorize(r, "vpc:get", "vpc/"+ref); err != nil {
		writeForbidden(w, err)
		return
	}
	detail, err := s.netSvc().VPCDetail(s.project, ref)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	writeData(w, detail, nil)
}

func (s *Server) handleSubnetDependencies(w http.ResponseWriter, r *http.Request) {
	subnetID := r.PathValue("subnetId")
	if subnetID == "" {
		subnetID = r.PathValue("id")
	}
	deps, err := s.netSvc().SubnetDependencies(subnetID)
	if err != nil {
		writeNotFound(w, "subnet not found")
		return
	}
	writeData(w, deps, nil)
}

func (s *Server) handleVPCDependencies(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	deps, err := s.netSvc().VPCDependencies(s.project, ref)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	writeData(w, deps, nil)
}

// ---- Subnets ----------------------------------------------------------------

func (s *Server) handleListVPCSubnetsUnified(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	if err := s.authorize(r, "vpc:get", "vpc/"+ref); err != nil {
		writeForbidden(w, err)
		return
	}
	purpose := r.URL.Query().Get("purpose")
	var (
		subs []vpc.Subnet
		err  error
	)
	if purpose != "" {
		subs, err = s.netSvc().ListSubnetsForPurpose(s.project, ref, networking.SubnetPurpose(purpose))
	} else {
		v, gerr := s.netSvc().GetVPC(s.project, ref)
		if gerr != nil {
			writeNotFound(w, "vpc not found")
			return
		}
		subs, err = s.ctrl.Store.VPC.ListSubnets(v.ID)
	}
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	writeData(w, subs, nil)
}

func (s *Server) handleCreateVPCSubnetUnified(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	if err := s.authorize(r, "vpc:update", "vpc/"+ref); err != nil {
		writeForbidden(w, err)
		return
	}
	v, err := s.netSvc().GetVPC(s.project, ref)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	var req struct {
		Name               string         `json:"name"`
		Slug               string         `json:"slug"`
		CIDR               string         `json:"cidr"`
		RegionID           string         `json:"regionId"`
		ZoneID             string         `json:"zoneId"`
		Zone               string         `json:"zone"` // legacy alias
		SubnetType         vpc.SubnetKind `json:"subnetType"`
		Kind               vpc.SubnetKind `json:"kind"`
		AutoAssignPublicIP bool           `json:"autoAssignPublicIp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.ZoneID == "" {
		req.ZoneID = req.Zone
	}
	kind := req.SubnetType
	if kind == "" {
		kind = req.Kind
	}
	sub, err := s.netSvc().CreateSubnet(s.project, networking.CreateSubnetInput{
		VPCID:              v.ID,
		Name:               req.Name,
		Slug:               req.Slug,
		CIDR:               req.CIDR,
		RegionID:           req.RegionID,
		ZoneID:             req.ZoneID,
		SubnetType:         kind,
		AutoAssignPublicIP: req.AutoAssignPublicIP,
	})
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: sub})
}

func (s *Server) handleGetSubnet(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	sub, err := s.ctrl.Store.VPC.GetSubnetByID(r.PathValue("subnetId"))
	if err != nil {
		writeNotFound(w, "subnet not found")
		return
	}
	writeData(w, sub, nil)
}

func (s *Server) handlePatchSubnet(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	sub, err := s.ctrl.Store.VPC.GetSubnetByID(r.PathValue("subnetId"))
	if err != nil {
		writeNotFound(w, "subnet not found")
		return
	}
	var patch struct {
		Name               string `json:"name"`
		AutoAssignPublicIP *bool  `json:"autoAssignPublicIp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeBadRequest(w, err)
		return
	}
	if patch.Name != "" {
		sub.Name = patch.Name
	}
	if patch.AutoAssignPublicIP != nil {
		sub.AutoAssignPublicIP = *patch.AutoAssignPublicIP
	}
	updated, err := s.ctrl.Store.VPC.UpdateSubnet(sub)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, updated, nil)
}

func (s *Server) handleDeleteSubnet(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	subnetID := r.PathValue("subnetId")
	if err := s.netSvc().DeleteSubnet(subnetID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Route tables -----------------------------------------------------------

func (s *Server) handleListRouteTables(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	v, err := s.netSvc().GetVPC(s.project, ref)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	rts, err := s.ctrl.Store.VPC.ListRouteTables(v.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, rts, nil)
}

func (s *Server) handleCreateRouteTable(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	ref := r.PathValue("vpcId")
	if ref == "" {
		ref = r.PathValue("vpc")
	}
	v, err := s.netSvc().GetVPC(s.project, ref)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	rt, err := s.ctrl.Store.VPC.CreateRouteTable(v.ID, req.Name)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: rt})
}

func (s *Server) handleGetRouteTable(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	rt, err := s.ctrl.Store.VPC.GetRouteTableByID(r.PathValue("routeTableId"))
	if err != nil {
		writeNotFound(w, "route table not found")
		return
	}
	routes, _ := s.ctrl.Store.VPC.ListRoutes(rt.ID)
	writeData(w, map[string]any{"routeTable": rt, "routes": routes}, nil)
}

func (s *Server) handleAddRoute(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		DestinationCIDR string `json:"destinationCidr"`
		Destination     string `json:"destination"`
		TargetType      string `json:"targetType"`
		TargetID        string `json:"targetId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	dest := req.DestinationCIDR
	if dest == "" {
		dest = req.Destination
	}
	route, err := s.ctrl.Store.VPC.AddRoute(r.PathValue("routeTableId"), dest, req.TargetType, req.TargetID)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: route})
}

func (s *Server) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.VPC.DeleteRoute(r.PathValue("routeId")); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAssociateSubnetRouteTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RouteTableID string `json:"routeTableId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := s.ctrl.Store.VPC.AssociateSubnet(r.PathValue("subnetId"), req.RouteTableID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Security groups --------------------------------------------------------

func (s *Server) handleListSecurityGroups(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	var sgs []vpc.SecurityGroup
	var err error
	if vpcID != "" {
		v, gerr := s.netSvc().GetVPC(s.project, vpcID)
		if gerr != nil {
			writeNotFound(w, "vpc not found")
			return
		}
		sgs, err = s.ctrl.Store.VPC.ListSecurityGroups(v.ID)
	} else {
		vpcs, _ := s.netSvc().ListVPCs(s.project)
		for _, v := range vpcs {
			part, _ := s.ctrl.Store.VPC.ListSecurityGroups(v.ID)
			sgs = append(sgs, part...)
		}
	}
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, sgs, nil)
}

func (s *Server) handleCreateSecurityGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		VPCID       string `json:"vpcId"`
		Name        string `json:"name"`
		Description string `json:"description"`
		DefaultDeny *bool  `json:"defaultDeny"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	v, err := s.netSvc().GetVPC(s.project, req.VPCID)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	deny := true
	if req.DefaultDeny != nil {
		deny = *req.DefaultDeny
	}
	sg, err := s.ctrl.Store.VPC.CreateSecurityGroup(v.ID, req.Name, req.Description, deny)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: sg})
}

func (s *Server) handleGetSecurityGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	sgID := r.PathValue("sgId")
	vpcID := r.URL.Query().Get("vpcId")
	sg, err := s.ctrl.Store.VPC.GetSecurityGroup(sgID, vpcID)
	if err != nil {
		writeNotFound(w, "security group not found")
		return
	}
	rules, _ := s.ctrl.Store.VPC.ListSGRules(sg.ID)
	writeData(w, map[string]any{"securityGroup": sg, "rules": rules}, nil)
}

func (s *Server) handleDeleteSecurityGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	if err := s.ctrl.Store.VPC.DeleteSecurityGroup(r.PathValue("sgId"), vpcID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddSGRule(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Direction string `json:"direction"`
		Protocol  string `json:"protocol"`
		FromPort  int    `json:"fromPort"`
		ToPort    int    `json:"toPort"`
		CIDR      string `json:"cidr"`
		CIDRIpv4  string `json:"cidrIpv4"`
		Action    string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	cidr := req.CIDR
	if cidr == "" {
		cidr = req.CIDRIpv4
	}
	dir := vpc.SGRuleDirection(req.Direction)
	rule, err := s.ctrl.Store.VPC.AddSGRule(r.PathValue("sgId"), dir, req.Protocol, cidr, req.FromPort, req.ToPort, req.Action)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: rule})
}

func (s *Server) handleDeleteSGRule(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.VPC.DeleteSGRule(r.PathValue("ruleId")); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Internet gateways ------------------------------------------------------

func (s *Server) handleListIGWs(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	if vpcID == "" {
		writeData(w, []vpc.InternetGateway{}, nil)
		return
	}
	v, err := s.netSvc().GetVPC(s.project, vpcID)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	igws, err := s.ctrl.Store.VPC.ListIGWs(v.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, igws, nil)
}

func (s *Server) handleCreateIGW(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		VPCID string `json:"vpcId"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	v, err := s.netSvc().GetVPC(s.project, req.VPCID)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	igw, err := s.ctrl.Store.VPC.CreateIGW(v.ID, req.Name)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: igw})
}

func (s *Server) handleDeleteIGW(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	if err := s.ctrl.Store.VPC.DeleteIGW(r.PathValue("igwId"), vpcID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- NAT gateways -----------------------------------------------------------

func (s *Server) handleListNATGateways(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	if vpcID == "" {
		writeData(w, []vpc.NATGateway{}, nil)
		return
	}
	v, err := s.netSvc().GetVPC(s.project, vpcID)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	nats, err := s.ctrl.Store.VPC.ListNATGateways(v.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, nats, nil)
}

func (s *Server) handleCreateNATGateway(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		VPCID    string `json:"vpcId"`
		SubnetID string `json:"subnetId"`
		Name     string `json:"name"`
		PublicIP string `json:"publicIp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.VPCID == "" {
		writeBadRequest(w, fmt.Errorf("vpcId is required"))
		return
	}
	if req.SubnetID == "" {
		writeBadRequest(w, fmt.Errorf("subnetId is required"))
		return
	}
	v, err := s.netSvc().GetVPC(s.project, req.VPCID)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	sub, serr := s.ctrl.Store.VPC.GetSubnetByID(req.SubnetID)
	if serr != nil {
		writeBadRequest(w, fmt.Errorf("subnet not found: %w", serr))
		return
	}
	if sub.VPCID != v.ID {
		writeBadRequest(w, fmt.Errorf("subnet %s is not in vpc %s", req.SubnetID, v.ID))
		return
	}
	nat, err := s.ctrl.Store.VPC.CreateNATGateway(v.ID, req.SubnetID, req.Name, req.PublicIP)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: nat})
}

func (s *Server) handleGetNATGateway(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	nat, err := s.ctrl.Store.VPC.GetNATGateway(r.PathValue("natId"), vpcID)
	if err != nil {
		writeNotFound(w, "nat gateway not found")
		return
	}
	writeData(w, nat, nil)
}

func (s *Server) handleDeleteNATGateway(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	if err := s.ctrl.Store.VPC.DeleteNATGateway(r.PathValue("natId"), vpcID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Network ACLs -----------------------------------------------------------

func (s *Server) handleListNetworkACLs(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	if vpcID == "" {
		writeData(w, []vpc.NetworkACL{}, nil)
		return
	}
	v, err := s.netSvc().GetVPC(s.project, vpcID)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	acls, err := s.ctrl.Store.VPC.ListNetworkACLs(v.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, acls, nil)
}

func (s *Server) handleCreateNetworkACL(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		VPCID string `json:"vpcId"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	v, err := s.netSvc().GetVPC(s.project, req.VPCID)
	if err != nil {
		writeNotFound(w, "vpc not found")
		return
	}
	acl, err := s.ctrl.Store.VPC.CreateNetworkACL(v.ID, req.Name, false)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: acl})
}

func (s *Server) handleGetNetworkACL(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	acl, err := s.ctrl.Store.VPC.GetNetworkACL(r.PathValue("aclId"), vpcID)
	if err != nil {
		writeNotFound(w, "network acl not found")
		return
	}
	entries, _ := s.ctrl.Store.VPC.ListNetworkACLEntries(acl.ID)
	writeData(w, map[string]any{"networkAcl": acl, "entries": entries}, nil)
}

func (s *Server) handleDeleteNetworkACL(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vpcID := r.URL.Query().Get("vpcId")
	if err := s.ctrl.Store.VPC.DeleteNetworkACL(r.PathValue("aclId"), vpcID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddNetworkACLEntry(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		RuleNumber int    `json:"ruleNumber"`
		Direction  string `json:"direction"`
		Action     string `json:"action"`
		Protocol   string `json:"protocol"`
		CIDR       string `json:"cidr"`
		FromPort   int    `json:"fromPort"`
		ToPort     int    `json:"toPort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	e, err := s.ctrl.Store.VPC.AddNetworkACLEntry(r.PathValue("aclId"), req.Direction, req.Action, req.Protocol, req.CIDR, req.RuleNumber, req.FromPort, req.ToPort)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: e})
}

func (s *Server) handleDeleteNetworkACLEntry(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "vpc:update", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	ruleNum, _ := strconv.Atoi(r.PathValue("ruleNumber"))
	if err := s.ctrl.Store.VPC.DeleteNetworkACLEntry(r.PathValue("aclId"), ruleNum); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
