package vpc

// VPC is the isolated L3 network boundary for a project.
type VPC struct {
	ID                     string            `json:"id"`
	OrgID                  string            `json:"orgId,omitempty"`
	AccountID              string            `json:"accountId,omitempty"`
	Project                string            `json:"project"`
	RealmID                string            `json:"realmId,omitempty"`
	Slug                   string            `json:"slug,omitempty"`
	Name                   string            `json:"name"`
	Description            string            `json:"description,omitempty"`
	Status                 string            `json:"status"`
	StateReason            string            `json:"stateReason,omitempty"`
	PrimaryIPv4CIDR        string            `json:"primaryIpv4Cidr"`
	CIDR                   string            `json:"cidr"` // alias for primaryIpv4Cidr
	DNSDomain              string            `json:"dnsDomain,omitempty"`
	DNSSupport             bool              `json:"dnsSupport"`
	DNSHostnames           bool              `json:"dnsHostnames"`
	HomeRegionID           string            `json:"homeRegionId,omitempty"`
	DefaultSecurityGroupID string            `json:"defaultSecurityGroupId,omitempty"`
	DefaultNetworkACLID    string            `json:"defaultNetworkAclId,omitempty"`
	MainRouteTableID       string            `json:"mainRouteTableId,omitempty"`
	EnableFlowLogs         bool              `json:"enableFlowLogs"`
	MobilityPolicy         string            `json:"mobilityPolicy,omitempty"`
	Tags                   map[string]string `json:"tags,omitempty"`
	Labels                 map[string]string `json:"labels,omitempty"`
	CreatedBy              string            `json:"createdBy,omitempty"`
	CreatedAt              string            `json:"createdAt"`
	UpdatedAt              string            `json:"updatedAt,omitempty"`
	DeletedAt              string            `json:"deletedAt,omitempty"`
}

// SubnetKind controls whether traffic can route to the public internet.
type SubnetKind string

const (
	SubnetPublic   SubnetKind = "public"
	SubnetPrivate  SubnetKind = "private"
	SubnetIsolated SubnetKind = "isolated"
	SubnetLB       SubnetKind = "lb"
	SubnetService  SubnetKind = "service"
	SubnetStorage  SubnetKind = "storage"
	SubnetEdge     SubnetKind = "edge"
)

const (
	VPCStatusAvailable = "available"
	VPCStatusPending   = "pending"
	VPCStatusDeleting  = "deleting"
	VPCStatusError     = "error"
)

// Subnet is an address segment inside a VPC, backed by a Linux bridge.
type Subnet struct {
	ID                 string            `json:"id"`
	VPCID              string            `json:"vpcId"`
	RealmID            string            `json:"realmId,omitempty"`
	RegionID           string            `json:"regionId,omitempty"`
	ZoneID             string            `json:"zoneId,omitempty"`
	Zone               string            `json:"zone,omitempty"` // legacy alias for zoneId
	Slug               string            `json:"slug,omitempty"`
	Name               string            `json:"name"`
	CIDR               string            `json:"cidr"`
	SubnetType         SubnetKind        `json:"subnetType"`
	Kind               SubnetKind        `json:"kind"` // alias
	RouteTableID       string            `json:"routeTableId,omitempty"`
	NetworkACLID       string            `json:"networkAclId,omitempty"`
	AutoAssignPublicIP bool              `json:"autoAssignPublicIp"`
	AvailableIPCount   int               `json:"availableIpCount,omitempty"`
	Status             string            `json:"status,omitempty"`
	BridgeName         string            `json:"bridgeName,omitempty"`
	GatewayIP          string            `json:"gatewayIp,omitempty"`
	Gateway            string            `json:"gateway,omitempty"` // alias
	Mode               string            `json:"mode,omitempty"`
	Tags               map[string]string `json:"tags,omitempty"`
	CreatedAt          string            `json:"createdAt"`
	UpdatedAt          string            `json:"updatedAt,omitempty"`
}

// RouteTable holds routing rules for one or more subnets.
type RouteTable struct {
	ID        string `json:"id"`
	VPCID     string `json:"vpcId"`
	Name      string `json:"name"`
	IsMain    bool   `json:"isMain"`
	CreatedAt string `json:"createdAt"`
}

// Route is a single routing entry in a RouteTable.
type Route struct {
	ID              string `json:"id"`
	RouteTableID    string `json:"routeTableId"`
	DestinationCIDR string `json:"destinationCidr"`
	Destination     string `json:"destination,omitempty"` // alias
	TargetType      string `json:"targetType"`
	TargetID        string `json:"targetId"`
	Origin          string `json:"origin,omitempty"`
	State           string `json:"state,omitempty"`
}

// RouteTableAssociation links a subnet to a route table.
type RouteTableAssociation struct {
	ID           string `json:"id"`
	SubnetID     string `json:"subnetId,omitempty"`
	RouteTableID string `json:"routeTableId"`
	GatewayID    string `json:"gatewayId,omitempty"`
}

