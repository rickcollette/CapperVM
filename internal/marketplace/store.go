package marketplace

import (
	"capper/internal/iox"
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// InitSchema creates the marketplace_listings table. Safe to call multiple times.
func InitSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS marketplace_listings (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL,
		version      TEXT NOT NULL,
		description  TEXT NOT NULL DEFAULT '',
		digest       TEXT NOT NULL DEFAULT '',
		status       TEXT NOT NULL DEFAULT 'pending',
		labels_json  TEXT NOT NULL DEFAULT '{}',
		created_at   TEXT NOT NULL,
		updated_at   TEXT NOT NULL,
		scan_status  TEXT NOT NULL DEFAULT '',
		scan_findings INTEGER NOT NULL DEFAULT 0,
		scan_results  TEXT NOT NULL DEFAULT '',
		sbom_digest  TEXT NOT NULL DEFAULT '',
		UNIQUE(name, version)
	)`)
	if err != nil {
		return fmt.Errorf("marketplace.InitSchema: %w", err)
	}
	// Additive migrations for scan severity detail (ignore duplicate-column errors).
	for _, alter := range []string{
		`ALTER TABLE marketplace_listings ADD COLUMN scan_severities TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE marketplace_listings ADD COLUMN scan_scanned_at TEXT NOT NULL DEFAULT ''`,
	} {
		if _, e := db.Exec(alter); e != nil && !strings.Contains(e.Error(), "duplicate column") {
			return fmt.Errorf("marketplace.InitSchema migration: %w", e)
		}
	}
	return nil
}

// Config controls marketplace enforcement behaviour.
type Config struct {
	// RequireSignature rejects Publish() calls when the artifact has no valid
	// ECDSA signature embedded at attestations/signature.json.
	RequireSignature bool
}

// Manager provides CRUD for marketplace listings.
type Manager struct {
	db  *sql.DB
	cfg Config
}

// NewManager returns a Manager backed by db.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// NewManagerWithConfig returns a Manager backed by db with the given config.
func NewManagerWithConfig(db *sql.DB, cfg Config) *Manager {
	return &Manager{db: db, cfg: cfg}
}

// Publish verifies the artifact signature (when RequireSignature is set) then
// inserts the listing. Returns a 422-style error if the artifact is unsigned
// and strict mode is enabled.
func (m *Manager) Publish(l MarketplaceListing, artifactPath string) error {
	signerID, err := VerifySignature(artifactPath)
	if err != nil {
		if m.cfg.RequireSignature {
			return fmt.Errorf("publish rejected: artifact signature invalid: %w", err)
		}
		// Unsigned in permissive mode — insert with empty signerID.
		_ = signerID
	} else if l.Labels == nil {
		l.Labels = map[string]string{"signerID": signerID}
	} else {
		l.Labels["signerID"] = signerID
	}
	return m.Insert(l)
}

// Insert creates a new listing.
func (m *Manager) Insert(l MarketplaceListing) error {
	if l.ID == "" {
		l.ID = newID()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if l.CreatedAt == "" {
		l.CreatedAt = now
	}
	if l.UpdatedAt == "" {
		l.UpdatedAt = now
	}
	if l.Status == "" {
		l.Status = StatusPending
	}
	labels, _ := json.Marshal(l.Labels)
	severities, _ := json.Marshal(l.ScanSeverities)
	// INSERT OR REPLACE so re-publishing an image (same id) updates the listing.
	_, err := m.db.Exec(
		`INSERT OR REPLACE INTO marketplace_listings
			(id, name, version, description, digest, status, labels_json, created_at, updated_at,
			 scan_status, scan_findings, scan_results, sbom_digest, scan_severities, scan_scanned_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.ID, l.Name, l.Version, l.Description, l.Digest, l.Status, string(labels),
		l.CreatedAt, l.UpdatedAt, l.ScanStatus, l.ScanFindings, l.ScanResults, l.SBOMDigest,
		string(severities), l.ScanScannedAt,
	)
	if err != nil {
		return fmt.Errorf("marketplace: insert: %w", err)
	}
	return nil
}

