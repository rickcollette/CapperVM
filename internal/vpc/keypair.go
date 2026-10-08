package vpc

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// KeyPair is an SSH public key for instance access.
type KeyPair struct {
	ID          string `json:"id"`
	Project     string `json:"project"`
	Name        string `json:"name"`
	PublicKey   string `json:"publicKey"`
	Fingerprint string `json:"fingerprint"`
	KeyType     string `json:"keyType"`
	CreatedAt   string `json:"createdAt"`
}

func (s *Store) InsertKeyPair(k KeyPair) error {
	_, err := s.db.Exec(
		`INSERT INTO capvpc_key_pairs (id, project, name, public_key, fingerprint, key_type, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.Project, k.Name, k.PublicKey, k.Fingerprint, k.KeyType, k.CreatedAt,
	)
	return err
}

func (s *Store) GetKeyPair(project, name string) (KeyPair, error) {
	var k KeyPair
	err := s.db.QueryRow(
		`SELECT id, project, name, public_key, fingerprint, key_type, created_at FROM capvpc_key_pairs WHERE project=? AND name=?`,
		project, name,
	).Scan(&k.ID, &k.Project, &k.Name, &k.PublicKey, &k.Fingerprint, &k.KeyType, &k.CreatedAt)
	if err == sql.ErrNoRows {
		return k, fmt.Errorf("key pair %q not found", name)
	}
	return k, err
}

func (s *Store) ListKeyPairs(project string) ([]KeyPair, error) {
	rows, err := s.db.Query(`SELECT id, project, name, public_key, fingerprint, key_type, created_at FROM capvpc_key_pairs WHERE project=? ORDER BY name`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KeyPair
	for rows.Next() {
		var k KeyPair
		if err := rows.Scan(&k.ID, &k.Project, &k.Name, &k.PublicKey, &k.Fingerprint, &k.KeyType, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) DeleteKeyPair(project, name string) error {
	_, err := s.db.Exec(`DELETE FROM capvpc_key_pairs WHERE project=? AND name=?`, project, name)
	return err
}

func FingerprintPublicKey(pub string) string {
	sum := sha256.Sum256([]byte(pub))
	return "SHA256:" + hex.EncodeToString(sum[:])
}

func (m *Manager) ImportKeyPair(project, name, pubKey, keyType string) (KeyPair, error) {
	if keyType == "" {
		keyType = "ssh-rsa"
	}
	k := KeyPair{
		ID:          newID("key"),
		Project:     project,
		Name:        name,
		PublicKey:   pubKey,
		Fingerprint: FingerprintPublicKey(pubKey),
		KeyType:     keyType,
		CreatedAt:   now(),
	}
	if err := m.store.InsertKeyPair(k); err != nil {
		return KeyPair{}, err
	}
	return k, nil
}

func (m *Manager) GetKeyPair(project, name string) (KeyPair, error) {
	return m.store.GetKeyPair(project, name)
}

func (m *Manager) ListKeyPairs(project string) ([]KeyPair, error) {
	return m.store.ListKeyPairs(project)
}

func (m *Manager) DeleteKeyPair(project, name string) error {
	return m.store.DeleteKeyPair(project, name)
}
