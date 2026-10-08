package api

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"capper/internal/compute"
	"capper/internal/control"
	"capper/internal/controller"
	csdbackend "capper/internal/csd/backend"
	csdserver "capper/internal/csd/server"
	capstore "capper/internal/storage"
	"capper/internal/store"
	"capper/internal/vpcmover"
)

// csd returns the shared CSD server, creating and starting it on first call.
func (s *Server) csd() *csdserver.Server {
	s.csdOnce.Do(func() {
		root := filepath.Join(s.ctrl.Store.Paths.Root, "csd")
		backend, err := csdbackend.NewLocalBackend(root)
		if err != nil {
			// Log and fall through — handlers will fail gracefully.
			return
		}
		srv := csdserver.NewServer(s.ctrl.Store.CSD, backend)
		_ = srv.Start(context.Background())
		s.csdSrv = srv
	})
	return s.csdSrv
}

// Server is the Capper REST API control plane.
type Server struct {
	ctrl       controller.Controller
	project    string
	mux        *http.ServeMux
	daemon     *control.Daemon
	staticRoot string
	storage    *capstore.Manager
	vpc        *vpcmover.Runner
	// allowedOrigins is the operator-configured CORS allowlist of exact origins
	// (e.g. "https://console.example.com"). Loopback origins are always allowed.
	allowedOrigins []string
	// proxySecret, when set, enables trusted reverse-proxy identity: a request
	// carrying this secret in X-Capper-Proxy-Secret plus an X-Auth-Request-Email
	// (injected by oauth2-proxy at the edge) is authenticated as that SSO user.
	proxySecret string
	// allowedEmailDomains restricts proxy-authenticated identities to these email
	// domains (defense-in-depth alongside oauth2-proxy). Empty = no restriction.
	allowedEmailDomains []string
	// csdMounts tracks live FUSE mounts keyed by attachment ID.
	csdMounts   map[string]csdMountHandle
	csdMountsMu sync.Mutex
	// csdSrv is the persistent CSD server shared across all requests.
	// Initialised lazily via csd().
	csdSrv  *csdserver.Server
	csdOnce sync.Once
}

type csdMountHandle interface {
	Unmount() error
}

// Options configures the API server.
type Options struct {
	Project    string
	StaticRoot string
	Daemon     *control.Daemon
	// DevMode disables production-mode enforcement checks. Default false (production).
	DevMode bool
	// AllowedOrigins is the CORS allowlist of exact origins permitted to make
	// credentialed cross-origin requests. Loopback origins (localhost / 127.0.0.1
	// / ::1, any port) are always allowed regardless of this list.
	AllowedOrigins []string
	// ProxySecret enables trusted reverse-proxy (oauth2-proxy) identity when set.
	ProxySecret string
	// AllowedEmailDomains restricts proxy-authenticated SSO identities to these
	// email domains. Empty disables the server-side domain check.
	AllowedEmailDomains []string
}

// NewServer creates an API server wrapping the given controller.
func NewServer(ctrl controller.Controller, opts Options) *Server {
	if opts.Project == "" {
		opts.Project = "default"
	}
	sm := capstore.NewManager(ctrl.Store.Storage, capstore.Paths{
		Volumes:   ctrl.Store.Paths.StorageVolumes,
		Buckets:   ctrl.Store.Paths.StorageBuckets,
		Snapshots: ctrl.Store.Paths.StorageSnapshots,
	})
	if err := sm.EnsurePaths(); err != nil {
		_ = err
	}
	s := &Server{
		ctrl:                ctrl,
		project:             opts.Project,
		mux:                 http.NewServeMux(),
		daemon:              opts.Daemon,
		staticRoot:          opts.StaticRoot,
		storage:             sm,
		vpc:                 vpcmover.NewRunner(ctrl.Store.VPCMover),
		csdMounts:           make(map[string]csdMountHandle),
		allowedOrigins:      opts.AllowedOrigins,
		proxySecret:         opts.ProxySecret,
		allowedEmailDomains: opts.AllowedEmailDomains,
	}
	s.routes()

	_ = compute.NewManager(ctrl.Store.Compute).SeedStandardTypes()

	if !opts.DevMode {
		if ctrl.Store.IAM == nil {
			log.Fatal("FATAL: IAM manager is nil in production mode — S3 authorization cannot be enforced")
		}
	}

	return s
}

func (s *Server) Handler() http.Handler {
	return s.chain(s.mux)
}

func (s *Server) newHTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// ListenAndServe serves plain HTTP. Session/CSRF cookies are marked Secure, so a
// cookie-based browser session only works behind a TLS terminator; bearer tokens
// and request bodies travel in cleartext. When binding a non-loopback address
// without TLS we log a prominent warning — use ListenAndServeTLS, or front this
// with a TLS-terminating proxy, for any exposed deployment.
func (s *Server) ListenAndServe(addr string) error {
	if !addrIsLoopback(addr) {
		log.Printf("WARNING: API serving plain HTTP on non-loopback address %q — "+
			"bearer tokens and request bodies are unencrypted. Use ListenAndServeTLS "+
			"or front it with a TLS-terminating proxy.", addr)
	}
	return s.newHTTPServer(addr).ListenAndServe()
}

// ListenAndServeTLS serves HTTPS using the given certificate and key files.
func (s *Server) ListenAndServeTLS(addr, certFile, keyFile string) error {
	return s.newHTTPServer(addr).ListenAndServeTLS(certFile, keyFile)
}

