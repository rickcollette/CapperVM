package dns

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// InitSchema creates the DNS tables. Safe to call multiple times.
func InitSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS dns_zones (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			type        TEXT NOT NULL DEFAULT 'private',
			network_id  TEXT NOT NULL DEFAULT '',
			default_ttl INTEGER NOT NULL DEFAULT 30,
			description TEXT NOT NULL DEFAULT '',
			labels_json TEXT NOT NULL DEFAULT '{}',
			created_at  TEXT NOT NULL,
			UNIQUE(name, network_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dns_zones_name ON dns_zones(name);`,
		`CREATE TABLE IF NOT EXISTS dns_records (
			id            TEXT PRIMARY KEY,
			zone_id       TEXT NOT NULL,
			name          TEXT NOT NULL,
			fqdn          TEXT NOT NULL,
			type          TEXT NOT NULL,
			values_json   TEXT NOT NULL,
			ttl           INTEGER NOT NULL,
			source        TEXT NOT NULL DEFAULT 'manual',
			enabled       INTEGER NOT NULL DEFAULT 1,
			weight        INTEGER NOT NULL DEFAULT 0,
			priority      INTEGER NOT NULL DEFAULT 0,
			created_at    TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dns_records_fqdn ON dns_records(zone_id, fqdn, type);`,
		`CREATE TABLE IF NOT EXISTS dns_services (
			id              TEXT PRIMARY KEY,
			zone_id         TEXT NOT NULL,
			network_id      TEXT NOT NULL,
			name            TEXT NOT NULL,
			fqdn            TEXT NOT NULL,
			selector_type   TEXT NOT NULL DEFAULT 'label',
			selector_key    TEXT NOT NULL DEFAULT '',
			selector_value  TEXT NOT NULL,
			protocol        TEXT NOT NULL DEFAULT 'tcp',
			port            INTEGER NOT NULL,
			ttl             INTEGER NOT NULL DEFAULT 5,
			health_source   TEXT NOT NULL DEFAULT '',
			routing_policy  TEXT NOT NULL DEFAULT 'multivalue',
			created_at      TEXT NOT NULL,
			UNIQUE(zone_id, name)
		);`,
		`CREATE TABLE IF NOT EXISTS dns_forwarders (
			id             TEXT PRIMARY KEY,
			network_id     TEXT,
			upstreams_json TEXT NOT NULL,
			enabled        INTEGER NOT NULL DEFAULT 1,
			created_at     TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS dns_zone_vpc_assoc (
			zone_id TEXT NOT NULL,
			vpc_id  TEXT NOT NULL,
			PRIMARY KEY (zone_id, vpc_id)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("dns.InitSchema: %w", err)
		}
	}
	return nil
}

// Store provides CRUD operations for DNS configuration.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store backed by db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// ---- zones ------------------------------------------------------------------

// InsertZone creates a new zone. Fails if name+networkID already exists.
func (s *Store) InsertZone(z Zone) error {
	labels, _ := json.Marshal(z.Labels)
	_, err := s.db.Exec(
		`INSERT INTO dns_zones (id, name, type, network_id, default_ttl, description, labels_json, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		z.ID, z.Name, z.Type, z.NetworkID, z.DefaultTTL, z.Description, string(labels), z.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("dns: insert zone: %w", err)
	}
	return nil
}

// GetZone returns a zone by ID or name. When networkID is non-empty, restricts
// to that network's private zone.
func (s *Store) GetZone(nameOrID, networkID string) (Zone, error) {
	var row *sql.Row
	if networkID != "" {
		row = s.db.QueryRow(
			`SELECT id, name, type, network_id, default_ttl, description, labels_json, created_at
			FROM dns_zones WHERE (id=? OR name=?) AND network_id=?`,
			nameOrID, nameOrID, networkID)
	} else {
		row = s.db.QueryRow(
			`SELECT id, name, type, network_id, default_ttl, description, labels_json, created_at
			FROM dns_zones WHERE id=? OR name=? ORDER BY created_at LIMIT 1`,
			nameOrID, nameOrID)
	}
	return scanZone(row)
}

// ListZones returns all zones, optionally filtered by networkID.
func (s *Store) ListZones(networkID string) ([]Zone, error) {
	var rows *sql.Rows
	var err error
	if networkID != "" {
		rows, err = s.db.Query(
			`SELECT id, name, type, network_id, default_ttl, description, labels_json, created_at
			FROM dns_zones WHERE network_id=? ORDER BY name`,
			networkID)
	} else {
		rows, err = s.db.Query(
			`SELECT id, name, type, network_id, default_ttl, description, labels_json, created_at
			FROM dns_zones ORDER BY name`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Zone
	for rows.Next() {
		z, err := scanZone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

// DeleteZone removes a zone and all its records and services.
func (s *Store) DeleteZone(zoneID string) error {
	if _, err := s.db.Exec(`DELETE FROM dns_records WHERE zone_id=?`, zoneID); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM dns_services WHERE zone_id=?`, zoneID); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM dns_zones WHERE id=?`, zoneID)
	return err
}

// FindZonesForName returns all zones whose name is a suffix of qname,
// sorted longest-suffix first (best match first).
func (s *Store) FindZonesForName(qname string) ([]Zone, error) {
	qname = strings.TrimSuffix(strings.ToLower(qname), ".")
	rows, err := s.db.Query(
		`SELECT id, name, type, network_id, default_ttl, description, labels_json, created_at
		FROM dns_zones ORDER BY LENGTH(name) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Zone
	for rows.Next() {
		z, err := scanZone(rows)
		if err != nil {
			return nil, err
		}
		zoneName := strings.ToLower(z.Name)
		if qname == zoneName || strings.HasSuffix(qname, "."+zoneName) {
			out = append(out, z)
		}
	}
	return out, rows.Err()
}

// ---- records ----------------------------------------------------------------

// InsertRecord adds a DNS record.
func (s *Store) InsertRecord(r Record) error {
	vals, _ := json.Marshal(r.Values)
	_, err := s.db.Exec(
		`INSERT INTO dns_records
			(id, zone_id, name, fqdn, type, values_json, ttl, source, enabled, weight, priority, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.ZoneID, r.Name, r.FQDN, r.Type, string(vals),
		r.TTL, r.Source, boolInt(r.Enabled), r.Weight, r.Priority, r.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("dns: insert record: %w", err)
	}
	return nil
}

// GetRecord returns a record by ID.
func (s *Store) GetRecord(recordID string) (Record, error) {
	row := s.db.QueryRow(
		`SELECT id, zone_id, name, fqdn, type, values_json, ttl, source, enabled, weight, priority, created_at
		FROM dns_records WHERE id=?`, recordID)
	return scanRecord(row)
}

// ListRecords returns all enabled records for a zone.
func (s *Store) ListRecords(zoneID string) ([]Record, error) {
	rows, err := s.db.Query(
		`SELECT id, zone_id, name, fqdn, type, values_json, ttl, source, enabled, weight, priority, created_at
		FROM dns_records WHERE zone_id=? ORDER BY name, type`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

// LookupRecords returns enabled records matching fqdn and type for a zone.
// type "*" returns all types.
func (s *Store) LookupRecords(zoneID, fqdn, recType string) ([]Record, error) {
	fqdn = normFQDN(fqdn)
	var rows *sql.Rows
	var err error
	if recType == "*" || recType == "" {
		rows, err = s.db.Query(
			`SELECT id, zone_id, name, fqdn, type, values_json, ttl, source, enabled, weight, priority, created_at
			FROM dns_records WHERE zone_id=? AND fqdn=? AND enabled=1`, zoneID, fqdn)
	} else {
		rows, err = s.db.Query(
			`SELECT id, zone_id, name, fqdn, type, values_json, ttl, source, enabled, weight, priority, created_at
			FROM dns_records WHERE zone_id=? AND fqdn=? AND type=? AND enabled=1`,
			zoneID, fqdn, strings.ToUpper(recType))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

// DeleteRecord removes a record.
func (s *Store) DeleteRecord(recordID string) error {
	res, err := s.db.Exec(`DELETE FROM dns_records WHERE id=?`, recordID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("dns: record %q not found", recordID)
	}
	return nil
}

// ---- services ---------------------------------------------------------------

// InsertService adds a ServiceRecord.
func (s *Store) InsertService(svc ServiceRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO dns_services
			(id, zone_id, network_id, name, fqdn, selector_type, selector_key, selector_value,
			 protocol, port, ttl, health_source, routing_policy, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		svc.ID, svc.ZoneID, svc.NetworkID, svc.Name, svc.FQDN,
		svc.SelectorType, svc.SelectorKey, svc.SelectorValue,
		svc.Protocol, svc.Port, svc.TTL, svc.HealthSource, svc.RoutingPolicy, svc.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("dns: insert service: %w", err)
	}
	return nil
}

// ListServices returns all service records for a zone.
func (s *Store) ListServices(zoneID string) ([]ServiceRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, zone_id, network_id, name, fqdn, selector_type, selector_key, selector_value,
			protocol, port, ttl, health_source, routing_policy, created_at
		FROM dns_services WHERE zone_id=? ORDER BY name`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanServices(rows)
}

// LookupService returns the first ServiceRecord whose FQDN matches.
func (s *Store) LookupService(zoneID, fqdn string) (ServiceRecord, bool, error) {
	fqdn = normFQDN(fqdn)
	row := s.db.QueryRow(
		`SELECT id, zone_id, network_id, name, fqdn, selector_type, selector_key, selector_value,
			protocol, port, ttl, health_source, routing_policy, created_at
		FROM dns_services WHERE zone_id=? AND fqdn=? LIMIT 1`, zoneID, fqdn)
	svc, err := scanService(row)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return ServiceRecord{}, false, nil
		}
		return ServiceRecord{}, false, err
	}
	return svc, true, nil
}

// DeleteService removes a service record.
func (s *Store) DeleteService(serviceID string) error {
	res, err := s.db.Exec(`DELETE FROM dns_services WHERE id=?`, serviceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("dns: service %q not found", serviceID)
	}
	return nil
}

// ---- forwarders -------------------------------------------------------------

// UpsertForwarder sets the upstream forwarders for a network (or globally).
func (s *Store) UpsertForwarder(f Forwarder) error {
	ups, _ := json.Marshal(f.Upstreams)
	_, err := s.db.Exec(
		`INSERT INTO dns_forwarders (id, network_id, upstreams_json, enabled, created_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET upstreams_json=excluded.upstreams_json, enabled=excluded.enabled`,
		f.ID, f.NetworkID, string(ups), boolInt(f.Enabled), f.CreatedAt,
	)
	return err
}

// GetForwarder returns the forwarder for the given network (or the global one).
func (s *Store) GetForwarder(networkID string) (Forwarder, bool, error) {
	row := s.db.QueryRow(
		`SELECT id, network_id, upstreams_json, enabled, created_at
		FROM dns_forwarders WHERE network_id=? AND enabled=1 LIMIT 1`,
		networkID)
	f, err := scanForwarder(row)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return Forwarder{}, false, nil
		}
		return Forwarder{}, false, err
	}
	return f, true, nil
}

