package host

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunDoctor executes all capability checks for the local host and returns
// results in a fixed order. storeRoot is used for disk-space checks.
func RunDoctor(storeRoot string) []DoctorResult {
	return []DoctorResult{
		checkKernelVersion(),
		checkCgroupV2(),
		checkUserNamespaces(),
		checkBwrap(),
		checkCrun(),
		checkRunc(),
		checkLXC(),
		checkQEMU(),
		checkQEMUImg(),
		checkKVM(),
		checkNftables(),
		checkDisk(storeRoot),
		checkClockSync(),
	}
}

// ---- individual checks ------------------------------------------------------

func checkKernelVersion() DoctorResult {
	const name = "kernel >= 5.15"
	if kernelAtLeast(5, 15, 0) {
		return pass(name, "kernel "+kernelVersion())
	}
	return fail(name, "kernel "+kernelVersion()+" is below 5.15; upgrade for full cgroup v2 support")
}

func checkCgroupV2() DoctorResult {
	const name = "cgroup v2"
	controllers := "/sys/fs/cgroup/cgroup.controllers"
	if _, err := os.Stat(controllers); err != nil {
		return fail(name, "cgroup v2 not found (missing "+controllers+")")
	}
	subtree := "/sys/fs/cgroup/cgroup.subtree_control"
	data, err := os.ReadFile(subtree)
	if err != nil {
		return fail(name, "cannot read cgroup.subtree_control: "+err.Error())
	}
	controllers_enabled := strings.TrimSpace(string(data))
	return pass(name, "controllers enabled: "+controllers_enabled)
}

func checkUserNamespaces() DoctorResult {
	const name = "unprivileged user namespaces"
	path := "/proc/sys/kernel/unprivileged_userns_clone"
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Some kernels don't have this sysctl; assume enabled.
		return pass(name, "sysctl not present (enabled by default on this kernel)")
	}
	if err != nil {
		return fail(name, "cannot read "+path+": "+err.Error())
	}
	if strings.TrimSpace(string(data)) == "1" {
		return pass(name, "unprivileged_userns_clone=1")
	}
	return fail(name, "unprivileged_userns_clone=0; run: sysctl -w kernel.unprivileged_userns_clone=1")
}

func checkBwrap() DoctorResult {
	return checkBinary("bwrap", "bubblewrap (bwrap)")
}

func checkCrun() DoctorResult {
	return checkBinary("crun", "crun OCI runtime")
}

func checkRunc() DoctorResult {
	return checkBinary("runc", "runc OCI runtime")
}

func checkNftables() DoctorResult {
	const name = "nftables"
	path, err := exec.LookPath("nft")
	if err != nil {
		return fail(name, "nft not found in PATH; install nftables")
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return fail(name, "nft --version failed: "+err.Error())
	}
	version := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	return pass(name, version)
}

func checkDisk(path string) DoctorResult {
	const name = "disk space (>= 10 GiB)"
	if path == "" {
		path = "/tmp"
	}
	avail, err := diskFreeBytes(path)
	if err != nil {
		return fail(name, "statfs failed: "+err.Error())
	}
	const minBytes = 10 * 1024 * 1024 * 1024
	msg := fmt.Sprintf("%s available on %s", humanBytes(avail), path)
	if avail >= minBytes {
		return pass(name, msg)
	}
	return fail(name, msg+" (need >= 10 GiB)")
}

func checkClockSync() DoctorResult {
	const name = "clock sync (chrony/ntpd/timesyncd)"
	for _, daemon := range []string{"chronyd", "ntpd", "systemd-timesyncd"} {
		if err := exec.Command("pgrep", "-x", daemon).Run(); err == nil {
			return pass(name, daemon+" is running")
		}
	}
	// timedatectl is a soft check — warn, not fail
	if out, err := exec.Command("timedatectl", "show", "--property=NTPSynchronized").Output(); err == nil {
		if strings.Contains(string(out), "NTPSynchronized=yes") {
			return pass(name, "NTPSynchronized=yes (via timedatectl)")
		}
	}
	return DoctorResult{
		Check:   name,
		Pass:    false,
		Message: "no NTP daemon detected; clock drift may affect log timestamps",
	}
}


func checkLXC() DoctorResult {
	const name = "lxc (lxc-start)"
	if _, err := exec.LookPath("lxc-start"); err != nil {
		return fail(name, "lxc-start not found; install lxc for --runtime lxc / runtimeMode=lxc")
	}
	return checkBinary("lxc-start", name)
}

func checkQEMU() DoctorResult {
	const name = "qemu-system"
	for _, bin := range []string{"qemu-system-x86_64", "qemu-system-aarch64", "qemu-system-" + goarch()} {
		if path, err := exec.LookPath(bin); err == nil {
			return pass(name, path)
		}
	}
	return fail(name, "qemu-system-* not found; install qemu-system for runtimeMode=qemu")
}

func checkQEMUImg() DoctorResult {
	return checkBinary("qemu-img", "qemu-img")
}

func checkKVM() DoctorResult {
	const name = "KVM (/dev/kvm)"
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return DoctorResult{
			Check:   name,
			Pass:    false,
			Message: "/dev/kvm not usable; QEMU will fall back to TCG (slower): " + err.Error(),
		}
	}
	_ = f.Close()
	return pass(name, "/dev/kvm readable/writable")
}

func goarch() string {
	// lightweight to avoid importing runtime in doctor hot path naming
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return "x86_64"
	}
	m := strings.TrimSpace(string(out))
	switch m {
	case "x86_64", "amd64":
		return "x86_64"
	case "aarch64", "arm64":
		return "aarch64"
	default:
		return m
	}
}

// ---- helpers ----------------------------------------------------------------

func checkBinary(binary, label string) DoctorResult {
	path, err := exec.LookPath(binary)
	if err != nil {
		return fail(label, binary+" not found in PATH")
	}
	return pass(label, path)
}

func pass(check, msg string) DoctorResult {
	return DoctorResult{Check: check, Pass: true, Message: msg}
}

func fail(check, msg string) DoctorResult {
	return DoctorResult{Check: check, Pass: false, Message: msg}
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
