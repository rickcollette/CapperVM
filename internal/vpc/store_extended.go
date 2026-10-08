package vpc

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

func scanSubnet(row *sql.Row) (Subnet, error) {
	var sub Subnet
	var kind string
	var autoPublic int
	err := row.Scan(
		&sub.ID, &sub.VPCID, &sub.Name, &sub.CIDR, &sub.Zone, &kind, &sub.BridgeName, &sub.GatewayIP, &sub.CreatedAt,
		&sub.RealmID, &sub.RegionID, &sub.ZoneID, &sub.Slug, &sub.RouteTableID, &sub.NetworkACLID, &autoPublic, &sub.Status, &sub.UpdatedAt,
	)
	if err != nil {
		return sub, err
	}
	sub.Kind = SubnetKind(kind)
	sub.SubnetType = sub.Kind
	sub.Gateway = sub.GatewayIP
	sub.AutoAssignPublicIP = autoPublic == 1
	return sub, nil
}

const subnetSelectCols = `id, vpc_id, name, cidr, zone, kind, bridge_name, gateway_ip, created_at,
	COALESCE(realm_id,''), COALESCE(region_id,''), COALESCE(zone_id,''), COALESCE(slug,''),
	COALESCE(route_table_id,''), COALESCE(network_acl_id,''), COALESCE(auto_public_ip,0),
	COALESCE(status,'available'), COALESCE(updated_at,'')`

func (s *Store) GetSubnet(nameOrID, vpcID string) (Subnet, error) {
	row := s.db.QueryRow(
		`SELECT `+subnetSelectCols+` FROM capvpc_subnets WHERE (id=? OR name=? OR slug=?) AND vpc_id=?`,
		nameOrID, nameOrID, nameOrID, vpcID,
	)
	sub, err := scanSubnet(row)
	if err == sql.ErrNoRows {
		return sub, fmt.Errorf("subnet %q not found in vpc %q", nameOrID, vpcID)
	}
	return sub, err
}

func (s *Store) GetSubnetByID(subnetID string) (Subnet, error) {
	row := s.db.QueryRow(`SELECT `+subnetSelectCols+` FROM capvpc_subnets WHERE id=?`, subnetID)
	sub, err := scanSubnet(row)
	if err == sql.ErrNoRows {
		return sub, fmt.Errorf("subnet %q not found", subnetID)
	}
	return sub, err
}

