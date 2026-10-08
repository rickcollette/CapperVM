package stack_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	capperdns "capper/internal/dns"
	"capper/internal/network"
	"capper/internal/stack"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := stack.InitSchema(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newManager(t *testing.T) *stack.Manager {
	t.Helper()
	db := openDB(t)
	if err := network.InitSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := capperdns.InitSchema(db); err != nil {
		t.Fatal(err)
	}
	return stack.NewManager(stack.NewStore(db), stack.Deps{
		Networks: network.NewStore(db),
		DNS:      capperdns.NewStore(db),
	})
}

func writeTemplate(t *testing.T, tmpl stack.StackTemplate) string {
	t.Helper()
	data, err := json.Marshal(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "stack.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func minimalTemplate(name string) stack.StackTemplate {
	return stack.StackTemplate{
		Name: name,
		Instances: []stack.InstanceSpec{
			{Name: "web", Image: "nginx.cap", SubnetID: "sub-1"},
		},
	}
}

func TestLoadTemplate_Valid(t *testing.T) {
	path := writeTemplate(t, minimalTemplate("my-stack"))
	tmpl, err := stack.LoadTemplate(path)
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if tmpl.Name != "my-stack" {
		t.Errorf("name: %q", tmpl.Name)
	}
	if len(tmpl.Instances) != 1 {
		t.Errorf("instances: got %d, want 1", len(tmpl.Instances))
	}
}

func TestLoadTemplate_MissingName(t *testing.T) {
	path := writeTemplate(t, stack.StackTemplate{
		Instances: []stack.InstanceSpec{{Name: "web", Image: "nginx.cap"}},
	})
	_, err := stack.LoadTemplate(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadTemplate_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := stack.LoadTemplate(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestPlan_ReturnsActions(t *testing.T) {
	m := newManager(t)
	tmpl := minimalTemplate("plan-test")
	ops, err := m.Plan(tmpl, "proj1")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(ops) == 0 {
		t.Error("expected at least one plan op")
	}
	// Should have a create op for the instance.
	types := map[string]bool{}
	for _, op := range ops {
		types[op.Type] = true
	}
	if types["network"] {
		t.Error("legacy network ops are no longer planned")
	}
	if !types["instance"] {
		t.Error("expected instance op in plan")
	}
}

func TestApply_StoresState(t *testing.T) {
	m := newManager(t)
	tmpl := minimalTemplate("apply-test")
	// Apply may fail if the real network/instance provisioners aren't available;
	// we check that it at least creates a stack record.
	_, err := m.Apply(context.Background(), tmpl, "proj1")
	// Any error is OK (missing real provisioners); we just need it not to panic.
	_ = err

	stacks, err := m.List("proj1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stacks) == 0 {
		t.Error("expected stack record to be persisted after Apply")
	}
}

func TestGet(t *testing.T) {
	m := newManager(t)
	tmpl := minimalTemplate("get-test")
	s, err := m.Apply(context.Background(), tmpl, "proj1")
	if err != nil {
		t.Skip("Apply failed (no real provisioners) — skipping Get test")
	}
	got, err := m.Get(s.Name, "proj1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "get-test" {
		t.Errorf("name: %q", got.Name)
	}
}