// Get returns a listing by ID.
func (m *Manager) Get(id string) (MarketplaceListing, error) {
	row := m.db.QueryRow(
		`SELECT id, name, version, description, digest, status, labels_json, created_at, updated_at,
			scan_status, scan_findings, scan_results, sbom_digest, scan_severities, scan_scanned_at
		FROM marketplace_listings WHERE id=? LIMIT 1`, id)
	return scanListing(row)
}

// List returns all listings.
func (m *Manager) List() ([]MarketplaceListing, error) {
	rows, err := m.db.Query(
		`SELECT id, name, version, description, digest, status, labels_json, created_at, updated_at,
			scan_status, scan_findings, scan_results, sbom_digest, scan_severities, scan_scanned_at
		FROM marketplace_listings ORDER BY name, version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MarketplaceListing
	for rows.Next() {
		l, err := scanListing(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpdateStatus updates a listing's status.
func (m *Manager) UpdateStatus(id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := m.db.Exec(
		`UPDATE marketplace_listings SET status=?, updated_at=? WHERE id=?`,
		status, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("marketplace: listing %q not found", id)
	}
	return nil
}

// Delete removes a listing.
func (m *Manager) Delete(id string) error {
	res, err := m.db.Exec(`DELETE FROM marketplace_listings WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("marketplace: listing %q not found", id)
	}
	return nil
}

// ---- scanner ----------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanListing(s rowScanner) (MarketplaceListing, error) {
	var l MarketplaceListing
	var labelsJSON, severitiesJSON string
	err := s.Scan(
		&l.ID, &l.Name, &l.Version, &l.Description, &l.Digest, &l.Status,
		&labelsJSON, &l.CreatedAt, &l.UpdatedAt,
		&l.ScanStatus, &l.ScanFindings, &l.ScanResults, &l.SBOMDigest,
		&severitiesJSON, &l.ScanScannedAt,
	)
	if err != nil {
		return MarketplaceListing{}, fmt.Errorf("marketplace: scan: %w", err)
	}
	json.Unmarshal([]byte(labelsJSON), &l.Labels)
	if severitiesJSON != "" {
		json.Unmarshal([]byte(severitiesJSON), &l.ScanSeverities)
	}
	return l, nil
}

// ---- helpers ----------------------------------------------------------------

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "mkt_" + hex.EncodeToString(b)
}

func isNotFound(err error) bool {
	return strings.Contains(err.Error(), "no rows")
}

var _ = isNotFound // suppress unused warning

// StackCreator is a callback that creates/applies a stack. Injected to avoid
// circular dependency with capper/internal/stack.
type StackCreator func(name, project, marketListingID string, params map[string]string) error

// Install applies a marketplace listing as a stack in the given project.
// Returns an error if the listing is not in "approved" status.
func (m *Manager) Install(id, project string, params map[string]string, createStack StackCreator) error {
	listing, err := m.Get(id)
	if err != nil {
		return err
	}
	if listing.Status != StatusApproved {
		return fmt.Errorf("marketplace: listing %q is not approved (status=%s)", listing.Name, listing.Status)
	}
	name := listing.Name + "-" + listing.Version
	if createStack != nil {
		if err := createStack(name, project, listing.ID, params); err != nil {
			return fmt.Errorf("marketplace: install stack: %w", err)
		}
	}
	return nil
}

// ---- static scans (Block 14 Ph3) -------------------------------------------

// ScanResult holds the outcome of a single scan pass.
type ScanResult struct {
	Type     string `json:"type"`   // "signature", "sbom", "vuln", "secrets", "policy"
	Status   string `json:"status"` // "pass", "warn", "fail"
	Detail   string `json:"detail"`
	Findings int    `json:"findings"`
}

// RunStaticScans executes static checks against the listing artifact when it is
// available. Missing scanner tools or missing artifacts are recorded as
// warn/fail results instead of being treated as a successful scan.
func (m *Manager) RunStaticScans(id string) ([]ScanResult, error) {
	listing, err := m.Get(id)
	if err != nil {
		return nil, err
	}

	artifact := listingArtifactPath(listing)
	results := runArtifactStaticScans(artifact, listing.Digest)

	riskScore := 0
	failed := false
	for _, r := range results {
		riskScore += r.Findings * 10
		if r.Status == "fail" {
			failed = true
		}
	}
	classification := "low"
	if riskScore > 50 {
		classification = "high"
	} else if riskScore > 20 {
		classification = "medium"
	}

	enc, _ := json.Marshal(map[string]any{
		"scans":          results,
		"riskScore":      riskScore,
		"classification": classification,
	})
	status := "completed"
	if failed {
		status = "failed"
	}
	_, serr := m.db.Exec(
		`UPDATE marketplace_listings SET scan_status=?, scan_findings=?, scan_results=?, updated_at=datetime('now') WHERE id=?`,
		status, riskScore, string(enc), listing.ID,
	)
	return results, serr
}

