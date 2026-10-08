package networking

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"capper/internal/vpc"
)

func TestUpdateVPC_CanDisableBooleans(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := vpc.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	svc := NewService(db, nil)
	created, err := svc.VPC().CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "p", Name: "v1", CIDR: "10.70.0.0/16",
		DNSSupport: true, DNSHostnames: true, EnableFlowLogs: true,
	})
	if err != nil {
		t.Fatalf("CreateVPCExtended: %v", err)
	}
	off := false
	updated, err := svc.UpdateVPC("p", created.ID, VPCPatch{
		DNSSupport: &off, DNSHostnames: &off, EnableFlowLogs: &off,
	})
	if err != nil {
		t.Fatalf("UpdateVPC: %v", err)
	}
	if updated.DNSSupport || updated.DNSHostnames || updated.EnableFlowLogs {
		t.Fatalf("expected all flags false, got dns=%v host=%v flow=%v",
			updated.DNSSupport, updated.DNSHostnames, updated.EnableFlowLogs)
	}
	got, err := svc.GetVPC("p", created.ID)
	if err != nil {
		t.Fatalf("GetVPC: %v", err)
	}
	if got.DNSSupport || got.DNSHostnames || got.EnableFlowLogs {
		t.Fatalf("persisted flags still true: dns=%v host=%v flow=%v",
			got.DNSSupport, got.DNSHostnames, got.EnableFlowLogs)
	}
}
