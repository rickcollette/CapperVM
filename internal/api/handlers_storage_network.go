package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"

	capstore "capper/internal/storage"
	"capper/internal/storagepolicy"
)

func (s *Server) handleListBuckets(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "storage:bucket:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	buckets, err := s.storage.ListBuckets()
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, buckets, nil)
}

func (s *Server) handleGetBucket(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	if err := s.authorize(r, "storage:bucket:inspect", "bucket/"+bucket); err != nil {
		writeForbidden(w, err)
		return
	}
	b, err := s.storage.GetBucket(bucket)
	if err != nil {
		writeNotFound(w, "bucket not found")
		return
	}
	writeData(w, b, nil)
}

func (s *Server) handleCreateBucket(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "storage:bucket:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name       string `json:"name"`
		Versioning bool   `json:"versioning"`
		QuotaBytes int64  `json:"quotaBytes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	b, err := s.storage.CreateBucket(capstore.CreateBucketOptions{
		Name:       req.Name,
		Versioning: req.Versioning,
		QuotaBytes: req.QuotaBytes,
	})
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: b})
}

func (s *Server) handleDeleteBucket(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	if err := s.authorize(r, "storage:bucket:delete", "bucket/"+bucket); err != nil {
		writeForbidden(w, err)
		return
	}
	force := r.URL.Query().Get("force") == "true"
	if err := s.storage.DeleteBucket(bucket, force); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListObjects(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	if err := s.authorize(r, "storage:object:list", "bucket/"+bucket); err != nil {
		writeForbidden(w, err)
		return
	}
	prefix := r.URL.Query().Get("prefix")
	objects, err := s.storage.ListObjects(bucket, prefix)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, objects, nil)
}

func (s *Server) handleDeleteObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	if err := s.authorize(r, "storage:object:delete", "bucket/"+bucket); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.storage.DeleteObject(bucket, key); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	if err := s.authorize(r, "storage:object:get", "bucket/"+bucket); err != nil {
		writeForbidden(w, err)
		return
	}
	mgr := s.storage
	meta, rc, err := mgr.ObjectService().GetObject(bucket, key)
	if err != nil {
		writeNotFound(w, "object not found")
		return
	}
	defer rc.Close()
	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	_, _ = io.Copy(w, rc)
}

func (s *Server) handlePutObject(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	if err := s.authorize(r, "storage:object:put", "bucket/"+bucket); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		writeBadRequest(w, err)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	defer file.Close()
	tmp, err := os.CreateTemp(s.ctrl.Store.Paths.Tmp, "obj-*")
	if err != nil {
		writeInternal(w, err)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		writeInternal(w, err)
		return
	}
	if err := tmp.Close(); err != nil {
		writeInternal(w, err)
		return
	}
	obj, err := s.storage.PutObject(bucket, key, tmpPath)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: obj})
}

func (s *Server) handlePutObjectRaw(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	if err := s.authorize(r, "storage:object:put", "bucket/"+bucket); err != nil {
		writeForbidden(w, err)
		return
	}
	tmp, err := os.CreateTemp(s.ctrl.Store.Paths.Tmp, "obj-*")
	if err != nil {
		writeInternal(w, err)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, r.Body); err != nil {
		tmp.Close()
		writeInternal(w, err)
		return
	}
	if err := tmp.Close(); err != nil {
		writeInternal(w, err)
		return
	}
	obj, err := s.storage.PutObject(bucket, key, tmpPath)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: obj})
}

func (s *Server) handleListVolumes(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "storage:volume:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	vols, err := s.storage.ListVolumes()
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, vols, nil)
}

func (s *Server) handleCreateVolume(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "storage:volume:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name      string `json:"name"`
		SizeBytes int64  `json:"sizeBytes"`
		Class     string `json:"class,omitempty"`
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
	v, err := s.storage.CreateVolume(capstore.CreateVolumeOptions{
		Name:      req.Name,
		SizeBytes: req.SizeBytes,
		Class:     req.Class,
	})
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: v})
}

func (s *Server) handleAttachVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "storage:volume:attach", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		InstanceID string `json:"instanceId"`
		MountPath  string `json:"mountPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := s.storage.AttachVolume(name, req.InstanceID, req.MountPath); err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, map[string]string{"status": "attached"}, nil)
}

func (s *Server) handleDetachVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "storage:volume:detach", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.storage.DetachVolume(name); err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, map[string]string{"status": "detached"}, nil)
}

func (s *Server) handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "storage:volume:delete", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.storage.DeleteVolume(name); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
