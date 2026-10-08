package cappersdk_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	cappersdk "capper/sdk/go"
	"capper/internal/adminconfig"
	"capper/internal/api"
	"capper/internal/controller"
	"capper/internal/hoststorage"
	"capper/internal/manager"
	"capper/internal/network"
	"capper/internal/store"
	"capper/internal/types"
	"capper/internal/vpc"
)

var ctx = context.Background()

// testEnv bundles the test server, client, and store for helpers that need
// to seed data directly without going through HTTP.
type testEnv struct {
	Client *cappersdk.Client
	Store  *store.Store
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	// Use /dev/shm (tmpfs) to avoid slow ext4 fsync during SQLite schema init.
	tmpDir, err := os.MkdirTemp("/dev/shm", "capper-sdk-test-*")
	if err != nil {
		// Fall back to os.TempDir() if /dev/shm is unavailable.
		tmpDir = t.TempDir()
	} else {
		t.Cleanup(func() { os.RemoveAll(tmpDir) })
	}
	st, err := store.Open(store.NewPaths(tmpDir))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	ctrl := controller.New(st, false, "auto")
	srv := api.NewServer(ctrl, api.Options{})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	pType, pID := st.IAM.LocalPrincipal()
	bearer, _, err := st.IAM.Issue("sdk-test", pType, pID, time.Hour)
	if err != nil {
		t.Fatalf("IAM.Issue: %v", err)
	}

	return &testEnv{
		Client: cappersdk.New(ts.URL, bearer),
		Store:  st,
	}
}

// newTestServer is the legacy helper used by tests that only need the client.
func newTestServer(t *testing.T) *cappersdk.Client {
	return newTestEnv(t).Client
}

// seedImage inserts a minimal image record directly into the store so that
// instance creation can reference it without a full file upload.
func seedImage(t *testing.T, st *store.Store, name string) string {
	t.Helper()
	img := types.ImageRecord{
		ID:        "img_" + name,
		Name:      name,
		Version:   "latest",
		Path:      "/dev/null",
		Digest:    "sha256:test",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := st.UpsertImage(img); err != nil {
		t.Fatalf("seedImage %s: %v", name, err)
	}
	return name + ":latest"
}

// buildTestCapsule creates a minimal but real runnable .cap image in the store's
// images directory so that Loader.Load can resolve and use it. Returns the image
// reference to pass to Instances.Create (e.g. "alpine.cap").
// Skips the test if bwrap or static busybox is not available.
func buildTestCapsule(t *testing.T, st *store.Store, name string) string {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not available — skipping instance test")
	}
	const busyboxPath = "/bin/busybox"
	if _, err := os.Stat(busyboxPath); err != nil {
		t.Skip("static /bin/busybox not available — skipping instance test")
	}
	root, err := os.MkdirTemp("/dev/shm", "capper-cap-*")
	if err != nil {
		root = t.TempDir()
	} else {
		t.Cleanup(func() { os.RemoveAll(root) })
	}
	binDir := filepath.Join(root, "rootfs", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("buildTestCapsule mkdir: %v", err)
	}
	busybox, err := os.ReadFile(busyboxPath)
	if err != nil {
		t.Fatalf("buildTestCapsule read busybox: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "sh"), busybox, 0o755); err != nil {
		t.Fatalf("buildTestCapsule write sh: %v", err)
	}
	configPath := filepath.Join(root, "capper.json")
	content := fmt.Sprintf(
		`{"name":%q,"version":"0.1.0","rootfs":"./rootfs","entrypoint":["/bin/sh"],"args":["-c","exit 0"]}`,
		name,
	)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("buildTestCapsule write config: %v", err)
	}
	capFile := name + ".cap"
	mgr := manager.ImageManager{Store: st}
	if _, err := mgr.Create(capFile, configPath); err != nil {
		t.Fatalf("buildTestCapsule Create: %v", err)
	}
	return name + ".cap"
}

