package topology

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// InitSchema creates all topology tables idempotently.
func InitSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS realms (
			id          TEXT PRIMARY KEY,
			slug        TEXT NOT NULL UNIQUE,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'active',
			labels_json TEXT NOT NULL DEFAULT '{}',
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS regions (
			id           TEXT PRIMARY KEY,
			realm_id     TEXT NOT NULL,
			slug         TEXT NOT NULL,
			name         TEXT NOT NULL,
			description  TEXT NOT NULL DEFAULT '',
			location     TEXT NOT NULL DEFAULT '',
			country      TEXT NOT NULL DEFAULT '',
			region_code  TEXT NOT NULL DEFAULT '',
			latitude     REAL NOT NULL DEFAULT 0,
			longitude    REAL NOT NULL DEFAULT 0,
			status       TEXT NOT NULL DEFAULT 'active',
			control_url  TEXT NOT NULL DEFAULT '',
			api_url      TEXT NOT NULL DEFAULT '',
			labels_json  TEXT NOT NULL DEFAULT '{}',
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL,
			UNIQUE(realm_id, slug),
			FOREIGN KEY(realm_id) REFERENCES realms(id)
		)`,
		`CREATE TABLE IF NOT EXISTS zones (
			id             TEXT PRIMARY KEY,
			realm_id       TEXT NOT NULL,
			region_id      TEXT NOT NULL,
			slug           TEXT NOT NULL,
			name           TEXT NOT NULL,
			description    TEXT NOT NULL DEFAULT '',
			failure_domain TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL DEFAULT 'active',
			control_url    TEXT NOT NULL DEFAULT '',
			network_cidr   TEXT NOT NULL DEFAULT '',
			labels_json    TEXT NOT NULL DEFAULT '{}',
			created_at     TEXT NOT NULL,
			updated_at     TEXT NOT NULL,
			UNIQUE(region_id, slug),
			FOREIGN KEY(realm_id) REFERENCES realms(id),
			FOREIGN KEY(region_id) REFERENCES regions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id             TEXT PRIMARY KEY,
			realm_id       TEXT NOT NULL,
			region_id      TEXT NOT NULL,
			zone_id        TEXT NOT NULL,
			slug           TEXT NOT NULL,
			name           TEXT NOT NULL,
			address        TEXT NOT NULL,
			status         TEXT NOT NULL DEFAULT 'ready',
			failure_domain TEXT NOT NULL DEFAULT '',
			labels_json    TEXT NOT NULL DEFAULT '{}',
			cpu_count      INTEGER NOT NULL DEFAULT 0,
			memory_bytes   INTEGER NOT NULL DEFAULT 0,
			disk_bytes     INTEGER NOT NULL DEFAULT 0,
			created_at     TEXT NOT NULL,
			updated_at     TEXT NOT NULL,
			UNIQUE(zone_id, slug),
			FOREIGN KEY(realm_id) REFERENCES realms(id),
			FOREIGN KEY(region_id) REFERENCES regions(id),
			FOREIGN KEY(zone_id) REFERENCES zones(id)
		)`,
		`CREATE TABLE IF NOT EXISTS vpcs (
			id              TEXT PRIMARY KEY,
			realm_id        TEXT NOT NULL,
			project         TEXT NOT NULL,
			slug            TEXT NOT NULL,
			name            TEXT NOT NULL,
			cidr            TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT 'active',
			home_region_id  TEXT NOT NULL DEFAULT '',
			mobility_policy TEXT NOT NULL DEFAULT 'manual',
			labels_json     TEXT NOT NULL DEFAULT '{}',
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL,
			UNIQUE(project, slug)
		)`,
		`CREATE TABLE IF NOT EXISTS vpc_subnets (
			id         TEXT PRIMARY KEY,
			vpc_id     TEXT NOT NULL,
			realm_id   TEXT NOT NULL,
			region_id  TEXT NOT NULL,
			zone_id    TEXT NOT NULL,
			slug       TEXT NOT NULL,
			name       TEXT NOT NULL,
			cidr       TEXT NOT NULL,
			gateway    TEXT NOT NULL,
			bridge     TEXT NOT NULL DEFAULT '',
			mode       TEXT NOT NULL DEFAULT 'nat',
			status     TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(vpc_id, zone_id, slug),
			FOREIGN KEY(vpc_id) REFERENCES vpcs(id)
		)`,
		`CREATE TABLE IF NOT EXISTS vpc_routes (
			id          TEXT PRIMARY KEY,
			vpc_id      TEXT NOT NULL,
			scope       TEXT NOT NULL DEFAULT 'zone',
			realm_id    TEXT NOT NULL,
			region_id   TEXT NOT NULL DEFAULT '',
			zone_id     TEXT NOT NULL DEFAULT '',
			dest_cidr   TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id   TEXT NOT NULL DEFAULT '',
			priority    INTEGER NOT NULL DEFAULT 100,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			FOREIGN KEY(vpc_id) REFERENCES vpcs(id)
		)`,
		`CREATE TABLE IF NOT EXISTS placement_policies (
			id                     TEXT PRIMARY KEY,
			realm_id               TEXT NOT NULL,
			project                TEXT NOT NULL,
			slug                   TEXT NOT NULL,
			name                   TEXT NOT NULL,
			description            TEXT NOT NULL DEFAULT '',
			scope                  TEXT NOT NULL DEFAULT 'region',
			strategy               TEXT NOT NULL DEFAULT 'spread-zones',
			min_regions            INTEGER NOT NULL DEFAULT 1,
			min_zones              INTEGER NOT NULL DEFAULT 1,
			max_zones              INTEGER NOT NULL DEFAULT 0,
			preferred_regions_json TEXT NOT NULL DEFAULT '[]',
			preferred_zones_json   TEXT NOT NULL DEFAULT '[]',
			required_labels_json   TEXT NOT NULL DEFAULT '{}',
			anti_affinity_json     TEXT NOT NULL DEFAULT '{}',
			labels_json            TEXT NOT NULL DEFAULT '{}',
			created_at             TEXT NOT NULL,
			updated_at             TEXT NOT NULL,
			UNIQUE(project, slug)
		)`,
		`CREATE TABLE IF NOT EXISTS image_replicas (
			id         TEXT PRIMARY KEY,
			image_id   TEXT NOT NULL,
			digest     TEXT NOT NULL,
			realm_id   TEXT NOT NULL,
			region_id  TEXT NOT NULL,
			zone_id    TEXT NOT NULL DEFAULT '',
			node_id    TEXT NOT NULL DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'pending',
			size_bytes INTEGER NOT NULL DEFAULT 0,
			location   TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS storage_replicas (
			id         TEXT PRIMARY KEY,
			bucket     TEXT NOT NULL,
			object_key TEXT NOT NULL,
			etag       TEXT NOT NULL,
			realm_id   TEXT NOT NULL,
			region_id  TEXT NOT NULL,
			zone_id    TEXT NOT NULL,
			node_id    TEXT NOT NULL DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'pending',
			size_bytes INTEGER NOT NULL DEFAULT 0,
			location   TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS service_health (
			id           TEXT PRIMARY KEY,
			scope        TEXT NOT NULL,
			realm_id     TEXT NOT NULL DEFAULT '',
			region_id    TEXT NOT NULL DEFAULT '',
			zone_id      TEXT NOT NULL DEFAULT '',
			node_id      TEXT NOT NULL DEFAULT '',
			service_name TEXT NOT NULL,
			status       TEXT NOT NULL,
			message      TEXT NOT NULL DEFAULT '',
			checked_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS migration_plans (
			id               TEXT PRIMARY KEY,
			realm_id         TEXT NOT NULL,
			project          TEXT NOT NULL,
			name             TEXT NOT NULL,
			migration_type   TEXT NOT NULL,
			source_region_id TEXT NOT NULL DEFAULT '',
			target_region_id TEXT NOT NULL DEFAULT '',
			source_zone_id   TEXT NOT NULL DEFAULT '',
			target_zone_id   TEXT NOT NULL DEFAULT '',
			vpc_id           TEXT NOT NULL DEFAULT '',
			status           TEXT NOT NULL DEFAULT 'draft',
			plan_json        TEXT NOT NULL DEFAULT '{}',
			result_json      TEXT NOT NULL DEFAULT '{}',
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}

	// Additive migrations on existing tables.
	for _, alt := range []string{
		`ALTER TABLE instances ADD COLUMN realm_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE instances ADD COLUMN region_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE instances ADD COLUMN zone_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE instances ADD COLUMN node_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE instances ADD COLUMN placement_policy_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE instances ADD COLUMN desired_state TEXT NOT NULL DEFAULT 'running'`,
		`ALTER TABLE instances ADD COLUMN generation INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE lb_load_balancers ADD COLUMN realm_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN region_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN zone_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lb_load_balancers ADD COLUMN scope TEXT NOT NULL DEFAULT 'zone'`,
		// Node extended fields
		`ALTER TABLE nodes ADD COLUMN gpu_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN gpu_memory_bytes INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN agent_version TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN last_heartbeat TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN cordoned INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := db.Exec(alt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				return fmt.Errorf("topology schema migration (%s): %w", alt, err)
			}
		}
	}

	// New node-related tables (idempotent).
	for _, s := range []string{
		`CREATE TABLE IF NOT EXISTS node_roles (
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			role    TEXT NOT NULL,
			PRIMARY KEY (node_id, role)
		)`,
		`CREATE TABLE IF NOT EXISTS node_labels (
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			key     TEXT NOT NULL,
			value   TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (node_id, key)
		)`,
		`CREATE TABLE IF NOT EXISTS node_taints (
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			key     TEXT NOT NULL,
			value   TEXT NOT NULL DEFAULT '',
			effect  TEXT NOT NULL DEFAULT 'NoSchedule',
			PRIMARY KEY (node_id, key)
		)`,
		`CREATE TABLE IF NOT EXISTS node_services (
			node_id       TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			service_name  TEXT NOT NULL,
			desired_state TEXT NOT NULL DEFAULT 'running',
			actual_state  TEXT NOT NULL DEFAULT 'unknown',
			version       TEXT NOT NULL DEFAULT '',
			health        TEXT NOT NULL DEFAULT 'unknown',
			message       TEXT NOT NULL DEFAULT '',
			last_seen     TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (node_id, service_name)
		)`,
		`CREATE TABLE IF NOT EXISTS node_heartbeats (
			node_id           TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
			status            TEXT NOT NULL DEFAULT 'unknown',
			cpu_used          INTEGER NOT NULL DEFAULT 0,
			memory_used_bytes INTEGER NOT NULL DEFAULT 0,
			disk_used_bytes   INTEGER NOT NULL DEFAULT 0,
			gpu_used          INTEGER NOT NULL DEFAULT 0,
			message           TEXT NOT NULL DEFAULT '',
			seen_at           TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS node_pools (
			id               TEXT PRIMARY KEY,
			name             TEXT NOT NULL UNIQUE,
			realm_id         TEXT NOT NULL DEFAULT '',
			region_id        TEXT NOT NULL DEFAULT '',
			zone_id          TEXT NOT NULL DEFAULT '',
			status           TEXT NOT NULL DEFAULT 'active',
			min_nodes        INTEGER NOT NULL DEFAULT 0,
			desired_nodes    INTEGER NOT NULL DEFAULT 0,
			max_nodes        INTEGER NOT NULL DEFAULT 0,
			placement_policy TEXT NOT NULL DEFAULT 'spread',
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS node_pool_roles (
			pool_id TEXT NOT NULL REFERENCES node_pools(id) ON DELETE CASCADE,
			role    TEXT NOT NULL,
			PRIMARY KEY (pool_id, role)
		)`,
		`CREATE TABLE IF NOT EXISTS node_pool_members (
			pool_id TEXT NOT NULL REFERENCES node_pools(id) ON DELETE CASCADE,
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			PRIMARY KEY (pool_id, node_id)
		)`,
		`CREATE TABLE IF NOT EXISTS node_pool_labels (
			pool_id TEXT NOT NULL REFERENCES node_pools(id) ON DELETE CASCADE,
			key     TEXT NOT NULL,
			value   TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (pool_id, key)
		)`,
		`CREATE TABLE IF NOT EXISTS join_tokens (
			id         TEXT PRIMARY KEY,
			token      TEXT NOT NULL UNIQUE,
			realm_id   TEXT NOT NULL DEFAULT '',
			region_id  TEXT NOT NULL DEFAULT '',
			zone_id    TEXT NOT NULL DEFAULT '',
			roles      TEXT NOT NULL DEFAULT '[]',
			uses_left  INTEGER NOT NULL DEFAULT 1,
			expires_at TEXT NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
	} {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("topology new-table migration: %w", err)
		}
	}
	// Seed the default local realm/region/zone so AIO mode works out of the box.
	seeds := []string{
		`INSERT OR IGNORE INTO realms (id, slug, name, description, status, labels_json, created_at, updated_at)
		 VALUES ('realm_local','local','Local','Default local realm','active','{}',datetime('now'),datetime('now'))`,
		`INSERT OR IGNORE INTO regions (id, realm_id, slug, name, description, status, labels_json, created_at, updated_at)
		 VALUES ('region_local','realm_local','local','Local','Default local region','active','{}',datetime('now'),datetime('now'))`,
		`INSERT OR IGNORE INTO zones (id, realm_id, region_id, slug, name, description, status, labels_json, created_at, updated_at)
		 VALUES ('zone_local','realm_local','region_local','local-a','Local A','Default local zone','active','{}',datetime('now'),datetime('now'))`,
	}
	for _, s := range seeds {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("topology seed: %w", err)
		}
	}
	return nil
}

// Store provides CRUD for all topology resources.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// ---- helpers ----------------------------------------------------------------

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func marshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func unmarshalStringSlice(s string) []string {
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func unmarshalStringMap(s string) map[string]string {
	out := make(map[string]string)
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func genID(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

// ---- Realm CRUD -------------------------------------------------------------

func (s *Store) InsertRealm(r Realm) error {
	if r.ID == "" {
		r.ID = genID("rlm_")
	}
	n := now()
	if r.CreatedAt == "" {
		r.CreatedAt = n
	}
	r.UpdatedAt = n
	_, err := s.db.Exec(
		`INSERT INTO realms (id,slug,name,description,status,labels_json,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		r.ID, r.Slug, r.Name, r.Description, orDefault(r.Status, StatusActive),
		marshalJSON(r.Labels), r.CreatedAt, r.UpdatedAt,
	)
	return err
}

func (s *Store) GetRealm(slugOrID string) (Realm, error) {
	row := s.db.QueryRow(
		`SELECT id,slug,name,description,status,labels_json,created_at,updated_at
		 FROM realms WHERE id=? OR slug=?`, slugOrID, slugOrID)
	return scanRealm(row.Scan)
}

func (s *Store) ListRealms() ([]Realm, error) {
	rows, err := s.db.Query(`SELECT id,slug,name,description,status,labels_json,created_at,updated_at FROM realms ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Realm
	for rows.Next() {
		r, err := scanRealm(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateRealm(r Realm) error {
	r.UpdatedAt = now()
	_, err := s.db.Exec(
		`UPDATE realms SET slug=?,name=?,description=?,status=?,labels_json=?,updated_at=? WHERE id=?`,
		r.Slug, r.Name, r.Description, r.Status, marshalJSON(r.Labels), r.UpdatedAt, r.ID,
	)
	return err
}

func (s *Store) DeleteRealm(slugOrID string) error {
	_, err := s.db.Exec(`DELETE FROM realms WHERE id=? OR slug=?`, slugOrID, slugOrID)
	return err
}

func scanRealm(scan func(...any) error) (Realm, error) {
	var r Realm
	var labelsJSON string
	err := scan(&r.ID, &r.Slug, &r.Name, &r.Description, &r.Status, &labelsJSON, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return r, ErrNotFound
	}
	r.Labels = unmarshalStringMap(labelsJSON)
	return r, err
}

// ---- Region CRUD ------------------------------------------------------------

func (s *Store) InsertRegion(r Region) error {
	if r.ID == "" {
		r.ID = genID("reg_")
	}
	n := now()
	if r.CreatedAt == "" {
		r.CreatedAt = n
	}
	r.UpdatedAt = n
	_, err := s.db.Exec(
		`INSERT INTO regions (id,realm_id,slug,name,description,location,country,region_code,
		                       latitude,longitude,status,control_url,api_url,labels_json,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.RealmID, r.Slug, r.Name, r.Description, r.Location, r.Country, r.RegionCode,
		r.Latitude, r.Longitude, orDefault(r.Status, StatusActive),
		r.ControlURL, r.APIURL, marshalJSON(r.Labels), r.CreatedAt, r.UpdatedAt,
	)
	return err
}

func (s *Store) GetRegion(slugOrID string) (Region, error) {
	row := s.db.QueryRow(
		`SELECT id,realm_id,slug,name,description,location,country,region_code,
		        latitude,longitude,status,control_url,api_url,labels_json,created_at,updated_at
		 FROM regions WHERE id=? OR slug=?`, slugOrID, slugOrID)
	return scanRegion(row.Scan)
}

func (s *Store) ListRegions(realmID string) ([]Region, error) {
	var rows *sql.Rows
	var err error
	if realmID != "" {
		rows, err = s.db.Query(
			`SELECT id,realm_id,slug,name,description,location,country,region_code,
			        latitude,longitude,status,control_url,api_url,labels_json,created_at,updated_at
			 FROM regions WHERE realm_id=? ORDER BY slug`, realmID)
	} else {
		rows, err = s.db.Query(
			`SELECT id,realm_id,slug,name,description,location,country,region_code,
			        latitude,longitude,status,control_url,api_url,labels_json,created_at,updated_at
			 FROM regions ORDER BY slug`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Region
	for rows.Next() {
		r, err := scanRegion(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateRegion(r Region) error {
	r.UpdatedAt = now()
	_, err := s.db.Exec(
		`UPDATE regions SET slug=?,name=?,description=?,location=?,country=?,region_code=?,
		                    latitude=?,longitude=?,status=?,control_url=?,api_url=?,labels_json=?,updated_at=?
		 WHERE id=?`,
		r.Slug, r.Name, r.Description, r.Location, r.Country, r.RegionCode,
		r.Latitude, r.Longitude, r.Status, r.ControlURL, r.APIURL, marshalJSON(r.Labels), r.UpdatedAt, r.ID,
	)
	return err
}

func (s *Store) DeleteRegion(slugOrID string) error {
	_, err := s.db.Exec(`DELETE FROM regions WHERE id=? OR slug=?`, slugOrID, slugOrID)
	return err
}

func scanRegion(scan func(...any) error) (Region, error) {
	var r Region
	var labelsJSON string
	err := scan(&r.ID, &r.RealmID, &r.Slug, &r.Name, &r.Description, &r.Location, &r.Country,
		&r.RegionCode, &r.Latitude, &r.Longitude, &r.Status, &r.ControlURL, &r.APIURL,
		&labelsJSON, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return r, ErrNotFound
	}
	r.Labels = unmarshalStringMap(labelsJSON)
	return r, err
}

// ---- Zone CRUD --------------------------------------------------------------

func (s *Store) InsertZone(z Zone) error {
	if z.ID == "" {
		z.ID = genID("zon_")
	}
	n := now()
	if z.CreatedAt == "" {
		z.CreatedAt = n
	}
	z.UpdatedAt = n
	_, err := s.db.Exec(
		`INSERT INTO zones (id,realm_id,region_id,slug,name,description,failure_domain,
		                     status,control_url,network_cidr,labels_json,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		z.ID, z.RealmID, z.RegionID, z.Slug, z.Name, z.Description, z.FailureDomain,
		orDefault(z.Status, StatusActive), z.ControlURL, z.NetworkCIDR,
		marshalJSON(z.Labels), z.CreatedAt, z.UpdatedAt,
	)
	return err
}

func (s *Store) GetZone(slugOrID string) (Zone, error) {
	row := s.db.QueryRow(
		`SELECT id,realm_id,region_id,slug,name,description,failure_domain,
		        status,control_url,network_cidr,labels_json,created_at,updated_at
		 FROM zones WHERE id=? OR slug=?`, slugOrID, slugOrID)
	return scanZone(row.Scan)
}

func (s *Store) ListZones(regionID string) ([]Zone, error) {
	var rows *sql.Rows
	var err error
	if regionID != "" {
		rows, err = s.db.Query(
			`SELECT id,realm_id,region_id,slug,name,description,failure_domain,
			        status,control_url,network_cidr,labels_json,created_at,updated_at
			 FROM zones WHERE region_id=? ORDER BY slug`, regionID)
	} else {
		rows, err = s.db.Query(
			`SELECT id,realm_id,region_id,slug,name,description,failure_domain,
			        status,control_url,network_cidr,labels_json,created_at,updated_at
			 FROM zones ORDER BY slug`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Zone
	for rows.Next() {
		z, err := scanZone(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

func (s *Store) UpdateZone(z Zone) error {
	z.UpdatedAt = now()
	_, err := s.db.Exec(
		`UPDATE zones SET slug=?,name=?,description=?,failure_domain=?,status=?,
		                  control_url=?,network_cidr=?,labels_json=?,updated_at=?
		 WHERE id=?`,
		z.Slug, z.Name, z.Description, z.FailureDomain, z.Status,
		z.ControlURL, z.NetworkCIDR, marshalJSON(z.Labels), z.UpdatedAt, z.ID,
	)
	return err
}

func (s *Store) UpdateZoneStatus(slugOrID, status string) error {
	_, err := s.db.Exec(
		`UPDATE zones SET status=?,updated_at=? WHERE id=? OR slug=?`,
		status, now(), slugOrID, slugOrID,
	)
	return err
}

func (s *Store) DeleteZone(slugOrID string) error {
	_, err := s.db.Exec(`DELETE FROM zones WHERE id=? OR slug=?`, slugOrID, slugOrID)
	return err
}

func scanZone(scan func(...any) error) (Zone, error) {
	var z Zone
	var labelsJSON string
	err := scan(&z.ID, &z.RealmID, &z.RegionID, &z.Slug, &z.Name, &z.Description,
		&z.FailureDomain, &z.Status, &z.ControlURL, &z.NetworkCIDR, &labelsJSON, &z.CreatedAt, &z.UpdatedAt)
	if err == sql.ErrNoRows {
		return z, ErrNotFound
	}
	z.Labels = unmarshalStringMap(labelsJSON)
	return z, err
}

// ---- Node CRUD --------------------------------------------------------------

func (s *Store) InsertNode(n Node) (Node, error) {
	if n.ID == "" {
		n.ID = genID("nod_")
	}
	ts := now()
	if n.CreatedAt == "" {
		n.CreatedAt = ts
	}
	n.UpdatedAt = ts
	cordonedInt := 0
	if n.Cordoned {
		cordonedInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO nodes (id,realm_id,region_id,zone_id,slug,name,address,status,
		                     failure_domain,labels_json,cpu_count,memory_bytes,disk_bytes,
		                     gpu_count,gpu_memory_bytes,agent_version,last_heartbeat,cordoned,
		                     created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		n.ID, n.RealmID, n.RegionID, n.ZoneID, n.Slug, n.Name, n.Address,
		orDefault(n.Status, StatusReady), n.FailureDomain, marshalJSON(n.Labels),
		n.CPUCount, n.MemoryBytes, n.DiskBytes,
		n.GPUCount, n.GPUMemoryBytes, n.AgentVersion, n.LastHeartbeat, cordonedInt,
		n.CreatedAt, n.UpdatedAt,
	)
	if err != nil {
		return n, err
	}
	return s.GetNode(n.ID)
}

func (s *Store) GetNode(slugOrID string) (Node, error) {
	row := s.db.QueryRow(
		`SELECT id,realm_id,region_id,zone_id,slug,name,address,status,
		        failure_domain,labels_json,cpu_count,memory_bytes,disk_bytes,
		        gpu_count,gpu_memory_bytes,agent_version,last_heartbeat,cordoned,
		        created_at,updated_at
		 FROM nodes WHERE id=? OR slug=?`, slugOrID, slugOrID)
	n, err := scanNode(row.Scan)
	if err != nil {
		return n, err
	}
	n.Roles, _ = s.GetNodeRoles(n.ID)
	return n, nil
}

func (s *Store) ListNodes(zoneID string) ([]Node, error) {
	var rows *sql.Rows
	var err error
	if zoneID != "" {
		rows, err = s.db.Query(
			`SELECT id,realm_id,region_id,zone_id,slug,name,address,status,
			        failure_domain,labels_json,cpu_count,memory_bytes,disk_bytes,
			        gpu_count,gpu_memory_bytes,agent_version,last_heartbeat,cordoned,
			        created_at,updated_at
			 FROM nodes WHERE zone_id=? ORDER BY slug`, zoneID)
	} else {
		rows, err = s.db.Query(
			`SELECT id,realm_id,region_id,zone_id,slug,name,address,status,
			        failure_domain,labels_json,cpu_count,memory_bytes,disk_bytes,
			        gpu_count,gpu_memory_bytes,agent_version,last_heartbeat,cordoned,
			        created_at,updated_at
			 FROM nodes ORDER BY slug`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		n, err := scanNode(rows.Scan)
		if err != nil {
			return nil, err
		}
		n.Roles, _ = s.GetNodeRoles(n.ID)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) UpdateNode(n Node) error {
	n.UpdatedAt = now()
	cordonedInt := 0
	if n.Cordoned {
		cordonedInt = 1
	}
	_, err := s.db.Exec(
		`UPDATE nodes SET slug=?,name=?,address=?,status=?,failure_domain=?,
		                  labels_json=?,cpu_count=?,memory_bytes=?,disk_bytes=?,
		                  gpu_count=?,gpu_memory_bytes=?,agent_version=?,last_heartbeat=?,cordoned=?,
		                  updated_at=?
		 WHERE id=?`,
		n.Slug, n.Name, n.Address, n.Status, n.FailureDomain,
		marshalJSON(n.Labels), n.CPUCount, n.MemoryBytes, n.DiskBytes,
		n.GPUCount, n.GPUMemoryBytes, n.AgentVersion, n.LastHeartbeat, cordonedInt,
		n.UpdatedAt, n.ID,
	)
	return err
}

// UpdateNodeHeartbeat updates a node's last_heartbeat timestamp and status.
func (s *Store) UpdateNodeHeartbeat(nodeID, status string) error {
	_, err := s.db.Exec(
		`UPDATE nodes SET status=?,last_heartbeat=?,updated_at=? WHERE id=?`,
		status, now(), now(), nodeID,
	)
	return err
}

func (s *Store) UpdateNodeStatus(slugOrID, status string) error {
	_, err := s.db.Exec(
		`UPDATE nodes SET status=?,updated_at=? WHERE id=? OR slug=?`,
		status, now(), slugOrID, slugOrID,
	)
	return err
}

func (s *Store) DeleteNode(slugOrID string) error {
	_, err := s.db.Exec(`DELETE FROM nodes WHERE id=? OR slug=?`, slugOrID, slugOrID)
	return err
}

func scanNode(scan func(...any) error) (Node, error) {
	var n Node
	var labelsJSON string
	var cordonedInt int
	err := scan(&n.ID, &n.RealmID, &n.RegionID, &n.ZoneID, &n.Slug, &n.Name, &n.Address,
		&n.Status, &n.FailureDomain, &labelsJSON, &n.CPUCount, &n.MemoryBytes, &n.DiskBytes,
		&n.GPUCount, &n.GPUMemoryBytes, &n.AgentVersion, &n.LastHeartbeat, &cordonedInt,
		&n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return n, ErrNotFound
	}
	n.Labels = unmarshalStringMap(labelsJSON)
	n.Cordoned = cordonedInt != 0
	return n, err
}

// ---- VPC CRUD ---------------------------------------------------------------

func (s *Store) InsertVPC(v VPC) error {
	if v.ID == "" {
		v.ID = genID("vpc_")
	}
	n := now()
	if v.CreatedAt == "" {
		v.CreatedAt = n
	}
	v.UpdatedAt = n
	_, err := s.db.Exec(
		`INSERT INTO vpcs (id,realm_id,project,slug,name,cidr,status,home_region_id,
		                    mobility_policy,labels_json,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		v.ID, v.RealmID, v.Project, v.Slug, v.Name, v.CIDR,
		orDefault(v.Status, StatusActive), v.HomeRegionID,
		orDefault(v.MobilityPolicy, MobilityManual), marshalJSON(v.Labels), v.CreatedAt, v.UpdatedAt,
	)
	return err
}

func (s *Store) GetVPC(project, slugOrID string) (VPC, error) {
	row := s.db.QueryRow(
		`SELECT id,realm_id,project,slug,name,cidr,status,home_region_id,
		        mobility_policy,labels_json,created_at,updated_at
		 FROM vpcs WHERE (id=? OR slug=?) AND project=?`, slugOrID, slugOrID, project)
	return scanVPC(row.Scan)
}

func (s *Store) ListVPCs(project string) ([]VPC, error) {
	rows, err := s.db.Query(
		`SELECT id,realm_id,project,slug,name,cidr,status,home_region_id,
		        mobility_policy,labels_json,created_at,updated_at
		 FROM vpcs WHERE project=? ORDER BY slug`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VPC
	for rows.Next() {
		v, err := scanVPC(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) UpdateVPC(v VPC) error {
	v.UpdatedAt = now()
	_, err := s.db.Exec(
		`UPDATE vpcs SET slug=?,name=?,cidr=?,status=?,home_region_id=?,
		                 mobility_policy=?,labels_json=?,updated_at=?
		 WHERE id=?`,
		v.Slug, v.Name, v.CIDR, v.Status, v.HomeRegionID,
		v.MobilityPolicy, marshalJSON(v.Labels), v.UpdatedAt, v.ID,
	)
	return err
}

func (s *Store) DeleteVPC(project, slugOrID string) error {
	_, err := s.db.Exec(
		`DELETE FROM vpcs WHERE (id=? OR slug=?) AND project=?`, slugOrID, slugOrID, project)
	return err
}

func scanVPC(scan func(...any) error) (VPC, error) {
	var v VPC
	var labelsJSON string
	err := scan(&v.ID, &v.RealmID, &v.Project, &v.Slug, &v.Name, &v.CIDR, &v.Status,
		&v.HomeRegionID, &v.MobilityPolicy, &labelsJSON, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return v, ErrNotFound
	}
	v.Labels = unmarshalStringMap(labelsJSON)
	return v, err
}

// ---- VPCSubnet CRUD ---------------------------------------------------------

func (s *Store) InsertSubnet(sub VPCSubnet) error {
	if sub.ID == "" {
		sub.ID = genID("sub_")
	}
	n := now()
	if sub.CreatedAt == "" {
		sub.CreatedAt = n
	}
	sub.UpdatedAt = n
	_, err := s.db.Exec(
		`INSERT INTO vpc_subnets (id,vpc_id,realm_id,region_id,zone_id,slug,name,
		                           cidr,gateway,bridge,mode,status,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sub.ID, sub.VPCID, sub.RealmID, sub.RegionID, sub.ZoneID,
		sub.Slug, sub.Name, sub.CIDR, sub.Gateway, sub.Bridge,
		orDefault(sub.Mode, "nat"), orDefault(sub.Status, "pending"), sub.CreatedAt, sub.UpdatedAt,
	)
	return err
}

func (s *Store) ListSubnets(vpcID string) ([]VPCSubnet, error) {
	rows, err := s.db.Query(
		`SELECT id,vpc_id,realm_id,region_id,zone_id,slug,name,
		        cidr,gateway,bridge,mode,status,created_at,updated_at
		 FROM vpc_subnets WHERE vpc_id=? ORDER BY slug`, vpcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VPCSubnet
	for rows.Next() {
		var sub VPCSubnet
		if err := rows.Scan(&sub.ID, &sub.VPCID, &sub.RealmID, &sub.RegionID, &sub.ZoneID,
			&sub.Slug, &sub.Name, &sub.CIDR, &sub.Gateway, &sub.Bridge,
			&sub.Mode, &sub.Status, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// ---- VPCRoute CRUD ----------------------------------------------------------

func (s *Store) InsertRoute(rt VPCRoute) error {
	if rt.ID == "" {
		rt.ID = genID("rte_")
	}
	n := now()
	if rt.CreatedAt == "" {
		rt.CreatedAt = n
	}
	rt.UpdatedAt = n
	_, err := s.db.Exec(
		`INSERT INTO vpc_routes (id,vpc_id,scope,realm_id,region_id,zone_id,
		                          dest_cidr,target_type,target_id,priority,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		rt.ID, rt.VPCID, orDefault(rt.Scope, "zone"), rt.RealmID, rt.RegionID, rt.ZoneID,
		rt.DestCIDR, rt.TargetType, rt.TargetID, rt.Priority, rt.CreatedAt, rt.UpdatedAt,
	)
	return err
}

func (s *Store) ListRoutes(vpcID string) ([]VPCRoute, error) {
	rows, err := s.db.Query(
		`SELECT id,vpc_id,scope,realm_id,region_id,zone_id,
		        dest_cidr,target_type,target_id,priority,created_at,updated_at
		 FROM vpc_routes WHERE vpc_id=? ORDER BY priority`, vpcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VPCRoute
	for rows.Next() {
		var rt VPCRoute
		if err := rows.Scan(&rt.ID, &rt.VPCID, &rt.Scope, &rt.RealmID, &rt.RegionID, &rt.ZoneID,
			&rt.DestCIDR, &rt.TargetType, &rt.TargetID, &rt.Priority, &rt.CreatedAt, &rt.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

// ---- PlacementPolicy CRUD ---------------------------------------------------

func (s *Store) InsertPlacementPolicy(p PlacementPolicy) error {
	if p.ID == "" {
		p.ID = genID("plc_")
	}
	n := now()
	if p.CreatedAt == "" {
		p.CreatedAt = n
	}
	p.UpdatedAt = n
	_, err := s.db.Exec(
		`INSERT INTO placement_policies (id,realm_id,project,slug,name,description,scope,strategy,
		  min_regions,min_zones,max_zones,preferred_regions_json,preferred_zones_json,
		  required_labels_json,anti_affinity_json,labels_json,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.RealmID, p.Project, p.Slug, p.Name, p.Description,
		orDefault(p.Scope, "region"), orDefault(p.Strategy, StrategySpreadZones),
		p.MinRegions, p.MinZones, p.MaxZones,
		marshalJSON(p.PreferredRegions), marshalJSON(p.PreferredZones),
		marshalJSON(p.RequiredLabels), marshalJSON(p.AntiAffinity),
		marshalJSON(p.Labels), p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (s *Store) GetPlacementPolicy(project, slugOrID string) (PlacementPolicy, error) {
	row := s.db.QueryRow(
		`SELECT id,realm_id,project,slug,name,description,scope,strategy,
		        min_regions,min_zones,max_zones,preferred_regions_json,preferred_zones_json,
		        required_labels_json,anti_affinity_json,labels_json,created_at,updated_at
		 FROM placement_policies WHERE (id=? OR slug=?) AND project=?`, slugOrID, slugOrID, project)
	return scanPlacementPolicy(row.Scan)
}

func (s *Store) ListPlacementPolicies(project string) ([]PlacementPolicy, error) {
	rows, err := s.db.Query(
		`SELECT id,realm_id,project,slug,name,description,scope,strategy,
		        min_regions,min_zones,max_zones,preferred_regions_json,preferred_zones_json,
		        required_labels_json,anti_affinity_json,labels_json,created_at,updated_at
		 FROM placement_policies WHERE project=? ORDER BY slug`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlacementPolicy
	for rows.Next() {
		p, err := scanPlacementPolicy(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeletePlacementPolicy(project, slugOrID string) error {
	_, err := s.db.Exec(
		`DELETE FROM placement_policies WHERE (id=? OR slug=?) AND project=?`,
		slugOrID, slugOrID, project)
	return err
}

func scanPlacementPolicy(scan func(...any) error) (PlacementPolicy, error) {
	var p PlacementPolicy
	var prefRegions, prefZones, reqLabels, antiAff, labelsJSON string
	err := scan(&p.ID, &p.RealmID, &p.Project, &p.Slug, &p.Name, &p.Description,
		&p.Scope, &p.Strategy, &p.MinRegions, &p.MinZones, &p.MaxZones,
		&prefRegions, &prefZones, &reqLabels, &antiAff, &labelsJSON, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return p, ErrNotFound
	}
	p.PreferredRegions = unmarshalStringSlice(prefRegions)
	p.PreferredZones = unmarshalStringSlice(prefZones)
	p.RequiredLabels = unmarshalStringMap(reqLabels)
	p.AntiAffinity = unmarshalStringMap(antiAff)
	p.Labels = unmarshalStringMap(labelsJSON)
	return p, err
}

// ---- MigrationPlan CRUD -----------------------------------------------------

func (s *Store) InsertMigrationPlan(m MigrationPlan) error {
	if m.ID == "" {
		m.ID = genID("mig_")
	}
	n := now()
	if m.CreatedAt == "" {
		m.CreatedAt = n
	}
	m.UpdatedAt = n
	_, err := s.db.Exec(
		`INSERT INTO migration_plans (id,realm_id,project,name,migration_type,
		  source_region_id,target_region_id,source_zone_id,target_zone_id,
		  vpc_id,status,plan_json,result_json,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.RealmID, m.Project, m.Name, m.MigrationType,
		m.SourceRegionID, m.TargetRegionID, m.SourceZoneID, m.TargetZoneID,
		m.VPCID, orDefault(m.Status, "draft"),
		orDefault(m.PlanJSON, "{}"), orDefault(m.ResultJSON, "{}"),
		m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (s *Store) GetMigrationPlan(project, id string) (MigrationPlan, error) {
	row := s.db.QueryRow(
		`SELECT id,realm_id,project,name,migration_type,
		        source_region_id,target_region_id,source_zone_id,target_zone_id,
		        vpc_id,status,plan_json,result_json,created_at,updated_at
		 FROM migration_plans WHERE id=? AND project=?`, id, project)
	return scanMigrationPlan(row.Scan)
}

func (s *Store) ListMigrationPlans(project string) ([]MigrationPlan, error) {
	rows, err := s.db.Query(
		`SELECT id,realm_id,project,name,migration_type,
		        source_region_id,target_region_id,source_zone_id,target_zone_id,
		        vpc_id,status,plan_json,result_json,created_at,updated_at
		 FROM migration_plans WHERE project=? ORDER BY created_at DESC`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MigrationPlan
	for rows.Next() {
		m, err := scanMigrationPlan(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanMigrationPlan(scan func(...any) error) (MigrationPlan, error) {
	var m MigrationPlan
	err := scan(&m.ID, &m.RealmID, &m.Project, &m.Name, &m.MigrationType,
		&m.SourceRegionID, &m.TargetRegionID, &m.SourceZoneID, &m.TargetZoneID,
		&m.VPCID, &m.Status, &m.PlanJSON, &m.ResultJSON, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return m, ErrNotFound
	}
	return m, err
}

// ---- ServiceHealth CRUD -----------------------------------------------------

func (s *Store) UpsertServiceHealth(h ServiceHealth) error {
	if h.ID == "" {
		h.ID = genID("hlth_")
	}
	n := now()
	if h.CheckedAt == "" {
		h.CheckedAt = n
	}
	h.UpdatedAt = n
	_, err := s.db.Exec(
		`INSERT INTO service_health (id,scope,realm_id,region_id,zone_id,node_id,
		  service_name,status,message,checked_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET status=excluded.status, message=excluded.message,
		   checked_at=excluded.checked_at, updated_at=excluded.updated_at`,
		h.ID, h.Scope, h.RealmID, h.RegionID, h.ZoneID, h.NodeID,
		h.ServiceName, h.Status, h.Message, h.CheckedAt, h.UpdatedAt,
	)
	return err
}

func (s *Store) ListServiceHealth(scope, realmID, regionID, zoneID string) ([]ServiceHealth, error) {
	return s.listServiceHealth(scope, realmID, regionID, zoneID, "")
}

// ListServiceHealthByNode filters by node ID in addition to the standard fields.
func (s *Store) ListServiceHealthByNode(scope, realmID, regionID, zoneID, nodeID string) ([]ServiceHealth, error) {
	return s.listServiceHealth(scope, realmID, regionID, zoneID, nodeID)
}

func (s *Store) listServiceHealth(scope, realmID, regionID, zoneID, nodeID string) ([]ServiceHealth, error) {
	q := `SELECT id,scope,realm_id,region_id,zone_id,node_id,
	             service_name,status,message,checked_at,updated_at
	      FROM service_health WHERE 1=1`
	args := []any{}
	if scope != "" {
		q += " AND scope=?"
		args = append(args, scope)
	}
	if realmID != "" {
		q += " AND realm_id=?"
		args = append(args, realmID)
	}
	if regionID != "" {
		q += " AND region_id=?"
		args = append(args, regionID)
	}
	if zoneID != "" {
		q += " AND zone_id=?"
		args = append(args, zoneID)
	}
	if nodeID != "" {
		q += " AND node_id=?"
		args = append(args, nodeID)
	}
	q += " ORDER BY scope, service_name"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServiceHealth
	for rows.Next() {
		var h ServiceHealth
		if err := rows.Scan(&h.ID, &h.Scope, &h.RealmID, &h.RegionID, &h.ZoneID, &h.NodeID,
			&h.ServiceName, &h.Status, &h.Message, &h.CheckedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ---- Region status update ---------------------------------------------------

func (s *Store) UpdateRegionStatus(slugOrID, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE regions SET status=?, updated_at=? WHERE id=? OR slug=?`,
		status, now, slugOrID, slugOrID,
	)
	return err
}

// ---- ImageReplica CRUD ------------------------------------------------------

func (s *Store) InsertImageReplica(r ImageReplica) error {
	if r.ID == "" {
		r.ID = genID("imgr_")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO image_replicas
			(id,image_id,digest,realm_id,region_id,zone_id,node_id,status,size_bytes,location,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.ImageID, r.Digest, r.RealmID, r.RegionID, r.ZoneID, r.NodeID,
		r.Status, r.SizeBytes, r.Location, now, now,
	)
	return err
}

func (s *Store) ListImageReplicas(imageID string) ([]ImageReplica, error) {
	rows, err := s.db.Query(`
		SELECT id,image_id,digest,realm_id,region_id,zone_id,node_id,status,size_bytes,location,created_at,updated_at
		FROM image_replicas WHERE image_id=?`, imageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImageReplica
	for rows.Next() {
		var r ImageReplica
		if err := rows.Scan(&r.ID, &r.ImageID, &r.Digest, &r.RealmID, &r.RegionID, &r.ZoneID,
			&r.NodeID, &r.Status, &r.SizeBytes, &r.Location, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateImageReplicaStatus(id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE image_replicas SET status=?, updated_at=? WHERE id=?`, status, now, id)
	return err
}

// ---- StorageReplica CRUD ----------------------------------------------------

func (s *Store) InsertStorageReplica(r StorageReplica) error {
	if r.ID == "" {
		r.ID = genID("srep_")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO storage_replicas
			(id,bucket,object_key,etag,realm_id,region_id,zone_id,node_id,status,size_bytes,location,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Bucket, r.ObjectKey, r.ETag, r.RealmID, r.RegionID, r.ZoneID, r.NodeID,
		r.Status, r.SizeBytes, r.Location, now, now,
	)
	return err
}

func (s *Store) ListStorageReplicas(bucket, key string) ([]StorageReplica, error) {
	rows, err := s.db.Query(`
		SELECT id,bucket,object_key,etag,realm_id,region_id,zone_id,node_id,status,size_bytes,location,created_at,updated_at
		FROM storage_replicas WHERE bucket=? AND object_key=?`, bucket, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StorageReplica
	for rows.Next() {
		var r StorageReplica
		if err := rows.Scan(&r.ID, &r.Bucket, &r.ObjectKey, &r.ETag, &r.RealmID, &r.RegionID,
			&r.ZoneID, &r.NodeID, &r.Status, &r.SizeBytes, &r.Location, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- MigrationPlan extras ---------------------------------------------------

// ListAllMigrationPlans returns plans across all projects (used by the reconciler).
func (s *Store) ListAllMigrationPlans() ([]MigrationPlan, error) {
	rows, err := s.db.Query(`
		SELECT id,realm_id,project,name,migration_type,source_region_id,target_region_id,
		       source_zone_id,target_zone_id,vpc_id,status,plan_json,result_json,created_at,updated_at
		FROM migration_plans ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MigrationPlan
	for rows.Next() {
		var m MigrationPlan
		if err := rows.Scan(&m.ID, &m.RealmID, &m.Project, &m.Name, &m.MigrationType,
			&m.SourceRegionID, &m.TargetRegionID, &m.SourceZoneID, &m.TargetZoneID, &m.VPCID,
			&m.Status, &m.PlanJSON, &m.ResultJSON, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) UpdateMigrationPlanStatus(id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE migration_plans SET status=?, updated_at=? WHERE id=?`, status, now, id)
	return err
}

// ---- utility ----------------------------------------------------------------

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ---- Node roles -------------------------------------------------------------

func (s *Store) SetNodeRoles(nodeID string, roles []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM node_roles WHERE node_id=?`, nodeID); err != nil {
		return err
	}
	for _, r := range roles {
		if _, err := tx.Exec(`INSERT INTO node_roles (node_id,role) VALUES (?,?)`, nodeID, r); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetNodeRoles(nodeID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT role FROM node_roles WHERE node_id=? ORDER BY role`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- Node taints ------------------------------------------------------------

func (s *Store) SetNodeTaints(nodeID string, taints []NodeTaint) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM node_taints WHERE node_id=?`, nodeID); err != nil {
		return err
	}
	for _, t := range taints {
		if _, err := tx.Exec(
			`INSERT INTO node_taints (node_id,key,value,effect) VALUES (?,?,?,?)`,
			nodeID, t.Key, t.Value, t.Effect,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetNodeTaints(nodeID string) ([]NodeTaint, error) {
	rows, err := s.db.Query(`SELECT key,value,effect FROM node_taints WHERE node_id=?`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NodeTaint
	for rows.Next() {
		var t NodeTaint
		if err := rows.Scan(&t.Key, &t.Value, &t.Effect); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---- Node labels ------------------------------------------------------------

func (s *Store) SetNodeLabels(nodeID string, labels map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM node_labels WHERE node_id=?`, nodeID); err != nil {
		return err
	}
	for k, v := range labels {
		if _, err := tx.Exec(
			`INSERT INTO node_labels (node_id,key,value) VALUES (?,?,?)`,
			nodeID, k, v,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetNodeLabels(nodeID string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key,value FROM node_labels WHERE node_id=?`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// ---- Node services ----------------------------------------------------------

func (s *Store) UpsertNodeService(svc NodeService) error {
	if svc.LastSeen == "" {
		svc.LastSeen = now()
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO node_services
		 (node_id,service_name,desired_state,actual_state,version,health,message,last_seen)
		 VALUES (?,?,?,?,?,?,?,?)`,
		svc.NodeID, svc.ServiceName, svc.DesiredState, svc.ActualState,
		svc.Version, svc.Health, svc.Message, svc.LastSeen,
	)
	return err
}

func (s *Store) ListNodeServices(nodeID string) ([]NodeService, error) {
	rows, err := s.db.Query(
		`SELECT node_id,service_name,desired_state,actual_state,version,health,message,last_seen
		 FROM node_services WHERE node_id=?`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NodeService
	for rows.Next() {
		var svc NodeService
		if err := rows.Scan(&svc.NodeID, &svc.ServiceName, &svc.DesiredState, &svc.ActualState,
			&svc.Version, &svc.Health, &svc.Message, &svc.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, svc)
	}
	return out, rows.Err()
}

// ---- Node heartbeats --------------------------------------------------------

func (s *Store) UpsertHeartbeat(hb NodeHeartbeat) error {
	if hb.SeenAt == "" {
		hb.SeenAt = now()
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO node_heartbeats
		 (node_id,status,cpu_used,memory_used_bytes,disk_used_bytes,gpu_used,message,seen_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		hb.NodeID, hb.Status, hb.CPUUsed, hb.MemoryUsedBytes,
		hb.DiskUsedBytes, hb.GPUUsed, hb.Message, hb.SeenAt,
	)
	return err
}

func (s *Store) GetHeartbeat(nodeID string) (NodeHeartbeat, error) {
	row := s.db.QueryRow(
		`SELECT node_id,status,cpu_used,memory_used_bytes,disk_used_bytes,gpu_used,message,seen_at
		 FROM node_heartbeats WHERE node_id=?`, nodeID)
	var hb NodeHeartbeat
	err := row.Scan(&hb.NodeID, &hb.Status, &hb.CPUUsed, &hb.MemoryUsedBytes,
		&hb.DiskUsedBytes, &hb.GPUUsed, &hb.Message, &hb.SeenAt)
	if err == sql.ErrNoRows {
		return hb, ErrNotFound
	}
	return hb, err
}

func (s *Store) ListStaleNodes(olderThan time.Duration) ([]Node, error) {
	threshold := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	rows, err := s.db.Query(
		`SELECT id,realm_id,region_id,zone_id,slug,name,address,status,
		        failure_domain,labels_json,cpu_count,memory_bytes,disk_bytes,
		        gpu_count,gpu_memory_bytes,agent_version,last_heartbeat,cordoned,
		        created_at,updated_at
		 FROM nodes WHERE last_heartbeat != '' AND last_heartbeat < ?`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		n, err := scanNode(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ---- Node pools -------------------------------------------------------------

func (s *Store) CreatePool(p NodePool) (NodePool, error) {
	if p.ID == "" {
		p.ID = genID("pool_")
	}
	ts := now()
	if p.CreatedAt == "" {
		p.CreatedAt = ts
	}
	p.UpdatedAt = ts
	_, err := s.db.Exec(
		`INSERT INTO node_pools (id,name,realm_id,region_id,zone_id,status,
		  min_nodes,desired_nodes,max_nodes,placement_policy,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.RealmID, p.RegionID, p.ZoneID,
		orDefault(p.Status, "active"),
		p.MinNodes, p.DesiredNodes, p.MaxNodes,
		orDefault(p.PlacementPolicy, "spread"),
		p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return p, err
	}
	if err := s.setPoolRoles(p.ID, p.Roles); err != nil {
		return p, err
	}
	return s.GetPool(p.ID)
}

func (s *Store) GetPool(nameOrID string) (NodePool, error) {
	row := s.db.QueryRow(
		`SELECT id,name,realm_id,region_id,zone_id,status,
		        min_nodes,desired_nodes,max_nodes,placement_policy,created_at,updated_at
		 FROM node_pools WHERE id=? OR name=?`, nameOrID, nameOrID)
	p, err := scanPool(row.Scan)
	if err != nil {
		return p, err
	}
	p.Roles, _ = s.getPoolRoles(p.ID)
	p.MemberCount = s.poolMemberCount(p.ID)
	return p, nil
}

func (s *Store) ListPools() ([]NodePool, error) {
	rows, err := s.db.Query(
		`SELECT id,name,realm_id,region_id,zone_id,status,
		        min_nodes,desired_nodes,max_nodes,placement_policy,created_at,updated_at
		 FROM node_pools ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NodePool
	for rows.Next() {
		p, err := scanPool(rows.Scan)
		if err != nil {
			return nil, err
		}
		p.Roles, _ = s.getPoolRoles(p.ID)
		p.MemberCount = s.poolMemberCount(p.ID)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdatePool(id string, updates map[string]any) error {
	allowed := map[string]string{
		"name": "name", "status": "status",
		"desiredNodes": "desired_nodes", "minNodes": "min_nodes", "maxNodes": "max_nodes",
		"placementPolicy": "placement_policy",
	}
	parts := []string{}
	args := []any{}
	for k, col := range allowed {
		if v, ok := updates[k]; ok {
			parts = append(parts, col+"=?")
			args = append(args, v)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	parts = append(parts, "updated_at=?")
	args = append(args, now())
	args = append(args, id)
	_, err := s.db.Exec(
		`UPDATE node_pools SET `+strings.Join(parts, ",")+` WHERE id=?`,
		args...,
	)
	return err
}

func (s *Store) DeletePool(nameOrID string) error {
	_, err := s.db.Exec(`DELETE FROM node_pools WHERE id=? OR name=?`, nameOrID, nameOrID)
	return err
}

func (s *Store) AddPoolMember(poolID, nodeID string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO node_pool_members (pool_id,node_id) VALUES (?,?)`, poolID, nodeID)
	return err
}

func (s *Store) RemovePoolMember(poolID, nodeID string) error {
	_, err := s.db.Exec(
		`DELETE FROM node_pool_members WHERE pool_id=? AND node_id=?`, poolID, nodeID)
	return err
}

func (s *Store) ListPoolMembers(poolID string) ([]Node, error) {
	rows, err := s.db.Query(
		`SELECT n.id,n.realm_id,n.region_id,n.zone_id,n.slug,n.name,n.address,n.status,
		        n.failure_domain,n.labels_json,n.cpu_count,n.memory_bytes,n.disk_bytes,
		        n.gpu_count,n.gpu_memory_bytes,n.agent_version,n.last_heartbeat,n.cordoned,
		        n.created_at,n.updated_at
		 FROM nodes n
		 JOIN node_pool_members m ON m.node_id=n.id
		 WHERE m.pool_id=? ORDER BY n.slug`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		n, err := scanNode(rows.Scan)
		if err != nil {
			return nil, err
		}
		n.Roles, _ = s.GetNodeRoles(n.ID)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) setPoolRoles(poolID string, roles []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM node_pool_roles WHERE pool_id=?`, poolID); err != nil {
		return err
	}
	for _, r := range roles {
		if _, err := tx.Exec(`INSERT INTO node_pool_roles (pool_id,role) VALUES (?,?)`, poolID, r); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) getPoolRoles(poolID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT role FROM node_pool_roles WHERE pool_id=? ORDER BY role`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) poolMemberCount(poolID string) int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM node_pool_members WHERE pool_id=?`, poolID).Scan(&n)
	return n
}

func scanPool(scan func(...any) error) (NodePool, error) {
	var p NodePool
	err := scan(&p.ID, &p.Name, &p.RealmID, &p.RegionID, &p.ZoneID, &p.Status,
		&p.MinNodes, &p.DesiredNodes, &p.MaxNodes, &p.PlacementPolicy, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return p, ErrNotFound
	}
	return p, err
}

// ---- Join tokens ------------------------------------------------------------

func (s *Store) CreateJoinToken(t JoinToken) (JoinToken, error) {
	if t.ID == "" {
		t.ID = genID("jt_")
	}
	if t.Token == "" {
		tok, err := randTokenHex(32)
		if err != nil {
			return t, err
		}
		t.Token = tok
	}
	if t.CreatedAt == "" {
		t.CreatedAt = now()
	}
	rolesJSON := marshalJSON(t.Roles)
	_, err := s.db.Exec(
		`INSERT INTO join_tokens (id,token,realm_id,region_id,zone_id,roles,uses_left,expires_at,created_by,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Token, t.RealmID, t.RegionID, t.ZoneID, rolesJSON,
		t.UsesLeft, t.ExpiresAt, t.CreatedBy, t.CreatedAt,
	)
	if err != nil {
		return t, err
	}
	return s.getJoinTokenByID(t.ID)
}

func (s *Store) GetJoinToken(token string) (JoinToken, error) {
	row := s.db.QueryRow(
		`SELECT id,token,realm_id,region_id,zone_id,roles,uses_left,expires_at,created_by,created_at
		 FROM join_tokens WHERE token=?`, token)
	return scanJoinToken(row.Scan)
}

func (s *Store) getJoinTokenByID(id string) (JoinToken, error) {
	row := s.db.QueryRow(
		`SELECT id,token,realm_id,region_id,zone_id,roles,uses_left,expires_at,created_by,created_at
		 FROM join_tokens WHERE id=?`, id)
	return scanJoinToken(row.Scan)
}

func (s *Store) ConsumeJoinToken(token string) (JoinToken, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return JoinToken{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	row := tx.QueryRow(
		`SELECT id,token,realm_id,region_id,zone_id,roles,uses_left,expires_at,created_by,created_at
		 FROM join_tokens WHERE token=?`, token)
	jt, err := scanJoinToken(row.Scan)
	if err != nil {
		return jt, err
	}
	if jt.UsesLeft <= 0 {
		return jt, fmt.Errorf("join token exhausted")
	}
	if jt.ExpiresAt != "" && jt.ExpiresAt < now() {
		return jt, fmt.Errorf("join token expired")
	}
	if _, err := tx.Exec(`UPDATE join_tokens SET uses_left=uses_left-1 WHERE token=?`, token); err != nil {
		return jt, err
	}
	jt.UsesLeft--
	return jt, tx.Commit()
}

func (s *Store) ListJoinTokens() ([]JoinToken, error) {
	rows, err := s.db.Query(
		`SELECT id,token,realm_id,region_id,zone_id,roles,uses_left,expires_at,created_by,created_at
		 FROM join_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JoinToken
	for rows.Next() {
		jt, err := scanJoinToken(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, jt)
	}
	return out, rows.Err()
}

func (s *Store) DeleteJoinToken(id string) error {
	_, err := s.db.Exec(`DELETE FROM join_tokens WHERE id=?`, id)
	return err
}

func scanJoinToken(scan func(...any) error) (JoinToken, error) {
	var jt JoinToken
	var rolesJSON string
	err := scan(&jt.ID, &jt.Token, &jt.RealmID, &jt.RegionID, &jt.ZoneID,
		&rolesJSON, &jt.UsesLeft, &jt.ExpiresAt, &jt.CreatedBy, &jt.CreatedAt)
	if err == sql.ErrNoRows {
		return jt, ErrNotFound
	}
	jt.Roles = unmarshalStringSlice(rolesJSON)
	return jt, err
}

func randTokenHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := cryptorand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
