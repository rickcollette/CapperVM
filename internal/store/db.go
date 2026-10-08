package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	capperdns "capper/internal/dns"
	"capper/internal/ai"
	"capper/internal/alert"
	"capper/internal/backup"
	"capper/internal/cert"
	"capper/internal/compute"
	autoscalestore "capper/internal/autoscale/store"
	csdstore "capper/internal/csd/store"
	"capper/internal/topology"
	"capper/internal/database"
	"capper/internal/health"
	"capper/internal/lb"
	"capper/internal/firewall"
	"capper/internal/host"
	"capper/internal/iam"
	"capper/internal/kms"
	"capper/internal/marketplace"
	"capper/internal/billing"
	"capper/internal/eventing"
	"capper/internal/ingress"
	"capper/internal/metadata"
	"capper/internal/queue"
	"capper/internal/network"
	"capper/internal/networking"
	"capper/internal/posture"
	"capper/internal/org"
	"capper/internal/registry"
	"capper/internal/resource"
	"capper/internal/secret"
	caps3 "capper/internal/s3server"
	"capper/internal/stack"
	"capper/internal/vpc"
	"capper/internal/storage"
	bottlestore "capper/internal/bottle/store"
	"capper/internal/vpcmover"
	"capper/internal/audit"
	"capper/internal/quotas"
	"capper/internal/resourcemon"
	"capper/internal/functions"
	"capper/internal/mcpserver"
	"capper/internal/ipam"
	"capper/internal/adminconfig"
	"capper/internal/hoststorage"
	"capper/internal/hostsec/fail2ban"
)

type Store struct {
	DB        *sql.DB
	Paths     Paths
	SecretKey []byte // AES-256 master key for secrets (also used for S3 credential encryption)
	Resources *resource.Store
	Projects  *org.Store
	IAM       *iam.Manager
	Hosts     *host.Store
	Networks    *network.Store
	Networking  *networking.Service
	Firewalls   *firewall.Store
	DNS       *capperdns.Store
	Compute   *compute.Store
	Storage   *storage.Store
	Registry  *registry.Store
	Events    *EventStore
	Secrets   *secret.Manager
	KMS       *kms.Manager
	Certs     *cert.Manager
	Posture   *posture.Scanner
	LB        *lb.Manager
	Backup    *backup.Manager
	Health    *health.Store
	Stack       *stack.Manager
	Jobs        *stack.JobStore
	Bottles     *bottlestore.Store
	Databases   *database.Manager
	AI          *ai.Manager
	Marketplace *marketplace.Manager
	Metadata    *metadata.Manager
	Billing     *billing.Manager
	Queue       *queue.Manager
	Ingress     *ingress.Manager
	Eventing    *eventing.Manager
	CSD         *csdstore.Store
	Autoscale   *autoscalestore.Store
	Topology    *topology.Manager
	VPC         *vpc.Manager
	VPCMover    *vpcmover.Store
	Audit       *audit.Store
	Quotas      *quotas.Store
	ResourceMon *resourcemon.Store
	Functions   *functions.Store
	MCPServers  *mcpserver.Store
	IPAM        *ipam.Store
	AdminConfig *adminconfig.Store
	HostStorage *hoststorage.Store
	Fail2ban    *fail2ban.Store
}

