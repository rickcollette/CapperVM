package vpc_test

import (
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"

	"capper/internal/vpc"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("foreign_keys pragma: %v", err)
	}
	if err := vpc.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newManager(t *testing.T) *vpc.Manager {
	return vpc.NewManager(openDB(t))
}

// ---- VPC --------------------------------------------------------------------

func TestCreateAndListVPC(t *testing.T) {
	m := newManager(t)
	v, err := m.CreateVPC("proj1", "main-vpc", "10.0.0.0/16", "")
	if err != nil {
		t.Fatalf("CreateVPC: %v", err)
	}
	if v.ID == "" {
		t.Error("ID must be set")
	}

	list, err := m.ListVPCs("proj1")
	if err != nil {
		t.Fatalf("ListVPCs: %v", err)
	}
	if len(list) != 1 || list[0].Name != "main-vpc" {
		t.Errorf("ListVPCs: got %v", list)
	}
}

func TestGetVPC(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("proj1", "my-vpc", "10.1.0.0/16", "internal")

	got, err := m.GetVPC("my-vpc", "proj1")
	if err != nil {
		t.Fatalf("GetVPC by name: %v", err)
	}
	if got.CIDR != "10.1.0.0/16" {
		t.Errorf("cidr: %q", got.CIDR)
	}
	if got.DNSDomain != "internal" {
		t.Errorf("dnsDomain: %q", got.DNSDomain)
	}

	got2, err := m.GetVPC(v.ID, "proj1")
	if err != nil {
		t.Fatalf("GetVPC by ID: %v", err)
	}
	if got2.Name != "my-vpc" {
		t.Errorf("name: %q", got2.Name)
	}
}

func TestCreateVPCExtended_DuplicateName(t *testing.T) {
	m := newManager(t)
	if _, err := m.CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "proj1",
		Name:    "todd",
		Slug:    "todd",
		CIDR:    "10.1.0.0/16",
	}); err != nil {
		t.Fatalf("first CreateVPCExtended: %v", err)
	}
	_, err := m.CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "proj1",
		Name:    "todd",
		Slug:    "todd",
		CIDR:    "10.2.0.0/16",
	})
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if !errors.Is(err, vpc.ErrVPCNameTaken) {
		t.Fatalf("expected ErrVPCNameTaken, got: %v", err)
	}
}

func TestDeleteVPC(t *testing.T) {
	m := newManager(t)
	m.CreateVPC("proj1", "del-vpc", "10.2.0.0/16", "")
	if err := m.DeleteVPC("del-vpc", "proj1"); err != nil {
		t.Fatalf("DeleteVPC: %v", err)
	}
	list, _ := m.ListVPCs("proj1")
	if len(list) != 0 {
		t.Errorf("expected 0 VPCs after delete, got %d", len(list))
	}
}

func TestCreateVPC_Validation(t *testing.T) {
	m := newManager(t)
	if _, err := m.CreateVPC("", "name", "10.0.0.0/8", ""); err == nil {
		t.Error("expected error for empty project")
	}
	if _, err := m.CreateVPC("proj1", "", "10.0.0.0/8", ""); err == nil {
		t.Error("expected error for empty name")
	}
	if _, err := m.CreateVPC("proj1", "name", "", ""); err == nil {
		t.Error("expected error for empty cidr")
	}
}

// ---- Subnet -----------------------------------------------------------------

func TestCreateAndListSubnet(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("proj1", "vpc1", "10.0.0.0/16", "")

	sub, err := m.CreateSubnet(v.ID, "pub-a", "10.0.1.0/24", "zone-a", vpc.SubnetPublic)
	if err != nil {
		t.Fatalf("CreateSubnet: %v", err)
	}
	if sub.Kind != vpc.SubnetPublic {
		t.Errorf("kind: %q", sub.Kind)
	}

	list, err := m.ListSubnets(v.ID)
	if err != nil {
		t.Fatalf("ListSubnets: %v", err)
	}
	if len(list) != 1 || list[0].Name != "pub-a" {
		t.Errorf("ListSubnets: %v", list)
	}
}

func TestDeleteSubnet(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("proj1", "vpc1", "10.0.0.0/16", "")
	m.CreateSubnet(v.ID, "sub1", "10.0.1.0/24", "", vpc.SubnetPrivate)
	if err := m.DeleteSubnet("sub1", v.ID); err != nil {
		t.Fatalf("DeleteSubnet: %v", err)
	}
	list, _ := m.ListSubnets(v.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 subnets, got %d", len(list))
	}
}

// ---- RouteTable and Routes --------------------------------------------------

