// Package sign provides Ed25519 image signing and verification for .cap files.
//
// A signed image carries an additional "signature.json" entry inside the .cap
// archive.  The signature covers the sha256 digest of "checksums.json", which
// in turn covers the rest of the archive contents.  Trusting the signature
// therefore transitively validates the full image.
//
// Key files:
//
//	capper.key  — private key (PEM-encoded PKCS#8 Ed25519 private key)
//	capper.pub  — public key  (PEM-encoded PKIX Ed25519 public key)
package sign

import (
	"capper/internal/iox"
	"archive/tar"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const signatureFile = "signature.json"

// Signature is stored as signature.json inside the .cap archive.
type Signature struct {
	Algorithm string `json:"algorithm"` // "ed25519"
	Digest    string `json:"digest"`    // sha256 digest of checksums.json being signed
	Value     string `json:"value"`     // hex-encoded signature bytes
}

// GenerateKeyPair creates a new Ed25519 key pair and writes them to keyPath
// (private) and pubPath (public) in PEM format.
func GenerateKeyPair(keyPath, pubPath string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	if err := os.WriteFile(keyPath, privPEM, 0o600); err != nil {
		return err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return os.WriteFile(pubPath, pubPEM, 0o644)
}

// LoadPrivateKey reads an Ed25519 private key from a PEM file.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found in key file")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("key file does not contain an Ed25519 private key")
	}
	return priv, nil
}

// LoadPublicKey reads an Ed25519 public key from a PEM file.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found in key file")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("key file does not contain an Ed25519 public key")
	}
	return pub, nil
}

// SignImage reads the .cap archive at src, adds a signature over its
// checksums.json entry, and writes the signed archive to dst.
// dst may equal src (the file is atomically replaced).
func SignImage(src, dst string, key ed25519.PrivateKey) error {
	// Extract checksums.json from the archive.
	checksumsData, err := extractEntry(src, "checksums.json")
	if err != nil {
		return fmt.Errorf("read checksums.json: %w", err)
	}

	// Sign the sha256 digest of checksums.json.
	digest := sha256.Sum256(checksumsData)
	digestHex := "sha256:" + hex.EncodeToString(digest[:])
	sig := ed25519.Sign(key, digest[:])

	sigRecord := Signature{
		Algorithm: "ed25519",
		Digest:    digestHex,
		Value:     hex.EncodeToString(sig),
	}
	sigData, err := json.MarshalIndent(sigRecord, "", "  ")
	if err != nil {
		return err
	}
	sigData = append(sigData, '\n')

	// Re-pack the archive, replacing or adding signature.json.
	tmp, err := os.CreateTemp(filepath.Dir(dst), "cap-sign-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	if err := repackWithSignature(src, tmp, sigData); err != nil {
		return fmt.Errorf("repack archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, dst)
}

// VerifyImage checks the Ed25519 signature inside the .cap archive at path
// against the given public key.  Returns nil if the signature is valid.
func VerifyImage(path string, pub ed25519.PublicKey) error {
	checksumsData, err := extractEntry(path, "checksums.json")
	if err != nil {
		return fmt.Errorf("read checksums.json: %w", err)
	}
	sigData, err := extractEntry(path, signatureFile)
	if err != nil {
		return fmt.Errorf("image is not signed (no %s): %w", signatureFile, err)
	}
	var sig Signature
	if err := json.Unmarshal(sigData, &sig); err != nil {
		return fmt.Errorf("parse signature.json: %w", err)
	}
	if sig.Algorithm != "ed25519" {
		return fmt.Errorf("unsupported signature algorithm: %s", sig.Algorithm)
	}
	sigBytes, err := hex.DecodeString(sig.Value)
	if err != nil {
		return fmt.Errorf("decode signature value: %w", err)
	}
	digest := sha256.Sum256(checksumsData)
	if !ed25519.Verify(pub, digest[:], sigBytes) {
		return errors.New("signature verification failed")
	}
	return nil
}

// extractEntry reads a single named file from a .cap (tar) archive.
func extractEntry(capPath, name string) ([]byte, error) {
	f, err := os.Open(capPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("entry %q not found in archive", name)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == name {
			return io.ReadAll(tr)
		}
	}
}

// repackWithSignature copies a .cap archive to dst, replacing or appending
// signature.json.
func repackWithSignature(src string, dst io.Writer, sigData []byte) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	tw := tar.NewWriter(dst)
	defer tw.Close()
	tr := tar.NewReader(f)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Skip any existing signature.json — we append the new one below.
		if hdr.Name == signatureFile {
			if _, err := iox.CopyLimited(io.Discard, tr, iox.DefaultMaxArchiveEntry); err != nil {
				return err
			}
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := iox.CopyLimited(tw, tr, iox.DefaultMaxArchiveEntry); err != nil {
			return err
		}
	}

	// Append the new signature.json.
	hdr := &tar.Header{
		Name: signatureFile,
		Mode: 0o644,
		Size: int64(len(sigData)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(sigData); err != nil {
		return err
	}
	return nil
}
