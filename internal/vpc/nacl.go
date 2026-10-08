package vpc

// NetworkACL is a stateless subnet-level firewall.
type NetworkACL struct {
	ID        string `json:"id"`
	VPCID     string `json:"vpcId"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
	CreatedAt string `json:"createdAt"`
}

// NetworkACLEntry is an ordered allow/deny rule in a network ACL.
type NetworkACLEntry struct {
	ID           string `json:"id"`
	NetworkACLID string `json:"networkAclId"`
	RuleNumber   int    `json:"ruleNumber"`
	Direction    string `json:"direction"` // ingress, egress
	Action       string `json:"action"`    // allow, deny
	Protocol     string `json:"protocol"`
	CIDR         string `json:"cidr"`
	FromPort     int    `json:"fromPort,omitempty"`
	ToPort       int    `json:"toPort,omitempty"`
}
