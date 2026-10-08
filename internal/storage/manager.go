package storage

import (
	"capper/internal/iox"
	"archive/tar"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"capper/internal/s3server"
)

// Paths carries the root directories used for storage data.
type Paths struct {
	Volumes   string // ~/.local/share/capper/storage/volumes
	Buckets   string // ~/.local/share/capper/storage/objects
	Snapshots string // ~/.local/share/capper/storage/snapshots
}

// Manager provides high-level storage lifecycle operations.
// KMSEncryptor is injected to avoid a circular import with capper/internal/kms.
// When set, objects stored in KMS-enabled buckets are encrypted at rest.
type KMSEncryptor interface {
	Encrypt(keyName, project string, plaintext []byte) ([]byte, error)
	Decrypt(keyName, project string, ciphertext []byte) ([]byte, error)
}

type Manager struct {
	store  *Store
	paths  Paths
	objSvc *s3server.ObjectService
	kms    KMSEncryptor // optional; nil = no at-rest encryption
}

// NewManager returns a Manager backed by the given store and paths.
func NewManager(s *Store, p Paths) *Manager {
	return &Manager{
		store:  s,
		paths:  p,
		objSvc: s3server.NewObjectService(p.Buckets),
	}
}

// SetKMS attaches a KMS encryptor. Once set, any PutObject call into a bucket
// that has KMSKeyName set will transparently encrypt object bytes at rest.
func (m *Manager) SetKMS(k KMSEncryptor) { m.kms = k }

// ObjectService returns the underlying ObjectService for direct use (e.g. S3 server).
func (m *Manager) ObjectService() *s3server.ObjectService {
	return m.objSvc
}

// EnsurePaths creates the storage root directories if they do not exist.
func (m *Manager) EnsurePaths() error {
	for _, dir := range []string{m.paths.Volumes, m.paths.Buckets, m.paths.Snapshots} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("storage: ensure path %s: %w", dir, err)
		}
	}
	return nil
}

// ---- volumes ----------------------------------------------------------------

// CreateVolume provisions a directory-backed volume and registers it.
func (m *Manager) CreateVolume(opts CreateVolumeOptions) (Volume, error) {
	if opts.Name == "" {
		return Volume{}, fmt.Errorf("storage: volume name is required")
	}
	if opts.Class == "" {
		opts.Class = VolumeClassLocal
	}
	volDir := filepath.Join(m.paths.Volumes, opts.Name)
	if err := os.MkdirAll(volDir, 0o700); err != nil {
		return Volume{}, fmt.Errorf("storage: create volume dir: %w", err)
	}
	v := Volume{
		ID:        newID("vol"),
		Name:      opts.Name,
		SizeBytes: opts.SizeBytes,
		Class:     opts.Class,
		Backend:   VolumeBackendDirectory,
		Path:      volDir,
		Encrypted: opts.Encrypted,
		CreatedAt: now(),
	}
	if err := m.store.InsertVolume(v); err != nil {
		// Clean up directory on failure.
		os.RemoveAll(volDir)
		return Volume{}, err
	}
	return v, nil
}

// GetVolume returns a volume by name or ID.
func (m *Manager) GetVolume(nameOrID string) (Volume, error) {
	v, err := m.store.GetVolume(nameOrID)
	if err != nil {
		return Volume{}, fmt.Errorf("storage: volume %q not found: %w", nameOrID, err)
	}
	return v, nil
}

// ListVolumes returns all volumes.
func (m *Manager) ListVolumes() ([]Volume, error) {
	return m.store.ListVolumes()
}

// AttachVolume records that a volume is attached to an instance. The bind mount
// itself happens at instance launch time using the volume path.
func (m *Manager) AttachVolume(volumeNameOrID, instanceID, instancePath string) error {
	v, err := m.store.GetVolume(volumeNameOrID)
	if err != nil {
		return fmt.Errorf("storage: volume %q not found", volumeNameOrID)
	}
	if v.AttachedInstanceID != "" {
		return fmt.Errorf("storage: volume %q is already attached to instance %q", v.Name, v.AttachedInstanceID)
	}
	return m.store.UpdateVolumeAttachment(volumeNameOrID, instanceID, instancePath)
}

