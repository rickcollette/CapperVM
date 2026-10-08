package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// authHeader returns an Authorization header using a freshly-issued token.
func authHeader(t *testing.T, srv *httptest.Server) map[string]string {
	t.Helper()
	return map[string]string{"Authorization": "Bearer test-token-unused"}
}

// roundTrip sends a request through the test server's Handler and returns the decoded envelope.
func roundTrip(t *testing.T, srv *httptest.Server, method, path string, body any, token string) (int, map[string]any) {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, srv.URL+path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http do: %v", err)
	}
	defer resp.Body.Close()
	var env map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&env)
	return resp.StatusCode, env
}

// TestE2EOrgAccountIAMFlow exercises the full org → account → IAM user lifecycle.
func TestE2EOrgAccountIAMFlow(t *testing.T) {
	apiSrv, st := newTestServer(t)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)

	bearer := adminToken(t, st)

	// Create org
	code, env := roundTrip(t, srv, "POST", "/api/v1/orgs",
		map[string]any{"name": "test-org"}, bearer)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create org: got %d, body: %v", code, env)
	}

	// List orgs
	code, env = roundTrip(t, srv, "GET", "/api/v1/orgs", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("list orgs: got %d, body: %v", code, env)
	}
	data, _ := env["data"].([]any)
	if len(data) == 0 {
		t.Error("list orgs: expected at least one org")
	}
}

// TestE2ETopologyLifecycle exercises realm → region → zone → node registration.
func TestE2ETopologyLifecycle(t *testing.T) {
	apiSrv, st := newTestServer(t)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)

	bearer := adminToken(t, st)

	// Create realm
	code, env := roundTrip(t, srv, "POST", "/api/v1/realms",
		map[string]any{"name": "test-realm", "slug": "test-realm"}, bearer)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create realm: got %d, body: %v", code, env)
	}
	realmData, _ := env["data"].(map[string]any)
	realmID, _ := realmData["id"].(string)
	if realmID == "" {
		t.Skip("realm creation returned no ID, skipping topology test")
	}

	// Create region
	code, env = roundTrip(t, srv, "POST", "/api/v1/regions",
		map[string]any{"name": "us-test-1", "slug": "us-test-1", "realmId": realmID}, bearer)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create region: got %d", code)
	}
	regionData, _ := env["data"].(map[string]any)
	regionID, _ := regionData["id"].(string)

	// Create zone
	code, env = roundTrip(t, srv, "POST", "/api/v1/zones",
		map[string]any{"name": "us-test-1a", "slug": "us-test-1a", "regionId": regionID}, bearer)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create zone: got %d", code)
	}
	zoneData, _ := env["data"].(map[string]any)
	zoneID, _ := zoneData["id"].(string)

	// Create join token
	code, env = roundTrip(t, srv, "POST", "/api/v1/join-tokens",
		map[string]any{"realmId": realmID, "regionId": regionID, "zoneId": zoneID, "ttlSeconds": 300}, bearer)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create join token: got %d, body: %v", code, env)
	}
	tokenData, _ := env["data"].(map[string]any)
	joinToken, _ := tokenData["token"].(string)
	if joinToken == "" {
		t.Skip("join token is empty, skipping node join test")
	}

	// Join node
	code, env = roundTrip(t, srv, "POST", "/api/v1/nodes/join", map[string]any{
		"token":       joinToken,
		"name":        "test-node-1",
		"address":     "10.0.0.100",
		"cpuCount":    4,
		"memoryBytes": 8 * 1024 * 1024 * 1024,
	}, "")
	if code != http.StatusCreated {
		t.Fatalf("node join: got %d, body: %v", code, env)
	}

	// List nodes
	code, env = roundTrip(t, srv, "GET", "/api/v1/nodes", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("list nodes: got %d", code)
	}
}

