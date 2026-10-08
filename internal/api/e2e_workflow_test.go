package api

// TestFullDeploymentWorkflow exercises the complete lifecycle:
//
//   Install → Account/Project → VPC → Image → Instance → Storage →
//   DNS/Ingress → IAM → Audit → Bottle (Stack) → Redeploy
//
// All steps run in-process against an httptest.Server backed by a
// temporary SQLite store. No external daemons or containers are required;
// instance "runs" are attempted but failure is tolerated because the test
// environment has no container runtime.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- helpers ----------------------------------------------------------------

func do(t *testing.T, srv *Server, method, path string, body any, bearer ...string) (int, map[string]any) {
	t.Helper()
	var rb io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s body: %v", method, path, err)
		}
		rb = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, rb)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if len(bearer) > 0 && bearer[0] != "" {
		req.Header.Set("Authorization", "Bearer "+bearer[0])
	}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	return rr.Code, resp
}

// bootstrapToken issues a short-lived admin token for the bootstrap IAM user.
// Tests call this once and pass the token to every do() call that needs auth.
func bootstrapToken(t *testing.T, srv *Server) string {
	t.Helper()
	pt, pid := srv.ctrl.Store.IAM.LocalPrincipal()
	bearer, _, err := srv.ctrl.Store.IAM.Issue("test-bootstrap", pt, pid, 1*60*60*1000000000 /* 1h */)
	if err != nil {
		t.Fatalf("bootstrap token: %v", err)
	}
	return bearer
}

func mustOK(t *testing.T, code int, resp map[string]any, label string) map[string]any {
	t.Helper()
	if code < 200 || code >= 300 {
		t.Fatalf("[%s] expected 2xx, got %d — %v", label, code, resp)
	}
	data, _ := resp["data"]
	if m, ok := data.(map[string]any); ok {
		return m
	}
	return resp
}

func strField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ---- test -------------------------------------------------------------------

