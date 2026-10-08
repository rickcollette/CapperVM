package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"capper/internal/loader"
	"capper/internal/marketplace"
	"capper/internal/sbom"
	"capper/internal/storagepolicy"
	"capper/internal/types"
)

func (s *Server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "image:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if _, err := storagepolicy.RequireDefaultPool(s.ctrl.Store.AdminConfig, s.ctrl.Store.HostStorage); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeBadRequest(w, err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeBadRequest(w, fmt.Errorf("file field required"))
		return
	}
	defer file.Close()

	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}
	if name == "" {
		writeBadRequest(w, fmt.Errorf("name is required"))
		return
	}
	if !strings.HasSuffix(name, ".cap") {
		name += ".cap"
	}
	dest := filepath.Join(s.ctrl.Store.Paths.Images, filepath.Base(name))
	tmp, err := os.CreateTemp(s.ctrl.Store.Paths.Tmp, "upload-*.cap")
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
	if err := os.Rename(tmpPath, dest); err != nil {
		writeInternal(w, err)
		return
	}
	img, err := s.registerImageFile(dest, name)
	if err != nil {
		writeInternal(w, err)
		return
	}
	s.recordEvent(r, "image", img.Name, "image.uploaded", nil)
	writeJSON(w, http.StatusCreated, Envelope{Data: img})
}

func (s *Server) handleScanImage(w http.ResponseWriter, r *http.Request) {
	name := decodeImageName(r.PathValue("name"))
	if err := s.authorize(r, "image:scan", "image/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	rootfs, cleanup, err := s.extractImageRootfs(name)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	defer cleanup()
	if s.ctrl.Store.Posture == nil {
		writeError(w, http.StatusNotImplemented, "posture scanner not configured")
		return
	}
	result, err := s.ctrl.Store.Posture.Scan(s.project, rootfs)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, result, nil)
}

func (s *Server) handleImageSBOM(w http.ResponseWriter, r *http.Request) {
	name := decodeImageName(r.PathValue("name"))
	if err := s.authorize(r, "image:inspect", "image/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	ld := loader.Loader{Paths: s.ctrl.Store.Paths}
	loaded, cleanup, err := ld.Load(name)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	defer cleanup()
	doc := sbom.GenerateSPDX(loaded.Manifest, loaded.Digest)
	data, err := sbom.MarshalJSON(doc)
	if err != nil {
		writeInternal(w, err)
		return
	}
	embed := r.URL.Query().Get("embed") == "true"
	if embed {
		if err := sbom.EmbedInCap(loaded.ImagePath, loaded.ImagePath, sbom.EntryNameSBOM, data); err != nil {
			writeInternal(w, err)
			return
		}
		writeData(w, map[string]string{"status": "embedded", "entry": sbom.EntryNameSBOM}, nil)
		return
	}
	w.Header().Set("Content-Type", "application/spdx+json")
	w.Write(data)
}

func (s *Server) handleImageProvenance(w http.ResponseWriter, r *http.Request) {
	name := decodeImageName(r.PathValue("name"))
	if err := s.authorize(r, "image:inspect", "image/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	ld := loader.Loader{Paths: s.ctrl.Store.Paths}
	loaded, cleanup, err := ld.Load(name)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	defer cleanup()
	prov := sbom.GenerateProvenance(loaded.Manifest, filepath.Base(loaded.ImagePath), loaded.Digest)
	data, err := sbom.MarshalJSON(prov)
	if err != nil {
		writeInternal(w, err)
		return
	}
	embed := r.URL.Query().Get("embed") == "true"
	if embed {
		if err := sbom.EmbedInCap(loaded.ImagePath, loaded.ImagePath, sbom.EntryNameProvenance, data); err != nil {
			writeInternal(w, err)
			return
		}
		writeData(w, map[string]string{"status": "embedded", "entry": sbom.EntryNameProvenance}, nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handlePublishImage(w http.ResponseWriter, r *http.Request) {
	name := decodeImageName(r.PathValue("name"))
	if err := s.authorize(r, "image:publish", "image/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Description string            `json:"description,omitempty"`
		Labels      map[string]string `json:"labels,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	img, err := s.ctrl.Store.GetImage(name)
	if err != nil {
		writeNotFound(w, "image not found")
		return
	}

	listing := marketplace.MarketplaceListing{
		ID:          img.ID,
		Name:        img.Name,
		Version:     img.Version,
		Description: req.Description,
		Digest:      img.Digest,
		Status:      "pending",
		Labels:      req.Labels,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	// Run posture scan if scanner is available.
	if s.ctrl.Store.Posture != nil {
		rootfs, cleanup, serr := s.extractImageRootfs(name)
		if serr == nil {
			result, serr := s.ctrl.Store.Posture.Scan(s.project, rootfs)
			cleanup()
			if serr == nil {
				sevs := map[string]int{}
				for _, f := range result.Findings {
					sevs[string(f.Severity)]++
				}
				listing.ScanFindings = len(result.Findings)
				listing.ScanSeverities = sevs
				listing.ScanScannedAt = time.Now().UTC().Format(time.RFC3339)
				switch {
				case sevs["critical"] > 0 || sevs["high"] > 0:
					listing.ScanStatus = "fail"
				case sevs["medium"] > 0 || len(result.Findings) > 0:
					listing.ScanStatus = "warn"
				default:
					listing.ScanStatus = "pass"
				}
			}
		}
	}

	// Generate SBOM digest from the artifact tar's attestations/sbom.spdx.json entry.
	if img.Path != "" {
		digest, sbomErr := marketplace.ExtractSBOMDigest(img.Path)
		if sbomErr == nil {
			listing.SBOMDigest = digest
		}
		// If no embedded SBOM yet, generate one and embed it for future lookups.
		if listing.SBOMDigest == "" {
			doc := marketplace.GenerateSBOM(img.Name, img.Version, img.Digest, nil)
			if embedErr := marketplace.EmbedSBOMInArtifact(img.Path, doc); embedErr == nil {
				sum, _ := marketplace.ExtractSBOMDigest(img.Path)
				listing.SBOMDigest = sum
			}
		}
	}

	// Insert is an upsert (INSERT OR REPLACE), so re-publishing replaces the listing.
	if err := s.ctrl.Store.Marketplace.Insert(listing); err != nil {
		writeInternal(w, err)
		return
	}
	s.recordEvent(r, "image", img.Name, "image.published", map[string]any{"status": listing.ScanStatus})
	writeJSON(w, http.StatusCreated, Envelope{Data: listing})
}

func (s *Server) extractImageRootfs(name string) (string, func(), error) {
	ld := loader.Loader{Paths: s.ctrl.Store.Paths}
	loaded, cleanup, err := ld.Load(name)
	if err != nil {
		return "", nil, err
	}
	rootfs := filepath.Join(loaded.WorkDir, "rootfs")
	if st, err := os.Stat(rootfs); err != nil || !st.IsDir() {
		cleanup()
		return "", nil, fmt.Errorf("rootfs not found in image")
	}
	return rootfs, cleanup, nil
}

func (s *Server) registerImageFile(dest, name string) (types.ImageRecord, error) {
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
		return types.ImageRecord{}, err
	}
	return img, nil
}
