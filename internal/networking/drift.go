package networking

import "capper/internal/vpc"

// DriftReport summarizes desired vs observed networking state.
type DriftReport struct {
	ResourceID string `json:"resourceId"`
	Drifted    bool   `json:"drifted"`
	Reason     string `json:"reason,omitempty"`
}

// CheckSubnetDrift compares subnet desired config with dataplane observation.
func CheckSubnetDrift(sub vpc.Subnet) DriftReport {
	dp := vpc.Dataplane{}
	ok, reason := dp.ReconcileSubnet(sub)
	return DriftReport{ResourceID: sub.ID, Drifted: !ok, Reason: reason}
}

// ListVPCDrift returns drift reports for all subnets in a VPC.
func ListVPCDrift(svc *Service, project, vpcRef string) ([]DriftReport, error) {
	v, err := svc.GetVPC(project, vpcRef)
	if err != nil {
		return nil, err
	}
	subs, err := svc.VPC().ListSubnets(v.ID)
	if err != nil {
		return nil, err
	}
	var out []DriftReport
	for _, sub := range subs {
		out = append(out, CheckSubnetDrift(sub))
	}
	return out, nil
}
