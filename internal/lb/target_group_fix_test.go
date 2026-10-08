package lb

import (
	"testing"
)

func TestAddTarget_DuplicateReturnsExistingID(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	m := NewManager(s)

	lbRec, err := m.Create("dup-lb", "proj", "net-1", ":80", ModeTCP)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tg, err := m.CreateTargetGroupForLB(lbRec.Name, "proj", "tg1", "tcp", 80, "/")
	if err != nil {
		t.Fatalf("CreateTargetGroupForLB: %v", err)
	}
	first, err := m.AddTarget(lbRec.Name, "proj", tg.ID, "10.0.0.5:80")
	if err != nil {
		t.Fatalf("AddTarget: %v", err)
	}
	second, err := m.AddTarget(lbRec.Name, "proj", tg.ID, "10.0.0.5:80")
	if err != nil {
		t.Fatalf("AddTarget duplicate: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate AddTarget fabricated new id: first=%q second=%q", first.ID, second.ID)
	}
	targets, err := s.ListTargets(tg.ID)
	if err != nil {
		t.Fatalf("ListTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target row, got %d", len(targets))
	}
}

func TestDeleteTargetGroup_RemovesListeners(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	m := NewManager(s)

	lbRec, err := m.Create("del-tg-lb", "proj", "net-1", ":80", ModeTCP)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tg, err := m.CreateTargetGroupForLB(lbRec.Name, "proj", "tg1", "tcp", 80, "/")
	if err != nil {
		t.Fatalf("CreateTargetGroupForLB: %v", err)
	}
	lst, err := m.CreateListenerForLB(lbRec.Name, "proj", tg.ID, "tcp", 8080, "")
	if err != nil {
		t.Fatalf("CreateListenerForLB: %v", err)
	}
	if err := m.DeleteTargetGroup(lbRec.Name, "proj", tg.ID); err != nil {
		t.Fatalf("DeleteTargetGroup: %v", err)
	}
	if _, err := s.GetListener(lst.ID); err == nil {
		t.Fatal("expected listener deleted with target group")
	}
	if _, err := s.GetTargetGroup(tg.ID); err == nil {
		t.Fatal("expected target group deleted")
	}
}

func TestAddTarget_RejectsEmptyLBOwnership(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	m := NewManager(s)

	lbRec, err := m.Create("own-lb", "proj", "net-1", ":80", ModeTCP)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tg, err := s.CreateTargetGroup("proj", "orphan", "", "", "tcp", 80, "/")
	if err != nil {
		t.Fatalf("CreateTargetGroup: %v", err)
	}
	if _, err := m.AddTarget(lbRec.Name, "proj", tg.ID, "10.0.0.9:80"); err == nil {
		t.Fatal("expected ownership rejection for empty loadBalancerId")
	}
}
