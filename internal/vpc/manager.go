package vpc

import (
	"database/sql"
	"fmt"
)

// Manager provides business-logic operations for all VPC resources.
type Manager struct {
	store *Store
}

// NewManager returns a Manager backed by db.
func NewManager(db *sql.DB) *Manager {
	return &Manager{store: NewStore(db)}
}

// Store exposes the underlying store for testing.
func (m *Manager) Store() *Store { return m.store }

// ---- VPC operations ---------------------------------------------------------

// CreateVPC creates a new VPC.
func (m *Manager) CreateVPC(project, name, cidr, dnsDomain string) (VPC, error) {
	if project == "" {
		return VPC{}, fmt.Errorf("project is required")
	}
	if name == "" {
		return VPC{}, fmt.Errorf("name is required")
	}
	if cidr == "" {
		return VPC{}, fmt.Errorf("cidr is required")
	}
	v := VPC{
		ID:        newID("vpc"),
		Project:   project,
		Name:      name,
		CIDR:      cidr,
		DNSDomain: dnsDomain,
		CreatedAt: now(),
	}
	if err := m.store.InsertVPC(v); err != nil {
		return VPC{}, fmt.Errorf("create vpc: %w", err)
	}
	return v, nil
}

// GetVPC retrieves a VPC by name or ID within a project.
func (m *Manager) GetVPC(nameOrID, project string) (VPC, error) {
	return m.store.GetVPC(nameOrID, project)
}

// ListVPCs returns all VPCs in a project.
func (m *Manager) ListVPCs(project string) ([]VPC, error) {
	return m.store.ListVPCs(project)
}

// DeleteVPC deletes a VPC and all its dependents (cascaded by SQLite FK).
func (m *Manager) DeleteVPC(nameOrID, project string) error {
	return m.store.DeleteVPC(nameOrID, project)
}

// ---- Subnet operations ------------------------------------------------------

// CreateSubnet creates a subnet inside a VPC.
func (m *Manager) CreateSubnet(vpcID, name, cidr, zone string, kind SubnetKind) (Subnet, error) {
	if vpcID == "" {
		return Subnet{}, fmt.Errorf("vpcID is required")
	}
	if name == "" {
		return Subnet{}, fmt.Errorf("name is required")
	}
	if kind == "" {
		kind = SubnetPrivate
	}
	sub := Subnet{
		ID:        newID("sub"),
		VPCID:     vpcID,
		Name:      name,
		CIDR:      cidr,
		Zone:      zone,
		Kind:      kind,
		CreatedAt: now(),
	}
	if err := m.store.InsertSubnet(sub); err != nil {
		return Subnet{}, fmt.Errorf("create subnet: %w", err)
	}
	return sub, nil
}

// GetSubnet retrieves a subnet by name or ID within a VPC.
func (m *Manager) GetSubnet(nameOrID, vpcID string) (Subnet, error) {
	return m.store.GetSubnet(nameOrID, vpcID)
}

// ListSubnets returns all subnets in a VPC.
func (m *Manager) ListSubnets(vpcID string) ([]Subnet, error) {
	return m.store.ListSubnets(vpcID)
}

// DeleteSubnet deletes a subnet.
func (m *Manager) DeleteSubnet(nameOrID, vpcID string) error {
	return m.store.DeleteSubnet(nameOrID, vpcID)
}

// ---- RouteTable operations --------------------------------------------------

// CreateRouteTable creates a route table inside a VPC.
func (m *Manager) CreateRouteTable(vpcID, name string) (RouteTable, error) {
	return m.createRouteTable(vpcID, name, false)
}

// CreateRouteTableMain creates the main route table for a VPC.
func (m *Manager) CreateRouteTableMain(vpcID, name string) (RouteTable, error) {
	return m.createRouteTable(vpcID, name, true)
}

func (m *Manager) createRouteTable(vpcID, name string, isMain bool) (RouteTable, error) {
	rt := RouteTable{
		ID:        newID("rtb"),
		VPCID:     vpcID,
		Name:      name,
		IsMain:    isMain,
		CreatedAt: now(),
	}
	if err := m.store.InsertRouteTable(rt); err != nil {
		return RouteTable{}, fmt.Errorf("create route table: %w", err)
	}
	return rt, nil
}

// GetRouteTable retrieves a route table by name or ID.
func (m *Manager) GetRouteTable(nameOrID, vpcID string) (RouteTable, error) {
	return m.store.GetRouteTable(nameOrID, vpcID)
}

// ListRouteTables returns all route tables in a VPC.
func (m *Manager) ListRouteTables(vpcID string) ([]RouteTable, error) {
	return m.store.ListRouteTables(vpcID)
}

// DeleteRouteTable deletes a route table.
func (m *Manager) DeleteRouteTable(nameOrID, vpcID string) error {
	return m.store.DeleteRouteTable(nameOrID, vpcID)
}

// AddRoute adds a route to a route table.
func (m *Manager) AddRoute(routeTableID, destCIDR, targetType, targetID string) (Route, error) {
	r := Route{
		ID:              newID("rt"),
		RouteTableID:    routeTableID,
		DestinationCIDR: destCIDR,
		TargetType:      targetType,
		TargetID:        targetID,
	}
	if err := m.store.InsertRoute(r); err != nil {
		return Route{}, fmt.Errorf("add route: %w", err)
	}
	return r, nil
}

// ListRoutes returns all routes in a route table.
func (m *Manager) ListRoutes(routeTableID string) ([]Route, error) {
	return m.store.ListRoutes(routeTableID)
}

