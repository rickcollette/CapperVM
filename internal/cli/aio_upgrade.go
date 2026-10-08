package cli

import (
	"capper/internal/iox"
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"capper/internal/controller"
	"capper/internal/version"
)

// AIO upgrade layout. Binaries live under a versioned directory; a `current`
// symlink selects the active version, and /usr/local/bin/* symlink through it, so
// an upgrade/rollback is an atomic symlink flip.
const (
	aioLibRoot   = "/usr/local/lib/capper"
	aioCurrent   = aioLibRoot + "/current"
	aioBinDir    = "/usr/local/bin"
	aioHealthURL = "http://localhost:8080/api/v1/health"
)

var aioManagedBinaries = []string{"capper", "capper-agent", "capinit", "capdb-server"}

// aioUpgradeCmd performs a seamless, auto-rollback AIO upgrade.
func aioUpgradeCmd(opts *options) *cobra.Command {
	var bundle, url, sha, toVersion, channel, feed string
	var rollback, yes bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade (or roll back) the AIO node from a release bundle",
		Long: "Verifies and stages a release bundle into a versioned directory, snapshots the\n" +
			"database, swaps the active version, runs migrations, and health-gates the new\n" +
			"version — automatically rolling back binaries and database on failure.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Geteuid() != 0 {
				return fmt.Errorf("aio upgrade must run as root")
			}
			if rollback {
				return aioRollback(opts, timeout)
			}
			// A channel selects url+sha+version from the update feed.
			if channel != "" {
				entry, err := resolveChannel(feed, channel)
				if err != nil {
					return err
				}
				if entry.MinUpgradeFrom != "" && version.Version != "0.0.0-dev" &&
					compareVersions(version.Version, entry.MinUpgradeFrom) < 0 {
					return fmt.Errorf("current version %s is below the channel's minUpgradeFrom %s; upgrade incrementally",
						version.Version, entry.MinUpgradeFrom)
				}
				url, sha, toVersion = entry.URL, entry.SHA256, entry.Version
				fmt.Printf("→ channel %q → version %s\n", channel, entry.Version)
			}
			if bundle == "" && url == "" {
				return fmt.Errorf("provide --bundle PATH, --url URL, or --channel NAME")
			}

			// 1. Resolve the bundle to a local file (download if needed).
			localBundle, cleanup, err := resolveAIOBundle(bundle, url)
			if err != nil {
				return err
			}
			defer cleanup()

			// 2. Verify integrity.
			if sha != "" {
				if err := verifySHA256(localBundle, sha); err != nil {
					return err
				}
				fmt.Println("✓ checksum verified")
			} else {
				fmt.Println("warning: no --sha256 provided; skipping checksum verification")
			}

			// 3. Stage into a versioned dir and read its VERSION.
			stageDir, newVer, err := stageAIOBundle(localBundle, toVersion)
			if err != nil {
				return err
			}
			fmt.Printf("✓ staged version %s at %s\n", newVer, stageDir)

			prevVer := readCurrentVersion()
			if prevVer == newVer {
				fmt.Printf("version %s already active; nothing to do\n", newVer)
				return nil
			}
			if !yes {
				fmt.Printf("Upgrade %s → %s. Proceed? [y/N] ", orDash(prevVer), newVer)
				var ans string
				_, _ = fmt.Scanln(&ans)
				if !strings.EqualFold(strings.TrimSpace(ans), "y") {
					return fmt.Errorf("aborted")
				}
			}

			// 4. Stop services, then snapshot the DB at rest for rollback.
			fmt.Println("→ stopping services")
			if err := aioStopServices(); err != nil {
				return fmt.Errorf("stop services: %w", err)
			}
			snapshot, err := aioSnapshotDB(opts)
			if err != nil {
				// Restart what we stopped before bailing.
				_ = aioStartServices()
				return fmt.Errorf("pre-upgrade snapshot failed: %w", err)
			}
			fmt.Printf("✓ database snapshot: %s\n", snapshot)

			// 5. Flip the active version and ensure bin symlinks point through it.
			if err := repointCurrent(stageDir); err != nil {
				return err
			}
			if err := ensureBinSymlinks(); err != nil {
				return err
			}

			// 6. Start the new version (control plane runs migrations on boot) and
			//    health-gate it.
			fmt.Println("→ starting new version")
			startErr := aioStartServices()
			if startErr == nil {
				startErr = waitForHealth(aioHealthURL, timeout)
			}
			if startErr != nil {
				fmt.Printf("✗ new version unhealthy (%v) — rolling back\n", startErr)
				if err := aioPerformRollback(opts, prevVer, snapshot); err != nil {
					return fmt.Errorf("rollback FAILED after bad upgrade: %w (snapshot preserved at %s)", err, snapshot)
				}
				return fmt.Errorf("upgrade rolled back to %s", orDash(prevVer))
			}

			fmt.Printf("✓ upgrade complete: %s → %s (healthy). Snapshot kept at %s\n", orDash(prevVer), newVer, snapshot)
			return nil
		},
	}
	cmd.Flags().StringVar(&bundle, "bundle", "", "path to a capper-aio-*.tgz release bundle")
	cmd.Flags().StringVar(&url, "url", "", "URL to download the release bundle from")
	cmd.Flags().StringVar(&channel, "channel", "", "update channel to pull from the feed (e.g. stable, edge)")
	cmd.Flags().StringVar(&feed, "feed", "", "channel feed URL (default $CAPPER_UPDATE_FEED)")
	cmd.Flags().StringVar(&sha, "sha256", "", "expected SHA-256 of the bundle (hex)")
	cmd.Flags().StringVar(&toVersion, "version", "", "override the version label for the staged dir")
	cmd.Flags().BoolVar(&rollback, "rollback", false, "roll back to the previous version")
	cmd.Flags().BoolVar(&yes, "yes", false, "do not prompt for confirmation")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "health-gate timeout before auto-rollback")
	return cmd
}

