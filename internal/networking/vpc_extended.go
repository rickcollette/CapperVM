package networking

import (
	"fmt"

	"capper/internal/topology"
	"capper/internal/vpc"
)

// InitialSubnetInput is one subnet to create during VPC provisioning.
type InitialSubnetInput struct {
	Name               string
	Slug               string
	CIDR               string
	ZoneID             string
	SubnetType         vpc.SubnetKind
	AutoAssignPublicIP bool
}

// NATGatewayInput configures optional NAT creation during VPC create.
type NATGatewayInput struct {
	SubnetID   string
	SubnetCIDR string
	Name       string
	PublicIP   string
}

// CreateVPCInput is the API create contract for a VPC.
type CreateVPCInput struct {
	Project                string
	Name                   string
	Slug                   string
	CIDR                   string
	RealmID                string
	HomeRegionID           string
	Description            string
	DNSDomain              string
	DNSSupport             *bool
	DNSHostnames           *bool
	MobilityPolicy         string
	EnableFlowLogs         bool
	Labels                 map[string]string
	CreatedBy              string
	InitialSubnets         []InitialSubnetInput
	AttachInternetGateway  bool
	NATGateway             *NATGatewayInput
}

// SubnetPurpose filters subnets for dependent services.
type SubnetPurpose string

const (
	PurposeLaunch        SubnetPurpose = "launch"
	PurposeLB            SubnetPurpose = "lb"
	PurposeLBInternal    SubnetPurpose = "lb-internal"
	PurposeNAT           SubnetPurpose = "nat"
)

// KindsForPurpose returns subnet kinds appropriate for a UI picker purpose.
func KindsForPurpose(purpose SubnetPurpose) []vpc.SubnetKind {
	switch purpose {
	case PurposeLaunch:
		return []vpc.SubnetKind{vpc.SubnetPrivate, vpc.SubnetPublic}
	case PurposeLB:
		return []vpc.SubnetKind{vpc.SubnetPublic, vpc.SubnetEdge, vpc.SubnetLB}
	case PurposeLBInternal:
		return []vpc.SubnetKind{vpc.SubnetPrivate, vpc.SubnetService, vpc.SubnetLB}
	case PurposeNAT:
		return []vpc.SubnetKind{vpc.SubnetPublic, vpc.SubnetEdge}
	default:
		return nil
	}
}

// CreateVPC creates a VPC with optional bundled subnets and gateways.
func (s *Service) CreateVPC(in CreateVPCInput) (vpc.VPCDetail, error) {
	if in.Project == "" {
		return vpc.VPCDetail{}, fmt.Errorf("project is required")
	}
	if in.Name == "" {
		return vpc.VPCDetail{}, fmt.Errorf("name is required")
	}
	if in.CIDR == "" {
		return vpc.VPCDetail{}, fmt.Errorf("cidr is required")
	}
	if err := vpc.ValidateCIDR(in.CIDR); err != nil {
		return vpc.VPCDetail{}, err
	}
	slug := in.Slug
	if slug == "" {
		slug = slugify(in.Name)
	}
	dnsSupport := true
	dnsHostnames := true
	if in.DNSSupport != nil {
		dnsSupport = *in.DNSSupport
	}
	if in.DNSHostnames != nil {
		dnsHostnames = *in.DNSHostnames
	}
	mobility := in.MobilityPolicy
	if mobility == "" {
		mobility = "disabled"
	}

	v, err := s.vpc.CreateVPCExtended(vpc.CreateVPCOptions{
		Project:        in.Project,
		Name:           in.Name,
		Slug:           slug,
		CIDR:           in.CIDR,
		RealmID:        in.RealmID,
		HomeRegionID:   in.HomeRegionID,
		Description:    in.Description,
		DNSDomain:      in.DNSDomain,
		DNSSupport:     dnsSupport,
		DNSHostnames:   dnsHostnames,
		MobilityPolicy: mobility,
		EnableFlowLogs: in.EnableFlowLogs,
		Labels:         in.Labels,
		CreatedBy:      in.CreatedBy,
	})
	if err != nil {
		return vpc.VPCDetail{}, err
	}
	cleanup := func() {
		_ = s.vpc.DeleteVPC(v.ID, in.Project)
		if s.topology != nil {
			_ = s.topology.DeleteVPC(in.Project, v.Slug)
		}
	}

	if s.topology != nil {
		if err := s.topology.InsertVPC(topologyVPCFrom(v)); err != nil {
			cleanup()
			return vpc.VPCDetail{}, fmt.Errorf("topology vpc: %w", err)
		}
	}

	for _, subIn := range in.InitialSubnets {
		_, err := s.CreateSubnet(in.Project, CreateSubnetInput{
			VPCID:              v.ID,
			Name:               subIn.Name,
			Slug:               subIn.Slug,
			CIDR:               subIn.CIDR,
			ZoneID:             subIn.ZoneID,
			SubnetType:         subIn.SubnetType,
			AutoAssignPublicIP: subIn.AutoAssignPublicIP,
		})
		if err != nil {
			cleanup()
			return vpc.VPCDetail{}, fmt.Errorf("initial subnet %q: %w", subIn.Name, err)
		}
	}

	if in.AttachInternetGateway {
		igw, err := s.vpc.CreateIGW(v.ID, v.Name+"-igw")
		if err != nil {
			cleanup()
			return vpc.VPCDetail{}, fmt.Errorf("internet gateway: %w", err)
		}
		if v.MainRouteTableID != "" {
			_, _ = s.vpc.AddRoute(v.MainRouteTableID, "0.0.0.0/0", "internet-gateway", igw.ID)
		}
	}

	if in.NATGateway != nil {
		subnetID := in.NATGateway.SubnetID
		if subnetID == "" && in.NATGateway.SubnetCIDR != "" {
			subs, err := s.vpc.ListSubnets(v.ID)
			if err == nil {
				for _, sub := range subs {
					if sub.CIDR == in.NATGateway.SubnetCIDR {
						subnetID = sub.ID
						break
					}
				}
			}
		}
		if subnetID != "" {
			name := in.NATGateway.Name
			if name == "" {
				name = v.Name + "-nat"
			}
			if _, err := s.vpc.CreateNATGateway(v.ID, subnetID, name, in.NATGateway.PublicIP); err != nil {
				cleanup()
				return vpc.VPCDetail{}, fmt.Errorf("nat gateway: %w", err)
			}
		}
	}

	return s.VPCDetail(in.Project, v.ID)
}

