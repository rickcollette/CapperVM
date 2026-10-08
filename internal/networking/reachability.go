package networking

import (
	"fmt"
	"sort"
	"strings"

	"capper/internal/vpc"
)

// ReachabilityRequest is input to the path analyzer.
type ReachabilityRequest struct {
	SourceType      string `json:"sourceType"`
	SourceID        string `json:"sourceId"`
	DestinationType string `json:"destinationType"`
	DestinationID   string `json:"destinationId"`
	Protocol        string `json:"protocol"`
	Port            int    `json:"port"`
}

// ReachabilityResult is the analyzer output.
type ReachabilityResult struct {
	Allowed      bool     `json:"allowed"`
	BlockingRule string   `json:"blockingRule,omitempty"`
	Path         []string `json:"path,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

// AnalyzeReachability performs a control-plane reachability check.
func AnalyzeReachability(req ReachabilityRequest) ReachabilityResult {
	if req.SourceType == "" || req.DestinationType == "" {
		return ReachabilityResult{Allowed: false, BlockingRule: "missing source or destination"}
	}
	return ReachabilityResult{
		Allowed:      false,
		BlockingRule: "insufficient security-group data for allow decision",
		Path: []string{
			fmt.Sprintf("%s:%s", req.SourceType, req.SourceID),
			"route-table:evaluated",
			"security-group:indeterminate",
			fmt.Sprintf("%s:%s", req.DestinationType, req.DestinationID),
		},
		Warnings: []string{"reachability requires security group evaluation; use AnalyzeReachabilityWithVPC"},
	}
}

// AnalyzeReachabilityWithVPC walks SG rules when source is an instance with security groups.
func AnalyzeReachabilityWithVPC(req ReachabilityRequest, vpcMgr *vpc.Manager, instSGs []string, port int) ReachabilityResult {
	if req.SourceType == "" || req.DestinationType == "" {
		return ReachabilityResult{Allowed: false, BlockingRule: "missing source or destination"}
	}
	path := []string{
		fmt.Sprintf("%s:%s", req.SourceType, req.SourceID),
		"route-table:local",
	}
	proto := strings.ToLower(req.Protocol)
	if proto == "" {
		proto = "tcp"
	}
	if port == 0 {
		port = req.Port
	}
	if len(instSGs) == 0 {
		return ReachabilityResult{
			Allowed:      false,
			BlockingRule: "no security groups attached; deny by default",
			Path:         path,
		}
	}
	for _, sgID := range instSGs {
		path = append(path, fmt.Sprintf("security-group:%s", sgID))
		rules, err := vpcMgr.ListSGRules(sgID)
		if err != nil {
			continue
		}
		for _, r := range rules {
			if r.Direction != vpc.SGIngress {
				continue
			}
			if r.Protocol != "all" && r.Protocol != proto && r.Protocol != "-1" {
				continue
			}
			if port > 0 && (port < r.FromPort || port > r.ToPort) && r.FromPort != 0 {
				continue
			}
			if r.Action == "allow" {
				allowed, blocking, evaluated := evaluateNetworkACL(vpcMgr, sgID, req, proto, port)
				if !evaluated {
					return ReachabilityResult{
						Allowed:      false,
						BlockingRule: blocking,
						Path:         append(path, "network-acl:indeterminate"),
					}
				}
				if !allowed {
					return ReachabilityResult{
						Allowed:      false,
						BlockingRule: blocking,
						Path:         append(path, "network-acl:deny"),
					}
				}
				path = append(path, "network-acl:allow", fmt.Sprintf("%s:%s", req.DestinationType, req.DestinationID))
				return ReachabilityResult{Allowed: true, Path: path}
			}
		}
	}
	return ReachabilityResult{
		Allowed:      false,
		BlockingRule: fmt.Sprintf("no attached security group allows %s/%d", proto, port),
		Path:         path,
	}
}

// evaluateNetworkACL applies the VPC network ACL that governs this flow.
// A security-group allow is not enough: a matching deny entry blocks the path,
// and a missing ACL is reported as indeterminate rather than allowed.
func evaluateNetworkACL(vpcMgr *vpc.Manager, sgID string, req ReachabilityRequest, proto string, port int) (allowed bool, blocking string, evaluated bool) {
	if vpcMgr == nil {
		return false, "network ACL not evaluated", false
	}
	sg, err := vpcMgr.GetSecurityGroupByID(sgID)
	if err != nil {
		return false, "network ACL not evaluated", false
	}
	aclID := ""
	if req.DestinationType == "subnet" && req.DestinationID != "" {
		if sub, subErr := vpcMgr.GetSubnetByID(req.DestinationID); subErr == nil && sub.NetworkACLID != "" {
			aclID = sub.NetworkACLID
		}
	}
	if aclID == "" {
		v, vpcErr := vpcMgr.GetVPC(sg.VPCID, "")
		if vpcErr != nil || v.DefaultNetworkACLID == "" {
			return false, "network ACL not evaluated", false
		}
		aclID = v.DefaultNetworkACLID
	}
	entries, err := vpcMgr.ListNetworkACLEntries(aclID)
	if err != nil {
		return false, "network ACL not evaluated", false
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].RuleNumber < entries[j].RuleNumber })
	for _, e := range entries {
		if e.Direction != "" && e.Direction != "ingress" {
			continue
		}
		p := strings.ToLower(e.Protocol)
		if p != "all" && p != "-1" && p != proto && p != "" {
			continue
		}
		if port > 0 && e.FromPort != 0 && (port < e.FromPort || port > e.ToPort) {
			continue
		}
		if strings.EqualFold(e.Action, "deny") {
			return false, fmt.Sprintf("network ACL %s rule %d denies %s/%d", aclID, e.RuleNumber, proto, port), true
		}
		if strings.EqualFold(e.Action, "allow") {
			return true, "", true
		}
	}
	return false, fmt.Sprintf("network ACL %s has no matching allow for %s/%d", aclID, proto, port), true
}

// TopologyGraph is the networking topology API response.
type TopologyGraph struct {
	VPCs  []vpc.VPC   `json:"vpcs"`
	Edges []GraphEdge `json:"edges"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// BuildTopologyGraph constructs VPC → subnet → gateway edges.
func BuildTopologyGraph(svc *Service, project string) (TopologyGraph, error) {
	vpcs, err := svc.ListVPCs(project)
	if err != nil {
		return TopologyGraph{}, err
	}
	var edges []GraphEdge
	for _, v := range vpcs {
		subs, _ := svc.VPC().ListSubnets(v.ID)
		for _, sub := range subs {
			edges = append(edges, GraphEdge{From: v.ID, To: sub.ID, Kind: "subnet"})
			if sub.RouteTableID != "" {
				edges = append(edges, GraphEdge{From: sub.ID, To: sub.RouteTableID, Kind: "route-table"})
			}
		}
		igws, _ := svc.VPC().ListIGWs(v.ID)
		for _, igw := range igws {
			edges = append(edges, GraphEdge{From: v.ID, To: igw.ID, Kind: "internet-gateway"})
		}
		nats, _ := svc.VPC().ListNATGateways(v.ID)
		for _, nat := range nats {
			edges = append(edges, GraphEdge{From: v.ID, To: nat.ID, Kind: "nat-gateway"})
		}
	}
	return TopologyGraph{VPCs: vpcs, Edges: edges}, nil
}
