package storagepolicy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"capper/internal/adminconfig"
	"capper/internal/hoststorage"
	"capper/internal/storagepolicy"
	"capper/internal/store"
)

func TestRequireDefaultPoolNotConfigured(t *testing.T) {
	if _, err := storagepolicy.RequireDefaultPool(nil, nil); err == nil {
		t.Fatal("expected error for nil stores")
	}
	st, err := store.Open(store.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	_, err = storagepolicy.RequireDefaultPool(st.AdminConfig, st.HostStorage)
	if err == nil || !strings.Contains(err.Error(), "no default storage pool") {
		t.Fatalf("got %v, want no default storage pool error", err)
	}
}

func TestRequireDefaultPoolAndCapacity(t *testing.T) {
	st, err := store.Open(store.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	dir := filepath.Join(st.Paths.Root, "pool")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pool, err := hoststorage.NewManager(st.HostStorage).CreatePool(hoststorage.CreatePoolOptions{
		Name: "p", Backend: hoststorage.BackendDirectory, Mountpoint: dir, TotalBytes: 1 << 30,
	})
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if err := st.AdminConfig.Set(adminconfig.KeyDefaultInstancePool, pool.ID); err != nil {
		t.Fatalf("set default pool: %v", err)
	}
	id, err := storagepolicy.RequireDefaultPool(st.AdminConfig, st.HostStorage)
	if err != nil || id != pool.ID {
		t.Fatalf("RequireDefaultPool: id=%q err=%v", id, err)
	}
	if err := storagepolicy.ValidatePoolCapacity(st.AdminConfig, st.HostStorage, 1<<20); err != nil {
		t.Fatalf("small request should fit: %v", err)
	}
	if err := storagepolicy.ValidatePoolCapacity(st.AdminConfig, st.HostStorage, 1<<40); err == nil {
		t.Fatal("oversized request should be rejected")
	}
}