// DetachVolume clears the attachment record for a volume.
func (m *Manager) DetachVolume(volumeNameOrID string) error {
	if _, err := m.store.GetVolume(volumeNameOrID); err != nil {
		return fmt.Errorf("storage: volume %q not found", volumeNameOrID)
	}
	return m.store.UpdateVolumeAttachment(volumeNameOrID, "", "")
}

// DeleteVolume removes a volume's directory and its store record. Returns an
// error if the volume is currently attached.
func (m *Manager) DeleteVolume(nameOrID string) error {
	v, err := m.store.GetVolume(nameOrID)
	if err != nil {
		return fmt.Errorf("storage: volume %q not found", nameOrID)
	}
	if v.AttachedInstanceID != "" {
		return fmt.Errorf("storage: volume %q is attached to %q; detach before deleting", v.Name, v.AttachedInstanceID)
	}
	if err := os.RemoveAll(v.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: remove volume dir: %w", err)
	}
	return m.store.DeleteVolume(v.ID)
}

// SnapshotVolume creates a tar.zst archive of a volume's directory.
func (m *Manager) SnapshotVolume(volumeNameOrID, snapshotName string) (Snapshot, error) {
	v, err := m.store.GetVolume(volumeNameOrID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("storage: volume %q not found", volumeNameOrID)
	}
	if snapshotName == "" {
		snapshotName = fmt.Sprintf("%s-%s", v.Name, shortID())
	}
	snapPath := filepath.Join(m.paths.Snapshots, snapshotName+".tar.zst")
	size, digest, err := tarZstDir(v.Path, snapPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("storage: snapshot volume: %w", err)
	}
	snap := Snapshot{
		ID:         newID("snap"),
		Name:       snapshotName,
		SourceType: SnapshotSourceVolume,
		SourceID:   v.ID,
		Path:       snapPath,
		Digest:     digest,
		SizeBytes:  size,
		CreatedAt:  now(),
	}
	if err := m.store.InsertSnapshot(snap); err != nil {
		os.Remove(snapPath)
		return Snapshot{}, err
	}
	return snap, nil
}

// RestoreSnapshot extracts a snapshot archive into a volume's directory,
// replacing its current contents.
func (m *Manager) RestoreSnapshot(snapshotNameOrID, volumeNameOrID string) error {
	snap, err := m.store.GetSnapshot(snapshotNameOrID)
	if err != nil {
		return fmt.Errorf("storage: snapshot %q not found", snapshotNameOrID)
	}
	v, err := m.store.GetVolume(volumeNameOrID)
	if err != nil {
		return fmt.Errorf("storage: volume %q not found", volumeNameOrID)
	}
	if v.AttachedInstanceID != "" {
		return fmt.Errorf("storage: volume %q is attached; detach before restoring", v.Name)
	}
	if err := os.RemoveAll(v.Path); err != nil {
		return fmt.Errorf("storage: clear volume dir: %w", err)
	}
	if err := os.MkdirAll(v.Path, 0o700); err != nil {
		return fmt.Errorf("storage: recreate volume dir: %w", err)
	}
	return untarZstDir(snap.Path, v.Path)
}

// GetSnapshot returns a snapshot by name or ID.
func (m *Manager) GetSnapshot(nameOrID string) (Snapshot, error) {
	snap, err := m.store.GetSnapshot(nameOrID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("storage: snapshot %q not found: %w", nameOrID, err)
	}
	return snap, nil
}

// ListSnapshots returns all snapshots, optionally filtered to a source volume.
func (m *Manager) ListSnapshots(sourceID string) ([]Snapshot, error) {
	return m.store.ListSnapshots(sourceID)
}

