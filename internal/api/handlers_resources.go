package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"capper/internal/backup"
	"capper/internal/firewall"
	"capper/internal/lb"
	"capper/internal/manager"
	"capper/internal/stack"
)

// ============================================================
// Load Balancers
// ============================================================

func (s *Server) handleListLBs(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "lb:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	lbs, err := s.ctrl.Store.LB.List(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, lbs, nil)
}

func (s *Server) handleCreateLB(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "lb:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name              string         `json:"name"`
		Scheme            lb.LBScheme    `json:"scheme"`
		Type              lb.LBType      `json:"type"`
		VPCID             string         `json:"vpcId"`
		SubnetID          string         `json:"subnetId"`
		NetworkID         string         `json:"networkId,omitempty"`
		PoolID            string         `json:"poolId"`
		VIP               string         `json:"vip"`
		AutoVIP           bool           `json:"autoVip"`
		ListenAddr        string         `json:"listenAddr"`
		Mode              lb.LBMode      `json:"mode"`
		Algorithm         lb.LBAlgorithm `json:"algorithm,omitempty"`
		Selector          string         `json:"selector,omitempty"`
		TLSCertID         string         `json:"tlsCertId,omitempty"`
		ListenerProtocol  string         `json:"listenerProtocol"`
		ListenerPort      int            `json:"listenerPort"`
		ListenerCertID    string         `json:"listenerCertId"`
		TargetGroupName   string         `json:"targetGroupName"`
		TargetGroupPort   int            `json:"targetGroupPort"`
		InitialTargetAddr string         `json:"initialTargetAddr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	s.createLBFromRequest(w, r, req)
}

func (s *Server) handleGetLB(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "lb:inspect", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	detail, err := s.ctrl.Store.LB.GetDetail(name, s.project)
	if err != nil {
		writeNotFound(w, "lb not found")
		return
	}
	writeData(w, detail, nil)
}

func (s *Server) handleDeleteLB(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "lb:delete", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	lbRec, err := s.ctrl.Store.LB.Get(name, s.project)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	placer := s.lbVIPPlacer()
	_ = placer.ReleaseVIP(lbRec)
	if err := s.ctrl.Store.LB.Delete(name, s.project); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddLBBackend(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "lb:update", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	backend, err := s.ctrl.Store.LB.AddBackend(name, s.project, req.Address)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: backend})
}

func (s *Server) handleRemoveLBBackend(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	address := r.PathValue("address")
	if err := s.authorize(r, "lb:update", "lb/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.LB.RemoveBackend(name, s.project, address); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Firewalls
// ============================================================

type firewallView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Network    string `json:"network"`
	RulesCount int    `json:"rulesCount"`
	Status     string `json:"status"`
}

func toFirewallView(fw firewall.Firewall) firewallView {
	return firewallView{
		ID:     fw.NetworkID,
		Name:   fw.NetworkName,
		Status: fw.Status,
	}
}

func (s *Server) handleListFirewalls(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "firewall:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	fwMgr := firewall.NewManager(s.ctrl.Store.Firewalls)
	fws, err := fwMgr.List()
	if err != nil {
		writeInternal(w, err)
		return
	}
	views := make([]firewallView, len(fws))
	for i, fw := range fws {
		views[i] = toFirewallView(fw)
	}
	writeData(w, views, nil)
}

func (s *Server) handleCreateFirewall(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "firewall:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name    string `json:"name"`
		Network string `json:"network"`
		Mode    string `json:"mode,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if req.Name == "" {
		req.Name = req.Network
	}
	fwMgr := firewall.NewManager(s.ctrl.Store.Firewalls)
	fw, err := fwMgr.Init(req.Name, req.Name, req.Mode)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: toFirewallView(fw)})
}