// aioCheckUpdateCmd reports whether the channel feed offers a newer version than
// the running binary.
func aioCheckUpdateCmd(opts *options) *cobra.Command {
	var channel, feed string
	cmd := &cobra.Command{
		Use:   "check-update",
		Short: "check the update feed for a newer version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entry, err := resolveChannel(feed, channel)
			if err != nil {
				return err
			}
			cur := version.Version
			cmp := compareVersions(cur, entry.Version)
			if opts.json {
				return printJSON(map[string]any{
					"current": cur, "channel": channel, "latest": entry.Version,
					"updateAvailable": cmp < 0,
				})
			}
			switch {
			case cmp < 0:
				fmt.Printf("Update available: %s → %s (channel %s)\n", cur, entry.Version, channel)
				fmt.Printf("Run: sudo capper aio upgrade --channel %s\n", channel)
			case cmp == 0:
				fmt.Printf("Up to date: %s is the latest on channel %s\n", cur, channel)
			default:
				fmt.Printf("Running %s, newer than channel %s (%s)\n", cur, channel, entry.Version)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "stable", "update channel to check")
	cmd.Flags().StringVar(&feed, "feed", "", "channel feed URL (default $CAPPER_UPDATE_FEED)")
	return cmd
}

// resolveAIOBundle returns a local path to the bundle, downloading from url if a
// local bundle path was not given. cleanup removes any temp download.
func resolveAIOBundle(bundle, url string) (string, func(), error) {
	if bundle != "" {
		if _, err := os.Stat(bundle); err != nil {
			return "", func() {}, fmt.Errorf("bundle not found: %w", err)
		}
		return bundle, func() {}, nil
	}
	tmp, err := os.CreateTemp("", "capper-aio-*.tgz")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	fmt.Printf("→ downloading %s\n", url)
	resp, err := http.Get(url) //nolint:gosec // operator-supplied URL
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cleanup()
		return "", func() {}, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		cleanup()
		return "", func() {}, err
	}
	_ = tmp.Close()
	return tmp.Name(), cleanup, nil
}

// channelEntry is one channel's release pointer in the update feed.
type channelEntry struct {
	Version        string `json:"version"`
	URL            string `json:"url"`
	SHA256         string `json:"sha256"`
	MinUpgradeFrom string `json:"minUpgradeFrom"`
}

