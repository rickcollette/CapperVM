package vpc

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// VPCEndpoint is a gateway or interface endpoint.
type VPCEndpoint struct {
	ID          string `json:"id"`
	VPCID       string `json:"vpcId"`
	Name        string `json:"name"`
	ServiceName string `json:"serviceName"`
	EndpointType string `json:"endpointType"` // gateway, interface
	SubnetIDs   []string `json:"subnetIds,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
}

// VPCPeering connects two VPCs.
type VPCPeering struct {
	ID              string `json:"id"`
	RequesterVPCID  string `json:"requesterVpcId"`
	AccepterVPCID   string `json:"accepterVpcId"`
	Status          string `json:"status"`
	CreatedAt       string `json:"createdAt"`
}

// FlowLog captures traffic metadata for a resource.
type FlowLog struct {
	ID           string `json:"id"`
	ResourceType string `json:"resourceType"` // vpc, subnet
	ResourceID   string `json:"resourceId"`
	Destination  string `json:"destination"` // file, syslog
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
}

func (s *Store) InsertVPCEndpoint(e VPCEndpoint) error {
	subs, _ := json.Marshal(e.SubnetIDs)
	_, err := s.db.Exec(
		`INSERT INTO capvpc_endpoints (id, vpc_id, name, service_name, endpoint_type, subnet_ids_json, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.VPCID, e.Name, e.ServiceName, e.EndpointType, string(subs), e.Status, e.CreatedAt,
	)
	return err
}

func (s *Store) ListVPCEndpoints(vpcID string) ([]VPCEndpoint, error) {
	q := `SELECT id, vpc_id, name, service_name, endpoint_type, subnet_ids_json, status, created_at FROM capvpc_endpoints`
	var rows *sql.Rows
	var err error
	if vpcID != "" {
		rows, err = s.db.Query(q+` WHERE vpc_id=? ORDER BY name`, vpcID)
	} else {
		rows, err = s.db.Query(q + ` ORDER BY name`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEndpoints(rows)
}

func scanEndpoints(rows *sql.Rows) ([]VPCEndpoint, error) {
	var out []VPCEndpoint
	for rows.Next() {
		var e VPCEndpoint
		var subsJSON string
		if err := rows.Scan(&e.ID, &e.VPCID, &e.Name, &e.ServiceName, &e.EndpointType, &subsJSON, &e.Status, &e.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(subsJSON), &e.SubnetIDs)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) InsertVPCPeering(p VPCPeering) error {
	_, err := s.db.Exec(
		`INSERT INTO capvpc_peerings (id, requester_vpc_id, accepter_vpc_id, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.RequesterVPCID, p.AccepterVPCID, p.Status, p.CreatedAt,
	)
	return err
}

func (s *Store) ListVPCPeerings(vpcID string) ([]VPCPeering, error) {
	q := `SELECT id, requester_vpc_id, accepter_vpc_id, status, created_at FROM capvpc_peerings`
	var rows *sql.Rows
	var err error
	if vpcID != "" {
		rows, err = s.db.Query(q+` WHERE requester_vpc_id=? OR accepter_vpc_id=? ORDER BY created_at DESC`, vpcID, vpcID)
	} else {
		rows, err = s.db.Query(q + ` ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VPCPeering
	for rows.Next() {
		var p VPCPeering
		if err := rows.Scan(&p.ID, &p.RequesterVPCID, &p.AccepterVPCID, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) InsertFlowLog(f FlowLog) error {
	_, err := s.db.Exec(
		`INSERT INTO capvpc_flow_logs (id, resource_type, resource_id, destination, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		f.ID, f.ResourceType, f.ResourceID, f.Destination, f.Status, f.CreatedAt,
	)
	return err
}

func (s *Store) ListFlowLogs(resourceID string) ([]FlowLog, error) {
	q := `SELECT id, resource_type, resource_id, destination, status, created_at FROM capvpc_flow_logs`
	var rows *sql.Rows
	var err error
	if resourceID != "" {
		rows, err = s.db.Query(q+` WHERE resource_id=? ORDER BY created_at DESC`, resourceID)
	} else {
		rows, err = s.db.Query(q + ` ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FlowLog
	for rows.Next() {
		var f FlowLog
		if err := rows.Scan(&f.ID, &f.ResourceType, &f.ResourceID, &f.Destination, &f.Status, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (m *Manager) CreateVPCEndpoint(vpcID, name, serviceName, endpointType string, subnetIDs []string) (VPCEndpoint, error) {
	if _, err := m.store.GetVPC(vpcID, ""); err != nil {
		return VPCEndpoint{}, fmt.Errorf("vpc not found")
	}
	e := VPCEndpoint{
		ID:           newID("vpce"),
		VPCID:        vpcID,
		Name:         name,
		ServiceName:  serviceName,
		EndpointType: endpointType,
		SubnetIDs:    subnetIDs,
		Status:       "available",
		CreatedAt:    now(),
	}
	if e.EndpointType == "" {
		e.EndpointType = "gateway"
	}
	return e, m.store.InsertVPCEndpoint(e)
}

func (m *Manager) ListVPCEndpoints(vpcID string) ([]VPCEndpoint, error) {
	return m.store.ListVPCEndpoints(vpcID)
}

func (m *Manager) CreateVPCPeering(requesterVPCID, accepterVPCID string) (VPCPeering, error) {
	if requesterVPCID == accepterVPCID {
		return VPCPeering{}, fmt.Errorf("cannot peer vpc with itself")
	}
	p := VPCPeering{
		ID:             newID("pcx"),
		RequesterVPCID: requesterVPCID,
		AccepterVPCID:  accepterVPCID,
		Status:         "pending-acceptance",
		CreatedAt:      now(),
	}
	return p, m.store.InsertVPCPeering(p)
}

func (m *Manager) ListVPCPeerings(vpcID string) ([]VPCPeering, error) {
	return m.store.ListVPCPeerings(vpcID)
}

func (m *Manager) CreateFlowLog(resourceType, resourceID, destination string) (FlowLog, error) {
	f := FlowLog{
		ID:           newID("fl"),
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Destination:  destination,
		Status:       "active",
		CreatedAt:    now(),
	}
	if f.Destination == "" {
		f.Destination = "file"
	}
	return f, m.store.InsertFlowLog(f)
}

func (m *Manager) ListFlowLogs(resourceID string) ([]FlowLog, error) {
	return m.store.ListFlowLogs(resourceID)
}
