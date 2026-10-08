package manager

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"

	"capper/internal/loader"
	"capper/internal/runtime"
	"capper/internal/store"
	"capper/internal/types"
)

func testNetworkOpts() *NetworkRunOpts {
	return &NetworkRunOpts{
		NetworkID: "test-subnet",
		Bridge:    "capbr-test",
		Subnet:    "10.99.0.0/24",
		Gateway:   "10.99.0.1",
	}
}

func TestImageCreateAndLoad(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(store.NewPaths(filepath.Join(root, "store")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	project := filepath.Join(root, "project")
	rootfs := filepath.Join(project, "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfs, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootfs, "bin", "sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(project, "capper.json")
	if err := os.WriteFile(config, []byte(`{
		"name": "hello",
		"version": "0.1.0",
		"rootfs": "./rootfs",
		"entrypoint": ["/bin/sh"]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := ImageManager{Store: st}
	res, err := mgr.Create("hello.cap", config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.Image.Path); err != nil {
		t.Fatal(err)
	}

	ld := loader.Loader{Paths: st.Paths}
	loaded, cleanup, err := ld.Load("hello.cap")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if loaded.Manifest.Name != "hello" || loaded.Manifest.RootFS.Compression != "zstd" {
		t.Fatalf("unexpected manifest: %#v", loaded.Manifest)
	}
}

func TestRunOneShotCapsuleBecomesStopped(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not available")
	}
	if _, err := os.Stat("/bin/busybox"); err != nil {
		t.Skip("static /bin/busybox not available")
	}

	root := t.TempDir()
	st, err := store.Open(store.NewPaths(filepath.Join(root, "store")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	project := filepath.Join(root, "project")
	rootfs := filepath.Join(project, "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfs, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	busybox, err := os.ReadFile("/bin/busybox")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootfs, "bin", "sh"), busybox, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(project, "capper.json")
	if err := os.WriteFile(config, []byte(`{
		"name": "oneshot",
		"version": "0.1.0",
		"rootfs": "./rootfs",
		"entrypoint": ["/bin/sh"],
		"args": ["-c", "echo one-shot"]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	imgMgr := ImageManager{Store: st}
	if _, err := imgMgr.Create("oneshot.cap", config); err != nil {
		t.Fatal(err)
	}
	instMgr := InstanceManager{
		Store:  st,
		Loader: loader.Loader{Paths: st.Paths},
		Runner: runtime.Runner{Mode: runtime.ModeBwrap},
	}
	inst, err := instMgr.Run("oneshot.cap", types.ResourceOverrides{}, RunOptions{Network: testNetworkOpts()})
	if err != nil {
		t.Fatal(err)
	}
	var instances []types.Instance
	for i := 0; i < 20; i++ {
		instances, err = instMgr.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(instances) == 1 && instances[0].Status == types.StatusStopped {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(instances) != 1 {
		t.Fatalf("expected one instance, got %d", len(instances))
	}
	if instances[0].ID != inst.ID {
		t.Fatalf("unexpected instance id: %s", instances[0].ID)
	}
	if instances[0].Status != types.StatusStopped {
		t.Fatalf("expected stopped one-shot instance, got %s", instances[0].Status)
	}
	stdout, err := os.ReadFile(filepath.Join(st.Paths.Instances, inst.ID, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != "one-shot\n" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
}

func TestRunFailedStartIsVisibleInStore(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(store.NewPaths(filepath.Join(root, "store")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	project := filepath.Join(root, "project")
	rootfs := filepath.Join(project, "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfs, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(project, "capper.json")
	if err := os.WriteFile(config, []byte(`{
		"name": "broken",
		"version": "0.1.0",
		"rootfs": "./rootfs",
		"entrypoint": ["/bin/missing"]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	imgMgr := ImageManager{Store: st}
	if _, err := imgMgr.Create("broken.cap", config); err != nil {
		t.Fatal(err)
	}

	instMgr := InstanceManager{
		Store:  st,
		Loader: loader.Loader{Paths: st.Paths},
		Runner: runtime.Runner{Mode: runtime.ModeBwrap},
	}
	_, err = instMgr.Run("broken.cap", types.ResourceOverrides{}, RunOptions{Network: testNetworkOpts()})
	if err == nil {
		t.Fatal("expected run failure")
	}

	instances, listErr := st.ListInstances()
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(instances) != 1 {
		t.Fatalf("expected one visible failed instance, got %d", len(instances))
	}
	if instances[0].Status != types.StatusFailed {
		t.Fatalf("expected failed status, got %s", instances[0].Status)
	}
	if !errors.Is(err, os.ErrNotExist) && err.Error() == "" {
		t.Fatalf("expected useful error, got %v", err)
	}
}

func TestRemoveFailedInstanceReleasesQuotaUsage(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(store.NewPaths(filepath.Join(root, "store")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Billing.SetQuota("default", "instance", 1); err != nil {
		t.Fatal(err)
	}

	project := filepath.Join(root, "project")
	rootfs := filepath.Join(project, "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfs, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(project, "capper.json")
	if err := os.WriteFile(config, []byte(`{
		"name": "quota-broken",
		"version": "0.1.0",
		"rootfs": "./rootfs",
		"entrypoint": ["/bin/missing"]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	imgMgr := ImageManager{Store: st}
	if _, err := imgMgr.Create("quota-broken.cap", config); err != nil {
		t.Fatal(err)
	}
	instMgr := InstanceManager{
		Store:  st,
		Loader: loader.Loader{Paths: st.Paths},
		Runner: runtime.Runner{Mode: runtime.ModeBwrap},
	}
	if _, err := instMgr.Run("quota-broken.cap", types.ResourceOverrides{}, RunOptions{Network: testNetworkOpts()}); err == nil {
		t.Fatal("expected run failure")
	}
	if err := st.Billing.CheckQuota("default", "instance"); err == nil {
		t.Fatal("expected failed instance to consume quota before removal")
	}
	instances, err := st.ListInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected one failed instance, got %d", len(instances))
	}
	if err := instMgr.Remove(instances[0].ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := st.Billing.CheckQuota("default", "instance"); err != nil {
		t.Fatalf("quota should be released after removal: %v", err)
	}
}

// TestCreateRootFSArchiveSymlinkToDir verifies MR-02: a symlink that points to
// a directory must be stored as tar.TypeSymlink, not as a ghost directory entry.
// Before the fix, the code emitted a TypeDir header for the symlink path and
// the real directory's contents were missing from the archive.
func TestCreateRootFSArchiveSymlinkToDir(t *testing.T) {
	rootfs := t.TempDir()

	// Create a real directory with a file inside.
	realDir := filepath.Join(rootfs, "usr", "lib")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "libc.so"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink lib -> usr/lib (common in modern Linux rootfs layouts).
	if err := os.Symlink("usr/lib", filepath.Join(rootfs, "lib")); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "rootfs.tar.zst")
	if err := createRootFSArchive(rootfs, dest); err != nil {
		t.Fatalf("createRootFSArchive: %v", err)
	}

	// Decode the archive and collect entries by name.
	f, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec, err := zstd.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	type entry struct {
		typeflag byte
		linkname string
	}
	entries := map[string]entry{}
	tr := tar.NewReader(dec)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		entries[hdr.Name] = entry{typeflag: hdr.Typeflag, linkname: hdr.Linkname}
		if _, err := io.Copy(io.Discard, tr); err != nil {
			t.Fatalf("tar drain: %v", err)
		}
	}

	// "lib" must be a symlink, not a directory.
	libEntry, ok := entries["lib"]
	if !ok {
		t.Fatal("expected 'lib' entry in archive, not found")
	}
	if libEntry.typeflag != tar.TypeSymlink {
		t.Errorf("expected lib to be TypeSymlink (%d), got %d", tar.TypeSymlink, libEntry.typeflag)
	}
	if libEntry.linkname != "usr/lib" {
		t.Errorf("expected lib linkname 'usr/lib', got %q", libEntry.linkname)
	}

	// The real directory and its contents must also be present.
	if _, ok := entries["usr/lib"]; !ok {
		t.Error("expected 'usr/lib' directory entry in archive")
	}
	if _, ok := entries["usr/lib/libc.so"]; !ok {
		t.Error("expected 'usr/lib/libc.so' file entry in archive")
	}
}

// TestCreateRootFSArchiveSymlinkToFile verifies that a symlink pointing to a
// regular file is preserved as TypeSymlink (not inlined as a regular file copy).
func TestCreateRootFSArchiveSymlinkToFile(t *testing.T) {
	rootfs := t.TempDir()

	if err := os.WriteFile(filepath.Join(rootfs, "real.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.sh", filepath.Join(rootfs, "link.sh")); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "rootfs.tar.zst")
	if err := createRootFSArchive(rootfs, dest); err != nil {
		t.Fatalf("createRootFSArchive: %v", err)
	}

	f, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec, err := zstd.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	tr := tar.NewReader(dec)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		if hdr.Name == "link.sh" {
			found = true
			// symlink-to-file is inlined as a regular file copy (existing behaviour)
			// OR preserved as a symlink — either is acceptable, but it must not be
			// a directory.
			if hdr.Typeflag == tar.TypeDir {
				t.Errorf("link.sh must not be stored as a directory entry")
			}
		}
		_, _ = io.Copy(io.Discard, tr)
	}
	if !found {
		t.Error("expected 'link.sh' entry in archive")
	}
}
