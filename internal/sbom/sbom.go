// Package sbom generates Software Bill of Materials (SBOM) and provenance
// attestations for .cap images.
//
// SBOM format: SPDX 2.3 JSON (https://spdx.github.io/spdx-spec/v2.3/)
// Provenance format: SLSA v0.2-inspired JSON
//
// Both can be generated standalone (--out FILE) or embedded inside the .cap
// archive under attestations/sbom.spdx.json and attestations/provenance.json.
package sbom

import (
	"capper/internal/iox"
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"capper/internal/types"
)

// ---- SPDX 2.3 minimal structures ----------------------------------------

// Document is a minimal SPDX 2.3 JSON document describing a capsule image.
type Document struct {
	SPDXVersion       string       `json:"spdxVersion"`
	DataLicense       string       `json:"dataLicense"`
	SPDXID            string       `json:"SPDXID"`
	Name              string       `json:"name"`
	DocumentNamespace string       `json:"documentNamespace"`
	CreationInfo      CreationInfo `json:"creationInfo"`
	Packages          []Package    `json:"packages"`
}

// CreationInfo records who/when generated the document.
type CreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

// Package describes the capsule as an SPDX package.
type Package struct {
	SPDXID           string     `json:"SPDXID"`
	Name             string     `json:"name"`
	Version          string     `json:"versionInfo"`
	DownloadLocation string     `json:"downloadLocation"`
	FilesAnalyzed    bool       `json:"filesAnalyzed"`
	Checksums        []Checksum `json:"packageChecksums,omitempty"`
	ExternalRefs     []ExtRef   `json:"externalRefs,omitempty"`
}

