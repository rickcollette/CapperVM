package lb

import "net/http"

// LBMode is the load balancing protocol.
type LBMode string

const (
	ModeTCP   LBMode = "tcp"
	ModeHTTP  LBMode = "http"
	ModeHTTPS LBMode = "https" // HTTP with required TLS termination
)

// LBScheme controls internal vs internet-facing exposure.
type LBScheme string

const (
	SchemeInternal         LBScheme = "internal"
	SchemeInternetFacing   LBScheme = "internet-facing"
)

// LBType classifies application (HTTP/HTTPS) vs network (TCP) listeners.
type LBType string

const (
	TypeApplication LBType = "application"
	TypeNetwork     LBType = "network"
)

// LBStatus tracks whether the load balancer is active.
type LBStatus string

const (
	StatusActive  LBStatus = "active"
	StatusStopped LBStatus = "stopped"
)

// LBAlgorithm controls how backends are selected.
type LBAlgorithm string

const (
	AlgoRoundRobin       LBAlgorithm = "round-robin"
	AlgoLeastConnections LBAlgorithm = "least-connections"
)

// ListenerProtocol is the front-end protocol on a listener.
type ListenerProtocol string

const (
	ProtoHTTP  ListenerProtocol = "HTTP"
	ProtoHTTPS ListenerProtocol = "HTTPS"
	ProtoTCP   ListenerProtocol = "TCP"
)

// LoadBalancer is a virtual load balancer with VIP and listeners.
type LoadBalancer struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Project       string      `json:"project"`
	VPCID         string      `json:"vpcId,omitempty"`
	NetworkID     string      `json:"networkId,omitempty"` // subnet id (legacy alias)
	SubnetID      string      `json:"subnetId,omitempty"`
	Scheme        LBScheme    `json:"scheme,omitempty"`
	Type          LBType      `json:"type,omitempty"`
	VIPAddress    string      `json:"vipAddress,omitempty"`
	RoutableIPID  string      `json:"routableIpId,omitempty"`
	ENIID         string      `json:"eniId,omitempty"`
	DNSName       string      `json:"dnsName,omitempty"`
	Mode          LBMode      `json:"mode"` // deprecated: derived from listeners
	ListenAddr    string      `json:"listenAddr"` // deprecated: use vip + listener port
	Status        LBStatus    `json:"status"`
	Algorithm     LBAlgorithm `json:"algorithm,omitempty"`
	Selector      string      `json:"selector,omitempty"`
	TLSCertName   string      `json:"tlsCertName,omitempty"` // deprecated: per-listener cert
	ServiceAlias  string      `json:"serviceAlias,omitempty"`
	CreatedAt     string      `json:"createdAt"`
}

// ACMEChallengeHandler serves http-01 challenges at /.well-known/acme-challenge/.
type ACMEChallengeHandler func(w http.ResponseWriter, r *http.Request)

// Backend is a legacy upstream endpoint on the LB (migrated into target groups).
type Backend struct {
	ID      string `json:"id"`
	LBID    string `json:"lbId"`
	Address string `json:"address"` // "ip:port"
	Healthy bool   `json:"healthy"`
}

// Target is a registered backend in a target group.
type Target struct {
	ID            string `json:"id"`
	TargetGroupID string `json:"targetGroupId"`
	Address       string `json:"address"` // ip:port
	Weight        int    `json:"weight,omitempty"`
}

// TargetGroup groups backends for an LB listener.
type TargetGroup struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Project        string `json:"project"`
	LoadBalancerID string `json:"loadBalancerId,omitempty"`
	VPCID          string `json:"vpcId"`
	Protocol       string `json:"protocol"`
	Port           int    `json:"port"`
	HealthPath     string `json:"healthPath"`
	CreatedAt      string `json:"createdAt"`
}

// Listener binds a load balancer front-end to a target group.
type Listener struct {
	ID             string `json:"id"`
	LoadBalancerID string `json:"loadBalancerId"`
	TargetGroupID  string `json:"targetGroupId"`
	Protocol       string `json:"protocol"`
	Port           int    `json:"port"`
	CertificateID  string `json:"certificateId,omitempty"`
	CreatedAt      string `json:"createdAt"`
}

// LBDetail aggregates an LB with listeners, target groups, and targets.
type LBDetail struct {
	LoadBalancer LoadBalancer  `json:"lb"`
	Listeners    []Listener    `json:"listeners"`
	TargetGroups []TargetGroup `json:"targetGroups"`
	Targets      []Target      `json:"targets"`
	Backends     []Backend     `json:"backends,omitempty"` // legacy
}

// ProxySpec describes one running proxy (one per listener, or legacy per LB).
type ProxySpec struct {
	Key           string
	LB            LoadBalancer
	Listener      Listener
	ListenAddr    string
	Mode          LBMode
	TargetGroupID string
	TLSCertName   string
}

// CreateOptions configures LB creation with optional first listener.
type CreateOptions struct {
	Name         string
	Project      string
	Scheme       LBScheme
	Type         LBType
	VPCID        string
	SubnetID     string
	VIPAddress   string
	RoutableIPID string
	Algorithm    LBAlgorithm
	Selector     string
	// Optional first listener + target group
	ListenerProtocol   string
	ListenerPort       int
	ListenerCertID     string
	TargetGroupName    string
	TargetGroupPort    int
	InitialTargetAddr  string
}
