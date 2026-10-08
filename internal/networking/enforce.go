package networking

import (
	"fmt"

	"capper/internal/network"
	"capper/internal/vpc"
)

// ResolveSubnetForLaunch loads a subnet by ID and ensures dataplane fields exist.
func ResolveSubnetForLaunch(vpcMgr *vpc.Manager, subnetID, vpcID string) (vpc.Subnet, error) {
	if subnetID == "" {
		return vpc.Subnet{}, fmt.Errorf("subnetId is required")
	}
	sub, err := vpcMgr.GetSubnetByID(subnetID)
	if err != nil {
		return vpc.Subnet{}, fmt.Errorf("subnet not found: %w", err)
	}
	if vpcID != "" && sub.VPCID != vpcID {
		return vpc.Subnet{}, fmt.Errorf("subnet %s is not in vpc %s", subnetID, vpcID)
	}
	sub, err = EnsureSubnetBridge(vpcMgr, sub)
	if err != nil {
		return vpc.Subnet{}, err
	}
	return sub, nil
}

// EnsureSubnetBridge provisions bridge/gateway for a VPC subnet when missing.
func EnsureSubnetBridge(vpcMgr *vpc.Manager, sub vpc.Subnet) (vpc.Subnet, error) {
	if sub.BridgeName != "" && sub.GatewayIP != "" {
		return sub, nil
	}
	gw, err := network.GatewayForSubnet(sub.CIDR)
	if err != nil {
		return sub, fmt.Errorf("subnet cidr: %w", err)
	}
	bridge := sub.BridgeName
	if bridge == "" {
		bridge = network.BridgeName(sub.Name)
	}
	mode := string(sub.SubnetType)
	if mode == "" {
		mode = network.ModeNAT
	}
	if err := network.CreateBridge(bridge, gw, sub.CIDR, mode); err != nil {
		return sub, fmt.Errorf("provision subnet bridge: %w", err)
	}
	sub.BridgeName = bridge
	sub.GatewayIP = gw
	sub.Gateway = gw
	_ = vpcMgr.UpdateSubnetBridge(sub.ID, bridge, gw)
	return sub, nil
}

// DefaultSubnet picks the first subnet in the first VPC for a project (metadata fallback).
func DefaultSubnet(vpcMgr *vpc.Manager, project string) (vpc.Subnet, error) {
	vpcs, err := vpcMgr.ListVPCs(project)
	if err != nil {
		return vpc.Subnet{}, err
	}
	if len(vpcs) == 0 {
		return vpc.Subnet{}, fmt.Errorf("no vpc exists: create a vpc and subnet first")
	}
	for _, v := range vpcs {
		subs, err := vpcMgr.ListSubnets(v.ID)
		if err != nil {
			continue
		}
		if len(subs) > 0 {
			return EnsureSubnetBridge(vpcMgr, subs[0])
		}
	}
	return vpc.Subnet{}, fmt.Errorf("no subnet exists: create a subnet in a vpc first")
}