// DeleteSnapshot removes the snapshot archive and its record.
func (m *Manager) DeleteSnapshot(nameOrID string) error {
	snap, err := m.store.GetSnapshot(nameOrID)
	if err != nil {
		return fmt.Errorf("storage: snapshot %q not found", nameOrID)
	}
	if err := os.Remove(snap.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: remove snapshot file: %w", err)
	}
	return m.store.DeleteSnapshot(snap.ID)
}

// ---- buckets ----------------------------------------------------------------

// CreateBucket provisions a local directory bucket and registers it.
func (m *Manager) CreateBucket(opts CreateBucketOptions) (Bucket, error) {
	if opts.Name == "" {
		return Bucket{}, fmt.Errorf("storage: bucket name is required")
	}
	bucketDir := filepath.Join(m.paths.Buckets, opts.Name)
	if err := os.MkdirAll(bucketDir, 0o700); err != nil {
		return Bucket{}, fmt.Errorf("storage: create bucket dir: %w", err)
	}
	b := Bucket{
		ID:         newID("bkt"),
		Name:       opts.Name,
		Backend:    BucketBackendLocal,
		Path:       bucketDir,
		Versioning: opts.Versioning,
		Encrypted:  opts.Encrypted,
		QuotaBytes: opts.QuotaBytes,
		KMSKeyName: opts.KMSKeyName,
		CreatedAt:  now(),
	}
	if err := m.store.InsertBucket(b); err != nil {
		os.RemoveAll(bucketDir)
		return Bucket{}, err
	}
	return b, nil
}

// GetBucket returns a bucket by name or ID.
func (m *Manager) GetBucket(nameOrID string) (Bucket, error) {
	b, err := m.store.GetBucket(nameOrID)
	if err != nil {
		return Bucket{}, fmt.Errorf("storage: bucket %q not found: %w", nameOrID, err)
	}
	return b, nil
}

// ListBuckets returns all buckets.
func (m *Manager) ListBuckets() ([]Bucket, error) {
	return m.store.ListBuckets()
}

// DeleteBucket removes the bucket directory and all its records.
// Returns an error if the bucket is not empty unless force is true.
func (m *Manager) DeleteBucket(nameOrID string, force bool) error {
	b, err := m.store.GetBucket(nameOrID)
	if err != nil {
		return fmt.Errorf("storage: bucket %q not found", nameOrID)
	}
	if !force && m.objSvc.BucketHasObjects(b.Name) {
		return fmt.Errorf("storage: bucket %q is not empty; use --force to delete anyway", b.Name)
	}
	if err := os.RemoveAll(b.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: remove bucket dir: %w", err)
	}
	return m.store.DeleteBucket(b.ID)
}

// BucketUsageBytes returns the total bytes used by all objects in the bucket.
func (m *Manager) BucketUsageBytes(bucketName string) int64 {
	res, _ := m.objSvc.ListObjects(bucketName, "", "", 0)
	if res == nil {
		return 0
	}
	var total int64
	for _, obj := range res.Objects {
		total += obj.Size
	}
	return total
}