// DeleteRoute deletes a route by ID.
func (m *Manager) DeleteRoute(id string) error {
	return m.store.DeleteRoute(id)
}

// AssociateSubnet associates a subnet with a route table.
func (m *Manager) AssociateSubnet(subnetID, routeTableID string) error {
	return m.store.AssociateSubnetRouteTable(subnetID, routeTableID)
}

// ---- SecurityGroup operations -----------------------------------------------

// CreateSecurityGroup creates a security group within a VPC.
func (m *Manager) CreateSecurityGroup(vpcID, name, description string, defaultDeny bool) (SecurityGroup, error) {
	sg := SecurityGroup{
		ID:          newID("sg"),
		VPCID:       vpcID,
		Name:        name,
		Description: description,
		DefaultDeny: defaultDeny,
		CreatedAt:   now(),
	}
	if err := m.store.InsertSecurityGroup(sg); err != nil {
		return SecurityGroup{}, fmt.Errorf("create security group: %w", err)
	}
	return sg, nil
}

// GetSecurityGroupByID retrieves a security group by its ID.
func (m *Manager) GetSecurityGroupByID(id string) (SecurityGroup, error) {
	return m.store.GetSecurityGroupByID(id)
}

// GetSecurityGroup retrieves a security group by name or ID.
func (m *Manager) GetSecurityGroup(nameOrID, vpcID string) (SecurityGroup, error) {
	return m.store.GetSecurityGroup(nameOrID, vpcID)
}

// ListSecurityGroups returns all security groups in a VPC.
func (m *Manager) ListSecurityGroups(vpcID string) ([]SecurityGroup, error) {
	return m.store.ListSecurityGroups(vpcID)
}

// DeleteSecurityGroup deletes a security group.
func (m *Manager) DeleteSecurityGroup(nameOrID, vpcID string) error {
	return m.store.DeleteSecurityGroup(nameOrID, vpcID)
}

// AddSGRule adds an ingress or egress rule to a security group.
func (m *Manager) AddSGRule(sgID string, dir SGRuleDirection, proto, cidr string, fromPort, toPort int, action string) (SecurityGroupRule, error) {
	if action == "" {
		action = "allow"
	}
	rule := SecurityGroupRule{
		ID:              newID("sgr"),
		SecurityGroupID: sgID,
		Direction:       dir,
		Protocol:        proto,
		FromPort:        fromPort,
		ToPort:          toPort,
		CIDR:            cidr,
		Action:          action,
	}
	if err := m.store.InsertSGRule(rule); err != nil {
		return SecurityGroupRule{}, fmt.Errorf("add sg rule: %w", err)
	}
	return rule, nil
}

// ListSGRules returns all rules in a security group.
func (m *Manager) ListSGRules(sgID string) ([]SecurityGroupRule, error) {
	return m.store.ListSGRules(sgID)
}

// DeleteSGRule removes a rule by ID.
func (m *Manager) DeleteSGRule(id string) error {
	return m.store.DeleteSGRule(id)
}

// ---- InternetGateway operations ---------------------------------------------

// CreateIGW creates an internet gateway for a VPC.
func (m *Manager) CreateIGW(vpcID, name string) (InternetGateway, error) {
	igw := InternetGateway{
		ID:        newID("igw"),
		VPCID:     vpcID,
		Name:      name,
		CreatedAt: now(),
	}
	if err := m.store.InsertIGW(igw); err != nil {
		return InternetGateway{}, fmt.Errorf("create igw: %w", err)
	}
	return igw, nil
}

// GetIGW retrieves an internet gateway by name or ID.
func (m *Manager) GetIGW(nameOrID, vpcID string) (InternetGateway, error) {
	return m.store.GetIGW(nameOrID, vpcID)
}

// ListIGWs returns all internet gateways attached to a VPC.
func (m *Manager) ListIGWs(vpcID string) ([]InternetGateway, error) {
	return m.store.ListIGWs(vpcID)
}

// DeleteIGW detaches and deletes an internet gateway.
func (m *Manager) DeleteIGW(nameOrID, vpcID string) error {
	return m.store.DeleteIGW(nameOrID, vpcID)
}

// ---- NATGateway operations --------------------------------------------------

// CreateNATGateway creates a NAT gateway in a subnet.
func (m *Manager) CreateNATGateway(vpcID, subnetID, name, publicIP string) (NATGateway, error) {
	nat := NATGateway{
		ID:        newID("nat"),
		VPCID:     vpcID,
		SubnetID:  subnetID,
		Name:      name,
		PublicIP:  publicIP,
		CreatedAt: now(),
	}
	if err := m.store.InsertNATGateway(nat); err != nil {
		return NATGateway{}, fmt.Errorf("create nat gateway: %w", err)
	}
	return nat, nil
}

// GetNATGateway retrieves a NAT gateway by name or ID.
func (m *Manager) GetNATGateway(nameOrID, vpcID string) (NATGateway, error) {
	return m.store.GetNATGateway(nameOrID, vpcID)
}

// ListNATGateways returns all NAT gateways in a VPC.
func (m *Manager) ListNATGateways(vpcID string) ([]NATGateway, error) {
	return m.store.ListNATGateways(vpcID)
}

// DeleteNATGateway deletes a NAT gateway.
func (m *Manager) DeleteNATGateway(nameOrID, vpcID string) error {
	return m.store.DeleteNATGateway(nameOrID, vpcID)
}
