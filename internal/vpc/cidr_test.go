package vpc_test

import (
	"testing"

	"capper/internal/vpc"
)

func TestValidateCIDR(t *testing.T) {
	if err := vpc.ValidateCIDR("10.0.0.0/16"); err != nil {
		t.Fatalf("valid cidr: %v", err)
	}
	if err := vpc.ValidateCIDR("bad"); err == nil {
		t.Fatal("expected error for bad cidr")
	}
}

func TestContainsCIDR(t *testing.T) {
	ok, err := vpc.ContainsCIDR("10.0.0.0/16", "10.0.1.0/24")
	if err != nil || !ok {
		t.Fatalf("contains: ok=%v err=%v", ok, err)
	}
	ok, _ = vpc.ContainsCIDR("10.0.0.0/16", "10.1.0.0/16")
	if ok {
		t.Fatal("expected no containment")
	}
}

func TestOverlapCIDR(t *testing.T) {
	ok, _ := vpc.OverlapCIDR("10.0.0.0/24", "10.0.0.128/25")
	if !ok {
		t.Fatal("expected overlap")
	}
}

func TestCreateENI(t *testing.T) {
	m := newManager(t)
	v, _ := m.CreateVPC("p1", "vpc1", "10.0.0.0/16", "")
	sub, _ := m.CreateSubnet(v.ID, "sub1", "10.0.1.0/24", "z1", vpc.SubnetPublic)
	eni, err := m.CreateENI(v.ID, sub.ID, nil, "")
	if err != nil {
		t.Fatalf("CreateENI: %v", err)
	}
	if eni.PrimaryPrivateIP == "" {
		t.Fatal("expected primary private ip")
	}
}

func TestKeyPairImport(t *testing.T) {
	m := newManager(t)
	k, err := m.ImportKeyPair("p1", "dev", "ssh-rsa AAAAB3...", "rsa")
	if err != nil {
		t.Fatalf("ImportKeyPair: %v", err)
	}
	if k.Fingerprint == "" {
		t.Fatal("expected fingerprint")
	}
}
