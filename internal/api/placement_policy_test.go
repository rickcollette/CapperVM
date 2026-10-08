package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"capper/internal/store"
	"capper/internal/vpc"
)

// policyEnv bundles an authenticated server with a seeded VPC and subnet.
type policyEnv struct {
	do       func(method, path, body string) (int, string)
	store    *store.Store
	vpcID    string
	subnetID string
}

func newPolicyEnv(t *testing.T) policyEnv {
	t.Helper()
	srv, st := newTestServer(t)
	bearer := adminToken(t, st)
	v, err := st.VPC.CreateVPCExtended(vpc.CreateVPCOptions{Project: "default", Name: "pol-vpc", Slug: "pol-vpc", CIDR: "10.88.0.0/16"})
	if err != nil {
		t.Fatalf("create vpc: %v", err)
	}
	sub, err := st.VPC.CreateSubnetExtended(vpc.CreateSubnetOptions{VPCID: v.ID, Name: "pol-subnet", Slug: "pol-subnet", CIDR: "10.88.1.0/24", Kind: vpc.SubnetPrivate})
	if err != nil {
		t.Fatalf("create subnet: %v", err)
	}
	return policyEnv{
		do: func(method, path, body string) (int, string) {
			rr := doRequest(t, srv, method, path, body, map[string]string{"Authorization": "Bearer " + bearer})
			return rr.Code, rr.Body.String()
		},
		store:    st,
		vpcID:    v.ID,
		subnetID: sub.ID,
	}
}

func expectBadRequest(t *testing.T, env policyEnv, method, path, body, wantSubstr string) {
	t.Helper()
	code, resp := env.do(method, path, body)
	if code != http.StatusBadRequest {
		t.Fatalf("%s %s: want 400, got %d: %s", method, path, code, resp)
	}
	if !strings.Contains(resp, wantSubstr) {
		t.Fatalf("%s %s: want error containing %q, got %s", method, path, wantSubstr, resp)
	}
}

func TestManagedDatabaseRequiresSubnet(t *testing.T) {
	env := newPolicyEnv(t)
	expectBadRequest(t, env, "POST", "/api/v1/databases", `{"name":"db1","engine":"postgres"}`, "subnetId is required")
	// legacy networkId is treated as a subnet id and must resolve
	expectBadRequest(t, env, "POST", "/api/v1/databases", `{"name":"db1","engine":"postgres","networkId":"missing"}`, "subnet")
	expectBadRequest(t, env, "POST", "/api/v1/databases", `{"name":"db1","engine":"postgres","subnetId":"`+env.subnetID+`","vpcId":"other-vpc"}`, "not in vpc")
}

func TestPrivateDNSZoneRequiresSubnet(t *testing.T) {
	env := newPolicyEnv(t)
	expectBadRequest(t, env, "POST", "/api/v1/dns/zones", `{"name":"a.example"}`, "networkId")
	expectBadRequest(t, env, "POST", "/api/v1/dns/zones", `{"name":"a.example","type":"private"}`, "networkId")
	expectBadRequest(t, env, "POST", "/api/v1/dns/zones", `{"name":"a.example","networkId":"nope"}`, "vpc subnet")
	code, resp := env.do("POST", "/api/v1/dns/zones", `{"name":"a.example","networkId":"`+env.subnetID+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("create private zone with subnet: got %d: %s", code, resp)
	}
}

func TestENIAndNATGatewayPlacementValidation(t *testing.T) {
	env := newPolicyEnv(t)
	expectBadRequest(t, env, "POST", "/api/v1/network-interfaces", `{"subnetId":"`+env.subnetID+`"}`, "vpcId is required")
	expectBadRequest(t, env, "POST", "/api/v1/network-interfaces", `{"vpcId":"`+env.vpcID+`"}`, "subnetId is required")
	expectBadRequest(t, env, "POST", "/api/v1/network-interfaces", `{"vpcId":"other","subnetId":"`+env.subnetID+`"}`, "not in vpc")

	expectBadRequest(t, env, "POST", "/api/v1/nat-gateways", `{"subnetId":"`+env.subnetID+`"}`, "vpcId is required")
	expectBadRequest(t, env, "POST", "/api/v1/nat-gateways", `{"vpcId":"`+env.vpcID+`"}`, "subnetId is required")
}

