package lb

import (
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Store persists LB configuration in SQLite.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// InitSchema creates the lb tables if they do not exist.
func InitSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS lb_load_balancers (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			project      TEXT NOT NULL DEFAULT 'default',
			network_id   TEXT NOT NULL DEFAULT '',
			mode         TEXT NOT NULL DEFAULT 'tcp',
			listen_addr  TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT 'active',
			created_at   TEXT NOT NULL,
			algorithm    TEXT NOT NULL DEFAULT '',
			selector     TEXT NOT NULL DEFAULT '',
			tls_cert_name TEXT NOT NULL DEFAULT '',
			UNIQUE(name, project)
		)`,
		`CREATE TABLE IF NOT EXISTS lb_backends (
			id      TEXT PRIMARY KEY,
			lb_id   TEXT NOT NULL REFERENCES lb_load_balancers(id) ON DELETE CASCADE,
			address TEXT NOT NULL,
			UNIQUE(lb_id, address)
		)`,
		`CREATE TABLE IF NOT EXISTS lb_target_groups (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			project     TEXT NOT NULL DEFAULT 'default',
			vpc_id      TEXT NOT NULL DEFAULT '',
			protocol    TEXT NOT NULL DEFAULT 'tcp',
			port        INTEGER NOT NULL DEFAULT 80,
			health_path TEXT NOT NULL DEFAULT '/',
			created_at  TEXT NOT NULL,
			UNIQUE(name, project)
		)`,
		`CREATE TABLE IF NOT EXISTS lb_listeners (
			id               TEXT PRIMARY KEY,
			load_balancer_id TEXT NOT NULL,
			target_group_id  TEXT NOT NULL,
			protocol         TEXT NOT NULL DEFAULT 'tcp',
			port             INTEGER NOT NULL DEFAULT 80,
			created_at       TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS lb_target_group_targets (
			id              TEXT PRIMARY KEY,
			target_group_id TEXT NOT NULL,
			address         TEXT NOT NULL,
			weight          INTEGER NOT NULL DEFAULT 1,
			UNIQUE(target_group_id, address)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	for _, alter := range []string{
		`ALTER TABLE lb_load_balancers ADD COLUMN algorithm TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN selector TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN tls_cert_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN service_alias TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN scheme TEXT NOT NULL DEFAULT 'internal'`,
		`ALTER TABLE lb_load_balancers ADD COLUMN type TEXT NOT NULL DEFAULT 'application'`,
		`ALTER TABLE lb_load_balancers ADD COLUMN vpc_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN subnet_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN vip_address TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN routable_ip_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN dns_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN eni_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_target_groups ADD COLUMN load_balancer_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_listeners ADD COLUMN certificate_id TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(alter); err != nil {
			if !isDupeCol(err) {
				return err
			}
		}
	}
	return migrateLegacyLBs(db)
}

func isDupeCol(err error) bool {
	s := err.Error()
	return strings.Contains(s, "duplicate column") || strings.Contains(s, "already exists")
}

func (s *Store) Insert(lb LoadBalancer) error {
	subnetID := lb.SubnetID
	if subnetID == "" {
		subnetID = lb.NetworkID
	}
	_, err := s.db.Exec(
		`INSERT INTO lb_load_balancers
		 (id, name, project, network_id, subnet_id, vpc_id, scheme, type, vip_address, routable_ip_id,
		  eni_id, dns_name, mode, listen_addr, status, algorithm, selector, tls_cert_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lb.ID, lb.Name, lb.Project, subnetID, subnetID, lb.VPCID, string(lb.Scheme), string(lb.Type),
		lb.VIPAddress, lb.RoutableIPID, lb.ENIID, lb.DNSName, lb.Mode, lb.ListenAddr, lb.Status,
		string(lb.Algorithm), lb.Selector, lb.TLSCertName, lb.CreatedAt,
	)
	return err
}

const lbCols = `id, name, project, network_id, subnet_id, vpc_id, scheme, type, vip_address,
	routable_ip_id, eni_id, dns_name, mode, listen_addr, status, algorithm, selector, tls_cert_name,
	service_alias, created_at`

const (
	selectLBByIDOrName = `SELECT id, name, project, network_id, subnet_id, vpc_id, scheme, type, vip_address, routable_ip_id, eni_id, dns_name, mode, listen_addr, status, algorithm, selector, tls_cert_name, service_alias, created_at FROM lb_load_balancers WHERE (id=? OR name=?) LIMIT 1`
	selectLBByIDOrNameProject = `SELECT id, name, project, network_id, subnet_id, vpc_id, scheme, type, vip_address, routable_ip_id, eni_id, dns_name, mode, listen_addr, status, algorithm, selector, tls_cert_name, service_alias, created_at FROM lb_load_balancers WHERE (id=? OR name=?) AND project=? LIMIT 1`
	selectAllLBs = `SELECT id, name, project, network_id, subnet_id, vpc_id, scheme, type, vip_address, routable_ip_id, eni_id, dns_name, mode, listen_addr, status, algorithm, selector, tls_cert_name, service_alias, created_at FROM lb_load_balancers ORDER BY name`
	selectLBsByProject = `SELECT id, name, project, network_id, subnet_id, vpc_id, scheme, type, vip_address, routable_ip_id, eni_id, dns_name, mode, listen_addr, status, algorithm, selector, tls_cert_name, service_alias, created_at FROM lb_load_balancers WHERE project=? ORDER BY name`
)


func (s *Store) Get(nameOrID, project string) (LoadBalancer, error) {
	var row *sql.Row
	if project == "" {
		row = s.db.QueryRow(selectLBByIDOrName, nameOrID, nameOrID)
	} else {
		row = s.db.QueryRow(selectLBByIDOrNameProject, nameOrID, nameOrID, project)
	}
	return scanLB(row)
}

func (s *Store) List(project string) ([]LoadBalancer, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if project == "" {
		rows, err = s.db.Query(selectAllLBs)
	} else {
		rows, err = s.db.Query(selectLBsByProject, project)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LoadBalancer
	for rows.Next() {
		lb, err := scanLB(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lb)
	}
	return out, rows.Err()
}

func (s *Store) ListActive() ([]LoadBalancer, error) {
	rows, err := s.db.Query(
		`SELECT `+lbCols+` FROM lb_load_balancers WHERE status='active'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LoadBalancer
	for rows.Next() {
		lb, err := scanLB(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lb)
	}
	return out, rows.Err()
}

func (s *Store) UpdateStatus(id string, status LBStatus) error {
	_, err := s.db.Exec(`UPDATE lb_load_balancers SET status=? WHERE id=?`, status, id)
	return err
}

func (s *Store) UpdateListenAddr(id, addr string) error {
	_, err := s.db.Exec(`UPDATE lb_load_balancers SET listen_addr=? WHERE id=?`, addr, id)
	return err
}

func (s *Store) SetMeta(id string, selector, tlsCertName string, algo LBAlgorithm) error {
	_, err := s.db.Exec(
		`UPDATE lb_load_balancers SET selector=?, tls_cert_name=?, algorithm=? WHERE id=?`,
		selector, tlsCertName, string(algo), id,
	)
	return err
}

func (s *Store) SetServiceAlias(id, alias string) error {
	_, err := s.db.Exec(`UPDATE lb_load_balancers SET service_alias=? WHERE id=?`, alias, id)
	return err
}

func (s *Store) SetVIP(id, vip, routableIPID string) error {
	_, err := s.db.Exec(
		`UPDATE lb_load_balancers SET vip_address=?, routable_ip_id=? WHERE id=?`,
		vip, routableIPID, id,
	)
	return err
}

func (s *Store) Delete(nameOrID, project string) error {
	lb, gerr := s.Get(nameOrID, project)
	var res sql.Result
	var err error
	if project == "" {
		res, err = s.db.Exec(`DELETE FROM lb_load_balancers WHERE id=? OR name=?`, nameOrID, nameOrID)
	} else {
		res, err = s.db.Exec(`DELETE FROM lb_load_balancers WHERE (id=? OR name=?) AND project=?`,
			nameOrID, nameOrID, project)
	}
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("lb %q not found", nameOrID)
	}
	if gerr == nil {
		_, _ = s.db.Exec(`DELETE FROM lb_backends WHERE lb_id=?`, lb.ID)
		_, _ = s.db.Exec(`DELETE FROM lb_target_group_targets WHERE target_group_id IN (SELECT id FROM lb_target_groups WHERE load_balancer_id=?)`, lb.ID)
		_, _ = s.db.Exec(`DELETE FROM lb_listeners WHERE load_balancer_id=?`, lb.ID)
		_, _ = s.db.Exec(`DELETE FROM lb_target_groups WHERE load_balancer_id=?`, lb.ID)
	}
	return nil
}

func (s *Store) AddBackend(lbID, address string) (Backend, error) {
	id := newID()
	_, err := s.db.Exec(
		`INSERT INTO lb_backends (id, lb_id, address) VALUES (?, ?, ?)
		 ON CONFLICT(lb_id, address) DO NOTHING`,
		id, lbID, address,
	)
	if err != nil {
		return Backend{}, err
	}
	return Backend{ID: id, LBID: lbID, Address: address, Healthy: true}, nil
}

func (s *Store) RemoveBackend(lbID, address string) error {
	res, err := s.db.Exec(`DELETE FROM lb_backends WHERE lb_id=? AND address=?`, lbID, address)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("backend %q not found on lb %q", address, lbID)
	}
	return nil
}

func (s *Store) ListBackends(lbID string) ([]Backend, error) {
	rows, err := s.db.Query(
		`SELECT id, lb_id, address FROM lb_backends WHERE lb_id=? ORDER BY address`,
		lbID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Backend
	for rows.Next() {
		var b Backend
		b.Healthy = true
		if err := rows.Scan(&b.ID, &b.LBID, &b.Address); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListProxySpecs returns proxy configurations for all active listeners.
func (s *Store) ListProxySpecs() ([]ProxySpec, error) {
	active, err := s.ListActive()
	if err != nil {
		return nil, err
	}
	var specs []ProxySpec
	for _, lb := range active {
		listeners, err := s.ListListeners(lb.ID)
		if err != nil {
			return nil, err
		}
		if len(listeners) == 0 {
			if lb.ListenAddr == "" {
				continue
			}
			specs = append(specs, legacyProxySpec(lb))
			continue
		}
		for _, lst := range listeners {
			spec, err := s.listenerProxySpec(lb, lst)
			if err != nil {
				return nil, err
			}
			if spec.ListenAddr == "" {
				continue
			}
			if spec.Mode == ModeHTTPS && spec.TLSCertName == "" {
				continue // HTTPS requires cert before proxy starts
			}
			specs = append(specs, spec)
		}
	}
	return specs, nil
}

func legacyProxySpec(lb LoadBalancer) ProxySpec {
	return ProxySpec{
		Key:           lb.ID,
		LB:            lb,
		ListenAddr:    lb.ListenAddr,
		Mode:          lb.Mode,
		TargetGroupID: "",
		TLSCertName:   lb.TLSCertName,
	}
}

func (s *Store) listenerProxySpec(lb LoadBalancer, lst Listener) (ProxySpec, error) {
	mode, err := listenerMode(lst.Protocol)
	if err != nil {
		return ProxySpec{}, err
	}
	addr := formatListenAddr(lb.VIPAddress, lst.Port, lb.ListenAddr)
	return ProxySpec{
		Key:           lst.ID,
		LB:            lb,
		Listener:      lst,
		ListenAddr:    addr,
		Mode:          mode,
		TargetGroupID: lst.TargetGroupID,
		TLSCertName:   lst.CertificateID,
	}, nil
}

func listenerMode(proto string) (LBMode, error) {
	switch strings.ToUpper(proto) {
	case string(ProtoHTTP):
		return ModeHTTP, nil
	case string(ProtoHTTPS):
		return ModeHTTPS, nil
	case string(ProtoTCP):
		return ModeTCP, nil
	default:
		return ModeTCP, fmt.Errorf("unknown listener protocol %q", proto)
	}
}

func formatListenAddr(vip string, port int, fallback string) string {
	if vip != "" {
		if strings.Contains(vip, ":") {
			return net.JoinHostPort(vip, strconv.Itoa(port))
		}
		return fmt.Sprintf("%s:%d", vip, port)
	}
	if fallback != "" {
		return fallback
	}
	return fmt.Sprintf("0.0.0.0:%d", port)
}

func (s *Store) GetDetail(nameOrID, project string) (LBDetail, error) {
	lb, err := s.Get(nameOrID, project)
	if err != nil {
		return LBDetail{}, err
	}
	listeners, _ := s.ListListeners(lb.ID)
	tgs, _ := s.ListTargetGroupsForLB(lb.ID)
	var targets []Target
	for _, tg := range tgs {
		tgTargets, _ := s.ListTargets(tg.ID)
		targets = append(targets, tgTargets...)
	}
	backends, _ := s.ListBackends(lb.ID)
	return LBDetail{
		LoadBalancer: lb,
		Listeners:    listeners,
		TargetGroups: tgs,
		Targets:      targets,
		Backends:     backends,
	}, nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scanLB(s rowScanner) (LoadBalancer, error) {
	var lb LoadBalancer
	var algo, selector, tlsCert, serviceAlias, scheme, lbType, subnetID string
	if err := s.Scan(
		&lb.ID, &lb.Name, &lb.Project, &lb.NetworkID, &subnetID, &lb.VPCID,
		&scheme, &lbType, &lb.VIPAddress, &lb.RoutableIPID, &lb.ENIID, &lb.DNSName,
		&lb.Mode, &lb.ListenAddr, &lb.Status, &algo, &selector, &tlsCert, &serviceAlias, &lb.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return LoadBalancer{}, fmt.Errorf("lb not found")
		}
		return LoadBalancer{}, fmt.Errorf("lb: scan: %w", err)
	}
	lb.SubnetID = subnetID
	if lb.NetworkID == "" {
		lb.NetworkID = subnetID
	}
	lb.Scheme = LBScheme(scheme)
	lb.Type = LBType(lbType)
	lb.Algorithm = LBAlgorithm(algo)
	lb.Selector = selector
	lb.TLSCertName = tlsCert
	lb.ServiceAlias = serviceAlias
	return lb, nil
}

func newID() string {
	return "lb_" + fmt.Sprintf("%d", time.Now().UnixNano())
}

func newTGID() string {
	return "tg_" + fmt.Sprintf("%d", time.Now().UnixNano())
}

func newLstID() string {
	return "lst_" + fmt.Sprintf("%d", time.Now().UnixNano())
}

func newTgtID() string {
	return "tgt_" + fmt.Sprintf("%d", time.Now().UnixNano())
}