func Open(paths Paths) (*Store, error) {
	if err := paths.Ensure(); err != nil {
		return nil, err
	}
	db, err := openDB(paths)
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db, Paths: paths}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	// Backfill org_id and account_id columns to existing tables. Versioned and
	// idempotent: duplicate-column errors (already applied) are ignored, but any
	// other failure surfaces instead of being silently swallowed.
	if err := ensureMigrationsTable(db); err != nil {
		db.Close()
		return nil, err
	}
	var tenancyStmts []string
	for _, tbl := range []string{"projects", "instances", "images"} {
		tenancyStmts = append(tenancyStmts,
			`ALTER TABLE `+tbl+` ADD COLUMN org_id TEXT NOT NULL DEFAULT 'org_local'`,
			`ALTER TABLE `+tbl+` ADD COLUMN account_id TEXT NOT NULL DEFAULT 'acct_local'`)
	}
	tenancyStmts = append(tenancyStmts,
		`UPDATE projects SET org_id='org_local', account_id='acct_local' WHERE org_id='' OR org_id IS NULL`,
		`UPDATE instances SET org_id='org_local', account_id='acct_local' WHERE org_id='' OR org_id IS NULL`,
		`UPDATE images SET org_id='org_local', account_id='acct_local' WHERE org_id='' OR org_id IS NULL`)
	if err := applyMigration(db, "0001_tenancy_columns", tenancyStmts); err != nil {
		db.Close()
		return nil, err
	}

	s.Resources = resource.NewStore(db)
	s.Projects = org.NewStore(db)
	if err := s.Projects.EnsureDefault(); err != nil {
		db.Close()
		return nil, err
	}
	iamStore := iam.NewStore(db)
	iamMgr, err := iam.NewManager(iamStore, paths.Root)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := iamMgr.Bootstrap(); err != nil {
		db.Close()
		return nil, err
	}
	// Wire IAM local principal as org/account root user.
	if err := bootstrapOrgIAM(s.Projects, iamMgr); err != nil {
		_ = err // non-fatal — log would go here
	}
	s.IAM = iamMgr
	s.Hosts = host.NewStore(db)
	s.Networks = network.NewStore(db)
	s.Firewalls = firewall.NewStore(db)
	s.DNS = capperdns.NewStore(db)
	s.Compute = compute.NewStore(db)
	s.Storage = storage.NewStore(db)
	s.Registry = registry.NewStore(db)
	s.Events = newEventStore(db)
	secretKey, err := secret.LoadOrCreateKey(filepath.Join(paths.Root, "secret.key"))
	if err != nil {
		db.Close()
		return nil, err
	}
	kmsKey, err := secret.LoadOrCreateKey(filepath.Join(paths.Root, "kms.key"))
	if err != nil {
		db.Close()
		return nil, err
	}
	s.SecretKey = secretKey
	s.Secrets = secret.NewManager(secret.NewStore(db), secretKey)
	s.KMS = kms.NewManager(kms.NewStore(db), kmsKey)
	ca, err := cert.LoadOrCreateCA(paths.Root)
	if err != nil {
		db.Close()
		return nil, err
	}
	s.Certs = cert.NewManager(ca, cert.NewStore(db))
	s.Posture = posture.NewScanner(posture.NewStore(db))
	s.LB = lb.NewManager(lb.NewStore(db))
	s.Backup = backup.NewManager(backup.NewStore(db), db, paths.Root)
	s.Health = health.NewStore(db)
	s.Stack = stack.NewManager(stack.NewStore(db), stack.Deps{Networks: s.Networks, DNS: s.DNS})
	s.Jobs = stack.NewJobStore(db)
	s.Bottles = bottlestore.New(db)
	s.Databases = database.NewManager(database.NewStore(db))
	s.AI = ai.NewManager(ai.NewStore(db))
	s.Marketplace = marketplace.NewManager(db)
	metaStore := metadata.NewStore(db)
	s.Metadata = metadata.NewManager(metaStore, nil)
	s.Billing = billing.NewManager(billing.NewStore(db))
	s.Queue = queue.NewManager(queue.NewStore(db))
	s.Ingress = ingress.NewManager(ingress.NewStore(db))
	s.Eventing = eventing.NewManager(db)
	s.CSD = csdstore.New(db)
	s.Autoscale = autoscalestore.New(db)
	s.Topology = topology.NewManager(db)
	if err := s.Topology.EnsureLocalTopology(); err != nil {
		db.Close()
		return nil, err
	}
	s.VPC = vpc.NewManager(db)
	s.Networking = networking.NewService(db, s.Topology.Store())
	s.VPCMover = vpcmover.NewStore(db)
	s.Audit = audit.NewStore(db)
	s.Quotas = quotas.NewStore(db)
	s.ResourceMon = resourcemon.NewStore(db)
	s.Functions = functions.NewStore(db)
	s.MCPServers = mcpserver.NewStore(db)
	s.IPAM = ipam.NewStore(db)
	s.AdminConfig = adminconfig.NewStore(db)
	s.HostStorage = hoststorage.NewStore(db)
	s.Fail2ban = fail2ban.NewStore(db)
	return s, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) init() error {
	if err := resource.InitSchema(s.DB); err != nil {
		return err
	}
	if err := org.InitSchema(s.DB); err != nil {
		return err
	}
	if err := iam.InitSchema(s.DB); err != nil {
		return err
	}
	if err := iam.InitCrossAccountSchema(s.DB); err != nil {
		return err
	}
	if err := host.InitSchema(s.DB); err != nil {
		return err
	}
	if err := network.InitSchema(s.DB); err != nil {
		return err
	}
	if err := firewall.InitSchema(s.DB); err != nil {
		return err
	}
	if err := capperdns.InitSchema(s.DB); err != nil {
		return err
	}
	if err := compute.InitSchema(s.DB); err != nil {
		return err
	}
	if err := storage.InitSchema(s.DB); err != nil {
		return err
	}
	if err := storage.InitShareSchema(s.DB); err != nil {
		return err
	}
	if err := registry.InitSchema(s.DB); err != nil {
		return err
	}
	if err := registry.InitTokenSchema(s.DB); err != nil {
		return err
	}
	if err := InitEventSchema(s.DB); err != nil {
		return err
	}
	if err := secret.InitSchema(s.DB); err != nil {
		return err
	}
	if err := kms.InitSchema(s.DB); err != nil {
		return err
	}
	if err := cert.InitSchema(s.DB); err != nil {
		return err
	}
	if err := posture.InitSchema(s.DB); err != nil {
		return err
	}
	if err := lb.InitSchema(s.DB); err != nil {
		return err
	}
	if err := alert.InitSchema(s.DB); err != nil {
		return err
	}
	if err := backup.InitSchema(s.DB); err != nil {
		return err
	}
	if err := health.InitSchema(s.DB); err != nil {
		return err
	}
	if err := stack.InitSchema(s.DB); err != nil {
		return err
	}
	if err := stack.InitJobSchema(s.DB); err != nil {
		return err
	}
	if err := bottlestore.InitSchema(s.DB); err != nil {
		return err
	}
	if err := database.InitSchema(s.DB); err != nil {
		return err
	}
	if err := ai.InitSchema(s.DB); err != nil {
		return err
	}
	if err := marketplace.InitSchema(s.DB); err != nil {
		return err
	}
	if err := metadata.InitSchema(s.DB); err != nil {
		return err
	}
	if err := billing.InitSchema(s.DB); err != nil {
		return err
	}
	if err := queue.InitSchema(s.DB); err != nil {
		return err
	}
	if err := ingress.InitSchema(s.DB); err != nil {
		return err
	}
	if err := csdstore.InitSchema(s.DB); err != nil {
		return err
	}
	if err := autoscalestore.InitSchema(s.DB); err != nil {
		return err
	}
	if err := caps3.InitS3CredSchema(s.DB); err != nil {
		return err
	}
	if err := caps3.InitBucketPolicySchema(s.DB); err != nil {
		return err
	}
	if err := vpc.InitSchema(s.DB); err != nil {
		return err
	}
	if err := vpcmover.InitSchema(s.DB); err != nil {
		return err
	}
	if err := audit.InitSchema(s.DB); err != nil {
		return err
	}
	if err := quotas.NewStore(s.DB).InitSchema(); err != nil {
		return err
	}
	if err := resourcemon.NewStore(s.DB).InitSchema(); err != nil {
		return err
	}
	if err := functions.NewStore(s.DB).InitSchema(); err != nil {
		return err
	}
	if err := mcpserver.NewStore(s.DB).InitSchema(); err != nil {
		return err
	}
	if err := ipam.NewStore(s.DB).InitSchema(); err != nil {
		return err
	}
	if err := adminconfig.NewStore(s.DB).InitSchema(); err != nil {
		return err
	}
	if err := hoststorage.NewStore(s.DB).InitSchema(); err != nil {
		return err
	}
	if err := fail2ban.NewStore(s.DB).InitSchema(); err != nil {
		return err
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS images (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			version TEXT NOT NULL,
			path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			digest TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS instances (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			image_name TEXT NOT NULL,
			image_id TEXT NOT NULL,
			image_digest TEXT NOT NULL,
			pid INTEGER,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			started_at TEXT,
			stopped_at TEXT,
			rootfs_path TEXT NOT NULL,
			command TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return err
		}
	}
	// Additive migrations: add columns that may not exist in older DBs.
	// Only suppress the expected "duplicate column name" error; surface anything else.
	for _, alter := range []string{
		`ALTER TABLE instances ADD COLUMN restart_policy TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE instances ADD COLUMN restart_count INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := s.DB.Exec(alter); err != nil {
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("schema migration failed (%s): %w", alter, err)
			}
		}
	}
	// Topology schema runs after instances and lb_load_balancers tables exist
	// because it adds columns to them via ALTER TABLE.
	if err := topology.InitSchema(s.DB); err != nil {
		return err
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	return strings.Contains(err.Error(), "duplicate column name")
}

// ensureMigrationsTable creates the schema_migrations bookkeeping table.
func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}
	return nil
}

// applyMigration runs an additive migration once. If the version is already
// recorded it is a no-op. Otherwise each statement runs; "duplicate column"
// errors (column already present from an older unversioned run) are tolerated so
// the migration is idempotent, but any other error surfaces instead of being
// silently swallowed. On success the version is recorded.
func applyMigration(db *sql.DB, version string, stmts []string) error {
	var applied string
	err := db.QueryRow(`SELECT version FROM schema_migrations WHERE version=?`, version).Scan(&applied)
	if err == nil {
		return nil // already applied
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			if isDuplicateColumnError(err) {
				continue
			}
			return fmt.Errorf("store: migration %s failed (%s): %w", version, stmt, err)
		}
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
		version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("store: record migration %s: %w", version, err)
	}
	return nil
}

// bootstrapOrgIAM wires the local IAM principal as org/account root user.
// Duplicate-entry errors are silently ignored (idempotent).
func bootstrapOrgIAM(orgStore *org.Store, iamMgr *iam.Manager) error {
	_, pid := iamMgr.LocalPrincipal()
	if pid == "" || pid == "unknown" {
		return nil
	}
	if _, err := orgStore.AddOrgRootUser("org_local", pid, pid); err != nil {
		if !isDuplicateError(err) {
			return err
		}
	}
	if _, err := orgStore.AddAccountRootUser("org_local", "acct_local", pid, pid); err != nil {
		if !isDuplicateError(err) {
			return err
		}
	}
	return nil
}

func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "duplicate")
}
