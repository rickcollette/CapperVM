package api

import (
	"encoding/json"
	"net/http"

	"capper/internal/certmgr"
)

func (s *Server) certMgr() *certmgr.CertManager {
	if s.ctrl.CertMgr == nil {
		return nil
	}
	return s.ctrl.CertMgr
}

// ---- ACME Accounts ---------------------------------------------------------

func (s *Server) handleCreateACMEAccount(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	var req struct {
		Name         string `json:"name"`
		Email        string `json:"email"`
		DirectoryURL string `json:"directoryUrl"`
		Issuer       string `json:"issuer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, "name and email are required")
		return
	}
	dirURL := req.DirectoryURL
	if dirURL == "" {
		switch req.Issuer {
		case "production", "letsencrypt":
			dirURL = certmgr.DirectoryLetsEncrypt
		default:
			dirURL = certmgr.DirectoryLetsEncryptStaging
		}
	}
	acc := certmgr.ACMEAccount{
		Name:         req.Name,
		Email:        req.Email,
		DirectoryURL: dirURL,
		Status:       "active",
	}
	created, err := s.ctrl.CertMgr.GetStore().CreateACMEAccount(acc)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: created})
}

func (s *Server) handleListACMEAccounts(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeData(w, []any{}, nil)
		return
	}
	accounts, err := s.ctrl.CertMgr.GetStore().ListACMEAccounts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, accounts, nil)
}

func (s *Server) handleGetACMEAccount(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	name := r.PathValue("acmeAccount")
	acc, err := s.ctrl.CertMgr.GetStore().GetACMEAccount(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "ACME account not found")
		return
	}
	writeData(w, acc, nil)
}

func (s *Server) handleDeleteACMEAccount(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	name := r.PathValue("acmeAccount")
	if err := s.ctrl.CertMgr.GetStore().DeleteACMEAccount(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Certificates ----------------------------------------------------------

func (s *Server) handleCreateCertificate(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	var req struct {
		Name             string   `json:"name"`
		CommonName       string   `json:"commonName"`
		SANs             []string `json:"sans"`
		Issuer           string   `json:"issuer"`
		ValidationMethod string   `json:"validationMethod"`
		ACMEAccount      string   `json:"acmeAccount"`
		AutoRenew        bool     `json:"autoRenew"`
		CertPEM          string   `json:"certPem"`
		KeyPEM           string   `json:"keyPem"`
		ChainPEM         string   `json:"chainPem"`
		Project          string   `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	ac := authContextFrom(r)
	project := req.Project
	if project == "" {
		project = ac.ProjectID
	}

	var cert *certmgr.Certificate
	var err error

	if req.Issuer == "imported" || req.CertPEM != "" {
		if req.CertPEM == "" {
			writeError(w, http.StatusBadRequest, "certPem is required for imported certificates")
			return
		}
		cert, err = s.ctrl.CertMgr.ImportCertificate(r.Context(), certmgr.ImportCertRequest{
			Project:   project,
			AccountID: ac.AccountID,
			Name:      req.Name,
			CertPEM:   req.CertPEM,
			KeyPEM:    req.KeyPEM,
			ChainPEM:  req.ChainPEM,
		})
	} else {
		if req.CommonName == "" {
			writeError(w, http.StatusBadRequest, "commonName is required")
			return
		}
		cert, err = s.ctrl.CertMgr.RequestCertificate(r.Context(), certmgr.CertRequest{
			Project:          project,
			AccountID:        ac.AccountID,
			Name:             req.Name,
			CommonName:       req.CommonName,
			SANs:             req.SANs,
			Issuer:           req.Issuer,
			ValidationMethod: req.ValidationMethod,
			ACMEAccountName:  req.ACMEAccount,
			AutoRenew:        req.AutoRenew,
		})
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: cert})
}

func (s *Server) handleListCertificates(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeData(w, []any{}, nil)
		return
	}
	ac := authContextFrom(r)
	status := r.URL.Query().Get("status")
	certs, err := s.ctrl.CertMgr.GetStore().ListCertificates("", ac.AccountID, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, certs, nil)
}

func (s *Server) handleGetCertificate(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	certID := r.PathValue("cert")
	cert, err := s.ctrl.CertMgr.GetStore().GetCertificate(certID)
	if err != nil {
		writeError(w, http.StatusNotFound, "certificate not found")
		return
	}
	writeData(w, cert, nil)
}

func (s *Server) handleRenewCertificate(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	certID := r.PathValue("cert")
	if err := s.ctrl.CertMgr.RenewCertificate(r.Context(), certID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cert, _ := s.ctrl.CertMgr.GetStore().GetCertificate(certID)
	writeData(w, cert, nil)
}

func (s *Server) handleReissueCertificate(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	certID := r.PathValue("cert")
	if err := s.ctrl.CertMgr.ReissueCertificate(r.Context(), certID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cert, _ := s.ctrl.CertMgr.GetStore().GetCertificate(certID)
	writeData(w, cert, nil)
}

func (s *Server) handleRevokeCertificate(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	certID := r.PathValue("cert")
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.ctrl.CertMgr.RevokeCertificate(r.Context(), certID, req.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteCertificate(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	certID := r.PathValue("cert")
	if err := s.ctrl.CertMgr.GetStore().DeleteCertificate(certID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Bindings --------------------------------------------------------------

func (s *Server) handleCreateCertBinding(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	certID := r.PathValue("cert")
	var req struct {
		TargetType string `json:"targetType"`
		TargetID   string `json:"targetId"`
		Hostname   string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	binding, err := s.ctrl.CertMgr.GetStore().CreateBinding(certmgr.CertificateBinding{
		CertificateID: certID,
		TargetType:    req.TargetType,
		TargetID:      req.TargetID,
		Hostname:      req.Hostname,
		Status:        "active",
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: binding})
}

func (s *Server) handleListCertBindings(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeData(w, []any{}, nil)
		return
	}
	certID := r.PathValue("cert")
	bindings, err := s.ctrl.CertMgr.GetStore().ListBindings(certID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, bindings, nil)
}

func (s *Server) handleDeleteCertBinding(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	bindingID := r.PathValue("binding")
	if err := s.ctrl.CertMgr.GetStore().DeleteBinding(bindingID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAttachCertToLB(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	lbName := r.PathValue("lb")
	var req struct {
		CertID   string `json:"certId"`
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	lbRec, err := s.ctrl.Store.LB.Get(lbName, s.project)
	if err != nil {
		writeNotFound(w, "lb not found")
		return
	}
	if err := s.ctrl.CertMgr.AttachToLoadBalancer(r.Context(), req.CertID, lbRec.ID, req.Hostname); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.ctrl.Store.LB.SetTLSCert(lbName, s.project, req.CertID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDetachCertFromLB(w http.ResponseWriter, r *http.Request) {
	if s.certMgr() == nil {
		writeError(w, http.StatusServiceUnavailable, "certificate manager not initialized")
		return
	}
	lbName := r.PathValue("lb")
	certID := r.PathValue("cert")
	hostname := r.URL.Query().Get("hostname")
	lbRec, err := s.ctrl.Store.LB.Get(lbName, s.project)
	if err != nil {
		writeNotFound(w, "lb not found")
		return
	}
	if err := s.ctrl.CertMgr.DetachFromLoadBalancer(r.Context(), certID, lbRec.ID, hostname); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.ctrl.Store.LB.ClearTLSCert(lbName, s.project); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