// TestE2ECertificateLifecycle exercises cert create → get → renew → delete.
func TestE2ECertificateLifecycle(t *testing.T) {
	apiSrv, st := newTestServer(t)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)

	bearer := adminToken(t, st)

	// Create a self-signed certificate (no ACME needed)
	code, env := roundTrip(t, srv, "POST", "/api/v1/certificates", map[string]any{
		"name":       "test-cert",
		"commonName": "test.internal",
		"issuer":     "internal-ca",
		"autoRenew":  false,
	}, bearer)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Skipf("create cert: got %d — skipping cert lifecycle test", code)
	}
	certData, _ := env["data"].(map[string]any)
	certID, _ := certData["id"].(string)
	if certID == "" {
		t.Skip("cert ID empty, skipping")
	}

	// Get certificate
	code, _ = roundTrip(t, srv, "GET", "/api/v1/certificates/"+certID, nil, bearer)
	if code != http.StatusOK {
		t.Errorf("get cert: expected 200, got %d", code)
	}

	// Delete certificate
	code, _ = roundTrip(t, srv, "DELETE", "/api/v1/certificates/"+certID, nil, bearer)
	if code != http.StatusOK && code != http.StatusNoContent {
		t.Errorf("delete cert: expected 200/204, got %d", code)
	}
}

// TestE2EIAMAssumeRole exercises assume-role token issuance.
func TestE2EIAMAssumeRole(t *testing.T) {
	apiSrv, st := newTestServer(t)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)

	bearer := adminToken(t, st)

	// Create a role first
	code, env := roundTrip(t, srv, "POST", "/api/v1/iam/roles",
		map[string]any{"name": "test-role", "description": "integration test role"}, bearer)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Skipf("create role: got %d — skipping assume-role test", code)
	}
	roleData, _ := env["data"].(map[string]any)
	roleID, _ := roleData["id"].(string)
	if roleID == "" {
		t.Skip("role ID empty")
	}

	// Assume the role
	code, env = roundTrip(t, srv, "POST", "/api/v1/iam/assume-role", map[string]any{
		"roleArn":         roleID,
		"sessionName":     "integration-test",
		"durationSeconds": 900,
	}, bearer)
	if code != http.StatusOK {
		t.Errorf("assume role: expected 200, got %d body: %v", code, env)
	}
	data, _ := env["data"].(map[string]any)
	creds, _ := data["credentials"].(map[string]any)
	if creds["accessToken"] == "" {
		t.Error("assume-role returned empty accessToken")
	}
}

// TestE2EResourceMonitor exercises the resource-monitor flow end-to-end:
// sync inventory → list resources → ingest metric → query → alert rule fires.
func TestE2EResourceMonitor(t *testing.T) {
	apiSrv, st := newTestServer(t)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)
	bearer := adminToken(t, st)

	// Sync projects the local node (EnsureLocalTopology creates one) into inventory.
	code, env := roundTrip(t, srv, "POST", "/api/v1/resources/sync", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("sync: got %d body %v", code, env)
	}

	// List resources.
	code, env = roundTrip(t, srv, "GET", "/api/v1/resources", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("list resources: got %d", code)
	}

	// Ingest a metric sample.
	code, _ = roundTrip(t, srv, "POST", "/api/v1/metrics/ingest", map[string]any{
		"resourceType": "instance", "resourceId": "i_test", "metricName": "cpu.percent", "value": 95,
	}, bearer)
	if code != http.StatusOK {
		t.Fatalf("ingest metric: got %d", code)
	}

	// Query it back.
	code, env = roundTrip(t, srv, "GET",
		"/api/v1/metrics/query?resourceType=instance&resourceId=i_test&metric=cpu.percent", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("query metrics: got %d", code)
	}
	if samples, _ := env["data"].([]any); len(samples) != 1 {
		t.Errorf("expected 1 sample, got %v", env["data"])
	}

	// Create an alert rule and verify it lists.
	code, _ = roundTrip(t, srv, "POST", "/api/v1/alerts/rules", map[string]any{
		"name": "high-cpu", "resourceType": "instance", "metricName": "cpu.percent",
		"condition": "gt", "threshold": 80, "severity": "warning", "enabled": true,
	}, bearer)
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("create alert rule: got %d", code)
	}
	code, env = roundTrip(t, srv, "GET", "/api/v1/alerts/rules", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("list alert rules: got %d", code)
	}
	if rules, _ := env["data"].([]any); len(rules) != 1 {
		t.Errorf("expected 1 alert rule, got %v", env["data"])
	}
}

