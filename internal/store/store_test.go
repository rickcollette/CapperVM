package store

import (
	"os"
	"testing"

	"capper/internal/types"
)

func TestStoreImageAndInstanceRepositories(t *testing.T) {
	st, err := Open(NewPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	img := types.ImageRecord{
		ID:        "abc12345",
		Name:      "hello.cap",
		Version:   "0.1.0",
		Path:      "/tmp/hello.cap",
		CreatedAt: "2026-06-08T00:00:00Z",
		SizeBytes: 12,
		Digest:    "sha256:abc",
	}
	if err := st.UpsertImage(img); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetImage("hello.cap")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != img.Name || got.Version != img.Version {
		t.Fatalf("unexpected image: %#v", got)
	}

	inst := types.Instance{
		ID:          "deadbeef",
		Name:        "hello-quiet-raven",
		Image:       "hello.cap",
		ImageID:     img.ID,
		ImageDigest: img.Digest,
		PID:         123,
		Status:      types.StatusRunning,
		CreatedAt:   "2026-06-08T00:01:00Z",
		StartedAt:   "2026-06-08T00:01:01Z",
		RootFSPath:  "/tmp/rootfs",
		Command:     "/bin/sh",
	}
	if err := st.InsertInstance(inst); err != nil {
		t.Fatal(err)
	}
	resolved, err := st.ResolveInstance(inst.Name)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != inst.ID || resolved.PID != inst.PID {
		t.Fatalf("unexpected instance: %#v", resolved)
	}
}

func TestResolveInstanceMergesInstanceJSON(t *testing.T) {
	st, err := Open(NewPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	inst := types.Instance{
		ID:                    "deadbeef",
		Name:                  "hello-quiet-raven",
		Image:                 "hello.cap",
		ImageID:               "img12345",
		ImageDigest:           "sha256:abc",
		PID:                   123,
		Status:                types.StatusRunning,
		CreatedAt:             "2026-06-08T00:01:00Z",
		StartedAt:             "2026-06-08T00:01:01Z",
		RootFSPath:            st.Paths.Instances + "/deadbeef/rootfs",
		Entrypoint:            []string{"/bin/sh"},
		Args:                  []string{"-c", "sleep 3600"},
		Shell:                 "/bin/ash",
		User:                  types.UserConfig{UID: 1000, GID: 1000},
		RuntimeMode:           "qemu",
		TerminationProtection: true,
		Command:               "/bin/sh -c sleep 3600",
	}
	if err := st.InsertInstance(inst); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.Paths.Instances+"/deadbeef", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := st.WriteInstanceJSON(inst); err != nil {
		t.Fatal(err)
	}
	resolved, err := st.ResolveInstance(inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Shell != "/bin/ash" {
		t.Fatalf("expected shell from instance.json, got %q", resolved.Shell)
	}
	if len(resolved.Entrypoint) != 1 || resolved.Entrypoint[0] != "/bin/sh" {
		t.Fatalf("expected entrypoint from instance.json, got %#v", resolved.Entrypoint)
	}
	if resolved.User.UID != 1000 || resolved.User.GID != 1000 {
		t.Fatalf("expected user from instance.json, got %#v", resolved.User)
	}
	if resolved.RuntimeMode != "qemu" {
		t.Fatalf("expected runtime mode from instance.json, got %q", resolved.RuntimeMode)
	}
	if !resolved.TerminationProtection {
		t.Fatal("expected termination protection from instance.json")
	}
	inst.TerminationProtection = false
	if err := st.WriteInstanceJSON(inst); err != nil {
		t.Fatal(err)
	}
	cleared, err := st.ResolveInstance(inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.TerminationProtection {
		t.Fatal("rewriting instance.json with protection disabled must clear it")
	}
}

func TestListImagesResolvesRelativePathsAgainstStoreParent(t *testing.T) {
	root := t.TempDir()
	storeRoot := root + "/store"
	st, err := Open(NewPaths(storeRoot))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	imagePath := storeRoot + "/images/alpine.cap"
	if err := os.WriteFile(imagePath, []byte("cap"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertImage(types.ImageRecord{
		ID:        "abc12345",
		Name:      "alpine.cap",
		Version:   "1",
		Path:      "store/images/alpine.cap",
		CreatedAt: "2026-06-08T00:00:00Z",
		SizeBytes: 3,
		Digest:    "sha256:abc",
	}); err != nil {
		t.Fatal(err)
	}

	images, err := st.ListImages()
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("expected one image, got %d", len(images))
	}
	if images[0].Path != imagePath {
		t.Fatalf("expected resolved image path %q, got %q", imagePath, images[0].Path)
	}
}
