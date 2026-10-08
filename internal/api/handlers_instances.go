package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"capper/internal/adminconfig"
	"capper/internal/cgroup"
	"capper/internal/hoststorage"
	"capper/internal/ipam"
	"capper/internal/manager"
	"capper/internal/metadata"
	"capper/internal/networking"
	"capper/internal/runtime"
	"capper/internal/storagepolicy"
	"capper/internal/store"
	"capper/internal/systemlabels"
	"capper/internal/topology"
	"capper/internal/types"
	"capper/internal/vpc"
)

type createInstanceRequest struct {
	Image           string            `json:"image"`
	Name            string            `json:"name,omitempty"`
	InstanceType    string            `json:"instanceType,omitempty"`
	Network         string            `json:"network,omitempty"` // legacy
	VPCID           string            `json:"vpcId,omitempty"`
	SubnetID        string            `json:"subnetId,omitempty"`
	SecurityGroupIDs []string         `json:"securityGroupIds,omitempty"`
	KeyName         string            `json:"keyName,omitempty"`
	PrivateIPAddress string           `json:"privateIpAddress,omitempty"`
	PublicIPBehavior string          `json:"publicIpBehavior,omitempty"` // none, auto, existing-allocation, new-allocation
	AllocationID     string          `json:"allocationId,omitempty"`
	TerminationProtection *bool       `json:"terminationProtection,omitempty"`
	ShutdownBehavior string           `json:"shutdownBehavior,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	CapInitTemplate string            `json:"capInitTemplate,omitempty"`
	CapInitContent  string            `json:"capInitContent,omitempty"`
	CapInitMetadata map[string]any    `json:"capInitMetadata,omitempty"`
	Volumes         []volumeAttach    `json:"volumes,omitempty"`
	Placement       *placementRequest `json:"placement,omitempty"`
	// DiskBytes overrides the instance type's root-disk size. When a default
	// instance storage pool is configured the disk is drawn from it, so the
	// request must fit the pool's available capacity.
	DiskBytes             int64  `json:"diskBytes,omitempty"`
	LaunchTemplateID      string `json:"launchTemplateId,omitempty"`
	LaunchTemplateVersion int    `json:"launchTemplateVersion,omitempty"`
	RuntimeMode           string `json:"runtimeMode,omitempty"` // auto|bwrap|chroot|crun|runc|lxc|qemu
}

type placementRequest struct {
	Region          string            `json:"region,omitempty"`
	Zone            string            `json:"zone,omitempty"`
	Node            string            `json:"node,omitempty"`
	Strategy        string            `json:"strategy,omitempty"`
	MinZones        int               `json:"minZones,omitempty"`
	RequireLabel    map[string]string `json:"requireLabel,omitempty"`
	AntiAffinity    map[string]string `json:"antiAffinity,omitempty"`
	PlacementPolicy string            `json:"placementPolicy,omitempty"`
}

type volumeAttach struct {
	Name       string `json:"name"`
	MountPath  string `json:"mountPath"`
	Type       string `json:"type,omitempty"`       // "csd" for shared volumes; empty = classic storage
	AccessMode string `json:"accessMode,omitempty"` // "rw" (default) or "ro"
	Required   bool   `json:"required,omitempty"`   // fail instance creation if mount fails
}

func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "instance:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	instances, err := s.ctrl.Instances.List()
	if err != nil {
		writeInternal(w, err)
		return
	}
	visible := make([]types.Instance, 0, len(instances))
	for _, inst := range instances {
		if systemlabels.IsHidden(inst.Labels) {
			continue
		}
		visible = append(visible, inst)
	}
	writeData(w, visible, nil)
}

func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:inspect", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	inst, err := s.ctrl.Store.ResolveInstance(id)
	if err != nil {
		writeNotFound(w, "instance not found")
		return
	}
	_ = s.ctrl.Instances.Refresh(inst)
	writeData(w, inst, instanceCaps(s, r, inst.ID))
}

// PATCH /api/v1/instances/{id} — update mutable instance properties: resource
// limits (memory/cpu/pids/file-size), restart policy, and labels. Memory and
// pids limits are applied live to a running instance's cgroup; cpu-time and
// file-size limits (rlimits) and other changes take effect on the next start.
func (s *Server) handlePatchInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:update", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	inst, err := s.ctrl.Store.ResolveInstance(id)
	if err != nil {
		writeNotFound(w, "instance not found")
		return
	}
	var req struct {
		Resources     *types.ResourceLimits `json:"resources"`
		RestartPolicy *string               `json:"restartPolicy"`
		Labels        map[string]string     `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.Resources != nil {
		if req.Resources.MemoryBytes > 0 {
			inst.Resources.MemoryBytes = req.Resources.MemoryBytes
		}
		if req.Resources.DiskBytes > 0 {
			inst.Resources.DiskBytes = req.Resources.DiskBytes
		}
		if req.Resources.CPUCount > 0 {
			inst.Resources.CPUCount = req.Resources.CPUCount
		}
		if req.Resources.CPUTimeSecs > 0 {
			inst.Resources.CPUTimeSecs = req.Resources.CPUTimeSecs
		}
		if req.Resources.MaxProcesses > 0 {
			inst.Resources.MaxProcesses = req.Resources.MaxProcesses
		}
		if req.Resources.FileSizeBytes > 0 {
			inst.Resources.FileSizeBytes = req.Resources.FileSizeBytes
		}
	}
	if req.RestartPolicy != nil {
		inst.RestartPolicy = types.RestartPolicy(*req.RestartPolicy)
	}
	if req.Labels != nil {
		inst.Labels = req.Labels
	}
	if err := s.ctrl.Store.UpdateInstance(*inst); err != nil {
		writeInternal(w, err)
		return
	}
	// Live-apply memory/pids to a running instance via its cgroup; cpu-time and
	// file-size rlimits require a restart.
	liveApplied := false
	needsRestart := req.Resources != nil && (req.Resources.CPUTimeSecs > 0 || req.Resources.FileSizeBytes > 0 || req.Resources.DiskBytes > 0)
	if inst.Status == types.StatusRunning {
		if cgm := cgroup.Open(inst.ID); cgm != nil {
			_ = cgm.Apply(inst.Resources)
			liveApplied = true
		}
	}
	s.recordEvent(r, "instance", inst.ID, "instance.updated", nil)
	writeData(w, map[string]any{
		"instance":     inst,
		"liveApplied":  liveApplied,
		"needsRestart": needsRestart,
	}, instanceCaps(s, r, inst.ID))
}

