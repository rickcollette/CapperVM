package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleListKeyPairs(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "keypair:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	keys, err := s.ctrl.Store.VPC.ListKeyPairs(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, keys, nil)
}

func (s *Server) handleCreateKeyPair(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "keypair:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name      string `json:"name"`
		PublicKey string `json:"publicKey"`
		KeyType   string `json:"keyType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	k, err := s.ctrl.Store.VPC.ImportKeyPair(s.project, req.Name, req.PublicKey, req.KeyType)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: k})
}

func (s *Server) handleGetKeyPair(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "keypair:inspect", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	k, err := s.ctrl.Store.VPC.GetKeyPair(s.project, r.PathValue("keyName"))
	if err != nil {
		writeNotFound(w, "key pair not found")
		return
	}
	writeData(w, k, nil)
}

func (s *Server) handleDeleteKeyPair(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "keypair:delete", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.VPC.DeleteKeyPair(s.project, r.PathValue("keyName")); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRebootInstance(w http.ResponseWriter, r *http.Request) {
	s.handleRestartInstance(w, r)
}

func (s *Server) handleProtectTermination(w http.ResponseWriter, r *http.Request) {
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
	inst.TerminationProtection = true
	if err := s.ctrl.Store.WriteInstanceJSON(*inst); err != nil {
		writeInternal(w, err)
		return
	}
	_ = s.ctrl.Store.UpdateInstance(*inst)
	writeData(w, inst, nil)
}

func (s *Server) handleUnprotectTermination(w http.ResponseWriter, r *http.Request) {
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
	inst.TerminationProtection = false
	if err := s.ctrl.Store.WriteInstanceJSON(*inst); err != nil {
		writeInternal(w, err)
		return
	}
	_ = s.ctrl.Store.UpdateInstance(*inst)
	writeData(w, inst, nil)
}

func (s *Server) handleAttachENIToInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:update", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		ENIID           string `json:"eniId"`
		AttachmentIndex int    `json:"attachmentIndex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	eni, err := s.ctrl.Store.VPC.AttachENI(req.ENIID, id, req.AttachmentIndex)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, eni, nil)
}

func (s *Server) handleDetachENIFromInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "instance:update", "instance/"+id); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		ENIID string `json:"eniId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	eni, err := s.ctrl.Store.VPC.DetachENI(req.ENIID)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, eni, nil)
}
