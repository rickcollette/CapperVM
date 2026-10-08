package lb

import (
	"fmt"

	"capper/internal/ipam"
	"capper/internal/vpc"
)

// VIPPlacer orchestrates VIP assignment for load balancers.
type VIPPlacer struct {
	IPAM *ipam.Manager
	VPC  *vpc.Store
	LB   *Store
}

// AllocateVIP reserves or assigns a VIP based on scheme.
func (p *VIPPlacer) AllocateVIP(scheme LBScheme, project, lbName, subnetID, poolID, explicitVIP string) (vip, routableIPID string, err error) {
	if p == nil {
		return "", "", fmt.Errorf("vip placer not configured")
	}
	sub, err := p.VPC.GetSubnetByID(subnetID)
	if err != nil {
		return "", "", fmt.Errorf("subnet: %w", err)
	}

	switch scheme {
	case SchemeInternetFacing:
		if p.IPAM == nil {
			return "", "", fmt.Errorf("ipam not configured for internet-facing LB")
		}
		if explicitVIP != "" {
			ips, err := p.IPAM.Store().ListIPs(poolID, string(ipam.IPAvailable))
			if err != nil {
				return "", "", err
			}
			for _, ip := range ips {
				if ip.Address == explicitVIP {
					reserved, err := p.IPAM.Reserve(ipam.ReserveOptions{
						PoolID:  poolID,
						Project: project,
						Name:    lbName + "-vip",
						Purpose: "load-balancer",
						Address: explicitVIP,
					})
					if err != nil {
						return "", "", err
					}
					return reserved.Address, reserved.ID, nil
				}
			}
			return "", "", fmt.Errorf("address %q not available in pool", explicitVIP)
		}
		reserved, err := p.IPAM.Reserve(ipam.ReserveOptions{
			PoolID:  poolID,
			Project: project,
			Name:    lbName + "-vip",
			Purpose: "load-balancer",
		})
		if err != nil {
			return "", "", err
		}
		return reserved.Address, reserved.ID, nil

	case SchemeInternal:
		if explicitVIP != "" {
			vips, _ := p.LB.ListSubnetVIPs(subnetID)
			used := append(vips, collectENIIPs(p.VPC, subnetID)...)
			for _, u := range used {
				if u == explicitVIP {
					return "", "", fmt.Errorf("address %q already in use", explicitVIP)
				}
			}
			return explicitVIP, "", nil
		}
		used := collectENIIPs(p.VPC, subnetID)
		vips, _ := p.LB.ListSubnetVIPs(subnetID)
		used = append(used, vips...)
		vpcCIDR := ""
		if v, err := p.VPC.GetVPC(sub.VPCID, ""); err == nil {
			vpcCIDR = v.CIDR
		}
		ip, err := vpc.AllocateSubnetIP(sub.CIDR, vpcCIDR, used)
		if err != nil {
			return "", "", err
		}
		return ip, "", nil

	default:
		return "", "", fmt.Errorf("unknown scheme %q", scheme)
	}
}

// AttachRoutableIP binds a reserved routable IP to an LB.
func (p *VIPPlacer) AttachRoutableIP(ipID, lbID string) error {
	if p == nil || p.IPAM == nil {
		return fmt.Errorf("ipam not configured")
	}
	_, err := p.IPAM.Attach(ipID, ipam.IPBinding{
		TargetType:  "load-balancer",
		TargetID:    lbID,
		BindingMode: "vip",
	})
	return err
}

// ReleaseVIP detaches routable IP or clears internal VIP on LB delete.
func (p *VIPPlacer) ReleaseVIP(lb LoadBalancer) error {
	if lb.RoutableIPID != "" && p != nil && p.IPAM != nil {
		_ = p.IPAM.Detach(lb.RoutableIPID)
		_ = p.IPAM.Release(lb.RoutableIPID)
	}
	return nil
}

func collectENIIPs(vpcStore *vpc.Store, subnetID string) []string {
	enis, err := vpcStore.ListENIs("")
	if err != nil {
		return nil
	}
	var used []string
	for _, e := range enis {
		if e.SubnetID != subnetID {
			continue
		}
		for _, ip := range e.PrivateIPAddresses {
			used = append(used, ip)
		}
		if e.PrimaryPrivateIP != "" {
			used = append(used, e.PrimaryPrivateIP)
		}
	}
	return used
}

// ListAvailableSubnetIPs returns free host addresses in a subnet (preview, no commit).
func ListAvailableSubnetIPs(vpcStore *vpc.Store, lbStore *Store, subnetID string, limit int) ([]string, error) {
	sub, err := vpcStore.GetSubnetByID(subnetID)
	if err != nil {
		return nil, err
	}
	used := collectENIIPs(vpcStore, subnetID)
	vips, _ := lbStore.ListSubnetVIPs(subnetID)
	used = append(used, vips...)
	vpcCIDR := ""
	if v, err := vpcStore.GetVPC(sub.VPCID, ""); err == nil {
		vpcCIDR = v.CIDR
	}
	if limit <= 0 {
		limit = 20
	}
	var out []string
	scanUsed := append([]string(nil), used...)
	for len(out) < limit {
		ip, err := vpc.AllocateSubnetIP(sub.CIDR, vpcCIDR, scanUsed)
		if err != nil {
			break
		}
		out = append(out, ip)
		scanUsed = append(scanUsed, ip)
	}
	return out, nil
}
