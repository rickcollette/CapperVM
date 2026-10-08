package network

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

// Manager provides the high-level network lifecycle operations.
type Manager struct {
	store *Store
}

// NewManager creates a Manager.
func NewManager(s *Store) *Manager {
	return &Manager{store: s}
}

// CreateOptions configures network creation.
type CreateOptions struct {
	Subnet string // CIDR; default "10.42.0.0/24"
	Mode   string // "nat" | "isolated" | "host-exposed"; default "nat"
	Labels map[string]string
}

// ErrFlatNetworksRemoved is returned by Manager.Create: user-facing flat
// networks no longer exist; callers must create a VPC subnet instead. The
// bridge/IPAM/TAP helpers in this package remain for the VPC dataplane.
var ErrFlatNetworksRemoved = errors.New("flat networks removed; create a VPC subnet")

// Create is retained for API compatibility but always fails: flat networks have
// been removed in favour of VPC subnets. List/Get/Inspect keep working for any
// leftover rows.
func (m *Manager) Create(name, project string, opts CreateOptions) (Network, error) {
	return Network{}, ErrFlatNetworksRemoved
}

// Delete removes the OS bridge and the store record.
func (m *Manager) Delete(nameOrID, project string) error {
	n, err := m.store.Get(nameOrID, project)
	if err != nil {
		return err
	}
	_ = RemoveMetadataDNAT(n.Bridge, n.Gateway)
	if err := DeleteBridge(n.Bridge, n.Subnet, n.Mode); err != nil {
		return err
	}
	return m.store.Delete(n.ID)
}

// Connect allocates an IP for instanceID on the named network, creates a veth
// pair, and returns the lease.
func (m *Manager) Connect(instanceID, networkNameOrID, project, preferredIP string) (NetworkLease, error) {
	n, err := m.store.Get(networkNameOrID, project)
	if err != nil {
		return NetworkLease{}, err
	}

	lease, err := AllocateIP(m.store, n, instanceID, preferredIP)
	if err != nil {
		return NetworkLease{}, err
	}

	hostVeth, instanceVeth := VethNames(instanceID)
	if err := CreateVeth(n.Bridge, hostVeth, instanceVeth); err != nil {
		// Roll back the IP allocation on veth failure.
		_ = ReleaseIP(m.store, n.ID, instanceID)
		return NetworkLease{}, err
	}

	return lease, nil
}

// Disconnect releases the IP and removes the veth pair for the given instance.
func (m *Manager) Disconnect(instanceID, networkNameOrID, project string) error {
	n, err := m.store.Get(networkNameOrID, project)
	if err != nil {
		return err
	}

	hostVeth, _ := VethNames(instanceID)
	_ = DeleteVeth(hostVeth) // best-effort; clean up IPAM regardless

	return ReleaseIP(m.store, n.ID, instanceID)
}

// List returns networks in the given project.
func (m *Manager) List(project string) ([]Network, error) {
	return m.store.List(project)
}

// Inspect returns the network and its active leases.
func (m *Manager) Inspect(nameOrID, project string) (Network, []NetworkLease, error) {
	n, err := m.store.Get(nameOrID, project)
	if err != nil {
		return Network{}, nil, err
	}
	leases, err := m.store.LeasesForNetwork(n.ID)
	return n, leases, err
}

func newNetID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "net_" + hex.EncodeToString(b)
}
