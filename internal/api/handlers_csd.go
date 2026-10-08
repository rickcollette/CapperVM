package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"capper/internal/csd"
	csdclient "capper/internal/csd/client"
	csdfuse "capper/internal/csd/fuse"
	csdserver "capper/internal/csd/server"
	"capper/internal/storagepolicy"
	"capper/internal/types"
)

func (s *Server) csdVolumes() *csdserver.VolumeManager {
	return csdserver.NewVolumeManager(s.ctrl.Store.CSD)
}

// ---- volumes ----------------------------------------------------------------

func (s *Server) handleListCSDVolumes(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "csd:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vols, err := s.csdVolumes().List(r.Context(), s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, vols, nil)
}

func (s *Server) handleCreateCSDVolume(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "csd:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name         string `json:"name"`
		Mode         string `json:"mode"`
		SizeBytes    int64  `json:"sizeBytes"`
		StorageClass string `json:"storageClass"`
		ReplicaCount int    `json:"replicaCount"`
		Encrypted    bool   `json:"encrypted"`
		EncKeyID     string `json:"encryptionKeyId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if _, err := storagepolicy.RequireDefaultPool(s.ctrl.Store.AdminConfig, s.ctrl.Store.HostStorage); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := storagepolicy.ValidatePoolCapacity(s.ctrl.Store.AdminConfig, s.ctrl.Store.HostStorage, req.SizeBytes); err != nil {
		writeBadRequest(w, err)
		return
	}
	v, err := s.csdVolumes().Create(r.Context(), csdserver.CreateVolumeOpts{
		Project:      s.project,
		Name:         req.Name,
		Mode:         req.Mode,
		SizeBytes:    req.SizeBytes,
		StorageClass: req.StorageClass,
		ReplicaCount: req.ReplicaCount,
		Encrypted:    req.Encrypted,
		EncKeyID:     req.EncKeyID,
	})
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	// Ensure root inode exists in the persistent CSD server.
	if srv := s.csd(); srv != nil {
		_, _ = srv.Metadata.EnsureRoot(r.Context(), v.ID)
	}
	s.recordEvent(r, "csd_volume", v.ID, "csd.volume.created", map[string]any{"name": req.Name})
	writeJSON(w, http.StatusCreated, Envelope{Data: v})
}

func (s *Server) handleGetCSDVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("vol")
	if err := s.authorize(r, "csd:inspect", "csd/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	v, err := s.csdVolumes().Get(r.Context(), name, s.project)
	if err != nil {
		writeNotFound(w, "volume not found")
		return
	}
	writeData(w, v, nil)
}

func (s *Server) handleDeleteCSDVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("vol")
	if err := s.authorize(r, "csd:delete", "csd/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.csdVolumes().Delete(r.Context(), name, s.project); err != nil {
		if err == csd.ErrVolumeActive {
			writeJSON(w, http.StatusConflict, Envelope{Error: err.Error()})
			return
		}
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "csd_volume", name, "csd.volume.deleted", nil)
	writeJSON(w, http.StatusNoContent, nil)
}

// ---- attachments ------------------------------------------------------------

func (s *Server) handleAttachCSDVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("vol")
	if err := s.authorize(r, "csd:attach", "csd/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		InstanceID string `json:"instanceId"`
		NodeID     string `json:"nodeId"`
		MountPath  string `json:"mountPath"`
		AccessMode string `json:"accessMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	v, err := s.csdVolumes().Get(r.Context(), name, s.project)
	if err != nil {
		writeNotFound(w, "volume not found")
		return
	}
	a, err := s.csdVolumes().Attach(r.Context(), csdserver.AttachOpts{
		VolumeID:   v.ID,
		InstanceID: req.InstanceID,
		NodeID:     req.NodeID,
		MountPath:  req.MountPath,
		AccessMode: req.AccessMode,
	})
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "csd_volume", v.ID, "csd.volume.attached",
		map[string]any{"instanceId": req.InstanceID, "mountPath": req.MountPath})
	writeJSON(w, http.StatusCreated, Envelope{Data: a})
}

