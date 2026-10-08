package vpc

import (
	"fmt"
	"os/exec"
)

// Dataplane applies compiled networking state to the Linux host.
type Dataplane struct{}

// ApplyRouteTable programs routes for a subnet bridge (best-effort).
func (Dataplane) ApplyRouteTable(subnet Subnet, routes []Route) error {
	if subnet.BridgeName == "" {
		return nil
	}
	for _, r := range routes {
		if r.State == "blackhole" {
			continue
		}
		switch r.TargetType {
		case "local":
			continue
		case "internet-gateway", "igw":
			// default route via bridge gateway
			_ = exec.Command("ip", "route", "replace", r.DestinationCIDR, "via", subnet.GatewayIP, "dev", subnet.BridgeName).Run()
		case "nat-gateway", "nat":
			_ = exec.Command("ip", "route", "replace", r.DestinationCIDR, "via", subnet.GatewayIP, "dev", subnet.BridgeName).Run()
		default:
			if r.TargetID != "" {
				_ = exec.Command("ip", "route", "replace", r.DestinationCIDR, "via", subnet.GatewayIP, "dev", subnet.BridgeName).Run()
			}
		}
	}
	return nil
}

// ApplySecurityGroups compiles SG rules to nftables (delegates to firewall package when wired).
func (d Dataplane) ApplySecurityGroups(eniID string, sgIDs []string) error {
	if len(sgIDs) == 0 {
		return nil
	}
	_ = eniID
	return fmt.Errorf("sg dataplane apply pending full firewall integration")
}

// ReconcileSubnet marks drift when observed state differs; placeholder for resource monitor.
func (Dataplane) ReconcileSubnet(subnet Subnet) (bool, string) {
	_ = subnet
	return true, ""
}