func topologyVPCFrom(v vpc.VPC) topology.VPC {
	return topology.VPC{
		ID:             v.ID,
		RealmID:        v.RealmID,
		Project:        v.Project,
		Slug:           v.Slug,
		Name:           v.Name,
		CIDR:           v.CIDR,
		Status:         v.Status,
		HomeRegionID:   v.HomeRegionID,
		MobilityPolicy: v.MobilityPolicy,
		Labels:         v.Labels,
		CreatedAt:      v.CreatedAt,
		UpdatedAt:      v.UpdatedAt,
	}
}

// VPCDetail returns the aggregate VPC view for detail pages.
func (s *Service) VPCDetail(project, ref string) (vpc.VPCDetail, error) {
	v, err := s.vpc.GetVPC(ref, project)
	if err != nil {
		return vpc.VPCDetail{}, err
	}
	subs, _ := s.vpc.ListSubnets(v.ID)
	rts, _ := s.vpc.ListRouteTables(v.ID)
	var rtDetails []vpc.RouteTableDetail
	for _, rt := range rts {
		routes, _ := s.vpc.ListRoutes(rt.ID)
		rtDetails = append(rtDetails, vpc.RouteTableDetail{RouteTable: rt, Routes: routes})
	}
	sgs, _ := s.vpc.ListSecurityGroups(v.ID)
	var sgDetails []vpc.SecurityGroupDetail
	for _, sg := range sgs {
		rules, _ := s.vpc.ListSGRules(sg.ID)
		sgDetails = append(sgDetails, vpc.SecurityGroupDetail{SecurityGroup: sg, Rules: rules})
	}
	acls, _ := s.vpc.ListNetworkACLs(v.ID)
	var aclDetails []vpc.NetworkACLDetail
	for _, acl := range acls {
		entries, _ := s.vpc.ListNetworkACLEntries(acl.ID)
		aclDetails = append(aclDetails, vpc.NetworkACLDetail{NetworkACL: acl, Entries: entries})
	}
	igws, _ := s.vpc.ListIGWs(v.ID)
	nats, _ := s.vpc.ListNATGateways(v.ID)
	deps, _ := s.VPCDependencies(project, v.ID)
	return vpc.VPCDetail{
		VPC:              v,
		Subnets:          subs,
		RouteTables:      rtDetails,
		SecurityGroups:   sgDetails,
		NetworkACLs:      aclDetails,
		InternetGateways: igws,
		NATGateways:      nats,
		Dependencies:     deps,
	}, nil
}