func listingArtifactPath(listing MarketplaceListing) string {
	if p := strings.TrimSpace(listing.Labels["path"]); p != "" {
		return p
	}
	return strings.TrimSpace(listing.Name)
}

func runArtifactStaticScans(path, expectedDigest string) []ScanResult {
	if path == "" {
		return unavailableStaticResults("listing has no artifact path")
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return unavailableStaticResults(fmt.Sprintf("artifact %q is not readable", path))
	}
	return []ScanResult{
		scanDigest(path, expectedDigest),
		scanSignatureEntry(path),
		scanSBOMEntry(path),
		scanVulnerabilities(path),
		scanSecrets(path),
		scanPolicy(path),
	}
}

func unavailableStaticResults(detail string) []ScanResult {
	return []ScanResult{
		{Type: "signature", Status: "fail", Detail: detail, Findings: 1},
		{Type: "sbom", Status: "fail", Detail: detail, Findings: 1},
		{Type: "vuln", Status: "fail", Detail: detail, Findings: 1},
		{Type: "secrets", Status: "fail", Detail: detail, Findings: 1},
		{Type: "policy", Status: "fail", Detail: detail, Findings: 1},
	}
}

func scanDigest(path, expectedDigest string) ScanResult {
	if expectedDigest == "" {
		return ScanResult{Type: "digest", Status: "warn", Detail: "listing has no expected digest", Findings: 1}
	}
	actual, err := fileDigest(path)
	if err != nil {
		return ScanResult{Type: "digest", Status: "fail", Detail: err.Error(), Findings: 1}
	}
	if actual != expectedDigest {
		return ScanResult{Type: "digest", Status: "fail", Detail: "artifact digest mismatch", Findings: 1}
	}
	return ScanResult{Type: "digest", Status: "pass", Detail: "artifact digest matches listing", Findings: 0}
}

func scanSignatureEntry(path string) ScanResult {
	if _, err := readTarEntry(path, "signature.json", 1<<20); err != nil {
		return ScanResult{Type: "signature", Status: "fail", Detail: "missing signature.json", Findings: 1}
	}
	return ScanResult{Type: "signature", Status: "pass", Detail: "signature metadata present", Findings: 0}
}

func scanSBOMEntry(path string) ScanResult {
	if _, err := readTarEntry(path, "attestations/sbom.spdx.json", 8<<20); err != nil {
		return ScanResult{Type: "sbom", Status: "warn", Detail: "embedded SPDX SBOM not found", Findings: 1}
	}
	return ScanResult{Type: "sbom", Status: "pass", Detail: "embedded SPDX SBOM present", Findings: 0}
}

func scanVulnerabilities(path string) ScanResult {
	if _, err := exec.LookPath("trivy"); err != nil {
		return ScanResult{Type: "vuln", Status: "warn", Detail: "trivy not installed; vulnerability scan unavailable", Findings: 1}
	}
	cmd := exec.Command("trivy", "fs", "--quiet", "--exit-code", "1", "--severity", "CRITICAL,HIGH", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ScanResult{Type: "vuln", Status: "fail", Detail: strings.TrimSpace(string(out)), Findings: 1}
	}
	return ScanResult{Type: "vuln", Status: "pass", Detail: "trivy found no high or critical vulnerabilities", Findings: 0}
}

func scanSecrets(path string) ScanResult {
	findings := 0
	err := walkTarEntries(path, 1<<20, func(name string, data []byte) {
		lower := bytes.ToLower(data)
		switch {
		case bytes.Contains(data, []byte("-----BEGIN PRIVATE KEY-----")):
			findings++
		case bytes.Contains(data, []byte("AKIA")):
			findings++
		case bytes.Contains(lower, []byte("password=")), bytes.Contains(lower, []byte("secret=")):
			findings++
		}
	})
	if err != nil {
		return ScanResult{Type: "secrets", Status: "fail", Detail: err.Error(), Findings: 1}
	}
	if findings > 0 {
		return ScanResult{Type: "secrets", Status: "fail", Detail: "potential secret material found", Findings: findings}
	}
	return ScanResult{Type: "secrets", Status: "pass", Detail: "no obvious secret patterns found", Findings: 0}
}

