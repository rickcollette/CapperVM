package vpc

import (
	"database/sql"
	"strings"
)

// ListLBNamesInVPC returns load balancer names using this VPC.
func (s *Store) ListLBNamesInVPC(vpcID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM lb_load_balancers WHERE vpc_id=?`, vpcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringCol(rows)
}

// ListLBNamesInSubnet returns load balancer names using this subnet.
func (s *Store) ListLBNamesInSubnet(subnetID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT name FROM lb_load_balancers WHERE subnet_id=? OR network_id=?`,
		subnetID, subnetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringCol(rows)
}

// ListDNSZonesForVPC returns DNS zone IDs associated with a VPC.
func (s *Store) ListDNSZonesForVPC(vpcID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT zone_id FROM dns_zone_vpc_assoc WHERE vpc_id=?`, vpcID)
	if err != nil {
		if isMissingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanStringCol(rows)
}

// ListENIIDsInVPC returns ENI ids in a VPC.
func (s *Store) ListENIIDsInVPC(vpcID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM capvpc_enis WHERE vpc_id=?`, vpcID)
	if err != nil {
		if isMissingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanStringCol(rows)
}

// ListENIIDsInSubnet returns ENI ids in a subnet.
func (s *Store) ListENIIDsInSubnet(subnetID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM capvpc_enis WHERE subnet_id=?`, subnetID)
	if err != nil {
		if isMissingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanStringCol(rows)
}

// ListInstanceIDsFromENIs returns distinct instance ids attached to ENIs in a VPC.
func (s *Store) ListInstanceIDsFromENIs(vpcID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT instance_id FROM capvpc_enis WHERE vpc_id=? AND instance_id != ''`,
		vpcID,
	)
	if err != nil {
		if isMissingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanStringCol(rows)
}

// ListInstanceIDsInSubnet returns instance ids with ENIs in a subnet.
func (s *Store) ListInstanceIDsInSubnet(subnetID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT instance_id FROM capvpc_enis WHERE subnet_id=? AND instance_id != ''`,
		subnetID,
	)
	if err != nil {
		if isMissingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanStringCol(rows)
}

func scanStringCol(rows *sql.Rows) ([]string, error) {
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no such table")
}
