package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"capper/internal/compute"
	"capper/internal/loader"
	"capper/internal/storagepolicy"
	"capper/internal/types"
)

func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "image:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	images, err := s.ctrl.Images.List()
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, images, nil)
}

func (s *Server) handleGetImage(w http.ResponseWriter, r *http.Request) {
	name := decodeImageName(r.PathValue("name"))
	if err := s.authorize(r, "image:inspect", "image/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	img, err := s.ctrl.Store.GetImage(name)
	if err != nil {
		writeNotFound(w, "image not found")
		return
	}
	manifest, _ := s.loadImageManifest(img.Name)
	writeData(w, map[string]any{
		"image":    img,
		"manifest": manifest,
	}, imageCaps(s, r, name))
}

func (s *Server) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	name := decodeImageName(r.PathValue("name"))
	if err := s.authorize(r, "image:delete", "image/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Images.Delete(name); err != nil {
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "image", name, "image.deleted", nil)
	w.WriteHeader(http.StatusNoContent)
}

func decodeImageName(name string) string {
	name, _ = url.PathUnescape(name)
	return name
}

func (s *Server) loadImageManifest(imageName string) (any, error) {
	ld := loader.Loader{Paths: s.ctrl.Store.Paths}
	loaded, cleanup, err := ld.Load(imageName)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return loaded.Manifest, nil
}

func (s *Server) handleListCapsuleTypes(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "type:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	types, err := s.ctrl.Store.Compute.ListInstanceTypes()
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, types, nil)
}

func (s *Server) handleGetCapsuleType(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "type:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	it, err := s.ctrl.Store.Compute.GetInstanceType(name)
	if err != nil {
		writeNotFound(w, "capsule type not found")
		return
	}
	writeData(w, it, nil)
}

func (s *Server) handleCreateCapsuleType(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "type:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name        string `json:"name"`
		Family      string `json:"family"`
		CPUCount    int    `json:"cpuCount"`
		MemoryBytes int64  `json:"memoryBytes"`
		DiskBytes   int64  `json:"diskBytes"`
		PIDLimit    int    `json:"pidLimit"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	it := compute.InstanceType{
		Name:        req.Name,
		Family:      req.Family,
		CPUCount:    req.CPUCount,
		MemoryBytes: req.MemoryBytes,
		DiskBytes:   req.DiskBytes,
		PIDLimit:    req.PIDLimit,
		Description: req.Description,
	}
	if it.Family == "" {
		it.Family = compute.InstanceTypeFamilyCompute
	}
	created, err := compute.NewManager(s.ctrl.Store.Compute).CreateInstanceType(it)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: created})
}

func (s *Server) handleDeprecateCapsuleType(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "type:deprecate", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	it, err := compute.NewManager(s.ctrl.Store.Compute).DeprecateInstanceType(name)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "capsule-type", it.ID, "type.deprecated", map[string]any{"name": name})
	writeData(w, it, nil)
}

func (s *Server) handleDeleteCapsuleType(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "type:delete", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := compute.NewManager(s.ctrl.Store.Compute).DeleteInstanceType(r.PathValue("name")); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleImportImage(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "image:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if _, err := storagepolicy.RequireDefaultPool(s.ctrl.Store.AdminConfig, s.ctrl.Store.HostStorage); err != nil {
		writeBadRequest(w, err)
		return
	}
	var req struct {
		Path string `json:"path"`
		Name string `json:"name,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.Path == "" {
		writeBadRequest(w, fmt.Errorf("path is required"))
		return
	}
	src := req.Path
	if !filepath.IsAbs(src) {
		writeBadRequest(w, fmt.Errorf("path must be absolute"))
		return
	}
	stagingDir := s.ctrl.Store.Paths.ImportStaging
	if stagingDir == "" {
		writeBadRequest(w, fmt.Errorf("path-based import is not configured; use the upload endpoint"))
		return
	}
	resolved, err := filepath.EvalSymlinks(src)
	if err != nil {
		writeBadRequest(w, fmt.Errorf("cannot resolve path: %v", err))
		return
	}
	if !strings.HasPrefix(resolved+string(filepath.Separator), stagingDir+string(filepath.Separator)) {
		writeBadRequest(w, fmt.Errorf("path must be within the import staging directory"))
		return
	}
	name := req.Name
	if name == "" {
		name = filepath.Base(src)
	}
	if strings.ContainsAny(name, "/\\") {
		writeBadRequest(w, fmt.Errorf("invalid image name"))
		return
	}
	if !strings.HasSuffix(name, ".cap") {
		name += ".cap"
	}
	dest := filepath.Join(s.ctrl.Store.Paths.Images, name)
	if err := copyFile(src, dest); err != nil {
		writeInternal(w, err)
		return
	}
	digest, _ := loader.FileDigest(dest)
	info, _ := os.Stat(dest)
	id, _ := randomHex(8)
	img := types.ImageRecord{
		ID:        id,
		Name:      name,
		Path:      dest,
		Digest:    digest,
		SizeBytes: info.Size(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.ctrl.Store.UpsertImage(img); err != nil {
		writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: img})
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
