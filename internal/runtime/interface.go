package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"capper/internal/types"
)

// Runtime is a pluggable isolation backend for capsule instances.
type Runtime interface {
	Name() string
	Start(instanceID, instDir string, manifest types.CapsuleManifest, opts StartOptions) (pid int, err error)
	Exec(instanceID, rootfs, netNS string, command []string, user types.UserConfig) error
	Connect(instanceID, rootfs, netNS string, shells []string, user types.UserConfig) error
	Stop(instanceID string, pid int, timeout time.Duration, killNow bool) error
	StartShellPTY(instanceID, rootfs, shell, netNS string, user types.UserConfig, term string) (*exec.Cmd, *os.File, error)
}

const (
	ModeLXC  = "lxc"
	ModeQEMU = "qemu"
)

// ValidModes is the full allowlist for host and per-instance runtime selection.
var ValidModes = []string{
	ModeAuto, ModeBwrap, ModeChroot, ModeCrun, ModeRunc, ModeLXC, ModeQEMU,
}

// ValidateMode reports whether mode is an allowed runtime name.
func ValidateMode(mode string) error {
	for _, m := range ValidModes {
		if mode == m {
			return nil
		}
	}
	return fmt.Errorf("invalid runtime: %s (valid: %v)", mode, ValidModes)
}

// Resolve returns a Runtime for the given mode. auto/bwrap/chroot/crun/runc use
// the process/OCI Runner; lxc and qemu use dedicated backends.
func Resolve(mode string) Runtime {
	switch mode {
	case ModeLXC:
		return LXC{}
	case ModeQEMU:
		return QEMU{}
	case ModeBwrap, ModeChroot, ModeCrun, ModeRunc, ModeAuto, "":
		if mode == "" {
			mode = ModeAuto
		}
		return Runner{Mode: mode}
	default:
		return Runner{Mode: ModeAuto}
	}
}

// ResolvePreferred picks request → capsule preference → host default.
func ResolvePreferred(request, capsulePreferred, hostDefault string) string {
	if request != "" {
		return request
	}
	if capsulePreferred != "" {
		return capsulePreferred
	}
	if hostDefault != "" {
		return hostDefault
	}
	return ModeAuto
}

// Name implements Runtime for Runner.
func (r Runner) Name() string {
	return r.mode()
}