func (s *Server) handleGetFirewall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "firewall:inspect", "firewall/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	fwMgr := firewall.NewManager(s.ctrl.Store.Firewalls)
	fw, rules, err := fwMgr.Inspect(name)
	if err != nil {
		writeNotFound(w, "firewall not found")
		return
	}
	writeData(w, map[string]any{"firewall": fw, "rules": rules}, nil)
}

func (s *Server) handleDeleteFirewall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "firewall:delete", "firewall/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	fwMgr := firewall.NewManager(s.ctrl.Store.Firewalls)
	if err := fwMgr.Delete(name); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleApplyFirewall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "firewall:apply", "firewall/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		DryRun bool `json:"dryRun,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	fwMgr := firewall.NewManager(s.ctrl.Store.Firewalls)
	fw, _, err := fwMgr.Inspect(name)
	if err != nil {
		writeNotFound(w, "firewall not found")
		return
	}
	result, err := fwMgr.Apply(fw, firewall.NetworkInfo{ID: fw.NetworkID}, nil, nil, req.DryRun)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, result, nil)
}

func (s *Server) handleListFirewallRules(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "firewall:inspect", "firewall/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	fwMgr := firewall.NewManager(s.ctrl.Store.Firewalls)
	_, rules, err := fwMgr.Inspect(name)
	if err != nil {
		writeNotFound(w, "firewall not found")
		return
	}
	writeData(w, rules, nil)
}

func (s *Server) handleCreateFirewallRule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "firewall:create", "firewall/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var spec firewall.RuleSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeBadRequest(w, err)
		return
	}
	fwMgr := firewall.NewManager(s.ctrl.Store.Firewalls)
	rule, err := fwMgr.AddRule(name, spec)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: rule})
}

func (s *Server) handleDeleteFirewallRule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ruleID := r.PathValue("id")
	if err := s.authorize(r, "firewall:create", "firewall/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	fwMgr := firewall.NewManager(s.ctrl.Store.Firewalls)
	if err := fwMgr.DeleteRule(name, ruleID); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Health checks
// ============================================================

func (s *Server) handleListHealthChecks(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "health:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	results, err := s.ctrl.Store.Health.List()
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, results, nil)
}

func (s *Server) handleGetHealthCheck(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("instanceId")
	if err := s.authorize(r, "health:inspect", "instance/"+instanceID); err != nil {
		writeForbidden(w, err)
		return
	}
	result, err := s.ctrl.Store.Health.Get(instanceID)
	if err != nil {
		writeNotFound(w, "health check not found")
		return
	}
	writeData(w, result, nil)
}

// ============================================================
// Stacks
// ============================================================

func (s *Server) handleListStacks(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "stack:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	stacks, err := s.ctrl.Store.Stack.List(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, stacks, nil)
}

func (s *Server) handleCreateStack(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "stack:apply", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var tmpl stack.StackTemplate
	if err := json.NewDecoder(r.Body).Decode(&tmpl); err != nil {
		writeBadRequest(w, err)
		return
	}
	if len(tmpl.Networks) > 0 {
		writeBadRequest(w, fmt.Errorf("stack networks[] is removed; use VPC subnets and set subnetId on instances"))
		return
	}
	for _, inst := range tmpl.Instances {
		if inst.Network != "" {
			writeBadRequest(w, fmt.Errorf("instance %q: network field is removed; use subnetId", inst.Name))
			return
		}
	}
	result, err := s.ctrl.Store.Stack.Apply(r.Context(), tmpl, s.project)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: result})
}

func (s *Server) handleGetStack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "stack:inspect", "stack/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	st, err := s.ctrl.Store.Stack.Get(name, s.project)
	if err != nil {
		writeNotFound(w, "stack not found")
		return
	}
	writeData(w, st, nil)
}

func (s *Server) handleDeleteStack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "stack:destroy", "stack/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.Stack.Destroy(r.Context(), name, s.project); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStackDiff(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "stack:inspect", "stack/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	ops, err := s.ctrl.Store.Stack.Diff(name, s.project)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, ops, nil)
}

