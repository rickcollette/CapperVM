package networking

import (
	"capper/internal/resourcemon"
	"capper/internal/vpc"
)

// RegisterVPCResources upserts resource monitor inventory for VPC subresources.
func RegisterVPCResources(rm *resourcemon.Store, project string, v vpc.VPC) {
	if rm == nil {
		return
	}
	_, _ = rm.UpsertResource(resourcemon.Resource{
		ResourceType: "vpc",
		Name:         v.Name,
		Project:      project,
		Status:       v.Status,
		RegionID:     v.HomeRegionID,
	})
}

// RegisterENIResource records an ENI in resource monitor.
func RegisterENIResource(rm *resourcemon.Store, project string, e vpc.ENI) {
	if rm == nil {
		return
	}
	_, _ = rm.UpsertResource(resourcemon.Resource{
		ResourceType: "eni",
		Name:         e.ID,
		Project:      project,
		Status:       e.Status,
		ZoneID:       e.ZoneID,
	})
}
