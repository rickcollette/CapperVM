package lb

import (
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// migrateLegacyLBs converts single-listener LBs (listen_addr + lb_backends) into
// default target groups and listeners. Idempotent: skips LBs that already have listeners.
func migrateLegacyLBs(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, name, project, network_id, subnet_id, vpc_id, scheme, type, vip_address, routable_ip_id, eni_id, dns_name, mode, listen_addr, status, algorithm, selector, tls_cert_name, service_alias, created_at FROM lb_load_balancers`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lbs []LoadBalancer
	for rows.Next() {
		lb, err := scanLB(rows)
		if err != nil {
			return err
		}
		lbs = append(lbs, lb)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, lb := range lbs {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM lb_listeners WHERE load_balancer_id=?`, lb.ID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if lb.ListenAddr == "" && lb.VIPAddress == "" {
			continue
		}
		if err := migrateOneLegacyLB(db, lb); err != nil {
			return fmt.Errorf("migrate lb %q: %w", lb.Name, err)
		}
	}
	return nil
}

func migrateOneLegacyLB(db *sql.DB, lb LoadBalancer) error {
	port, proto := parseLegacyListen(lb.ListenAddr, lb.Mode)
	tgID := "tg_" + fmt.Sprintf("%d", time.Now().UnixNano())
	tgName := lb.Name + "-default"
	now := time.Now().UTC().Format(time.RFC3339)

	subnetID := lb.SubnetID
	if subnetID == "" {
		subnetID = lb.NetworkID
	}
	vpcID := lb.VPCID

	if _, err := db.Exec(
		`INSERT INTO lb_target_groups (id, name, project, vpc_id, load_balancer_id, protocol, port, health_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '/', ?)`,
		tgID, tgName, lb.Project, vpcID, lb.ID, strings.ToLower(string(proto)), port, now,
	); err != nil {
		return err
	}

	backends, _ := listBackendsRaw(db, lb.ID)
	for _, b := range backends {
		tgtID := "tgt_" + fmt.Sprintf("%d", time.Now().UnixNano())
		if _, err := db.Exec(
			`INSERT INTO lb_target_group_targets (id, target_group_id, address, weight) VALUES (?, ?, ?, 1)`,
			tgtID, tgID, b.Address,
		); err != nil {
			return err
		}
	}

	lstID := "lst_" + fmt.Sprintf("%d", time.Now().UnixNano())
	certID := lb.TLSCertName
	if _, err := db.Exec(
		`INSERT INTO lb_listeners (id, load_balancer_id, target_group_id, protocol, port, certificate_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		lstID, lb.ID, tgID, string(proto), port, certID, now,
	); err != nil {
		return err
	}

	// Set scheme/vip for legacy rows.
	vip := lb.VIPAddress
	if vip == "" {
		vip = "0.0.0.0"
	}
	scheme := string(lb.Scheme)
	if scheme == "" {
		scheme = string(SchemeInternal)
	}
	lbType := string(lb.Type)
	if lbType == "" {
		if proto == ProtoTCP {
			lbType = string(TypeNetwork)
		} else {
			lbType = string(TypeApplication)
		}
	}
	_, err := db.Exec(
		`UPDATE lb_load_balancers SET scheme=?, type=?, vip_address=?, subnet_id=COALESCE(NULLIF(subnet_id,''), network_id) WHERE id=?`,
		scheme, lbType, vip, lb.ID,
	)
	return err
}

func parseLegacyListen(listenAddr string, mode LBMode) (port int, proto ListenerProtocol) {
	host, portStr, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return 80, protoFromMode(mode)
	}
	_ = host
	p, _ := strconv.Atoi(portStr)
	if p == 0 {
		p = 80
	}
	return p, protoFromMode(mode)
}

func protoFromMode(mode LBMode) ListenerProtocol {
	switch mode {
	case ModeHTTPS:
		return ProtoHTTPS
	case ModeHTTP:
		return ProtoHTTP
	default:
		return ProtoTCP
	}
}

func listBackendsRaw(db *sql.DB, lbID string) ([]Backend, error) {
	rows, err := db.Query(`SELECT id, lb_id, address FROM lb_backends WHERE lb_id=?`, lbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Backend
	for rows.Next() {
		var b Backend
		if err := rows.Scan(&b.ID, &b.LBID, &b.Address); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