// ============================================================
// Backups
// ============================================================

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "backup:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	records, err := s.ctrl.Store.Backup.ListRecords(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, records, nil)
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "backup:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		DestDir string `json:"destDir,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	rec, err := s.ctrl.Store.Backup.BackupStore(s.project, req.DestDir)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: rec})
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.authorize(r, "backup:restore", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.Backup.Restore(id, s.project); err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, map[string]string{"status": "restored"}, nil)
}

func (s *Server) handleListBackupPolicies(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "backup:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	policies, err := s.ctrl.Store.Backup.ListPolicies(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, policies, nil)
}

func (s *Server) handleCreateBackupPolicy(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "backup:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name         string `json:"name"`
		TargetPath   string `json:"targetPath,omitempty"`
		IntervalSecs int    `json:"intervalSecs,omitempty"`
		Retention    int    `json:"retention,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	policy, err := s.ctrl.Store.Backup.CreatePolicy(req.Name, s.project, req.TargetPath, backup.BackupTypeStore, req.IntervalSecs, req.Retention)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: policy})
}

func (s *Server) handleDeleteBackupPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "backup:delete", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.Backup.DeletePolicy(name, s.project); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Databases
// ============================================================

func (s *Server) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "database:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	dbs, err := s.ctrl.Store.Databases.List(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, dbs, nil)
}

func (s *Server) handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "database:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name      string `json:"name"`
		Engine    string `json:"engine"`
		Version   string `json:"version,omitempty"`
		NetworkID string `json:"networkId,omitempty"`
		Port      int    `json:"port,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	db, err := manager.CreateManagedDatabase(
		s.ctrl.Store, s.ctrl.Instances, s.ctrl.Store.Metadata,
		req.Name, s.project, req.Engine, req.Version, req.NetworkID, req.Port,
	)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "database", db.ID, "database.created", map[string]any{"engine": db.Engine, "instanceId": db.InstanceID})
	writeJSON(w, http.StatusCreated, Envelope{Data: db})
}

func (s *Server) handleGetDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "database:inspect", "database/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	db, err := s.ctrl.Store.Databases.Get(name, s.project)
	if err != nil {
		writeNotFound(w, "database not found")
		return
	}
	writeData(w, db, nil)
}

func (s *Server) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "database:delete", "database/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	db, err := manager.DeleteManagedDatabase(s.ctrl.Store, s.ctrl.Instances, name, s.project)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	s.recordEvent(r, "database", db.ID, "database.deleted", nil)
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// AI
// ============================================================

func (s *Server) handleListAIAgents(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "ai:agent:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	agents, err := s.ctrl.Store.AI.ListAgents(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, agents, nil)
}

func (s *Server) handleCreateAIAgent(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "ai:agent:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name         string `json:"name"`
		Model        string `json:"model,omitempty"`
		Owner        string `json:"owner,omitempty"`
		RoleTemplate string `json:"roleTemplate,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	agent, err := s.ctrl.Store.AI.RegisterAgent(req.Name, s.project, req.Model, req.Owner, req.RoleTemplate)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: agent})
}

func (s *Server) handleListAISessions(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "ai:session:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	sessions, err := s.ctrl.Store.AI.ListSessions(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, sessions, nil)
}

func (s *Server) handleCreateAISession(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "ai:session:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		AgentID   string `json:"agentId"`
		Principal string `json:"principal,omitempty"`
		Model     string `json:"model,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	_, pid := principalFromContext(r.Context())
	if req.Principal == "" {
		req.Principal = pid
	}
	session, err := s.ctrl.Store.AI.StartSession(req.AgentID, s.project, req.Principal, req.Model)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: session})
}

func (s *Server) handleListMCP(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "ai:mcp:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	servers, err := s.ctrl.Store.AI.ListMCP(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, servers, nil)
}