func TestENIEndpointsRequireProjectOwnedPlacement(t *testing.T) {
	env := newPolicyEnv(t)
	foreignVPC, err := env.store.VPC.CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "other-project", Name: "foreign-vpc", Slug: "foreign-vpc", CIDR: "10.99.0.0/16",
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignSubnet, err := env.store.VPC.CreateSubnetExtended(vpc.CreateSubnetOptions{
		VPCID: foreignVPC.ID, Name: "foreign-subnet", Slug: "foreign-subnet",
		CIDR: "10.99.1.0/24", Kind: vpc.SubnetPrivate,
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignENI, err := env.store.VPC.CreateENI(foreignVPC.ID, foreignSubnet.ID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	projectENI, err := env.store.VPC.CreateENI(env.vpcID, env.subnetID, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"/api/v1/network-interfaces",
		"/api/v1/network-interfaces?vpcId=" + foreignVPC.ID,
		"/api/v1/network-interfaces?subnetId=" + foreignSubnet.ID,
	} {
		code, resp := env.do("GET", path, "")
		if path == "/api/v1/network-interfaces" {
			if code != http.StatusOK || !strings.Contains(resp, projectENI.ID) || strings.Contains(resp, foreignENI.ID) {
				t.Fatalf("unfiltered ENI list must return only project-owned ENIs, got %d: %s", code, resp)
			}
		} else if code != http.StatusNotFound {
			t.Fatalf("foreign placement list %s: want 404, got %d: %s", path, code, resp)
		}
	}

	code, resp := env.do("POST", "/api/v1/network-interfaces",
		`{"vpcId":"`+foreignVPC.ID+`","subnetId":"`+foreignSubnet.ID+`"}`)
	if code != http.StatusBadRequest || !strings.Contains(resp, "not found in project") {
		t.Fatalf("foreign ENI create: want project ownership rejection, got %d: %s", code, resp)
	}
}

func TestStackRequiresSubnets(t *testing.T) {
	env := newPolicyEnv(t)
	expectBadRequest(t, env, "POST", "/api/v1/stacks",
		`{"name":"s","instances":[{"name":"web","image":"x"}]}`, "subnetId is required")
	expectBadRequest(t, env, "POST", "/api/v1/stacks",
		`{"name":"s","instances":[{"name":"web","image":"x","subnetId":"`+env.subnetID+`"}],"load_balancers":[{"name":"lb","mode":"tcp","listen":":80","network":"n"}]}`,
		"network field is removed")
	expectBadRequest(t, env, "POST", "/api/v1/stacks",
		`{"name":"s","instances":[{"name":"web","image":"x","subnetId":"`+env.subnetID+`"}],"load_balancers":[{"name":"lb","mode":"tcp","listen":":80"}]}`,
		"subnetId is required")
	expectBadRequest(t, env, "POST", "/api/v1/stacks",
		`{"name":"s","instances":[{"name":"web","image":"x","subnetId":"missing"}]}`, "subnet not found")
}

func TestFirewallNetworkMustBeSubnet(t *testing.T) {
	env := newPolicyEnv(t)
	expectBadRequest(t, env, "POST", "/api/v1/firewalls", `{"name":"fw","network":"not-a-subnet"}`, "vpc subnet id")
	code, resp := env.do("POST", "/api/v1/firewalls", `{"name":"fw","network":"`+env.subnetID+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("create firewall on subnet: got %d: %s", code, resp)
	}
}

func TestStoragePoolGates(t *testing.T) {
	env := newPolicyEnv(t) // no default pool configured
	const want = "storage pool"
	expectBadRequest(t, env, "POST", "/api/v1/csd/volumes", `{"name":"v1","sizeBytes":1048576}`, want)
	expectBadRequest(t, env, "POST", "/api/v1/images/import", `{"path":"/tmp/x.cap"}`, want)
	expectBadRequest(t, env, "POST", "/api/v1/images/upload", ``, want)
	expectBadRequest(t, env, "POST", "/api/v1/backups", `{}`, want)
	expectBadRequest(t, env, "POST", "/api/v1/backup-policies", `{"name":"p"}`, want)

	// Buckets live in the object store and are exempt from storage pools.
	code, resp := env.do("POST", "/api/v1/storage/buckets", `{"name":"pool-exempt"}`)
	if code != http.StatusCreated {
		t.Fatalf("bucket create without pool: want 201, got %d: %s", code, resp)
	}
}

func TestSearchListsVPCsAndSubnets(t *testing.T) {
	env := newPolicyEnv(t)
	code, resp := env.do("GET", "/api/v1/search?type=vpcs,subnets&project=default", "")
	if code != http.StatusOK {
		t.Fatalf("search: got %d: %s", code, resp)
	}
	var out struct {
		Results []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var gotVPC, gotSubnet bool
	for _, r := range out.Results {
		if r.Type == "network" {
			t.Errorf("flat network result returned: %+v", r)
		}
		gotVPC = gotVPC || (r.Type == "vpc" && r.ID == env.vpcID)
		gotSubnet = gotSubnet || (r.Type == "subnet" && r.ID == env.subnetID)
	}
	if !gotVPC || !gotSubnet {
		t.Errorf("expected vpc and subnet in results, got %s", resp)
	}
}
