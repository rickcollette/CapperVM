package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"capper/internal/network"
	"capper/internal/types"
)

// LXC runs capsules as LXC containers against an extracted rootfs.
type LXC struct{}

func (LXC) Name() string { return ModeLXC }

func (l LXC) Start(instanceID, instDir string, manifest types.CapsuleManifest, opts StartOptions) (int, error) {
	if _, err := exec.LookPath("lxc-start"); err != nil {
		return 0, fmt.Errorf("lxc runtime requested but lxc-start was not found")
	}
	rootfs := filepath.Join(instDir, "rootfs")
	if len(manifest.Entrypoint) == 0 {
		return 0, fmt.Errorf("manifest entrypoint is required for lxc")
	}
	name := lxcName(instanceID)
	cfgPath := filepath.Join(instDir, "lxc.conf")
	if err := writeLXCConfig(cfgPath, name, rootfs, opts.NetNS, manifest); err != nil {
		return 0, err
	}
	// Ensure a defined container pointing at our config (lxc-start -f is enough for many setups).
	cmd := exec.Command("lxc-start", "-n", name, "-f", cfgPath, "-d")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		// Capture stderr for diagnostics.
		out, _ := exec.Command("lxc-start", "-n", name, "-f", cfgPath, "-d").CombinedOutput()
		writeStartupError(instDir, fmt.Errorf("lxc-start: %v: %s", err, strings.TrimSpace(string(out))))
		return 0, fmt.Errorf("lxc-start: %w", err)
	}
	pid, err := lxcInitPID(name)
	if err != nil {
		_ = exec.Command("lxc-stop", "-n", name, "-k").Run()
		return 0, err
	}
	_ = os.WriteFile(filepath.Join(instDir, "lxc.name"), []byte(name), 0o644)
	return pid, nil
}

func (l LXC) Exec(instanceID, rootfs, netNS string, command []string, user types.UserConfig) error {
	_ = rootfs
	_ = netNS
	if len(command) == 0 {
		return fmt.Errorf("exec requires at least one command argument")
	}
	if _, err := exec.LookPath("lxc-attach"); err != nil {
		return fmt.Errorf("lxc-attach not found")
	}
	args := []string{"-n", lxcName(instanceID), "--"}
	args = append(args, command...)
	cmd := exec.Command("lxc-attach", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = user
	return cmd.Run()
}

func (l LXC) Connect(instanceID, rootfs, netNS string, shells []string, user types.UserConfig) error {
	shell := "/bin/sh"
	for _, s := range shells {
		if s != "" {
			shell = s
			break
		}
	}
	return l.Exec(instanceID, rootfs, netNS, []string{shell, "-l"}, user)
}

func (l LXC) Stop(instanceID string, pid int, timeout time.Duration, killNow bool) error {
	name := lxcName(instanceID)
	if _, err := exec.LookPath("lxc-stop"); err != nil {
		return stopByPID(pid, timeout, killNow)
	}
	args := []string{"-n", name}
	if killNow {
		args = append(args, "-k")
	} else {
		sec := int(timeout.Seconds())
		if sec < 1 {
			sec = 10
		}
		args = append(args, "-t", strconv.Itoa(sec))
	}
	if err := exec.Command("lxc-stop", args...).Run(); err != nil {
		return stopByPID(pid, timeout, true)
	}
	return nil
}

func (l LXC) StartShellPTY(instanceID, rootfs, shell, netNS string, user types.UserConfig, term string) (*exec.Cmd, *os.File, error) {
	_ = rootfs
	_ = netNS
	_ = user
	if shell == "" {
		shell = "/bin/sh"
	}
	if _, err := exec.LookPath("lxc-attach"); err != nil {
		return nil, nil, fmt.Errorf("lxc-attach not found")
	}
	cmd := exec.Command("lxc-attach", "-n", lxcName(instanceID), "--", "env", "TERM="+shellTerm(term), shell, "-l")
	return startPTY(cmd)
}

func lxcName(instanceID string) string {
	name := "capper-" + instanceID
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func writeLXCConfig(path, name, rootfs, netNS string, manifest types.CapsuleManifest) error {
	var b strings.Builder
	b.WriteString("lxc.uts.name = " + name + "\n")
	b.WriteString("lxc.rootfs.path = dir:" + rootfs + "\n")
	b.WriteString("lxc.tty.max = 0\n")
	b.WriteString("lxc.pty.max = 1024\n")
	b.WriteString("lxc.cap.drop =\n")
	b.WriteString("lxc.mount.auto = proc:mixed sys:mixed cgroup:mixed\n")
	if manifest.Hostname != "" {
		b.WriteString("lxc.uts.name = " + manifest.Hostname + "\n")
	}
	if netNS != "" {
		// Join Capper-prepared netns; empty = LXC creates its own (isolated).
		b.WriteString("lxc.net.0.type = none\n")
		b.WriteString("# Capper netns: " + network.NetNSPath(netNS) + "\n")
		// lxc.namespace.share.net requires absolute path on newer LXC.
		b.WriteString("lxc.namespace.share.net = " + network.NetNSPath(netNS) + "\n")
	} else {
		b.WriteString("lxc.net.0.type = empty\n")
	}
	if mem := manifest.Resources.MemoryBytes; mem > 0 {
		b.WriteString(fmt.Sprintf("lxc.cgroup2.memory.max = %d\n", mem))
	}
	if cpu := manifest.Resources.CPUCount; cpu > 0 {
		// Approximate CPU limit: cpu.max quota for period 100000.
		quota := int(cpu * 100000)
		if quota < 1000 {
			quota = 1000
		}
		b.WriteString(fmt.Sprintf("lxc.cgroup2.cpu.max = %d 100000\n", quota))
	}
	init := strings.Join(manifest.Entrypoint, " ")
	if len(manifest.Args) > 0 {
		init += " " + strings.Join(manifest.Args, " ")
	}
	if init != "" {
		b.WriteString("lxc.init.cmd = " + init + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func lxcInitPID(name string) (int, error) {
	out, err := exec.Command("lxc-info", "-n", name, "-pH").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("lxc-info: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("lxc-info: invalid pid %q", strings.TrimSpace(string(out)))
	}
	return pid, nil
}