// ---- scanners ---------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanZone(s rowScanner) (Zone, error) {
	var z Zone
	var labelsJSON string
	err := s.Scan(&z.ID, &z.Name, &z.Type, &z.NetworkID, &z.DefaultTTL, &z.Description, &labelsJSON, &z.CreatedAt)
	if err != nil {
		return Zone{}, fmt.Errorf("dns: scan zone: %w", err)
	}
	json.Unmarshal([]byte(labelsJSON), &z.Labels)
	return z, nil
}

func scanRecord(s rowScanner) (Record, error) {
	var r Record
	var enabled int
	var valsJSON string
	err := s.Scan(&r.ID, &r.ZoneID, &r.Name, &r.FQDN, &r.Type, &valsJSON, &r.TTL, &r.Source, &enabled, &r.Weight, &r.Priority, &r.CreatedAt)
	if err != nil {
		return Record{}, fmt.Errorf("dns: scan record: %w", err)
	}
	r.Enabled = enabled != 0
	json.Unmarshal([]byte(valsJSON), &r.Values)
	return r, nil
}

func scanRecords(rows *sql.Rows) ([]Record, error) {
	var out []Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanService(s rowScanner) (ServiceRecord, error) {
	var svc ServiceRecord
	err := s.Scan(
		&svc.ID, &svc.ZoneID, &svc.NetworkID, &svc.Name, &svc.FQDN,
		&svc.SelectorType, &svc.SelectorKey, &svc.SelectorValue,
		&svc.Protocol, &svc.Port, &svc.TTL, &svc.HealthSource, &svc.RoutingPolicy, &svc.CreatedAt,
	)
	if err != nil {
		return ServiceRecord{}, fmt.Errorf("dns: scan service: %w", err)
	}
	return svc, nil
}

func scanServices(rows *sql.Rows) ([]ServiceRecord, error) {
	var out []ServiceRecord
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, svc)
	}
	return out, rows.Err()
}

func scanForwarder(s rowScanner) (Forwarder, error) {
	var f Forwarder
	var enabled int
	var upsJSON string
	err := s.Scan(&f.ID, &f.NetworkID, &upsJSON, &enabled, &f.CreatedAt)
	if err != nil {
		return Forwarder{}, err
	}
	f.Enabled = enabled != 0
	json.Unmarshal([]byte(upsJSON), &f.Upstreams)
	return f, nil
}

// ---- helpers ----------------------------------------------------------------

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// normFQDN lower-cases and ensures a trailing dot, matching miekg/dns wire format.
func normFQDN(name string) string {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	return name + "."
}