func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "instance:run", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req createInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := mergeLaunchTemplateIntoRequest(s, s.project, &req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.Image == "" {
		writeBadRequest(w, fmt.Errorf("image is required"))
		return
	}

	if req.Network != "" {
		writeBadRequest(w, fmt.Errorf("network field is removed; use subnetId"))
		return
	}
	if req.SubnetID == "" {
		writeBadRequest(w, fmt.Errorf("subnetId is required"))
		return
	}
	if _, err := storagepolicy.RequireDefaultPool(s.ctrl.Store.AdminConfig, s.ctrl.Store.HostStorage); err != nil {
		writeBadRequest(w, err)
		return
	}

	resources, err := s.resolveInstanceTypeResources(r, req.Image, req.InstanceType)
	if err != nil {
		writeBadRequest(w, err)
		return
	}

	// Optional root-disk size override, validated against the instance storage pool.
	if req.DiskBytes > 0 {
		if err := s.validateInstanceDiskSize(req.DiskBytes); err != nil {
			writeBadRequest(w, err)
			return
		}
		resources.Limits.DiskBytes = req.DiskBytes
		resources.DiskSet = true
	}
	diskBytes := resources.Limits.DiskBytes
	if err := storagepolicy.ValidatePoolCapacity(s.ctrl.Store.AdminConfig, s.ctrl.Store.HostStorage, diskBytes); err != nil {
		writeBadRequest(w, err)
		return
	}

	if qerr := s.ctrl.Store.Billing.CheckQuota(s.project, "instance"); qerr != nil {
		writeJSON(w, http.StatusPaymentRequired, Envelope{Error: qerr.Error()})
		return
	}
	if err := s.ctrl.Store.CheckHostDeployLimit(); err != nil {
		writeBadRequest(w, err)
		return
	}

	if req.Labels == nil {
		req.Labels = make(map[string]string)
	}
	req.Labels["project"] = s.project
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	req.Env["CAPPER_METADATA_URL"] = "http://169.254.169.254/capper/v1"
	req.Env["CAPPER_METADATA_TOKEN_FILE"] = "/run/capper/metadata-token"
	runOpts := manager.RunOptions{Name: req.Name, Labels: req.Labels, Env: req.Env, RuntimeMode: req.RuntimeMode}
	var primaryENI string
	sub, serr := networking.ResolveSubnetForLaunch(s.ctrl.Store.VPC, req.SubnetID, req.VPCID)
	if serr != nil {
		writeBadRequest(w, fmt.Errorf("subnet: %w", serr))
		return
	}
	vpcID := sub.VPCID
	if req.VPCID != "" {
		vpcID = req.VPCID
	}
	eni, eerr := s.ctrl.Store.VPC.CreateENI(vpcID, sub.ID, req.SecurityGroupIDs, req.PrivateIPAddress)
	if eerr != nil {
		writeBadRequest(w, fmt.Errorf("eni: %w", eerr))
		return
	}
	primaryENI = eni.ID
	runOpts.Network = &manager.NetworkRunOpts{
		NetworkID:   sub.ID,
		Bridge:      sub.BridgeName,
		Subnet:      sub.CIDR,
		Gateway:     sub.GatewayIP,
		PreferredIP: eni.PrimaryPrivateIP,
	}

	// Resolve topology placement using the scheduler.
	// If the caller provided a placement block, use it; otherwise default to local topology.
	placement := resolveInstancePlacement(s, req)

	// Pre-generate a mount-session ID so CSD FUSE host paths are stable before
	// the instance ID is known. The real instance ID is passed to
	// mountCSDVolumesForInstance after Run, but for the bind mount path we use
	// this pre-generated UUID to avoid a chicken-and-egg ordering issue.
	mountSessionID := uuid.New().String()
	csdBinds, _ := s.mountCSDVolumesForInstance(r.Context(), mountSessionID, req.Volumes)
	runOpts.Mounts = append(runOpts.Mounts, csdBinds...)

	inst, err := s.ctrl.Instances.Run(req.Image, resources, runOpts)
	if err != nil {
		if primaryENI != "" {
			_ = s.ctrl.Store.VPC.DeleteENI(primaryENI)
		}
		writeBadRequest(w, err)
		return
	}

	// Persist placement fields onto the instance record.
	inst.RealmID = placement.RealmID
	inst.RegionID = placement.RegionID
	inst.ZoneID = placement.ZoneID
	inst.NodeID = placement.NodeID
	inst.DesiredState = "running"
	inst.Generation = 1
	inst.InstanceType = req.InstanceType
	if req.SubnetID != "" || primaryENI != "" {
		inst.VPCID = sub.VPCID
		inst.SubnetID = sub.ID
		inst.PrimaryENIID = primaryENI
		inst.PrivateIPAddress = inst.NetworkIP
		inst.SecurityGroupIDs = req.SecurityGroupIDs
		if primaryENI != "" {
			if inst.NetworkIP == "" {
				// Soft dataplane failure (e.g. missing CAP_NET): reclaim ENI so it is
				// not left in-use without a real netns/IP, but keep the instance.
				_ = s.ctrl.Store.VPC.DeleteENI(primaryENI)
				primaryENI = ""
				inst.PrimaryENIID = ""
				inst.PrivateIPAddress = ""
			} else if _, aerr := s.ctrl.Store.VPC.AttachENI(primaryENI, inst.ID, 0); aerr != nil {
				_ = s.ctrl.Store.VPC.DeleteENI(primaryENI)
				_ = s.ctrl.Instances.Remove(inst.ID)
				writeBadRequest(w, aerr)
				return
			}
		}
	}
	if req.KeyName != "" {
		if _, kerr := s.ctrl.Store.VPC.GetKeyPair(s.project, req.KeyName); kerr != nil {
			if primaryENI != "" {
				_ = s.ctrl.Store.VPC.DeleteENI(primaryENI)
			}
			_ = s.ctrl.Instances.Remove(inst.ID)
			writeBadRequest(w, fmt.Errorf("key pair %q not found", req.KeyName))
			return
		}
		inst.KeyName = req.KeyName
	}
	if err := s.applyPublicIPBehavior(req, inst, primaryENI); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.TerminationProtection != nil {
		inst.TerminationProtection = *req.TerminationProtection
	}
	if req.ShutdownBehavior != "" {
		inst.ShutdownBehavior = req.ShutdownBehavior
	}
	if req.Tags != nil {
		inst.Tags = req.Tags
	}
	_ = s.ctrl.Store.UpdateInstance(*inst)

	_ = s.ctrl.Store.Billing.RecordUsage(s.project, "instance", inst.ID, "count", 1)

	// Block 10: create metadata record after a successful launch.
	if inst.NetworkIP != "" {
		gateway := ""
		if runOpts.Network != nil {
			gateway = runOpts.Network.Gateway
		}
		userData := req.CapInitContent
		if req.KeyName != "" {
			if kp, kerr := s.ctrl.Store.VPC.GetKeyPair(s.project, req.KeyName); kerr == nil && kp.PublicKey != "" {
				keyBlock := "#capper-ssh-key\n" + kp.PublicKey + "\n"
				if userData != "" {
					userData = userData + "\n" + keyBlock
				} else {
					userData = keyBlock
				}
			}
		}
		_, _ = s.ctrl.Store.Metadata.CreateRecord(metadata.InstanceMetadata{
			InstanceID: inst.ID,
			Hostname:   inst.Name,
			Project:    s.project,
			Labels:     inst.Labels,
			NetworkIP:  inst.NetworkIP,
			Gateway:    gateway,
			DNS:        gateway,
			UserData:   userData,
		})
	}

	for _, vol := range req.Volumes {
		if vol.Name == "" || vol.MountPath == "" {
			continue
		}
		_ = s.storage.AttachVolume(vol.Name, inst.ID, vol.MountPath)
	}
	if req.CapInitTemplate != "" || req.CapInitContent != "" || len(req.CapInitMetadata) > 0 {
		meta := req.CapInitMetadata
		if meta == nil {
			meta = map[string]any{}
		}
		if req.CapInitTemplate != "" {
			meta["templateId"] = req.CapInitTemplate
		}
		if req.CapInitContent != "" {
			meta["userData"] = req.CapInitContent
		}
		_ = saveInstanceMetadata(s, inst.ID, meta)
	}
	s.recordEvent(r, "instance", inst.ID, "instance.created", map[string]any{"image": req.Image})
	writeJSON(w, http.StatusCreated, Envelope{Data: inst, Capabilities: instanceCaps(s, r, inst.ID)})
}

