package networking

// Dashboard summarizes project-wide VPC networking state.
type Dashboard struct {
	VPCCount       int          `json:"vpcCount"`
	SubnetCount    int          `json:"subnetCount"`
	DriftWarnings  []DriftReport `json:"driftWarnings"`
}

// BuildDashboard aggregates VPC/subnet counts and drift for a project.
func BuildDashboard(svc *Service, project string) (Dashboard, error) {
	vpcs, err := svc.ListVPCs(project)
	if err != nil {
		return Dashboard{}, err
	}
	dash := Dashboard{VPCCount: len(vpcs)}
	var drift []DriftReport
	for _, v := range vpcs {
		subs, err := svc.VPC().ListSubnets(v.ID)
		if err != nil {
			continue
		}
		dash.SubnetCount += len(subs)
		for _, sub := range subs {
			rep := CheckSubnetDrift(sub)
			if rep.Drifted {
				drift = append(drift, rep)
			}
		}
	}
	dash.DriftWarnings = drift
	return dash, nil
}
