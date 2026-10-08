package vpc_test

import (
	"testing"

	"capper/internal/vpc"
)

func TestCreateVPCExtended_PersistsDefaultIDs(t *testing.T) {
	m := newManager(t)
	created, err := m.CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "proj1", Name: "ext-vpc", CIDR: "10.20.0.0/16",
		DNSSupport: true, DNSHostnames: true,
	})
	if err != nil {
		t.Fatalf("CreateVPCExtended: %v", err)
	}
	if created.MainRouteTableID == "" || created.DefaultSecurityGroupID == "" || created.DefaultNetworkACLID == "" {
		t.Fatalf("create response missing defaults: %#v", created)
	}

	got, err := m.GetVPC(created.ID, "proj1")
	if err != nil {
		t.Fatalf("GetVPC: %v", err)
	}
	if got.MainRouteTableID != created.MainRouteTableID {
		t.Errorf("mainRouteTableId not persisted: got %q want %q", got.MainRouteTableID, created.MainRouteTableID)
	}
	if got.DefaultSecurityGroupID != created.DefaultSecurityGroupID {
		t.Errorf("defaultSecurityGroupId not persisted: got %q want %q", got.DefaultSecurityGroupID, created.DefaultSecurityGroupID)
	}
	if got.DefaultNetworkACLID != created.DefaultNetworkACLID {
		t.Errorf("defaultNetworkAclId not persisted: got %q want %q", got.DefaultNetworkACLID, created.DefaultNetworkACLID)
	}

	rts, err := m.ListRouteTables(got.ID)
	if err != nil {
		t.Fatalf("ListRouteTables: %v", err)
	}
	var foundMain bool
	for _, rt := range rts {
		if rt.ID == got.MainRouteTableID && rt.IsMain {
			foundMain = true
		}
	}
	if !foundMain {
		t.Fatal("main route table isMain not persisted")
	}
}

func TestCreateSubnetExtended_BindsDefaults(t *testing.T) {
	m := newManager(t)
	v, err := m.CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "proj1", Name: "sub-vpc", CIDR: "10.30.0.0/16",
		DNSSupport: true, DNSHostnames: true,
	})
	if err != nil {
		t.Fatalf("CreateVPCExtended: %v", err)
	}
	sub, err := m.CreateSubnetExtended(vpc.CreateSubnetOptions{
		VPCID: v.ID, Name: "public-a", CIDR: "10.30.1.0/24", Kind: vpc.SubnetPublic,
	})
	if err != nil {
		t.Fatalf("CreateSubnetExtended: %v", err)
	}
	if sub.RouteTableID != v.MainRouteTableID {
		t.Errorf("routeTableId=%q want %q", sub.RouteTableID, v.MainRouteTableID)
	}
	if sub.NetworkACLID != v.DefaultNetworkACLID {
		t.Errorf("networkAclId=%q want %q", sub.NetworkACLID, v.DefaultNetworkACLID)
	}

	got, err := m.GetSubnetByID(sub.ID)
	if err != nil {
		t.Fatalf("GetSubnetByID: %v", err)
	}
	if got.RouteTableID != v.MainRouteTableID || got.NetworkACLID != v.DefaultNetworkACLID {
		t.Errorf("persisted subnet defaults: rt=%q acl=%q", got.RouteTableID, got.NetworkACLID)
	}
}

func TestAssociateSubnet_ReplacesPriorAssociation(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "proj1", Name: "assoc-vpc", CIDR: "10.40.0.0/16",
		DNSSupport: true, DNSHostnames: true,
	})
	sub, _ := m.CreateSubnetExtended(vpc.CreateSubnetOptions{
		VPCID: v.ID, Name: "sub1", CIDR: "10.40.1.0/24", Kind: vpc.SubnetPrivate,
	})
	rt2, err := m.CreateRouteTable(v.ID, "custom")
	if err != nil {
		t.Fatalf("CreateRouteTable: %v", err)
	}
	if err := m.AssociateSubnet(sub.ID, rt2.ID); err != nil {
		t.Fatalf("AssociateSubnet: %v", err)
	}
	got, err := m.GetSubnetByID(sub.ID)
	if err != nil {
		t.Fatalf("GetSubnetByID: %v", err)
	}
	if got.RouteTableID != rt2.ID {
		t.Errorf("route_table_id column=%q want %q", got.RouteTableID, rt2.ID)
	}

	rt3, err := m.CreateRouteTable(v.ID, "custom-2")
	if err != nil {
		t.Fatalf("CreateRouteTable 2: %v", err)
	}
	if err := m.AssociateSubnet(sub.ID, rt3.ID); err != nil {
		t.Fatalf("re-associate: %v", err)
	}
	got, err = m.GetSubnetByID(sub.ID)
	if err != nil {
		t.Fatalf("GetSubnetByID after reassoc: %v", err)
	}
	if got.RouteTableID != rt3.ID {
		t.Fatalf("after reassoc route_table_id=%q want %q (must replace, not stack)", got.RouteTableID, rt3.ID)
	}
}

func TestCreateENI_PersistsSecurityGroups(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "proj1", Name: "eni-vpc", CIDR: "10.50.0.0/16",
		DNSSupport: true, DNSHostnames: true,
	})
	sub, _ := m.CreateSubnetExtended(vpc.CreateSubnetOptions{
		VPCID: v.ID, Name: "sub1", CIDR: "10.50.1.0/24", Kind: vpc.SubnetPrivate,
	})
	sg := v.DefaultSecurityGroupID
	eni, err := m.CreateENI(v.ID, sub.ID, []string{sg}, "")
	if err != nil {
		t.Fatalf("CreateENI: %v", err)
	}
	got, err := m.GetENI(eni.ID)
	if err != nil {
		t.Fatalf("GetENI: %v", err)
	}
	if len(got.SecurityGroupIDs) != 1 || got.SecurityGroupIDs[0] != sg {
		t.Fatalf("securityGroupIds=%v want [%s]", got.SecurityGroupIDs, sg)
	}
}

func TestDeleteVPCExtended_CleansNACLsAndENIs(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "proj1", Name: "del-vpc", CIDR: "10.60.0.0/16",
		DNSSupport: true, DNSHostnames: true,
	})
	sub, _ := m.CreateSubnetExtended(vpc.CreateSubnetOptions{
		VPCID: v.ID, Name: "sub1", CIDR: "10.60.1.0/24", Kind: vpc.SubnetPrivate,
	})
	_, _ = m.CreateENI(v.ID, sub.ID, []string{v.DefaultSecurityGroupID}, "")
	aclID := v.DefaultNetworkACLID
	if err := m.DeleteVPC(v.ID, "proj1"); err != nil {
		t.Fatalf("DeleteVPC: %v", err)
	}
	if _, err := m.GetNetworkACL(aclID, v.ID); err == nil {
		t.Fatal("expected network ACL to be deleted")
	}
	enis, _ := m.ListENIs(v.ID)
	if len(enis) != 0 {
		t.Fatalf("expected 0 ENIs after delete, got %d", len(enis))
	}
}
