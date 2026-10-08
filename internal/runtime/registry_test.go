package runtime

import "testing"

func TestValidateModeAllowlist(t *testing.T) {
	for _, mode := range ValidModes {
		if err := ValidateMode(mode); err != nil {
			t.Fatalf("valid mode %q rejected: %v", mode, err)
		}
	}
	if err := ValidateMode("firecracker"); err == nil {
		t.Fatal("expected invalid mode to fail")
	}
}

func TestResolvePreferredOrder(t *testing.T) {
	if got := ResolvePreferred("lxc", "qemu", "bwrap"); got != "lxc" {
		t.Fatalf("request should win, got %q", got)
	}
	if got := ResolvePreferred("", "qemu", "bwrap"); got != "qemu" {
		t.Fatalf("capsule preference should win, got %q", got)
	}
	if got := ResolvePreferred("", "", "chroot"); got != "chroot" {
		t.Fatalf("host default should win, got %q", got)
	}
	if got := ResolvePreferred("", "", ""); got != ModeAuto {
		t.Fatalf("empty should be auto, got %q", got)
	}
}

func TestResolveBackends(t *testing.T) {
	if Resolve(ModeLXC).Name() != ModeLXC {
		t.Fatal("expected LXC backend")
	}
	if Resolve(ModeQEMU).Name() != ModeQEMU {
		t.Fatal("expected QEMU backend")
	}
	if Resolve(ModeBwrap).Name() != ModeBwrap {
		t.Fatal("expected bwrap runner")
	}
}