// bootstrapLaunchPrereqs creates a VPC subnet and default storage pool required
// for instance and volume APIs.
func bootstrapLaunchPrereqs(t *testing.T, env *testEnv) string {
	t.Helper()
	v, err := env.Store.VPC.CreateVPCExtended(vpc.CreateVPCOptions{
		Project: "default",
		Name:    "sdk-vpc",
		Slug:    "sdk-vpc",
		CIDR:    "10.77.0.0/16",
	})
	if err != nil {
		t.Fatalf("bootstrap vpc: %v", err)
	}
	sub, err := env.Store.VPC.CreateSubnetExtended(vpc.CreateSubnetOptions{
		VPCID: v.ID,
		Name:  "sdk-subnet",
		Slug:  "sdk-subnet",
		CIDR:  "10.77.1.0/24",
		Kind:  vpc.SubnetPrivate,
	})
	if err != nil {
		t.Fatalf("bootstrap subnet: %v", err)
	}
	gw, err := network.GatewayForSubnet(sub.CIDR)
	if err != nil {
		t.Fatalf("bootstrap gateway: %v", err)
	}
	if err := env.Store.VPC.UpdateSubnetBridge(sub.ID, network.BridgeName(sub.Name), gw); err != nil {
		t.Fatalf("bootstrap subnet bridge: %v", err)
	}
	poolDir := filepath.Join(env.Store.Paths.Root, "sdk-pool")
	if err := os.MkdirAll(poolDir, 0o755); err != nil {
		t.Fatalf("bootstrap pool dir: %v", err)
	}
	pool, err := hoststorage.NewManager(env.Store.HostStorage).CreatePool(hoststorage.CreatePoolOptions{
		Name: "sdk-pool", Backend: hoststorage.BackendDirectory, Mountpoint: poolDir, TotalBytes: 10 * 1024 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("bootstrap pool: %v", err)
	}
	if err := env.Store.AdminConfig.Set(adminconfig.KeyDefaultInstancePool, pool.ID); err != nil {
		t.Fatalf("bootstrap pool default: %v", err)
	}
	return sub.ID
}

// containsID returns true if any element in list has the given ID field.
func containsID[T interface{ GetID() string }](list []T, id string) bool {
	for _, item := range list {
		if item.GetID() == id {
			return true
		}
	}
	return false
}

// ---- Instance lifecycle -----------------------------------------------------

func TestInstanceLifecycle(t *testing.T) {
	env := newTestEnv(t)
	c := env.Client

	imgRef := buildTestCapsule(t, env.Store, "alpine")
	subnetID := bootstrapLaunchPrereqs(t, env)

	// Create
	inst, err := c.Instances.Create(ctx, cappersdk.CreateInstanceRequest{
		Name:     "test-vm",
		Image:    imgRef,
		SubnetID: subnetID,
	})
	if err != nil {
		t.Fatalf("Instances.Create: %v", err)
	}
	if inst.Name != "test-vm" {
		t.Errorf("Name: got %q, want %q", inst.Name, "test-vm")
	}
	if inst.ID == "" {
		t.Error("ID must not be empty")
	}

	// Get
	got, err := c.Instances.Get(ctx, inst.ID)
	if err != nil {
		t.Fatalf("Instances.Get: %v", err)
	}
	if got.ID != inst.ID {
		t.Errorf("Get ID: got %q, want %q", got.ID, inst.ID)
	}

	// List — must contain the created instance
	list, err := c.Instances.List(ctx, "default")
	if err != nil {
		t.Fatalf("Instances.List: %v", err)
	}
	found := false
	for _, item := range list {
		if item.ID == inst.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("instance %s not found in list", inst.ID)
	}

	// Stop (in case it is still running), then Delete.
	_ = c.Instances.Stop(ctx, inst.ID)
	if err := c.Instances.Delete(ctx, inst.ID); err != nil {
		t.Fatalf("Instances.Delete: %v", err)
	}

	// Get after delete — must return not-found error
	_, err = c.Instances.Get(ctx, inst.ID)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
	if !errors.Is(err, cappersdk.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

// ---- VPC prerequisites (legacy flat networks removed) ------------------------

func TestVPCSubnetBootstrap(t *testing.T) {
	env := newTestEnv(t)
	subnetID := bootstrapLaunchPrereqs(t, env)
	if subnetID == "" {
		t.Fatal("expected subnet id")
	}
	vpcs, err := env.Store.VPC.ListVPCs("default")
	if err != nil || len(vpcs) == 0 {
		t.Fatalf("ListVPCs: %v", err)
	}
}

// ---- Network lifecycle (removed) ----------------------------------------------

func TestNetworkLifecycle(t *testing.T) {
	t.Skip("legacy /api/v1/networks API removed — use VPC subnets")
	c := newTestServer(t)

	// Create
	net, err := c.Networks.Create(ctx, "default", cappersdk.CreateNetworkRequest{
		Name:   "test-net",
		Subnet: "10.99.0.0/24",
	})
	if err != nil {
		t.Fatalf("Networks.Create: %v", err)
	}
	if net.Name != "test-net" {
		t.Errorf("Name: got %q, want %q", net.Name, "test-net")
	}

	// List — must include created network
	list, err := c.Networks.List(ctx, "default")
	if err != nil {
		t.Fatalf("Networks.List: %v", err)
	}
	found := false
	for _, n := range list {
		if n.Name == "test-net" {
			found = true
		}
	}
	if !found {
		t.Errorf("network test-net not found in list")
	}

	// Delete
	if err := c.Networks.Delete(ctx, "test-net"); err != nil {
		t.Fatalf("Networks.Delete: %v", err)
	}

	// List after delete — must not include deleted network
	list2, _ := c.Networks.List(ctx, "default")
	for _, n := range list2 {
		if n.Name == "test-net" {
			t.Error("deleted network still appears in list")
		}
	}
}

// ---- Image lifecycle ---------------------------------------------------------

func TestImageLifecycle(t *testing.T) {
	env := newTestEnv(t)
	c := env.Client

	seedImage(t, env.Store, "ubuntu")

	list, err := c.Images.List(ctx)
	if err != nil {
		t.Fatalf("Images.List: %v", err)
	}
	found := false
	for _, img := range list {
		if img.Name == "ubuntu" {
			found = true
		}
	}
	if !found {
		t.Error("seeded image ubuntu not found via Images.List")
	}
}

// ---- DNS lifecycle -----------------------------------------------------------

func TestDNSLifecycle(t *testing.T) {
	c := newTestServer(t)

	// Create zone
	zone, err := c.DNS.CreateZone(ctx, "example.com")
	if err != nil {
		t.Fatalf("DNS.CreateZone: %v", err)
	}
	if zone.Name != "example.com" {
		t.Errorf("zone name: got %q, want %q", zone.Name, "example.com")
	}

	// List zones
	zones, err := c.DNS.ListZones(ctx)
	if err != nil {
		t.Fatalf("DNS.ListZones: %v", err)
	}
	found := false
	for _, z := range zones {
		if z.Name == "example.com" {
			found = true
		}
	}
	if !found {
		t.Error("created zone not found in list")
	}

	// Create record
	rec, err := c.DNS.CreateRecord(ctx, zone.ID, "www", "A", "1.2.3.4", 300)
	if err != nil {
		t.Fatalf("DNS.CreateRecord: %v", err)
	}
	if len(rec.Values) == 0 || rec.Values[0] != "1.2.3.4" {
		t.Errorf("record value: got %v, want [1.2.3.4]", rec.Values)
	}

	// Delete record
	if err := c.DNS.DeleteRecord(ctx, zone.ID, rec.ID); err != nil {
		t.Fatalf("DNS.DeleteRecord: %v", err)
	}

	// Delete zone
	if err := c.DNS.DeleteZone(ctx, zone.ID); err != nil {
		t.Fatalf("DNS.DeleteZone: %v", err)
	}
}

// ---- Load Balancer lifecycle ------------------------------------------------

func TestLBLifecycle(t *testing.T) {
	env := newTestEnv(t)
	c := env.Client
	subnetID := bootstrapLaunchPrereqs(t, env)

	// Create
	lb, err := c.LB.Create(ctx, cappersdk.CreateLBRequest{
		Name:     "api-lb",
		SubnetID: subnetID,
		Mode:     "http",
		Project:  "default",
	})
	if err != nil {
		t.Fatalf("LB.Create: %v", err)
	}
	if lb.Name != "api-lb" {
		t.Errorf("LB name: got %q, want %q", lb.Name, "api-lb")
	}

	// List
	list, err := c.LB.List(ctx, "default")
	if err != nil {
		t.Fatalf("LB.List: %v", err)
	}
	found := false
	for _, item := range list {
		if item.Name == "api-lb" {
			found = true
		}
	}
	if !found {
		t.Error("created LB not found in list")
	}

	// Delete
	if err := c.LB.Delete(ctx, "api-lb"); err != nil {
		t.Fatalf("LB.Delete: %v", err)
	}
}

// ---- KMS lifecycle ----------------------------------------------------------

func TestKMSLifecycle(t *testing.T) {
	c := newTestServer(t)
	project := "default"

	// Create key
	key, err := c.KMS.Create(ctx, "test-key", project, "AES-256-GCM")
	if err != nil {
		t.Fatalf("KMS.Create: %v", err)
	}
	if key.Name != "test-key" {
		t.Errorf("key name: got %q, want %q", key.Name, "test-key")
	}

	// List keys
	keys, err := c.KMS.List(ctx, project)
	if err != nil {
		t.Fatalf("KMS.List: %v", err)
	}
	found := false
	for _, k := range keys {
		if k.Name == "test-key" {
			found = true
		}
	}
	if !found {
		t.Error("created key not found in list")
	}

	// Encrypt and decrypt
	ciphertext, err := c.KMS.Encrypt(ctx, "test-key", "hello world")
	if err != nil {
		t.Fatalf("KMS.Encrypt: %v", err)
	}
	if ciphertext == "" || ciphertext == "hello world" {
		t.Errorf("encrypt: got bad ciphertext %q", ciphertext)
	}

	plaintext, err := c.KMS.Decrypt(ctx, "test-key", ciphertext)
	if err != nil {
		t.Fatalf("KMS.Decrypt: %v", err)
	}
	if plaintext != "hello world" {
		t.Errorf("decrypt: got %q, want %q", plaintext, "hello world")
	}

	// Delete
	if err := c.KMS.Delete(ctx, "test-key", project); err != nil {
		t.Fatalf("KMS.Delete: %v", err)
	}

	// List after delete — key must be gone
	keys2, _ := c.KMS.List(ctx, project)
	for _, k := range keys2 {
		if k.Name == "test-key" {
			t.Error("deleted key still in list")
		}
	}
}

// ---- S3 Credential lifecycle ------------------------------------------------

func TestS3CredentialLifecycle(t *testing.T) {
	c := newTestServer(t)
	accountID := "acct_test"

	// Create
	cred, err := c.S3Creds.Create(ctx, accountID)
	if err != nil {
		t.Fatalf("S3Creds.Create: %v", err)
	}
	if cred.AccessKey == "" {
		t.Error("AccessKey must not be empty")
	}
	if cred.SecretKey == "" {
		t.Error("SecretKey must be present on initial create response")
	}

	// List
	creds, err := c.S3Creds.List(ctx, accountID)
	if err != nil {
		t.Fatalf("S3Creds.List: %v", err)
	}
	found := false
	for _, cr := range creds {
		if cr.ID == cred.ID {
			found = true
		}
		if cr.SecretKey != "" {
			t.Errorf("SecretKey must be omitted in list response for credential %s", cr.ID)
		}
	}
	if !found {
		t.Error("created credential not found in list")
	}

	// Delete
	if err := c.S3Creds.Delete(ctx, cred.ID); err != nil {
		t.Fatalf("S3Creds.Delete: %v", err)
	}

	// List after delete — must be gone
	creds2, _ := c.S3Creds.List(ctx, accountID)
	for _, cr := range creds2 {
		if cr.ID == cred.ID {
			t.Error("deleted credential still in list")
		}
	}
}

// ---- Auth negative tests ----------------------------------------------------

func TestAuthNegativeNoToken(t *testing.T) {
	env := newTestEnv(t)
	// Client with no token
	unauthClient := cappersdk.New(env.Client.BaseURL(), "")

	_, err := unauthClient.Instances.List(ctx, "default")
	if err == nil {
		t.Fatal("expected error for unauthenticated request, got nil")
	}
	if !errors.Is(err, cappersdk.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestAuthNegativeBadToken(t *testing.T) {
	env := newTestEnv(t)
	badClient := cappersdk.New(env.Client.BaseURL(), "bad-token")

	_, err := badClient.Instances.List(ctx, "default")
	if err == nil {
		t.Fatal("expected error for bad token, got nil")
	}
	if !errors.Is(err, cappersdk.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestAuthNegativeGetMissingResource(t *testing.T) {
	c := newTestServer(t)

	_, err := c.Instances.Get(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for missing instance, got nil")
	}
	if !errors.Is(err, cappersdk.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ---- Context (org/account/project) header injection -------------------------

// TestContextHeaders verifies that UseOrg/UseAccount/UseProject cause the
// corresponding X-Capper-* headers to be sent on every request, and that
// unset context fields send no header.
func TestContextHeaders(t *testing.T) {
	var gotHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(ts.Close)

	// No context set: none of the X-Capper-* headers should be present.
	c := cappersdk.New(ts.URL, "token")
	if _, err := c.Instances.List(ctx, "default"); err != nil {
		t.Fatalf("List (no context): %v", err)
	}
	for _, h := range []string{"X-Capper-Org-ID", "X-Capper-Account-ID", "X-Capper-Project-ID"} {
		if v := gotHeaders.Get(h); v != "" {
			t.Errorf("expected no %s header without context, got %q", h, v)
		}
	}

	// Context set: each header must reflect the configured value, and the
	// fluent setters must be chainable.
	c.UseOrg("org_acme").UseAccount("acct_prod").UseProject("proj_web")
	if _, err := c.Instances.List(ctx, "default"); err != nil {
		t.Fatalf("List (with context): %v", err)
	}
	want := map[string]string{
		"X-Capper-Org-ID":     "org_acme",
		"X-Capper-Account-ID": "acct_prod",
		"X-Capper-Project-ID": "proj_web",
	}
	for h, exp := range want {
		if got := gotHeaders.Get(h); got != exp {
			t.Errorf("header %s = %q, want %q", h, got, exp)
		}
	}
}

// ---- Pagination -------------------------------------------------------------

func TestPagination(t *testing.T) {
	env := newTestEnv(t)
	c := env.Client

	imgRef := buildTestCapsule(t, env.Store, "busybox")
	subnetID := bootstrapLaunchPrereqs(t, env)

	// Seed 15 instances (keep low to avoid slow test from actual process launch)
	for i := range 15 {
		_, err := c.Instances.Create(ctx, cappersdk.CreateInstanceRequest{
			Name:     fmt.Sprintf("vm-%02d", i),
			Image:    imgRef,
			SubnetID: subnetID,
		})
		if err != nil {
			t.Fatalf("Create vm-%02d: %v", i, err)
		}
	}

	list, err := c.Instances.List(ctx, "default")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) < 15 {
		t.Errorf("expected at least 15 instances, got %d", len(list))
	}
}

