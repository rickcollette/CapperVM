package topology_test

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"capper/internal/topology"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// topology.InitSchema adds columns to instances and lb_load_balancers via ALTER TABLE;
	// create those tables first so the migrations succeed in an isolated test DB.
	prereqs := []string{
		`CREATE TABLE IF NOT EXISTS instances (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS lb_load_balancers (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
	}
	for _, stmt := range prereqs {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("prereq: %v", err)
		}
	}
	if err := topology.InitSchema(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newManager(t *testing.T) *topology.Manager {
	t.Helper()
	m := topology.NewManager(openDB(t))
	if err := m.EnsureLocalTopology(); err != nil {
		t.Fatalf("EnsureLocalTopology: %v", err)
	}
	return m
}

func TestEnsureLocalTopology_Idempotent(t *testing.T) {
	db := openDB(t)
	m := topology.NewManager(db)
	if err := m.EnsureLocalTopology(); err != nil {
		t.Fatalf("first EnsureLocalTopology: %v", err)
	}
	if err := m.EnsureLocalTopology(); err != nil {
		t.Fatalf("second EnsureLocalTopology (idempotency): %v", err)
	}
}

func TestDefaultRealm(t *testing.T) {
	m := newManager(t)
	realm, err := m.DefaultRealm()
	if err != nil {
		t.Fatalf("DefaultRealm: %v", err)
	}
	if realm.ID == "" {
		t.Error("default realm ID must be set")
	}
}

func TestStore_InsertAndGetRealm(t *testing.T) {
	db := openDB(t)
	s := topology.NewStore(db)
	r := topology.Realm{
		ID:   "realm-test-1",
		Name: "Test Realm",
		Slug: "test-realm",
	}
	if err := s.InsertRealm(r); err != nil {
		t.Fatalf("InsertRealm: %v", err)
	}
	got, err := s.GetRealm("test-realm")
	if err != nil {
		t.Fatalf("GetRealm: %v", err)
	}
	if got.Name != "Test Realm" {
		t.Errorf("name: %q", got.Name)
	}
}

func TestStore_EmptyListsAreArrays(t *testing.T) {
	s := topology.NewStore(openDB(t))
	realms, err := s.ListRealms()
	if err != nil {
		t.Fatal(err)
	}
	regions, err := s.ListRegions("")
	if err != nil {
		t.Fatal(err)
	}
	zones, err := s.ListZones("")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := s.ListNodes("")
	if err != nil {
		t.Fatal(err)
	}
	if realms == nil || regions == nil || zones == nil || nodes == nil {
		t.Fatalf("empty list results must be non-nil slices: realms=%v regions=%v zones=%v nodes=%v",
			realms, regions, zones, nodes)
	}
}

func TestStore_InsertAndGetZone(t *testing.T) {
	db := openDB(t)
	s := topology.NewStore(db)

	realm := topology.Realm{ID: "r1", Name: "Realm1", Slug: "realm1"}
	region := topology.Region{ID: "reg1", RealmID: "r1", Name: "Region1", Slug: "region1", RegionCode: "r1"}
	zone := topology.Zone{ID: "z1", RealmID: "r1", RegionID: "reg1", Name: "Zone1", Slug: "zone1"}

	_ = s.InsertRealm(realm)
	_ = s.InsertRegion(region)
	if err := s.InsertZone(zone); err != nil {
		t.Fatalf("InsertZone: %v", err)
	}
	got, err := s.GetZone("zone1")
	if err != nil {
		t.Fatalf("GetZone: %v", err)
	}
	if got.Name != "Zone1" {
		t.Errorf("zone name: %q", got.Name)
	}

	zones, err := s.ListZones("reg1")
	if err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	if len(zones) != 1 {
		t.Errorf("ListZones: got %d, want 1", len(zones))
	}
}

func TestStore_InsertAndGetNode(t *testing.T) {
	db := openDB(t)
	s := topology.NewStore(db)

	_ = s.InsertRealm(topology.Realm{ID: "r1", Name: "R1", Slug: "r1"})
	_ = s.InsertRegion(topology.Region{ID: "reg1", RealmID: "r1", Name: "Reg1", Slug: "reg1", RegionCode: "rc1"})
	_ = s.InsertZone(topology.Zone{ID: "z1", RealmID: "r1", RegionID: "reg1", Name: "Z1", Slug: "z1"})
	node := topology.Node{ID: "n1", ZoneID: "z1", RealmID: "r1", RegionID: "reg1", Name: "node-1", Slug: "node-1"}
	if _, err := s.InsertNode(node); err != nil {
		t.Fatalf("InsertNode: %v", err)
	}
	got, err := s.GetNode("node-1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Name != "node-1" {
		t.Errorf("node name: %q", got.Name)
	}

	nodes, err := s.ListNodes("z1")
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("ListNodes: got %d, want 1", len(nodes))
	}
}

func TestScheduler_Simulate_NoNodes(t *testing.T) {
	// Use a fresh DB without EnsureLocalTopology so there are truly no nodes.
	db := openDB(t)
	s := topology.NewStore(db)
	sched := topology.NewScheduler(s)
	req := topology.PlacementRequest{
		Project:  "proj1",
		Image:    "nginx.cap",
		Strategy: topology.StrategySpreadZones,
	}
	result := sched.Simulate(context.Background(), req)
	// With no candidate nodes, Allowed should be false.
	if result.Allowed {
		t.Error("expected Allowed=false with no nodes available")
	}
}

func TestScheduler_Simulate_WithNode(t *testing.T) {
	db := openDB(t)
	s := topology.NewStore(db)
	_ = s.InsertRealm(topology.Realm{ID: "r1", Name: "R1", Slug: "r1"})
	_ = s.InsertRegion(topology.Region{ID: "reg1", RealmID: "r1", Name: "Reg1", Slug: "reg1", RegionCode: "rc1"})
	_ = s.InsertZone(topology.Zone{ID: "z1", RealmID: "r1", RegionID: "reg1", Name: "Z1", Slug: "z1"})
	_, _ = s.InsertNode(topology.Node{
		ID: "n1", ZoneID: "z1", RealmID: "r1", RegionID: "reg1",
		Name:        "node-1",
		Slug:        "node-1",
		Status:      topology.StatusReady,
		MemoryBytes: 8 * 1024 * 1024 * 1024,
	})

	m := topology.NewManager(db)
	sched := topology.NewScheduler(m.Store())
	req := topology.PlacementRequest{
		Project:  "proj1",
		Image:    "nginx.cap",
		Strategy: topology.StrategySingleNode,
	}
	result := sched.Simulate(context.Background(), req)
	if !result.Allowed {
		t.Errorf("expected Allowed=true with one online node; rejections: %v", result.Rejections)
	}
	if len(result.Candidates) == 0 {
		t.Error("expected at least one candidate")
	}
}