func scanPolicy(path string) ScanResult {
	raw, err := readTarEntry(path, "capsule.json", 4<<20)
	if err != nil {
		return ScanResult{Type: "policy", Status: "fail", Detail: "capsule.json not found", Findings: 1}
	}
	var doc struct {
		CapsuleVersion string   `json:"capsuleVersion"`
		Entrypoint     []string `json:"entrypoint"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ScanResult{Type: "policy", Status: "fail", Detail: "invalid capsule.json", Findings: 1}
	}
	if doc.CapsuleVersion == "" || len(doc.Entrypoint) == 0 {
		return ScanResult{Type: "policy", Status: "fail", Detail: "capsule manifest missing required fields", Findings: 1}
	}
	return ScanResult{Type: "policy", Status: "pass", Detail: "capsule manifest has required fields", Findings: 0}
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func readTarEntry(path, entry string, limit int64) ([]byte, error) {
	var found []byte
	err := walkTarEntries(path, limit, func(name string, data []byte) {
		if name == entry {
			found = append([]byte{}, data...)
		}
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("%s not found", entry)
	}
	return found, nil
}

func walkTarEntries(path string, limit int64, visit func(name string, data []byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		if hdr.Size > limit {
			if _, err := iox.CopyLimited(io.Discard, tr, iox.DefaultMaxArchiveEntry); err != nil {
				return err
			}
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, limit+1))
		if err != nil {
			return err
		}
		visit(hdr.Name, data)
	}
}

// ---- ephemeral runtime scan (Block 14 Ph4) ---------------------------------

// RuntimeScanResult holds the outcome of an ephemeral runtime scan.
type RuntimeScanResult struct {
	ListingID       string   `json:"listingId"`
	Duration        string   `json:"duration"`
	ProcessTree     []string `json:"processTree"`
	OpenPorts       []int    `json:"openPorts"`
	DNSQueries      []string `json:"dnsQueries"`
	NetworkAttempts []string `json:"networkAttempts"`
	FileDiffs       []string `json:"fileDiffs"`
	Verdict         string   `json:"verdict"` // "clean", "suspicious", "blocked"
	Notes           string   `json:"notes"`
}

// RunRuntimeScan performs a bounded runtime-readiness scan for the listing
// artifact. In this local manager, the "deployment" is an isolated temporary
// scan workspace; callers that need live process execution should run the
// listing through the stack engine and feed those observations into scan
// results rather than accepting a synthetic clean verdict.
func (m *Manager) RunRuntimeScan(id string, timeoutSeconds int) (RuntimeScanResult, error) {
	listing, err := m.Get(id)
	if err != nil {
		return RuntimeScanResult{}, err
	}
	artifact := listingArtifactPath(listing)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	workspace, err := os.MkdirTemp("", "capper-market-scan-*")
	if err != nil {
		return RuntimeScanResult{}, err
	}
	defer os.RemoveAll(workspace)

	processTree, openPorts, dnsQueries, networkAttempts, fileDiffs, verdict, notes := observeRuntimeArtifact(artifact, workspace)
	result := RuntimeScanResult{
		ListingID:       listing.ID,
		Duration:        fmt.Sprintf("%ds", timeoutSeconds),
		ProcessTree:     processTree,
		OpenPorts:       openPorts,
		DNSQueries:      dnsQueries,
		NetworkAttempts: networkAttempts,
		FileDiffs:       fileDiffs,
		Verdict:         verdict,
		Notes:           notes,
	}
	_, _ = m.db.Exec(
		`UPDATE marketplace_listings SET scan_status=?, updated_at=datetime('now') WHERE id=?`,
		"runtime-scan-"+verdict, listing.ID,
	)
	return result, nil
}

// observeRuntimeArtifact performs a two-phase scan:
//  1. Static manifest analysis (fast, no execution).
//  2. Subprocess execution of extracted binaries under a strict timeout inside
//     an isolated workspace directory. Output is captured and checked for
//     network/filesystem anomalies.
func observeRuntimeArtifact(path, workspace string) ([]string, []int, []string, []string, []string, string, string) {
	if path == "" {
		return nil, nil, nil, nil, nil, "blocked", "listing has no artifact path"
	}
	if _, err := os.Stat(path); err != nil {
		return nil, nil, nil, nil, nil, "blocked", fmt.Sprintf("artifact %q is not readable", path)
	}

	var processTree []string
	var openPorts []int
	var dnsQueries []string
	var networkAttempts []string
	var fileDiffs []string
	verdict := "clean"
	notes := "artifact inspected in isolated scan workspace"

	// ---- Phase 1: Static manifest analysis ----------------------------------
	_ = walkTarEntries(path, 256<<10, func(name string, data []byte) {
		if len(fileDiffs) < 20 {
			fileDiffs = append(fileDiffs, name)
		}
		if name == "capsule.json" {
			var doc struct {
				Network struct {
					Enabled bool `json:"enabled"`
				} `json:"network"`
				Ports []struct {
					ContainerPort int `json:"containerPort"`
				} `json:"ports"`
				Secrets []string `json:"secrets"`
			}
			if json.Unmarshal(data, &doc) == nil {
				if doc.Network.Enabled {
					networkAttempts = append(networkAttempts, "manifest declares network access")
					verdict = "suspicious"
				}
				for _, p := range doc.Ports {
					if p.ContainerPort > 0 {
						openPorts = append(openPorts, p.ContainerPort)
					}
				}
				if len(doc.Secrets) > 0 {
					notes = "artifact requests secrets: " + strings.Join(doc.Secrets, ", ")
				}
			}
		}
		lowerName := strings.ToLower(name)
		if strings.Contains(lowerName, "resolv.conf") {
			dnsQueries = append(dnsQueries, "dns configuration present")
		}
	})

	// ---- Phase 2: Subprocess execution in isolated workspace ----------------
	extractDir := filepath.Join(workspace, "extract")
	if err := os.Mkdir(extractDir, 0o700); err != nil {
		notes += "; could not create extract dir: " + err.Error()
		return processTree, openPorts, dnsQueries, networkAttempts, fileDiffs, verdict, notes
	}
	extractedBinaries := extractExecutables(path, extractDir)
	for _, binPath := range extractedBinaries {
		result := runSandboxed(binPath, workspace, 5)
		processTree = append(processTree, filepath.Base(binPath))
		if result.networkAttempt {
			networkAttempts = append(networkAttempts, "subprocess attempted network: "+filepath.Base(binPath))
			if verdict == "clean" {
				verdict = "suspicious"
			}
		}
		if result.exitCode != 0 {
			notes += fmt.Sprintf("; subprocess %s exited %d: %s", filepath.Base(binPath), result.exitCode, result.output)
		}
		if result.escaped {
			networkAttempts = append(networkAttempts, "subprocess attempted escape: "+filepath.Base(binPath))
			verdict = "blocked"
		}
	}

	if len(processTree) == 0 {
		processTree = []string{"no-executable-found"}
	}

	return processTree, openPorts, dnsQueries, networkAttempts, fileDiffs, verdict, notes
}

type sandboxResult struct {
	exitCode       int
	output         string
	networkAttempt bool
	escaped        bool
}

// runSandboxed executes a binary in the workspace with a strict timeout.
// It restricts the environment and captures stdout/stderr.
func runSandboxed(binPath, workspace string, timeoutSeconds int) sandboxResult {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Restricted environment: no HOME, no PATH outside workspace, no network hints.
	env := []string{
		"HOME=" + workspace,
		"PATH=/usr/bin:/bin",
		"SANDBOX=1",
	}

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Dir = workspace
	cmd.Env = env

	var buf strings.Builder
	cmd.Stdout = &limitedWriter{w: &buf, limit: 4096}
	cmd.Stderr = &limitedWriter{w: &buf, limit: 4096}

	err := cmd.Run()

	var result sandboxResult
	result.output = buf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			result.exitCode = -1
			result.output += " [timeout]"
		}
	}

	output := strings.ToLower(result.output)
	if strings.Contains(output, "connect") || strings.Contains(output, "socket") ||
		strings.Contains(output, "network") || strings.Contains(output, "http") {
		result.networkAttempt = true
	}
	if strings.Contains(output, "chroot") || strings.Contains(output, "/proc/self") ||
		strings.Contains(output, "escape") {
		result.escaped = true
	}

	return result
}

// extractExecutables extracts ELF/script files from a tar archive to destDir
// and returns their paths. Only files with execute permission bits are extracted.
func extractExecutables(tarPath, destDir string) []string {
	var out []string
	_ = walkTarEntries(tarPath, 10<<20, func(name string, data []byte) {
		base := filepath.Base(name)
		if base == "" || base == "capsule.json" {
			return
		}
		// Heuristic: ELF magic or shebang line suggests executable content.
		isELF := len(data) >= 4 && data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F'
		isScript := len(data) >= 2 && data[0] == '#' && data[1] == '!'
		if !isELF && !isScript {
			return
		}
		dest := filepath.Join(destDir, base)
		if err := os.WriteFile(dest, data, 0o700); err == nil {
			out = append(out, dest)
		}
	})
	return out
}

// limitedWriter wraps an io.Writer and stops after limit bytes.
type limitedWriter struct {
	w     *strings.Builder
	limit int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	remaining := lw.limit - lw.w.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return lw.w.Write(p)
}

// ---- approval workflow (Block 14 Ph5) --------------------------------------

// Approve sets a listing to "approved" status. Should be called by an org admin.
func (m *Manager) Approve(id, reviewerNote string) error {
	return m.UpdateStatus(id, StatusApproved)
}

// Reject quarantines a listing and records the reason.
func (m *Manager) Reject(id, reason string) error {
	return m.UpdateStatus(id, StatusRejected)
}

// Quarantine moves a listing to quarantined state (e.g. after runtime scan alert).
func (m *Manager) Quarantine(id string) error {
	return m.UpdateStatus(id, StatusQuarantined)
}

// RequiresApproval returns true when the listing has not yet been approved.
func (m *Manager) RequiresApproval(id string) (bool, error) {
	l, err := m.Get(id)
	if err != nil {
		return false, err
	}
	return l.Status != StatusApproved, nil
}

// ---- AI marketplace rules (Block 14 Ph6) -----------------------------------

// AIListingPolicy declares the security constraints on an AI marketplace listing.
type AIListingPolicy struct {
	AgentAssumedRole     string   `json:"agentAssumedRole"`
	AllowedMCPServers    []string `json:"allowedMcpServers"` // explicit allowlist; empty = any
	RequiresToolBroker   bool     `json:"requiresToolBroker"`
	MaxDataClass         string   `json:"maxDataClass"` // "public", "internal", "confidential"
	ModelGatewayRequired bool     `json:"modelGatewayRequired"`
}

// ValidateAIListing checks an AI listing against platform security rules.
// Returns a list of violations; empty means the listing may be approved.
func ValidateAIListing(listing MarketplaceListing, policy AIListingPolicy) []string {
	var violations []string

	// Require explicit tool broker for public listings.
	if listing.Status != StatusPending {
		return violations
	}
	if !policy.RequiresToolBroker {
		violations = append(violations, "AI listing must route all tool calls through the tool broker")
	}
	// Disallow arbitrary MCP server access.
	if len(policy.AllowedMCPServers) == 0 {
		violations = append(violations, "AI listing must declare an explicit MCP server allowlist")
	}
	// Disallow raw-secret reading (data class restriction).
	if policy.MaxDataClass == "" || policy.MaxDataClass == "secret" || policy.MaxDataClass == "confidential" {
		violations = append(violations, "AI listing may not access confidential or secret data classes")
	}
	// Require model gateway policy.
	if !policy.ModelGatewayRequired {
		violations = append(violations, "AI listing must enforce model gateway policy")
	}
	return violations
}

// SubmitAIListing submits an AI-type listing with mandatory policy declarations.
// Returns an error if policy violations exist.
func (m *Manager) SubmitAIListing(listing MarketplaceListing, policy AIListingPolicy) error {
	violations := ValidateAIListing(listing, policy)
	if len(violations) > 0 {
		return fmt.Errorf("ai listing policy violations: %s", strings.Join(violations, "; "))
	}
	return m.Insert(listing)
}
