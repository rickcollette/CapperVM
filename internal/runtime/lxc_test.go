package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"capper/internal/types"
)

func TestWriteLXCConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "lxc.conf")
	err := writeLXCConfig(cfg, "capper-test", filepath.Join(dir, "rootfs"), "capns-abc", types.CapsuleManifest{
		Hostname:   "demo",
		Entrypoint: []string{"/sbin/init"},
		Resources:  types.ResourceLimits{MemoryBytes: 256 << 20, CPUCount: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"lxc.rootfs.path = dir:",
		"lxc.namespace.share.net =",
		"lxc.cgroup2.memory.max",
		"lxc.cgroup2.cpu.max",
		"lxc.init.cmd = /sbin/init",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("config missing %q:\n%s", want, s)
		}
	}
}

func TestLXCNameSanitizes(t *testing.T) {
	got := lxcName("inst_abc/def!")
	if strings.ContainsAny(got, "/!") {
		t.Fatalf("unsanitized name: %q", got)
	}
	if !strings.HasPrefix(got, "capper-") {
		t.Fatalf("expected capper- prefix, got %q", got)
	}
}