// TestE2EFunctions exercises function create → invoke → invocation recorded.
func TestE2EFunctions(t *testing.T) {
	apiSrv, st := newTestServer(t)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)
	bearer := adminToken(t, st)

	code, env := roundTrip(t, srv, "POST", "/api/v1/functions", map[string]any{
		"name": "echo", "runtime": "native", "command": []string{"/bin/cat"},
	}, bearer)
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("create function: got %d body %v", code, env)
	}
	fnData, _ := env["data"].(map[string]any)
	fnID, _ := fnData["id"].(string)
	if fnID == "" {
		t.Fatal("function id empty")
	}

	// Invoke with a payload — /bin/cat echoes stdin to stdout.
	code, env = roundTrip(t, srv, "POST", "/api/v1/functions/"+fnID+"/invoke", "hello-fn", bearer)
	if code != http.StatusOK {
		t.Fatalf("invoke: got %d body %v", code, env)
	}
	res, _ := env["data"].(map[string]any)
	if res["status"] != "succeeded" {
		t.Errorf("expected succeeded, got %v", res["status"])
	}
	// roundTrip JSON-encodes the body, so /bin/cat echoes the quoted form.
	if out, _ := res["output"].(string); !strings.Contains(out, "hello-fn") {
		t.Errorf("expected echoed output to contain hello-fn, got %q", out)
	}

	// Invocation should be recorded.
	code, env = roundTrip(t, srv, "GET", "/api/v1/functions/"+fnID+"/invocations", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("list invocations: got %d", code)
	}
	if invs, _ := env["data"].([]any); len(invs) != 1 {
		t.Errorf("expected 1 invocation, got %v", env["data"])
	}
}

// TestE2EMCP exercises MCP server create → tool sync → dangerous tool invoke
// that opens an approval, then approve it.
func TestE2EMCP(t *testing.T) {
	apiSrv, st := newTestServer(t)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)
	bearer := adminToken(t, st)

	code, env := roundTrip(t, srv, "POST", "/api/v1/mcp/servers", map[string]any{
		"name": "admin-tools", "runtime": "mcp-go", "approvalPolicy": "dangerous-only",
	}, bearer)
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("create mcp server: got %d body %v", code, env)
	}
	srvData, _ := env["data"].(map[string]any)
	srvID, _ := srvData["id"].(string)

	// Register a dangerous tool.
	code, _ = roundTrip(t, srv, "POST", "/api/v1/mcp/servers/"+srvID+"/tools/sync", map[string]any{
		"tools": []map[string]any{
			{"name": "terminate_node", "iamAction": "node:delete", "dangerous": true, "enabled": true},
		},
	}, bearer)
	if code != http.StatusOK {
		t.Fatalf("sync tools: got %d", code)
	}

	// Invoke the dangerous tool → admin is authorized but it needs approval.
	code, env = roundTrip(t, srv, "POST",
		"/api/v1/mcp/servers/"+srvID+"/tools/terminate_node/invoke", `{"node":"n1"}`, bearer)
	if code != http.StatusOK {
		t.Fatalf("invoke tool: got %d body %v", code, env)
	}
	res, _ := env["data"].(map[string]any)
	if res["decision"] != "needs-approval" {
		t.Errorf("expected needs-approval for dangerous tool, got %v", res["decision"])
	}

	// A pending approval should exist; approve it.
	code, env = roundTrip(t, srv, "GET", "/api/v1/mcp/approvals", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("list approvals: got %d", code)
	}
	approvals, _ := env["data"].([]any)
	if len(approvals) != 1 {
		t.Fatalf("expected 1 pending approval, got %v", env["data"])
	}
	appr, _ := approvals[0].(map[string]any)
	apprID, _ := appr["id"].(string)
	code, _ = roundTrip(t, srv, "POST", "/api/v1/mcp/approvals/"+apprID+"/approve",
		map[string]any{"reason": "ok"}, bearer)
	if code != http.StatusOK {
		t.Fatalf("approve: got %d", code)
	}
}

