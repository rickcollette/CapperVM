package cappersdk

import (
	"context"
	"fmt"
)

type LaunchTemplate struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	DefaultVersion int    `json:"defaultVersion"`
	LatestVersion  int    `json:"latestVersion"`
}

type KeyPair struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	KeyType     string `json:"keyType"`
}

type NetworkInterface struct {
	ID               string `json:"id"`
	VPCID            string `json:"vpcId"`
	SubnetID         string `json:"subnetId"`
	InstanceID       string `json:"instanceId,omitempty"`
	PrimaryPrivateIP string `json:"primaryPrivateIp,omitempty"`
	Status           string `json:"status"`
}

func (a *InstancesAPI) ListLaunchTemplates(ctx context.Context) ([]LaunchTemplate, error) {
	var out struct{ Data []LaunchTemplate `json:"data"` }
	err := a.c.get(ctx, "launch-templates", &out)
	return out.Data, err
}

func (a *InstancesAPI) CreateLaunchTemplate(ctx context.Context, name string, config map[string]any) (LaunchTemplate, error) {
	var out struct{ Data LaunchTemplate `json:"data"` }
	err := a.c.post(ctx, "launch-templates", map[string]any{"name": name, "config": config}, &out)
	return out.Data, err
}

func (a *InstancesAPI) ListKeyPairs(ctx context.Context) ([]KeyPair, error) {
	var out struct{ Data []KeyPair `json:"data"` }
	err := a.c.get(ctx, "key-pairs", &out)
	return out.Data, err
}

func (a *InstancesAPI) CreateKeyPair(ctx context.Context, name, publicKey string) (KeyPair, error) {
	var out struct{ Data KeyPair `json:"data"` }
	err := a.c.post(ctx, "key-pairs", map[string]any{"name": name, "publicKey": publicKey}, &out)
	return out.Data, err
}

func (a *InstancesAPI) ListNetworkInterfaces(ctx context.Context, subnetID string) ([]NetworkInterface, error) {
	path := "network-interfaces"
	if subnetID != "" {
		path = fmt.Sprintf("network-interfaces?subnetId=%s", subnetID)
	}
	var out struct{ Data []NetworkInterface `json:"data"` }
	err := a.c.get(ctx, path, &out)
	return out.Data, err
}

func (a *InstancesAPI) CreateNetworkInterface(ctx context.Context, vpcID, subnetID string, securityGroupIDs []string) (NetworkInterface, error) {
	var out struct{ Data NetworkInterface `json:"data"` }
	err := a.c.post(ctx, "network-interfaces", map[string]any{
		"vpcId": vpcID, "subnetId": subnetID, "securityGroupIds": securityGroupIDs,
	}, &out)
	return out.Data, err
}
