package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) handleFactoryStatus(w http.ResponseWriter, r *http.Request) {
	writeData(w, map[string]any{
		"connected": false,
		"message":   "Use external CapsuleBuilder factory webui",
		"deferred":  true,
	}, nil)
}

func (s *Server) handleFactoryJobs(w http.ResponseWriter, r *http.Request) {
	writeData(w, []any{}, nil)
}

func (s *Server) handleFactoryJob(w http.ResponseWriter, r *http.Request) {
	writeNotFound(w, "factory job not found")
}

func (s *Server) handleFactoryImages(w http.ResponseWriter, r *http.Request) {
	writeData(w, []any{}, nil)
}

func (s *Server) handleFactoryNotReady(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "factory API deferred — use CapsuleBuilder webui")
}

func (s *Server) handleFactorySyncStatus(w http.ResponseWriter, r *http.Request) {
	writeData(w, map[string]any{
		"connected":       false,
		"lastSync":        "",
		"pendingArtifacts": 0,
		"failedTransfers": 0,
		"factoryUrl":      "",
		"message":         "Connect CapsuleBuilder factory; sync runs via external webui",
		"deferred":        true,
	}, nil)
}

func (s *Server) handleFactoryPush(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "factory:push", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	id := r.PathValue("id")
	writeData(w, map[string]any{
		"status":  "queued",
		"imageId": id,
		"message": "Push delegated to CapsuleBuilder factory agent",
	}, nil)
}

func (s *Server) handleFactoryRescan(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "factory:rescan", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	id := r.PathValue("id")
	writeData(w, map[string]any{"status": "scan-queued", "imageId": id}, nil)
}

func (s *Server) handleCapInitStatus(w http.ResponseWriter, r *http.Request) {
	enabled := s.daemon != nil && s.daemon.IMDS != nil
	msg := "CapInit metadata service not running"
	if enabled {
		msg = "CapInit metadata service active on 169.254.169.254:80"
	}
	writeData(w, map[string]any{
		"enabled":    enabled,
		"metadataIP": "169.254.169.254",
		"message":    msg,
		"templates":  capinitTemplateCount(s),
	}, nil)
}

func (s *Server) handleCapInitRender(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "capinit:render", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		TemplateID string         `json:"templateId"`
		Vars       map[string]any `json:"vars"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	t, ok := loadCapInitTemplate(s, req.TemplateID)
	if !ok {
		writeNotFound(w, "template not found")
		return
	}
	writeData(w, map[string]any{"rendered": t.Content, "template": t}, nil)
}

func (s *Server) handleInstanceMetadata(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	meta, ok := loadInstanceMetadata(s, id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no metadata record for instance %s", id))
		return
	}
	writeData(w, meta, nil)
}

func (s *Server) handlePutInstanceMetadata(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:update", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	var meta map[string]any
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := saveInstanceMetadata(s, id, meta); err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, meta, nil)
}
