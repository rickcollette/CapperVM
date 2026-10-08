package networking

import (
	"database/sql"
	"fmt"
	"strings"

	"capper/internal/topology"
	"capper/internal/vpc"
)

// Service is the unified networking control-plane facade over canonical VPC resources.
type Service struct {
	vpc      *vpc.Manager
	topology *topology.Store
}

// NewService returns a networking service.
func NewService(db *sql.DB, topo *topology.Store) *Service {
	return &Service{
		vpc:      vpc.NewManager(db),
		topology: topo,
	}
}

// VPC returns the underlying VPC manager.
func (s *Service) VPC() *vpc.Manager { return s.vpc }

// GetVPC resolves a VPC by id, name, or slug within a project.
func (s *Service) GetVPC(project, ref string) (vpc.VPC, error) {
	return s.vpc.GetVPC(ref, project)
}

// ListVPCs lists VPCs in a project.
func (s *Service) ListVPCs(project string) ([]vpc.VPC, error) {
	return s.vpc.ListVPCs(project)
}

// VPCPatch carries optional VPC field updates. Bool pointers allow turning flags off.
type VPCPatch struct {
	Name           string
	Description    string
	MobilityPolicy string
	Labels         map[string]string
	DNSDomain      string
	DNSSupport     *bool
	DNSHostnames   *bool
	EnableFlowLogs *bool
}

// UpdateVPC patches mutable VPC fields.
func (s *Service) UpdateVPC(project, ref string, patch VPCPatch) (vpc.VPC, error) {
	cur, err := s.vpc.GetVPC(ref, project)
	if err != nil {
		return vpc.VPC{}, err
	}
	if patch.Name != "" {
		cur.Name = patch.Name
	}
	if patch.Description != "" {
		cur.Description = patch.Description
	}
	if patch.MobilityPolicy != "" {
		cur.MobilityPolicy = patch.MobilityPolicy
	}
	if patch.Labels != nil {
		cur.Labels = patch.Labels
	}
	if patch.DNSDomain != "" {
		cur.DNSDomain = patch.DNSDomain
	}
	if patch.DNSSupport != nil {
		cur.DNSSupport = *patch.DNSSupport
	}
	if patch.DNSHostnames != nil {
		cur.DNSHostnames = *patch.DNSHostnames
	}
	if patch.EnableFlowLogs != nil {
		cur.EnableFlowLogs = *patch.EnableFlowLogs
	}
	updated, err := s.vpc.UpdateVPC(cur)
	if err != nil {
		return vpc.VPC{}, err
	}
	if s.topology != nil {
		_ = s.topology.UpdateVPC(topology.VPC{
			ID:             updated.ID,
			RealmID:        updated.RealmID,
			Project:        updated.Project,
			Slug:           updated.Slug,
			Name:           updated.Name,
			CIDR:           updated.CIDR,
			Status:         updated.Status,
			HomeRegionID:   updated.HomeRegionID,
			MobilityPolicy: updated.MobilityPolicy,
			Labels:         updated.Labels,
			UpdatedAt:      updated.UpdatedAt,
		})
	}
	return updated, nil
}

// VPCSummary returns dashboard counts for a VPC.
func (s *Service) VPCSummary(project, ref string) (vpc.VPCSummary, error) {
	v, err := s.vpc.GetVPC(ref, project)
	if err != nil {
		return vpc.VPCSummary{}, err
	}
	subs, _ := s.vpc.ListSubnets(v.ID)
	rts, _ := s.vpc.ListRouteTables(v.ID)
	sgs, _ := s.vpc.ListSecurityGroups(v.ID)
	acls, _ := s.vpc.ListNetworkACLs(v.ID)
	igws, _ := s.vpc.ListIGWs(v.ID)
	nats, _ := s.vpc.ListNATGateways(v.ID)
	return vpc.VPCSummary{
		VPC:                v,
		SubnetCount:        len(subs),
		RouteTableCount:    len(rts),
		SecurityGroupCount: len(sgs),
		NetworkACLCount:    len(acls),
		IGWCount:           len(igws),
		NATCount:           len(nats),
	}, nil
}

// CreateSubnetInput is the subnet create contract.
type CreateSubnetInput struct {
	VPCID              string
	Name               string
	Slug               string
	CIDR               string
	RegionID           string
	ZoneID             string
	SubnetType         vpc.SubnetKind
	AutoAssignPublicIP bool
	RouteTableID       string
	NetworkACLID       string
}

// CreateSubnet creates a subnet with CIDR validation against the VPC.
func (s *Service) CreateSubnet(project string, in CreateSubnetInput) (vpc.Subnet, error) {
	v, err := s.vpc.GetVPC(in.VPCID, project)
	if err != nil {
		return vpc.Subnet{}, err
	}
	if err := vpc.ValidateCIDR(in.CIDR); err != nil {
		return vpc.Subnet{}, err
	}
	ok, err := vpc.ContainsCIDR(v.CIDR, in.CIDR)
	if err != nil {
		return vpc.Subnet{}, err
	}
	if !ok {
		return vpc.Subnet{}, fmt.Errorf("subnet cidr %s is not contained in vpc cidr %s", in.CIDR, v.CIDR)
	}
	existing, _ := s.vpc.ListSubnets(v.ID)
	for _, sub := range existing {
		overlap, err := vpc.OverlapCIDR(sub.CIDR, in.CIDR)
		if err != nil {
			return vpc.Subnet{}, err
		}
		if overlap {
			return vpc.Subnet{}, fmt.Errorf("subnet cidr overlaps existing subnet %s", sub.Name)
		}
	}
	kind := in.SubnetType
	if kind == "" {
		kind = vpc.SubnetPrivate
	}
	return s.vpc.CreateSubnetExtended(vpc.CreateSubnetOptions{
		VPCID:              v.ID,
		RealmID:            v.RealmID,
		Name:               in.Name,
		Slug:               in.Slug,
		CIDR:               in.CIDR,
		RegionID:           in.RegionID,
		ZoneID:             in.ZoneID,
		Kind:               kind,
		AutoAssignPublicIP: in.AutoAssignPublicIP,
		RouteTableID:       in.RouteTableID,
		NetworkACLID:       in.NetworkACLID,
	})
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}
