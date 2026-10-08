package lb

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"capper/internal/types"
)

// proxyRunner is the common interface for TCP and HTTP proxies.
type proxyRunner interface {
	Start(ctx context.Context) error
	Stop()
}

// InstanceLister supplies the current instance list for selector-mode backend sync.
type InstanceLister func() ([]types.Instance, error)

// NodeHealthChecker returns true if the node hosting an instance is healthy.
type NodeHealthChecker func(nodeID string) bool

type runningEntry struct {
	runner proxyRunner
	spec   ProxySpec
}

// Manager manages LB records and coordinates running proxies.
type Manager struct {
	store       *Store
	mu          sync.Mutex
	running     map[string]runningEntry // keyed by proxy spec key (listener ID or legacy LB ID)
	certRes     CertResolver
	acmeHandler ACMEChallengeHandler
	logDir      string
	instList    InstanceLister
	nodeHealthy NodeHealthChecker
}

func NewManager(s *Store) *Manager {
	return &Manager{store: s, running: make(map[string]runningEntry)}
}

func (m *Manager) SetCertResolver(fn CertResolver)                 { m.certRes = fn }
func (m *Manager) SetACMEChallengeHandler(fn ACMEChallengeHandler) { m.acmeHandler = fn }
func (m *Manager) SetLogDir(dir string)                            { m.logDir = dir }
func (m *Manager) SetInstanceLister(fn InstanceLister)             { m.instList = fn }
func (m *Manager) SetNodeHealthChecker(fn NodeHealthChecker)       { m.nodeHealthy = fn }
func (m *Manager) Store() *Store                                   { return m.store }

type DNSAliasCreator func(zoneID, alias, target string) error

func (m *Manager) SetAlias(nameOrID, project, zoneID, alias, target string, createCNAME DNSAliasCreator) error {
	lb, err := m.store.Get(nameOrID, project)
	if err != nil {
		return fmt.Errorf("lb: alias: %w", err)
	}
	if err := m.store.SetServiceAlias(lb.ID, alias); err != nil {
		return fmt.Errorf("lb: alias: store: %w", err)
	}
	if createCNAME != nil && zoneID != "" && alias != "" && target != "" {
		if err := createCNAME(zoneID, alias, target); err != nil {
			return fmt.Errorf("lb: alias: dns: %w", err)
		}
	}
	return nil
}

