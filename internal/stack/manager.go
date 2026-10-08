package stack

import (
	"context"
	"crypto/sha256"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	capperdns "capper/internal/dns"
	"capper/internal/network"
)

// InstanceStatusFunc returns the status string for an instance by ID.
// Injected to avoid importing capper/internal/store (circular dep).
type InstanceStatusFunc func(instanceID string) (string, error)

// InstanceRestartFunc re-launches a failed instance replica.
type InstanceRestartFunc func(ctx context.Context, instanceID string) error

// Deps holds the sub-stores stack operations need, avoiding an import cycle
// with capper/internal/store.
type Deps struct {
	Networks       *network.Store
	DNS            *capperdns.Store
	InstanceStatus InstanceStatusFunc
	InstanceRestart InstanceRestartFunc
}

// Manager implements Plan/Apply/Diff/Destroy for stacks.
type Manager struct {
	stackStore *Store
	deps       Deps
}

// NewManager creates a stack Manager.
func NewManager(stackStore *Store, deps Deps) *Manager {
	return &Manager{stackStore: stackStore, deps: deps}
}

// Get returns a stack by name or ID.
func (m *Manager) Get(nameOrID, project string) (Stack, error) {
	return m.stackStore.Get(nameOrID, project)
}

// List returns all stacks for the project.
func (m *Manager) List(project string) ([]Stack, error) {
	return m.stackStore.List(project)
}

// Plan returns what would be created to apply tmpl.
func (m *Manager) Plan(tmpl StackTemplate, project string) ([]PlanOp, error) {
	if len(tmpl.Networks) > 0 {
		return nil, fmt.Errorf("stack networks[] is removed; use VPC subnets")
	}
	var ops []PlanOp
	for _, inst := range tmpl.Instances {
		ops = append(ops, PlanOp{Action: "create", Type: "instance", Name: inst.Name, Reason: "defined in template"})
	}
	for _, lb := range tmpl.LBs {
		ops = append(ops, PlanOp{Action: "create", Type: "lb", Name: lb.Name, Reason: "defined in template"})
	}
	for _, d := range tmpl.DNS {
		ops = append(ops, PlanOp{Action: "create", Type: "dns_record", Name: d.Name + "." + d.Zone, Reason: "defined in template"})
	}
	return ops, nil
}

// Apply creates or updates resources defined in tmpl.
func (m *Manager) Apply(ctx context.Context, tmpl StackTemplate, project string) (*Stack, error) {
	hash := templateHash(tmpl)

	// Reuse or create the stack record.
	existing, err := m.stackStore.Get(tmpl.Name, project)
	var st Stack
	if err == nil {
		st = existing
	} else {
		st = Stack{
			ID:           newID(),
			Name:         tmpl.Name,
			Project:      project,
			TemplateHash: hash,
			Status:       StackActive,
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		}
		if ierr := m.stackStore.Insert(st); ierr != nil {
			return nil, fmt.Errorf("stack: store: %w", ierr)
		}
	}

	var resources []StackResource

	if len(tmpl.Networks) > 0 {
		return nil, fmt.Errorf("stack networks[] is removed; use VPC subnets")
	}

	// Instances — record only (image file must exist for actual launch)
	for _, spec := range tmpl.Instances {
		resources = append(resources, StackResource{Type: "instance", Name: spec.Name, ID: ""})
	}

	// 3. LBs — record plan; actual proxy start happens via daemon reconcile
	for _, spec := range tmpl.LBs {
		resources = append(resources, StackResource{Type: "lb", Name: spec.Name, ID: ""})
	}

	// 4. DNS records
	dnsMgr := capperdns.NewManager(m.deps.DNS)
	for _, spec := range tmpl.DNS {
		networkID := ""
		// Try to find a network ID for the zone
		for _, nr := range resources {
			if nr.Type == "network" {
				networkID = nr.ID
				break
			}
		}
		z, zerr := dnsMgr.CreateZone(spec.Zone, capperdns.ZoneTypePrivate, networkID, 30, "stack:"+tmpl.Name)
		if zerr != nil {
			return nil, fmt.Errorf("stack: create zone %q: %w", spec.Zone, zerr)
		}
		ttl := spec.TTL
		if ttl == 0 {
			ttl = 30
		}
		r, rerr := dnsMgr.CreateRecord(z.Name, networkID, spec.Name, spec.Type, spec.Values, ttl)
		if rerr != nil {
			return nil, fmt.Errorf("stack: create record %q: %w", spec.Name, rerr)
		}
		resources = append(resources, StackResource{Type: "dns_record", Name: spec.Name + "." + spec.Zone, ID: r.ID})
	}

	st.Resources = resources
	st.TemplateHash = hash
	st.Status = StackActive
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := m.stackStore.UpdateResources(st.ID, resources); err != nil {
		return nil, err
	}
	_ = m.stackStore.UpdateStatus(st.ID, StackActive)
	return &st, nil
}

// Diff compares stored resources against live state.
func (m *Manager) Diff(nameOrID, project string) ([]PlanOp, error) {
	st, err := m.stackStore.Get(nameOrID, project)
	if err != nil {
		return nil, err
	}
	var ops []PlanOp
	for _, r := range st.Resources {
		switch r.Type {
		case "network":
			if _, nerr := m.deps.Networks.Get(r.ID, ""); nerr != nil {
				ops = append(ops, PlanOp{Action: "delete", Type: r.Type, Name: r.Name, Reason: "missing from live state"})
			}
		}
	}
	return ops, nil
}

// Destroy deletes all resources for the stack in reverse order.
func (m *Manager) Destroy(ctx context.Context, nameOrID, project string) error {
	st, err := m.stackStore.Get(nameOrID, project)
	if err != nil {
		return err
	}

	// Reverse order: dns_record, lb, instance, network
	dnsMgr := capperdns.NewManager(m.deps.DNS)
	netMgr := network.NewManager(m.deps.Networks)

	for i := len(st.Resources) - 1; i >= 0; i-- {
		r := st.Resources[i]
		switch r.Type {
		case "dns_record":
			if r.ID != "" {
				// Find zone from name (name is "record.zone")
				parts := splitDotN(r.Name, 2)
				if len(parts) == 2 {
					_ = dnsMgr.DeleteRecord(parts[1], "", r.ID)
				}
			}
		case "network":
			if r.ID != "" {
				_ = netMgr.Delete(r.ID, project)
			}
		}
	}

	_ = m.stackStore.UpdateStatus(st.ID, StackDestroyed)
	return m.stackStore.Delete(st.ID)
}

// ReconcileReplicas checks all active stacks for failed instance resources and
// restarts them using the injected InstanceRestart callback.
func (m *Manager) ReconcileReplicas(ctx context.Context, project string) error {
	if m.deps.InstanceStatus == nil || m.deps.InstanceRestart == nil {
		return nil
	}
	stacks, err := m.stackStore.List(project)
	if err != nil {
		return err
	}
	for _, st := range stacks {
		if st.Status != StackActive {
			continue
		}
		for _, res := range st.Resources {
			if res.Type != "instance" || res.ID == "" {
				continue
			}
			status, err := m.deps.InstanceStatus(res.ID)
			if err != nil || status != "failed" {
				continue
			}
			_ = m.deps.InstanceRestart(ctx, res.ID)
		}
	}
	return nil
}

func templateHash(tmpl StackTemplate) string {
	data, _ := json.Marshal(tmpl)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "stk_" + hex.EncodeToString(b)
}

func splitDotN(s string, n int) []string {
	idx := -1
	for i, c := range s {
		if c == '.' {
			idx = i
			break
		}
	}
	if idx < 0 || n < 2 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}
