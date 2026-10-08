package vpc

import "fmt"

// CreateVPCOptions holds extended VPC creation parameters.
type CreateVPCOptions struct {
	Project        string
	Name           string
	Slug           string
	CIDR           string
	RealmID        string
	HomeRegionID   string
	Description    string
	DNSDomain      string
	DNSSupport     bool
	DNSHostnames   bool
	MobilityPolicy string
	EnableFlowLogs bool
	Labels         map[string]string
	CreatedBy      string
}

// CreateSubnetOptions holds extended subnet creation parameters.
type CreateSubnetOptions struct {
	VPCID              string
	RealmID            string
	Name               string
	Slug               string
	CIDR               string
	RegionID           string
	ZoneID             string
	Kind               SubnetKind
	AutoAssignPublicIP bool
	RouteTableID       string
	NetworkACLID       string
}

func (m *Manager) assertVPCNameAvailable(project, name, slug string) error {
	if name != "" {
		if _, err := m.store.GetVPC(name, project); err == nil {
			return fmt.Errorf("%w: %q", ErrVPCNameTaken, name)
		}
	}
	if slug != "" && slug != name {
		if _, err := m.store.GetVPC(slug, project); err == nil {
			return fmt.Errorf("%w: slug %q", ErrVPCNameTaken, slug)
		}
	}
	return nil
}

// CreateVPCExtended creates a VPC with default networking resources.
func (m *Manager) CreateVPCExtended(opts CreateVPCOptions) (VPC, error) {
	if opts.Project == "" || opts.Name == "" || opts.CIDR == "" {
		return VPC{}, fmt.Errorf("project, name, and cidr are required")
	}
	if err := ValidateCIDR(opts.CIDR); err != nil {
		return VPC{}, err
	}
	if err := m.assertVPCNameAvailable(opts.Project, opts.Name, opts.Slug); err != nil {
		return VPC{}, err
	}
	ts := now()
	v := VPC{
		ID:              newID("vpc"),
		Project:         opts.Project,
		Name:            opts.Name,
		Slug:            opts.Slug,
		CIDR:            opts.CIDR,
		PrimaryIPv4CIDR: opts.CIDR,
		RealmID:         opts.RealmID,
		HomeRegionID:    opts.HomeRegionID,
		Description:     opts.Description,
		DNSDomain:       opts.DNSDomain,
		DNSSupport:      opts.DNSSupport,
		DNSHostnames:    opts.DNSHostnames,
		MobilityPolicy:  opts.MobilityPolicy,
		EnableFlowLogs:  opts.EnableFlowLogs,
		Labels:          opts.Labels,
		CreatedBy:       opts.CreatedBy,
		Status:          VPCStatusAvailable,
		CreatedAt:       ts,
		UpdatedAt:       ts,
	}
	if err := m.store.InsertVPC(v); err != nil {
		if IsConstraintError(err) {
			return VPC{}, fmt.Errorf("%w: %q", ErrVPCNameTaken, opts.Name)
		}
		return VPC{}, fmt.Errorf("create vpc: %w", err)
	}
	rollback := func() { _ = m.store.DeleteVPC(v.ID, opts.Project) }

	rt, err := m.CreateRouteTableMain(v.ID, "main")
	if err != nil {
		rollback()
		return VPC{}, err
	}
	_, _ = m.AddRoute(rt.ID, v.CIDR, "local", "")
	v.MainRouteTableID = rt.ID

	sg, err := m.CreateSecurityGroup(v.ID, "default", "default VPC security group", true)
	if err != nil {
		rollback()
		return VPC{}, err
	}
	sg.IsDefault = true
	v.DefaultSecurityGroupID = sg.ID
	_, _ = m.AddSGRule(sg.ID, SGEgress, "all", "0.0.0.0/0", 0, 0, "allow")

	acl, err := m.CreateNetworkACL(v.ID, "default", true)
	if err != nil {
		rollback()
		return VPC{}, err
	}
	v.DefaultNetworkACLID = acl.ID

	v.UpdatedAt = now()
	if err := m.store.UpdateVPC(v); err != nil {
		rollback()
		return VPC{}, err
	}
	return v, nil
}

// UpdateVPC persists VPC field changes.
func (m *Manager) UpdateVPC(v VPC) (VPC, error) {
	v.UpdatedAt = now()
	if err := m.store.UpdateVPC(v); err != nil {
		return VPC{}, err
	}
	return m.store.GetVPC(v.ID, v.Project)
}

