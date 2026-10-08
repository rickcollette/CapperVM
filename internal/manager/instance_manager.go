package manager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"capper/internal/adminconfig"
	"capper/internal/cgroup"
	"capper/internal/diskquota"
	"capper/internal/hoststorage"
	"capper/internal/loader"
	"capper/internal/network"
	"capper/internal/runtime"
	"capper/internal/store"
	"capper/internal/types"
)

type InstanceManager struct {
	Store  *store.Store
	Loader loader.Loader
	Runner runtime.Runner
}

// NetworkRunOpts specifies a network to attach the instance to at launch time.
// All fields must be populated by the caller (resolved from the network store).
type NetworkRunOpts struct {
	NetworkID   string // store ID of the network
	Bridge      string // Linux bridge name
	Subnet      string // CIDR, e.g. "10.42.0.0/24"
	Gateway     string // gateway IP (also used as DNS resolver)
	PreferredIP string // optional; leave empty for automatic IPAM allocation
}

type RunOptions struct {
	Name          string              // custom instance name; empty means generate a random name
	Mounts        []types.Mount       // additional bind mounts merged over manifest mounts
	Ports         []types.PortMapping // additional port mappings merged over manifest ports
	RestartPolicy types.RestartPolicy // override restart policy; empty means use manifest value
	Network       *NetworkRunOpts     // optional; nil means no network attachment
	Env           map[string]string   // extra env vars merged over manifest env (e.g. injected secrets)
	Labels        map[string]string   // metadata labels attached to the instance at launch
	Entrypoint    []string            // optional entrypoint override
	Args          []string            // optional args override (used with Entrypoint)
	RuntimeMode   string              // optional per-instance runtime; empty → host default
}

// setupInstanceDisk provisions the instance's size-capped upper layer. When an
// admin has configured a default storage pool, the disk is drawn from that pool
// (a directory-backed image on the pool mount, or an LVM logical volume) so its
// capacity is accounted against real host storage; otherwise it falls back to a
// loop image under the instance directory. A pool that is full or unhealthy
// fails the launch loudly rather than silently ignoring the limit.
func (m InstanceManager) setupInstanceDisk(instID, instDir string, diskBytes int64) error {
	if diskBytes <= 0 {
		return diskquota.SetupOverlay(instDir, diskBytes)
	}
	poolID := ""
	if m.Store.AdminConfig != nil {
		if v, ok, _ := m.Store.AdminConfig.Get(adminconfig.KeyDefaultInstancePool); ok {
			poolID = v
		}
	}
	if poolID == "" {
		return fmt.Errorf("no default storage pool configured: register one under Admin → Storage")
	}
	hs := hoststorage.NewManager(m.Store.HostStorage)
	alloc, err := hs.Allocate(hoststorage.AllocateOptions{
		PoolID: poolID, Owner: instID, Name: instID, SizeBytes: diskBytes,
	})
	if err != nil {
		return fmt.Errorf("instance disk: %w", err)
	}
	if alloc.Device != "" {
		// LVM-backed: the logical volume is already ext4 and sized; use it directly.
		if err := diskquota.SetupOverlayDevice(instDir, alloc.Device); err != nil {
			_ = hs.Release(alloc.ID)
			return err
		}
		return nil
	}
	// Directory-backed: place the size-capped image inside the allocation dir.
	backing := filepath.Join(alloc.Path, "disk.img")
	if err := diskquota.SetupOverlayBacking(instDir, diskBytes, backing); err != nil {
		_ = hs.Release(alloc.ID)
		return err
	}
	return nil
}