// resolveChannel fetches the feed (JSON map of channel→entry) and returns the
// requested channel's release pointer.
func resolveChannel(feed, channel string) (channelEntry, error) {
	if feed == "" {
		feed = os.Getenv("CAPPER_UPDATE_FEED")
	}
	if feed == "" {
		return channelEntry{}, fmt.Errorf("no feed URL; pass --feed or set CAPPER_UPDATE_FEED")
	}
	resp, err := http.Get(feed) //nolint:gosec // operator-supplied feed URL
	if err != nil {
		return channelEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return channelEntry{}, fmt.Errorf("fetch feed %s: HTTP %d", feed, resp.StatusCode)
	}
	var channels map[string]channelEntry
	if err := json.NewDecoder(resp.Body).Decode(&channels); err != nil {
		return channelEntry{}, fmt.Errorf("parse feed: %w", err)
	}
	entry, ok := channels[channel]
	if !ok {
		return channelEntry{}, fmt.Errorf("channel %q not found in feed", channel)
	}
	if entry.URL == "" {
		return channelEntry{}, fmt.Errorf("channel %q has no url", channel)
	}
	return entry, nil
}

// compareVersions does a best-effort numeric semver compare (major.minor.patch),
// ignoring any pre-release suffix. Returns -1, 0, or 1.
func compareVersions(a, b string) int {
	pa := parseSemver(a)
	pb := parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	// Drop any pre-release/build suffix.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		n := 0
		for _, r := range part {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		out[i] = n
	}
	return out
}

// verifySHA256 checks that the file at path matches the expected hex digest.
func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(expected)) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, expected)
	}
	return nil
}

// stageAIOBundle extracts the bundle into aioLibRoot/<version>/ and returns the
// staged directory and its version. The bundle's top-level dir is stripped.
func stageAIOBundle(bundlePath, versionOverride string) (string, string, error) {
	tmpExtract, err := os.MkdirTemp("", "capper-stage-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmpExtract)
	if err := extractTarGz(bundlePath, tmpExtract); err != nil {
		return "", "", err
	}
	// The bundle has a single top-level dir (capper-aio-<ver>-...). Descend into it.
	root, err := singleSubdir(tmpExtract)
	if err != nil {
		return "", "", err
	}
	ver := versionOverride
	if ver == "" {
		if b, err := os.ReadFile(filepath.Join(root, "VERSION")); err == nil {
			ver = strings.TrimSpace(string(b))
		}
	}
	if ver == "" {
		return "", "", fmt.Errorf("could not determine bundle version (no VERSION file); pass --version")
	}
	dest := filepath.Join(aioLibRoot, ver)
	if err := os.MkdirAll(aioLibRoot, 0o755); err != nil {
		return "", "", err
	}
	_ = os.RemoveAll(dest) // re-stage idempotently
	if err := os.Rename(root, dest); err != nil {
		// Cross-device rename can fail; fall back to a copy.
		if err := copyTree(root, dest); err != nil {
			return "", "", err
		}
	}
	// Make binaries executable.
	binDir := filepath.Join(dest, "bin")
	if entries, err := os.ReadDir(binDir); err == nil {
		for _, e := range entries {
			_ = os.Chmod(filepath.Join(binDir, e.Name()), 0o755)
		}
	}
	return dest, ver, nil
}

// repointCurrent atomically points aioCurrent at versionDir.
func repointCurrent(versionDir string) error {
	tmp := aioCurrent + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(versionDir, tmp); err != nil {
		return fmt.Errorf("symlink: %w", err)
	}
	if err := os.Rename(tmp, aioCurrent); err != nil {
		return fmt.Errorf("activate version: %w", err)
	}
	return nil
}

// ensureBinSymlinks makes /usr/local/bin/<bin> point at current/bin/<bin>.
func ensureBinSymlinks() error {
	for _, b := range aioManagedBinaries {
		target := filepath.Join(aioCurrent, "bin", b)
		if _, err := os.Stat(target); err != nil {
			continue // bundle may not ship this binary
		}
		link := filepath.Join(aioBinDir, b)
		_ = os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("link %s: %w", b, err)
		}
	}
	return nil
}

// readCurrentVersion resolves the version the `current` symlink points at.
func readCurrentVersion() string {
	dest, err := os.Readlink(aioCurrent)
	if err != nil {
		return ""
	}
	return filepath.Base(dest)
}

// aioSnapshotDB writes a consistent DB snapshot (services should be stopped).
func aioSnapshotDB(opts *options) (string, error) {
	var snap string
	err := withController(opts, func(ctrl controller.Controller) error {
		snap = ctrl.Store.SnapshotPath()
		return ctrl.Store.SnapshotDB(snap)
	})
	return snap, err
}

