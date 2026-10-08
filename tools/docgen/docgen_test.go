package main

import (
	"os"
	"path/filepath"
	"testing"

	"capper/internal/cli"
)

func TestResolveWebPathServesDirectoryIndexes(t *testing.T) {
	webDir := t.TempDir()
	index := filepath.Join(webDir, "index.html")
	if err := os.WriteFile(index, []byte("root"), 0o600); err != nil {
		t.Fatal(err)
	}
	nestedDir := filepath.Join(webDir, "guide")
	if err := os.Mkdir(nestedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	nestedIndex := filepath.Join(nestedDir, "index.html")
	if err := os.WriteFile(nestedIndex, []byte("guide"), 0o600); err != nil {
		t.Fatal(err)
	}
	for requestPath, want := range map[string]string{"/": index, "/guide": nestedIndex, "/guide/": nestedIndex} {
		got, err := resolveWebPath(webDir, requestPath)
		if err != nil {
			t.Fatalf("resolveWebPath(%q): %v", requestPath, err)
		}
		if got != want {
			t.Errorf("resolveWebPath(%q) = %q, want %q", requestPath, got, want)
		}
	}
}

func TestResolveWebPathRejectsTraversalAndSymlinksOutsideRoot(t *testing.T) {
	webDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.html")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(webDir, "outside.html")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, requestPath := range []string{"/../secret.html", "/outside.html"} {
		if _, err := resolveWebPath(webDir, requestPath); err == nil {
			t.Errorf("resolveWebPath(%q) unexpectedly succeeded", requestPath)
		}
	}
	emptyDir := filepath.Join(webDir, "empty")
	if err := os.Mkdir(emptyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveWebPath(webDir, "/empty"); err == nil {
		t.Fatal("directory without index should be rejected")
	}
}

// chdirRepoRoot points the test at the module root so the relative paths the
// generators use (internal/api, docs/src) resolve.
func chdirRepoRoot(t *testing.T) {
	t.Helper()
	if err := os.Chdir(repoRoot()); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
}

// TestExtractAPIRoutes guards the API route generator: it must find a healthy
// number of routes including well-known ones, so a refactor of the extractor (or
// the mux registration style) can't silently drop coverage.
func TestExtractAPIRoutes(t *testing.T) {
	chdirRepoRoot(t)
	routes, err := extractAPIRoutes(filepath.Join("internal", "api"))
	if err != nil {
		t.Fatalf("extractAPIRoutes: %v", err)
	}
	if len(routes) < 100 {
		t.Fatalf("expected >=100 API routes, got %d (extractor likely broke)", len(routes))
	}
	want := map[string]bool{
		"GET /api/v1/instances":    false,
		"GET /api/v1/iam/users":    false,
		"GET /api/v1/vpcs":         false,
		"GET /api/v1/storage/volumes": false,
	}
	for _, r := range routes {
		key := r.method + " " + r.path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("expected route %q not found", key)
		}
	}
}

// TestCLITreeCoversTopLevel guards the CLI generator: NewRootCmd must expose the
// full set of command groups, and visibleSubcommands must surface the headline
// ones.
func TestCLITreeCoversTopLevel(t *testing.T) {
	root := cli.NewRootCmd()
	tops := visibleSubcommands(root)
	if len(tops) < 60 {
		t.Fatalf("expected >=60 top-level commands, got %d", len(tops))
	}
	have := map[string]bool{}
	for _, c := range tops {
		have[c.Name()] = true
	}
	for _, name := range []string{"iam", "compute", "storage", "network", "vpc", "lb", "dns", "secret", "kms", "node"} {
		if !have[name] {
			t.Errorf("expected top-level command %q in the tree", name)
		}
	}
}

// TestGroupOf checks the route grouping helper.
func TestGroupOf(t *testing.T) {
	cases := map[string]string{
		"/api/v1/instances":          "instances",
		"/api/v1/instances/{id}":     "instances",
		"/api/v1/iam/users":          "iam",
		"/api/v1/":                   "root",
	}
	for path, want := range cases {
		if got := groupOf(path); got != want {
			t.Errorf("groupOf(%q) = %q, want %q", path, got, want)
		}
	}
}