// SecurityGroup is a stateful instance/ENI firewall within a VPC.
type SecurityGroup struct {
	ID          string `json:"id"`
	VPCID       string `json:"vpcId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	DefaultDeny bool   `json:"defaultDeny"`
	IsDefault   bool   `json:"isDefault"`
	CreatedAt   string `json:"createdAt"`
}

// SGRuleDirection specifies whether a rule applies to inbound or outbound traffic.
type SGRuleDirection string

const (
	SGIngress SGRuleDirection = "ingress"
	SGEgress  SGRuleDirection = "egress"
)

// SecurityGroupRule is a single allow/deny rule within a SecurityGroup.
type SecurityGroupRule struct {
	ID              string          `json:"id"`
	SecurityGroupID string          `json:"securityGroupId"`
	Direction       SGRuleDirection `json:"direction"`
	Protocol        string          `json:"protocol"`
	FromPort        int             `json:"fromPort,omitempty"`
	ToPort          int             `json:"toPort,omitempty"`
	CIDR            string          `json:"cidr,omitempty"`
	CIDRIpv4        string          `json:"cidrIpv4,omitempty"` // alias
	SourceSGID      string          `json:"sourceSgId,omitempty"`
	Action          string          `json:"action"`
	Description     string          `json:"description,omitempty"`
}

// InternetGateway provides public internet access for a VPC.
type InternetGateway struct {
	ID            string `json:"id"`
	VPCID         string `json:"vpcId"`
	Name          string `json:"name"`
	AttachedVPCID string `json:"attachedVpcId,omitempty"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
}

// NATGateway provides outbound internet access for private subnets.
type NATGateway struct {
	ID               string `json:"id"`
	VPCID            string `json:"vpcId"`
	SubnetID         string `json:"subnetId"`
	Name             string `json:"name"`
	ConnectivityType string `json:"connectivityType"`
	PublicIP         string `json:"publicIp,omitempty"`
	AllocationID     string `json:"allocationId,omitempty"`
	PrivateIP        string `json:"privateIp,omitempty"`
	Status           string `json:"status"`
	CreatedAt        string `json:"createdAt"`
}

// VPCSummary is a dashboard aggregate for a VPC.
type VPCSummary struct {
	VPC              VPC `json:"vpc"`
	SubnetCount      int `json:"subnetCount"`
	RouteTableCount  int `json:"routeTableCount"`
	SecurityGroupCount int `json:"securityGroupCount"`
	NetworkACLCount  int `json:"networkAclCount"`
	IGWCount         int `json:"igwCount"`
	NATCount         int `json:"natCount"`
	InstanceCount    int `json:"instanceCount"`
	ENICount         int `json:"eniCount"`
}

// VPCDependencies lists resources blocking delete/move.
type VPCDependencies struct {
	VPCID         string   `json:"vpcId"`
	Subnets       []string `json:"subnets,omitempty"`
	Instances     []string `json:"instances,omitempty"`
	ENIs          []string `json:"enis,omitempty"`
	LoadBalancers []string `json:"loadBalancers,omitempty"`
	NATGateways   []string `json:"natGateways,omitempty"`
	RouteTables   []string `json:"routeTables,omitempty"`
	DNSZones      []string `json:"dnsZones,omitempty"`
	Blocked       bool     `json:"blocked"`
}

// SubnetDependencies lists resources blocking subnet delete.
type SubnetDependencies struct {
	SubnetID      string   `json:"subnetId"`
	ENIs          []string `json:"enis,omitempty"`
	Instances     []string `json:"instances,omitempty"`
	LoadBalancers []string `json:"loadBalancers,omitempty"`
	NATGateways   []string `json:"natGateways,omitempty"`
	Blocked       bool     `json:"blocked"`
}

// RouteTableDetail bundles a route table with its routes.
type RouteTableDetail struct {
	RouteTable RouteTable `json:"routeTable"`
	Routes     []Route    `json:"routes"`
}

// SecurityGroupDetail bundles a security group with its rules.
type SecurityGroupDetail struct {
	SecurityGroup SecurityGroup       `json:"securityGroup"`
	Rules         []SecurityGroupRule `json:"rules"`
}

// NetworkACLDetail bundles an ACL with entries.
type NetworkACLDetail struct {
	NetworkACL NetworkACL          `json:"networkAcl"`
	Entries    []NetworkACLEntry   `json:"entries"`
}

// VPCDetail is the aggregate view for VPC detail pages.
type VPCDetail struct {
	VPC              VPC                   `json:"vpc"`
	Subnets          []Subnet              `json:"subnets"`
	RouteTables      []RouteTableDetail    `json:"routeTables"`
	SecurityGroups   []SecurityGroupDetail `json:"securityGroups"`
	NetworkACLs      []NetworkACLDetail    `json:"networkAcls"`
	InternetGateways []InternetGateway     `json:"internetGateways"`
	NATGateways      []NATGateway          `json:"natGateways"`
	Dependencies     VPCDependencies       `json:"dependencies"`
}
