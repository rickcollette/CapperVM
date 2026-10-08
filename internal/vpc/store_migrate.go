package vpc

import (
	"database/sql"
	"fmt"
	"strings"
)

// MigrateSchema applies additive schema changes for the unified VPC model.
func MigrateSchema(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE capvpc_vpcs ADD COLUMN realm_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN slug TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN status TEXT NOT NULL DEFAULT 'available'`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN home_region_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN mobility_policy TEXT NOT NULL DEFAULT 'disabled'`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN labels_json TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN dns_support INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN dns_hostnames INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN default_sg_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN default_acl_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN main_rt_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN enable_flow_logs INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE capvpc_vpcs ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_subnets ADD COLUMN realm_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_subnets ADD COLUMN region_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_subnets ADD COLUMN zone_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_subnets ADD COLUMN slug TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_subnets ADD COLUMN route_table_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_subnets ADD COLUMN network_acl_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_subnets ADD COLUMN auto_public_ip INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE capvpc_subnets ADD COLUMN status TEXT NOT NULL DEFAULT 'available'`,
		`ALTER TABLE capvpc_subnets ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_route_tables ADD COLUMN is_main INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE capvpc_routes ADD COLUMN origin TEXT NOT NULL DEFAULT 'static'`,
		`ALTER TABLE capvpc_routes ADD COLUMN state TEXT NOT NULL DEFAULT 'active'`,
		`ALTER TABLE capvpc_internet_gateways ADD COLUMN status TEXT NOT NULL DEFAULT 'attached'`,
		`ALTER TABLE capvpc_nat_gateways ADD COLUMN connectivity_type TEXT NOT NULL DEFAULT 'public'`,
		`ALTER TABLE capvpc_nat_gateways ADD COLUMN private_ip TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE capvpc_nat_gateways ADD COLUMN status TEXT NOT NULL DEFAULT 'available'`,
		`ALTER TABLE capvpc_security_groups ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS capvpc_network_acls (
			id         TEXT PRIMARY KEY,
			vpc_id     TEXT NOT NULL REFERENCES capvpc_vpcs(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			UNIQUE(vpc_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_network_acl_entries (
			id             TEXT PRIMARY KEY,
			network_acl_id TEXT NOT NULL REFERENCES capvpc_network_acls(id) ON DELETE CASCADE,
			rule_number    INTEGER NOT NULL,
			direction      TEXT NOT NULL,
			action         TEXT NOT NULL,
			protocol       TEXT NOT NULL,
			cidr           TEXT NOT NULL,
			from_port      INTEGER NOT NULL DEFAULT 0,
			to_port        INTEGER NOT NULL DEFAULT 0,
			UNIQUE(network_acl_id, direction, rule_number)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_subnet_acl_assoc (
			subnet_id      TEXT NOT NULL REFERENCES capvpc_subnets(id) ON DELETE CASCADE,
			network_acl_id TEXT NOT NULL REFERENCES capvpc_network_acls(id) ON DELETE CASCADE,
			PRIMARY KEY (subnet_id, network_acl_id)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_enis (
			id                  TEXT PRIMARY KEY,
			vpc_id              TEXT NOT NULL,
			subnet_id           TEXT NOT NULL,
			zone_id             TEXT NOT NULL DEFAULT '',
			instance_id         TEXT NOT NULL DEFAULT '',
			attachment_index    INTEGER NOT NULL DEFAULT 0,
			mac_address         TEXT NOT NULL DEFAULT '',
			source_dest_check   INTEGER NOT NULL DEFAULT 1,
			delete_on_termination INTEGER NOT NULL DEFAULT 1,
			status              TEXT NOT NULL DEFAULT 'available',
			created_at          TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_eni_private_ips (
			id         TEXT PRIMARY KEY,
			eni_id     TEXT NOT NULL REFERENCES capvpc_enis(id) ON DELETE CASCADE,
			address    TEXT NOT NULL,
			is_primary INTEGER NOT NULL DEFAULT 0,
			status     TEXT NOT NULL DEFAULT 'assigned'
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_eni_security_groups (
			eni_id            TEXT NOT NULL REFERENCES capvpc_enis(id) ON DELETE CASCADE,
			security_group_id TEXT NOT NULL REFERENCES capvpc_security_groups(id) ON DELETE CASCADE,
			PRIMARY KEY (eni_id, security_group_id)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_key_pairs (
			id          TEXT PRIMARY KEY,
			project     TEXT NOT NULL,
			name        TEXT NOT NULL,
			public_key  TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			key_type    TEXT NOT NULL,
			created_at  TEXT NOT NULL,
			UNIQUE(project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_launch_templates (
			id              TEXT PRIMARY KEY,
			project         TEXT NOT NULL,
			name            TEXT NOT NULL,
			default_version INTEGER NOT NULL DEFAULT 1,
			latest_version  INTEGER NOT NULL DEFAULT 1,
			created_at      TEXT NOT NULL,
			UNIQUE(project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_launch_template_versions (
			id          TEXT PRIMARY KEY,
			template_id TEXT NOT NULL REFERENCES capvpc_launch_templates(id) ON DELETE CASCADE,
			version     INTEGER NOT NULL,
			config_json TEXT NOT NULL,
			created_at  TEXT NOT NULL,
			UNIQUE(template_id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_endpoints (
			id              TEXT PRIMARY KEY,
			vpc_id          TEXT NOT NULL REFERENCES capvpc_vpcs(id) ON DELETE CASCADE,
			name            TEXT NOT NULL,
			service_name    TEXT NOT NULL,
			endpoint_type   TEXT NOT NULL DEFAULT 'gateway',
			subnet_ids_json TEXT NOT NULL DEFAULT '[]',
			status          TEXT NOT NULL DEFAULT 'available',
			created_at      TEXT NOT NULL,
			UNIQUE(vpc_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_peerings (
			id               TEXT PRIMARY KEY,
			requester_vpc_id TEXT NOT NULL,
			accepter_vpc_id  TEXT NOT NULL,
			status           TEXT NOT NULL DEFAULT 'pending-acceptance',
			created_at       TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_flow_logs (
			id            TEXT PRIMARY KEY,
			resource_type TEXT NOT NULL,
			resource_id   TEXT NOT NULL,
			destination   TEXT NOT NULL DEFAULT 'file',
			status        TEXT NOT NULL DEFAULT 'active',
			created_at    TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_instance_metadata_options (
			instance_id  TEXT PRIMARY KEY,
			http_tokens  TEXT NOT NULL DEFAULT 'optional',
			http_endpoint TEXT NOT NULL DEFAULT 'enabled',
			hop_limit    INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_instance_block_devices (
			id          TEXT PRIMARY KEY,
			instance_id TEXT NOT NULL,
			device_name TEXT NOT NULL,
			volume_id   TEXT NOT NULL DEFAULT '',
			size_bytes  INTEGER NOT NULL DEFAULT 0,
			delete_on_termination INTEGER NOT NULL DEFAULT 1
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
			id              TEXT PRIMARY KEY,
			load_balancer_id TEXT NOT NULL,
			target_group_id  TEXT NOT NULL,
			protocol         TEXT NOT NULL DEFAULT 'tcp',
			port             INTEGER NOT NULL DEFAULT 80,
			created_at       TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS dns_zone_vpc_assoc (
			zone_id TEXT NOT NULL,
			vpc_id  TEXT NOT NULL,
			PRIMARY KEY (zone_id, vpc_id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			if !isDuplicateColumn(err) {
				return fmt.Errorf("vpc.MigrateSchema: %w", err)
			}
		}
	}
	return nil
}

func isDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}
