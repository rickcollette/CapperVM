package cappersdk

import (
	"context"
	"fmt"
)

// VPCNetAPI covers canonical VPC networking resources.
type VPCNetAPI struct {
	c *Client
}

func (c *Client) VPCNet() *VPCNetAPI { return &VPCNetAPI{c: c} }

type VPCResource struct {
	ID              string            `json:"id"`
	Project         string            `json:"project"`
	Name            string            `json:"name"`
	Slug            string            `json:"slug"`
	CIDR            string            `json:"cidr"`
	PrimaryIPv4CIDR string            `json:"primaryIpv4Cidr"`
	Status          string            `json:"status"`
	RealmID         string            `json:"realmId,omitempty"`
	HomeRegionID    string            `json:"homeRegionId,omitempty"`
	MobilityPolicy  string            `json:"mobilityPolicy,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       string            `json:"createdAt"`
}

type SubnetResource struct {
	ID         string `json:"id"`
	VPCID      string `json:"vpcId"`
	Name       string `json:"name"`
	CIDR       string `json:"cidr"`
	SubnetType string `json:"subnetType"`
	ZoneID     string `json:"zoneId,omitempty"`
	Status     string `json:"status,omitempty"`
}

type SecurityGroupResource struct {
	ID          string `json:"id"`
	VPCID       string `json:"vpcId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	DefaultDeny bool   `json:"defaultDeny"`
}

func (a *VPCNetAPI) ListVPCs(ctx context.Context) ([]VPCResource, error) {
	var out struct{ Data []VPCResource `json:"data"` }
	err := a.c.get(ctx, "vpcs", &out)
	return out.Data, err
}

func (a *VPCNetAPI) CreateVPC(ctx context.Context, name, cidr string) (VPCResource, error) {
	var out struct{ Data VPCResource `json:"data"` }
	err := a.c.post(ctx, "vpcs", map[string]any{"name": name, "cidr": cidr}, &out)
	return out.Data, err
}

func (a *VPCNetAPI) GetVPC(ctx context.Context, ref string) (VPCResource, error) {
	var out struct{ Data VPCResource `json:"data"` }
	err := a.c.get(ctx, fmt.Sprintf("vpcs/%s", ref), &out)
	return out.Data, err
}

func (a *VPCNetAPI) DeleteVPC(ctx context.Context, ref string) error {
	return a.c.del(ctx, fmt.Sprintf("vpcs/%s", ref))
}

func (a *VPCNetAPI) ListSubnets(ctx context.Context, vpcRef string) ([]SubnetResource, error) {
	var out struct{ Data []SubnetResource `json:"data"` }
	err := a.c.get(ctx, fmt.Sprintf("vpcs/%s/subnets", vpcRef), &out)
	return out.Data, err
}

func (a *VPCNetAPI) CreateSubnet(ctx context.Context, vpcRef, name, cidr, subnetType string) (SubnetResource, error) {
	var out struct{ Data SubnetResource `json:"data"` }
	err := a.c.post(ctx, fmt.Sprintf("vpcs/%s/subnets", vpcRef), map[string]any{
		"name": name, "cidr": cidr, "subnetType": subnetType,
	}, &out)
	return out.Data, err
}

func (a *VPCNetAPI) ListSecurityGroups(ctx context.Context, vpcID string) ([]SecurityGroupResource, error) {
	var out struct{ Data []SecurityGroupResource `json:"data"` }
	err := a.c.get(ctx, fmt.Sprintf("security-groups?vpcId=%s", vpcID), &out)
	return out.Data, err
}

func (a *VPCNetAPI) CreateSecurityGroup(ctx context.Context, vpcID, name, desc string) (SecurityGroupResource, error) {
	var out struct{ Data SecurityGroupResource `json:"data"` }
	err := a.c.post(ctx, "security-groups", map[string]any{
		"vpcId": vpcID, "name": name, "description": desc,
	}, &out)
	return out.Data, err
}

func (a *VPCNetAPI) ListRouteTables(ctx context.Context, vpcRef string) ([]map[string]any, error) {
	var out struct{ Data []map[string]any `json:"data"` }
	err := a.c.get(ctx, fmt.Sprintf("vpcs/%s/route-tables", vpcRef), &out)
	return out.Data, err
}

func (a *VPCNetAPI) AnalyzeReachability(ctx context.Context, body map[string]any) (map[string]any, error) {
	var out struct{ Data map[string]any `json:"data"` }
	err := a.c.post(ctx, "reachability/analyze", body, &out)
	return out.Data, err
}
