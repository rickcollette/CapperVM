package ipam

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewStore(db)
	if err := s.InitSchema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return s
}

func TestExpandCIDR(t *testing.T) {
	// /28 has 16 addresses; minus network, broadcast, gateway = 13 usable.
	addrs, err := ExpandCIDR("203.0.113.0/28", "203.0.113.1", nil, 0)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(addrs) != 13 {
		t.Fatalf("expected 13 usable addresses, got %d", len(addrs))
	}
	for _, a := range addrs {
		if a == "203.0.113.0" || a == "203.0.113.15" || a == "203.0.113.1" {
			t.Errorf("network/broadcast/gateway not excluded: %s", a)
		}
	}

	// Explicit exclusions are honored.
	addrs2, _ := ExpandCIDR("203.0.113.0/28", "203.0.113.1", []string{"203.0.113.5"}, 0)
	if len(addrs2) != 12 {
		t.Errorf("expected 12 with one extra exclusion, got %d", len(addrs2))
	}
}

func TestPoolMaterializationAndReserve(t *testing.T) {
	s := newStore(t)
	mgr := NewManager(s)

	pool, count, err := mgr.CreatePool(CreatePoolOptions{
		Pool: RoutableIPPool{
			Name: "public-main", CIDR: "203.0.113.0/28", Gateway: "203.0.113.1",
			Usage: []string{UsageLoadBalancer, UsageReserved}, AllowAutoAllocate: true, Status: PoolActive,
		},
	})
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if count != 13 {
		t.Errorf("expected 13 materialized addresses, got %d", count)
	}

	// Auto reserve for an allowed purpose.
	ip, err := mgr.Reserve(ReserveOptions{PoolID: pool.ID, Project: "prod", Name: "api-ip", Purpose: UsageLoadBalancer})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if ip.Status != IPReserved || ip.Name != "api-ip" {
		t.Errorf("unexpected reserved ip: %+v", ip)
	}

	// Disallowed purpose is rejected.
	if _, err := mgr.Reserve(ReserveOptions{PoolID: pool.ID, Purpose: UsageEgress}); err == nil {
		t.Error("expected rejection for disallowed purpose egress")
	}

	// Reserve a specific address (.5 is materialized and not the auto-picked one).
	ip2, err := mgr.Reserve(ReserveOptions{PoolID: pool.ID, Address: "203.0.113.5", Purpose: UsageReserved})
	if err != nil {
		t.Fatalf("reserve specific: %v", err)
	}
	if ip2.Address != "203.0.113.5" {
		t.Errorf("expected 203.0.113.5, got %s", ip2.Address)
	}

	// Release returns it to available.
	if err := mgr.Release(ip2.ID); err != nil {
		t.Fatalf("release: %v", err)
	}
	got, _ := s.GetIP(ip2.ID)
	if got.Status != IPAvailable {
		t.Errorf("expected available after release, got %s", got.Status)
	}
}

func TestAttachConflict(t *testing.T) {
	s := newStore(t)
	mgr := NewManager(s)
	pool, _, _ := mgr.CreatePool(CreatePoolOptions{
		Pool: RoutableIPPool{Name: "lb", CIDR: "198.51.100.0/29", Gateway: "198.51.100.1",
			Usage: []string{UsageLoadBalancer}, AllowAutoAllocate: true, Status: PoolActive},
	})
	ip, _ := mgr.Reserve(ReserveOptions{PoolID: pool.ID, Purpose: UsageLoadBalancer})

	// First bind 443/tcp to lb-1.
	if _, err := mgr.Attach(ip.ID, IPBinding{TargetType: "load-balancer", TargetID: "lb-1",
		BindingMode: ModeVIP, Protocol: "tcp", ExternalPort: 443}); err != nil {
		t.Fatalf("attach 1: %v", err)
	}
	// Same IP+proto+port to a different target conflicts.
	if _, err := mgr.Attach(ip.ID, IPBinding{TargetType: "load-balancer", TargetID: "lb-2",
		BindingMode: ModeVIP, Protocol: "tcp", ExternalPort: 443}); err == nil {
		t.Error("expected conflict for same IP+proto+port to different target")
	}
	// Port 80 on the same IP to the same LB is allowed.
	if _, err := mgr.Attach(ip.ID, IPBinding{TargetType: "load-balancer", TargetID: "lb-1",
		BindingMode: ModeVIP, Protocol: "tcp", ExternalPort: 80}); err != nil {
		t.Errorf("expected 80 to be allowed alongside 443: %v", err)
	}
}

func TestDetachBindingPreservesOtherAssociations(t *testing.T) {
	s := newStore(t)
	mgr := NewManager(s)
	pool, _, err := mgr.CreatePool(CreatePoolOptions{
		Pool: RoutableIPPool{Name: "bindings", CIDR: "198.51.100.0/29", Gateway: "198.51.100.1",
			Usage: []string{UsageLoadBalancer}, AllowAutoAllocate: true, Status: PoolActive},
	})
	if err != nil {
		t.Fatal(err)
	}
	ip, err := mgr.Reserve(ReserveOptions{PoolID: pool.ID, Purpose: UsageLoadBalancer})
	if err != nil {
		t.Fatal(err)
	}
	first, err := mgr.Attach(ip.ID, IPBinding{TargetType: "load-balancer", TargetID: "lb", BindingMode: ModeVIP, Protocol: "tcp", ExternalPort: 80})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Attach(ip.ID, IPBinding{TargetType: "load-balancer", TargetID: "lb", BindingMode: ModeVIP, Protocol: "tcp", ExternalPort: 443}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DetachBinding(first.ID); err != nil {
		t.Fatal(err)
	}
	bindings, err := s.ListBindings(ip.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetIP(ip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || got.Status != IPAttached {
		t.Fatalf("detaching one association removed too much: bindings=%d status=%s", len(bindings), got.Status)
	}
}

func TestReservedOnlyPoolRequiresExplicitAddress(t *testing.T) {
	s := newStore(t)
	mgr := NewManager(s)
	pool, _, _ := mgr.CreatePool(CreatePoolOptions{
		Pool: RoutableIPPool{Name: "reserved", CIDR: "203.0.113.0/29", Gateway: "203.0.113.1",
			Usage: []string{UsageReserved}, AllowAutoAllocate: false, Status: PoolActive},
	})
	if _, err := mgr.Reserve(ReserveOptions{PoolID: pool.ID, Purpose: UsageReserved}); err == nil {
		t.Error("expected reserved-only pool to reject auto-allocation")
	}
	if _, err := mgr.Reserve(ReserveOptions{PoolID: pool.ID, Address: "203.0.113.2", Purpose: UsageReserved}); err != nil {
		t.Errorf("explicit address reservation should succeed: %v", err)
	}
}