func (m InstanceManager) Run(imageName string, resources types.ResourceOverrides, opts RunOptions) (*types.Instance, error) {
	// Enforce per-project instance quota before doing any expensive work.
	project := opts.Labels["project"]
	if project == "" {
		project = "default"
	}
	if err := m.Store.Billing.CheckQuota(project, "instance"); err != nil {
		return nil, err
	}
	if err := m.Store.CheckHostDeployLimit(); err != nil {
		return nil, err
	}

	// Every instance must launch into a VPC subnet for metadata reachability.
	if opts.Network == nil {
		return nil, fmt.Errorf("subnet is required: instances must launch into a VPC subnet")
	}

	loaded, cleanup, err := m.Loader.Load(imageName)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	id, err := randomHexID()
	if err != nil {
		return nil, err
	}
	imageBase := filepath.Base(loaded.ImagePath)
	var name string
	if opts.Name != "" {
		exists, err := m.Store.InstanceNameExists(opts.Name)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("instance name already in use: %s", opts.Name)
		}
		name = opts.Name
	} else {
		name, err = randomName(imageBase, m.Store.InstanceNameExists)
		if err != nil {
			return nil, err
		}
	}
	instDir := filepath.Join(m.Store.Paths.Instances, id)
	rootfs := filepath.Join(instDir, "rootfs")
	if err := os.MkdirAll(rootfs, 0o700); err != nil {
		return nil, err
	}
	if err := loader.ExtractRootFS(filepath.Join(loaded.WorkDir, loaded.Manifest.RootFS.Archive), rootfs, loaded.Manifest.RootFS.Compression); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	effectiveResources := loaded.Manifest.Resources
	if !resources.Empty() {
		effectiveResources = resources.Apply(effectiveResources)
	}
	loaded.Manifest.Resources = effectiveResources
	if err := m.setupInstanceDisk(id, instDir, effectiveResources.DiskBytes); err != nil {
		_ = os.RemoveAll(instDir)
		return nil, err
	}
	// Generate masked /proc files so guests see only their own resource allocation.
	_ = runtime.WriteProcOverrides(instDir, effectiveResources)
	if len(opts.Mounts) > 0 {
		loaded.Manifest.Mounts = append(loaded.Manifest.Mounts, opts.Mounts...)
	}
	if len(opts.Ports) > 0 {
		loaded.Manifest.Network.Ports = append(loaded.Manifest.Network.Ports, opts.Ports...)
	}
	if len(opts.Env) > 0 {
		if loaded.Manifest.Env == nil {
			loaded.Manifest.Env = make(map[string]string)
		}
		for k, v := range opts.Env {
			loaded.Manifest.Env[k] = v
		}
	}
	restartPolicy := loaded.Manifest.RestartPolicy
	if opts.RestartPolicy != "" {
		restartPolicy = opts.RestartPolicy
		loaded.Manifest.RestartPolicy = restartPolicy
	}
	cmd := strings.Join(append(append([]string{}, loaded.Manifest.Entrypoint...), loaded.Manifest.Args...), " ")
	inst := types.Instance{
		ID:            id,
		Name:          name,
		Image:         imageBase,
		ImageDigest:   loaded.Digest,
		Status:        types.StatusCreated,
		CreatedAt:     now,
		RootFSPath:    rootfs,
		Entrypoint:    loaded.Manifest.Entrypoint,
		Args:          loaded.Manifest.Args,
		Shell:         loaded.Manifest.Shell,
		User:          loaded.Manifest.User,
		Command:       cmd,
		Resources:     effectiveResources,
		RestartPolicy: restartPolicy,
	}
	runtimeMode := runtime.ResolvePreferred(opts.RuntimeMode, loaded.Manifest.PreferredRuntime, m.Runner.Mode)
	if err := runtime.ValidateMode(runtimeMode); err != nil {
		_ = os.RemoveAll(instDir)
		return nil, err
	}
	inst.RuntimeMode = runtimeMode
	if img, err := m.Store.GetImage(imageBase); err == nil {
		inst.ImageID = img.ID
	}
	if len(opts.Labels) > 0 {
		inst.Labels = opts.Labels
	}
	if len(opts.Entrypoint) > 0 {
		loaded.Manifest.Entrypoint = opts.Entrypoint
		loaded.Manifest.Args = opts.Args
		inst.Entrypoint = opts.Entrypoint
		inst.Args = opts.Args
		cmd := strings.Join(append(append([]string{}, opts.Entrypoint...), opts.Args...), " ")
		inst.Command = cmd
	}

	// Network attachment: allocate IP, create veth pair, configure named netns,
	// and inject resolv.conf before the process starts.
	var startNetNS string
	if n := opts.Network; n != nil {
		netStore := m.Store.Networks
		fakeNet := network.Network{
			ID:      n.NetworkID,
			Bridge:  n.Bridge,
			Subnet:  n.Subnet,
			Gateway: n.Gateway,
		}
		lease, lerr := network.AllocateIP(netStore, fakeNet, id, n.PreferredIP)
		if lerr != nil {
			_ = os.RemoveAll(instDir)
			return nil, fmt.Errorf("network: allocate IP: %w", lerr)
		}
		hostVeth, instanceVeth := network.VethNames(id)
		netAttached := true
		if verr := network.CreateVeth(n.Bridge, hostVeth, instanceVeth); verr != nil {
			fmt.Fprintf(os.Stderr, "instance %s: create veth: %v\n", id, verr)
			_ = network.ReleaseIP(netStore, n.NetworkID, id)
			netAttached = false
		}
		if netAttached {
			prefix := network.SubnetPrefix(n.Subnet)
			if nerr := network.SetupInstanceNetNS(id, instanceVeth, lease.IP, prefix, n.Gateway); nerr != nil {
				fmt.Fprintf(os.Stderr, "instance %s: setup netns: %v\n", id, nerr)
				_ = network.DeleteVeth(hostVeth)
				_ = network.ReleaseIP(netStore, n.NetworkID, id)
				netAttached = false
			}
		}
		if netAttached {
			injectResolvConf(rootfs, n.Gateway)
			inst.NetworkID = n.NetworkID
			inst.NetworkIP = lease.IP
			startNetNS = network.NetNSName(id)
		}
	}

	if err := m.Store.WriteInstanceJSON(inst); err != nil {
		return nil, err
	}
	if err := m.Store.InsertInstance(inst); err != nil {
		return nil, err
	}
	// Record instance quota usage (best-effort; never fail launch on billing error).
	_ = m.Store.Billing.RecordUsage(project, "instance", id, "count", 1)

	rt := m.resolveRuntime(inst.RuntimeMode)
	if inst.RuntimeMode == runtime.ModeQEMU && opts.Network != nil && opts.Network.Bridge != "" {
		tapName := network.TAPName(id)
		if terr := network.CreateTAPOnBridge(opts.Network.Bridge, tapName); terr != nil {
			fmt.Fprintf(os.Stderr, "instance %s: create qemu tap: %v\n", id, terr)
		} else {
			_ = network.WriteTAPName(instDir, tapName)
		}
	}
	pid, err := rt.Start(id, instDir, loaded.Manifest, runtime.StartOptions{NetNS: startNetNS})
	if err != nil {
		_ = network.DeleteTAP(network.TAPName(id))
		if startNetNS != "" {
			_ = network.TeardownInstanceNetNS(id)
			hostVeth, _ := network.VethNames(id)
			_ = network.DeleteVeth(hostVeth)
			_ = network.ReleaseIP(m.Store.Networks, inst.NetworkID, id)
		}
		inst.Status = types.StatusFailed
		_ = m.Store.UpdateInstance(inst)
		_ = m.Store.WriteInstanceJSON(inst)
		_ = os.RemoveAll(instDir) // clean up partially-extracted rootfs
		return nil, err
	}
	// Best-effort cgroup v2 setup: apply resource limits and track the PID.
	if cgm, cgErr := cgroup.New(id); cgErr == nil && cgm != nil {
		_ = cgm.Apply(effectiveResources)
		_ = cgm.AddPID(pid)
	}
	started := time.Now().UTC().Format(time.RFC3339)
	inst.PID = pid
	inst.StartedAt = started
	// Probe whether the process is still alive after a short grace period.
	// Use a deadline loop rather than a fixed sleep so fast-exiting processes
	// are detected quickly and long-starting ones get enough time.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !runtime.Alive(pid) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !runtime.Alive(pid) {
		if setupErr := startupError(instDir); setupErr != "" {
			inst.Status = types.StatusFailed
			_ = m.Store.UpdateInstance(inst)
			_ = m.Store.WriteInstanceJSON(inst)
			return nil, fmt.Errorf("instance failed during startup: %s", setupErr)
		}
		nowStopped := time.Now().UTC().Format(time.RFC3339)
		inst.Status = types.StatusStopped
		inst.StoppedAt = &nowStopped
		if err := writePID(instDir, pid); err != nil {
			return nil, err
		}
		if err := m.Store.UpdateInstance(inst); err != nil {
			return nil, err
		}
		if err := m.Store.WriteInstanceJSON(inst); err != nil {
			return nil, err
		}
		return &inst, nil
	}
	inst.Status = types.StatusRunning
	if err := writePID(instDir, pid); err != nil {
		return nil, err
	}
	if err := m.Store.WriteInstanceJSON(inst); err != nil {
		return nil, err
	}
	if err := m.Store.UpdateInstance(inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

func (m InstanceManager) Remove(ref string) error {
	inst, err := m.Store.ResolveInstance(ref)
	if err != nil {
		return fmt.Errorf("instance not found: %s", ref)
	}
	if err := m.Refresh(inst); err != nil {
		return err
	}
	if inst.Status == types.StatusRunning {
		return fmt.Errorf("cannot remove running instance: stop it first")
	}
	instDir := filepath.Dir(inst.RootFSPath)
	diskquota.Teardown(instDir)
	m.detachNetwork(inst) // best-effort: tear down netns/veth/IPAM lease
	if err := os.RemoveAll(instDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Return any pool-backed disk capacity this instance drew.
	_ = hoststorage.NewManager(m.Store.HostStorage).ReleaseByOwner(inst.ID)
	// Best-effort cgroup cleanup: the cgroup should be empty by now since the
	// instance is stopped, but Remove() will fail silently if it isn't.
	if cgm := cgroup.Open(inst.ID); cgm != nil {
		_ = cgm.Remove()
	}
	if err := m.Store.DeleteInstance(inst.ID); err != nil {
		return err
	}
	project := inst.Labels["project"]
	if project == "" {
		project = "default"
	}
	_ = m.Store.Billing.ReleaseUsage(project, "instance", inst.ID)
	return nil
}

func (m InstanceManager) Exec(ref string, command []string) error {
	inst, err := m.Store.ResolveInstance(ref)
	if err != nil {
		return fmt.Errorf("instance not found: %s", ref)
	}
	if err := m.Refresh(inst); err != nil {
		return err
	}
	if inst.Status != types.StatusRunning {
		return fmt.Errorf("instance is not running: %s\n\nStatus:\n  %s", ref, inst.Status)
	}
	return m.resolveRuntime(inst.RuntimeMode).Exec(inst.ID, inst.RootFSPath, instNetNS(inst), command, inst.User)
}

// StartShellPTY opens an interactive shell with a PTY for WebSocket terminal
// sessions. term is the connecting client's terminal type (empty => a sane
// default is used so curses apps like top work).
func (m InstanceManager) StartShellPTY(ref, term string) (*exec.Cmd, *os.File, error) {
	inst, err := m.Store.ResolveInstance(ref)
	if err != nil {
		return nil, nil, fmt.Errorf("instance not found: %s", ref)
	}
	if err := m.Refresh(inst); err != nil {
		return nil, nil, err
	}
	if inst.Status != types.StatusRunning {
		return nil, nil, fmt.Errorf("instance is not running: %s", ref)
	}
	shell := runtime.PickShell(inst.RootFSPath, inst.Shell)
	return m.resolveRuntime(inst.RuntimeMode).StartShellPTY(inst.ID, inst.RootFSPath, shell, instNetNS(inst), inst.User, term)
}

func startupError(instDir string) string {
	data, err := os.ReadFile(filepath.Join(instDir, "startup-error"))
	if err != nil || len(data) == 0 {
		return ""
	}
	text := strings.TrimSpace(string(data))
	if len(text) > 500 {
		return text[len(text)-500:]
	}
	return text
}

func writePID(instDir string, pid int) error {
	return os.WriteFile(filepath.Join(instDir, "pid"), []byte(fmt.Sprintf("%d\n", pid)), 0o644)
}

func (m InstanceManager) Connect(ref string) error {
	inst, err := m.Store.ResolveInstance(ref)
	if err != nil {
		return fmt.Errorf("instance not found: %s", ref)
	}
	if err := m.Refresh(inst); err != nil {
		return err
	}
	if inst.Status != types.StatusRunning {
		return fmt.Errorf("instance is not running: %s\n\nStatus:\n  %s", ref, inst.Status)
	}
	shells := []string{}
	if inst.Shell != "" {
		shells = append(shells, inst.Shell)
	}
	shells = append(shells, "/bin/sh", "/bin/bash", "/busybox/sh")
	return m.resolveRuntime(inst.RuntimeMode).Connect(inst.ID, inst.RootFSPath, instNetNS(inst), unique(shells), inst.User)
}

// instNetNS returns the named network namespace for the instance, or "" if none.
func instNetNS(inst *types.Instance) string {
	if inst.NetworkID != "" {
		return network.NetNSName(inst.ID)
	}
	return ""
}

func (m InstanceManager) List() ([]types.Instance, error) {
	instances, err := m.Store.ListInstances()
	if err != nil {
		return nil, err
	}
	for i := range instances {
		if err := m.Refresh(&instances[i]); err != nil {
			return nil, err
		}
	}
	return instances, nil
}

func (m InstanceManager) Stop(ref string, timeout time.Duration, killNow bool) (*types.Instance, bool, error) {
	inst, err := m.Store.ResolveInstance(ref)
	if err != nil {
		return nil, false, fmt.Errorf("instance not found: %s", ref)
	}
	if err := m.Refresh(inst); err != nil {
		return nil, false, err
	}
	if inst.Status != types.StatusRunning {
		return inst, false, nil
	}
	if err := m.resolveRuntime(inst.RuntimeMode).Stop(inst.ID, inst.PID, timeout, killNow); err != nil {
		return nil, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	inst.Status = types.StatusStopped
	inst.StoppedAt = &now
	if err := m.Store.UpdateInstance(*inst); err != nil {
		return nil, false, err
	}
	if err := m.Store.WriteInstanceJSON(*inst); err != nil {
		return nil, false, err
	}
	return inst, true, nil
}

func (m InstanceManager) Refresh(inst *types.Instance) error {
	if inst.Status == types.StatusRunning && !runtime.Alive(inst.PID) {
		now := time.Now().UTC().Format(time.RFC3339)
		inst.Status = types.StatusStopped
		inst.StoppedAt = &now
		if err := m.Store.UpdateInstance(*inst); err != nil {
			return err
		}
		return m.Store.WriteInstanceJSON(*inst)
	}
	return nil
}

// injectResolvConf writes a minimal resolv.conf into the instance rootfs so
// DNS queries resolve via the network gateway (which runs the Capper DNS daemon).
func injectResolvConf(rootfs, dnsIP string) {
	dir := filepath.Join(rootfs, "etc")
	_ = os.MkdirAll(dir, 0o755)
	content := "nameserver " + dnsIP + "\noptions ndots:1\n"
	_ = os.WriteFile(filepath.Join(dir, "resolv.conf"), []byte(content), 0o644)
}

// detachNetwork tears down the named netns and veth pair for an instance that
// was attached to a network, and releases its IPAM lease.
func (m InstanceManager) detachNetwork(inst *types.Instance) {
	_ = network.DeleteTAP(network.TAPName(inst.ID))
	_ = network.TeardownInstanceNetNS(inst.ID)
	hostVeth, _ := network.VethNames(inst.ID)
	_ = network.DeleteVeth(hostVeth)
	// Release every lease this instance holds — not just inst.NetworkID — so a
	// stale or unset NetworkID can't leave an orphaned lease behind that would
	// later block network deletion and IP reuse.
	if leases, err := m.Store.Networks.LeasesForInstance(inst.ID); err == nil {
		for _, l := range leases {
			_ = network.ReleaseIP(m.Store.Networks, l.NetworkID, inst.ID)
		}
	}
	if inst.NetworkID != "" {
		_ = network.ReleaseIP(m.Store.Networks, inst.NetworkID, inst.ID)
	}
}

func unique(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func (m InstanceManager) resolveRuntime(mode string) runtime.Runtime {
	if mode == "" {
		mode = m.Runner.Mode
	}
	return runtime.Resolve(mode)
}