func (s *Store) ListSubnets(vpcID string) ([]Subnet, error) {
	rows, err := s.db.Query(`SELECT `+subnetSelectCols+` FROM capvpc_subnets WHERE vpc_id=? ORDER BY name`, vpcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subnet
	for rows.Next() {
		var sub Subnet
		var kind string
		var autoPublic int
		if err := rows.Scan(
			&sub.ID, &sub.VPCID, &sub.Name, &sub.CIDR, &sub.Zone, &kind, &sub.BridgeName, &sub.GatewayIP, &sub.CreatedAt,
			&sub.RealmID, &sub.RegionID, &sub.ZoneID, &sub.Slug, &sub.RouteTableID, &sub.NetworkACLID, &autoPublic, &sub.Status, &sub.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sub.Kind = SubnetKind(kind)
		sub.SubnetType = sub.Kind
		sub.Gateway = sub.GatewayIP
		sub.AutoAssignPublicIP = autoPublic == 1
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSubnetBridge(subnetID, bridge, gateway string) error {
	_, err := s.db.Exec(
		`UPDATE capvpc_subnets SET bridge_name=?, gateway_ip=? WHERE id=?`,
		bridge, gateway, subnetID,
	)
	return err
}

func (s *Store) UpdateSubnet(sub Subnet) error {
	kind := string(sub.Kind)
	if kind == "" {
		kind = string(sub.SubnetType)
	}
	autoPublic := 0
	if sub.AutoAssignPublicIP {
		autoPublic = 1
	}
	_, err := s.db.Exec(
		`UPDATE capvpc_subnets SET name=?, cidr=?, zone=?, kind=?, route_table_id=?, network_acl_id=?, auto_public_ip=?, status=?, updated_at=? WHERE id=?`,
		sub.Name, sub.CIDR, sub.Zone, kind, sub.RouteTableID, sub.NetworkACLID, autoPublic, sub.Status, sub.UpdatedAt, sub.ID,
	)
	return err
}

func (s *Store) UpdateVPC(v VPC) error {
	labels, _ := json.Marshal(v.Labels)
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
		`UPDATE capvpc_vpcs SET name=?, description=?, status=?, mobility_policy=?, labels_json=?, dns_domain=?,
		 dns_support=?, dns_hostnames=?, default_sg_id=?, default_acl_id=?, main_rt_id=?, enable_flow_logs=?, updated_at=? WHERE id=?`,
		v.Name, v.Description, v.Status, v.MobilityPolicy, string(labels), v.DNSDomain,
		dnsSupport, dnsHostnames, v.DefaultSecurityGroupID, v.DefaultNetworkACLID, v.MainRouteTableID,
		flowLogs, v.UpdatedAt, v.ID,
	)
	return err
}

// ---- Network ACL CRUD -------------------------------------------------------

func (s *Store) InsertNetworkACL(acl NetworkACL) error {
	isDef := 0
	if acl.IsDefault {
		isDef = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO capvpc_network_acls (id, vpc_id, name, is_default, created_at) VALUES (?, ?, ?, ?, ?)`,
		acl.ID, acl.VPCID, acl.Name, isDef, acl.CreatedAt,
	)
	return err
}

func (s *Store) GetNetworkACL(nameOrID, vpcID string) (NetworkACL, error) {
	var acl NetworkACL
	var isDef int
	err := s.db.QueryRow(
		`SELECT id, vpc_id, name, is_default, created_at FROM capvpc_network_acls WHERE (id=? OR name=?) AND vpc_id=?`,
		nameOrID, nameOrID, vpcID,
	).Scan(&acl.ID, &acl.VPCID, &acl.Name, &isDef, &acl.CreatedAt)
	if err == sql.ErrNoRows {
		return acl, fmt.Errorf("network acl %q not found", nameOrID)
	}
	acl.IsDefault = isDef == 1
	return acl, err
}

func (s *Store) ListNetworkACLs(vpcID string) ([]NetworkACL, error) {
	rows, err := s.db.Query(`SELECT id, vpc_id, name, is_default, created_at FROM capvpc_network_acls WHERE vpc_id=? ORDER BY name`, vpcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NetworkACL
	for rows.Next() {
		var acl NetworkACL
		var isDef int
		if err := rows.Scan(&acl.ID, &acl.VPCID, &acl.Name, &isDef, &acl.CreatedAt); err != nil {
			return nil, err
		}
		acl.IsDefault = isDef == 1
		out = append(out, acl)
	}
	return out, rows.Err()
}

func (s *Store) DeleteNetworkACL(nameOrID, vpcID string) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_network_acls WHERE (id=? OR name=?) AND vpc_id=?`, nameOrID, nameOrID, vpcID)
	return err
}

func (s *Store) InsertNetworkACLEntry(e NetworkACLEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO capvpc_network_acl_entries (id, network_acl_id, rule_number, direction, action, protocol, cidr, from_port, to_port) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.NetworkACLID, e.RuleNumber, e.Direction, e.Action, e.Protocol, e.CIDR, e.FromPort, e.ToPort,
	)
	return err
}

func (s *Store) ListNetworkACLEntries(aclID string) ([]NetworkACLEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, network_acl_id, rule_number, direction, action, protocol, cidr, from_port, to_port FROM capvpc_network_acl_entries WHERE network_acl_id=? ORDER BY direction, rule_number`,
		aclID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NetworkACLEntry
	for rows.Next() {
		var e NetworkACLEntry
		if err := rows.Scan(&e.ID, &e.NetworkACLID, &e.RuleNumber, &e.Direction, &e.Action, &e.Protocol, &e.CIDR, &e.FromPort, &e.ToPort); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) DeleteNetworkACLEntry(aclID string, ruleNumber int) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_network_acl_entries WHERE network_acl_id=? AND rule_number=?`, aclID, ruleNumber)
	return err
}

func (s *Store) AssociateSubnetNetworkACL(subnetID, aclID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM capvpc_subnet_acl_assoc WHERE subnet_id=?`, subnetID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO capvpc_subnet_acl_assoc (subnet_id, network_acl_id) VALUES (?, ?)`, subnetID, aclID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE capvpc_subnets SET network_acl_id=? WHERE id=?`, aclID, subnetID); err != nil {
		return err
	}
	return tx.Commit()
}
