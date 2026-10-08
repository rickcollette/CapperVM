package stack

import (
	"encoding/json"
	"fmt"
	"os"
)

type StackTemplate struct {
	Name      string         `json:"name"`
	Networks  []NetworkSpec  `json:"networks,omitempty"`
	Instances []InstanceSpec `json:"instances,omitempty"`
	LBs       []LBSpec       `json:"load_balancers,omitempty"`
	DNS       []DNSSpec      `json:"dns,omitempty"`
}

type NetworkSpec struct {
	Name   string `json:"name"`
	Subnet string `json:"subnet,omitempty"`
	Mode   string `json:"mode,omitempty"` // "nat", "bridge"
	DNS    bool   `json:"dns,omitempty"`
}

type InstanceSpec struct {
	Name     string            `json:"name"`
	Image    string            `json:"image"`
	Network  string            `json:"network,omitempty"` // deprecated
	SubnetID string            `json:"subnetId,omitempty"`
	VPCID    string            `json:"vpcId,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Restart  string            `json:"restart,omitempty"` // "always", "on-failure"
}

type LBSpec struct {
	Name    string `json:"name"`
	Mode    string `json:"mode"`    // "tcp", "http"
	Network string `json:"network,omitempty"`
	Listen  string `json:"listen"`
	Select  string `json:"select,omitempty"` // "label.role=web"
}

type DNSSpec struct {
	Zone   string   `json:"zone"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Values []string `json:"values"`
	TTL    int      `json:"ttl,omitempty"`
}

// LoadTemplate reads a JSON stack template from disk.
func LoadTemplate(path string) (StackTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StackTemplate{}, fmt.Errorf("read template: %w", err)
	}
	var tmpl StackTemplate
	if err := json.Unmarshal(data, &tmpl); err != nil {
		return StackTemplate{}, fmt.Errorf("parse template: %w", err)
	}
	if tmpl.Name == "" {
		return StackTemplate{}, fmt.Errorf("template: name is required")
	}
	return tmpl, nil
}
