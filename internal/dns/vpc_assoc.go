package dns

import (
	"database/sql"
	"fmt"
)

// ZoneVPCAssociation links a private zone to a VPC.
type ZoneVPCAssociation struct {
	ZoneID string `json:"zoneId"`
	VPCID  string `json:"vpcId"`
}

func (s *Store) AssociateZoneVPC(zoneID, vpcID string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO dns_zone_vpc_assoc (zone_id, vpc_id) VALUES (?, ?)`, zoneID, vpcID)
	if err != nil {
		return fmt.Errorf("dns: associate vpc: %w", err)
	}
	return nil
}

func (s *Store) DisassociateZoneVPC(zoneID, vpcID string) error {
	res, err := s.db.Exec(`DELETE FROM dns_zone_vpc_assoc WHERE zone_id=? AND vpc_id=?`, zoneID, vpcID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("association not found")
	}
	return nil
}

func (s *Store) ListZoneVPCs(zoneID string) ([]ZoneVPCAssociation, error) {
	rows, err := s.db.Query(`SELECT zone_id, vpc_id FROM dns_zone_vpc_assoc WHERE zone_id=?`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ZoneVPCAssociation
	for rows.Next() {
		var a ZoneVPCAssociation
		if err := rows.Scan(&a.ZoneID, &a.VPCID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListVPCZones(vpcID string) ([]ZoneVPCAssociation, error) {
	rows, err := s.db.Query(`SELECT zone_id, vpc_id FROM dns_zone_vpc_assoc WHERE vpc_id=?`, vpcID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ZoneVPCAssociation
	for rows.Next() {
		var a ZoneVPCAssociation
		if err := rows.Scan(&a.ZoneID, &a.VPCID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