func (s *Server) handleDetachCSDVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("vol")
	if err := s.authorize(r, "csd:detach", "csd/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		InstanceID string `json:"instanceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := s.csdVolumes().Detach(r.Context(), name, req.InstanceID); err != nil {
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "csd_volume", name, "csd.volume.detached",
		map[string]any{"instanceId": req.InstanceID})
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleListCSDAttachments(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("vol")
	if err := s.authorize(r, "csd:inspect", "csd/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	atts, err := s.csdVolumes().ListAttachments(r.Context(), name)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, atts, nil)
}

// ---- snapshots --------------------------------------------------------------

func (s *Server) handleCreateCSDSnapshot(w http.ResponseWriter, r *http.Request) {
	volName := r.PathValue("vol")
	if err := s.authorize(r, "csd:snapshot", "csd/"+volName); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	v, err := s.csdVolumes().Get(r.Context(), volName, s.project)
	if err != nil {
		writeNotFound(w, "volume not found")
		return
	}
	srv := s.csd()
	if srv == nil {
		writeInternal(w, fmt.Errorf("csd server unavailable"))
		return
	}
	snap, serr := srv.Snapshots.Create(r.Context(), v.ID, req.Name)
	if serr != nil {
		writeBadRequest(w, serr)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: snap})
}

func (s *Server) handleListCSDSnapshots(w http.ResponseWriter, r *http.Request) {
	volName := r.PathValue("vol")
	if err := s.authorize(r, "csd:inspect", "csd/"+volName); err != nil {
		writeForbidden(w, err)
		return
	}
	v, err := s.csdVolumes().Get(r.Context(), volName, s.project)
	if err != nil {
		writeNotFound(w, "volume not found")
		return
	}
	snaps, serr := s.ctrl.Store.CSD.Snapshots.List(v.ID)
	if serr != nil {
		writeInternal(w, serr)
		return
	}
	if snaps == nil {
		snaps = []csd.Snapshot{}
	}
	writeData(w, snaps, nil)
}

// ---- leases (admin/debug) ---------------------------------------------------

func (s *Server) handleListCSDLeases(w http.ResponseWriter, r *http.Request) {
	volName := r.PathValue("vol")
	if err := s.authorize(r, "csd:inspect", "csd/"+volName); err != nil {
		writeForbidden(w, err)
		return
	}
	v, err := s.csdVolumes().Get(r.Context(), volName, s.project)
	if err != nil {
		writeNotFound(w, "volume not found")
		return
	}
	leases, lerr := s.ctrl.Store.CSD.Leases.ForVolume(v.ID)
	if lerr != nil {
		writeInternal(w, lerr)
		return
	}
	if leases == nil {
		leases = []csd.Lease{}
	}
	writeData(w, leases, nil)
}

func (s *Server) handleRevokeCSDLeases(w http.ResponseWriter, r *http.Request) {
	volName := r.PathValue("vol")
	if err := s.authorize(r, "csd:admin", "csd/"+volName); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	v, err := s.csdVolumes().Get(r.Context(), volName, s.project)
	if err != nil {
		writeNotFound(w, "volume not found")
		return
	}
	srv := s.csd()
	if srv == nil {
		writeInternal(w, fmt.Errorf("csd server unavailable"))
		return
	}
	if err := srv.Leases.Revoke(r.Context(), v.ID, req.ClientID); err != nil {
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "csd_volume", v.ID, "csd.leases.revoked", map[string]any{"clientId": req.ClientID})
	writeJSON(w, http.StatusNoContent, nil)
}

// ---- replicas ---------------------------------------------------------------

func (s *Server) handleListCSDReplicas(w http.ResponseWriter, r *http.Request) {
	volName := r.PathValue("vol")
	if err := s.authorize(r, "csd:inspect", "csd/"+volName); err != nil {
		writeForbidden(w, err)
		return
	}
	v, err := s.csdVolumes().Get(r.Context(), volName, s.project)
	if err != nil {
		writeNotFound(w, "volume not found")
		return
	}
	replicas, rerr := s.ctrl.Store.CSD.Replicas.ListByVolume(v.ID)
	if rerr != nil {
		writeInternal(w, rerr)
		return
	}
	if replicas == nil {
		replicas = []csd.Replica{}
	}
	writeData(w, replicas, nil)
}

// ---- repair -----------------------------------------------------------------

func (s *Server) handleRepairCSDVolume(w http.ResponseWriter, r *http.Request) {
	volName := r.PathValue("vol")
	if err := s.authorize(r, "csd:admin", "csd/"+volName); err != nil {
		writeForbidden(w, err)
		return
	}
	v, err := s.csdVolumes().Get(r.Context(), volName, s.project)
	if err != nil {
		writeNotFound(w, "volume not found")
		return
	}
	srv := s.csd()
	if srv == nil {
		writeInternal(w, fmt.Errorf("csd server unavailable"))
		return
	}
	if err := srv.Journal.Replay(r.Context(), srv.Metadata, v.ID); err != nil {
		writeInternal(w, err)
		return
	}
	_ = s.ctrl.Store.CSD.Volumes.UpdateStatus(v.ID, csd.StatusAvailable)
	s.recordEvent(r, "csd_volume", v.ID, "csd.volume.repaired", nil)
	writeJSON(w, http.StatusAccepted, Envelope{Data: map[string]string{"status": "repair initiated"}})
}

// ---- CSD FUSE mount lifecycle helpers --------------------------------------

// unmountCSDVolumesForInstance unmounts all tracked CSD FUSE mounts for
// instanceID and detaches the corresponding attachment records.
func (s *Server) unmountCSDVolumesForInstance(instanceID string) {
	atts, err := s.ctrl.Store.CSD.Attachments.ListByInstance(instanceID)
	if err == nil {
		s.csdMountsMu.Lock()
		for _, a := range atts {
			if m, ok := s.csdMounts[a.ID]; ok {
				_ = m.Unmount()
				delete(s.csdMounts, a.ID)
			}
		}
		s.csdMountsMu.Unlock()
	}
	_ = s.ctrl.Store.CSD.Attachments.DeleteByInstance(instanceID)
}

// ---- CSD FUSE mount helpers for instance create ----------------------------

// csdMountResult is returned for each successfully mounted CSD volume.
type csdMountResult struct {
	attachmentID string
	mount        *csdfuse.Mount
	bindMount    types.Mount
}

// mountCSDVolumesForInstance mounts all CSD-type volumes in vols, registers
// the attachments, and returns bind mount specs to pass to bwrap.
// If a volume is Required and mounting fails, an error is returned.
func (s *Server) mountCSDVolumesForInstance(
	ctx context.Context,
	instanceID string,
	vols []volumeAttach,
) ([]types.Mount, error) {
	var binds []types.Mount

	srv := s.csd()
	if srv == nil {
		// CSD backend unavailable — skip all CSD volumes.
		for _, vol := range vols {
			if vol.Type == "csd" && vol.Required {
				return nil, fmt.Errorf("csd: server unavailable")
			}
		}
		return nil, nil
	}

	storeRoot := s.ctrl.Store.Paths.Root

	for _, vol := range vols {
		if vol.Type != "csd" {
			continue
		}
		if vol.Name == "" || vol.MountPath == "" {
			if vol.Required {
				return nil, fmt.Errorf("csd: volume entry missing name or mountPath")
			}
			continue
		}

		v, err := s.csdVolumes().Get(ctx, vol.Name, s.project)
		if err != nil {
			if vol.Required {
				return nil, fmt.Errorf("csd: volume %q not found: %w", vol.Name, err)
			}
			continue
		}

		att, err := s.csdVolumes().Attach(ctx, csdserver.AttachOpts{
			VolumeID:   v.ID,
			InstanceID: instanceID,
			MountPath:  vol.MountPath,
			AccessMode: vol.AccessMode,
		})
		if err != nil {
			if vol.Required {
				return nil, fmt.Errorf("csd: attach volume %q: %w", vol.Name, err)
			}
			continue
		}

		// Ensure root inode exists for newly created volume.
		if _, err := srv.Metadata.EnsureRoot(ctx, v.ID); err != nil {
			_ = s.csdVolumes().Detach(ctx, v.ID, instanceID)
			if vol.Required {
				return nil, fmt.Errorf("csd: ensure root inode for %q: %w", vol.Name, err)
			}
			continue
		}

		// Create host-side mount point and FUSE-mount the volume.
		hostPath := filepath.Join(storeRoot, "csd", "mounts", instanceID, att.ID)
		if err := os.MkdirAll(hostPath, 0o755); err != nil {
			_ = s.csdVolumes().Detach(ctx, v.ID, instanceID)
			if vol.Required {
				return nil, fmt.Errorf("csd: create host mount dir: %w", err)
			}
			continue
		}

		client := csdclient.NewClient(v.ID, instanceID, "local", srv)
		m := csdfuse.NewMount(client, hostPath)
		if err := m.Mount(); err != nil {
			_ = s.csdVolumes().Detach(ctx, v.ID, instanceID)
			if vol.Required {
				return nil, fmt.Errorf("csd: fuse mount volume %q: %w", vol.Name, err)
			}
			continue
		}

		// Track the live mount so we can unmount on instance delete.
		s.csdMountsMu.Lock()
		s.csdMounts[att.ID] = m
		s.csdMountsMu.Unlock()

		binds = append(binds, types.Mount{
			Source:   hostPath,
			Target:   vol.MountPath,
			ReadOnly: vol.AccessMode == csd.AccessRO,
		})
	}
	return binds, nil
}