// Checksum holds an algorithm/value pair.
type Checksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"checksumValue"`
}

// ExtRef is an external reference attached to an SPDX package.
type ExtRef struct {
	ReferenceCategory string `json:"referenceCategory"`
	ReferenceType     string `json:"referenceType"`
	ReferenceLocator  string `json:"referenceLocator"`
}

// GenerateSPDX builds a minimal SPDX 2.3 document from a CapsuleManifest.
// imageDigest should be in "sha256:hexhex" form; pass "" if unavailable.
func GenerateSPDX(manifest types.CapsuleManifest, imageDigest string) *Document {
	now := time.Now().UTC().Format(time.RFC3339)
	ns := fmt.Sprintf("https://capper.local/sbom/%s/%s/%s", manifest.Name, manifest.Version, now)

	pkg := Package{
		SPDXID:           "SPDXRef-Package",
		Name:             manifest.Name,
		Version:          manifest.Version,
		DownloadLocation: "NOASSERTION",
		FilesAnalyzed:    false,
	}
	if imageDigest != "" {
		parts := strings.SplitN(imageDigest, ":", 2)
		if len(parts) == 2 {
			pkg.Checksums = []Checksum{{
				Algorithm: strings.ToUpper(parts[0]),
				Value:     parts[1],
			}}
		}
	}
	if manifest.Network.Enabled {
		pkg.ExternalRefs = append(pkg.ExternalRefs, ExtRef{
			ReferenceCategory: "OTHER",
			ReferenceType:     "capper-network-enabled",
			ReferenceLocator:  "true",
		})
	}

	return &Document{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              manifest.Name + "-" + manifest.Version,
		DocumentNamespace: ns,
		CreationInfo: CreationInfo{
			Created:  now,
			Creators: []string{"Tool: capper"},
		},
		Packages: []Package{pkg},
	}
}

// ---- Provenance ---------------------------------------------------------

// Provenance is a minimal SLSA v0.2-inspired provenance record.
type Provenance struct {
	Type       string     `json:"_type"`
	Subject    []Subject  `json:"subject"`
	Builder    Builder    `json:"builder"`
	BuildType  string     `json:"buildType"`
	Invocation Invocation `json:"invocation"`
	Metadata   Metadata   `json:"metadata"`
}

// Subject names the artifact this provenance applies to.
type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// Builder identifies the tool that produced the artifact.
type Builder struct {
	ID string `json:"id"`
}

// Invocation captures the command/entrypoint that produced the artifact.
type Invocation struct {
	ConfigSource ConfigSource `json:"configSource"`
	Parameters   Parameters   `json:"parameters"`
}

// ConfigSource points to the capsule manifest.
type ConfigSource struct {
	EntryPoint string `json:"entryPoint"`
}

// Parameters records the entrypoint and resource limits used.
type Parameters struct {
	Entrypoint []string              `json:"entrypoint"`
	Resources  types.ResourceLimits  `json:"resources,omitempty"`
	Network    bool                  `json:"networkEnabled"`
}

// Metadata records build timing.
type Metadata struct {
	BuildFinishedOn string `json:"buildFinishedOn"`
	Reproducible    bool   `json:"reproducible"`
}

// GenerateProvenance builds a provenance record from a CapsuleManifest.
// imageDigest should be in "sha256:hexhex" form; pass "" if unavailable.
func GenerateProvenance(manifest types.CapsuleManifest, imageName, imageDigest string) *Provenance {
	now := time.Now().UTC().Format(time.RFC3339)

	subject := Subject{Name: imageName, Digest: map[string]string{}}
	if imageDigest != "" {
		parts := strings.SplitN(imageDigest, ":", 2)
		if len(parts) == 2 {
			subject.Digest[parts[0]] = parts[1]
		}
	}

	return &Provenance{
		Type:    "https://in-toto.io/Statement/v0.1",
		Subject: []Subject{subject},
		Builder: Builder{ID: "https://capper.local/builder/v0"},
		BuildType: "https://capper.local/buildTypes/capsule/v0",
		Invocation: Invocation{
			ConfigSource: ConfigSource{EntryPoint: "capsule.json"},
			Parameters: Parameters{
				Entrypoint: manifest.Entrypoint,
				Resources:  manifest.Resources,
				Network:    manifest.Network.Enabled,
			},
		},
		Metadata: Metadata{
			BuildFinishedOn: now,
			Reproducible:    false,
		},
	}
}

// ---- Archive embedding --------------------------------------------------

const (
	EntryNameSBOM       = "attestations/sbom.spdx.json"
	EntryNameProvenance = "attestations/provenance.json"
)

// EmbedInCap adds (or replaces) an entry in a .cap archive and updates
// checksums.json to include the new entry's sha256 digest.
// dst may equal src for an in-place update.
func EmbedInCap(src, dst, entryName string, data []byte) error {
	// Compute the sha256 digest of the new entry so checksums.json can be updated.
	sum := sha256.Sum256(data)
	entryDigest := "sha256:" + hex.EncodeToString(sum[:])

	tmp, err := os.CreateTemp(filepath.Dir(dst), "cap-attest-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	if err := repackWithEntry(src, tmp, entryName, data, entryDigest); err != nil {
		return fmt.Errorf("repack archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, dst)
}

// ExtractFromCap reads a single named entry from a .cap archive.
// Returns os.ErrNotExist if the entry is not present.
func ExtractFromCap(capPath, entryName string) ([]byte, error) {
	f, err := os.Open(capPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%w: %s not found in archive", os.ErrNotExist, entryName)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == entryName {
			return io.ReadAll(tr)
		}
	}
}

func repackWithEntry(src string, dst io.Writer, entryName string, entryData []byte, entryDigest string) error {
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
		// Skip the entry being replaced; we append the new version below.
		if hdr.Name == entryName {
			if _, err := iox.CopyLimited(io.Discard, tr, iox.DefaultMaxArchiveEntry); err != nil {
				return err
			}
			continue
		}
		// Update checksums.json to include the new entry's digest.
		if hdr.Name == "checksums.json" {
			raw, err := io.ReadAll(tr)
			if err != nil {
				return err
			}
			var sums types.Checksums
			if err := json.Unmarshal(raw, &sums); err != nil {
				return fmt.Errorf("parse checksums.json: %w", err)
			}
			if sums.Files == nil {
				sums.Files = make(map[string]string)
			}
			sums.Files[entryName] = entryDigest
			updated, err := json.MarshalIndent(sums, "", "  ")
			if err != nil {
				return err
			}
			updated = append(updated, '\n')
			updHdr := *hdr
			updHdr.Size = int64(len(updated))
			if err := tw.WriteHeader(&updHdr); err != nil {
				return err
			}
			if _, err := tw.Write(updated); err != nil {
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

	hdr := &tar.Header{
		Name: entryName,
		Mode: 0o644,
		Size: int64(len(entryData)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(entryData)
	return err
}

// MarshalJSON encodes v as indented JSON with a trailing newline.
func MarshalJSON(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