// PutObject copies a local file into the bucket using the S3-compatible object layout.
// If the bucket has KMSKeyName set and a KMSEncryptor is attached, the object
// bytes are AES-256-GCM encrypted before being written to disk.
func (m *Manager) PutObject(bucketNameOrID, key, srcPath string) (Object, error) {
	b, err := m.store.GetBucket(bucketNameOrID)
	if err != nil {
		return Object{}, fmt.Errorf("storage: bucket %q not found", bucketNameOrID)
	}
	if b.QuotaBytes > 0 {
		used := m.BucketUsageBytes(b.Name)
		if existing, herr := m.objSvc.HeadObject(b.Name, key); herr == nil {
			used -= existing.Size
			if used < 0 {
				used = 0
			}
		}
		fi, sterr := os.Stat(srcPath)
		if sterr == nil && used+fi.Size() > b.QuotaBytes {
			return Object{}, fmt.Errorf("storage: bucket %q quota exceeded (%d/%d bytes)", b.Name, used, b.QuotaBytes)
		}
	}
	// KMS at-rest encryption: read, encrypt, write to a temp file, then store.
	actualSrc := srcPath
	if m.kms != nil && b.KMSKeyName != "" {
		plaintext, rerr := os.ReadFile(srcPath)
		if rerr != nil {
			return Object{}, fmt.Errorf("storage: kms: read source: %w", rerr)
		}
		ciphertext, cerr := m.kms.Encrypt(b.KMSKeyName, "", plaintext)
		if cerr != nil {
			return Object{}, fmt.Errorf("storage: kms: encrypt: %w", cerr)
		}
		tmp, terr := os.CreateTemp("", "capper-kms-*")
		if terr != nil {
			return Object{}, fmt.Errorf("storage: kms: temp file: %w", terr)
		}
		defer os.Remove(tmp.Name())
		if _, werr := tmp.Write(ciphertext); werr != nil {
			_ = tmp.Close()
			return Object{}, fmt.Errorf("storage: kms: write temp: %w", werr)
		}
		_ = tmp.Close()
		actualSrc = tmp.Name()
	}
	meta, err := m.objSvc.PutObjectFromFile(b.Name, key, actualSrc)
	if err != nil {
		return Object{}, fmt.Errorf("storage: put object: %w", err)
	}
	return Object{
		Key:         key,
		BucketID:    b.ID,
		BucketName:  b.Name,
		SizeBytes:   meta.Size,
		Digest:      meta.ETag,
		ContentType: meta.ContentType,
		CreatedAt:   time.Unix(meta.LastModified, 0).UTC().Format(time.RFC3339),
	}, nil
}

// GetObject copies an object from the bucket to destPath.
func (m *Manager) GetObject(bucketNameOrID, key, destPath string) error {
	b, err := m.store.GetBucket(bucketNameOrID)
	if err != nil {
		return fmt.Errorf("storage: bucket %q not found", bucketNameOrID)
	}
	_, err = m.objSvc.GetObjectToFile(b.Name, key, destPath)
	return err
}

