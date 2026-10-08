package runtime

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"capper/internal/types"
)

// QEMU runs capsules as virtual machines. The MVP converts the extracted
// rootfs into a qcow2 disk (via virt-make-fs when available) and boots with
// a nocloud seed ISO for hostname/metadata.
type QEMU struct{}

func (QEMU) Name() string { return ModeQEMU }

func (q QEMU) Start(instanceID, instDir string, manifest types.CapsuleManifest, opts StartOptions) (int, error) {
	qemuBin, err := lookQEMU()
	if err != nil {
		return 0, err
	}
	if _, err := exec.LookPath("qemu-img"); err != nil {
		return 0, fmt.Errorf("qemu runtime requested but qemu-img was not found")
	}
	rootfs := filepath.Join(instDir, "rootfs")
	disk := filepath.Join(instDir, "disk.qcow2")
	if _, err := os.Stat(disk); err != nil {
		if err := buildQEMUDisk(rootfs, disk, manifest); err != nil {
			writeStartupError(instDir, err)
			return 0, err
		}
	}
	seedISO := filepath.Join(instDir, "seed.iso")
	if err := writeNoCloudSeed(instDir, seedISO, manifest); err != nil {
		return 0, err
	}
	pidFile := filepath.Join(instDir, "qemu.pid")
	qmpSock := filepath.Join(instDir, "qmp.sock")
	consoleLog := filepath.Join(instDir, "console.log")
	serialSock := filepath.Join(instDir, "serial.sock")
	_ = os.Remove(qmpSock)
	_ = os.Remove(serialSock)

	memMB := manifest.Resources.MemoryBytes / (1024 * 1024)
	if memMB < 128 {
		memMB = 512
	}
	smp := manifest.Resources.CPUCount
	if smp < 1 {
		smp = 1
	}

	args := []string{
		"-name", instanceID,
		"-machine", "q35",
		"-m", strconv.FormatInt(memMB, 10),
		"-smp", strconv.FormatInt(smp, 10),
		"-drive", fmt.Sprintf("file=%s,if=virtio,format=qcow2", disk),
		"-drive", fmt.Sprintf("file=%s,if=virtio,format=raw,media=cdrom", seedISO),
		"-nographic",
		"-serial", "file:" + consoleLog,
		"-chardev", "socket,id=serial0,path=" + serialSock + ",server=on,wait=off",
		"-device", "isa-serial,chardev=serial0",
		"-qmp", "unix:" + qmpSock + ",server=on,wait=off",
		"-pidfile", pidFile,
		"-daemonize",
	}
	if kvmAvailable() {
		args = append([]string{"-enable-kvm", "-cpu", "host"}, args...)
	} else {
		args = append([]string{"-cpu", "max"}, args...)
	}
	if opts.NetNS != "" {
		// TAP device name is prepared by the caller/net helper when present.
		tap := filepath.Join(instDir, "tap.name")
		if data, err := os.ReadFile(tap); err == nil {
			tapName := strings.TrimSpace(string(data))
			if tapName != "" {
				args = append(args,
					"-netdev", "tap,id=net0,ifname="+tapName+",script=no,downscript=no",
					"-device", "virtio-net-pci,netdev=net0",
				)
			}
		}
	}
	if opts.NetNS == "" || !hasNetDev(args) {
		args = append(args, "-netdev", "user,id=net0", "-device", "virtio-net-pci,netdev=net0")
	}

	cmd := exec.Command(qemuBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeStartupError(instDir, fmt.Errorf("qemu: %v: %s", err, strings.TrimSpace(string(out))))
		return 0, fmt.Errorf("qemu start: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, fmt.Errorf("qemu pidfile: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("qemu pidfile: invalid pid %q", strings.TrimSpace(string(pidData)))
	}
	_ = os.WriteFile(filepath.Join(instDir, "qmp.path"), []byte(qmpSock), 0o644)
	_ = os.WriteFile(filepath.Join(instDir, "serial.path"), []byte(serialSock), 0o644)
	return pid, nil
}

func (q QEMU) Exec(instanceID, rootfs, netNS string, command []string, user types.UserConfig) error {
	_ = instanceID
	_ = rootfs
	_ = netNS
	_ = command
	_ = user
	return fmt.Errorf("qemu exec is not supported in this Capper version (use serial console)")
}

func (q QEMU) Connect(instanceID, rootfs, netNS string, shells []string, user types.UserConfig) error {
	_ = rootfs
	_ = netNS
	_ = shells
	_ = user
	instDir := guessInstDir(instanceID, rootfs)
	serialPath := filepath.Join(instDir, "serial.sock")
	if data, err := os.ReadFile(filepath.Join(instDir, "serial.path")); err == nil {
		serialPath = strings.TrimSpace(string(data))
	}
	cmd := exec.Command("socat", "STDIN,raw,echo=0,escape=0x1d", "UNIX-CONNECT:"+serialPath)
	if _, err := exec.LookPath("socat"); err != nil {
		return fmt.Errorf("qemu connect requires socat to attach to serial console: %w", err)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (q QEMU) Stop(instanceID string, pid int, timeout time.Duration, killNow bool) error {
	_ = instanceID
	// Prefer QMP ACPI shutdown when available.
	if !killNow {
		if path := findQMPForPID(pid); path != "" {
			_ = qmpCommand(path, map[string]any{
				"execute": "system_powerdown",
			})
			deadline := time.Now().Add(timeout)
			for time.Now().Before(deadline) {
				if !Alive(pid) {
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	return stopByPID(pid, timeout, true)
}

func (q QEMU) StartShellPTY(instanceID, rootfs, shell, netNS string, user types.UserConfig, term string) (*exec.Cmd, *os.File, error) {
	_ = shell
	_ = netNS
	_ = user
	_ = term
	instDir := guessInstDir(instanceID, rootfs)
	serialPath := filepath.Join(instDir, "serial.sock")
	if data, err := os.ReadFile(filepath.Join(instDir, "serial.path")); err == nil {
		serialPath = strings.TrimSpace(string(data))
	}
	if _, err := exec.LookPath("socat"); err != nil {
		return nil, nil, fmt.Errorf("qemu console requires socat: %w", err)
	}
	cmd := exec.Command("socat", "-", "UNIX-CONNECT:"+serialPath)
	return startPTY(cmd)
}

func lookQEMU() (string, error) {
	candidates := []string{
		"qemu-system-" + runtime.GOARCH,
		"qemu-system-x86_64",
		"qemu-system-aarch64",
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("qemu runtime requested but qemu-system-* was not found")
}

func kvmAvailable() bool {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func hasNetDev(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "tap,") || a == "-netdev" {
			return true
		}
	}
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-netdev" {
			return true
		}
	}
	return false
}

func buildQEMUDisk(rootfs, disk string, manifest types.CapsuleManifest) error {
	sizeBytes := manifest.Resources.DiskBytes
	if sizeBytes < 1<<30 {
		sizeBytes = 2 << 30 // 2 GiB default
	}
	sizeGB := sizeBytes / (1 << 30)
	if sizeGB < 1 {
		sizeGB = 2
	}
	// Prefer virt-make-fs when available (turns a directory into a filesystem image).
	if _, err := exec.LookPath("virt-make-fs"); err == nil {
		raw := disk + ".raw"
		cmd := exec.Command("virt-make-fs", "--format=raw", "--type=ext4",
			fmt.Sprintf("--size=%dG", sizeGB), rootfs, raw)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("virt-make-fs: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		conv := exec.Command("qemu-img", "convert", "-f", "raw", "-O", "qcow2", raw, disk)
		if out, err := conv.CombinedOutput(); err != nil {
			return fmt.Errorf("qemu-img convert: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		_ = os.Remove(raw)
		return nil
	}
	// Fallback: empty qcow2 — guest must be prepared by operator/cloud-init.
	return fmt.Errorf("virt-make-fs not found: install libguestfs-tools to populate qcow2 from rootfs (qemu-img alone is not enough for a bootable disk)")
}

func writeNoCloudSeed(instDir, seedISO string, manifest types.CapsuleManifest) error {
	seedDir := filepath.Join(instDir, "nocloud")
	_ = os.MkdirAll(seedDir, 0o755)
	hostname := manifest.Hostname
	if hostname == "" {
		hostname = manifest.Name
	}
	meta := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", manifest.Name, hostname)
	user := "#cloud-config\n"
	user += "hostname: " + hostname + "\n"
	user += "manage_etc_hosts: true\n"
	if manifest.Env != nil {
		if url := manifest.Env["CAPPER_METADATA_URL"]; url != "" {
			user += "runcmd:\n  - [ sh, -c, \"echo CAPPER_METADATA_URL="+url+" >> /etc/environment\" ]\n"
		}
	}
	if err := os.WriteFile(filepath.Join(seedDir, "meta-data"), []byte(meta), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(seedDir, "user-data"), []byte(user), 0o644); err != nil {
		return err
	}
	// Prefer genisoimage/mkisofs/xorriso for seed ISO.
	for _, bin := range []string{"genisoimage", "mkisofs", "xorriso"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		var cmd *exec.Cmd
		if bin == "xorriso" {
			cmd = exec.Command(bin, "-as", "mkisofs", "-output", seedISO, "-volid", "cidata", "-joliet", "-rock", seedDir)
		} else {
			cmd = exec.Command(bin, "-output", seedISO, "-volid", "cidata", "-joliet", "-rock", seedDir)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w (%s)", bin, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	// Last resort: raw seed directory marker (QEMU may not boot without ISO).
	return fmt.Errorf("no genisoimage/mkisofs/xorriso found to build nocloud seed ISO")
}

func qmpCommand(sockPath string, msg map[string]any) error {
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	// Read greeting
	buf := make([]byte, 4096)
	_, _ = conn.Read(buf)
	enc := json.NewEncoder(conn)
	_ = enc.Encode(map[string]any{"execute": "qmp_capabilities"})
	_, _ = conn.Read(buf)
	if err := enc.Encode(msg); err != nil {
		return err
	}
	_, _ = conn.Read(buf)
	return nil
}

func findQMPForPID(pid int) string {
	if pid <= 0 {
		return ""
	}
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		parts := strings.Split(string(data), "\x00")
		for _, part := range parts {
			if strings.HasPrefix(part, "unix:") && strings.Contains(part, "qmp") {
				path := strings.TrimPrefix(part, "unix:")
				if idx := strings.Index(path, ","); idx >= 0 {
					path = path[:idx]
				}
				return path
			}
		}
	}
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return ""
	}
	for _, name := range []string{"qmp.sock", "qmp.path"} {
		p := filepath.Join(cwd, name)
		if name == "qmp.path" {
			if data, err := os.ReadFile(p); err == nil {
				return strings.TrimSpace(string(data))
			}
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func guessInstDir(instanceID, rootfs string) string {
	if rootfs != "" {
		return filepath.Dir(rootfs)
	}
	_ = instanceID
	return ""
}
