package iam

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Store handles all IAM persistence operations.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by an already-initialised database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// DB returns the underlying *sql.DB. Used by tests and cross-package helpers
// that need to run supplementary schema migrations on the same connection.
func (s *Store) DB() *sql.DB { return s.db }

// InitSchema creates all IAM tables and applies additive migrations.
// Safe to call multiple times.
func InitSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS iam_users (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			local_user TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS iam_group_members (
			group_id TEXT NOT NULL,
			user_id  TEXT NOT NULL,
			PRIMARY KEY(group_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS iam_groups (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS iam_policies (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			statements TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS iam_roles (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS iam_role_policies (
			role_id   TEXT NOT NULL,
			policy_id TEXT NOT NULL,
			PRIMARY KEY(role_id, policy_id)
		)`,
		`CREATE TABLE IF NOT EXISTS iam_grants (
			id             TEXT PRIMARY KEY,
			principal_type TEXT NOT NULL,
			principal_id   TEXT NOT NULL,
			role_id        TEXT NOT NULL,
			resource_scope TEXT NOT NULL DEFAULT '*',
			created_at     TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS iam_service_accounts (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			project    TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS iam_sa_roles (
			sa_id   TEXT NOT NULL,
			role_id TEXT NOT NULL,
			PRIMARY KEY(sa_id, role_id)
		)`,
		`CREATE TABLE IF NOT EXISTS iam_tokens (
			id             TEXT PRIMARY KEY,
			name           TEXT NOT NULL DEFAULT '',
			principal_type TEXT NOT NULL,
			principal_id   TEXT NOT NULL,
			expires_at     TEXT NOT NULL,
			created_at     TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS iam_audit (
			id             TEXT PRIMARY KEY,
			principal_type TEXT NOT NULL,
			principal_id   TEXT NOT NULL,
			action         TEXT NOT NULL,
			resource       TEXT NOT NULL,
			decision       TEXT NOT NULL,
			policy_id      TEXT NOT NULL DEFAULT '',
			timestamp      TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS iam_audit_ts ON iam_audit(timestamp)`,
		`CREATE INDEX IF NOT EXISTS iam_grants_principal ON iam_grants(principal_type, principal_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("iam: schema: %w", err)
		}
	}

	// Additive migrations — ignore "duplicate column" errors.
	migrations := []string{
		`ALTER TABLE iam_users ADD COLUMN account_id TEXT NOT NULL DEFAULT 'acct_local'`,
		`ALTER TABLE iam_groups ADD COLUMN account_id TEXT NOT NULL DEFAULT 'acct_local'`,
		`ALTER TABLE iam_groups ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE iam_roles ADD COLUMN account_id TEXT NOT NULL DEFAULT 'acct_local'`,
		`ALTER TABLE iam_roles ADD COLUMN trust_policy TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE iam_policies ADD COLUMN account_id TEXT NOT NULL DEFAULT 'acct_local'`,
		`ALTER TABLE iam_policies ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE iam_policies ADD COLUMN document_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE iam_policies ADD COLUMN managed BOOLEAN NOT NULL DEFAULT 0`,
		`ALTER TABLE iam_policies ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE iam_service_accounts ADD COLUMN account_id TEXT NOT NULL DEFAULT 'acct_local'`,
		`ALTER TABLE iam_service_accounts ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE iam_users ADD COLUMN email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE iam_users ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`,
		`ALTER TABLE iam_users ADD COLUMN provider TEXT NOT NULL DEFAULT 'local'`,
		`ALTER TABLE iam_users ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE iam_users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("iam: migration: %w", err)
		}
	}
	return nil
}

// ---- users ------------------------------------------------------------------

// userColumns is the canonical column list/scan order for full User rows.
const userColumns = `id, name, email, local_user, status, provider, created_at, must_change_password`

func scanUser(sc interface{ Scan(...any) error }) (User, error) {
	var u User
	var mustChange int
	err := sc.Scan(&u.ID, &u.Name, &u.Email, &u.LocalUser, &u.Status, &u.Provider, &u.CreatedAt, &mustChange)
	u.MustChangePassword = mustChange != 0
	return u, err
}

func (s *Store) InsertUser(u User) error {
	if u.CreatedAt == "" {
		u.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if u.Status == "" {
		u.Status = UserStatusActive
	}
	if u.Provider == "" {
		u.Provider = "local"
	}
	mustChange := 0
	if u.MustChangePassword {
		mustChange = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_users(id, name, email, local_user, status, provider, created_at, must_change_password) VALUES(?,?,?,?,?,?,?,?)`,
		u.ID, u.Name, u.Email, u.LocalUser, u.Status, u.Provider, u.CreatedAt, mustChange,
	)
	return err
}

// SetMustChangePassword flags (or clears) the forced password-change requirement.
func (s *Store) SetMustChangePassword(idOrName string, must bool) error {
	v := 0
	if must {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE iam_users SET must_change_password=? WHERE id=? OR name=?`, v, idOrName, idOrName)
	return err
}

// SetEmail updates a user's email (self-service or admin).
func (s *Store) SetEmail(idOrName, email string) error {
	res, err := s.db.Exec(`UPDATE iam_users SET email=? WHERE id=? OR name=?`, email, idOrName, idOrName)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return notFound("user", idOrName, sql.ErrNoRows)
	}
	return nil
}

func (s *Store) GetUser(nameOrID string) (User, error) {
	row := s.db.QueryRow(
		`SELECT `+userColumns+` FROM iam_users WHERE id=? OR name=? LIMIT 1`,
		nameOrID, nameOrID,
	)
	u, err := scanUser(row)
	if err != nil {
		return User{}, notFound("user", nameOrID, err)
	}
	u.Groups, _ = s.groupsForUser(u.ID)
	return u, nil
}

func (s *Store) GetUserByLocalUser(localUser string) (User, error) {
	row := s.db.QueryRow(
		`SELECT `+userColumns+` FROM iam_users WHERE local_user=? LIMIT 1`,
		localUser,
	)
	u, err := scanUser(row)
	if err != nil {
		return User{}, notFound("user", localUser, err)
	}
	return u, nil
}

// GetUserByEmail looks up a user by their (case-insensitive) email address.
func (s *Store) GetUserByEmail(email string) (User, error) {
	row := s.db.QueryRow(
		`SELECT `+userColumns+` FROM iam_users WHERE email<>'' AND lower(email)=lower(?) LIMIT 1`,
		email,
	)
	u, err := scanUser(row)
	if err != nil {
		return User{}, notFound("user", email, err)
	}
	u.Groups, _ = s.groupsForUser(u.ID)
	return u, nil
}

// CountUsers returns the total number of user records.
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM iam_users`).Scan(&n)
	return n, err
}

// CountUsersByProvider counts users created via a given identity provider. Used
// to detect the first SSO (e.g. "google") login — which is promoted to admin —
// without counting the local CLI/OS user that Bootstrap always creates.
func (s *Store) CountUsersByProvider(provider string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM iam_users WHERE provider=?`, provider).Scan(&n)
	return n, err
}

// SetPasswordHash stores a user's password hash (empty disables password login).
func (s *Store) SetPasswordHash(idOrName, hash string) error {
	res, err := s.db.Exec(`UPDATE iam_users SET password_hash=? WHERE id=? OR name=?`, hash, idOrName, idOrName)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return notFound("user", idOrName, sql.ErrNoRows)
	}
	return nil
}

// GetPasswordHash returns the stored password hash for a user (by id or name).
func (s *Store) GetPasswordHash(idOrName string) (string, error) {
	var h string
	err := s.db.QueryRow(`SELECT password_hash FROM iam_users WHERE id=? OR name=? LIMIT 1`, idOrName, idOrName).Scan(&h)
	if err != nil {
		return "", notFound("user", idOrName, err)
	}
	return h, nil
}

// SetUserStatus updates a user's access lifecycle state.
func (s *Store) SetUserStatus(idOrName, status string) error {
	res, err := s.db.Exec(`UPDATE iam_users SET status=? WHERE id=? OR name=?`, status, idOrName, idOrName)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return notFound("user", idOrName, sql.ErrNoRows)
	}
	return nil
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT ` + userColumns + ` FROM iam_users ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) DeleteUser(nameOrID string) error {
	u, err := s.GetUser(nameOrID)
	if err != nil {
		return err
	}
	// Cascade: revoke the user's role grants, group memberships, and API tokens.
	// Orphaned tokens are a security hole — they would keep authenticating as the
	// removed principal.
	_, _ = s.db.Exec(`DELETE FROM iam_grants WHERE principal_type='user' AND principal_id=?`, u.ID)
	_, _ = s.db.Exec(`DELETE FROM iam_group_members WHERE user_id=?`, u.ID)
	_, _ = s.db.Exec(`DELETE FROM iam_tokens WHERE principal_type='user' AND principal_id=?`, u.ID)
	_, err = s.db.Exec(`DELETE FROM iam_users WHERE id=?`, u.ID)
	return err
}

// ---- groups -----------------------------------------------------------------

func (s *Store) InsertGroup(g Group) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if g.CreatedAt == "" {
		g.CreatedAt = now
	}
	_, err := s.db.Exec(`INSERT INTO iam_groups(id, name, created_at) VALUES(?,?,?)`,
		g.ID, g.Name, g.CreatedAt)
	return err
}

func (s *Store) GetGroup(nameOrID string) (Group, error) {
	row := s.db.QueryRow(
		`SELECT id, name, created_at FROM iam_groups WHERE id=? OR name=? LIMIT 1`,
		nameOrID, nameOrID,
	)
	var g Group
	if err := row.Scan(&g.ID, &g.Name, &g.CreatedAt); err != nil {
		return Group{}, notFound("group", nameOrID, err)
	}
	g.Members, _ = s.membersOfGroup(g.ID)
	return g, nil
}

func (s *Store) AddGroupMember(groupNameOrID, userNameOrID string) error {
	g, err := s.GetGroup(groupNameOrID)
	if err != nil {
		return err
	}
	u, err := s.GetUser(userNameOrID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO iam_group_members(group_id, user_id) VALUES(?,?)`, g.ID, u.ID)
	return err
}

func (s *Store) RemoveGroupMember(groupNameOrID, userNameOrID string) error {
	g, err := s.GetGroup(groupNameOrID)
	if err != nil {
		return err
	}
	u, err := s.GetUser(userNameOrID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM iam_group_members WHERE group_id=? AND user_id=?`, g.ID, u.ID)
	return err
}

func (s *Store) membersOfGroup(groupID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT user_id FROM iam_group_members WHERE group_id=?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

func (s *Store) groupsForUser(userID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT group_id FROM iam_group_members WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

// ---- policies ---------------------------------------------------------------

func (s *Store) InsertPolicy(p Policy) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	stmts, err := json.Marshal(p.Statements)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO iam_policies(id, name, statements, created_at) VALUES(?,?,?,?)`,
		p.ID, p.Name, string(stmts), p.CreatedAt,
	)
	return err
}

func (s *Store) GetPolicy(nameOrID string) (Policy, error) {
	row := s.db.QueryRow(
		`SELECT id, name, statements, created_at FROM iam_policies WHERE id=? OR name=? LIMIT 1`,
		nameOrID, nameOrID,
	)
	return scanPolicy(row)
}

func (s *Store) ListPolicies() ([]Policy, error) {
	rows, err := s.db.Query(`SELECT id, name, statements, created_at FROM iam_policies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeletePolicy(nameOrID string) error {
	p, err := s.GetPolicy(nameOrID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM iam_policies WHERE id=?`, p.ID)
	return err
}

// ---- roles ------------------------------------------------------------------

func (s *Store) InsertRole(r Role) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if r.CreatedAt == "" {
		r.CreatedAt = now
	}
	_, err := s.db.Exec(`INSERT INTO iam_roles(id, name, created_at) VALUES(?,?,?)`,
		r.ID, r.Name, r.CreatedAt)
	return err
}

func (s *Store) GetRole(nameOrID string) (Role, error) {
	row := s.db.QueryRow(
		`SELECT id, name, created_at FROM iam_roles WHERE id=? OR name=? LIMIT 1`,
		nameOrID, nameOrID,
	)
	var r Role
	if err := row.Scan(&r.ID, &r.Name, &r.CreatedAt); err != nil {
		return Role{}, notFound("role", nameOrID, err)
	}
	r.Policies, _ = s.policiesForRole(r.ID)
	return r, nil
}

func (s *Store) ListRoles() ([]Role, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at FROM iam_roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Policies, _ = s.policiesForRole(r.ID)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) AttachPolicy(roleNameOrID, policyNameOrID string) error {
	role, err := s.GetRole(roleNameOrID)
	if err != nil {
		return err
	}
	pol, err := s.GetPolicy(policyNameOrID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO iam_role_policies(role_id, policy_id) VALUES(?,?)`,
		role.ID, pol.ID)
	return err
}

func (s *Store) DetachPolicy(roleNameOrID, policyNameOrID string) error {
	role, err := s.GetRole(roleNameOrID)
	if err != nil {
		return err
	}
	pol, err := s.GetPolicy(policyNameOrID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM iam_role_policies WHERE role_id=? AND policy_id=?`, role.ID, pol.ID)
	return err
}

func (s *Store) policiesForRole(roleID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT policy_id FROM iam_role_policies WHERE role_id=?`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

// ---- grants -----------------------------------------------------------------

func (s *Store) InsertGrant(g Grant) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if g.CreatedAt == "" {
		g.CreatedAt = now
	}
	if g.ResourceScope == "" {
		g.ResourceScope = "*"
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_grants(id, principal_type, principal_id, role_id, resource_scope, created_at)
		 VALUES(?,?,?,?,?,?)`,
		g.ID, g.PrincipalType, g.PrincipalID, g.RoleID, g.ResourceScope, g.CreatedAt,
	)
	return err
}

// GrantsForPrincipal returns all grants for a given principal (direct + via groups).
func (s *Store) GrantsForPrincipal(principalType, principalID string) ([]Grant, error) {
	rows, err := s.db.Query(
		`SELECT id, principal_type, principal_id, role_id, resource_scope, created_at
		 FROM iam_grants WHERE principal_type=? AND principal_id=?`,
		principalType, principalID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants, err := scanGrants(rows)
	if err != nil {
		return nil, err
	}

	// If this is a user, also collect grants for groups the user belongs to.
	if principalType == PrincipalUser {
		groupIDs, _ := s.groupsForUser(principalID)
		for _, gid := range groupIDs {
			gRows, err := s.db.Query(
				`SELECT id, principal_type, principal_id, role_id, resource_scope, created_at
				 FROM iam_grants WHERE principal_type=? AND principal_id=?`,
				PrincipalGroup, gid,
			)
			if err != nil {
				continue
			}
			gg, _ := scanGrants(gRows)
			gRows.Close()
			grants = append(grants, gg...)
		}
	}
	return grants, nil
}

func (s *Store) ListGrants() ([]Grant, error) {
	rows, err := s.db.Query(
		`SELECT id, principal_type, principal_id, role_id, resource_scope, created_at
		 FROM iam_grants ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGrants(rows)
}

func (s *Store) DeleteGrant(id string) error {
	res, err := s.db.Exec(`DELETE FROM iam_grants WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("grant %q not found", id)
	}
	return nil
}

// ---- service accounts -------------------------------------------------------

func (s *Store) InsertServiceAccount(sa ServiceAccount) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if sa.CreatedAt == "" {
		sa.CreatedAt = now
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_service_accounts(id, name, project, created_at) VALUES(?,?,?,?)`,
		sa.ID, sa.Name, sa.Project, sa.CreatedAt,
	)
	return err
}

func (s *Store) GetServiceAccount(nameOrID string) (ServiceAccount, error) {
	row := s.db.QueryRow(
		`SELECT id, name, project, created_at FROM iam_service_accounts WHERE id=? OR name=? LIMIT 1`,
		nameOrID, nameOrID,
	)
	var sa ServiceAccount
	if err := row.Scan(&sa.ID, &sa.Name, &sa.Project, &sa.CreatedAt); err != nil {
		return ServiceAccount{}, notFound("service-account", nameOrID, err)
	}
	sa.Roles, _ = s.rolesForSA(sa.ID)
	return sa, nil
}

func (s *Store) ListServiceAccounts() ([]ServiceAccount, error) {
	rows, err := s.db.Query(
		`SELECT id, name, project, created_at FROM iam_service_accounts ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServiceAccount
	for rows.Next() {
		var sa ServiceAccount
		if err := rows.Scan(&sa.ID, &sa.Name, &sa.Project, &sa.CreatedAt); err != nil {
			return nil, err
		}
		sa.Roles, _ = s.rolesForSA(sa.ID)
		out = append(out, sa)
	}
	return out, rows.Err()
}

func (s *Store) rolesForSA(saID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT role_id FROM iam_sa_roles WHERE sa_id=?`, saID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

// ---- tokens -----------------------------------------------------------------

func (s *Store) InsertToken(t Token) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_tokens(id, name, principal_type, principal_id, expires_at, created_at)
		 VALUES(?,?,?,?,?,?)`,
		t.ID, t.Name, t.PrincipalType, t.PrincipalID, t.ExpiresAt, t.CreatedAt,
	)
	return err
}

func (s *Store) GetToken(id string) (Token, error) {
	row := s.db.QueryRow(
		`SELECT id, name, principal_type, principal_id, expires_at, created_at
		 FROM iam_tokens WHERE id=?`, id,
	)
	var t Token
	if err := row.Scan(&t.ID, &t.Name, &t.PrincipalType, &t.PrincipalID, &t.ExpiresAt, &t.CreatedAt); err != nil {
		return Token{}, notFound("token", id, err)
	}
	return t, nil
}

func (s *Store) DeleteToken(id string) error {
	res, err := s.db.Exec(`DELETE FROM iam_tokens WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token %q not found", id)
	}
	return nil
}

func (s *Store) ListTokens(principalType, principalID string) ([]Token, error) {
	query := `SELECT id, name, principal_type, principal_id, expires_at, created_at FROM iam_tokens`
	var args []any
	if principalType != "" && principalID != "" {
		query += ` WHERE principal_type=? AND principal_id=?`
		args = append(args, principalType, principalID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.Name, &t.PrincipalType, &t.PrincipalID, &t.ExpiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---- audit ------------------------------------------------------------------

func (s *Store) InsertAudit(r AuditRecord) error {
	if r.Timestamp == "" {
		r.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_audit(id, principal_type, principal_id, action, resource, decision, policy_id, timestamp)
		 VALUES(?,?,?,?,?,?,?,?)`,
		r.ID, r.PrincipalType, r.PrincipalID, r.Action, r.Resource, r.Decision, r.PolicyID, r.Timestamp,
	)
	return err
}

// ListAudit returns audit records, optionally filtered by action prefix and/or principal.
// Both filters are substring/prefix matches; pass "" to skip.
func (s *Store) ListAudit(actionFilter, principalFilter, since string, limit int) ([]AuditRecord, error) {
	query := `SELECT id, principal_type, principal_id, action, resource, decision, policy_id, timestamp
	          FROM iam_audit WHERE 1=1`
	var args []any
	if actionFilter != "" {
		query += ` AND action LIKE ?`
		args = append(args, actionFilter+"%")
	}
	if principalFilter != "" {
		query += ` AND (principal_id LIKE ? OR principal_type||':'||principal_id LIKE ?)`
		args = append(args, principalFilter+"%", principalFilter+"%")
	}
	if since != "" {
		query += ` AND timestamp >= ?`
		args = append(args, since)
	}
	query += ` ORDER BY timestamp DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditRecord
	for rows.Next() {
		var r AuditRecord
		if err := rows.Scan(&r.ID, &r.PrincipalType, &r.PrincipalID, &r.Action, &r.Resource,
			&r.Decision, &r.PolicyID, &r.Timestamp); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- helpers ----------------------------------------------------------------

func notFound(kind, key string, err error) error {
	if strings.Contains(err.Error(), "no rows") {
		return fmt.Errorf("%s %q not found", kind, key)
	}
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPolicy(s rowScanner) (Policy, error) {
	var p Policy
	var stmts string
	if err := s.Scan(&p.ID, &p.Name, &stmts, &p.CreatedAt); err != nil {
		return Policy{}, err
	}
	if err := json.Unmarshal([]byte(stmts), &p.Statements); err != nil {
		return Policy{}, fmt.Errorf("parse statements: %w", err)
	}
	return p, nil
}

func scanGrants(rows *sql.Rows) ([]Grant, error) {
	var out []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ID, &g.PrincipalType, &g.PrincipalID, &g.RoleID,
			&g.ResourceScope, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func scanStringColumn(rows *sql.Rows) ([]string, error) {
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- account-scoped users ---------------------------------------------------

func (s *Store) ListUsersByAccount(accountID string) ([]User, error) {
	rows, err := s.db.Query(
		`SELECT id, name, email, account_id, local_user, created_at FROM iam_users WHERE account_id=? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.AccountID, &u.LocalUser, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CreateUserWithAccount(accountID, name, email, password string) (User, string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	u := User{
		ID:        newID("usr"),
		Name:      name,
		Email:     email,
		AccountID: accountID,
		CreatedAt: now,
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_users(id, name, email, account_id, local_user, created_at) VALUES(?,?,?,?,?,?)`,
		u.ID, u.Name, u.Email, u.AccountID, "", u.CreatedAt,
	)
	if err != nil {
		return User{}, "", err
	}
	// bearer token placeholder — callers can use Manager.Issue to get a real token
	return u, "", nil
}

func (s *Store) GetUserByAccount(accountID, userID string) (User, error) {
	row := s.db.QueryRow(
		`SELECT id, name, email, account_id, local_user, created_at FROM iam_users WHERE id=? AND account_id=? LIMIT 1`,
		userID, accountID,
	)
	var u User
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.AccountID, &u.LocalUser, &u.CreatedAt); err != nil {
		return User{}, notFound("user", userID, err)
	}
	return u, nil
}

func (s *Store) UpdateUserByAccount(accountID, userID string, updates map[string]string) error {
	allowed := map[string]bool{"name": true, "email": true}
	for k, v := range updates {
		if !allowed[k] {
			continue
		}
		if _, err := s.db.Exec(
			`UPDATE iam_users SET `+k+`=? WHERE id=? AND account_id=?`,
			v, userID, accountID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteUserByAccount(accountID, userID string) error {
	_, err := s.db.Exec(`DELETE FROM iam_users WHERE id=? AND account_id=?`, userID, accountID)
	return err
}

// ---- account-scoped groups --------------------------------------------------

func (s *Store) ListGroupsByAccount(accountID string) ([]Group, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, account_id, created_at FROM iam_groups WHERE account_id=? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.AccountID, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.Members, _ = s.membersOfGroup(g.ID)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CreateGroupByAccount(accountID, name, description string) (Group, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	g := Group{
		ID:          newID("grp"),
		Name:        name,
		Description: description,
		AccountID:   accountID,
		CreatedAt:   now,
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_groups(id, name, description, account_id, created_at) VALUES(?,?,?,?,?)`,
		g.ID, g.Name, g.Description, g.AccountID, g.CreatedAt,
	)
	return g, err
}

func (s *Store) GetGroupByAccount(accountID, groupID string) (Group, error) {
	row := s.db.QueryRow(
		`SELECT id, name, description, account_id, created_at FROM iam_groups WHERE id=? AND account_id=? LIMIT 1`,
		groupID, accountID,
	)
	var g Group
	if err := row.Scan(&g.ID, &g.Name, &g.Description, &g.AccountID, &g.CreatedAt); err != nil {
		return Group{}, notFound("group", groupID, err)
	}
	g.Members, _ = s.membersOfGroup(g.ID)
	return g, nil
}

func (s *Store) UpdateGroupByAccount(accountID, groupID string, updates map[string]string) error {
	allowed := map[string]bool{"name": true, "description": true}
	for k, v := range updates {
		if !allowed[k] {
			continue
		}
		if _, err := s.db.Exec(
			`UPDATE iam_groups SET `+k+`=? WHERE id=? AND account_id=?`,
			v, groupID, accountID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteGroupByAccount(accountID, groupID string) error {
	// Cascade: drop memberships and any role grants made to the group so they
	// aren't left dangling (and can't leak access if a group id is reused).
	_, _ = s.db.Exec(`DELETE FROM iam_group_members WHERE group_id=?`, groupID)
	_, _ = s.db.Exec(`DELETE FROM iam_grants WHERE principal_type='group' AND principal_id=?`, groupID)
	_, err := s.db.Exec(`DELETE FROM iam_groups WHERE id=? AND account_id=?`, groupID, accountID)
	return err
}

func (s *Store) AddGroupMemberByAccount(accountID, groupID, userID string) error {
	// Verify group belongs to account
	if _, err := s.GetGroupByAccount(accountID, groupID); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO iam_group_members(group_id, user_id) VALUES(?,?)`, groupID, userID)
	return err
}

func (s *Store) RemoveGroupMemberByAccount(accountID, groupID, userID string) error {
	if _, err := s.GetGroupByAccount(accountID, groupID); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM iam_group_members WHERE group_id=? AND user_id=?`, groupID, userID)
	return err
}

// ---- account-scoped roles ---------------------------------------------------

func (s *Store) ListRolesByAccount(accountID string) ([]Role, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, account_id, trust_policy, created_at FROM iam_roles WHERE account_id=? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.AccountID, &r.TrustPolicy, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Policies, _ = s.policiesForRole(r.ID)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateRoleByAccount(accountID, name, description, trustPolicy string) (Role, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	r := Role{
		ID:          newID("role"),
		Name:        name,
		Description: description,
		AccountID:   accountID,
		TrustPolicy: trustPolicy,
		CreatedAt:   now,
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_roles(id, name, description, account_id, trust_policy, created_at) VALUES(?,?,?,?,?,?)`,
		r.ID, r.Name, r.Description, r.AccountID, r.TrustPolicy, r.CreatedAt,
	)
	return r, err
}

func (s *Store) GetRoleByAccount(accountID, roleID string) (Role, error) {
	row := s.db.QueryRow(
		`SELECT id, name, description, account_id, trust_policy, created_at FROM iam_roles WHERE id=? AND account_id=? LIMIT 1`,
		roleID, accountID,
	)
	var r Role
	if err := row.Scan(&r.ID, &r.Name, &r.Description, &r.AccountID, &r.TrustPolicy, &r.CreatedAt); err != nil {
		return Role{}, notFound("role", roleID, err)
	}
	r.Policies, _ = s.policiesForRole(r.ID)
	return r, nil
}

func (s *Store) UpdateRoleByAccount(accountID, roleID string, updates map[string]string) error {
	allowed := map[string]bool{"name": true, "description": true, "trust_policy": true}
	for k, v := range updates {
		if !allowed[k] {
			continue
		}
		if _, err := s.db.Exec(
			`UPDATE iam_roles SET `+k+`=? WHERE id=? AND account_id=?`,
			v, roleID, accountID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteRoleByAccount(accountID, roleID string) error {
	_, err := s.db.Exec(`DELETE FROM iam_roles WHERE id=? AND account_id=?`, roleID, accountID)
	return err
}

// ---- account-scoped service accounts ----------------------------------------

func (s *Store) ListServiceAccountsByAccount(accountID string) ([]ServiceAccount, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, account_id, project, created_at FROM iam_service_accounts WHERE account_id=? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServiceAccount
	for rows.Next() {
		var sa ServiceAccount
		if err := rows.Scan(&sa.ID, &sa.Name, &sa.Description, &sa.AccountID, &sa.Project, &sa.CreatedAt); err != nil {
			return nil, err
		}
		sa.Roles, _ = s.rolesForSA(sa.ID)
		out = append(out, sa)
	}
	return out, rows.Err()
}

func (s *Store) CreateServiceAccountByAccount(accountID, name, description string) (ServiceAccount, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	sa := ServiceAccount{
		ID:          newID("sa"),
		Name:        name,
		Description: description,
		AccountID:   accountID,
		CreatedAt:   now,
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_service_accounts(id, name, description, account_id, project, created_at) VALUES(?,?,?,?,?,?)`,
		sa.ID, sa.Name, sa.Description, sa.AccountID, "", sa.CreatedAt,
	)
	return sa, err
}

func (s *Store) DeleteServiceAccountByAccount(accountID, saID string) error {
	_, err := s.db.Exec(`DELETE FROM iam_service_accounts WHERE id=? AND account_id=?`, saID, accountID)
	return err
}

// ---- account-scoped policies ------------------------------------------------

// AccountPolicy is a policy with the full document_json representation.
type AccountPolicy struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	DocumentJSON string `json:"document"`
	Managed      bool   `json:"managed"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func (s *Store) ListPoliciesByAccount(accountID string) ([]AccountPolicy, error) {
	rows, err := s.db.Query(
		`SELECT id, account_id, name, description, document_json, managed, created_at, updated_at
		 FROM iam_policies WHERE account_id=? OR account_id='system' ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountPolicy
	for rows.Next() {
		var p AccountPolicy
		var managed int
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.Description, &p.DocumentJSON, &managed, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Managed = managed != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreatePolicyByAccount(accountID, name, description, documentJSON string) (AccountPolicy, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	p := AccountPolicy{
		ID:           newID("pol"),
		AccountID:    accountID,
		Name:         name,
		Description:  description,
		DocumentJSON: documentJSON,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := s.db.Exec(
		`INSERT INTO iam_policies(id, account_id, name, description, document_json, managed, statements, created_at, updated_at)
		 VALUES(?,?,?,?,?,0,'[]',?,?)`,
		p.ID, p.AccountID, p.Name, p.Description, p.DocumentJSON, p.CreatedAt, p.UpdatedAt,
	)
	return p, err
}

func (s *Store) GetPolicyByAccount(accountID, policyID string) (AccountPolicy, error) {
	row := s.db.QueryRow(
		`SELECT id, account_id, name, description, document_json, managed, created_at, updated_at
		 FROM iam_policies WHERE id=? AND (account_id=? OR account_id='system') LIMIT 1`,
		policyID, accountID,
	)
	var p AccountPolicy
	var managed int
	if err := row.Scan(&p.ID, &p.AccountID, &p.Name, &p.Description, &p.DocumentJSON, &managed, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return AccountPolicy{}, notFound("policy", policyID, err)
	}
	p.Managed = managed != 0
	return p, nil
}

func (s *Store) UpdatePolicyDocumentByAccount(accountID, policyID, documentJSON string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE iam_policies SET document_json=?, updated_at=? WHERE id=? AND account_id=? AND managed=0`,
		documentJSON, now, policyID, accountID,
	)
	return err
}

func (s *Store) DeletePolicyByAccount(accountID, policyID string) error {
	_, err := s.db.Exec(
		`DELETE FROM iam_policies WHERE id=? AND account_id=? AND managed=0`,
		policyID, accountID,
	)
	return err
}

// AttachPolicyByAccount attaches a policy to a principal within an account.
// principalType is "user", "role", "group", or "service-account".
func (s *Store) AttachPolicyByAccount(accountID, policyID, principalType, principalID string) error {
	// Verify policy is accessible
	if _, err := s.GetPolicyByAccount(accountID, policyID); err != nil {
		return err
	}
	switch principalType {
	case "role":
		_, err := s.db.Exec(`INSERT OR IGNORE INTO iam_role_policies(role_id, policy_id) VALUES(?,?)`,
			principalID, policyID)
		return err
	default:
		return fmt.Errorf("unsupported principalType for attach: %s", principalType)
	}
}

// DetachPolicyByAccount detaches a policy from a principal within an account.
func (s *Store) DetachPolicyByAccount(accountID, policyID, principalType, principalID string) error {
	if _, err := s.GetPolicyByAccount(accountID, policyID); err != nil {
		return err
	}
	switch principalType {
	case "role":
		_, err := s.db.Exec(`DELETE FROM iam_role_policies WHERE role_id=? AND policy_id=?`,
			principalID, policyID)
		return err
	default:
		return fmt.Errorf("unsupported principalType for detach: %s", principalType)
	}
}
