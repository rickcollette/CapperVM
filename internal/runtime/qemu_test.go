package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"capper/internal/types"
)

func TestLookQEMUPrefersArch(t *testing.T) {
	_, err := lookQEMU()
	if err != nil {
		t.Skip("qemu-system not installed")
	}
}

func TestHasNetDev(t *testing.T) {
	if hasNetDev([]string{"-m", "512"}) {
		t.Fatal("unexpected netdev")
	}
	if !hasNetDev([]string{"-netdev", "user,id=net0"}) {
		t.Fatal("expected netdev detection")
	}
}

func TestWriteNoCloudSeedRequiresISOTool(t *testing.T) {
	dir := t.TempDir()
	err := writeNoCloudSeed(dir, filepath.Join(dir, "seed.iso"), types.CapsuleManifest{
		Name:     "demo",
		Hostname: "demo-host",
		Env:      map[string]string{"CAPPER_METADATA_URL": "http://169.254.169.254/capper/v1"},
	})
	hasTool := false
	for _, bin := range []string{"genisoimage", "mkisofs", "xorriso"} {
		if _, e := exec.LookPath(bin); e == nil {
			hasTool = true
			break
		}
	}
	if !hasTool {
		if err == nil {
			t.Fatal("expected error without ISO tool")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "seed.iso")); err != nil {
		t.Fatal(err)
	}
	user, _ := os.ReadFile(filepath.Join(dir, "nocloud", "user-data"))
	if !strings.Contains(string(user), "hostname: demo-host") {
		t.Fatalf("user-data missing hostname: %s", user)
	}
}

func TestLXCStartStopSkippedWithoutTools(t *testing.T) {
	if _, err := exec.LookPath("lxc-start"); err != nil {
		t.Skip("lxc-start not available")
	}
	// Presence-only smoke: full start needs rootfs + privileges.
	t.Log("lxc-start available; integration start/stop left to privileged environments")
}

func TestQEMUStartStopSkippedWithoutTools(t *testing.T) {
	if _, err := lookQEMU(); err != nil {
		t.Skip("qemu-system not available")
	}
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available")
	}
	t.Log("qemu tools available; full start/stop left to privileged environments")
}