func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:delete", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	inst, ierr := s.ctrl.Store.ResolveInstance(id)
	if ierr == nil && inst.TerminationProtection {
		writeError(w, http.StatusConflict, "instance has termination protection enabled")
		return
	}
	// Unmount any CSD FUSE volumes attached to this instance.
	s.unmountCSDVolumesForInstance(id)

	if err := s.ctrl.Instances.Remove(id); err != nil {
		writeBadRequest(w, err)
		return
	}
	s.reclaimInstanceENIs(id, inst, ierr == nil)
	_ = s.ctrl.Store.Billing.ReleaseUsage(s.project, "instance", id)
	s.recordEvent(r, "instance", id, "instance.deleted", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStopInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:stop", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	inst, stopped, err := s.ctrl.Instances.Stop(id, 5*time.Second, false)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	if stopped {
		s.recordEvent(r, "instance", inst.ID, "instance.stopped", nil)
	}
	writeData(w, inst, instanceCaps(s, r, inst.ID))
}

func (s *Server) handleStartInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:run", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	inst, err := s.restartInstanceProcess(id)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "instance", inst.ID, "instance.started", nil)
	writeData(w, inst, instanceCaps(s, r, inst.ID))
}

func (s *Server) handleRestartInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:stop", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.authorize(r, "instance:run", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	inst, err := s.ctrl.Store.ResolveInstance(id)
	if err != nil {
		writeNotFound(w, "instance not found")
		return
	}
	if inst.Status == types.StatusRunning {
		if _, _, err := s.ctrl.Instances.Stop(id, 5*time.Second, false); err != nil {
			writeBadRequest(w, err)
			return
		}
	}
	inst, err = s.restartInstanceProcess(id)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "instance", inst.ID, "instance.restarted", nil)
	writeData(w, inst, instanceCaps(s, r, inst.ID))
}