func TestRouteTableAndRoutes(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("proj1", "vpc1", "10.0.0.0/16", "")

	rt, err := m.CreateRouteTable(v.ID, "main-rt")
	if err != nil {
		t.Fatalf("CreateRouteTable: %v", err)
	}

	r, err := m.AddRoute(rt.ID, "0.0.0.0/0", "igw", "igw-123")
	if err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	if r.TargetType != "igw" {
		t.Errorf("target type: %q", r.TargetType)
	}

	routes, err := m.ListRoutes(rt.ID)
	if err != nil {
		t.Fatalf("ListRoutes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	if err := m.DeleteRoute(r.ID); err != nil {
		t.Fatalf("DeleteRoute: %v", err)
	}
	routes, _ = m.ListRoutes(rt.ID)
	if len(routes) != 0 {
		t.Errorf("expected 0 routes, got %d", len(routes))
	}
}

func TestAssociateSubnet(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("proj1", "vpc1", "10.0.0.0/16", "")
	sub, _ := m.CreateSubnet(v.ID, "sub1", "10.0.1.0/24", "", vpc.SubnetPrivate)
	rt, _ := m.CreateRouteTable(v.ID, "rt1")

	if err := m.AssociateSubnet(sub.ID, rt.ID); err != nil {
		t.Fatalf("AssociateSubnet: %v", err)
	}
	// Idempotent second call (INSERT OR REPLACE).
	if err := m.AssociateSubnet(sub.ID, rt.ID); err != nil {
		t.Fatalf("AssociateSubnet idempotent: %v", err)
	}
}

// ---- SecurityGroup ----------------------------------------------------------

func TestSecurityGroupCRUD(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("proj1", "vpc1", "10.0.0.0/16", "")

	sg, err := m.CreateSecurityGroup(v.ID, "web-sg", "allow web traffic", true)
	if err != nil {
		t.Fatalf("CreateSecurityGroup: %v", err)
	}
	if !sg.DefaultDeny {
		t.Error("defaultDeny should be true")
	}

	rule, err := m.AddSGRule(sg.ID, vpc.SGIngress, "tcp", "0.0.0.0/0", 80, 80, "allow")
	if err != nil {
		t.Fatalf("AddSGRule: %v", err)
	}
	if rule.Protocol != "tcp" {
		t.Errorf("protocol: %q", rule.Protocol)
	}

	rules, err := m.ListSGRules(sg.ID)
	if err != nil {
		t.Fatalf("ListSGRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Direction != vpc.SGIngress {
		t.Errorf("direction: %q", rules[0].Direction)
	}

	if err := m.DeleteSGRule(rule.ID); err != nil {
		t.Fatalf("DeleteSGRule: %v", err)
	}
	rules, _ = m.ListSGRules(sg.ID)
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete")
	}

	sgs, _ := m.ListSecurityGroups(v.ID)
	if len(sgs) != 1 {
		t.Fatalf("expected 1 SG, got %d", len(sgs))
	}

	if err := m.DeleteSecurityGroup(sg.Name, v.ID); err != nil {
		t.Fatalf("DeleteSecurityGroup: %v", err)
	}
	sgs, _ = m.ListSecurityGroups(v.ID)
	if len(sgs) != 0 {
		t.Errorf("expected 0 SGs after delete")
	}
}

// ---- InternetGateway --------------------------------------------------------

func TestInternetGatewayCRUD(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("proj1", "vpc1", "10.0.0.0/16", "")

	igw, err := m.CreateIGW(v.ID, "main-igw")
	if err != nil {
		t.Fatalf("CreateIGW: %v", err)
	}
	if igw.VPCID != v.ID {
		t.Errorf("vpcID: %q", igw.VPCID)
	}

	list, _ := m.ListIGWs(v.ID)
	if len(list) != 1 {
		t.Fatalf("expected 1 IGW, got %d", len(list))
	}

	got, err := m.GetIGW("main-igw", v.ID)
	if err != nil {
		t.Fatalf("GetIGW: %v", err)
	}
	if got.ID != igw.ID {
		t.Errorf("id mismatch")
	}

	if err := m.DeleteIGW(igw.Name, v.ID); err != nil {
		t.Fatalf("DeleteIGW: %v", err)
	}
	list, _ = m.ListIGWs(v.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 IGWs after delete")
	}
}

// ---- NATGateway -------------------------------------------------------------

func TestNATGatewayCRUD(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("proj1", "vpc1", "10.0.0.0/16", "")
	sub, _ := m.CreateSubnet(v.ID, "pub", "10.0.1.0/24", "z1", vpc.SubnetPublic)

	nat, err := m.CreateNATGateway(v.ID, sub.ID, "main-nat", "1.2.3.4")
	if err != nil {
		t.Fatalf("CreateNATGateway: %v", err)
	}
	if nat.PublicIP != "1.2.3.4" {
		t.Errorf("public ip: %q", nat.PublicIP)
	}

	list, _ := m.ListNATGateways(v.ID)
	if len(list) != 1 {
		t.Fatalf("expected 1 NAT, got %d", len(list))
	}

	got, err := m.GetNATGateway("main-nat", v.ID)
	if err != nil {
		t.Fatalf("GetNATGateway: %v", err)
	}
	if got.SubnetID != sub.ID {
		t.Errorf("subnetID: %q", got.SubnetID)
	}

	if err := m.DeleteNATGateway(nat.Name, v.ID); err != nil {
		t.Fatalf("DeleteNATGateway: %v", err)
	}
	list, _ = m.ListNATGateways(v.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 NATs after delete")
	}
}

// ---- Cascade delete ---------------------------------------------------------

func TestDeleteVPC_CascadesSubnets(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("proj1", "vpc1", "10.0.0.0/16", "")
	m.CreateSubnet(v.ID, "sub1", "10.0.1.0/24", "", vpc.SubnetPrivate)
	m.CreateIGW(v.ID, "igw1")
	m.CreateSecurityGroup(v.ID, "sg1", "", true)

	if err := m.DeleteVPC(v.ID, "proj1"); err != nil {
		t.Fatalf("DeleteVPC: %v", err)
	}
	// After cascade, subnets should be gone too.
	list, _ := m.ListSubnets(v.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 subnets after cascade delete, got %d", len(list))
	}
}
