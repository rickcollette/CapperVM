package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleListLaunchTemplates(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "launch-template:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	templates, err := s.ctrl.Store.VPC.ListLaunchTemplates(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, templates, nil)
}

func (s *Server) handleCreateLaunchTemplate(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "launch-template:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name   string         `json:"name"`
		Config map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	t, err := s.ctrl.Store.VPC.CreateLaunchTemplate(s.project, req.Name, req.Config)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: t})
}

func (s *Server) handleGetLaunchTemplate(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "launch-template:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	t, err := s.ctrl.Store.VPC.GetLaunchTemplate(s.project, r.PathValue("templateId"))
	if err != nil {
		writeNotFound(w, "launch template not found")
		return
	}
	writeData(w, t, nil)
}
