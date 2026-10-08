package ipam

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store persists IP pools, addresses, and bindings.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store over an open database.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// InitSchema creates the IPAM tables. Safe to call repeatedly.
func (s *Store) InitSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS routable_ip_pools (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			cidr TEXT NOT NULL,
			ip_version INTEGER NOT NULL DEFAULT 4,
			scope TEXT NOT NULL DEFAULT 'global',
			realm_id TEXT NOT NULL DEFAULT '',
			region_id TEXT NOT NULL DEFAULT '',
			zone_id TEXT NOT NULL DEFAULT '',
			node_id TEXT NOT NULL DEFAULT '',
			gateway TEXT NOT NULL DEFAULT '',
			vlan_id INTEGER NOT NULL DEFAULT 0,
			interface_name TEXT NOT NULL DEFAULT '',
			usage_json TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'active',
			allow_auto_allocate INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS routable_ips (
			id TEXT PRIMARY KEY,
			pool_id TEXT NOT NULL,
			address TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'available',
			project TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			purpose TEXT NOT NULL DEFAULT '',
			allocation_type TEXT NOT NULL DEFAULT 'auto',
			target_type TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			dns_name TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(address)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_routable_ips_proj_name
			ON routable_ips(project, name) WHERE name != ''`,
		`CREATE TABLE IF NOT EXISTS routable_ip_bindings (
			id TEXT PRIMARY KEY,
			ip_id TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			binding_mode TEXT NOT NULL,
			protocol TEXT NOT NULL DEFAULT '',
			external_port INTEGER NOT NULL DEFAULT 0,
			internal_ip TEXT NOT NULL DEFAULT '',
			internal_port INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(ip_id, target_type, target_id, binding_mode, external_port, protocol)
		)`,
		`CREATE TABLE IF NOT EXISTS ipam_exclusions (
			id TEXT PRIMARY KEY,
			address TEXT NOT NULL,
			pool_id TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(address, pool_id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("ipam: schema: %w", err)
		}
	}
	return nil
}

func nowTS() string { return time.Now().UTC().Format(time.RFC3339) }

// ---- pools -----------------------------------------------------------------

// InsertPool stores a pool record.
func (s *Store) InsertPool(p RoutableIPPool) (RoutableIPPool, error) {
	if p.ID == "" {
		p.ID = "ippool_" + uuid.NewString()
	}
	ts := nowTS()
	p.CreatedAt, p.UpdatedAt = ts, ts
	if p.IPVersion == 0 {
		p.IPVersion = 4
	}
	if p.Scope == "" {
		p.Scope = "global"
	}
	if p.Status == "" {
		p.Status = PoolActive
	}
	usage, _ := json.Marshal(p.Usage)
	auto := 0
	if p.AllowAutoAllocate {
		auto = 1
	}
	_, err := s.db.Exec(`INSERT INTO routable_ip_pools
		(id, name, cidr, ip_version, scope, realm_id, region_id, zone_id, node_id, gateway, vlan_id,
		 interface_name, usage_json, status, allow_auto_allocate, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.CIDR, p.IPVersion, p.Scope, p.RealmID, p.RegionID, p.ZoneID, p.NodeID,
		p.Gateway, p.VLANID, p.InterfaceName, string(usage), p.Status, auto, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return RoutableIPPool{}, err
	}
	return p, nil
}

func scanPool(row interface{ Scan(...any) error }) (RoutableIPPool, error) {
	var p RoutableIPPool
	var usage string
	var auto int
	if err := row.Scan(&p.ID, &p.Name, &p.CIDR, &p.IPVersion, &p.Scope, &p.RealmID, &p.RegionID,
		&p.ZoneID, &p.NodeID, &p.Gateway, &p.VLANID, &p.InterfaceName, &usage, &p.Status, &auto,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		return RoutableIPPool{}, err
	}
	_ = json.Unmarshal([]byte(usage), &p.Usage)
	p.AllowAutoAllocate = auto == 1
	return p, nil
}

const poolCols = `id, name, cidr, ip_version, scope, realm_id, region_id, zone_id, node_id, gateway,
	vlan_id, interface_name, usage_json, status, allow_auto_allocate, created_at, updated_at`

// GetPool returns a pool by ID or name.
func (s *Store) GetPool(idOrName string) (RoutableIPPool, error) {
	return scanPool(s.db.QueryRow(`SELECT `+poolCols+` FROM routable_ip_pools WHERE id=? OR name=?`,
		idOrName, idOrName))
}

// ListPools returns all pools.
func (s *Store) ListPools() ([]RoutableIPPool, error) {
	rows, err := s.db.Query(`SELECT ` + poolCols + ` FROM routable_ip_pools ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoutableIPPool
	for rows.Next() {
		p, err := scanPool(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetPoolStatus updates a pool's status.
func (s *Store) SetPoolStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE routable_ip_pools SET status=?, updated_at=? WHERE id=?`, status, nowTS(), id)
	return err
}

// DeletePool removes a pool and its addresses (only if no IP is attached).
func (s *Store) DeletePool(id string) error {
	var attached int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM routable_ips WHERE pool_id=? AND status IN ('attached','reserved','allocated')`, id).Scan(&attached)
	if attached > 0 {
		return fmt.Errorf("ipam: pool has %d in-use addresses; release them first", attached)
	}
	// Cascade: drop bindings on this pool's addresses, the addresses, and any
	// pool-scoped exclusions so nothing is left referencing the removed pool.
	if _, err := s.db.Exec(`DELETE FROM routable_ip_bindings WHERE ip_id IN
		(SELECT id FROM routable_ips WHERE pool_id=?)`, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM routable_ips WHERE pool_id=?`, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM ipam_exclusions WHERE pool_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM routable_ip_pools WHERE id=?`, id)
	return err
}

// ---- addresses -------------------------------------------------------------

// InsertIP stores an address row (used when materializing a pool).
func (s *Store) InsertIP(ip RoutableIP) error {
	if ip.ID == "" {
		ip.ID = "ip_" + uuid.NewString()
	}
	ts := nowTS()
	if ip.CreatedAt == "" {
		ip.CreatedAt = ts
	}
	ip.UpdatedAt = ts
	if ip.Status == "" {
		ip.Status = IPAvailable
	}
	if ip.AllocationType == "" {
		ip.AllocationType = "auto"
	}
	_, err := s.db.Exec(`INSERT INTO routable_ips
		(id, pool_id, address, status, project, name, purpose, allocation_type, target_type, target_id,
		 dns_name, description, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ip.ID, ip.PoolID, ip.Address, ip.Status, ip.Project, ip.Name, ip.Purpose, ip.AllocationType,
		ip.TargetType, ip.TargetID, ip.DNSName, ip.Description, ip.CreatedAt, ip.UpdatedAt)
	return err
}

func scanIP(row interface{ Scan(...any) error }) (RoutableIP, error) {
	var ip RoutableIP
	err := row.Scan(&ip.ID, &ip.PoolID, &ip.Address, &ip.Status, &ip.Project, &ip.Name, &ip.Purpose,
		&ip.AllocationType, &ip.TargetType, &ip.TargetID, &ip.DNSName, &ip.Description, &ip.CreatedAt, &ip.UpdatedAt)
	return ip, err
}

const ipCols = `id, pool_id, address, status, project, name, purpose, allocation_type, target_type,
	target_id, dns_name, description, created_at, updated_at`

// GetIP returns an address by ID.
func (s *Store) GetIP(id string) (RoutableIP, error) {
	return scanIP(s.db.QueryRow(`SELECT `+ipCols+` FROM routable_ips WHERE id=?`, id))
}

// GetIPByAddress returns the materialized row for an address (addresses are
// globally unique), or sql.ErrNoRows if it has not been materialized.
func (s *Store) GetIPByAddress(address string) (RoutableIP, error) {
	return scanIP(s.db.QueryRow(`SELECT `+ipCols+` FROM routable_ips WHERE address=?`, address))
}

// GetIPByName returns a named reserved address within a project.
func (s *Store) GetIPByName(project, name string) (RoutableIP, error) {
	return scanIP(s.db.QueryRow(`SELECT `+ipCols+` FROM routable_ips WHERE project=? AND name=?`, project, name))
}

// FirstAvailable returns the first available address in a pool, or sql.ErrNoRows.
func (s *Store) FirstAvailable(poolID string) (RoutableIP, error) {
	return scanIP(s.db.QueryRow(`SELECT `+ipCols+` FROM routable_ips
		WHERE pool_id=? AND status=? ORDER BY address LIMIT 1`, poolID, IPAvailable))
}

// GetAvailableAddress returns the row for a specific available address in a pool.
func (s *Store) GetAvailableAddress(poolID, address string) (RoutableIP, error) {
	return scanIP(s.db.QueryRow(`SELECT `+ipCols+` FROM routable_ips
		WHERE pool_id=? AND address=? AND status=?`, poolID, address, IPAvailable))
}

// ListIPs returns addresses, optionally filtered by pool and/or status.
func (s *Store) ListIPs(poolID, status string) ([]RoutableIP, error) {
	q := `SELECT ` + ipCols + ` FROM routable_ips WHERE 1=1`
	var args []any
	if poolID != "" {
		q += " AND pool_id=?"
		args = append(args, poolID)
	}
	if status != "" {
		q += " AND status=?"
		args = append(args, status)
	}
	q += " ORDER BY address"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoutableIP
	for rows.Next() {
		ip, err := scanIP(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	return out, rows.Err()
}

// UpdateIP applies status/ownership changes to an address.
func (s *Store) UpdateIP(ip RoutableIP) error {
	_, err := s.db.Exec(`UPDATE routable_ips SET status=?, project=?, name=?, purpose=?,
		allocation_type=?, target_type=?, target_id=?, dns_name=?, description=?, updated_at=? WHERE id=?`,
		ip.Status, ip.Project, ip.Name, ip.Purpose, ip.AllocationType, ip.TargetType, ip.TargetID,
		ip.DNSName, ip.Description, nowTS(), ip.ID)
	return err
}

// ---- bindings --------------------------------------------------------------

// InsertBinding stores a binding.
func (s *Store) InsertBinding(b IPBinding) (IPBinding, error) {
	if b.ID == "" {
		b.ID = "ipbind_" + uuid.NewString()
	}
	ts := nowTS()
	b.CreatedAt, b.UpdatedAt = ts, ts
	if b.Status == "" {
		b.Status = "active"
	}
	_, err := s.db.Exec(`INSERT INTO routable_ip_bindings
		(id, ip_id, target_type, target_id, binding_mode, protocol, external_port, internal_ip,
		 internal_port, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		b.ID, b.IPID, b.TargetType, b.TargetID, b.BindingMode, b.Protocol, b.ExternalPort,
		b.InternalIP, b.InternalPort, b.Status, b.CreatedAt, b.UpdatedAt)
	if err != nil {
		return IPBinding{}, err
	}
	return b, nil
}

// ListBindings returns bindings for an address.
func (s *Store) ListBindings(ipID string) ([]IPBinding, error) {
	rows, err := s.db.Query(`SELECT `+bindingCols+`
		FROM routable_ip_bindings WHERE ip_id=?`, ipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IPBinding
	for rows.Next() {
		b, err := scanBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

const bindingCols = `id, ip_id, target_type, target_id, binding_mode, protocol,
	external_port, internal_ip, internal_port, status, created_at, updated_at`

func scanBinding(row interface{ Scan(...any) error }) (IPBinding, error) {
	var b IPBinding
	err := row.Scan(&b.ID, &b.IPID, &b.TargetType, &b.TargetID, &b.BindingMode, &b.Protocol,
		&b.ExternalPort, &b.InternalIP, &b.InternalPort, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}

// GetBinding returns a binding by association ID.
func (s *Store) GetBinding(bindingID string) (IPBinding, error) {
	return scanBinding(s.db.QueryRow(`SELECT `+bindingCols+` FROM routable_ip_bindings WHERE id=?`, bindingID))
}

// DeleteBinding removes one association.
func (s *Store) DeleteBinding(bindingID string) error {
	res, err := s.db.Exec(`DELETE FROM routable_ip_bindings WHERE id=?`, bindingID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteBindingsForIP removes all bindings for an address.
func (s *Store) DeleteBindingsForIP(ipID string) error {
	_, err := s.db.Exec(`DELETE FROM routable_ip_bindings WHERE ip_id=?`, ipID)
	return err
}

// ---- exclusions ------------------------------------------------------------

const exclusionCols = `id, address, pool_id, reason, created_by, created_at`

// InsertExclusion stores an admin-managed address exclusion.
func (s *Store) InsertExclusion(e IPExclusion) (IPExclusion, error) {
	if e.ID == "" {
		e.ID = "ipexcl_" + uuid.NewString()
	}
	if e.CreatedAt == "" {
		e.CreatedAt = nowTS()
	}
	_, err := s.db.Exec(`INSERT INTO ipam_exclusions
		(id, address, pool_id, reason, created_by, created_at) VALUES (?,?,?,?,?,?)`,
		e.ID, e.Address, e.PoolID, e.Reason, e.CreatedBy, e.CreatedAt)
	if err != nil {
		return IPExclusion{}, err
	}
	return e, nil
}

func scanExclusion(row interface{ Scan(...any) error }) (IPExclusion, error) {
	var e IPExclusion
	err := row.Scan(&e.ID, &e.Address, &e.PoolID, &e.Reason, &e.CreatedBy, &e.CreatedAt)
	return e, err
}

// GetExclusion returns an exclusion by ID.
func (s *Store) GetExclusion(id string) (IPExclusion, error) {
	return scanExclusion(s.db.QueryRow(`SELECT `+exclusionCols+` FROM ipam_exclusions WHERE id=?`, id))
}

// ListExclusions returns exclusions, optionally filtered to a pool. Global
// exclusions (pool_id='') are always included.
func (s *Store) ListExclusions(poolID string) ([]IPExclusion, error) {
	q := `SELECT ` + exclusionCols + ` FROM ipam_exclusions`
	var args []any
	if poolID != "" {
		q += ` WHERE pool_id='' OR pool_id=?`
		args = append(args, poolID)
	}
	q += ` ORDER BY address`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IPExclusion
	for rows.Next() {
		e, err := scanExclusion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteExclusion removes an exclusion by ID.
func (s *Store) DeleteExclusion(id string) error {
	_, err := s.db.Exec(`DELETE FROM ipam_exclusions WHERE id=?`, id)
	return err
}
