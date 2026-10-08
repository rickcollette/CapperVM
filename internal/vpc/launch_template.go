package vpc

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// LaunchTemplate is a reusable instance launch definition.
type LaunchTemplate struct {
	ID             string `json:"id"`
	Project        string `json:"project"`
	Name           string `json:"name"`
	DefaultVersion int    `json:"defaultVersion"`
	LatestVersion  int    `json:"latestVersion"`
	CreatedAt      string `json:"createdAt"`
}

// LaunchTemplateVersion holds versioned launch config JSON.
type LaunchTemplateVersion struct {
	ID         string         `json:"id"`
	TemplateID string         `json:"templateId"`
	Version    int            `json:"version"`
	Config     map[string]any `json:"config"`
	CreatedAt  string         `json:"createdAt"`
}

func (s *Store) InsertLaunchTemplate(t LaunchTemplate) error {
	_, err := s.db.Exec(
		`INSERT INTO capvpc_launch_templates (id, project, name, default_version, latest_version, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.Project, t.Name, t.DefaultVersion, t.LatestVersion, t.CreatedAt,
	)
	return err
}

func (s *Store) InsertLaunchTemplateVersion(v LaunchTemplateVersion) error {
	cfg, _ := json.Marshal(v.Config)
	_, err := s.db.Exec(
		`INSERT INTO capvpc_launch_template_versions (id, template_id, version, config_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		v.ID, v.TemplateID, v.Version, string(cfg), v.CreatedAt,
	)
	return err
}

func (s *Store) ListLaunchTemplates(project string) ([]LaunchTemplate, error) {
	rows, err := s.db.Query(`SELECT id, project, name, default_version, latest_version, created_at FROM capvpc_launch_templates WHERE project=? ORDER BY name`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LaunchTemplate
	for rows.Next() {
		var t LaunchTemplate
		if err := rows.Scan(&t.ID, &t.Project, &t.Name, &t.DefaultVersion, &t.LatestVersion, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetLaunchTemplateVersion(templateID string, version int) (LaunchTemplateVersion, error) {
	var v LaunchTemplateVersion
	var cfgJSON string
	err := s.db.QueryRow(
		`SELECT id, template_id, version, config_json, created_at FROM capvpc_launch_template_versions WHERE template_id=? AND version=?`,
		templateID, version,
	).Scan(&v.ID, &v.TemplateID, &v.Version, &cfgJSON, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return v, fmt.Errorf("launch template version %d not found", version)
	}
	if err != nil {
		return v, err
	}
	_ = json.Unmarshal([]byte(cfgJSON), &v.Config)
	return v, nil
}

func (s *Store) UpdateLaunchTemplateVersions(templateID string, latest int) error {
	_, err := s.db.Exec(`UPDATE capvpc_launch_templates SET latest_version=? WHERE id=?`, latest, templateID)
	return err
}

func (s *Store) ListLaunchTemplateVersions(templateID string) ([]LaunchTemplateVersion, error) {
	rows, err := s.db.Query(
		`SELECT id, template_id, version, config_json, created_at FROM capvpc_launch_template_versions WHERE template_id=? ORDER BY version`,
		templateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LaunchTemplateVersion
	for rows.Next() {
		var v LaunchTemplateVersion
		var cfgJSON string
		if err := rows.Scan(&v.ID, &v.TemplateID, &v.Version, &cfgJSON, &v.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(cfgJSON), &v.Config)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetLaunchTemplate(project, nameOrID string) (LaunchTemplate, error) {
	var t LaunchTemplate
	err := s.db.QueryRow(
		`SELECT id, project, name, default_version, latest_version, created_at FROM capvpc_launch_templates WHERE project=? AND (id=? OR name=?)`,
		project, nameOrID, nameOrID,
	).Scan(&t.ID, &t.Project, &t.Name, &t.DefaultVersion, &t.LatestVersion, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return t, fmt.Errorf("launch template %q not found", nameOrID)
	}
	return t, err
}

func (m *Manager) CreateLaunchTemplate(project, name string, config map[string]any) (LaunchTemplate, error) {
	t := LaunchTemplate{
		ID:             newID("lt"),
		Project:        project,
		Name:           name,
		DefaultVersion: 1,
		LatestVersion:  1,
		CreatedAt:      now(),
	}
	if err := m.store.InsertLaunchTemplate(t); err != nil {
		return LaunchTemplate{}, err
	}
	v := LaunchTemplateVersion{
		ID:         newID("ltv"),
		TemplateID: t.ID,
		Version:    1,
		Config:     config,
		CreatedAt:  now(),
	}
	if err := m.store.InsertLaunchTemplateVersion(v); err != nil {
		return LaunchTemplate{}, err
	}
	return t, nil
}

func (m *Manager) ListLaunchTemplates(project string) ([]LaunchTemplate, error) {
	return m.store.ListLaunchTemplates(project)
}

func (m *Manager) GetLaunchTemplate(project, ref string) (LaunchTemplate, error) {
	return m.store.GetLaunchTemplate(project, ref)
}

func (m *Manager) ListLaunchTemplateVersions(project, templateRef string) ([]LaunchTemplateVersion, error) {
	t, err := m.store.GetLaunchTemplate(project, templateRef)
	if err != nil {
		return nil, err
	}
	return m.store.ListLaunchTemplateVersions(t.ID)
}

func (m *Manager) GetLaunchTemplateVersion(project, templateRef string, version int) (LaunchTemplateVersion, error) {
	t, err := m.store.GetLaunchTemplate(project, templateRef)
	if err != nil {
		return LaunchTemplateVersion{}, err
	}
	if version == 0 {
		version = t.DefaultVersion
	}
	return m.store.GetLaunchTemplateVersion(t.ID, version)
}

func (m *Manager) CreateLaunchTemplateVersion(project, templateRef string, config map[string]any) (LaunchTemplateVersion, error) {
	t, err := m.store.GetLaunchTemplate(project, templateRef)
	if err != nil {
		return LaunchTemplateVersion{}, err
	}
	next := t.LatestVersion + 1
	v := LaunchTemplateVersion{
		ID:         newID("ltv"),
		TemplateID: t.ID,
		Version:    next,
		Config:     config,
		CreatedAt:  now(),
	}
	if err := m.store.InsertLaunchTemplateVersion(v); err != nil {
		return LaunchTemplateVersion{}, err
	}
	t.LatestVersion = next
	_ = m.store.UpdateLaunchTemplateVersions(t.ID, next)
	return v, nil
}

// ResolveLaunchConfig merges a launch template version into a create request map.
func (m *Manager) ResolveLaunchConfig(project, templateRef string, version int, overrides map[string]any) (map[string]any, error) {
	v, err := m.GetLaunchTemplateVersion(project, templateRef, version)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for k, val := range v.Config {
		out[k] = val
	}
	for k, val := range overrides {
		if val != nil && val != "" {
			out[k] = val
		}
	}
	return out, nil
}
