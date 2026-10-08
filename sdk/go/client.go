// Package cappersdk provides a Go client for the Capper Control Plane REST API.
//
// Usage:
//
//	c := cappersdk.New("http://localhost:8080", "my-api-token")
//	instances, err := c.Instances.List(ctx, "default")
package cappersdk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// APIError is returned when the server responds with a 4xx or 5xx status code.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("capper api: HTTP %d: %s", e.StatusCode, e.Message)
}

// ErrNotFound is returned when the server responds with 404.
var ErrNotFound = errors.New("not found")

// ErrForbidden is returned when the server responds with 403.
var ErrForbidden = errors.New("forbidden")

// ErrUnauthorized is returned when the server responds with 401.
var ErrUnauthorized = errors.New("unauthorized")

// ErrConflict is returned when the server responds with 409.
var ErrConflict = errors.New("conflict")

// Client is the Capper API client.
type Client struct {
	base      string
	token     string
	orgID     string
	accountID string
	projectID string
	http      *http.Client
	Instances *InstancesAPI
	Networks  *NetworksAPI
	Images    *ImagesAPI
	DNS       *DNSAPI
	LB        *LBAPI
	IAM       *IAMAPI
	KMS       *KMSAPI
	S3Creds   *S3CredsAPI
	Search    *SearchAPI
	Realms    *RealmsAPI
	Regions   *RegionsAPI
	Zones     *ZonesAPI
	Nodes     *NodesAPI
	VPCs      *VPCsAPI
	Scheduler *SchedulerAPI
	Resources    *ResourceMonAPI
	Functions    *FunctionsAPI
	MCP          *MCPAPI
	IPAM         *IPAMAPI
	Firewalls    *FirewallsAPI
	Secrets      *SecretsAPI
	Databases    *DatabasesAPI
	Certificates *CertificatesAPI
	Storage      *StorageAPI
	Orgs         *OrgsAPI
	Stacks       *StacksAPI
	Queues       *QueuesAPI
	Backups      *BackupsAPI
	Quotas       *QuotasAPI
	Ingress      *IngressAPI
	AI            *AIAPI
	Autoscale     *AutoscaleAPI
	Placement     *PlacementAPI
	InstanceTypes *InstanceTypesAPI
	GPU           *GPUAPI
	Groups        *ComputeGroupsAPI
	Governance    *GovernanceAPI
	Marketplace   *MarketplaceAPI
	NodePools     *NodePoolsAPI
	Posture       *PostureAPI
	CSD           *CSDAPI
	Migrations    *MigrationsAPI
	Health        *HealthAPI
	BackupPolicies *BackupPoliciesAPI
}

// UseOrg sets the active organization for all subsequent requests.
func (c *Client) UseOrg(orgID string) *Client { c.orgID = orgID; return c }

// UseAccount sets the active account for all subsequent requests.
func (c *Client) UseAccount(accountID string) *Client { c.accountID = accountID; return c }

// UseProject sets the active project for all subsequent requests.
func (c *Client) UseProject(projectID string) *Client { c.projectID = projectID; return c }

// BaseURL returns the base URL this client is configured against.
func (c *Client) BaseURL() string { return c.base }

// New creates a Client that authenticates with the given bearer token.
func New(baseURL, token string) *Client {
	c := &Client{base: baseURL, token: token, http: &http.Client{}}
	c.Instances = &InstancesAPI{c}
	c.Networks = &NetworksAPI{c}
	c.Images = &ImagesAPI{c}
	c.DNS = &DNSAPI{c}
	c.LB = &LBAPI{c}
	c.IAM = &IAMAPI{c}
	c.KMS = &KMSAPI{c}
	c.S3Creds = &S3CredsAPI{c}
	c.Search = &SearchAPI{c}
	c.Realms = &RealmsAPI{c}
	c.Regions = &RegionsAPI{c}
	c.Zones = &ZonesAPI{c}
	c.Nodes = &NodesAPI{c}
	c.VPCs = &VPCsAPI{c}
	c.Scheduler = &SchedulerAPI{c}
	c.Resources = &ResourceMonAPI{c}
	c.Functions = &FunctionsAPI{c}
	c.MCP = &MCPAPI{c}
	c.IPAM = &IPAMAPI{c}
	c.Firewalls = &FirewallsAPI{c}
	c.Secrets = &SecretsAPI{c}
	c.Databases = &DatabasesAPI{c}
	c.Certificates = &CertificatesAPI{c}
	c.Storage = &StorageAPI{c}
	c.Orgs = &OrgsAPI{c}
	c.Stacks = &StacksAPI{c}
	c.Queues = &QueuesAPI{c}
	c.Backups = &BackupsAPI{c}
	c.Quotas = &QuotasAPI{c}
	c.Ingress = &IngressAPI{c}
	c.AI = &AIAPI{c}
	c.Autoscale = &AutoscaleAPI{c}
	c.Placement = &PlacementAPI{c}
	c.InstanceTypes = &InstanceTypesAPI{c}
	c.GPU = &GPUAPI{c}
	c.Groups = &ComputeGroupsAPI{c}
	c.Governance = &GovernanceAPI{c}
	c.Marketplace = &MarketplaceAPI{c}
	c.NodePools = &NodePoolsAPI{c}
	c.Posture = &PostureAPI{c}
	c.CSD = &CSDAPI{c}
	c.Migrations = &MigrationsAPI{c}
	c.Health = &HealthAPI{c}
	c.BackupPolicies = &BackupPoliciesAPI{c}
	return c
}

