package loader

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

	"github.com/klauspost/compress/zstd"
	"golang.org/x/sys/unix"

	"capper/internal/store"
	"capper/internal/types"
)

type Loader struct {
	Paths store.Paths
	Debug bool
}

type LoadedImage struct {
	ImagePath string
	WorkDir   string
	Manifest  types.CapsuleManifest
	Digest    string
}

func (l Loader) ResolveImage(name string) (string, []string, error) {
	if strings.Contains(name, "..") {
		return "", nil, fmt.Errorf("unsafe image path (traversal not allowed): %s", name)
	}
	// For absolute paths, use directly without any store lookup.
	if filepath.IsAbs(name) {
		if st, err := os.Stat(name); err == nil && !st.IsDir() {
			return name, []string{name}, nil
		}
		return "", []string{name}, fmt.Errorf("image not found: %s", name)
	}
	// Relative or bare name: try CWD-relative first, then images store.
	// Also normalise a missing .cap suffix when searching the store.
	storeCandidate := filepath.Join(l.Paths.Images, name)
	if !strings.HasSuffix(name, ".cap") {
		storeCandidate = filepath.Join(l.Paths.Images, name+".cap")
	}
	search := []string{name, storeCandidate}
	for _, candidate := range search {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, search, nil
		}
	}
	return "", search, fmt.Errorf("image not found: %s", name)
}

func (l Loader) Load(imageName string) (*LoadedImage, func(), error) {
	imagePath, _, err := l.ResolveImage(imageName)
	if err != nil {
		return nil, nil, err
	}
	digest, err := FileDigest(imagePath)
	if err != nil {
		return nil, nil, err
	}
	workDir, err := os.MkdirTemp(l.Paths.Tmp, "load-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if !l.Debug {
			_ = os.RemoveAll(workDir)
		}
	}
	if err := ExtractTarFile(imagePath, workDir); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := VerifyChecksums(workDir); err != nil {
		cleanup()
		return nil, nil, err
	}
	manifest, err := ReadManifest(filepath.Join(workDir, "capsule.json"))
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := VerifyManifestDigests(workDir, manifest); err != nil {
		cleanup()
		return nil, nil, err
	}
	return &LoadedImage{ImagePath: imagePath, WorkDir: workDir, Manifest: manifest, Digest: digest}, cleanup, nil
}

func ReadManifest(path string) (types.CapsuleManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return types.CapsuleManifest{}, err
	}
	var manifest types.CapsuleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return types.CapsuleManifest{}, err
	}
	if manifest.CapsuleVersion != "0.1" {
		return types.CapsuleManifest{}, fmt.Errorf("unsupported capsule version: %s", manifest.CapsuleVersion)
	}
	if len(manifest.Entrypoint) == 0 {
		return types.CapsuleManifest{}, fmt.Errorf("capsule manifest entrypoint is required")
	}
	if manifest.RootFS.Archive == "" {
		return types.CapsuleManifest{}, fmt.Errorf("capsule manifest rootfs archive is required")
	}
	return manifest, nil
}

func ExtractRootFS(archivePath, dest, compression string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	var reader io.Reader = file
	var decoder *zstd.Decoder
	if compression == "" || compression == "zstd" {
		decoder, err = zstd.NewReader(file)
		if err != nil {
			return err
		}
		defer decoder.Close()
		reader = decoder
	} else if compression != "none" {
		return fmt.Errorf("unsupported rootfs compression: %s", compression)
	}
	// Use ExtractRootFSTar (not ExtractTar) so that symlinks and hardlinks
	// present in real-world rootfs archives are handled correctly.
	return ExtractRootFSTar(reader, dest)
}

// ExtractRootFSTar extracts a rootfs tar archive into dest, supporting symlinks
// and hardlinks as required by real-world Linux rootfs archives (Alpine, Debian,
// BusyBox etc.). Security invariants:
//   - All paths are validated by safeJoin (no traversal outside dest).
//   - Directory creation uses safeMkdirAll which does not follow symlinks.
//   - Regular-file creation uses O_NOFOLLOW so symlinks already in dest are
//     never followed when writing new files.
//   - Hard-link targets are validated to be within dest.
//   - Device nodes and fifos are rejected.
func ExtractRootFSTar(reader io.Reader, dest string) error {
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(dest, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := safeMkdirAll(dest, target); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := safeMkdirAll(dest, filepath.Dir(target)); err != nil {
				return err
			}
			out, err := openRegularNoFollow(target, mode(hdr))
			if err != nil {
				return err
			}
			if _, err := iox.CopyLimited(out, tr, iox.DefaultMaxArchiveEntry); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := safeMkdirAll(dest, filepath.Dir(target)); err != nil {
				return err
			}
			// Remove any existing entry at the target path before creating the
			// symlink (tar archives may overwrite earlier entries).
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			linkTarget, err := safeJoin(dest, hdr.Linkname)
			if err != nil {
				return fmt.Errorf("unsafe hardlink target %s: %w", hdr.Linkname, err)
			}
			if err := safeMkdirAll(dest, filepath.Dir(target)); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Link(linkTarget, target); err != nil {
				return err
			}
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return fmt.Errorf("unsupported archive device entry: %s", hdr.Name)
		default:
			return fmt.Errorf("unsupported archive entry type %d: %s", hdr.Typeflag, hdr.Name)
		}
	}
}

