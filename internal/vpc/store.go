package vpc

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Store provides SQLite-backed persistence for all VPC objects.
// Tables use the capvpc_ prefix to avoid conflicts with topology's vpcs table.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store backed by db.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// InitSchema creates all VPC tables idempotently.
func InitSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS capvpc_vpcs (
			id         TEXT PRIMARY KEY,
			project    TEXT NOT NULL,
			name       TEXT NOT NULL,
			cidr       TEXT NOT NULL,
			dns_domain TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_subnets (
			id          TEXT PRIMARY KEY,
			vpc_id      TEXT NOT NULL REFERENCES capvpc_vpcs(id) ON DELETE CASCADE,
			name        TEXT NOT NULL,
			cidr        TEXT NOT NULL,
			zone        TEXT NOT NULL DEFAULT '',
			kind        TEXT NOT NULL DEFAULT 'private',
			bridge_name TEXT NOT NULL DEFAULT '',
			gateway_ip  TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			UNIQUE(vpc_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_route_tables (
			id         TEXT PRIMARY KEY,
			vpc_id     TEXT NOT NULL REFERENCES capvpc_vpcs(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(vpc_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_subnet_rt_assoc (
			subnet_id      TEXT NOT NULL REFERENCES capvpc_subnets(id) ON DELETE CASCADE,
			route_table_id TEXT NOT NULL REFERENCES capvpc_route_tables(id) ON DELETE CASCADE,
			PRIMARY KEY (subnet_id, route_table_id)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_routes (
			id               TEXT PRIMARY KEY,
			route_table_id   TEXT NOT NULL REFERENCES capvpc_route_tables(id) ON DELETE CASCADE,
			destination_cidr TEXT NOT NULL,
			target_type      TEXT NOT NULL,
			target_id        TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_security_groups (
			id           TEXT PRIMARY KEY,
			vpc_id       TEXT NOT NULL REFERENCES capvpc_vpcs(id) ON DELETE CASCADE,
			name         TEXT NOT NULL,
			description  TEXT NOT NULL DEFAULT '',
			default_deny INTEGER NOT NULL DEFAULT 1,
			created_at   TEXT NOT NULL,
			UNIQUE(vpc_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_sg_rules (
			id                TEXT PRIMARY KEY,
			security_group_id TEXT NOT NULL REFERENCES capvpc_security_groups(id) ON DELETE CASCADE,
			direction         TEXT NOT NULL,
			protocol          TEXT NOT NULL,
			from_port         INTEGER NOT NULL DEFAULT 0,
			to_port           INTEGER NOT NULL DEFAULT 0,
			cidr              TEXT NOT NULL DEFAULT '',
			source_sg_id      TEXT NOT NULL DEFAULT '',
			action            TEXT NOT NULL DEFAULT 'allow'
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_internet_gateways (
			id         TEXT PRIMARY KEY,
			vpc_id     TEXT NOT NULL REFERENCES capvpc_vpcs(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(vpc_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS capvpc_nat_gateways (
			id         TEXT PRIMARY KEY,
			vpc_id     TEXT NOT NULL REFERENCES capvpc_vpcs(id) ON DELETE CASCADE,
			subnet_id  TEXT NOT NULL,
			name       TEXT NOT NULL,
			public_ip  TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(vpc_id, name)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("vpc.InitSchema: %w", err)
		}
	}
	return MigrateSchema(db)
}

// ---- VPC CRUD ---------------------------------------------------------------

func (s *Store) InsertVPC(v VPC) error {
	labels, _ := json.Marshal(v.Labels)
	if v.PrimaryIPv4CIDR == "" {
		v.PrimaryIPv4CIDR = v.CIDR
	}
	if v.CIDR == "" {
		v.CIDR = v.PrimaryIPv4CIDR
	}
	if v.Status == "" {
		v.Status = VPCStatusAvailable
	}
	if v.UpdatedAt == "" {
		v.UpdatedAt = v.CreatedAt
	}
	dnsSupport, dnsHostnames, flowLogs := 1, 1, 0
	if !v.DNSSupport {
		dnsSupport = 0
	}
	if !v.DNSHostnames {
		dnsHostnames = 0
	}
	if v.EnableFlowLogs {
		flowLogs = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO capvpc_vpcs (
			id, project, name, cidr, dns_domain, created_at,
			realm_id, slug, description, status, home_region_id, mobility_policy, labels_json,
			dns_support, dns_hostnames, default_sg_id, default_acl_id, main_rt_id, enable_flow_logs, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.Project, v.Name, v.CIDR, v.DNSDomain, v.CreatedAt,
		v.RealmID, v.Slug, v.Description, v.Status, v.HomeRegionID, v.MobilityPolicy, string(labels),
		dnsSupport, dnsHostnames, v.DefaultSecurityGroupID, v.DefaultNetworkACLID, v.MainRouteTableID, flowLogs, v.UpdatedAt,
	)
	return err
}

func (s *Store) GetVPC(nameOrID, project string) (VPC, error) {
	var v VPC
	var labelsJSON string
	var dnsSupport, dnsHostnames, flowLogs int
	var err error
	q := `SELECT id, project, name, cidr, dns_domain, created_at,
		COALESCE(realm_id,''), COALESCE(slug,''), COALESCE(description,''), COALESCE(status,'available'),
		COALESCE(home_region_id,''), COALESCE(mobility_policy,'disabled'), COALESCE(labels_json,'{}'),
		COALESCE(dns_support,1), COALESCE(dns_hostnames,1),
		COALESCE(default_sg_id,''), COALESCE(default_acl_id,''), COALESCE(main_rt_id,''),
		COALESCE(enable_flow_logs,0), COALESCE(updated_at,'')
		FROM capvpc_vpcs WHERE `
	if project != "" {
		err = s.db.QueryRow(q+`(id=? OR name=? OR slug=?) AND project=?`, nameOrID, nameOrID, nameOrID, project).Scan(
			&v.ID, &v.Project, &v.Name, &v.CIDR, &v.DNSDomain, &v.CreatedAt,
			&v.RealmID, &v.Slug, &v.Description, &v.Status, &v.HomeRegionID, &v.MobilityPolicy, &labelsJSON,
			&dnsSupport, &dnsHostnames, &v.DefaultSecurityGroupID, &v.DefaultNetworkACLID, &v.MainRouteTableID, &flowLogs, &v.UpdatedAt,
		)
	} else {
		err = s.db.QueryRow(q+`id=? OR name=? OR slug=?`, nameOrID, nameOrID, nameOrID).Scan(
			&v.ID, &v.Project, &v.Name, &v.CIDR, &v.DNSDomain, &v.CreatedAt,
			&v.RealmID, &v.Slug, &v.Description, &v.Status, &v.HomeRegionID, &v.MobilityPolicy, &labelsJSON,
			&dnsSupport, &dnsHostnames, &v.DefaultSecurityGroupID, &v.DefaultNetworkACLID, &v.MainRouteTableID, &flowLogs, &v.UpdatedAt,
		)
	}
	if err == sql.ErrNoRows {
		return v, fmt.Errorf("vpc %q not found", nameOrID)
	}
	if err != nil {
		return v, err
	}
	_ = json.Unmarshal([]byte(labelsJSON), &v.Labels)
	v.PrimaryIPv4CIDR = v.CIDR
	v.DNSSupport = dnsSupport == 1
	v.DNSHostnames = dnsHostnames == 1
	v.EnableFlowLogs = flowLogs == 1
	return v, nil
}

func (s *Store) ListVPCs(project string) ([]VPC, error) {
	rows, err := s.db.Query(
		`SELECT id, project, name, cidr, dns_domain, created_at,
			COALESCE(realm_id,''), COALESCE(slug,''), COALESCE(description,''), COALESCE(status,'available'),
			COALESCE(home_region_id,''), COALESCE(mobility_policy,'disabled'), COALESCE(labels_json,'{}'),
			COALESCE(dns_support,1), COALESCE(dns_hostnames,1),
			COALESCE(default_sg_id,''), COALESCE(default_acl_id,''), COALESCE(main_rt_id,''),
			COALESCE(enable_flow_logs,0), COALESCE(updated_at,'')
		FROM capvpc_vpcs WHERE project=? ORDER BY name`, project,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VPC
	for rows.Next() {
		var v VPC
		var labelsJSON string
		var dnsSupport, dnsHostnames, flowLogs int
		if err := rows.Scan(
			&v.ID, &v.Project, &v.Name, &v.CIDR, &v.DNSDomain, &v.CreatedAt,
			&v.RealmID, &v.Slug, &v.Description, &v.Status, &v.HomeRegionID, &v.MobilityPolicy, &labelsJSON,
			&dnsSupport, &dnsHostnames, &v.DefaultSecurityGroupID, &v.DefaultNetworkACLID, &v.MainRouteTableID, &flowLogs, &v.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(labelsJSON), &v.Labels)
		v.PrimaryIPv4CIDR = v.CIDR
		v.DNSSupport = dnsSupport == 1
		v.DNSHostnames = dnsHostnames == 1
		v.EnableFlowLogs = flowLogs == 1
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) DeleteVPC(nameOrID, project string) error {
	// The child tables declare ON DELETE CASCADE, but SQLite foreign-key
	// enforcement is not enabled on the shared connection, so cascade explicitly
	// in dependency order to avoid orphaning subnets, routes, SGs, gateways, etc.
	v, gerr := s.GetVPC(nameOrID, project)
	if gerr == nil {
		id := v.ID
		stmts := []struct {
			q    string
			args []any
		}{
			{`DELETE FROM capvpc_eni_security_groups WHERE eni_id IN (SELECT id FROM capvpc_enis WHERE vpc_id=?)`, []any{id}},
			{`DELETE FROM capvpc_eni_private_ips WHERE eni_id IN (SELECT id FROM capvpc_enis WHERE vpc_id=?)`, []any{id}},
			{`DELETE FROM capvpc_enis WHERE vpc_id=?`, []any{id}},
			{`DELETE FROM capvpc_network_acl_entries WHERE network_acl_id IN (SELECT id FROM capvpc_network_acls WHERE vpc_id=?)`, []any{id}},
			{`DELETE FROM capvpc_subnet_acl_assoc WHERE subnet_id IN (SELECT id FROM capvpc_subnets WHERE vpc_id=?)`, []any{id}},
			{`DELETE FROM capvpc_network_acls WHERE vpc_id=?`, []any{id}},
			{`DELETE FROM capvpc_routes WHERE route_table_id IN (SELECT id FROM capvpc_route_tables WHERE vpc_id=?)`, []any{id}},
			{`DELETE FROM capvpc_subnet_rt_assoc WHERE subnet_id IN (SELECT id FROM capvpc_subnets WHERE vpc_id=?)`, []any{id}},
			{`DELETE FROM capvpc_sg_rules WHERE security_group_id IN (SELECT id FROM capvpc_security_groups WHERE vpc_id=?)`, []any{id}},
			{`DELETE FROM capvpc_route_tables WHERE vpc_id=?`, []any{id}},
			{`DELETE FROM capvpc_security_groups WHERE vpc_id=?`, []any{id}},
			{`DELETE FROM capvpc_nat_gateways WHERE vpc_id=?`, []any{id}},
			{`DELETE FROM capvpc_internet_gateways WHERE vpc_id=?`, []any{id}},
			{`DELETE FROM capvpc_endpoints WHERE vpc_id=?`, []any{id}},
			{`DELETE FROM capvpc_peerings WHERE requester_vpc_id=? OR accepter_vpc_id=?`, []any{id, id}},
			{`DELETE FROM capvpc_flow_logs WHERE resource_id=? OR resource_id IN (SELECT id FROM capvpc_subnets WHERE vpc_id=?)`, []any{id, id}},
			{`DELETE FROM capvpc_subnets WHERE vpc_id=?`, []any{id}},
		}
		for _, st := range stmts {
			if _, err := s.db.Exec(st.q, st.args...); err != nil {
				return err
			}
		}
	}
	_, err := s.db.Exec(`DELETE FROM capvpc_vpcs WHERE (id=? OR name=?) AND project=?`, nameOrID, nameOrID, project)
	return err
}

// ---- Subnet CRUD (extended in store_extended.go) ----------------------------

func (s *Store) InsertSubnet(sub Subnet) error {
	kind := string(sub.Kind)
	if kind == "" {
		kind = string(sub.SubnetType)
	}
	if sub.SubnetType == "" {
		sub.SubnetType = SubnetKind(kind)
	}
	autoPublic := 0
	if sub.AutoAssignPublicIP {
		autoPublic = 1
	}
	if sub.Status == "" {
		sub.Status = "available"
	}
	if sub.UpdatedAt == "" {
		sub.UpdatedAt = sub.CreatedAt
	}
	if sub.ZoneID == "" {
		sub.ZoneID = sub.Zone
	}
	if sub.Zone == "" {
		sub.Zone = sub.ZoneID
	}
	_, err := s.db.Exec(
		`INSERT INTO capvpc_subnets (
			id, vpc_id, name, cidr, zone, kind, bridge_name, gateway_ip, created_at,
			realm_id, region_id, zone_id, slug, route_table_id, network_acl_id, auto_public_ip, status, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.VPCID, sub.Name, sub.CIDR, sub.Zone, kind, sub.BridgeName, sub.GatewayIP, sub.CreatedAt,
		sub.RealmID, sub.RegionID, sub.ZoneID, sub.Slug, sub.RouteTableID, sub.NetworkACLID, autoPublic, sub.Status, sub.UpdatedAt,
	)
	return err
}

func (s *Store) DeleteSubnet(nameOrID, vpcID string) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_subnets WHERE (id=? OR name=?) AND vpc_id=?`, nameOrID, nameOrID, vpcID)
	return err
}

// ---- RouteTable CRUD --------------------------------------------------------

func (s *Store) InsertRouteTable(rt RouteTable) error {
	isMain := 0
	if rt.IsMain {
		isMain = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO capvpc_route_tables (id, vpc_id, name, is_main, created_at) VALUES (?, ?, ?, ?, ?)`,
		rt.ID, rt.VPCID, rt.Name, isMain, rt.CreatedAt,
	)
	return err
}

func (s *Store) SetRouteTableMain(id string, isMain bool) error {
	v := 0
	if isMain {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE capvpc_route_tables SET is_main=? WHERE id=?`, v, id)
	return err
}

func (s *Store) GetRouteTableByID(id string) (RouteTable, error) {
	var rt RouteTable
	var isMain int
	err := s.db.QueryRow(
		`SELECT id, vpc_id, name, COALESCE(is_main,0), created_at FROM capvpc_route_tables WHERE id=?`, id,
	).Scan(&rt.ID, &rt.VPCID, &rt.Name, &isMain, &rt.CreatedAt)
	if err == sql.ErrNoRows {
		return rt, fmt.Errorf("route table %q not found", id)
	}
	rt.IsMain = isMain == 1
	return rt, err
}

func (s *Store) GetRouteTable(nameOrID, vpcID string) (RouteTable, error) {
	var rt RouteTable
	var isMain int
	err := s.db.QueryRow(
		`SELECT id, vpc_id, name, COALESCE(is_main,0), created_at FROM capvpc_route_tables WHERE (id=? OR name=?) AND vpc_id=?`,
		nameOrID, nameOrID, vpcID,
	).Scan(&rt.ID, &rt.VPCID, &rt.Name, &isMain, &rt.CreatedAt)
	if err == sql.ErrNoRows {
		return rt, fmt.Errorf("route table %q not found", nameOrID)
	}
	rt.IsMain = isMain == 1
	return rt, err
}

func (s *Store) ListRouteTables(vpcID string) ([]RouteTable, error) {
	rows, err := s.db.Query(
		`SELECT id, vpc_id, name, COALESCE(is_main,0), created_at FROM capvpc_route_tables WHERE vpc_id=? ORDER BY name`, vpcID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RouteTable
	for rows.Next() {
		var rt RouteTable
		var isMain int
		if err := rows.Scan(&rt.ID, &rt.VPCID, &rt.Name, &isMain, &rt.CreatedAt); err != nil {
			return nil, err
		}
		rt.IsMain = isMain == 1
		out = append(out, rt)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRouteTable(nameOrID, vpcID string) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_route_tables WHERE (id=? OR name=?) AND vpc_id=?`, nameOrID, nameOrID, vpcID)
	return err
}

// ---- Route CRUD -------------------------------------------------------------

func (s *Store) InsertRoute(r Route) error {
	_, err := s.db.Exec(
		`INSERT INTO capvpc_routes (id, route_table_id, destination_cidr, target_type, target_id) VALUES (?, ?, ?, ?, ?)`,
		r.ID, r.RouteTableID, r.DestinationCIDR, r.TargetType, r.TargetID,
	)
	return err
}

func (s *Store) ListRoutes(routeTableID string) ([]Route, error) {
	rows, err := s.db.Query(
		`SELECT id, route_table_id, destination_cidr, target_type, target_id FROM capvpc_routes WHERE route_table_id=? ORDER BY destination_cidr`, routeTableID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Route
	for rows.Next() {
		var r Route
		if err := rows.Scan(&r.ID, &r.RouteTableID, &r.DestinationCIDR, &r.TargetType, &r.TargetID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRoute(id string) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_routes WHERE id=?`, id)
	return err
}

func (s *Store) AssociateSubnetRouteTable(subnetID, routeTableID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM capvpc_subnet_rt_assoc WHERE subnet_id=?`, subnetID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO capvpc_subnet_rt_assoc (subnet_id, route_table_id) VALUES (?, ?)`,
		subnetID, routeTableID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE capvpc_subnets SET route_table_id=? WHERE id=?`, routeTableID, subnetID); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- SecurityGroup CRUD -----------------------------------------------------

func (s *Store) InsertSecurityGroup(sg SecurityGroup) error {
	deny := 1
	if !sg.DefaultDeny {
		deny = 0
	}
	_, err := s.db.Exec(
		`INSERT INTO capvpc_security_groups (id, vpc_id, name, description, default_deny, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		sg.ID, sg.VPCID, sg.Name, sg.Description, deny, sg.CreatedAt,
	)
	return err
}

func (s *Store) GetSecurityGroup(nameOrID, vpcID string) (SecurityGroup, error) {
	var sg SecurityGroup
	var deny int
	err := s.db.QueryRow(
		`SELECT id, vpc_id, name, description, default_deny, created_at FROM capvpc_security_groups WHERE (id=? OR name=?) AND vpc_id=?`,
		nameOrID, nameOrID, vpcID,
	).Scan(&sg.ID, &sg.VPCID, &sg.Name, &sg.Description, &deny, &sg.CreatedAt)
	if err == sql.ErrNoRows {
		return sg, fmt.Errorf("security group %q not found", nameOrID)
	}
	sg.DefaultDeny = deny == 1
	return sg, err
}

func (s *Store) ListSecurityGroups(vpcID string) ([]SecurityGroup, error) {
	rows, err := s.db.Query(
		`SELECT id, vpc_id, name, description, default_deny, created_at FROM capvpc_security_groups WHERE vpc_id=? ORDER BY name`, vpcID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SecurityGroup
	for rows.Next() {
		var sg SecurityGroup
		var deny int
		if err := rows.Scan(&sg.ID, &sg.VPCID, &sg.Name, &sg.Description, &deny, &sg.CreatedAt); err != nil {
			return nil, err
		}
		sg.DefaultDeny = deny == 1
		out = append(out, sg)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSecurityGroup(nameOrID, vpcID string) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_security_groups WHERE (id=? OR name=?) AND vpc_id=?`, nameOrID, nameOrID, vpcID)
	return err
}

func (s *Store) InsertSGRule(rule SecurityGroupRule) error {
	_, err := s.db.Exec(
		`INSERT INTO capvpc_sg_rules (id, security_group_id, direction, protocol, from_port, to_port, cidr, source_sg_id, action) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.SecurityGroupID, string(rule.Direction), rule.Protocol,
		rule.FromPort, rule.ToPort, rule.CIDR, rule.SourceSGID, rule.Action,
	)
	return err
}

func (s *Store) ListSGRules(sgID string) ([]SecurityGroupRule, error) {
	rows, err := s.db.Query(
		`SELECT id, security_group_id, direction, protocol, from_port, to_port, cidr, source_sg_id, action FROM capvpc_sg_rules WHERE security_group_id=?`, sgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SecurityGroupRule
	for rows.Next() {
		var r SecurityGroupRule
		var dir string
		if err := rows.Scan(&r.ID, &r.SecurityGroupID, &dir, &r.Protocol, &r.FromPort, &r.ToPort, &r.CIDR, &r.SourceSGID, &r.Action); err != nil {
			return nil, err
		}
		r.Direction = SGRuleDirection(dir)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSGRule(id string) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_sg_rules WHERE id=?`, id)
	return err
}

// ---- InternetGateway CRUD ---------------------------------------------------

func (s *Store) InsertIGW(igw InternetGateway) error {
	_, err := s.db.Exec(
		`INSERT INTO capvpc_internet_gateways (id, vpc_id, name, created_at) VALUES (?, ?, ?, ?)`,
		igw.ID, igw.VPCID, igw.Name, igw.CreatedAt,
	)
	return err
}

func (s *Store) GetIGW(nameOrID, vpcID string) (InternetGateway, error) {
	var igw InternetGateway
	err := s.db.QueryRow(
		`SELECT id, vpc_id, name, created_at FROM capvpc_internet_gateways WHERE (id=? OR name=?) AND vpc_id=?`,
		nameOrID, nameOrID, vpcID,
	).Scan(&igw.ID, &igw.VPCID, &igw.Name, &igw.CreatedAt)
	if err == sql.ErrNoRows {
		return igw, fmt.Errorf("internet gateway %q not found", nameOrID)
	}
	return igw, err
}

func (s *Store) ListIGWs(vpcID string) ([]InternetGateway, error) {
	rows, err := s.db.Query(
		`SELECT id, vpc_id, name, created_at FROM capvpc_internet_gateways WHERE vpc_id=? ORDER BY name`, vpcID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InternetGateway
	for rows.Next() {
		var igw InternetGateway
		if err := rows.Scan(&igw.ID, &igw.VPCID, &igw.Name, &igw.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, igw)
	}
	return out, rows.Err()
}

func (s *Store) DeleteIGW(nameOrID, vpcID string) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_internet_gateways WHERE (id=? OR name=?) AND vpc_id=?`, nameOrID, nameOrID, vpcID)
	return err
}

// ---- NATGateway CRUD --------------------------------------------------------

func (s *Store) InsertNATGateway(nat NATGateway) error {
	_, err := s.db.Exec(
		`INSERT INTO capvpc_nat_gateways (id, vpc_id, subnet_id, name, public_ip, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		nat.ID, nat.VPCID, nat.SubnetID, nat.Name, nat.PublicIP, nat.CreatedAt,
	)
	return err
}

func (s *Store) GetNATGateway(nameOrID, vpcID string) (NATGateway, error) {
	var nat NATGateway
	err := s.db.QueryRow(
		`SELECT id, vpc_id, subnet_id, name, public_ip, created_at FROM capvpc_nat_gateways WHERE (id=? OR name=?) AND vpc_id=?`,
		nameOrID, nameOrID, vpcID,
	).Scan(&nat.ID, &nat.VPCID, &nat.SubnetID, &nat.Name, &nat.PublicIP, &nat.CreatedAt)
	if err == sql.ErrNoRows {
		return nat, fmt.Errorf("nat gateway %q not found", nameOrID)
	}
	return nat, err
}

func (s *Store) ListNATGateways(vpcID string) ([]NATGateway, error) {
	rows, err := s.db.Query(
		`SELECT id, vpc_id, subnet_id, name, public_ip, created_at FROM capvpc_nat_gateways WHERE vpc_id=? ORDER BY name`, vpcID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NATGateway
	for rows.Next() {
		var nat NATGateway
		if err := rows.Scan(&nat.ID, &nat.VPCID, &nat.SubnetID, &nat.Name, &nat.PublicIP, &nat.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, nat)
	}
	return out, rows.Err()
}

func (s *Store) DeleteNATGateway(nameOrID, vpcID string) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_nat_gateways WHERE (id=? OR name=?) AND vpc_id=?`, nameOrID, nameOrID, vpcID)
	return err
}

// ---- ID helpers -------------------------------------------------------------

func newID(prefix string) string {
	return prefix + "_" + fmt.Sprintf("%d", time.Now().UnixNano())
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }
