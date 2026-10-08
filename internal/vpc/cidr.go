package vpc

import (
	"fmt"
	"net"
)

// ValidateCIDR checks syntax for an IPv4 CIDR block.
func ValidateCIDR(cidr string) error {
	if cidr == "" {
		return fmt.Errorf("cidr is required")
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid cidr %q: %w", cidr, err)
	}
	if ipNet.IP.To4() == nil {
		return fmt.Errorf("only IPv4 CIDRs are supported")
	}
	return nil
}

// ContainsCIDR reports whether child is fully contained within parent.
func ContainsCIDR(parent, child string) (bool, error) {
	if err := ValidateCIDR(parent); err != nil {
		return false, err
	}
	if err := ValidateCIDR(child); err != nil {
		return false, err
	}
	_, parentNet, _ := net.ParseCIDR(parent)
	_, childNet, _ := net.ParseCIDR(child)
	return parentNet.Contains(childNet.IP) && bytesEqual(parentNet.Mask, maskAtLeast(parentNet.Mask, childNet.Mask)), nil
}

// OverlapCIDR reports whether two CIDR blocks overlap.
func OverlapCIDR(a, b string) (bool, error) {
	if err := ValidateCIDR(a); err != nil {
		return false, err
	}
	if err := ValidateCIDR(b); err != nil {
		return false, err
	}
	if a == b {
		return true, nil
	}
	_, netA, _ := net.ParseCIDR(a)
	_, netB, _ := net.ParseCIDR(b)
	return netA.Contains(netB.IP) || netB.Contains(netA.IP), nil
}

func maskAtLeast(parent, child net.IPMask) net.IPMask {
	if len(parent) != len(child) {
		return child
	}
	for i := range parent {
		if child[i] < parent[i] {
			return child
		}
	}
	return parent
}

func bytesEqual(a, b net.IPMask) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