// addrIsLoopback reports whether a listen address binds only the loopback
// interface (or is host-less, e.g. ":8686", which binds all interfaces → not
// loopback).
func addrIsLoopback(addr string) bool {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func (s *Server) routes() {
	// Org/account management
	s.mux.HandleFunc("GET /api/v1/orgs", s.handleListOrgs)
	s.mux.HandleFunc("POST /api/v1/orgs", s.handleCreateOrg)
	s.mux.HandleFunc("GET /api/v1/orgs/{org}", s.handleGetOrg)
	s.mux.HandleFunc("PATCH /api/v1/orgs/{org}", s.handlePatchOrg)
	s.mux.HandleFunc("DELETE /api/v1/orgs/{org}", s.handleDeleteOrg)

	s.mux.HandleFunc("GET /api/v1/orgs/{org}/accounts", s.handleListOrgAccounts)
	s.mux.HandleFunc("POST /api/v1/orgs/{org}/accounts", s.handleCreateOrgAccount)
	s.mux.HandleFunc("GET /api/v1/orgs/{org}/accounts/{account}", s.handleGetOrgAccount)
	s.mux.HandleFunc("PATCH /api/v1/orgs/{org}/accounts/{account}", s.handlePatchOrgAccount)
	s.mux.HandleFunc("DELETE /api/v1/orgs/{org}/accounts/{account}", s.handleDeleteOrgAccount)
	s.mux.HandleFunc("POST /api/v1/orgs/{org}/accounts/{account}/suspend", s.handleSuspendAccount)
	s.mux.HandleFunc("POST /api/v1/orgs/{org}/accounts/{account}/reactivate", s.handleReactivateAccount)

	s.mux.HandleFunc("GET /api/v1/orgs/{org}/root-users", s.handleListOrgRootUsers)
	s.mux.HandleFunc("POST /api/v1/orgs/{org}/root-users", s.handleAddOrgRootUser)
	s.mux.HandleFunc("DELETE /api/v1/orgs/{org}/root-users/{userID}", s.handleRemoveOrgRootUser)

	s.mux.HandleFunc("GET /api/v1/orgs/{org}/accounts/{account}/root-users", s.handleListAccountRootUsers)
	s.mux.HandleFunc("POST /api/v1/orgs/{org}/accounts/{account}/root-users", s.handleAddAccountRootUser)
	s.mux.HandleFunc("DELETE /api/v1/orgs/{org}/accounts/{account}/root-users/{userID}", s.handleRemoveAccountRootUser)

	s.mux.HandleFunc("GET /api/v1/orgs/{org}/guardrails", s.handleListGuardrails)
	s.mux.HandleFunc("POST /api/v1/orgs/{org}/guardrails", s.handleCreateGuardrail)
	s.mux.HandleFunc("GET /api/v1/orgs/{org}/guardrails/{id}", s.handleGetGuardrail)
	s.mux.HandleFunc("DELETE /api/v1/orgs/{org}/guardrails/{id}", s.handleDeleteGuardrail)

	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/version", s.handleVersion)
	s.mux.HandleFunc("GET /api/v1/openapi.json", s.handleOpenAPI)
	s.mux.HandleFunc("GET /api/v1/daemon/status", s.handleDaemonStatus)
	s.mux.HandleFunc("GET /api/v1/db/stats", s.handleDBStats)
	s.mux.HandleFunc("GET /api/v1/events", s.handleEvents)

	s.mux.HandleFunc("POST /api/v1/auth/session", s.handleAuthSession)
	s.mux.HandleFunc("DELETE /api/v1/auth/session", s.handleAuthSession)
	s.mux.HandleFunc("GET /api/v1/auth/session", s.handleAuthSessionInfo)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLocalLogin)
	s.mux.HandleFunc("GET /api/v1/auth/google/callback", s.handleGoogleCallback)

	s.mux.HandleFunc("GET /api/v1/instances", s.handleListInstances)
	s.mux.HandleFunc("POST /api/v1/instances", s.handleCreateInstance)
	s.mux.HandleFunc("GET /api/v1/instance-disk-capacity", s.handleInstanceDiskCapacity)
	s.mux.HandleFunc("GET /api/v1/instances/{id}", s.handleGetInstance)
	s.mux.HandleFunc("PATCH /api/v1/instances/{id}", s.handlePatchInstance)
	s.mux.HandleFunc("DELETE /api/v1/instances/{id}", s.handleDeleteInstance)
	s.mux.HandleFunc("POST /api/v1/instances/{id}/start", s.handleStartInstance)
	s.mux.HandleFunc("POST /api/v1/instances/{id}/stop", s.handleStopInstance)
	s.mux.HandleFunc("POST /api/v1/instances/{id}/restart", s.handleRestartInstance)
	s.mux.HandleFunc("POST /api/v1/instances/{id}/reboot", s.handleRebootInstance)
	s.mux.HandleFunc("POST /api/v1/instances/{id}/protect-termination", s.handleProtectTermination)
	s.mux.HandleFunc("DELETE /api/v1/instances/{id}/protect-termination", s.handleUnprotectTermination)
	s.mux.HandleFunc("POST /api/v1/instances/{id}/attach-network-interface", s.handleAttachENIToInstance)
	s.mux.HandleFunc("POST /api/v1/instances/{id}/detach-network-interface", s.handleDetachENIFromInstance)
	s.mux.HandleFunc("GET /api/v1/instances/{id}/logs", s.handleInstanceLogs)
	s.mux.HandleFunc("GET /api/v1/instances/{id}/logs/stdout", s.handleInstanceLogStdout)
	s.mux.HandleFunc("GET /api/v1/instances/{id}/logs/stderr", s.handleInstanceLogStderr)
	s.mux.HandleFunc("GET /api/v1/instances/{id}/events", s.handleInstanceEvents)
	s.mux.HandleFunc("GET /api/v1/instances/{id}/terminal", s.handleInstanceTerminal)
	s.mux.HandleFunc("GET /api/v1/instances/{id}/metadata", s.handleInstanceMetadata)
	s.mux.HandleFunc("PUT /api/v1/instances/{id}/metadata", s.handlePutInstanceMetadata)
	s.mux.HandleFunc("GET /api/v1/instances/{id}/metadata/{tab}", s.handleInstanceMetadataTab)

	s.mux.HandleFunc("GET /api/v1/images", s.handleListImages)
	s.mux.HandleFunc("POST /api/v1/images/import", s.handleImportImage)
	s.mux.HandleFunc("POST /api/v1/images/upload", s.handleUploadImage)
	s.mux.HandleFunc("GET /api/v1/images/{name}", s.handleGetImage)
	s.mux.HandleFunc("DELETE /api/v1/images/{name}", s.handleDeleteImage)
	s.mux.HandleFunc("POST /api/v1/images/{name}/scan", s.handleScanImage)
	s.mux.HandleFunc("GET /api/v1/images/{name}/sbom", s.handleImageSBOM)
	s.mux.HandleFunc("POST /api/v1/images/{name}/sbom", s.handleImageSBOM)
	s.mux.HandleFunc("GET /api/v1/images/{name}/provenance", s.handleImageProvenance)
	s.mux.HandleFunc("POST /api/v1/images/{name}/provenance", s.handleImageProvenance)
	s.mux.HandleFunc("POST /api/v1/images/{name}/publish", s.handlePublishImage)

	s.mux.HandleFunc("GET /api/v1/capsule-types", s.handleListCapsuleTypes)
	s.mux.HandleFunc("POST /api/v1/capsule-types", s.handleCreateCapsuleType)
	s.mux.HandleFunc("GET /api/v1/capsule-types/{name}", s.handleGetCapsuleType)
	s.mux.HandleFunc("GET /api/v1/capsule-types/{name}/audit", s.handleCapsuleTypeAudit)
	s.mux.HandleFunc("POST /api/v1/capsule-types/{name}/deprecate", s.handleDeprecateCapsuleType)
	s.mux.HandleFunc("DELETE /api/v1/capsule-types/{name}", s.handleDeleteCapsuleType)
	// instance-types alias (Instances spec)
	s.mux.HandleFunc("GET /api/v1/instance-types", s.handleListCapsuleTypes)
	s.mux.HandleFunc("GET /api/v1/instance-types/{name}", s.handleGetCapsuleType)

	// Key pairs
	s.mux.HandleFunc("GET /api/v1/key-pairs", s.handleListKeyPairs)
	s.mux.HandleFunc("POST /api/v1/key-pairs", s.handleCreateKeyPair)
	s.mux.HandleFunc("GET /api/v1/key-pairs/{keyName}", s.handleGetKeyPair)
	s.mux.HandleFunc("DELETE /api/v1/key-pairs/{keyName}", s.handleDeleteKeyPair)

	// Launch templates
	s.mux.HandleFunc("GET /api/v1/launch-templates", s.handleListLaunchTemplates)
	s.mux.HandleFunc("POST /api/v1/launch-templates", s.handleCreateLaunchTemplate)
	s.mux.HandleFunc("GET /api/v1/launch-templates/{templateId}", s.handleGetLaunchTemplate)
	s.mux.HandleFunc("GET /api/v1/launch-templates/{templateId}/versions", s.handleListLaunchTemplateVersions)
	s.mux.HandleFunc("POST /api/v1/launch-templates/{templateId}/versions", s.handleCreateLaunchTemplateVersion)

	// Reachability + advanced networking
	s.mux.HandleFunc("POST /api/v1/reachability/analyze", s.handleAnalyzeReachability)
	s.mux.HandleFunc("GET /api/v1/vpc-endpoints", s.handleListVpcEndpoints)
	s.mux.HandleFunc("POST /api/v1/vpc-endpoints", s.handleCreateVpcEndpoint)
	s.mux.HandleFunc("GET /api/v1/vpc-peerings", s.handleListVpcPeerings)
	s.mux.HandleFunc("POST /api/v1/vpc-peerings", s.handleCreateVpcPeering)
	s.mux.HandleFunc("GET /api/v1/flow-logs", s.handleListFlowLogs)
	s.mux.HandleFunc("POST /api/v1/flow-logs", s.handleCreateFlowLog)
	s.mux.HandleFunc("GET /api/v1/networking/topology", s.handleNetworkTopologyGraph)
	s.mux.HandleFunc("GET /api/v1/networking/dashboard", s.handleNetworkingDashboard)
	s.mux.HandleFunc("GET /api/v1/networking/drift", s.handleNetworkingDrift)

	// LB target groups + listeners
	s.mux.HandleFunc("GET /api/v1/target-groups", s.handleListTargetGroups)
	s.mux.HandleFunc("POST /api/v1/target-groups", s.handleCreateTargetGroup)
	s.mux.HandleFunc("GET /api/v1/subnets/{id}/available-ips", s.handleSubnetAvailableIPs)
	s.mux.HandleFunc("GET /api/v1/lb/{name}/listeners", s.handleListLBListeners)
	s.mux.HandleFunc("POST /api/v1/lb/{name}/listeners", s.handleCreateLBListener)
	s.mux.HandleFunc("GET /api/v1/lb/{name}/listeners/{id}", s.handleGetLBListener)
	s.mux.HandleFunc("PATCH /api/v1/lb/{name}/listeners/{id}", s.handleUpdateLBListener)
	s.mux.HandleFunc("DELETE /api/v1/lb/{name}/listeners/{id}", s.handleDeleteLBListener)
	s.mux.HandleFunc("POST /api/v1/lb/{name}/listeners/{id}/certificates", s.handleAttachListenerCert)
	s.mux.HandleFunc("DELETE /api/v1/lb/{name}/listeners/{id}/certificates", s.handleDetachListenerCert)
	s.mux.HandleFunc("GET /api/v1/lb/{name}/target-groups", s.handleGetLBTargetGroups)
	s.mux.HandleFunc("POST /api/v1/lb/{name}/target-groups", s.handleCreateLBTargetGroup)
	s.mux.HandleFunc("DELETE /api/v1/lb/{name}/target-groups/{tgId}", s.handleDeleteLBTargetGroup)
	s.mux.HandleFunc("GET /api/v1/lb/{name}/target-groups/{tgId}/targets", s.handleListLBTargets)
	s.mux.HandleFunc("POST /api/v1/lb/{name}/target-groups/{tgId}/targets", s.handleAddLBTarget)
	s.mux.HandleFunc("DELETE /api/v1/lb/{name}/target-groups/{tgId}/targets/{targetId}", s.handleRemoveLBTarget)

	s.mux.HandleFunc("GET /api/v1/marketplace/images", s.handleMarketplaceImages)
	s.mux.HandleFunc("GET /api/v1/marketplace/images/{id}", s.handleMarketplaceImage)
	s.mux.HandleFunc("GET /api/v1/marketplace/images/{id}/scans", s.handleMarketplaceScans)
	s.mux.HandleFunc("POST /api/v1/marketplace/images/{id}/install", s.handleMarketplaceInstall)
	s.mux.HandleFunc("POST /api/v1/marketplace/images/{id}/approve", s.handleMarketplaceApprove)
	s.mux.HandleFunc("POST /api/v1/marketplace/images/{id}/reject", s.handleMarketplaceReject)
	s.mux.HandleFunc("POST /api/v1/marketplace/images/{id}/quarantine", s.handleMarketplaceQuarantine)

	s.mux.HandleFunc("GET /api/v1/factory/status", s.handleFactoryStatus)
	s.mux.HandleFunc("GET /api/v1/factory/jobs", s.handleFactoryJobs)
	s.mux.HandleFunc("GET /api/v1/factory/jobs/{id}", s.handleFactoryJob)
	s.mux.HandleFunc("GET /api/v1/factory/images", s.handleFactoryImages)
	s.mux.HandleFunc("GET /api/v1/factory/sync/status", s.handleFactorySyncStatus)
	s.mux.HandleFunc("POST /api/v1/factory/images/{id}/push", s.handleFactoryPush)
	s.mux.HandleFunc("POST /api/v1/factory/images/{id}/rescan", s.handleFactoryRescan)

	s.mux.HandleFunc("GET /api/v1/storage/buckets", s.handleListBuckets)
	s.mux.HandleFunc("POST /api/v1/storage/buckets", s.handleCreateBucket)
	s.mux.HandleFunc("GET /api/v1/storage/buckets/{bucket}", s.handleGetBucket)
	s.mux.HandleFunc("DELETE /api/v1/storage/buckets/{bucket}", s.handleDeleteBucket)
	s.mux.HandleFunc("GET /api/v1/storage/buckets/{bucket}/objects", s.handleListObjects)
	s.mux.HandleFunc("GET /api/v1/storage/buckets/{bucket}/objects/{key...}", s.handleGetObject)
	s.mux.HandleFunc("PUT /api/v1/storage/buckets/{bucket}/objects/{key...}", s.handlePutObjectRaw)
	s.mux.HandleFunc("POST /api/v1/storage/buckets/{bucket}/objects/{key...}", s.handlePutObject)
	s.mux.HandleFunc("DELETE /api/v1/storage/buckets/{bucket}/objects/{key...}", s.handleDeleteObject)
	s.mux.HandleFunc("GET /api/v1/storage/volumes", s.handleListVolumes)
	s.mux.HandleFunc("POST /api/v1/storage/volumes", s.handleCreateVolume)
	s.mux.HandleFunc("POST /api/v1/storage/volumes/{name}/attach", s.handleAttachVolume)
	s.mux.HandleFunc("POST /api/v1/storage/volumes/{name}/detach", s.handleDetachVolume)
	s.mux.HandleFunc("DELETE /api/v1/storage/volumes/{name}", s.handleDeleteVolume)

	s.mux.HandleFunc("GET /api/v1/dns/zones", s.handleListDNSZones)
	s.mux.HandleFunc("POST /api/v1/dns/zones", s.handleCreateDNSZone)
	s.mux.HandleFunc("GET /api/v1/dns/zones/{zone}", s.handleGetDNSZone)
	s.mux.HandleFunc("DELETE /api/v1/dns/zones/{zone}", s.handleDeleteDNSZone)
	s.mux.HandleFunc("POST /api/v1/dns/zones/{zone}/records", s.handleCreateDNSRecord)
	s.mux.HandleFunc("DELETE /api/v1/dns/zones/{zone}/records/{id}", s.handleDeleteDNSRecord)
	s.mux.HandleFunc("GET /api/v1/dns/zones/{zone}/vpc-associations", s.handleListDNSZoneVPCs)
	s.mux.HandleFunc("POST /api/v1/dns/zones/{zone}/vpc-associations", s.handleAssociateDNSZoneVPC)
	s.mux.HandleFunc("DELETE /api/v1/dns/zones/{zone}/vpc-associations", s.handleDisassociateDNSZoneVPC)
	s.mux.HandleFunc("POST /api/v1/dns/query", s.handleDNSQuery)

	s.mux.HandleFunc("GET /api/v1/capinit/status", s.handleCapInitStatus)
	s.mux.HandleFunc("GET /api/v1/capinit/templates", s.handleListCapInitTemplates)
	s.mux.HandleFunc("POST /api/v1/capinit/templates", s.handleCreateCapInitTemplate)
	s.mux.HandleFunc("GET /api/v1/capinit/templates/{id}", s.handleGetCapInitTemplate)
	s.mux.HandleFunc("PUT /api/v1/capinit/templates/{id}", s.handleUpdateCapInitTemplate)
	s.mux.HandleFunc("DELETE /api/v1/capinit/templates/{id}", s.handleDeleteCapInitTemplate)
	s.mux.HandleFunc("POST /api/v1/capinit/render", s.handleCapInitRender)

	s.mux.HandleFunc("GET /api/v1/iam/users", s.handleListIAMUsers)
	s.mux.HandleFunc("POST /api/v1/iam/users", s.handleCreateIAMUser)
	s.mux.HandleFunc("DELETE /api/v1/iam/users/{name}", s.handleDeleteIAMUser)

	// RBAC user lifecycle: listing, self-identity, admin provisioning, roles.
	s.mux.HandleFunc("GET /api/v1/users", s.handleListRBACUsers)
	s.mux.HandleFunc("GET /api/v1/users/me", s.handleCurrentUser)
	s.mux.HandleFunc("PATCH /api/v1/users/me", s.handleUpdateOwnProfile)
	s.mux.HandleFunc("POST /api/v1/users/me/password", s.handleChangeOwnPassword)
	s.mux.HandleFunc("POST /api/v1/users", s.handleCreateRBACUser)
	s.mux.HandleFunc("POST /api/v1/users/{id}/password", s.handleSetUserPassword)
	s.mux.HandleFunc("POST /api/v1/users/{id}/approve", s.handleApproveUser)
	s.mux.HandleFunc("POST /api/v1/users/{id}/disable", s.handleDisableUser)
	s.mux.HandleFunc("POST /api/v1/users/{id}/roles", s.handleGrantUserRole)
	s.mux.HandleFunc("DELETE /api/v1/users/{id}/roles/{role}", s.handleRevokeUserRole)
	s.mux.HandleFunc("GET /api/v1/iam/groups", s.handleListIAMGroups)
	s.mux.HandleFunc("POST /api/v1/iam/groups", s.handleCreateIAMGroup)
	s.mux.HandleFunc("POST /api/v1/iam/groups/{group}/members", s.handleAddGroupMember)
	s.mux.HandleFunc("DELETE /api/v1/iam/groups/{group}/members/{user}", s.handleRemoveGroupMember)
	s.mux.HandleFunc("GET /api/v1/iam/roles", s.handleListIAMRoles)
	s.mux.HandleFunc("POST /api/v1/iam/roles", s.handleCreateIAMRole)
	s.mux.HandleFunc("GET /api/v1/iam/policies", s.handleListIAMPolicies)
	s.mux.HandleFunc("POST /api/v1/iam/policies", s.handleCreateIAMPolicy)
	s.mux.HandleFunc("POST /api/v1/iam/simulate", s.handleIAMSimulate)
	s.mux.HandleFunc("GET /api/v1/iam/audit", s.handleIAMAudit)
	s.mux.HandleFunc("POST /api/v1/iam/tokens", s.handleIssueToken)
	s.mux.HandleFunc("GET /api/v1/iam/tokens", s.handleListTokens)

	s.mux.HandleFunc("GET /api/v1/lb", s.handleListLBs)
	s.mux.HandleFunc("POST /api/v1/lb", s.handleCreateLB)
	s.mux.HandleFunc("GET /api/v1/lb/{name}", s.handleGetLB)
	s.mux.HandleFunc("DELETE /api/v1/lb/{name}", s.handleDeleteLB)
	s.mux.HandleFunc("POST /api/v1/lb/{name}/backends", s.handleAddLBBackend)
	s.mux.HandleFunc("DELETE /api/v1/lb/{name}/backends/{address}", s.handleRemoveLBBackend)

	s.mux.HandleFunc("GET /api/v1/firewalls", s.handleListFirewalls)
	s.mux.HandleFunc("POST /api/v1/firewalls", s.handleCreateFirewall)
	s.mux.HandleFunc("GET /api/v1/firewalls/{name}", s.handleGetFirewall)
	s.mux.HandleFunc("DELETE /api/v1/firewalls/{name}", s.handleDeleteFirewall)
	s.mux.HandleFunc("POST /api/v1/firewalls/{name}/apply", s.handleApplyFirewall)
	s.mux.HandleFunc("GET /api/v1/firewalls/{name}/rules", s.handleListFirewallRules)
	s.mux.HandleFunc("POST /api/v1/firewalls/{name}/rules", s.handleCreateFirewallRule)
	s.mux.HandleFunc("DELETE /api/v1/firewalls/{name}/rules/{id}", s.handleDeleteFirewallRule)

	s.mux.HandleFunc("GET /api/v1/health-checks", s.handleListHealthChecks)
	s.mux.HandleFunc("GET /api/v1/health-checks/{instanceId}", s.handleGetHealthCheck)

	s.mux.HandleFunc("GET /api/v1/stacks", s.handleListStacks)
	s.mux.HandleFunc("POST /api/v1/stacks", s.handleCreateStack)
	s.mux.HandleFunc("GET /api/v1/stacks/{name}", s.handleGetStack)
	s.mux.HandleFunc("DELETE /api/v1/stacks/{name}", s.handleDeleteStack)
	s.mux.HandleFunc("POST /api/v1/stacks/{name}/diff", s.handleStackDiff)

	s.mux.HandleFunc("GET /api/v1/backups", s.handleListBackups)
	s.mux.HandleFunc("POST /api/v1/backups", s.handleCreateBackup)
	s.mux.HandleFunc("POST /api/v1/backups/{id}/restore", s.handleRestoreBackup)
	s.mux.HandleFunc("GET /api/v1/backup-policies", s.handleListBackupPolicies)
	s.mux.HandleFunc("POST /api/v1/backup-policies", s.handleCreateBackupPolicy)
	s.mux.HandleFunc("DELETE /api/v1/backup-policies/{name}", s.handleDeleteBackupPolicy)

	s.mux.HandleFunc("GET /api/v1/databases", s.handleListDatabases)
	s.mux.HandleFunc("POST /api/v1/databases", s.handleCreateDatabase)
	s.mux.HandleFunc("GET /api/v1/databases/{name}", s.handleGetDatabase)
	s.mux.HandleFunc("DELETE /api/v1/databases/{name}", s.handleDeleteDatabase)

	s.mux.HandleFunc("GET /api/v1/ai/agents", s.handleListAIAgents)
	s.mux.HandleFunc("POST /api/v1/ai/agents", s.handleCreateAIAgent)
	s.mux.HandleFunc("GET /api/v1/ai/sessions", s.handleListAISessions)
	s.mux.HandleFunc("POST /api/v1/ai/sessions", s.handleCreateAISession)
	s.mux.HandleFunc("GET /api/v1/ai/mcp", s.handleListMCP)
	s.mux.HandleFunc("POST /api/v1/ai/mcp", s.handleCreateMCP)

	s.mux.HandleFunc("GET /api/v1/posture/findings", s.handleListPostureFindings)
	s.mux.HandleFunc("POST /api/v1/posture/scan", s.handlePostureScan)

	s.mux.HandleFunc("GET /api/v1/certs", s.handleListCerts)
	s.mux.HandleFunc("POST /api/v1/certs", s.handleCreateCert)
	s.mux.HandleFunc("DELETE /api/v1/certs/{name}", s.handleDeleteCert)

	s.mux.HandleFunc("GET /api/v1/kms/keys", s.handleListKMSKeys)
	s.mux.HandleFunc("POST /api/v1/kms/keys", s.handleCreateKMSKey)
	s.mux.HandleFunc("DELETE /api/v1/kms/keys/{name}", s.handleDeleteKMSKey)
	s.mux.HandleFunc("POST /api/v1/kms/keys/{name}/rotate", s.handleRotateKMSKey)
	s.mux.HandleFunc("POST /api/v1/kms/keys/{name}/encrypt", s.handleKMSEncrypt)
	s.mux.HandleFunc("POST /api/v1/kms/keys/{name}/decrypt", s.handleKMSDecrypt)

	s.mux.HandleFunc("GET /api/v1/s3/credentials", s.handleListS3Credentials)
	s.mux.HandleFunc("POST /api/v1/s3/credentials", s.handleCreateS3Credential)
	s.mux.HandleFunc("DELETE /api/v1/s3/credentials/{id}", s.handleDeleteS3Credential)

	s.mux.HandleFunc("GET /api/v1/s3/buckets/{bucket}/policy", s.handleGetBucketPolicy)
	s.mux.HandleFunc("PUT /api/v1/s3/buckets/{bucket}/policy", s.handlePutBucketPolicy)
	s.mux.HandleFunc("DELETE /api/v1/s3/buckets/{bucket}/policy", s.handleDeleteBucketPolicy)

	s.mux.HandleFunc("GET /api/v1/secrets", s.handleListSecrets)
	s.mux.HandleFunc("POST /api/v1/secrets", s.handleCreateSecret)
	s.mux.HandleFunc("GET /api/v1/secrets/{name}", s.handleGetSecret)
	s.mux.HandleFunc("DELETE /api/v1/secrets/{name}", s.handleDeleteSecret)

	s.mux.HandleFunc("GET /api/v1/governance/policies", s.handleListGovernancePolicies)
	s.mux.HandleFunc("POST /api/v1/governance/policies", s.handleCreateGovernancePolicy)
	s.mux.HandleFunc("POST /api/v1/governance/evaluate", s.handleEvaluateGovernance)

	s.mux.HandleFunc("GET /api/v1/quotas", s.handleListQuotas)
	s.mux.HandleFunc("POST /api/v1/quotas", s.handleSetQuota)

	s.mux.HandleFunc("GET /api/v1/ingress", s.handleListIngress)
	s.mux.HandleFunc("POST /api/v1/ingress", s.handleCreateIngress)
	s.mux.HandleFunc("DELETE /api/v1/ingress/{name}", s.handleDeleteIngress)

	s.mux.HandleFunc("GET /api/v1/queues", s.handleListQueues)
	s.mux.HandleFunc("POST /api/v1/queues", s.handleCreateQueue)
	s.mux.HandleFunc("DELETE /api/v1/queues/{name}", s.handleDeleteQueue)
	s.mux.HandleFunc("POST /api/v1/queues/{name}/publish", s.handlePublishQueue)
	s.mux.HandleFunc("POST /api/v1/queues/{name}/consume", s.handleConsumeQueue)

	// Compute groups
	s.mux.HandleFunc("GET /api/v1/groups", s.handleListGroups)
	s.mux.HandleFunc("POST /api/v1/groups", s.handleCreateGroup)
	s.mux.HandleFunc("GET /api/v1/groups/{name}", s.handleGetGroup)
	s.mux.HandleFunc("DELETE /api/v1/groups/{name}", s.handleDeleteGroup)
	s.mux.HandleFunc("POST /api/v1/groups/{name}/scale", s.handleScaleGroup)
	s.mux.HandleFunc("GET /api/v1/groups/{name}/instances", s.handleListGroupInstances)
	s.mux.HandleFunc("POST /api/v1/groups/{name}/reconcile", s.handleReconcileGroup)
	s.mux.HandleFunc("GET /api/v1/groups/{name}/autoscale", s.handleGetGroupAutoscale)
	s.mux.HandleFunc("POST /api/v1/groups/{name}/autoscale/disable", s.handleDisableGroupAutoscale)
	s.mux.HandleFunc("POST /api/v1/groups/{name}/autoscale/evaluate", s.handleEvaluateGroupAutoscale)
	s.mux.HandleFunc("GET /api/v1/groups/{name}/autoscale/decisions", s.handleListGroupAutoscaleDecisions)

	// Autoscale policies
	s.mux.HandleFunc("GET /api/v1/autoscale/policies", s.handleListAutoscalePolicies)
	s.mux.HandleFunc("POST /api/v1/autoscale/policies", s.handleCreateAutoscalePolicy)
	s.mux.HandleFunc("GET /api/v1/autoscale/policies/{policy}", s.handleGetAutoscalePolicy)
	s.mux.HandleFunc("PATCH /api/v1/autoscale/policies/{policy}", s.handleUpdateAutoscalePolicy)
	s.mux.HandleFunc("DELETE /api/v1/autoscale/policies/{policy}", s.handleDeleteAutoscalePolicy)

	// Custom metrics push
	s.mux.HandleFunc("POST /api/v1/metrics/custom", s.handlePushCustomMetric)

	// Topology: realms
	s.mux.HandleFunc("GET /api/v1/realms", s.handleListRealms)
	s.mux.HandleFunc("POST /api/v1/realms", s.handleCreateRealm)
	s.mux.HandleFunc("GET /api/v1/realms/{realm}", s.handleGetRealm)
	s.mux.HandleFunc("PATCH /api/v1/realms/{realm}", s.handlePatchRealm)
	s.mux.HandleFunc("DELETE /api/v1/realms/{realm}", s.handleDeleteRealm)

	// Topology: regions
	s.mux.HandleFunc("GET /api/v1/regions", s.handleListRegions)
	s.mux.HandleFunc("POST /api/v1/regions", s.handleCreateRegion)
	s.mux.HandleFunc("GET /api/v1/regions/{region}", s.handleGetRegion)
	s.mux.HandleFunc("PATCH /api/v1/regions/{region}", s.handlePatchRegion)
	s.mux.HandleFunc("DELETE /api/v1/regions/{region}", s.handleDeleteRegion)
	s.mux.HandleFunc("POST /api/v1/regions/{region}/drain", s.handleDrainRegion)
	s.mux.HandleFunc("POST /api/v1/regions/{region}/undrain", s.handleUndrainRegion)
	s.mux.HandleFunc("POST /api/v1/regions/{region}/evacuate", s.handleEvacuateRegion)
	s.mux.HandleFunc("POST /api/v1/regions/{region}/promote", s.handlePromoteRegion)

	// Topology: zones
	s.mux.HandleFunc("GET /api/v1/zones", s.handleListZones)
	s.mux.HandleFunc("POST /api/v1/zones", s.handleCreateZone)
	s.mux.HandleFunc("GET /api/v1/zones/{zone}", s.handleGetZone)
	s.mux.HandleFunc("PATCH /api/v1/zones/{zone}", s.handlePatchZone)
	s.mux.HandleFunc("DELETE /api/v1/zones/{zone}", s.handleDeleteZone)
	s.mux.HandleFunc("POST /api/v1/zones/{zone}/cordon", s.handleCordonZone)
	s.mux.HandleFunc("POST /api/v1/zones/{zone}/uncordon", s.handleUncordonZone)
	s.mux.HandleFunc("POST /api/v1/zones/{zone}/drain", s.handleDrainZone)
	s.mux.HandleFunc("POST /api/v1/zones/{zone}/undrain", s.handleUndrainZone)
	s.mux.HandleFunc("POST /api/v1/zones/{zone}/evacuate", s.handleEvacuateZone)

	// Topology: nodes
	s.mux.HandleFunc("GET /api/v1/nodes", s.handleListNodes)
	s.mux.HandleFunc("POST /api/v1/nodes", s.handleRegisterNode)
	s.mux.HandleFunc("POST /api/v1/nodes/join", s.handleNodeJoin)
	s.mux.HandleFunc("GET /api/v1/nodes/{node}", s.handleGetNode)
	s.mux.HandleFunc("PATCH /api/v1/nodes/{node}", s.handlePatchNode)
	s.mux.HandleFunc("DELETE /api/v1/nodes/{node}", s.handleDeleteNode)
	s.mux.HandleFunc("POST /api/v1/nodes/{node}/cordon", s.handleCordonNode)
	s.mux.HandleFunc("POST /api/v1/nodes/{node}/uncordon", s.handleUncordonNode)
	s.mux.HandleFunc("POST /api/v1/nodes/{node}/drain", s.handleDrainNode)
	s.mux.HandleFunc("POST /api/v1/nodes/{node}/undrain", s.handleUndrainNode)
	s.mux.HandleFunc("POST /api/v1/nodes/{node}/heartbeat", s.handleNodeHeartbeat)
	s.mux.HandleFunc("POST /api/v1/nodes/{node}/inventory", s.handleNodeInventory)
	s.mux.HandleFunc("POST /api/v1/nodes/{node}/services", s.handlePostNodeServices)
	s.mux.HandleFunc("GET /api/v1/nodes/{node}/services", s.handleListNodeServices)
	s.mux.HandleFunc("POST /api/v1/nodes/{node}/approve", s.handleApproveNode)

	// Join tokens
	s.mux.HandleFunc("GET /api/v1/join-tokens", s.handleListJoinTokens)
	s.mux.HandleFunc("POST /api/v1/join-tokens", s.handleCreateJoinToken)
	s.mux.HandleFunc("DELETE /api/v1/join-tokens/{id}", s.handleDeleteJoinToken)

	// Node pools
	s.mux.HandleFunc("GET /api/v1/node-pools", s.handleListNodePools)
	s.mux.HandleFunc("POST /api/v1/node-pools", s.handleCreateNodePool)
	s.mux.HandleFunc("GET /api/v1/node-pools/{pool}", s.handleGetNodePool)
	s.mux.HandleFunc("PATCH /api/v1/node-pools/{pool}", s.handlePatchNodePool)
	s.mux.HandleFunc("DELETE /api/v1/node-pools/{pool}", s.handleDeleteNodePool)
	s.mux.HandleFunc("POST /api/v1/node-pools/{pool}/members", s.handleAddPoolMember)
	s.mux.HandleFunc("DELETE /api/v1/node-pools/{pool}/members/{nodeID}", s.handleRemovePoolMember)
	s.mux.HandleFunc("GET /api/v1/node-pools/{pool}/members", s.handleListPoolMembers)

	// Service nodes
	s.mux.HandleFunc("GET /api/v1/service-nodes", s.handleListServiceNodes)
	s.mux.HandleFunc("GET /api/v1/service-nodes/{role}", s.handleGetServiceNodesByRole)

	// VPC (canonical unified model)
	s.mux.HandleFunc("GET /api/v1/vpcs", s.handleListVPCsUnified)
	s.mux.HandleFunc("POST /api/v1/vpcs", s.handleCreateVPCUnified)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}", s.handleGetVPCUnified)
	s.mux.HandleFunc("PATCH /api/v1/vpcs/{vpc}", s.handlePatchVPCUnified)
	s.mux.HandleFunc("DELETE /api/v1/vpcs/{vpc}", s.handleDeleteVPCUnified)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/summary", s.handleVPCSummary)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/detail", s.handleVPCDetail)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/dependencies", s.handleVPCDependencies)
	s.mux.HandleFunc("GET /api/v1/subnets/{subnetId}/dependencies", s.handleSubnetDependencies)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/subnets", s.handleListVPCSubnetsUnified)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/subnets", s.handleCreateVPCSubnetUnified)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/route-tables", s.handleListRouteTables)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/route-tables", s.handleCreateRouteTable)
	// Legacy topology flat routes (compat)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/routes", s.handleListVPCRoutes)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/routes", s.handleCreateVPCRoute)

	s.mux.HandleFunc("GET /api/v1/subnets/{subnetId}", s.handleGetSubnet)
	s.mux.HandleFunc("PATCH /api/v1/subnets/{subnetId}", s.handlePatchSubnet)
	s.mux.HandleFunc("DELETE /api/v1/subnets/{subnetId}", s.handleDeleteSubnet)
	s.mux.HandleFunc("POST /api/v1/subnets/{subnetId}/associate-route-table", s.handleAssociateSubnetRouteTable)

	s.mux.HandleFunc("GET /api/v1/route-tables/{routeTableId}", s.handleGetRouteTable)
	s.mux.HandleFunc("POST /api/v1/route-tables/{routeTableId}/routes", s.handleAddRoute)
	s.mux.HandleFunc("DELETE /api/v1/route-tables/{routeTableId}/routes/{routeId}", s.handleDeleteRoute)

	s.mux.HandleFunc("GET /api/v1/security-groups", s.handleListSecurityGroups)
	s.mux.HandleFunc("POST /api/v1/security-groups", s.handleCreateSecurityGroup)
	s.mux.HandleFunc("GET /api/v1/security-groups/{sgId}", s.handleGetSecurityGroup)
	s.mux.HandleFunc("DELETE /api/v1/security-groups/{sgId}", s.handleDeleteSecurityGroup)
	s.mux.HandleFunc("POST /api/v1/security-groups/{sgId}/rules", s.handleAddSGRule)
	s.mux.HandleFunc("DELETE /api/v1/security-group-rules/{ruleId}", s.handleDeleteSGRule)

	s.mux.HandleFunc("GET /api/v1/internet-gateways", s.handleListIGWs)
	s.mux.HandleFunc("POST /api/v1/internet-gateways", s.handleCreateIGW)
	s.mux.HandleFunc("DELETE /api/v1/internet-gateways/{igwId}", s.handleDeleteIGW)

	s.mux.HandleFunc("GET /api/v1/nat-gateways", s.handleListNATGateways)
	s.mux.HandleFunc("POST /api/v1/nat-gateways", s.handleCreateNATGateway)
	s.mux.HandleFunc("GET /api/v1/nat-gateways/{natId}", s.handleGetNATGateway)
	s.mux.HandleFunc("DELETE /api/v1/nat-gateways/{natId}", s.handleDeleteNATGateway)

	s.mux.HandleFunc("GET /api/v1/network-acls", s.handleListNetworkACLs)
	s.mux.HandleFunc("POST /api/v1/network-acls", s.handleCreateNetworkACL)
	s.mux.HandleFunc("GET /api/v1/network-acls/{aclId}", s.handleGetNetworkACL)
	s.mux.HandleFunc("DELETE /api/v1/network-acls/{aclId}", s.handleDeleteNetworkACL)
	s.mux.HandleFunc("POST /api/v1/network-acls/{aclId}/entries", s.handleAddNetworkACLEntry)
	s.mux.HandleFunc("DELETE /api/v1/network-acls/{aclId}/entries/{ruleNumber}", s.handleDeleteNetworkACLEntry)

	// ENIs
	s.mux.HandleFunc("GET /api/v1/network-interfaces", s.handleListENIs)
	s.mux.HandleFunc("POST /api/v1/network-interfaces", s.handleCreateENI)
	s.mux.HandleFunc("GET /api/v1/network-interfaces/{eniId}", s.handleGetENI)
	s.mux.HandleFunc("DELETE /api/v1/network-interfaces/{eniId}", s.handleDeleteENI)
	s.mux.HandleFunc("POST /api/v1/network-interfaces/{eniId}/attach", s.handleAttachENI)
	s.mux.HandleFunc("POST /api/v1/network-interfaces/{eniId}/detach", s.handleDetachENI)
	s.mux.HandleFunc("POST /api/v1/network-interfaces/{eniId}/private-ips", s.handleAssignENIPrivateIP)

	// Public IP aliases
	s.mux.HandleFunc("GET /api/v1/public-ips", s.handleListPublicIPs)
	s.mux.HandleFunc("POST /api/v1/public-ips/allocate", s.handleAllocatePublicIP)
	s.mux.HandleFunc("POST /api/v1/public-ips/{allocationId}/associate", s.handleAssociatePublicIP)
	s.mux.HandleFunc("POST /api/v1/public-ips/{associationId}/disassociate", s.handleDisassociatePublicIP)
	s.mux.HandleFunc("DELETE /api/v1/public-ips/{allocationId}", s.handleReleasePublicIP)

	// Topology: placement policies
	s.mux.HandleFunc("GET /api/v1/placement/policies", s.handleListPlacementPolicies)
	s.mux.HandleFunc("POST /api/v1/placement/policies", s.handleCreatePlacementPolicy)
	s.mux.HandleFunc("GET /api/v1/placement/policies/{policy}", s.handleGetPlacementPolicy)
	s.mux.HandleFunc("DELETE /api/v1/placement/policies/{policy}", s.handleDeletePlacementPolicy)

	// Scheduler
	s.mux.HandleFunc("POST /api/v1/scheduler/simulate", s.handleSchedulerSimulate)
	s.mux.HandleFunc("GET /api/v1/scheduler/capacity", s.handleSchedulerCapacity)
	s.mux.HandleFunc("GET /api/v1/scheduler/placements", s.handleSchedulerPlacements)

	// Service health and migrations
	s.mux.HandleFunc("GET /api/v1/topology/health", s.handleListServiceHealth)
	s.mux.HandleFunc("POST /api/v1/topology/health", s.handleUpsertServiceHealth)
	s.mux.HandleFunc("GET /api/v1/migrations", s.handleListMigrationPlans)
	s.mux.HandleFunc("POST /api/v1/migrations", s.handleCreateMigrationPlan)
	s.mux.HandleFunc("GET /api/v1/migrations/{plan}", s.handleGetMigrationPlan)

	// Block 11 Ph3 — global search with label and project filters
	s.mux.HandleFunc("GET /api/v1/search", s.handleSearch)

	s.mux.HandleFunc("GET /api/v1/gpu", s.handleListGPUs)
	s.mux.HandleFunc("POST /api/v1/gpu", s.handleAddGPU)
	s.mux.HandleFunc("DELETE /api/v1/gpu/{id}", s.handleDeleteGPU)
	s.mux.HandleFunc("POST /api/v1/gpu/{id}/release", s.handleReleaseGPU)
	s.mux.HandleFunc("POST /api/v1/gpu/{id}/assign", s.handleAssignGPU)

	// VPC Mobility
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/mobility/plans", s.handleCreateMobilityPlan)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/mobility/plans", s.handleListMobilityPlans)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/mobility/plans/{plan}", s.handleGetMobilityPlan)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/mobility/plans/{plan}/approve", s.handleApprovePlan)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/mobility/plans/{plan}/execute", s.handleExecutePlan)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/mobility/plans/{plan}/cancel", s.handleCancelPlan)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/mobility/plans/{plan}/dry-run", s.handleDryRunPlan)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/mobility/jobs", s.handleListMobilityJobs)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/mobility/jobs/{job}", s.handleGetMobilityJob)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/mobility/jobs/{job}/cutover", s.handleCutoverJob)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/mobility/jobs/{job}/rollback", s.handleRollbackJob)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/mobility/jobs/{job}/cancel", s.handleCancelJob)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/mobility/jobs/{job}/steps", s.handleListJobSteps)
	s.mux.HandleFunc("GET /api/v1/vpcs/{vpc}/mobility/jobs/{job}/mappings", s.handleListJobMappings)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/copy", s.handleVPCCopy)
	s.mux.HandleFunc("POST /api/v1/vpcs/{vpc}/move", s.handleVPCMove)

	// Account-scoped IAM — every route is guarded by requireAccountIAM, which
	// authorizes iam:read (GET/HEAD) or iam:write (mutations) against the account.
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/iam/users", s.requireAccountIAM(s.handleListAccountIAMUsers))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/users", s.requireAccountIAM(s.handleCreateAccountIAMUser))
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/iam/users/{userId}", s.requireAccountIAM(s.handleGetAccountIAMUser))
	s.mux.HandleFunc("PATCH /api/v1/accounts/{account}/iam/users/{userId}", s.requireAccountIAM(s.handlePatchAccountIAMUser))
	s.mux.HandleFunc("DELETE /api/v1/accounts/{account}/iam/users/{id}", s.requireAccountIAM(s.handleDeleteAccountIAMUser))
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/iam/groups", s.requireAccountIAM(s.handleListAccountIAMGroups))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/groups", s.requireAccountIAM(s.handleCreateAccountIAMGroup))
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/iam/groups/{groupId}", s.requireAccountIAM(s.handleGetAccountIAMGroup))
	s.mux.HandleFunc("PATCH /api/v1/accounts/{account}/iam/groups/{groupId}", s.requireAccountIAM(s.handlePatchAccountIAMGroup))
	s.mux.HandleFunc("DELETE /api/v1/accounts/{account}/iam/groups/{id}", s.requireAccountIAM(s.handleDeleteAccountIAMGroup))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/groups/{id}/members", s.requireAccountIAM(s.handleAddAccountGroupMember))
	s.mux.HandleFunc("DELETE /api/v1/accounts/{account}/iam/groups/{id}/members/{userID}", s.requireAccountIAM(s.handleRemoveAccountGroupMember))
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/iam/roles", s.requireAccountIAM(s.handleListAccountIAMRoles))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/roles", s.requireAccountIAM(s.handleCreateAccountIAMRole))
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/iam/roles/{roleId}", s.requireAccountIAM(s.handleGetAccountIAMRole))
	s.mux.HandleFunc("PATCH /api/v1/accounts/{account}/iam/roles/{roleId}", s.requireAccountIAM(s.handlePatchAccountIAMRole))
	s.mux.HandleFunc("DELETE /api/v1/accounts/{account}/iam/roles/{id}", s.requireAccountIAM(s.handleDeleteAccountIAMRole))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/roles/{roleId}/assume", s.requireAccountIAM(s.handleAccountAssumeRole))
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/iam/service-accounts", s.requireAccountIAM(s.handleListServiceAccounts))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/service-accounts", s.requireAccountIAM(s.handleCreateServiceAccount))
	s.mux.HandleFunc("DELETE /api/v1/accounts/{account}/iam/service-accounts/{id}", s.requireAccountIAM(s.handleDeleteServiceAccount))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/service-accounts/{id}/tokens", s.requireAccountIAM(s.handleIssueServiceAccountToken))
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/iam/policies", s.requireAccountIAM(s.handleListAccountIAMPolicies))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/policies", s.requireAccountIAM(s.handleCreateAccountIAMPolicy))
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/iam/policies/{id}", s.requireAccountIAM(s.handleGetAccountIAMPolicy))
	s.mux.HandleFunc("PUT /api/v1/accounts/{account}/iam/policies/{id}", s.requireAccountIAM(s.handleUpdateAccountIAMPolicy))
	s.mux.HandleFunc("DELETE /api/v1/accounts/{account}/iam/policies/{id}", s.requireAccountIAM(s.handleDeleteAccountIAMPolicy))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/policies/{id}/attach", s.requireAccountIAM(s.handleAttachAccountPolicy))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/policies/{id}/detach", s.requireAccountIAM(s.handleDetachAccountPolicy))
	s.mux.HandleFunc("POST /api/v1/accounts/{account}/iam/simulate", s.requireAccountIAM(s.handleAccountIAMSimulate))
	s.mux.HandleFunc("GET /api/v1/accounts/{account}/audit", s.requireAccount("audit:read", s.handleListAccountAuditEvents))

	// Cross-account role assumption (authorizes iam:assumerole inside the handler).
	s.mux.HandleFunc("POST /api/v1/iam/assume-role", s.handleAssumeRole)

	// Resource Monitor (capper-observe): unified inventory, config/drift,
	// metrics, events, and alerts.
	s.mux.HandleFunc("GET /api/v1/resources", s.handleListResources)
	s.mux.HandleFunc("POST /api/v1/resources/sync", s.handleSyncResources)
	s.mux.HandleFunc("GET /api/v1/resources/{id}", s.handleGetResource)
	s.mux.HandleFunc("GET /api/v1/resources/{id}/config", s.handleResourceConfig)
	s.mux.HandleFunc("GET /api/v1/resources/{id}/events", s.handleResourceEvents)
	s.mux.HandleFunc("GET /api/v1/resources/{id}/metrics", s.handleResourceMetrics)
	s.mux.HandleFunc("POST /api/v1/resources/{id}/drift/repair", s.handleRepairDrift)
	s.mux.HandleFunc("GET /api/v1/config/drift", s.handleListDrift)
	s.mux.HandleFunc("POST /api/v1/metrics/ingest", s.handleIngestMetric)
	s.mux.HandleFunc("GET /api/v1/metrics/query", s.handleQueryMetrics)
	s.mux.HandleFunc("POST /api/v1/resource-events", s.handleCreateResourceEvent)
	s.mux.HandleFunc("GET /api/v1/resource-events", s.handleListResourceEvents)
	s.mux.HandleFunc("GET /api/v1/alerts", s.handleListAlerts)
	s.mux.HandleFunc("GET /api/v1/alerts/rules", s.handleListAlertRules)
	s.mux.HandleFunc("POST /api/v1/alerts/rules", s.handleCreateAlertRule)
	s.mux.HandleFunc("PATCH /api/v1/alerts/rules/{id}", s.handleUpdateAlertRule)
	s.mux.HandleFunc("DELETE /api/v1/alerts/rules/{id}", s.handleDeleteAlertRule)
	s.mux.HandleFunc("POST /api/v1/alerts/{id}/ack", s.handleAckAlert)
	s.mux.HandleFunc("POST /api/v1/alerts/{id}/resolve", s.handleResolveAlert)
	// Service-specific monitoring (§6.6): latest metrics + recent events per resource.
	s.mux.HandleFunc("GET /api/v1/instances/{id}/monitoring", s.monitoringHandler("instance"))
	s.mux.HandleFunc("GET /api/v1/nodes/{id}/monitoring", s.monitoringHandler("node"))
	s.mux.HandleFunc("GET /api/v1/load-balancers/{id}/monitoring", s.monitoringHandler("load-balancer"))
	s.mux.HandleFunc("GET /api/v1/certificates/{id}/monitoring", s.monitoringHandler("certificate"))

	// Serverless Functions.
	s.mux.HandleFunc("POST /api/v1/functions", s.handleCreateFunction)
	s.mux.HandleFunc("GET /api/v1/functions", s.handleListFunctions)
	s.mux.HandleFunc("GET /api/v1/functions/{id}", s.handleGetFunction)
	s.mux.HandleFunc("PATCH /api/v1/functions/{id}", s.handlePatchFunction)
	s.mux.HandleFunc("DELETE /api/v1/functions/{id}", s.handleDeleteFunction)
	s.mux.HandleFunc("POST /api/v1/functions/{id}/versions", s.handleCreateFunctionVersion)
	s.mux.HandleFunc("GET /api/v1/functions/{id}/versions", s.handleListFunctionVersions)
	s.mux.HandleFunc("POST /api/v1/functions/{id}/invoke", s.handleInvokeFunction)
	s.mux.HandleFunc("POST /api/v1/functions/{id}/triggers", s.handleCreateTrigger)
	s.mux.HandleFunc("GET /api/v1/functions/{id}/triggers", s.handleListTriggers)
	s.mux.HandleFunc("DELETE /api/v1/functions/{id}/triggers/{triggerId}", s.handleDeleteTrigger)
	s.mux.HandleFunc("GET /api/v1/functions/{id}/invocations", s.handleListInvocations)

	// MCP Servers (managed Model Context Protocol tool servers).
	s.mux.HandleFunc("POST /api/v1/mcp/servers", s.handleCreateMCPServer)
	s.mux.HandleFunc("GET /api/v1/mcp/servers", s.handleListMCPServers)
	s.mux.HandleFunc("GET /api/v1/mcp/servers/{id}", s.handleGetMCPServer)
	s.mux.HandleFunc("DELETE /api/v1/mcp/servers/{id}", s.handleDeleteMCPServer)
	s.mux.HandleFunc("GET /api/v1/mcp/servers/{id}/tools", s.handleListMCPTools)
	s.mux.HandleFunc("POST /api/v1/mcp/servers/{id}/tools/sync", s.handleSyncMCPTools)
	s.mux.HandleFunc("POST /api/v1/mcp/servers/{id}/tools/{toolName}/invoke", s.handleInvokeMCPTool)
	s.mux.HandleFunc("GET /api/v1/mcp/servers/{id}/invocations", s.handleListMCPInvocations)
	s.mux.HandleFunc("GET /api/v1/mcp/approvals", s.handleListMCPApprovals)
	s.mux.HandleFunc("POST /api/v1/mcp/approvals/{id}/approve", s.handleApproveMCP)
	s.mux.HandleFunc("POST /api/v1/mcp/approvals/{id}/deny", s.handleDenyMCP)

	// Public IPAM (routable IP pools / Elastic IPs).
	s.mux.HandleFunc("POST /api/v1/ip-pools", s.handleCreateIPPool)
	s.mux.HandleFunc("GET /api/v1/ip-pools", s.handleListIPPools)
	s.mux.HandleFunc("GET /api/v1/ip-pools/{id}", s.handleGetIPPool)
	s.mux.HandleFunc("DELETE /api/v1/ip-pools/{id}", s.handleDeleteIPPool)
	s.mux.HandleFunc("POST /api/v1/ips/reserve", s.handleReserveIP)
	s.mux.HandleFunc("GET /api/v1/ips", s.handleListIPs)
	s.mux.HandleFunc("GET /api/v1/ips/{id}", s.handleGetIP)
	s.mux.HandleFunc("POST /api/v1/ips/{id}/release", s.handleReleaseIP)
	s.mux.HandleFunc("POST /api/v1/ips/{id}/attach", s.handleAttachIP)
	s.mux.HandleFunc("POST /api/v1/ips/{id}/detach", s.handleDetachIP)

	// Admin: routable-IP exclusions (unlist addresses from auto-allocation).
	s.mux.HandleFunc("GET /api/v1/admin/ip-exclusions", s.handleListIPExclusions)
	s.mux.HandleFunc("POST /api/v1/admin/ip-exclusions", s.handleCreateIPExclusion)
	s.mux.HandleFunc("DELETE /api/v1/admin/ip-exclusions/{id}", s.handleDeleteIPExclusion)

	// Admin: host-wide limits.
	s.mux.HandleFunc("GET /api/v1/admin/limits/host", s.handleGetHostLimits)
	s.mux.HandleFunc("PUT /api/v1/admin/limits/host", s.handleSetHostLimits)

	// Admin: host storage (physical disks, capacity pools, allocations).
	s.mux.HandleFunc("GET /api/v1/admin/disks", s.handleListDisks)
	s.mux.HandleFunc("GET /api/v1/admin/storage-pools", s.handleListStoragePools)
	s.mux.HandleFunc("POST /api/v1/admin/storage-pools", s.handleCreateStoragePool)
	s.mux.HandleFunc("DELETE /api/v1/admin/storage-pools/{id}", s.handleDeleteStoragePool)
	s.mux.HandleFunc("GET /api/v1/admin/storage-pools/{id}/allocations", s.handleListStorageAllocations)
	s.mux.HandleFunc("POST /api/v1/admin/storage-pools/{id}/allocations", s.handleCreateStorageAllocation)
	s.mux.HandleFunc("DELETE /api/v1/admin/storage-allocations/{id}", s.handleDeleteStorageAllocation)
	s.mux.HandleFunc("GET /api/v1/admin/storage/settings", s.handleGetStorageSettings)
	s.mux.HandleFunc("PUT /api/v1/admin/storage/settings", s.handleSetStorageSettings)

	// Admin: host security — node targeting (AIO local node vs Enterprise agents).
	s.mux.HandleFunc("GET /api/v1/admin/hostsec/nodes", s.handleHostsecNodes)

	// Admin: host security — fail2ban (host OS).
	s.mux.HandleFunc("GET /api/v1/admin/fail2ban/status", s.handleFail2banStatus)
	s.mux.HandleFunc("POST /api/v1/admin/fail2ban/ban", s.handleFail2banBan)
	s.mux.HandleFunc("POST /api/v1/admin/fail2ban/unban", s.handleFail2banUnban)
	s.mux.HandleFunc("POST /api/v1/admin/fail2ban/unban-all", s.handleFail2banUnbanAll)
	s.mux.HandleFunc("POST /api/v1/admin/fail2ban/flush", s.handleFail2banFlush)
	s.mux.HandleFunc("POST /api/v1/admin/fail2ban/reload", s.handleFail2banReload)
	s.mux.HandleFunc("GET /api/v1/admin/fail2ban/blocklist", s.handleFail2banBlocklist)
	s.mux.HandleFunc("POST /api/v1/admin/fail2ban/blocklist", s.handleFail2banAddBlocklist)
	s.mux.HandleFunc("DELETE /api/v1/admin/fail2ban/blocklist/{id}", s.handleFail2banRemoveBlocklist)
	s.mux.HandleFunc("GET /api/v1/admin/fail2ban/allowlist", s.handleFail2banGetAllowlist)
	s.mux.HandleFunc("PUT /api/v1/admin/fail2ban/allowlist", s.handleFail2banSetAllowlist)

	// Admin: host security — UFW firewall (host OS).
	s.mux.HandleFunc("GET /api/v1/admin/ufw/status", s.handleUFWStatus)
	s.mux.HandleFunc("POST /api/v1/admin/ufw/rules", s.handleUFWAddRule)
	s.mux.HandleFunc("DELETE /api/v1/admin/ufw/rules/{num}", s.handleUFWDeleteRule)
	s.mux.HandleFunc("GET /api/v1/admin/ufw/defaults", s.handleUFWGetDefaults)
	s.mux.HandleFunc("PUT /api/v1/admin/ufw/defaults", s.handleUFWSetDefault)
	s.mux.HandleFunc("POST /api/v1/admin/ufw/enable", s.handleUFWSetEnabled(true))
	s.mux.HandleFunc("POST /api/v1/admin/ufw/disable", s.handleUFWSetEnabled(false))

	// CSD shared volumes
	s.mux.HandleFunc("GET /api/v1/csd/volumes", s.handleListCSDVolumes)
	s.mux.HandleFunc("POST /api/v1/csd/volumes", s.handleCreateCSDVolume)
	s.mux.HandleFunc("GET /api/v1/csd/volumes/{vol}", s.handleGetCSDVolume)
	s.mux.HandleFunc("DELETE /api/v1/csd/volumes/{vol}", s.handleDeleteCSDVolume)
	s.mux.HandleFunc("POST /api/v1/csd/volumes/{vol}/attach", s.handleAttachCSDVolume)
	s.mux.HandleFunc("POST /api/v1/csd/volumes/{vol}/detach", s.handleDetachCSDVolume)
	s.mux.HandleFunc("GET /api/v1/csd/volumes/{vol}/attachments", s.handleListCSDAttachments)
	s.mux.HandleFunc("GET /api/v1/csd/volumes/{vol}/snapshots", s.handleListCSDSnapshots)
	s.mux.HandleFunc("POST /api/v1/csd/volumes/{vol}/snapshots", s.handleCreateCSDSnapshot)
	s.mux.HandleFunc("GET /api/v1/csd/volumes/{vol}/leases", s.handleListCSDLeases)
	s.mux.HandleFunc("POST /api/v1/csd/volumes/{vol}/leases/revoke", s.handleRevokeCSDLeases)
	s.mux.HandleFunc("GET /api/v1/csd/volumes/{vol}/replicas", s.handleListCSDReplicas)
	s.mux.HandleFunc("POST /api/v1/csd/volumes/{vol}/repair", s.handleRepairCSDVolume)

	// Certificate Manager routes
	s.certRoutes()

	if s.staticRoot != "" {
		s.mux.Handle("/", s.staticHandler())
	}
}

