package marketplace

import (
	"capper/internal/iox"
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// ExtractSBOMDigest reads the SPDX SBOM from the image artifact tar and returns
// its SHA-256 digest as "sha256:<hex>". Returns an error if the SBOM is absent.
func ExtractSBOMDigest(artifactPath string) (string, error) {
	data, err := readTarEntry(artifactPath, "attestations/sbom.spdx.json", 8<<20)
	if err != nil {
		return "", fmt.Errorf("SBOM not found in artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// SPDXDocument is a minimal SPDX 2.3 software bill of materials document.
type SPDXDocument struct {
	SPDXVersion       string           `json:"spdxVersion"`
	DataLicense       string           `json:"dataLicense"`
	SPDXID            string           `json:"SPDXID"`
	Name              string           `json:"name"`
	DocumentNamespace string           `json:"documentNamespace"`
	CreationInfo      SPDXCreationInfo `json:"creationInfo"`
	Packages          []SPDXPackage    `json:"packages"`
}

type SPDXCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type SPDXPackage struct {
	SPDXID           string          `json:"SPDXID"`
	Name             string          `json:"name"`
	Version          string          `json:"versionInfo"`
	DownloadLocation string          `json:"downloadLocation"`
	FilesAnalyzed    bool            `json:"filesAnalyzed"`
	ExternalRefs     []SPDXExternalRef `json:"externalRefs,omitempty"`
}

type SPDXExternalRef struct {
	Category string `json:"referenceCategory"`
	Type     string `json:"referenceType"`
	Locator  string `json:"referenceLocator"`
}

// GenerateSBOM produces a minimal SPDX 2.3 SBOM for an image artifact,
// listing the image itself and any layer checksums as packages.
func GenerateSBOM(imageName, imageVersion, imageDigest string, layers []string) SPDXDocument {
	digestShort := imageDigest
	if len(digestShort) > 12 {
		digestShort = digestShort[:12]
	}
	ns := fmt.Sprintf("https://capper.local/sbom/%s/%s/%s", imageName, imageVersion, digestShort)
	doc := SPDXDocument{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              imageName,
		DocumentNamespace: ns,
		CreationInfo: SPDXCreationInfo{
			Created:  time.Now().UTC().Format(time.RFC3339),
			Creators: []string{"Tool: Capper"},
		},
	}
	doc.Packages = append(doc.Packages, SPDXPackage{
		SPDXID:           "SPDXRef-image",
		Name:             imageName,
		Version:          imageVersion,
		DownloadLocation: "NOASSERTION",
		FilesAnalyzed:    false,
		ExternalRefs: []SPDXExternalRef{{
			Category: "PACKAGE-MANAGER",
			Type:     "purl",
			Locator:  fmt.Sprintf("pkg:oci/%s@%s", imageName, imageDigest),
		}},
	})
	for i, digest := range layers {
		ver := digest
		if len(ver) > 16 {
			ver = ver[:16]
		}
		doc.Packages = append(doc.Packages, SPDXPackage{
			SPDXID:           fmt.Sprintf("SPDXRef-layer-%d", i),
			Name:             fmt.Sprintf("%s-layer-%d", imageName, i),
			Version:          ver,
			DownloadLocation: "NOASSERTION",
			FilesAnalyzed:    false,
		})
	}
	return doc
}

// EmbedSBOMInArtifact adds attestations/sbom.spdx.json to an existing tar file.
func EmbedSBOMInArtifact(artifactPath string, sbom SPDXDocument) error {
	data, err := json.Marshal(sbom)
	if err != nil {
		return err
	}
	return addTarEntry(artifactPath, "attestations/sbom.spdx.json", data)
}

// addTarEntry appends a new file entry to the tar archive at path.
// It rewrites the file in-place using a temporary file.
func addTarEntry(path, name string, data []byte) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tr := tar.NewReader(in)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Name == name {
			// Skip existing entry — we'll write the new one at the end.
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := iox.CopyLimited(tw, tr, iox.DefaultMaxArchiveEntry); err != nil {
			return err
		}
	}
	// Append new entry.
	hdr := &tar.Header{
		Name:     name,
		Size:     int64(len(data)),
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
