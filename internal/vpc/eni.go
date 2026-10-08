package vpc

// ENI is an elastic network interface attached to a subnet.
type ENI struct {
	ID                  string   `json:"id"`
	VPCID               string   `json:"vpcId"`
	SubnetID            string   `json:"subnetId"`
	ZoneID              string   `json:"zoneId,omitempty"`
	InstanceID          string   `json:"instanceId,omitempty"`
	AttachmentIndex     int      `json:"attachmentIndex,omitempty"`
	MACAddress          string   `json:"macAddress"`
	PrivateIPAddresses  []string `json:"privateIpAddresses"`
	PrimaryPrivateIP    string   `json:"privateIpAddress,omitempty"`
	PublicIPAssociation string   `json:"publicIpAssociation,omitempty"`
	SecurityGroupIDs    []string `json:"securityGroupIds"`
	SourceDestCheck     bool     `json:"sourceDestCheck"`
	Status              string   `json:"status"`
	DeleteOnTermination bool     `json:"deleteOnTermination"`
	CreatedAt           string   `json:"createdAt"`
}

const (
	ENIStatusAvailable  = "available"
	ENIStatusInUse      = "in-use"
	ENIStatusAttaching  = "attaching"
	ENIStatusDetaching  = "detaching"
	ENIStatusDeleted    = "deleted"
)