// safeMkdirAll creates all directories in path (relative to base) without
// following any symlink component. If a symlink exists along the path an error
// is returned — this prevents symlink-redirect attacks during rootfs extraction.
func safeMkdirAll(base, path string) error {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	current := base
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		next := filepath.Join(current, part)
		lfi, lerr := os.Lstat(next)
		if lerr == nil {
			if lfi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in path during directory creation: %s", next)
			}
			current = next
			continue
		}
		if !os.IsNotExist(lerr) {
			return lerr
		}
		if err := os.Mkdir(next, 0o755); err != nil && !os.IsExist(err) {
			return err
		}
		current = next
	}
	return nil
}

func ExtractTarFile(path, dest string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return ExtractTar(file, dest)
}

func ExtractTar(reader io.Reader, dest string) error {
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(dest, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode(hdr)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := openRegularNoFollow(target, mode(hdr))
			if err != nil {
				return err
			}
			if _, err := iox.CopyLimited(out, tr, iox.DefaultMaxArchiveEntry); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("unsupported archive link entry: %s", hdr.Name)
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return fmt.Errorf("unsupported archive device entry: %s", hdr.Name)
		default:
			return fmt.Errorf("unsupported archive entry type %d: %s", hdr.Typeflag, hdr.Name)
		}
	}
}

func VerifyChecksums(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "checksums.json"))
	if err != nil {
		return err
	}
	var sums types.Checksums
	if err := json.Unmarshal(data, &sums); err != nil {
		return err
	}
	if sums.Algorithm != "sha256" {
		return fmt.Errorf("unsupported checksum algorithm: %s", sums.Algorithm)
	}
	for _, required := range []string{"capsule.json", "rootfs.tar.zst"} {
		if _, ok := sums.Files[required]; !ok {
			return fmt.Errorf("missing checksum entry: %s", required)
		}
	}
	for name, expected := range sums.Files {
		if !safeRelativeName(name) {
			return fmt.Errorf("unsafe checksum path: %s", name)
		}
		if !isKnownChecksumEntry(name) {
			return fmt.Errorf("unexpected checksum entry: %s", name)
		}
		target, err := safeJoin(dir, name)
		if err != nil {
			return err
		}
		actual, err := FileDigest(target)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("image checksum verification failed for %s", name)
		}
	}
	return nil
}

func VerifyManifestDigests(dir string, manifest types.CapsuleManifest) error {
	if !safeRelativeName(manifest.RootFS.Archive) {
		return fmt.Errorf("unsafe rootfs archive path: %s", manifest.RootFS.Archive)
	}
	if manifest.RootFS.Archive != "rootfs.tar.zst" {
		return fmt.Errorf("unsupported rootfs archive name: %s", manifest.RootFS.Archive)
	}
	path, err := safeJoin(dir, manifest.RootFS.Archive)
	if err != nil {
		return err
	}
	actual, err := FileDigest(path)
	if err != nil {
		return err
	}
	if manifest.RootFS.Digest == "" {
		return fmt.Errorf("capsule manifest rootfs digest is required")
	}
	if actual != manifest.RootFS.Digest {
		return fmt.Errorf("manifest rootfs digest mismatch")
	}
	return nil
}

func FileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func safeJoin(root, name string) (string, error) {
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("unsafe archive path (absolute): %s", name)
	}
	clean := filepath.Clean(name)
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("unsafe archive path: %s", name)
	}
	return target, nil
}

// isKnownChecksumEntry returns true for archive entries that may appear in
// checksums.json: the two required core files, and attestation entries written
// by `capper attest --embed`.
func isKnownChecksumEntry(name string) bool {
	switch name {
	case "capsule.json", "rootfs.tar.zst":
		return true
	}
	return strings.HasPrefix(name, "attestations/")
}

func safeRelativeName(name string) bool {
	if name == "" || filepath.IsAbs(name) {
		return false
	}
	clean := filepath.Clean(name)
	return clean == name && clean != "." && !strings.HasPrefix(clean, ".."+string(filepath.Separator)) && clean != ".."
}

func openRegularNoFollow(path string, perm os.FileMode) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_WRONLY|unix.O_TRUNC|unix.O_NOFOLLOW|unix.O_CLOEXEC, uint32(perm.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func mode(hdr *tar.Header) os.FileMode {
	if hdr.Mode == 0 {
		return 0o644
	}
	return os.FileMode(hdr.Mode)
}