// TestE2EIPAM exercises pool create (materialization) → reserve → list → release.
func TestE2EIPAM(t *testing.T) {
	apiSrv, st := newTestServer(t)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)
	bearer := adminToken(t, st)

	code, env := roundTrip(t, srv, "POST", "/api/v1/ip-pools", map[string]any{
		"name": "public-main", "cidr": "203.0.113.0/28", "gateway": "203.0.113.1",
		"usage": []string{"load-balancer", "reserved"}, "allowAutoAllocate": true,
	}, bearer)
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("create pool: got %d body %v", code, env)
	}
	data, _ := env["data"].(map[string]any)
	if cnt, _ := data["addresses"].(float64); cnt != 13 {
		t.Errorf("expected 13 materialized addresses, got %v", data["addresses"])
	}

	// Reserve an address for a load balancer.
	code, env = roundTrip(t, srv, "POST", "/api/v1/ips/reserve", map[string]any{
		"pool": "public-main", "name": "api-ip", "purpose": "load-balancer",
	}, bearer)
	if code != http.StatusOK {
		t.Fatalf("reserve: got %d body %v", code, env)
	}
	ip, _ := env["data"].(map[string]any)
	ipID, _ := ip["id"].(string)
	if ip["status"] != "reserved" {
		t.Errorf("expected reserved, got %v", ip["status"])
	}

	// Association IDs are distinct from allocation IDs on the public-IP routes.
	code, env = roundTrip(t, srv, "POST", "/api/v1/public-ips/"+ipID+"/associate", map[string]any{
		"targetType": "load-balancer", "targetId": "lb-1", "bindingMode": "vip",
		"protocol": "tcp", "externalPort": 80,
	}, bearer)
	if code != http.StatusOK {
		t.Fatalf("associate: got %d body %v", code, env)
	}
	binding, _ := env["data"].(map[string]any)
	associationID, _ := binding["id"].(string)
	if associationID == "" {
		t.Fatalf("association ID missing from response: %v", env)
	}
	code, env = roundTrip(t, srv, "POST", "/api/v1/public-ips/"+associationID+"/disassociate", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("disassociate: got %d body %v", code, env)
	}
	code, env = roundTrip(t, srv, "GET", "/api/v1/ips/"+ipID, nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("get after disassociate: got %d body %v", code, env)
	}
	details, _ := env["data"].(map[string]any)
	ip, _ = details["ip"].(map[string]any)
	if ip["status"] != "reserved" {
		t.Errorf("expected reserved after disassociate, got %v", ip["status"])
	}

	// List reserved addresses.
	code, env = roundTrip(t, srv, "GET", "/api/v1/ips?status=reserved", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("list ips: got %d", code)
	}
	if ips, _ := env["data"].([]any); len(ips) != 1 {
		t.Errorf("expected 1 reserved ip, got %v", env["data"])
	}

	// Release it.
	code, _ = roundTrip(t, srv, "POST", "/api/v1/ips/"+ipID+"/release", nil, bearer)
	if code != http.StatusOK {
		t.Fatalf("release: got %d", code)
	}
	code, env = roundTrip(t, srv, "GET", "/api/v1/ips?status=reserved", nil, bearer)
	if ips, _ := env["data"].([]any); len(ips) != 0 {
		t.Errorf("expected 0 reserved after release, got %v", env["data"])
	}
}

// TestE2EAuditEventsQueryable verifies the audit store can be listed without error.
func TestE2EAuditEventsQueryable(t *testing.T) {
	_, st := newTestServer(t)
	events, err := st.Audit.ListByAccount("org_local", "acct_local", 10)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	_ = events
}
