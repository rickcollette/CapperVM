package lb

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestSetServiceAlias verifies that SetAlias stores the alias in the DB and
// calls the DNSAliasCreator with the correct arguments.
func TestSetServiceAlias(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	m := NewManager(s)

	lb, err := m.Create("web-lb", "proj", "net-1", ":80", ModeTCP)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var createdZone, createdAlias, createdTarget string
	creator := DNSAliasCreator(func(zoneID, alias, target string) error {
		createdZone = zoneID
		createdAlias = alias
		createdTarget = target
		return nil
	})

	err = m.SetAlias(lb.ID, "proj", "zone-99", "myapp", "web-lb.local.", creator)
	if err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	// Verify DNS creator was called with correct args.
	if createdZone != "zone-99" || createdAlias != "myapp" || createdTarget != "web-lb.local." {
		t.Errorf("creator args: zone=%q alias=%q target=%q", createdZone, createdAlias, createdTarget)
	}

	// Verify alias persisted in store.
	got, err := s.Get(lb.ID, "proj")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ServiceAlias != "myapp" {
		t.Errorf("ServiceAlias = %q want %q", got.ServiceAlias, "myapp")
	}
}

// TestSetServiceAliasNilCreator verifies SetAlias with nil creator is safe.
func TestSetServiceAliasNilCreator(t *testing.T) {
	db := openTestDB(t)
	m := NewManager(NewStore(db))
	lb, _ := m.Create("lb2", "p", "", ":81", ModeHTTP)

	err := m.SetAlias(lb.ID, "p", "", "alias2", "", nil)
	if err != nil {
		t.Fatalf("SetAlias with nil creator: %v", err)
	}

	got, _ := m.Store().Get(lb.ID, "p")
	if got.ServiceAlias != "alias2" {
		t.Errorf("ServiceAlias = %q want %q", got.ServiceAlias, "alias2")
	}
}

// TestSetServiceAliasUnknownLB verifies SetAlias returns an error for missing LBs.
func TestSetServiceAliasUnknownLB(t *testing.T) {
	db := openTestDB(t)
	m := NewManager(NewStore(db))
	err := m.SetAlias("nonexistent-id", "proj", "", "", "", nil)
	if err == nil {
		t.Error("expected error for unknown LB ID")
	}
}

// TestLBCreateListDelete exercises the basic lifecycle.
func TestLBCreateListDelete(t *testing.T) {
	db := openTestDB(t)
	m := NewManager(NewStore(db))

	for _, name := range []string{"lb-a", "lb-b", "lb-c"} {
		if _, err := m.Create(name, "default", "", ":0", ModeTCP); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	list, err := m.List("default")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 LBs, got %d", len(list))
	}

	if err := m.Delete("lb-b", "default"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list2, _ := m.List("default")
	if len(list2) != 2 {
		t.Errorf("expected 2 LBs after delete, got %d", len(list2))
	}
}

func TestLBDeletePropagatesDependentFailure(t *testing.T) {
	db := openTestDB(t)
	m := NewManager(NewStore(db))
	lb, err := m.Create("delete-failure", "project", "", ":80", ModeTCP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddBackend(lb.ID, "project", "10.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_lb_backend_delete
		BEFORE DELETE ON lb_backends BEGIN SELECT RAISE(ABORT, 'backend delete failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(lb.ID, "project"); err == nil {
		t.Fatal("expected dependent deletion error")
	}
	if _, err := m.Store().Get(lb.ID, "project"); err != nil {
		t.Fatalf("load balancer was deleted despite dependent failure: %v", err)
	}
	backends, err := m.ListBackends(lb.ID, "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(backends) != 1 {
		t.Fatalf("dependent state was not preserved: got %d backends", len(backends))
	}
}

// TestLBAddRemoveBackend exercises the backend management path.
func TestLBAddRemoveBackend(t *testing.T) {
	db := openTestDB(t)
	m := NewManager(NewStore(db))
	lb, _ := m.Create("proxy", "proj", "", ":80", ModeTCP)

	if _, err := m.AddBackend(lb.ID, "proj", "10.0.0.1:8080"); err != nil {
		t.Fatalf("AddBackend: %v", err)
	}
	if _, err := m.AddBackend(lb.ID, "proj", "10.0.0.2:8080"); err != nil {
		t.Fatalf("AddBackend 2: %v", err)
	}

	backends, err := m.ListBackends(lb.ID, "proj")
	if err != nil {
		t.Fatalf("ListBackends: %v", err)
	}
	if len(backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(backends))
	}

	if err := m.RemoveBackend(lb.ID, "proj", "10.0.0.1:8080"); err != nil {
		t.Fatalf("RemoveBackend: %v", err)
	}
	backends2, _ := m.ListBackends(lb.ID, "proj")
	if len(backends2) != 1 {
		t.Errorf("expected 1 backend after remove, got %d", len(backends2))
	}
}
