package lb

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *Store) InsertTargetGroup(tg TargetGroup) error {
	_, err := s.db.Exec(
		`INSERT INTO lb_target_groups (id, name, project, vpc_id, load_balancer_id, protocol, port, health_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tg.ID, tg.Name, tg.Project, tg.VPCID, tg.LoadBalancerID, tg.Protocol, tg.Port, tg.HealthPath, tg.CreatedAt,
	)
	return err
}

func (s *Store) GetTargetGroup(id string) (TargetGroup, error) {
	var tg TargetGroup
	err := s.db.QueryRow(
		`SELECT id, name, project, vpc_id, load_balancer_id, protocol, port, health_path, created_at
		 FROM lb_target_groups WHERE id=?`, id,
	).Scan(&tg.ID, &tg.Name, &tg.Project, &tg.VPCID, &tg.LoadBalancerID, &tg.Protocol, &tg.Port, &tg.HealthPath, &tg.CreatedAt)
	if err == sql.ErrNoRows {
		return tg, fmt.Errorf("target group %q not found", id)
	}
	return tg, err
}

func (s *Store) ListTargetGroups(project string) ([]TargetGroup, error) {
	q := `SELECT id, name, project, vpc_id, load_balancer_id, protocol, port, health_path, created_at FROM lb_target_groups`
	var rows *sql.Rows
	var err error
	if project == "" {
		rows, err = s.db.Query(q + ` ORDER BY name`)
	} else {
		rows, err = s.db.Query(q+` WHERE project=? ORDER BY name`, project)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTargetGroups(rows)
}

func (s *Store) ListTargetGroupsForLB(lbID string) ([]TargetGroup, error) {
	rows, err := s.db.Query(
		`SELECT id, name, project, vpc_id, load_balancer_id, protocol, port, health_path, created_at
		 FROM lb_target_groups WHERE load_balancer_id=? ORDER BY name`, lbID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTargetGroups(rows)
}

func scanTargetGroups(rows *sql.Rows) ([]TargetGroup, error) {
	var out []TargetGroup
	for rows.Next() {
		var tg TargetGroup
		if err := rows.Scan(&tg.ID, &tg.Name, &tg.Project, &tg.VPCID, &tg.LoadBalancerID, &tg.Protocol, &tg.Port, &tg.HealthPath, &tg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, tg)
	}
	return out, rows.Err()
}

func (s *Store) UpdateTargetGroup(id string, port int, healthPath string) error {
	res, err := s.db.Exec(
		`UPDATE lb_target_groups SET port=?, health_path=? WHERE id=?`,
		port, healthPath, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("target group %q not found", id)
	}
	return nil
}

func (s *Store) DeleteTargetGroup(id string) error {
	_, _ = s.db.Exec(`DELETE FROM lb_target_group_targets WHERE target_group_id=?`, id)
	res, err := s.db.Exec(`DELETE FROM lb_target_groups WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("target group %q not found", id)
	}
	return nil
}