// do executes a request against /api/v1/<path> and decodes the JSON response.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		// Raw []byte bodies (e.g. function/tool invocation payloads) are sent
		// verbatim; everything else is JSON-encoded.
		if raw, ok := body.([]byte); ok {
			bodyReader = bytes.NewReader(raw)
		} else {
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			bodyReader = bytes.NewReader(b)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/v1/"+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.orgID != "" {
		req.Header.Set("X-Capper-Org-ID", c.orgID)
	}
	if c.accountID != "" {
		req.Header.Set("X-Capper-Account-ID", c.accountID)
	}
	if c.projectID != "" {
		req.Header.Set("X-Capper-Project-ID", c.projectID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		apiErr := &APIError{StatusCode: resp.StatusCode, Message: string(b)}
		switch resp.StatusCode {
		case http.StatusNotFound:
			return fmt.Errorf("%w: %s", ErrNotFound, apiErr)
		case http.StatusForbidden:
			return fmt.Errorf("%w: %s", ErrForbidden, apiErr)
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: %s", ErrUnauthorized, apiErr)
		case http.StatusConflict:
			return fmt.Errorf("%w: %s", ErrConflict, apiErr)
		}
		return apiErr
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// get is a convenience wrapper for GET requests.
func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// post is a convenience wrapper for POST requests.
func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

// del is a convenience wrapper for DELETE requests.
func (c *Client) del(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// ---- Instances --------------------------------------------------------------

type InstancesAPI struct{ c *Client }

type Instance struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Image       string            `json:"image"`
	NetworkIP   string            `json:"networkIp"`
	Labels      map[string]string `json:"labels"`
	CreatedAt   string            `json:"createdAt"`
	RuntimeMode string            `json:"runtimeMode,omitempty"`
}

func (a *InstancesAPI) List(ctx context.Context, project string) ([]Instance, error) {
	path := "instances"
	if project != "" {
		path += "?project=" + url.QueryEscape(project)
	}
	var out struct{ Data []Instance `json:"data"` }
	return out.Data, a.c.get(ctx, path, &out)
}

func (a *InstancesAPI) Get(ctx context.Context, id string) (Instance, error) {
	var out struct{ Data Instance `json:"data"` }
	return out.Data, a.c.get(ctx, "instances/"+id, &out)
}

func (a *InstancesAPI) Start(ctx context.Context, id string) error {
	return a.c.post(ctx, "instances/"+id+"/start", nil, nil)
}

func (a *InstancesAPI) Stop(ctx context.Context, id string) error {
	return a.c.post(ctx, "instances/"+id+"/stop", nil, nil)
}

func (a *InstancesAPI) Delete(ctx context.Context, id string) error {
	return a.c.del(ctx, "instances/"+id)
}

// ---- Networks ---------------------------------------------------------------

type NetworksAPI struct{ c *Client }

type Network struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`
	Project string `json:"project"`
}

func (a *NetworksAPI) List(ctx context.Context, project string) ([]Network, error) {
	path := "networks"
	if project != "" {
		path += "?project=" + url.QueryEscape(project)
	}
	var out struct{ Data []Network `json:"data"` }
	return out.Data, a.c.get(ctx, path, &out)
}

// ---- Images -----------------------------------------------------------------

type ImagesAPI struct{ c *Client }

type Image struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

func (a *ImagesAPI) List(ctx context.Context) ([]Image, error) {
	var out struct{ Data []Image `json:"data"` }
	return out.Data, a.c.get(ctx, "images", &out)
}

// ---- DNS --------------------------------------------------------------------

type DNSAPI struct{ c *Client }

type DNSZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *DNSAPI) ListZones(ctx context.Context) ([]DNSZone, error) {
	var out struct{ Data []DNSZone `json:"data"` }
	return out.Data, a.c.get(ctx, "dns/zones", &out)
}

// ---- Load Balancers ---------------------------------------------------------

type LBAPI struct{ c *Client }

type LoadBalancer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ListenAddr string `json:"listenAddr"`
	Mode       string `json:"mode"`
	Project    string `json:"project"`
}

func (a *LBAPI) List(ctx context.Context, project string) ([]LoadBalancer, error) {
	path := "lb"
	if project != "" {
		path += "?project=" + url.QueryEscape(project)
	}
	var out struct{ Data []LoadBalancer `json:"data"` }
	return out.Data, a.c.get(ctx, path, &out)
}

// ---- IAM --------------------------------------------------------------------

type IAMAPI struct{ c *Client }

type SimulateRequest struct {
	PrincipalType string `json:"principalType"`
	PrincipalID   string `json:"principalId"`
	Action        string `json:"action"`
	Resource      string `json:"resource"`
}

type SimulateResult struct {
	Decision string `json:"decision"`
	PolicyID string `json:"policyId"`
}

func (a *IAMAPI) Simulate(ctx context.Context, req SimulateRequest) (SimulateResult, error) {
	var out SimulateResult
	return out, a.c.post(ctx, "iam/simulate", req, &out)
}

// ---- Search -----------------------------------------------------------------

type SearchAPI struct{ c *Client }

type SearchResult struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Project string `json:"project"`
}

func (a *SearchAPI) Search(ctx context.Context, q, project, labelFilter, typeFilter string) ([]SearchResult, error) {
	params := url.Values{}
	if q != "" {
		params.Set("q", q)
	}
	if project != "" {
		params.Set("project", project)
	}
	if labelFilter != "" {
		params.Set("label", labelFilter)
	}
	if typeFilter != "" {
		params.Set("type", typeFilter)
	}
	path := "search"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var out struct {
		Results []SearchResult `json:"results"`
		Count   int            `json:"count"`
	}
	return out.Results, a.c.get(ctx, path, &out)
}

// ---- Instances (extended) ---------------------------------------------------

// CreateInstanceRequest is the request body for creating an instance.
type CreateInstanceRequest struct {
	Image       string            `json:"image"`
	Name        string            `json:"name,omitempty"`
	VPCID       string            `json:"vpcId,omitempty"`
	SubnetID    string            `json:"subnetId,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Command     string            `json:"command,omitempty"`
	RuntimeMode string            `json:"runtimeMode,omitempty"`
}

func (a *InstancesAPI) Create(ctx context.Context, req CreateInstanceRequest) (Instance, error) {
	var out struct{ Data Instance `json:"data"` }
	return out.Data, a.c.post(ctx, "instances", req, &out)
}

// ---- Networks (extended) ----------------------------------------------------

// CreateNetworkRequest is the request body for creating a network.
type CreateNetworkRequest struct {
	Name   string            `json:"name"`
	Subnet string            `json:"subnet,omitempty"`
	Mode   string            `json:"mode,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

func (a *NetworksAPI) Create(ctx context.Context, project string, req CreateNetworkRequest) (Network, error) {
	return Network{}, fmt.Errorf("networks API removed; create a subnet under a VPC via Subnets/VPCs APIs")
}

func (a *NetworksAPI) Delete(ctx context.Context, name string) error {
	return fmt.Errorf("networks API removed; delete the subnet via Subnets/VPCs APIs")
}

// ---- DNS (extended) ---------------------------------------------------------

// DNSRecord represents a DNS record returned by the API.
type DNSRecord struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Values []string `json:"values"`
	TTL    int      `json:"ttl"`
}

func (a *DNSAPI) CreateZone(ctx context.Context, name string) (DNSZone, error) {
	var out struct{ Data DNSZone `json:"data"` }
	return out.Data, a.c.post(ctx, "dns/zones", map[string]string{"name": name}, &out)
}

func (a *DNSAPI) DeleteZone(ctx context.Context, name string) error {
	return a.c.del(ctx, "dns/zones/"+name)
}

func (a *DNSAPI) CreateRecord(ctx context.Context, zone, name, recType, value string, ttl int) (DNSRecord, error) {
	body := map[string]any{"name": name, "type": recType, "values": []string{value}, "ttl": ttl}
	var out struct{ Data DNSRecord `json:"data"` }
	return out.Data, a.c.post(ctx, "dns/zones/"+zone+"/records", body, &out)
}

func (a *DNSAPI) DeleteRecord(ctx context.Context, zone, recordID string) error {
	return a.c.del(ctx, "dns/zones/"+zone+"/records/"+recordID)
}

// ---- Load Balancers (extended) ----------------------------------------------

type CreateLBRequest struct {
	Name       string `json:"name"`
	SubnetID   string `json:"subnetId"`
	ListenAddr string `json:"listenAddr,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Project    string `json:"project,omitempty"`
}

func (a *LBAPI) Create(ctx context.Context, req CreateLBRequest) (LoadBalancer, error) {
	var raw struct {
		Data json.RawMessage `json:"data"`
	}
	if err := a.c.post(ctx, "lb", req, &raw); err != nil {
		return LoadBalancer{}, err
	}
	var lb LoadBalancer
	if err := json.Unmarshal(raw.Data, &lb); err == nil && lb.Name != "" {
		return lb, nil
	}
	var detail struct {
		LoadBalancer LoadBalancer `json:"lb"`
	}
	if err := json.Unmarshal(raw.Data, &detail); err != nil {
		return LoadBalancer{}, err
	}
	return detail.LoadBalancer, nil
}

func (a *LBAPI) Delete(ctx context.Context, name string) error {
	return a.c.del(ctx, "lb/"+name)
}

// ---- KMS --------------------------------------------------------------------

// KMSAPI provides access to the Capper KMS.
type KMSAPI struct{ c *Client }

type KMSKey struct {
	Name      string `json:"name"`
	Algorithm string `json:"algorithm"`
	Project   string `json:"project"`
	CreatedAt string `json:"createdAt"`
}

func (a *KMSAPI) Create(ctx context.Context, name, project, algorithm string) (KMSKey, error) {
	body := map[string]string{"name": name, "project": project, "algorithm": algorithm}
	var out struct{ Data KMSKey `json:"data"` }
	return out.Data, a.c.post(ctx, "kms/keys", body, &out)
}

func (a *KMSAPI) List(ctx context.Context, project string) ([]KMSKey, error) {
	path := "kms/keys"
	if project != "" {
		path += "?project=" + url.QueryEscape(project)
	}
	var out struct{ Data []KMSKey `json:"data"` }
	return out.Data, a.c.get(ctx, path, &out)
}

func (a *KMSAPI) Encrypt(ctx context.Context, name, plaintext string) (string, error) {
	body := map[string]string{"plaintext": base64.StdEncoding.EncodeToString([]byte(plaintext))}
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	return out.Data.Ciphertext, a.c.post(ctx, "kms/keys/"+name+"/encrypt", body, &out)
}

func (a *KMSAPI) Decrypt(ctx context.Context, name, ciphertext string) (string, error) {
	body := map[string]string{"ciphertext": ciphertext}
	var out struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := a.c.post(ctx, "kms/keys/"+name+"/decrypt", body, &out); err != nil {
		return "", err
	}
	decoded, err := base64.StdEncoding.DecodeString(out.Data.Plaintext)
	return string(decoded), err
}

func (a *KMSAPI) Delete(ctx context.Context, name, project string) error {
	return a.c.del(ctx, "kms/keys/"+name+"?project="+url.QueryEscape(project))
}

// ---- S3 Credentials ---------------------------------------------------------

// S3CredsAPI provides access to S3 credential management.
type S3CredsAPI struct{ c *Client }

type S3Credential struct {
	ID        string `json:"id"`
	AccountID string `json:"accountID"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey,omitempty"`
	CreatedAt string `json:"createdAt"`
}

func (a *S3CredsAPI) Create(ctx context.Context, accountID string) (S3Credential, error) {
	body := map[string]string{"accountID": accountID}
	var out struct{ Data S3Credential `json:"data"` }
	return out.Data, a.c.post(ctx, "s3/credentials", body, &out)
}

func (a *S3CredsAPI) List(ctx context.Context, accountID string) ([]S3Credential, error) {
	path := "s3/credentials"
	if accountID != "" {
		path += "?account=" + url.QueryEscape(accountID)
	}
	var out struct{ Data []S3Credential `json:"data"` }
	return out.Data, a.c.get(ctx, path, &out)
}

func (a *S3CredsAPI) Delete(ctx context.Context, id string) error {
	return a.c.del(ctx, "s3/credentials/"+id)
}

// ---- helpers ----------------------------------------------------------------

func mergeMaps(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