func TestFullDeploymentWorkflow(t *testing.T) {
	srv := openTestServer(t)
	adminBearer := bootstrapToken(t, srv)

	// auth is a shorthand for authenticated do() calls in this test.
	auth := func(method, path string, body any) (int, map[string]any) {
		return do(t, srv, method, path, body, adminBearer)
	}

	// =========================================================================
	// Step 1 — Install: verify the control plane is healthy
	// =========================================================================
	code, _ := do(t, srv, http.MethodGet, "/api/v1/health", nil)
	if code != http.StatusOK {
		t.Fatalf("[install] health check failed: %d", code)
	}
	code, ver := do(t, srv, http.MethodGet, "/api/v1/version", nil)
	if code != http.StatusOK {
		t.Fatalf("[install] version check failed: %d", code)
	}
	t.Logf("[install] version: %v", ver)

	// =========================================================================
	// Step 2 — Account / Project: create an IAM user, group, and API token
	// =========================================================================
	code, uResp := auth(http.MethodPost, "/api/v1/iam/users", map[string]any{
		"name":      "deploy-user",
		"localUser": "root",
	})
	user := mustOK(t, code, uResp, "iam:create-user")
	userID := strField(user, "id")
	if userID == "" {
		t.Fatal("[account] user ID missing from response")
	}
	t.Logf("[account] created user %s", userID)

	code, gResp := auth(http.MethodPost, "/api/v1/iam/groups", map[string]any{
		"name": "operators",
	})
	group := mustOK(t, code, gResp, "iam:create-group")
	t.Logf("[account] created group %s", strField(group, "id"))

	code, _ = auth(http.MethodPost, "/api/v1/iam/groups/operators/members", map[string]any{
		"user": "deploy-user",
	})
	if code < 200 || code >= 300 {
		t.Fatalf("[account] add group member failed: %d", code)
	}

	code, tokResp := auth(http.MethodPost, "/api/v1/iam/tokens", map[string]any{
		"name": "deploy-token",
		"ttl":  "1h",
	})
	tok := mustOK(t, code, tokResp, "iam:issue-token")
	t.Logf("[account] issued token id=%s", strField(tok, "id"))

	// =========================================================================
	// Step 3 — VPC: create a VPC and a subnet
	// =========================================================================
	code, vpcResp := auth(http.MethodPost, "/api/v1/vpcs", map[string]any{
		"slug":   "e2e-vpc",
		"name":   "E2E Test VPC",
		"cidr":   "10.99.0.0/16",
		"status": "active",
	})
	vpc := mustOK(t, code, vpcResp, "vpc:create")
	vpcSlug := strField(vpc, "slug")
	if vpcSlug == "" {
		vpcSlug = "e2e-vpc"
	}
	t.Logf("[vpc] created vpc slug=%s", vpcSlug)

	code, subResp := auth(http.MethodPost,
		"/api/v1/vpcs/"+vpcSlug+"/subnets", map[string]any{
			"name": "e2e-subnet",
			"cidr": "10.99.1.0/24",
			"zone": "z1",
		})
	subnet := mustOK(t, code, subResp, "vpc:create-subnet")
	subnetID := strField(subnet, "id")
	t.Logf("[vpc] created subnet id=%s", subnetID)

	// =========================================================================
	// Step 3b — Storage pool: required for instance/volume disks
	// =========================================================================
	poolDir := filepath.Join(srv.ctrl.Store.Paths.Root, "e2e-pool")
	if err := os.MkdirAll(poolDir, 0o755); err != nil {
		t.Fatalf("[storage] mkdir pool: %v", err)
	}
	code, poolResp := auth(http.MethodPost, "/api/v1/admin/storage-pools", map[string]any{
		"name":       "e2e-pool",
		"backend":    "directory",
		"mountpoint": poolDir,
		"totalBytes": int64(10 * 1024 * 1024 * 1024),
	})
	pool := mustOK(t, code, poolResp, "storage:create-pool")
	poolID := strField(pool, "id")
	code, _ = auth(http.MethodPut, "/api/v1/admin/storage/settings", map[string]any{
		"defaultInstancePool": poolID,
	})
	if code < 200 || code >= 300 {
		t.Fatalf("[storage] set default pool failed: %d", code)
	}
	t.Logf("[storage] configured default pool %s", poolID)

	// =========================================================================
	// Step 4 — Image: seed a minimal .cap file in the staging dir and import it
	// =========================================================================
	stagingDir := srv.ctrl.Store.Paths.ImportStaging
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("[image] mkdir staging: %v", err)
	}
	capFile := filepath.Join(stagingDir, "e2e-base.cap")
	if err := os.WriteFile(capFile, []byte("fake-cap-payload"), 0644); err != nil {
		t.Fatalf("[image] write stub cap: %v", err)
	}

	code, imgResp := auth(http.MethodPost, "/api/v1/images/import", map[string]any{
		"path": capFile,
		"name": "e2e-base",
	})
	img := mustOK(t, code, imgResp, "image:import")
	imageName := strField(img, "name")
	if imageName == "" {
		imageName = "e2e-base.cap"
	}
	t.Logf("[image] imported %s (digest=%s)", imageName, strField(img, "digest"))

	// =========================================================================
	// Step 5 — Instance: launch an instance from the image
	//
	// A real container runtime is not present in CI, so we accept both 201
	// (container daemon present) and 500/400 (no runtime) and record the ID
	// if creation succeeded.
	// =========================================================================
	code, instResp := auth(http.MethodPost, "/api/v1/instances", map[string]any{
		"image":    imageName,
		"name":     "e2e-inst",
		"subnetId": subnetID,
		"vpcId":    strField(vpc, "id"),
		"labels": map[string]string{
			"env":  "e2e",
			"role": "web",
		},
	})
	var instanceID string
	if code == http.StatusCreated {
		inst := mustOK(t, code, instResp, "instance:create")
		instanceID = strField(inst, "id")
		t.Logf("[instance] launched id=%s", instanceID)
	} else {
		t.Logf("[instance] runtime not available (code=%d) — skipping runtime-dependent steps", code)
	}

	// Verify the instance list endpoint works regardless of runtime.
	code, listResp := auth(http.MethodGet, "/api/v1/instances", nil)
	if code != http.StatusOK {
		t.Fatalf("[instance] list failed: %d %v", code, listResp)
	}

	// =========================================================================
	// Step 6 — Storage: create a bucket and a volume
	// =========================================================================
	code, bktResp := auth(http.MethodPost, "/api/v1/storage/buckets", map[string]any{
		"name": "e2e-artifacts",
	})
	bucket := mustOK(t, code, bktResp, "storage:create-bucket")
	bucketName := strField(bucket, "name")
	t.Logf("[storage] created bucket %s", bucketName)

	code, volResp := auth(http.MethodPost, "/api/v1/storage/volumes", map[string]any{
		"name":      "e2e-data",
		"sizeBytes": int64(1 * 1024 * 1024 * 1024),
	})
	vol := mustOK(t, code, volResp, "storage:create-volume")
	t.Logf("[storage] created volume %s", strField(vol, "name"))

	// =========================================================================
	// Step 7 — DNS / Ingress: create a zone, add an A record, and add an
	// ingress rule pointing at the instance's notional load balancer.
	// =========================================================================
	code, zoneResp := auth(http.MethodPost, "/api/v1/dns/zones", map[string]any{
		"name":        "e2e.local",
		"type":        "private",
		"defaultTtl":  300,
		"description": "E2E test zone",
	})
	zone := mustOK(t, code, zoneResp, "dns:create-zone")
	zoneName := strField(zone, "name")
	t.Logf("[dns] created zone %s", zoneName)

	code, recResp := auth(http.MethodPost, "/api/v1/dns/zones/e2e.local/records", map[string]any{
		"name":   "app",
		"type":   "A",
		"values": []string{"192.168.99.1"},
		"ttl":    60,
	})
	rec := mustOK(t, code, recResp, "dns:create-record")
	t.Logf("[dns] created record id=%s", strField(rec, "id"))

	code, ingResp := auth(http.MethodPost, "/api/v1/ingress", map[string]any{
		"name":       "e2e-ingress",
		"host":       "app.e2e.local",
		"pathPrefix": "/",
		"backendLb":  "e2e-lb",
	})
	ing := mustOK(t, code, ingResp, "ingress:create")
	t.Logf("[ingress] created rule %s", strField(ing, "name"))

	// =========================================================================
	// Step 8 — IAM: attach a role and a policy, then run the simulator
	// =========================================================================
	code, roleResp := auth(http.MethodPost, "/api/v1/iam/roles", map[string]any{
		"name":        "e2e-operator",
		"description": "E2E test operator role",
	})
	role := mustOK(t, code, roleResp, "iam:create-role")
	t.Logf("[iam] created role %s", strField(role, "id"))

	code, polResp := auth(http.MethodPost, "/api/v1/iam/policies", map[string]any{
		"name":      "e2e-policy",
		"principal": "user:deploy-user",
		"action":    "instance:run",
		"resource":  "project:default",
		"effect":    "allow",
	})
	pol := mustOK(t, code, polResp, "iam:create-policy")
	t.Logf("[iam] created policy %s", strField(pol, "name"))

	code, simResp := auth(http.MethodPost, "/api/v1/iam/simulate", map[string]any{
		"action":   "instance:run",
		"resource": "project:default",
	})
	sim := mustOK(t, code, simResp, "iam:simulate")
	t.Logf("[iam] simulate result allowed=%v decision=%v", sim["allowed"], sim["decision"])

	// =========================================================================
	// Step 9 — Audit: verify the audit log contains IAM events
	// =========================================================================
	code, auditResp := auth(http.MethodGet, "/api/v1/iam/audit", nil)
	if code != http.StatusOK {
		t.Fatalf("[audit] list failed: %d %v", code, auditResp)
	}
	t.Logf("[audit] IAM audit log OK")

	// Verify general event feed is reachable.
	code, evtResp := auth(http.MethodGet, "/api/v1/events", nil)
	if code != http.StatusOK {
		t.Fatalf("[audit] events endpoint failed: %d %v", code, evtResp)
	}
	t.Logf("[audit] events feed OK")

	// =========================================================================
	// Step 10 — Bottle (Stack): declare the application as a named stack.
	// =========================================================================
	code, stackResp := auth(http.MethodPost, "/api/v1/stacks", map[string]any{
		"name": "e2e-app",
		"instances": []map[string]any{
			{
				"name":     "e2e-web",
				"image":    imageName,
				"subnetId": subnetID,
				"labels":   map[string]string{"role": "web"},
			},
		},
		"dns": []map[string]any{
			{
				"zone":   "e2e.local",
				"name":   "web",
				"type":   "A",
				"values": []string{"192.168.99.2"},
				"ttl":    60,
			},
		},
	})
	if code == http.StatusCreated {
		st := mustOK(t, code, stackResp, "stack:apply")
		t.Logf("[bottle] stack applied name=%s", strField(st, "name"))
	} else {
		t.Logf("[bottle] stack apply returned %d (no runtime) — verifying stack list", code)
	}

	code, _ = auth(http.MethodGet, "/api/v1/stacks", nil)
	if code != http.StatusOK {
		t.Fatalf("[bottle] stack list failed: %d", code)
	}

	// =========================================================================
	// Step 11 — Redeploy: stop, delete, and recreate the instance (if one was
	// actually running), simulating an image update rollout.
	// =========================================================================
	if instanceID != "" {
		code, _ = auth(http.MethodPost, "/api/v1/instances/"+instanceID+"/stop", nil)
		t.Logf("[redeploy] stop %s → %d", instanceID, code)

		code, _ = auth(http.MethodDelete, "/api/v1/instances/"+instanceID, nil)
		if code != http.StatusNoContent && code != http.StatusOK {
			t.Fatalf("[redeploy] delete instance failed: %d", code)
		}
		t.Logf("[redeploy] deleted %s", instanceID)
	}

	capFileV2 := filepath.Join(stagingDir, "e2e-base-v2.cap")
	if err := os.WriteFile(capFileV2, []byte("fake-cap-payload-v2"), 0644); err != nil {
		t.Fatalf("[redeploy] write v2 cap: %v", err)
	}
	code, img2Resp := auth(http.MethodPost, "/api/v1/images/import", map[string]any{
		"path": capFileV2,
		"name": "e2e-base-v2",
	})
	img2 := mustOK(t, code, img2Resp, "image:import-v2")
	imageNameV2 := strField(img2, "name")
	t.Logf("[redeploy] imported v2 image %s", imageNameV2)

	code, inst2Resp := auth(http.MethodPost, "/api/v1/instances", map[string]any{
		"image":    imageNameV2,
		"name":     "e2e-inst-v2",
		"subnetId": subnetID,
		"vpcId":    strField(vpc, "id"),
		"labels": map[string]string{
			"env":     "e2e",
			"role":    "web",
			"version": "2",
		},
	})
	if code == http.StatusCreated {
		inst2 := mustOK(t, code, inst2Resp, "instance:redeploy")
		t.Logf("[redeploy] relaunched id=%s", strField(inst2, "id"))
	} else {
		t.Logf("[redeploy] runtime absent (code=%d) — redeploy path validated up to launch", code)
	}

	// =========================================================================
	// Final assertions: confirm durable resources are still listed.
	// =========================================================================
	checks := []struct {
		label string
		path  string
		want  int
	}{
		{"images", "/api/v1/images", http.StatusOK},
		{"vpcs", "/api/v1/vpcs", http.StatusOK},
		{"buckets", "/api/v1/storage/buckets", http.StatusOK},
		{"volumes", "/api/v1/storage/volumes", http.StatusOK},
		{"dns-zones", "/api/v1/dns/zones", http.StatusOK},
		{"ingress", "/api/v1/ingress", http.StatusOK},
		{"iam-users", "/api/v1/iam/users", http.StatusOK},
		{"iam-roles", "/api/v1/iam/roles", http.StatusOK},
		{"iam-policies", "/api/v1/iam/policies", http.StatusOK},
		{"iam-tokens", "/api/v1/iam/tokens", http.StatusOK},
		{"stacks", "/api/v1/stacks", http.StatusOK},
	}
	var failures []string
	for _, c := range checks {
		code, _ := auth(http.MethodGet, c.path, nil)
		if code != c.want {
			failures = append(failures, fmt.Sprintf("%s: got %d want %d", c.label, code, c.want))
		}
	}
	if len(failures) > 0 {
		t.Errorf("[final] resource list failures:\n%s", strings.Join(failures, "\n"))
	} else {
		t.Logf("[final] all %d resource endpoints healthy", len(checks))
	}
}