func (s *Server) handleCreateMCP(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "ai:mcp:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name      string `json:"name"`
		Endpoint  string `json:"endpoint"`
		IAMAction string `json:"iamAction,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	srv, err := s.ctrl.Store.AI.RegisterMCP(req.Name, s.project, req.Endpoint, req.IAMAction)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: srv})
}

// ============================================================
// Posture
// ============================================================

func (s *Server) handleListPostureFindings(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "posture:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	findings, err := s.ctrl.Store.Posture.List(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, findings, nil)
}

func (s *Server) handlePostureScan(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "posture:scan", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		RootDir string `json:"rootDir,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	result, err := s.ctrl.Store.Posture.Scan(s.project, req.RootDir)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, result, nil)
}

// ============================================================
// Certs
// ============================================================

func (s *Server) handleListCerts(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "cert:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	certs, err := s.ctrl.Store.Certs.List(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, certs, nil)
}

func (s *Server) handleCreateCert(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "cert:issue", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name       string   `json:"name"`
		CommonName string   `json:"commonName,omitempty"`
		DNSNames   []string `json:"dnsNames,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	result, err := s.ctrl.Store.Certs.Issue(req.Name, s.project, req.CommonName, req.DNSNames)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: result})
}

func (s *Server) handleDeleteCert(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "cert:revoke", "cert/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.Certs.Revoke(name, s.project); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Quotas / Billing
// ============================================================

func (s *Server) handleListQuotas(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "quota:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	quotas, err := s.ctrl.Store.Billing.ListQuotas(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, quotas, nil)
}

func (s *Server) handleSetQuota(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "quota:set", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Resource string `json:"resource"`
		Limit    int64  `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	if err := s.ctrl.Store.Billing.SetQuota(s.project, req.Resource, req.Limit); err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, map[string]string{"status": "ok"}, nil)
}

// ============================================================
// Ingress
// ============================================================

func (s *Server) handleListIngress(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "ingress:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	rules, err := s.ctrl.Store.Ingress.List(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, rules, nil)
}

func (s *Server) handleCreateIngress(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "ingress:create", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Name       string `json:"name"`
		Host       string `json:"host,omitempty"`
		PathPrefix string `json:"pathPrefix,omitempty"`
		BackendLB  string `json:"backendLb,omitempty"`
		TLSCert    string `json:"tlsCert,omitempty"`
		RateLimit  int    `json:"rateLimit,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	rule, err := s.ctrl.Store.Ingress.Create(req.Name, s.project, req.Host, req.PathPrefix, req.BackendLB, req.TLSCert, req.RateLimit)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: rule})
}

func (s *Server) handleDeleteIngress(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "ingress:delete", "ingress/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.Ingress.Delete(name, s.project); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Queues
// ============================================================

func (s *Server) handleListQueues(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "queue:list", "project:"+s.project); err != nil {
		writeForbidden(w, err)
		return
	}
	queues, err := s.ctrl.Store.Queue.List(s.project)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeData(w, queues, nil)
}

func (s *Server) handleCreateQueue(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r, "queue:create", "project:"+s.project); err != nil {
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
	q, err := s.ctrl.Store.Queue.Create(req.Name, s.project)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: q})
}

func (s *Server) handleDeleteQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "queue:delete", "queue/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	if err := s.ctrl.Store.Queue.Delete(name, s.project); err != nil {
		writeBadRequest(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePublishQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "queue:publish", "queue/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, err)
		return
	}
	msg, err := s.ctrl.Store.Queue.Publish(name, s.project, req.Body)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Envelope{Data: msg})
}

func (s *Server) handleConsumeQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.authorize(r, "queue:consume", "queue/"+name); err != nil {
		writeForbidden(w, err)
		return
	}
	var req struct {
		Max int `json:"max,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	msgs, err := s.ctrl.Store.Queue.Consume(name, s.project, req.Max)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	writeData(w, msgs, nil)
}