// ListObjects returns all objects in a bucket, optionally filtered by prefix.
func (m *Manager) ListObjects(bucketNameOrID, prefix string) ([]Object, error) {
	b, err := m.store.GetBucket(bucketNameOrID)
	if err != nil {
		return nil, fmt.Errorf("storage: bucket %q not found", bucketNameOrID)
	}
	res, err := m.objSvc.ListObjects(b.Name, prefix, "", 0)
	if err != nil {
		return nil, err
	}
	out := make([]Object, 0, len(res.Objects))
	for _, obj := range res.Objects {
		out = append(out, Object{
			Key:         obj.Key,
			BucketID:    b.ID,
			BucketName:  b.Name,
			SizeBytes:   obj.Size,
			Digest:      obj.ETag,
			ContentType: obj.ContentType,
			CreatedAt:   obj.LastModified.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

// DeleteObject removes an object from a bucket.
func (m *Manager) DeleteObject(bucketNameOrID, key string) error {
	b, err := m.store.GetBucket(bucketNameOrID)
	if err != nil {
		return fmt.Errorf("storage: bucket %q not found", bucketNameOrID)
	}
	err = m.objSvc.DeleteObject(b.Name, key)
	if err == s3server.ErrNoSuchKey {
		return fmt.Errorf("storage: object %q not found in bucket %q", key, b.Name)
	}
	return err
}

// ---- BucketProvider interface (for S3 server) -------------------------------

// ---- shares -----------------------------------------------------------------

// CreateShare records a file share in the store.
func (m *Manager) CreateShare(name, hostPath, mountPath, instanceID string) (Share, error) {
	if name == "" {
		return Share{}, fmt.Errorf("storage: share name is required")
	}
	if hostPath == "" {
		return Share{}, fmt.Errorf("storage: --path is required")
	}
	sh := Share{
		ID:         newID("shr"),
		Name:       name,
		HostPath:   hostPath,
		MountPath:  mountPath,
		InstanceID: instanceID,
		CreatedAt:  now(),
	}
	if err := m.store.InsertShare(sh); err != nil {
		return Share{}, err
	}
	return sh, nil
}

// GetShare returns a share by name or ID.
func (m *Manager) GetShare(nameOrID string) (Share, error) {
	return m.store.GetShare(nameOrID)
}

// ListShares returns all shares.
func (m *Manager) ListShares() ([]Share, error) {
	return m.store.ListShares()
}

// DeleteShare removes a share record.
func (m *Manager) DeleteShare(nameOrID string) error {
	return m.store.DeleteShare(nameOrID)
}

// ListBucketsForS3 returns bucket names and creation dates for the S3 listing API.
func (m *Manager) ListBucketsForS3() ([]s3server.BucketEntry, error) {
	bkts, err := m.store.ListBuckets()
	if err != nil {
		return nil, err
	}
	out := make([]s3server.BucketEntry, 0, len(bkts))
	for _, b := range bkts {
		out = append(out, s3server.BucketEntry{Name: b.Name, CreatedAt: b.CreatedAt})
	}
	return out, nil
}

// CreateBucketForS3 creates a bucket by name (used by the S3 server PUT /:bucket).
func (m *Manager) CreateBucketForS3(name string) error {
	_, err := m.CreateBucket(CreateBucketOptions{Name: name})
	return err
}

// DeleteBucketForS3 deletes a bucket (used by the S3 server DELETE /:bucket).
// Returns *s3server.S3Err with BucketNotEmpty code if non-empty and force is false.
func (m *Manager) DeleteBucketForS3(name string, force bool) error {
	if !force && m.objSvc.BucketHasObjects(name) {
		return s3server.ErrBucketNotEmpty
	}
	return m.DeleteBucket(name, true)
}

// BucketQuotaBytes returns the quota in bytes for the named bucket.
// Returns 0 if the bucket does not exist or has no quota configured.
func (m *Manager) BucketQuotaBytes(name string) int64 {
	b, err := m.store.GetBucket(name)
	if err != nil {
		return 0
	}
	return b.QuotaBytes
}

// BucketExistsForS3 reports whether a bucket with the given name exists.
func (m *Manager) BucketExistsForS3(name string) bool {
	_, err := m.store.GetBucket(name)
	return err == nil
}

// ---- filesystem helpers -----------------------------------------------------

// tarZstDir archives src directory to destPath (tar.zst) and returns
// (compressed size, sha256 hex digest of the archive, error).
func tarZstDir(src, dest string) (int64, string, error) {
	f, err := os.Create(dest)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	hw := &hashWriter{w: f, h: sha256.New()}
	enc, err := zstd.NewWriter(hw)
	if err != nil {
		return 0, "", err
	}
	tw := tar.NewWriter(enc)

	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			fh, err := os.Open(path)
			if err != nil {
				return err
			}
			defer fh.Close()
			if _, err := io.Copy(tw, fh); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, "", err
	}
	if err := tw.Close(); err != nil {
		return 0, "", err
	}
	if err := enc.Close(); err != nil {
		return 0, "", err
	}

	fi, err := f.Stat()
	if err != nil {
		return 0, "", err
	}
	return fi.Size(), hex.EncodeToString(hw.h.Sum(nil)), nil
}

// untarZstDir extracts a tar.zst archive into dest.
func untarZstDir(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	dec, err := zstd.NewReader(f)
	if err != nil {
		return err
	}
	defer dec.Close()

	tr := tar.NewReader(dec)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.Clean(hdr.Name))
		// Guard against path traversal.
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			return fmt.Errorf("storage: unsafe tar path: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			fh, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := iox.CopyLimited(fh, tr, iox.DefaultMaxArchiveEntry); err != nil {
				fh.Close()
				return err
			}
			fh.Close()
		}
	}
	return nil
}

// ---- helpers ----------------------------------------------------------------

type hashWriter struct {
	w io.Writer
	h interface {
		Write([]byte) (int, error)
		Sum([]byte) []byte
	}
}

func (hw *hashWriter) Write(p []byte) (int, error) {
	_, _ = hw.h.Write(p)
	return hw.w.Write(p)
}

func newID(prefix string) string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func shortID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
