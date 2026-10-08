package vpc

import "fmt"

// CreateENI allocates a new ENI in a subnet with a primary private IP.
func (m *Manager) CreateENI(vpcID, subnetID string, sgIDs []string, privateIP string) (ENI, error) {
	sub, err := m.store.GetSubnetByID(subnetID)
	if err != nil {
		return ENI{}, err
	}
	if sub.VPCID != vpcID {
		return ENI{}, fmt.Errorf("subnet %s is not in vpc %s", subnetID, vpcID)
	}
	used, err := m.store.ListSubnetUsedIPs(subnetID)
	if err != nil {
		return ENI{}, fmt.Errorf("list subnet addresses: %w", err)
	}
	enis, err := m.store.ListENIs(vpcID)
	if err != nil {
		return ENI{}, fmt.Errorf("list enis: %w", err)
	}
	for _, e := range enis {
		used = append(used, e.PrivateIPAddresses...)
	}
	ip := privateIP
	if ip == "" {
		ip, err = AllocateSubnetIP(sub.CIDR, "", used)
		if err != nil {
			return ENI{}, err
		}
	}
	e := ENI{
		ID:                  newID("eni"),
		VPCID:               vpcID,
		SubnetID:            subnetID,
		ZoneID:              sub.ZoneID,
		MACAddress:          randomMAC(),
		SecurityGroupIDs:    sgIDs,
		SourceDestCheck:     true,
		Status:              ENIStatusAvailable,
		DeleteOnTermination: true,
		CreatedAt:           now(),
	}
	if err := m.store.InsertENI(e); err != nil {
		return ENI{}, err
	}
	if err := m.store.InsertENIPrivateIP(e.ID, ip, true); err != nil {
		return ENI{}, err
	}
	e.PrimaryPrivateIP = ip
	e.PrivateIPAddresses = []string{ip}
	return e, nil
}

func (m *Manager) GetENI(id string) (ENI, error) {
	return m.store.GetENI(id)
}

func (m *Manager) ListENIs(vpcID string) ([]ENI, error) {
	return m.store.ListENIs(vpcID)
}

func (m *Manager) AttachENI(eniID, instanceID string, index int) (ENI, error) {
	if err := m.store.UpdateENIAttachment(eniID, instanceID, index, ENIStatusInUse); err != nil {
		return ENI{}, err
	}
	return m.store.GetENI(eniID)
}

func (m *Manager) DetachENI(eniID string) (ENI, error) {
	if err := m.store.UpdateENIAttachment(eniID, "", 0, ENIStatusAvailable); err != nil {
		return ENI{}, err
	}
	return m.store.GetENI(eniID)
}

func (m *Manager) AssignENIPrivateIP(eniID, ip string, primary bool) error {
	return m.store.InsertENIPrivateIP(eniID, ip, primary)
}

func (m *Manager) DeleteENI(id string) error {
	return m.store.DeleteENI(id)
}

func (m *Manager) ListENIsByInstance(instanceID string) ([]ENI, error) {
	return m.store.ListENIsByInstance(instanceID)
}

func (m *Manager) ListENIsBySubnet(subnetID string) ([]ENI, error) {
	return m.store.ListENIsBySubnet(subnetID)
}