func (s *Server) restartInstanceProcess(ref string) (*types.Instance, error) {
	inst, err := s.ctrl.Store.ResolveInstance(ref)
	if err != nil {
		return nil, fmt.Errorf("instance not found")
	}
	instDir := filepath.Dir(inst.RootFSPath)
	specPath := filepath.Join(instDir, "launch.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("no launch spec (re-create instance from image): %w", err)
	}
	var spec struct {
		Manifest types.CapsuleManifest `json:"manifest"`
		NetNS    string                `json:"netNS"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	runner := runtime.Runner{Mode: s.ctrl.Instances.Runner.Mode}
	pid, err := runner.Start(inst.ID, instDir, spec.Manifest, runtime.StartOptions{NetNS: spec.NetNS})
	if err != nil {
		return nil, err
	}
	started := time.Now().UTC().Format(time.RFC3339)
	inst.PID = pid
	inst.Status = types.StatusRunning
	inst.StartedAt = started
	inst.StoppedAt = nil
	if err := s.ctrl.Store.UpdateInstance(*inst); err != nil {
		return nil, err
	}
	if err := s.ctrl.Store.WriteInstanceJSON(*inst); err != nil {
		return nil, err
	}
	return inst, nil
}

func (s *Server) handleInstanceLogs(w http.ResponseWriter, r *http.Request) {
	s.serveInstanceLog(w, r, "both")
}

func (s *Server) handleInstanceLogStdout(w http.ResponseWriter, r *http.Request) {
	s.serveInstanceLog(w, r, "stdout")
}

func (s *Server) handleInstanceLogStderr(w http.ResponseWriter, r *http.Request) {
	s.serveInstanceLog(w, r, "stderr")
}

func (s *Server) serveInstanceLog(w http.ResponseWriter, r *http.Request, which string) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:logs", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	inst, err := s.ctrl.Store.ResolveInstance(id)
	if err != nil {
		writeNotFound(w, "instance not found")
		return
	}
	instDir := filepath.Dir(inst.RootFSPath)
	follow := r.URL.Query().Get("follow") == "true"

	if follow {
		s.streamLogFile(r.Context(), w, instDir, which)
		return
	}

	out := map[string]string{}
	if which == "stdout" || which == "both" {
		out["stdout"] = readLogFile(filepath.Join(instDir, "stdout.log"))
	}
	if which == "stderr" || which == "both" {
		out["stderr"] = readLogFile(filepath.Join(instDir, "stderr.log"))
	}
	out["startupError"] = readLogFile(filepath.Join(instDir, "startup-error"))
	writeData(w, out, nil)
}

func readLogFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *Server) streamLogFile(ctx context.Context, w http.ResponseWriter, instDir, which string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	files := []string{}
	if which == "stdout" || which == "both" {
		files = append(files, filepath.Join(instDir, "stdout.log"))
	}
	if which == "stderr" || which == "both" {
		files = append(files, filepath.Join(instDir, "stderr.log"))
	}

	offsets := make([]int64, len(files))
	for {
		if ctx.Err() != nil {
			return
		}
		for i, f := range files {
			data, err := readFileFrom(f, offsets[i])
			if err == nil && len(data) > 0 {
				if _, err := w.Write(data); err != nil {
					return
				}
				offsets[i] += int64(len(data))
				flusher.Flush()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func readFileFrom(path string, offset int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

func (s *Server) handleInstanceEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:inspect", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	events, err := s.ctrl.Store.Events.List(store.ListEventsOptions{
		ResourceType: "instance",
		ResourceID:   id,
		Limit:        queryInt(r, "limit", 50),
	})
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, events, nil)
}

// instancePlacement holds the resolved topology IDs for a new instance.
type instancePlacement struct {
	RealmID  string
	RegionID string
	ZoneID   string
	NodeID   string
}

// resolveInstancePlacement uses the scheduler to pick realm/region/zone/node for
// a new instance. Falls back to the local topology if the scheduler finds no candidates.
func resolveInstancePlacement(s *Server, req createInstanceRequest) instancePlacement {
	schedReq := topology.PlacementRequest{Project: s.project}
	if req.Placement != nil {
		schedReq.Region = req.Placement.Region
		schedReq.Zone = req.Placement.Zone
		schedReq.Strategy = req.Placement.Strategy
		schedReq.MinZones = req.Placement.MinZones
		schedReq.RequireLabel = req.Placement.RequireLabel
		schedReq.AntiAffinity = req.Placement.AntiAffinity
	}

	sched := topology.NewScheduler(s.ctrl.Store.Topology.Store())
	result := sched.Simulate(context.Background(), schedReq)
	if result.Allowed && len(result.Candidates) > 0 {
		best := result.Candidates[0]
		return instancePlacement{
			RealmID:  best.RealmID,
			RegionID: best.RegionID,
			ZoneID:   best.ZoneID,
			NodeID:   best.NodeID,
		}
	}

	// Fallback: use first available node in local topology.
	nodes, _ := s.ctrl.Store.Topology.Store().ListNodes("")
	for _, n := range nodes {
		if n.Status == topology.StatusReady {
			return instancePlacement{
				RealmID:  n.RealmID,
				RegionID: n.RegionID,
				ZoneID:   n.ZoneID,
				NodeID:   n.ID,
			}
		}
	}
	return instancePlacement{}
}

// validateInstanceDiskSize ensures a requested root-disk size fits the configured
// instance storage pool's available capacity. With no pool configured the disk
// lives under the store path, so only a positive size is required.
func (s *Server) validateInstanceDiskSize(diskBytes int64) error {
	if diskBytes <= 0 {
		return fmt.Errorf("disk size must be positive")
	}
	return storagepolicy.ValidatePoolCapacity(s.ctrl.Store.AdminConfig, s.ctrl.Store.HostStorage, diskBytes)
}

// instanceDiskCapacity is the wizard's view of how much disk a new instance may
// claim and from where.
type instanceDiskCapacity struct {
	PoolConfigured bool   `json:"poolConfigured"`
	PoolName       string `json:"poolName,omitempty"`
	Backend        string `json:"backend,omitempty"`
	AvailableBytes int64  `json:"availableBytes"`
	TotalBytes     int64  `json:"totalBytes"`
	Degraded       bool   `json:"degraded,omitempty"`
}

// GET /api/v1/instance-disk-capacity — capacity available for a new instance
// disk drawn from the configured instance storage pool (e.g. a local LVM VG).
func (s *Server) handleInstanceDiskCapacity(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "instance:run", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	resp := instanceDiskCapacity{}
	poolID := ""
	if s.ctrl.Store.AdminConfig != nil {
		if v, ok, _ := s.ctrl.Store.AdminConfig.Get(adminconfig.KeyDefaultInstancePool); ok {
			poolID = v
		}
	}
	if poolID != "" {
		if pool, err := hoststorage.NewManager(s.ctrl.Store.HostStorage).GetPool(poolID); err == nil {
			resp = instanceDiskCapacity{
				PoolConfigured: true,
				PoolName:       pool.Name,
				Backend:        pool.Backend,
				AvailableBytes: pool.AvailableBytes,
				TotalBytes:     pool.TotalBytes,
				Degraded:       pool.Health == hoststorage.PoolDegraded,
			}
		}
	}
	writeData(w, resp, nil)
}

func (s *Server) resolveInstanceTypeResources(r *http.Request, image, typeName string) (types.ResourceOverrides, error) {
	if typeName == "" {
		return types.ResourceOverrides{}, nil
	}
	if err := s.authorize(r, "compute:type:use", "project:"+s.project); err != nil {
		return types.ResourceOverrides{}, err
	}
	it, err := s.ctrl.Store.Compute.GetInstanceType(typeName)
	if err != nil {
		return types.ResourceOverrides{}, fmt.Errorf("instance type %q not found", typeName)
	}
	overrides := types.ResourceOverrides{}
	if it.MemoryBytes > 0 {
		overrides.Limits.MemoryBytes = it.MemoryBytes
		overrides.MemorySet = true
	}
	if it.PIDLimit > 0 {
		overrides.Limits.MaxProcesses = int64(it.PIDLimit)
		overrides.PidsSet = true
	}
	if it.DiskBytes > 0 {
		overrides.Limits.DiskBytes = it.DiskBytes
		overrides.DiskSet = true
	}
	if it.CPUCount > 0 {
		overrides.Limits.CPUCount = int64(it.CPUCount)
		overrides.CPUSet = true
	}
	return overrides, nil
}

func (s *Server) applyPublicIPBehavior(req createInstanceRequest, inst *types.Instance, primaryENI string) error {
	behavior := req.PublicIPBehavior
	if behavior == "" || behavior == "none" {
		return nil
	}
	mgr := s.ipamManager()
	switch behavior {
	case "auto", "new-allocation":
		poolID, purpose, err := s.pickPublicIPPool()
		if err != nil {
			return err
		}
		ip, err := mgr.Reserve(ipam.ReserveOptions{
			PoolID: poolID, Project: s.project, Name: inst.Name + "-eip", Purpose: purpose,
		})
		if err != nil {
			return fmt.Errorf("publicIpBehavior auto: %w", err)
		}
		if _, err := mgr.Attach(ip.ID, ipam.IPBinding{
			TargetType: "instance", TargetID: inst.ID, BindingMode: ipam.ModeFloating,
		}); err != nil {
			_ = mgr.Release(ip.ID)
			return fmt.Errorf("publicIpBehavior auto attach: %w", err)
		}
		inst.PublicIPAddress = ip.Address
		return nil
	case "existing-allocation":
		if req.AllocationID == "" {
			return fmt.Errorf("allocationId is required when publicIpBehavior is existing-allocation")
		}
		ip, err := s.ipamStore().GetIP(req.AllocationID)
		if err != nil {
			return fmt.Errorf("allocation %q not found", req.AllocationID)
		}
		if _, err := mgr.Attach(ip.ID, ipam.IPBinding{
			TargetType: "instance", TargetID: inst.ID, BindingMode: ipam.ModeFloating,
		}); err != nil {
			return fmt.Errorf("publicIpBehavior existing-allocation: %w", err)
		}
		inst.PublicIPAddress = ip.Address
		return nil
	default:
		return fmt.Errorf("unsupported publicIpBehavior %q (use none, auto, or existing-allocation)", behavior)
	}
}

func (s *Server) pickPublicIPPool() (poolID, purpose string, err error) {
	pools, err := s.ipamStore().ListPools()
	if err != nil {
		return "", "", err
	}
	preferred := []string{ipam.UsageFloating, ipam.UsagePassthrough, ipam.UsageReserved}
	for _, purpose := range preferred {
		for _, p := range pools {
			if p.Status != ipam.PoolActive || !p.AllowAutoAllocate {
				continue
			}
			for _, u := range p.Usage {
				if u == purpose {
					return p.ID, purpose, nil
				}
			}
		}
	}
	return "", "", fmt.Errorf("no active auto-allocate public IP pool available for floating/passthrough use")
}

func (s *Server) reclaimInstanceENIs(instanceID string, inst *types.Instance, haveInst bool) {
	seen := map[string]bool{}
	var enis []vpc.ENI
	if instanceID != "" {
		enis, _ = s.ctrl.Store.VPC.ListENIsByInstance(instanceID)
	}
	primaryID := ""
	if haveInst && inst != nil {
		primaryID = inst.PrimaryENIID
		if primaryID != "" {
			if e, err := s.ctrl.Store.VPC.GetENI(primaryID); err == nil {
				enis = append(enis, e)
			}
		}
	}
	for _, e := range enis {
		if e.ID == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		if !e.DeleteOnTermination && e.ID != primaryID {
			_, _ = s.ctrl.Store.VPC.DetachENI(e.ID)
			continue
		}
		_ = s.ctrl.Store.VPC.DeleteENI(e.ID)
	}
}