func (s *Store) CreateTargetGroup(project, name, vpcID, lbID, protocol string, port int, healthPath string) (TargetGroup, error) {
	if name == "" {
		return TargetGroup{}, fmt.Errorf("name is required")
	}
	if protocol == "" {
		protocol = "tcp"
	}
	if port == 0 {
		port = 80
	}
	if healthPath == "" {
		healthPath = "/"
	}
	tg := TargetGroup{
		ID:             newTGID(),
		Name:           name,
		Project:        project,
		VPCID:          vpcID,
		LoadBalancerID: lbID,
		Protocol:       protocol,
		Port:           port,
		HealthPath:     healthPath,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	return tg, s.InsertTargetGroup(tg)
}

func (s *Store) InsertListener(l Listener) error {
	_, err := s.db.Exec(
		`INSERT INTO lb_listeners (id, load_balancer_id, target_group_id, protocol, port, certificate_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.LoadBalancerID, l.TargetGroupID, l.Protocol, l.Port, l.CertificateID, l.CreatedAt,
	)
	return err
}

func (s *Store) GetListener(id string) (Listener, error) {
	var l Listener
	err := s.db.QueryRow(
		`SELECT id, load_balancer_id, target_group_id, protocol, port, certificate_id, created_at
		 FROM lb_listeners WHERE id=?`, id,
	).Scan(&l.ID, &l.LoadBalancerID, &l.TargetGroupID, &l.Protocol, &l.Port, &l.CertificateID, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return l, fmt.Errorf("listener %q not found", id)
	}
	return l, err
}

func (s *Store) ListListeners(lbID string) ([]Listener, error) {
	rows, err := s.db.Query(
		`SELECT id, load_balancer_id, target_group_id, protocol, port, certificate_id, created_at
		 FROM lb_listeners WHERE load_balancer_id=? ORDER BY port`,
		lbID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Listener
	for rows.Next() {
		var l Listener
		if err := rows.Scan(&l.ID, &l.LoadBalancerID, &l.TargetGroupID, &l.Protocol, &l.Port, &l.CertificateID, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) UpdateListener(id string, port int, protocol, certID, tgID string) error {
	res, err := s.db.Exec(
		`UPDATE lb_listeners SET port=?, protocol=?, certificate_id=?, target_group_id=? WHERE id=?`,
		port, protocol, certID, tgID, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("listener %q not found", id)
	}
	return nil
}

func (s *Store) SetListenerCertificate(id, certID string) error {
	res, err := s.db.Exec(`UPDATE lb_listeners SET certificate_id=? WHERE id=?`, certID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("listener %q not found", id)
	}
	return nil
}

func (s *Store) DeleteListener(id string) error {
	res, err := s.db.Exec(`DELETE FROM lb_listeners WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("listener %q not found", id)
	}
	return nil
}

func (s *Store) CreateListener(lbID, tgID, protocol string, port int, certID string) (Listener, error) {
	if lbID == "" || tgID == "" {
		return Listener{}, fmt.Errorf("loadBalancerId and targetGroupId are required")
	}
	if protocol == "" {
		protocol = "TCP"
	}
	protocol = strings.ToUpper(protocol)
	if port == 0 {
		port = 80
	}
	l := Listener{
		ID:             newLstID(),
		LoadBalancerID: lbID,
		TargetGroupID:  tgID,
		Protocol:       protocol,
		Port:           port,
		CertificateID:  certID,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	return l, s.InsertListener(l)
}

func (s *Store) AddTarget(tgID, address string, weight int) (Target, error) {
	if weight <= 0 {
		weight = 1
	}
	t := Target{
		ID:            newTgtID(),
		TargetGroupID: tgID,
		Address:       address,
		Weight:        weight,
	}
	res, err := s.db.Exec(
		`INSERT INTO lb_target_group_targets (id, target_group_id, address, weight) VALUES (?, ?, ?, ?)
		 ON CONFLICT(target_group_id, address) DO NOTHING`,
		t.ID, tgID, address, weight,
	)
	if err != nil {
		return Target{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Return the existing row rather than a fabricated id.
		row := s.db.QueryRow(
			`SELECT id, target_group_id, address, weight FROM lb_target_group_targets WHERE target_group_id=? AND address=?`,
			tgID, address,
		)
		var existing Target
		if err := row.Scan(&existing.ID, &existing.TargetGroupID, &existing.Address, &existing.Weight); err != nil {
			return Target{}, fmt.Errorf("target already exists but could not be loaded: %w", err)
		}
		return existing, nil
	}
	return t, nil
}

func (s *Store) RemoveTarget(tgID, targetID string) error {
	res, err := s.db.Exec(
		`DELETE FROM lb_target_group_targets WHERE target_group_id=? AND (id=? OR address=?)`,
		tgID, targetID, targetID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("target %q not found", targetID)
	}
	return nil
}

func (s *Store) ListTargets(tgID string) ([]Target, error) {
	rows, err := s.db.Query(
		`SELECT id, target_group_id, address, weight FROM lb_target_group_targets WHERE target_group_id=? ORDER BY address`,
		tgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		var t Target
		if err := rows.Scan(&t.ID, &t.TargetGroupID, &t.Address, &t.Weight); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ListTargetAddresses(tgID string) ([]string, error) {
	targets, err := s.ListTargets(tgID)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(targets))
	for i, t := range targets {
		out[i] = t.Address
	}
	return out, nil
}

func (s *Store) ListListenerIDsForTG(tgID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM lb_listeners WHERE target_group_id=?`, tgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) ListSubnetVIPs(subnetID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT vip_address FROM lb_load_balancers WHERE subnet_id=? AND vip_address != ''`,
		subnetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	return out, rows.Err()
}
