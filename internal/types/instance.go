package types

type Instance struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Image         string        `json:"image"`
	ImageID       string        `json:"imageId,omitempty"`
	ImageDigest   string        `json:"imageDigest"`
	PID           int           `json:"pid"`
	Status        string        `json:"status"`
	CreatedAt     string        `json:"createdAt"`
	StartedAt     string        `json:"startedAt"`
	StoppedAt     *string       `json:"stoppedAt"`
	RootFSPath    string        `json:"rootfsPath"`
	Entrypoint    []string      `json:"entrypoint"`
	Args          []string      `json:"args"`
	Shell         string        `json:"shell"`
	User          UserConfig    `json:"user"`
	Command       string        `json:"command,omitempty"`
	Resources     ResourceLimits `json:"resources,omitempty"`
	RestartPolicy RestartPolicy  `json:"restartPolicy,omitempty"`
	RestartCount  int            `json:"restartCount,omitempty"`
	NetworkID     string            `json:"networkId,omitempty"`
	NetworkIP     string            `json:"networkIp,omitempty"`
	// VPC networking (AWS-style)
	VPCID            string            `json:"vpcId,omitempty"`
	SubnetID         string            `json:"subnetId,omitempty"`
	PrimaryENIID     string            `json:"primaryEniId,omitempty"`
	PrivateIPAddress string            `json:"privateIpAddress,omitempty"`
	PublicIPAddress  string            `json:"publicIpAddress,omitempty"`
	SecurityGroupIDs []string          `json:"securityGroupIds,omitempty"`
	InstanceType     string            `json:"instanceType,omitempty"`
	RuntimeMode      string            `json:"runtimeMode,omitempty"` // auto|bwrap|chroot|crun|runc|lxc|qemu
	KeyName          string            `json:"keyName,omitempty"`
	IAMRoleID        string            `json:"iamRoleId,omitempty"`
	TerminationProtection bool         `json:"terminationProtection,omitempty"`
	ShutdownBehavior string            `json:"shutdownBehavior,omitempty"`
	SourceDestCheck  *bool             `json:"sourceDestCheck,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	HealthCheck   *HealthCheck      `json:"healthCheck,omitempty"`

	// Topology placement fields (set by scheduler on create).
	RealmID           string `json:"realmId,omitempty"`
	RegionID          string `json:"regionId,omitempty"`
	ZoneID            string `json:"zoneId,omitempty"`
	NodeID            string `json:"nodeId,omitempty"`
	PlacementPolicyID string `json:"placementPolicyId,omitempty"`
	DesiredState      string `json:"desiredState,omitempty"`
	Generation        int    `json:"generation,omitempty"`
}

const (
	StatusCreated  = "created"  // legacy; maps to pending
	StatusPending  = "pending"
	StatusRunning  = "running"
	StatusStopping = "stopping"
	StatusStopped  = "stopped"
	StatusShutting = "shutting-down"
	StatusTerminated = "terminated"
	StatusRebooting = "rebooting"
	StatusFailed   = "failed"   // legacy; maps to error
	StatusError    = "error"
	StatusUnknown  = "unknown"
)

// PortPublish records a host→container port forwarding rule applied via iptables DNAT.
type PortPublish struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol,omitempty"`
}

type HealthCheck struct {
	Protocol  string `json:"protocol"`          // "tcp" or "http"
	Port      int    `json:"port"`
	Path      string `json:"path,omitempty"`    // for http
	Interval  int    `json:"interval"`          // seconds, default 30
	Timeout   int    `json:"timeout"`           // seconds, default 5
	Healthy   int    `json:"healthy"`           // threshold, default 2
	Unhealthy int    `json:"unhealthy"`         // threshold, default 3
}

type HealthStatus struct {
	Status    string `json:"status"`              // "healthy", "unhealthy", "unknown"
	CheckedAt string `json:"checkedAt,omitempty"`
	Message   string `json:"message,omitempty"`
}
