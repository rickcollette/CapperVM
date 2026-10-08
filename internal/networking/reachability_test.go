package networking

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"capper/internal/vpc"
)

func TestAnalyzeReachability_DefaultsDeny(t *testing.T) {
	res := AnalyzeReachability(ReachabilityRequest{
		SourceType: "instance", SourceID: "i1",
		DestinationType: "instance", DestinationID: "i2",
		Protocol: "tcp", Port: 22,
	})
	if res.Allowed {
		t.Fatal("expected deny when SG data missing")
	}
}

func TestAnalyzeReachabilityWithVPC_NoSGsDeny(t *testing.T) {
	res := AnalyzeReachabilityWithVPC(ReachabilityRequest{
		SourceType: "instance", SourceID: "i1",
		DestinationType: "instance", DestinationID: "i2",
		Protocol: "tcp", Port: 22,
	}, nil, nil, 22)
	if res.Allowed {
		t.Fatal("expected deny with empty security groups")
	}
}

func TestAnalyzeReachabilityWithVPC_AllowsAnyAttachedSecurityGroup(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := vpc.InitSchema(db); err != nil {
		t.Fatal(err)
	}
	mgr := vpc.NewManager(db)
	v, err := mgr.CreateVPC("project", "test", "10.0.0.0/16", "")
	if err != nil {
		t.Fatal(err)
	}
	empty, err := mgr.CreateSecurityGroup(v.ID, "empty", "", true)
	if err != nil {
		t.Fatal(err)
	}
	allow, err := mgr.CreateSecurityGroup(v.ID, "allow", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddSGRule(allow.ID, vpc.SGIngress, "tcp", "0.0.0.0/0", 22, 22, "allow"); err != nil {
		t.Fatal(err)
	}
	acl, err := mgr.CreateNetworkACL(v.ID, "default", true)
	if err != nil {
		t.Fatal(err)
	}
	v.DefaultNetworkACLID = acl.ID
	if _, err := mgr.UpdateVPC(v); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddNetworkACLEntry(acl.ID, "ingress", "allow", "tcp", "0.0.0.0/0", 100, 22, 22); err != nil {
		t.Fatal(err)
	}
	result := AnalyzeReachabilityWithVPC(ReachabilityRequest{
		SourceType: "instance", SourceID: "i1",
		DestinationType: "instance", DestinationID: "i2",
		Protocol: "tcp", Port: 22,
	}, mgr, []string{empty.ID, allow.ID}, 22)
	if !result.Allowed {
		t.Fatalf("expected later matching security group to allow traffic, got %+v", result)
	}
}

func TestAnalyzeReachabilityWithVPC_DenyNetworkACLBlocksSecurityGroupAllow(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := vpc.InitSchema(db); err != nil {
		t.Fatal(err)
	}
	mgr := vpc.NewManager(db)
	v, err := mgr.CreateVPC("project", "test", "10.0.0.0/16", "")
	if err != nil {
		t.Fatal(err)
	}
	allow, err := mgr.CreateSecurityGroup(v.ID, "allow", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddSGRule(allow.ID, vpc.SGIngress, "tcp", "0.0.0.0/0", 22, 22, "allow"); err != nil {
		t.Fatal(err)
	}
	acl, err := mgr.CreateNetworkACL(v.ID, "default", true)
	if err != nil {
		t.Fatal(err)
	}
	v.DefaultNetworkACLID = acl.ID
	if _, err := mgr.UpdateVPC(v); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddNetworkACLEntry(acl.ID, "ingress", "deny", "tcp", "0.0.0.0/0", 50, 22, 22); err != nil {
		t.Fatal(err)
	}
	result := AnalyzeReachabilityWithVPC(ReachabilityRequest{
		SourceType: "instance", SourceID: "i1",
		DestinationType: "instance", DestinationID: "i2",
		Protocol: "tcp", Port: 22,
	}, mgr, []string{allow.ID}, 22)
	if result.Allowed {
		t.Fatalf("expected network ACL deny to block an allowing security group, got %+v", result)
	}
}