// aioStopServices / aioStartServices wrap the same systemd ordering as aio up/down.
func aioStopServices() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return stopPIDs("/run/capper-aio.pid", 10*time.Second)
	}
	svcs := []string{"capper-agent", "capper-control"}
	if unitExists("capdb-server.service") {
		svcs = append(svcs, "capdb-server")
	}
	out, err := exec.Command("systemctl", append([]string{"stop"}, svcs...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

func aioStartServices() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not available; start services manually (capper aio up)")
	}
	svcs := []string{"capper-control", "capper-agent"}
	if unitExists("capdb-server.service") {
		svcs = append([]string{"capdb-server"}, svcs...)
	}
	for _, svc := range svcs {
		if out, err := exec.Command("systemctl", "start", svc).CombinedOutput(); err != nil {
			return fmt.Errorf("start %s: %w\n%s", svc, err, out)
		}
	}
	return nil
}

// aioPerformRollback restores the previous version and database snapshot.
func aioPerformRollback(opts *options, prevVer, snapshot string) error {
	_ = aioStopServices()
	if snapshot != "" {
		if err := restoreSnapshot(opts, snapshot); err != nil {
			return fmt.Errorf("restore DB: %w", err)
		}
	}
	if prevVer != "" {
		if err := repointCurrent(filepath.Join(aioLibRoot, prevVer)); err != nil {
			return err
		}
		_ = ensureBinSymlinks()
	}
	if err := aioStartServices(); err != nil {
		return err
	}
	return waitForHealth(aioHealthURL, 60*time.Second)
}

// aioRollback is the user-invoked `--rollback`: flip to the previous version.
func aioRollback(opts *options, timeout time.Duration) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("rollback must run as root")
	}
	// The previous version is any staged version dir other than current.
	prev := previousStagedVersion()
	if prev == "" {
		return fmt.Errorf("no previous version found under %s to roll back to", aioLibRoot)
	}
	fmt.Printf("→ rolling back %s → %s\n", orDash(readCurrentVersion()), prev)
	_ = aioStopServices()
	if err := repointCurrent(filepath.Join(aioLibRoot, prev)); err != nil {
		return err
	}
	if err := ensureBinSymlinks(); err != nil {
		return err
	}
	if err := aioStartServices(); err != nil {
		return err
	}
	if err := waitForHealth(aioHealthURL, timeout); err != nil {
		return fmt.Errorf("rolled back but unhealthy: %w", err)
	}
	fmt.Printf("✓ rolled back to %s\n", prev)
	return nil
}

// restoreSnapshot copies a DB snapshot back over the live control-plane DB. Only
// safe for the pure-Go (file) backend; for CapDB the operator restores server-side.
func restoreSnapshot(opts *options, snapshot string) error {
	var dbPath string
	if err := withController(opts, func(ctrl controller.Controller) error {
		dbPath = ctrl.Store.Paths.DB
		return nil
	}); err != nil {
		return err
	}
	if dbPath == "" {
		return fmt.Errorf("cannot determine DB path for restore")
	}
	// Remove WAL/SHM sidecars so the restored file is authoritative.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(dbPath + suffix)
	}
	return copyFile(snapshot, dbPath)
}

// ---- small fs helpers ------------------------------------------------------

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// Guard against path traversal.
		target := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := iox.CopyLimited(out, tr, iox.DefaultMaxArchiveEntry); err != nil {
				_ = out.Close()
				return err
			}
			_ = out.Close()
		}
	}
}

func singleSubdir(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 1 {
		return filepath.Join(dir, dirs[0]), nil
	}
	// No wrapping dir (or several) — treat dir itself as the root.
	return dir, nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

// previousStagedVersion returns the most recent staged version dir that is not
// the current one (by mtime).
func previousStagedVersion() string {
	cur := readCurrentVersion()
	entries, err := os.ReadDir(aioLibRoot)
	if err != nil {
		return ""
	}
	var best string
	var bestMod time.Time
	for _, e := range entries {
		if !e.IsDir() || e.Name() == cur || e.Name() == "current" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestMod) {
			bestMod = info.ModTime()
			best = e.Name()
		}
	}
	return best
}