// VPCDependencies lists blockers for VPC delete.
func (s *Service) VPCDependencies(project, ref string) (vpc.VPCDependencies, error) {
	v, err := s.vpc.GetVPC(ref, project)
	if err != nil {
		return vpc.VPCDependencies{}, err
	}
	store := s.vpc.Store()
	deps := vpc.VPCDependencies{VPCID: v.ID}

	subs, _ := s.vpc.ListSubnets(v.ID)
	for _, sub := range subs {
		deps.Subnets = append(deps.Subnets, sub.ID)
	}
	deps.ENIs, _ = store.ListENIIDsInVPC(v.ID)
	deps.Instances, _ = store.ListInstanceIDsFromENIs(v.ID)
	deps.LoadBalancers, _ = store.ListLBNamesInVPC(v.ID)
	nats, _ := s.vpc.ListNATGateways(v.ID)
	for _, n := range nats {
		deps.NATGateways = append(deps.NATGateways, n.ID)
	}
	rts, _ := s.vpc.ListRouteTables(v.ID)
	for _, rt := range rts {
		deps.RouteTables = append(deps.RouteTables, rt.ID)
	}
	deps.DNSZones, _ = store.ListDNSZonesForVPC(v.ID)

	deps.Blocked = len(deps.Instances) > 0 || len(deps.ENIs) > 0 ||
		len(deps.LoadBalancers) > 0 || len(deps.NATGateways) > 0 || len(deps.DNSZones) > 0
	return deps, nil
}

// SubnetDependencies lists blockers for subnet delete.
func (s *Service) SubnetDependencies(subnetID string) (vpc.SubnetDependencies, error) {
	sub, err := s.vpc.GetSubnetByID(subnetID)
	if err != nil {
		return vpc.SubnetDependencies{}, err
	}
	store := s.vpc.Store()
	deps := vpc.SubnetDependencies{SubnetID: sub.ID}
	deps.ENIs, _ = store.ListENIIDsInSubnet(sub.ID)
	deps.Instances, _ = store.ListInstanceIDsInSubnet(sub.ID)
	deps.LoadBalancers, _ = store.ListLBNamesInSubnet(sub.ID)
	nats, _ := s.vpc.ListNATGateways(sub.VPCID)
	for _, n := range nats {
		if n.SubnetID == sub.ID {
			deps.NATGateways = append(deps.NATGateways, n.ID)
		}
	}
	deps.Blocked = len(deps.ENIs) > 0 || len(deps.Instances) > 0 ||
		len(deps.LoadBalancers) > 0 || len(deps.NATGateways) > 0
	return deps, nil
}

// ListSubnetsForPurpose returns subnets filtered by kind for UI pickers.
func (s *Service) ListSubnetsForPurpose(project, vpcRef string, purpose SubnetPurpose) ([]vpc.Subnet, error) {
	v, err := s.vpc.GetVPC(vpcRef, project)
	if err != nil {
		return nil, err
	}
	subs, err := s.vpc.ListSubnets(v.ID)
	if err != nil {
		return nil, err
	}
	kinds := KindsForPurpose(purpose)
	if len(kinds) == 0 {
		return subs, nil
	}
	allowed := make(map[vpc.SubnetKind]bool, len(kinds))
	for _, k := range kinds {
		allowed[k] = true
	}
	var out []vpc.Subnet
	for _, sub := range subs {
		k := sub.Kind
		if k == "" {
			k = sub.SubnetType
		}
		if allowed[k] {
			out = append(out, sub)
		}
	}
	return out, nil
}

// DeleteVPC removes a VPC if no blockers exist.
func (s *Service) DeleteVPC(project, ref string) error {
	deps, err := s.VPCDependencies(project, ref)
	if err != nil {
		return err
	}
	if deps.Blocked {
		return fmt.Errorf("vpc has dependencies: instances=%v enis=%v loadBalancers=%v nat=%v dns=%v",
			deps.Instances, deps.ENIs, deps.LoadBalancers, deps.NATGateways, deps.DNSZones)
	}
	v, err := s.vpc.GetVPC(ref, project)
	if err != nil {
		return err
	}
	if err := s.vpc.DeleteVPC(ref, project); err != nil {
		return err
	}
	if s.topology != nil {
		_ = s.topology.DeleteVPC(project, v.Slug)
	}
	return nil
}

// DeleteSubnet removes a subnet if no blockers exist.
func (s *Service) DeleteSubnet(subnetID string) error {
	deps, err := s.SubnetDependencies(subnetID)
	if err != nil {
		return err
	}
	if deps.Blocked {
		return fmt.Errorf("subnet has dependencies: instances=%v enis=%v loadBalancers=%v nat=%v",
			deps.Instances, deps.ENIs, deps.LoadBalancers, deps.NATGateways)
	}
	sub, err := s.vpc.GetSubnetByID(subnetID)
	if err != nil {
		return err
	}
	return s.vpc.DeleteSubnet(sub.ID, sub.VPCID)
}