// Create stores a legacy load balancer record (backward compatible).
func (m *Manager) Create(name, project, networkID, listenAddr string, mode LBMode) (LoadBalancer, error) {
	if name == "" {
		return LoadBalancer{}, fmt.Errorf("lb: name is required")
	}
	lb := LoadBalancer{
		ID:         newID(),
		Name:       name,
		Project:    project,
		NetworkID:  networkID,
		SubnetID:   networkID,
		Scheme:     SchemeInternal,
		Type:       TypeApplication,
		Mode:       mode,
		ListenAddr: listenAddr,
		Status:     StatusActive,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if err := m.store.Insert(lb); err != nil {
		return LoadBalancer{}, fmt.Errorf("lb: store: %w", err)
	}
	return lb, nil
}

// CreateExtended creates an LB with scheme, VIP, and optional first listener.
func (m *Manager) CreateExtended(opts CreateOptions) (LBDetail, error) {
	if opts.Name == "" {
		return LBDetail{}, fmt.Errorf("lb: name is required")
	}
	if opts.Scheme == "" {
		opts.Scheme = SchemeInternal
	}
	if opts.Type == "" {
		opts.Type = TypeApplication
	}
	if opts.Algorithm == "" {
		opts.Algorithm = AlgoRoundRobin
	}
	lb := LoadBalancer{
		ID:           newID(),
		Name:         opts.Name,
		Project:      opts.Project,
		VPCID:        opts.VPCID,
		SubnetID:     opts.SubnetID,
		NetworkID:    opts.SubnetID,
		Scheme:       opts.Scheme,
		Type:         opts.Type,
		VIPAddress:   opts.VIPAddress,
		RoutableIPID: opts.RoutableIPID,
		Status:       StatusActive,
		Algorithm:    opts.Algorithm,
		Selector:     opts.Selector,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := m.store.Insert(lb); err != nil {
		return LBDetail{}, fmt.Errorf("lb: store: %w", err)
	}

	detail := LBDetail{LoadBalancer: lb}

	if opts.TargetGroupName != "" || opts.ListenerPort > 0 {
		tgName := opts.TargetGroupName
		if tgName == "" {
			tgName = opts.Name + "-default"
		}
		tgPort := opts.TargetGroupPort
		if tgPort == 0 {
			tgPort = 80
		}
		proto := strings.ToLower(opts.ListenerProtocol)
		if proto == "" {
			proto = "tcp"
		}
		tg, err := m.store.CreateTargetGroup(opts.Project, tgName, opts.VPCID, lb.ID, proto, tgPort, "/")
		if err != nil {
			_ = m.store.Delete(lb.ID, opts.Project)
			return LBDetail{}, err
		}
		detail.TargetGroups = append(detail.TargetGroups, tg)

		if opts.InitialTargetAddr != "" {
			t, err := m.store.AddTarget(tg.ID, opts.InitialTargetAddr, 1)
			if err != nil {
				_ = m.store.Delete(lb.ID, opts.Project)
				return LBDetail{}, err
			}
			detail.Targets = append(detail.Targets, t)
		}

		lstPort := opts.ListenerPort
		if lstPort == 0 {
			lstPort = 80
		}
		lstProto := opts.ListenerProtocol
		if lstProto == "" {
			lstProto = "TCP"
		}
		lst, err := m.store.CreateListener(lb.ID, tg.ID, lstProto, lstPort, opts.ListenerCertID)
		if err != nil {
			_ = m.store.Delete(lb.ID, opts.Project)
			return LBDetail{}, err
		}
		detail.Listeners = append(detail.Listeners, lst)
	}

	return detail, nil
}

func (m *Manager) Get(nameOrID, project string) (LoadBalancer, error) {
	return m.store.Get(nameOrID, project)
}

func (m *Manager) GetDetail(nameOrID, project string) (LBDetail, error) {
	return m.store.GetDetail(nameOrID, project)
}

func (m *Manager) List(project string) ([]LoadBalancer, error) {
	return m.store.List(project)
}

func (m *Manager) Delete(nameOrID, project string) error {
	lb, err := m.store.Get(nameOrID, project)
	if err != nil {
		return err
	}
	listeners, _ := m.store.ListListeners(lb.ID)
	m.stopProxiesForLB(lb.ID, listeners)
	return m.store.Delete(nameOrID, project)
}

func (m *Manager) Publish(nameOrID, project, addr string) error {
	lb, err := m.store.Get(nameOrID, project)
	if err != nil {
		return err
	}
	if err := m.store.UpdateListenAddr(lb.ID, addr); err != nil {
		return err
	}
	m.restartLBProxies(lb.ID)
	return nil
}

func (m *Manager) SetMeta(nameOrID, project, selector, tlsCertName string, algo LBAlgorithm) error {
	lb, err := m.store.Get(nameOrID, project)
	if err != nil {
		return err
	}
	if algo == "" {
		algo = lb.Algorithm
	}
	if err := m.store.SetMeta(lb.ID, selector, tlsCertName, algo); err != nil {
		return err
	}
	m.restartLBProxies(lb.ID)
	return nil
}

func (m *Manager) SetTLSCert(nameOrID, project, certRef string) error {
	lb, err := m.store.Get(nameOrID, project)
	if err != nil {
		return err
	}
	if err := m.store.SetMeta(lb.ID, lb.Selector, certRef, lb.Algorithm); err != nil {
		return err
	}
	m.restartLBProxies(lb.ID)
	return nil
}

func (m *Manager) ClearTLSCert(nameOrID, project string) error {
	return m.SetTLSCert(nameOrID, project, "")
}

func (m *Manager) SetListenerCertificate(lbName, project, listenerID, certID string) error {
	lb, err := m.store.Get(lbName, project)
	if err != nil {
		return err
	}
	if err := m.store.SetListenerCertificate(listenerID, certID); err != nil {
		return err
	}
	// Stop then reconcile so HTTPS picks up the new cert immediately.
	m.stopProxy(listenerID)
	if err := m.Reconcile(context.Background()); err != nil {
		return fmt.Errorf("lb: certificate saved but proxy reconcile failed: %w", err)
	}
	_ = lb
	return nil
}

func (m *Manager) AddBackend(nameOrID, project, address string) (Backend, error) {
	lb, err := m.store.Get(nameOrID, project)
	if err != nil {
		return Backend{}, err
	}
	b, err := m.store.AddBackend(lb.ID, address)
	if err != nil {
		return Backend{}, fmt.Errorf("lb: add backend: %w", err)
	}
	return b, nil
}

func (m *Manager) RemoveBackend(nameOrID, project, address string) error {
	lb, err := m.store.Get(nameOrID, project)
	if err != nil {
		return err
	}
	return m.store.RemoveBackend(lb.ID, address)
}

func (m *Manager) ListBackends(nameOrID, project string) ([]Backend, error) {
	lb, err := m.store.Get(nameOrID, project)
	if err != nil {
		return nil, err
	}
	return m.store.ListBackends(lb.ID)
}

func (m *Manager) CreateTargetGroupForLB(lbName, project, name, protocol string, port int, healthPath string) (TargetGroup, error) {
	lb, err := m.store.Get(lbName, project)
	if err != nil {
		return TargetGroup{}, err
	}
	return m.store.CreateTargetGroup(project, name, lb.VPCID, lb.ID, protocol, port, healthPath)
}

func (m *Manager) DeleteTargetGroup(lbName, project, tgID string) error {
	lb, err := m.store.Get(lbName, project)
	if err != nil {
		return err
	}
	tg, err := m.store.GetTargetGroup(tgID)
	if err != nil {
		return err
	}
	if tg.LoadBalancerID != lb.ID {
		return fmt.Errorf("target group does not belong to this load balancer")
	}
	listeners, err := m.store.ListListeners(lb.ID)
	if err != nil {
		return err
	}
	for _, lst := range listeners {
		if lst.TargetGroupID == tgID {
			m.stopProxy(lst.ID)
			if err := m.store.DeleteListener(lst.ID); err != nil {
				return fmt.Errorf("lb: delete listener %s: %w", lst.ID, err)
			}
		}
	}
	return m.store.DeleteTargetGroup(tgID)
}

func (m *Manager) CreateListenerForLB(lbName, project, tgID, protocol string, port int, certID string) (Listener, error) {
	lb, err := m.store.Get(lbName, project)
	if err != nil {
		return Listener{}, err
	}
	return m.store.CreateListener(lb.ID, tgID, protocol, port, certID)
}

func (m *Manager) DeleteListener(lbName, project, listenerID string) error {
	if _, err := m.store.Get(lbName, project); err != nil {
		return err
	}
	m.stopProxy(listenerID)
	return m.store.DeleteListener(listenerID)
}

func (m *Manager) AddTarget(lbName, project, tgID, address string) (Target, error) {
	lb, err := m.store.Get(lbName, project)
	if err != nil {
		return Target{}, err
	}
	tg, err := m.store.GetTargetGroup(tgID)
	if err != nil {
		return Target{}, err
	}
	if tg.LoadBalancerID != lb.ID {
		return Target{}, fmt.Errorf("target group does not belong to this load balancer")
	}
	t, err := m.store.AddTarget(tgID, address, 1)
	if err != nil {
		return Target{}, err
	}
	m.restartListenersForTG(tgID)
	return t, nil
}

func (m *Manager) RemoveTarget(lbName, project, tgID, targetID string) error {
	if _, err := m.store.Get(lbName, project); err != nil {
		return err
	}
	if err := m.store.RemoveTarget(tgID, targetID); err != nil {
		return err
	}
	m.restartListenersForTG(tgID)
	return nil
}

func (m *Manager) Reconcile(ctx context.Context) error {
	specs, err := m.store.ListProxySpecs()
	if err != nil {
		return err
	}

	if m.instList != nil {
		instances, ierr := m.instList()
		if ierr == nil {
			for _, spec := range specs {
				if spec.LB.Selector != "" && spec.TargetGroupID == "" {
					m.syncSelectorBackends(spec.LB, instances)
				}
			}
		}
	}

	activeKeys := make(map[string]ProxySpec, len(specs))
	for _, spec := range specs {
		activeKeys[spec.Key] = spec
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, entry := range m.running {
		if _, ok := activeKeys[key]; !ok {
			entry.runner.Stop()
			delete(m.running, key)
		}
	}

	for key, spec := range activeKeys {
		if entry, ok := m.running[key]; ok {
			if proxySpecEqual(entry.spec, spec) {
				continue
			}
			entry.runner.Stop()
			delete(m.running, key)
		}
		logPath := ""
		if m.logDir != "" {
			logPath = m.logDir + "/" + spec.Key + ".log"
		}
		var p proxyRunner
		if spec.Mode == ModeHTTP || spec.Mode == ModeHTTPS {
			p = newHTTPProxy(spec, m.store, m.certRes, m.acmeHandler, logPath)
		} else {
			p = newProxy(spec, m.store, m.certRes, logPath)
		}
		m.running[key] = runningEntry{runner: p, spec: spec}
		go func(p proxyRunner) { _ = p.Start(ctx) }(p)
	}
	return nil
}

func proxySpecEqual(a, b ProxySpec) bool {
	return a.Key == b.Key &&
		a.ListenAddr == b.ListenAddr &&
		a.Mode == b.Mode &&
		a.TargetGroupID == b.TargetGroupID &&
		a.TLSCertName == b.TLSCertName &&
		a.LB.Algorithm == b.LB.Algorithm &&
		a.LB.Selector == b.LB.Selector &&
		a.LB.VIPAddress == b.LB.VIPAddress
}

func (m *Manager) syncSelectorBackends(lb LoadBalancer, instances []types.Instance) {
	k, v, ok := strings.Cut(lb.Selector, "=")
	if !ok {
		return
	}
	existing, _ := m.store.ListBackends(lb.ID)
	have := make(map[string]bool, len(existing))
	for _, b := range existing {
		have[b.Address] = true
	}
	for _, inst := range instances {
		if inst.Labels[k] != v || inst.NetworkIP == "" {
			continue
		}
		if m.nodeHealthy != nil && inst.NodeID != "" && !m.nodeHealthy(inst.NodeID) {
			continue
		}
		addr := inst.NetworkIP + ":80"
		if !have[addr] {
			_, _ = m.store.AddBackend(lb.ID, addr)
		}
	}
}

func (m *Manager) ListTargetGroups(project string) ([]TargetGroup, error) {
	return m.store.ListTargetGroups(project)
}

func (m *Manager) CreateTargetGroup(project, name, vpcID, protocol string, port int, healthPath string) (TargetGroup, error) {
	return m.store.CreateTargetGroup(project, name, vpcID, "", protocol, port, healthPath)
}

func (m *Manager) ListListeners(lbID string) ([]Listener, error) {
	return m.store.ListListeners(lbID)
}

func (m *Manager) CreateListener(lbID, tgID, protocol string, port int) (Listener, error) {
	return m.store.CreateListener(lbID, tgID, protocol, port, "")
}

type LBStat struct {
	LBID          string
	LBName        string
	ListenerID    string
	TotalRequests uint64
	ActiveConns   int64
}

func (m *Manager) RunningStats() []LBStat {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]LBStat, 0, len(m.running))
	for key, entry := range m.running {
		stat := LBStat{LBID: entry.spec.LB.ID, LBName: entry.spec.LB.Name, ListenerID: key}
		switch pr := entry.runner.(type) {
		case *Proxy:
			stat.TotalRequests = pr.TotalRequests.Load()
			stat.ActiveConns = pr.ActiveConns.Load()
		case *HTTPProxy:
			stat.TotalRequests = pr.TotalRequests.Load()
			stat.ActiveConns = pr.ActiveConns.Load()
		}
		out = append(out, stat)
	}
	return out
}

func (m *Manager) stopProxy(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.running[key]; ok {
		entry.runner.Stop()
		delete(m.running, key)
	}
}

func (m *Manager) stopProxiesForLB(lbID string, listeners []Listener) {
	m.stopProxy(lbID)
	for _, lst := range listeners {
		m.stopProxy(lst.ID)
	}
}

func (m *Manager) restartLBProxies(lbID string) {
	listeners, _ := m.store.ListListeners(lbID)
	m.stopProxiesForLB(lbID, listeners)
}

func (m *Manager) restartListenersForTG(tgID string) {
	ids, err := m.store.ListListenerIDsForTG(tgID)
	if err != nil {
		return
	}
	for _, id := range ids {
		m.stopProxy(id)
	}
}