func (s *Server) staticHandler() http.Handler {
	root := s.staticRoot
	indexHTML := filepath.Join(root, "index.html")
	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		resolved, ok := resolvedWithinRoot(root, indexHTML)
		if !ok {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(resolved)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, "index.html", fi.ModTime(), f)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := r.URL.Path
		if path == "/" || path == "/index.html" {
			serveIndex(w, r)
			return
		}
		// Reject traversal without treating filepath.Clean as a sanitizer.
		if strings.Contains(path, "..") {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(path, "/")
		full := filepath.Join(root, filepath.FromSlash(rel))
		absRoot, err := filepath.Abs(root)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		absFull, err := filepath.Abs(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		sep := string(os.PathSeparator)
		if absFull != absRoot && !strings.HasPrefix(absFull, absRoot+sep) {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Lstat(absFull); err != nil {
			if os.IsNotExist(err) {
				serveIndex(w, r)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		resolved, ok := resolvedWithinRoot(root, absFull)
		if !ok {
			http.NotFound(w, r)
			return
		}
		resolvedInfo, err := os.Stat(resolved)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if resolvedInfo.IsDir() {
			// SPA fallback — never expose directory listings.
			serveIndex(w, r)
			return
		}
		http.ServeFile(w, r, resolved)
	})
}

// resolvedWithinRoot returns the symlink-resolved path when it stays inside root.
func resolvedWithinRoot(root, candidate string) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	sep := string(os.PathSeparator)
	if absCandidate != absRoot && !strings.HasPrefix(absCandidate, absRoot+sep) {
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(absCandidate)
	if err != nil {
		return "", false
	}
	if resolved != resolvedRoot && !strings.HasPrefix(resolved, resolvedRoot+sep) {
		return "", false
	}
	return resolved, true
}

func (s *Server) recordEvent(r *http.Request, resourceType, resourceID, action string, data map[string]any) {
	pt, pid := principalFromContext(r.Context())
	_ = s.ctrl.Store.Events.Insert(store.ResourceEvent{
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Action:        action,
		ProjectID:     s.project,
		PrincipalType: pt,
		PrincipalID:   pid,
		Data:          data,
	})
}