// CreateSubnetExtended creates a subnet with extended metadata.
func (m *Manager) CreateSubnetExtended(opts CreateSubnetOptions) (Subnet, error) {
	if opts.VPCID == "" || opts.Name == "" {
		return Subnet{}, fmt.Errorf("vpcId and name are required")
	}
	kind := opts.Kind
	if kind == "" {
		kind = SubnetPrivate
	}
	ts := now()
	slug := opts.Slug
	if slug == "" {
		slug = opts.Name
	}
	rtID := opts.RouteTableID
	aclID := opts.NetworkACLID
	if rtID == "" {
		v, err := m.GetVPC(opts.VPCID, "")
		if err == nil && v.MainRouteTableID != "" {
			rtID = v.MainRouteTableID
		}
	}
	if aclID == "" {
		v, err := m.GetVPC(opts.VPCID, "")
		if err == nil && v.DefaultNetworkACLID != "" {
			aclID = v.DefaultNetworkACLID
		}
	}
	sub := Subnet{
		ID:                 newID("subnet"),
		VPCID:              opts.VPCID,
		RealmID:            opts.RealmID,
		RegionID:           opts.RegionID,
		ZoneID:             opts.ZoneID,
		Slug:               slug,
		Name:               opts.Name,
		CIDR:               opts.CIDR,
		SubnetType:         kind,
		Kind:               kind,
		RouteTableID:       rtID,
		NetworkACLID:       aclID,
		AutoAssignPublicIP: opts.AutoAssignPublicIP,
		Status:             "available",
		CreatedAt:          ts,
		UpdatedAt:          ts,
	}
	if err := m.store.InsertSubnet(sub); err != nil {
		return Subnet{}, fmt.Errorf("create subnet: %w", err)
	}
	if rtID != "" {
		_ = m.AssociateSubnet(sub.ID, rtID)
	}
	if aclID != "" {
		_ = m.store.AssociateSubnetNetworkACL(sub.ID, aclID)
	}
	return sub, nil
}

// CreateNetworkACL creates a network ACL in a VPC.
func (m *Manager) CreateNetworkACL(vpcID, name string, isDefault bool) (NetworkACL, error) {
	acl := NetworkACL{
		ID:        newID("acl"),
		VPCID:     vpcID,
		Name:      name,
		IsDefault: isDefault,
		CreatedAt: now(),
	}
	if err := m.store.InsertNetworkACL(acl); err != nil {
		return NetworkACL{}, fmt.Errorf("create network acl: %w", err)
	}
	return acl, nil
}

func (m *Manager) GetNetworkACL(nameOrID, vpcID string) (NetworkACL, error) {
	return m.store.GetNetworkACL(nameOrID, vpcID)
}

func (m *Manager) ListNetworkACLs(vpcID string) ([]NetworkACL, error) {
	return m.store.ListNetworkACLs(vpcID)
}

func (m *Manager) DeleteNetworkACL(nameOrID, vpcID string) error {
	return m.store.DeleteNetworkACL(nameOrID, vpcID)
}

func (m *Manager) AddNetworkACLEntry(aclID, direction, action, proto, cidr string, ruleNum, fromPort, toPort int) (NetworkACLEntry, error) {
	e := NetworkACLEntry{
		ID:           newID("acle"),
		NetworkACLID: aclID,
		RuleNumber:   ruleNum,
		Direction:    direction,
		Action:       action,
		Protocol:     proto,
		CIDR:         cidr,
		FromPort:     fromPort,
		ToPort:       toPort,
	}
	if err := m.store.InsertNetworkACLEntry(e); err != nil {
		return NetworkACLEntry{}, err
	}
	return e, nil
}

func (m *Manager) ListNetworkACLEntries(aclID string) ([]NetworkACLEntry, error) {
	return m.store.ListNetworkACLEntries(aclID)
}

func (m *Manager) DeleteNetworkACLEntry(aclID string, ruleNumber int) error {
	return m.store.DeleteNetworkACLEntry(aclID, ruleNumber)
}

func (m *Manager) GetSubnetByID(subnetID string) (Subnet, error) {
	return m.store.GetSubnetByID(subnetID)
}

func (m *Manager) UpdateSubnetBridge(subnetID, bridge, gateway string) error {
	return m.store.UpdateSubnetBridge(subnetID, bridge, gateway)
}

func (m *Manager) UpdateSubnet(sub Subnet) (Subnet, error) {
	sub.UpdatedAt = now()
	if err := m.store.UpdateSubnet(sub); err != nil {
		return Subnet{}, err
	}
	return m.store.GetSubnetByID(sub.ID)
}

func (m *Manager) GetRouteTableByID(id string) (RouteTable, error) {
	return m.store.GetRouteTableByID(id)
}
