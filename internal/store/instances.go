package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"capper/internal/types"
)

const instanceCols = `id, name, image_name, image_id, image_digest, pid, status, created_at, started_at, stopped_at, rootfs_path, command, restart_policy, restart_count, realm_id, region_id, zone_id, node_id, placement_policy_id, desired_state, generation`

func (s *Store) InsertInstance(inst types.Instance) error {
	if inst.DesiredState == "" {
		inst.DesiredState = "running"
	}
	if inst.Generation == 0 {
		inst.Generation = 1
	}
	_, err := s.DB.Exec(`
		INSERT INTO instances (id, name, image_name, image_id, image_digest, pid, status, created_at, started_at, stopped_at, rootfs_path, command, restart_policy, restart_count, realm_id, region_id, zone_id, node_id, placement_policy_id, desired_state, generation)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, inst.ID, inst.Name, inst.Image, inst.ImageID, inst.ImageDigest, nullablePID(inst.PID), inst.Status,
		inst.CreatedAt, inst.StartedAt, inst.StoppedAt, inst.RootFSPath, inst.Command,
		string(inst.RestartPolicy), inst.RestartCount,
		inst.RealmID, inst.RegionID, inst.ZoneID, inst.NodeID,
		inst.PlacementPolicyID, inst.DesiredState, inst.Generation)
	return err
}

func (s *Store) UpdateInstance(inst types.Instance) error {
	_, err := s.DB.Exec(`
		UPDATE instances SET pid=?, status=?, started_at=?, stopped_at=?, rootfs_path=?, command=?,
			restart_policy=?, restart_count=?,
			realm_id=?, region_id=?, zone_id=?, node_id=?,
			placement_policy_id=?, desired_state=?, generation=?
		WHERE id=?
	`, nullablePID(inst.PID), inst.Status, inst.StartedAt, inst.StoppedAt, inst.RootFSPath, inst.Command,
		string(inst.RestartPolicy), inst.RestartCount,
		inst.RealmID, inst.RegionID, inst.ZoneID, inst.NodeID,
		inst.PlacementPolicyID, inst.DesiredState, inst.Generation,
		inst.ID)
	return err
}

func (s *Store) ResolveInstance(ref string) (*types.Instance, error) {
	row := s.DB.QueryRow(`SELECT `+instanceCols+` FROM instances WHERE id = ? OR name = ?`, ref, ref)
	inst, err := scanInstance(row)
	if err != nil {
		return nil, err
	}
	return s.mergeInstanceJSON(inst), nil
}

func (s *Store) ListInstances() ([]types.Instance, error) {
	rows, err := s.DB.Query(`SELECT ` + instanceCols + ` FROM instances ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.Instance, 0)
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

func (s *Store) RunningInstancesForImage(imageName string) ([]types.Instance, error) {
	rows, err := s.DB.Query(`SELECT `+instanceCols+` FROM instances WHERE image_name = ? AND status = ?`, imageName, types.StatusRunning)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.Instance, 0)
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

// ListInstancesByZone returns all instances placed in the given zone.
func (s *Store) ListInstancesByZone(zoneID string) ([]types.Instance, error) {
	rows, err := s.DB.Query(`SELECT `+instanceCols+` FROM instances WHERE zone_id = ? ORDER BY created_at DESC`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.Instance, 0)
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

// ListInstancesByNode returns all instances placed on the given node.
func (s *Store) ListInstancesByNode(nodeID string) ([]types.Instance, error) {
	rows, err := s.DB.Query(`SELECT `+instanceCols+` FROM instances WHERE node_id = ? ORDER BY created_at DESC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.Instance, 0)
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

// SetInstancePlacement writes topology IDs onto an existing instance row.
func (s *Store) SetInstancePlacement(id, realmID, regionID, zoneID, nodeID string) error {
	_, err := s.DB.Exec(
		`UPDATE instances SET realm_id=?, region_id=?, zone_id=?, node_id=? WHERE id=?`,
		realmID, regionID, zoneID, nodeID, id,
	)
	return err
}

func (s *Store) InstanceNameExists(name string) (bool, error) {
	row := s.DB.QueryRow(`SELECT 1 FROM instances WHERE name = ?`, name)
	var v int
	err := row.Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) WriteInstanceJSON(inst types.Instance) error {
	data, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(filepath.Dir(inst.RootFSPath), "instance.json"), append(data, '\n'), 0o600)
}

// MergeInstanceFromDisk is the exported form of mergeInstanceJSON.
func (s *Store) MergeInstanceFromDisk(inst types.Instance) *types.Instance {
	return s.mergeInstanceJSON(&inst)
}

func (s *Store) mergeInstanceJSON(inst *types.Instance) *types.Instance {
	data, err := os.ReadFile(filepath.Join(filepath.Dir(inst.RootFSPath), "instance.json"))
	if err != nil {
		return inst
	}
	var disk types.Instance
	if err := json.Unmarshal(data, &disk); err != nil {
		fmt.Fprintf(os.Stderr, "capper: warning: corrupt instance.json for %s: %v\n", inst.ID, err)
		return inst
	}
	inst.Entrypoint = disk.Entrypoint
	inst.Args = disk.Args
	inst.Shell = disk.Shell
	inst.User = disk.User
	inst.Resources = disk.Resources
	if disk.RuntimeMode != "" {
		inst.RuntimeMode = disk.RuntimeMode
	}
	if disk.RestartPolicy != "" {
		inst.RestartPolicy = disk.RestartPolicy
	}
	if disk.NetworkID != "" {
		inst.NetworkID = disk.NetworkID
	}
	if disk.NetworkIP != "" {
		inst.NetworkIP = disk.NetworkIP
	}
	if len(disk.Labels) > 0 {
		inst.Labels = disk.Labels
	}
	return inst
}

type scanner interface {
	Scan(dest ...any) error
}

func scanInstance(sc scanner) (*types.Instance, error) {
	var inst types.Instance
	var pid sql.NullInt64
	var started, stopped sql.NullString
	var restartPolicy string
	if err := sc.Scan(
		&inst.ID, &inst.Name, &inst.Image, &inst.ImageID, &inst.ImageDigest,
		&pid, &inst.Status, &inst.CreatedAt, &started, &stopped,
		&inst.RootFSPath, &inst.Command, &restartPolicy, &inst.RestartCount,
		&inst.RealmID, &inst.RegionID, &inst.ZoneID, &inst.NodeID,
		&inst.PlacementPolicyID, &inst.DesiredState, &inst.Generation,
	); err != nil {
		return nil, err
	}
	if pid.Valid {
		inst.PID = int(pid.Int64)
	}
	if started.Valid {
		inst.StartedAt = started.String
	}
	if stopped.Valid {
		inst.StoppedAt = &stopped.String
	}
	inst.RestartPolicy = types.RestartPolicy(restartPolicy)
	return &inst, nil
}

func (s *Store) DeleteInstance(id string) error {
	_, err := s.DB.Exec(`DELETE FROM instances WHERE id = ?`, id)
	return err
}

func nullablePID(pid int) any {
	if pid == 0 {
		return nil
	}
	return pid
}
