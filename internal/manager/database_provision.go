package manager

import (
	"fmt"
	"os"
	"path/filepath"

	"capper/internal/database"
	"capper/internal/metadata"
	"capper/internal/systemlabels"
	"capper/internal/types"
)

// ProvisionDatabase launches a hidden alpine instance running the requested engine.
// netOpts places the instance on a VPC subnet; it must be non-nil.
func (m InstanceManager) ProvisionDatabase(meta *metadata.Manager, db database.ManagedDB, project, password, image string, netOpts *NetworkRunOpts) (string, error) {
	if netOpts == nil {
		return "", fmt.Errorf("database: network placement is required")
	}
	if err := m.Store.CheckHostDeployLimit(); err != nil {
		return "", err
	}
	if image == "" {
		image = "alpine"
	}
	labels := map[string]string{
		"project":                       project,
		systemlabels.Hidden:             "true",
		systemlabels.Managed:            systemlabels.ManagedDB,
		systemlabels.DatabaseID:         db.ID,
		"capper.system/database-engine": string(db.Engine),
	}
	env := map[string]string{
		"CAPPER_DB_ENGINE":   string(db.Engine),
		"CAPPER_DB_PASSWORD": password,
		"CAPPER_DB_PORT":     fmt.Sprintf("%d", db.Port),
		"CAPPER_DB_NAME":     db.Name,
	}
	resources := types.ResourceOverrides{}
	resources.Limits.MemoryBytes = 512 << 20
	resources.MemorySet = true
	resources.Limits.DiskBytes = 8 << 30
	resources.DiskSet = true
	resources.Limits.MaxProcesses = 256
	resources.PidsSet = true
	resources.Limits.CPUCount = 1
	resources.CPUSet = true

	userData := dbEngineUserData(db.Engine)
	entrypoint, args := dbEngineEntrypoint(db.Engine)

	inst, err := m.Run(image, resources, RunOptions{
		Name:       "db-" + db.Name,
		Labels:     labels,
		Env:        env,
		Entrypoint: entrypoint,
		Args:       args,
		Network:    netOpts,
	})
	if err != nil {
		return "", fmt.Errorf("database: launch instance: %w", err)
	}

	if meta != nil && inst.NetworkIP != "" {
		_, _ = meta.CreateRecord(metadata.InstanceMetadata{
			InstanceID: inst.ID,
			Hostname:   inst.Name,
			Project:    project,
			Labels:     labels,
			NetworkIP:  inst.NetworkIP,
			UserData:   userData,
		})
	}

	if db.Engine == database.EngineCapDB {
		_ = copyCapDBServer(inst.RootFSPath)
	}

	return inst.ID, nil
}

func copyCapDBServer(rootfs string) error {
	src := "/usr/local/bin/capdb-server"
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	dst := filepath.Join(rootfs, "usr", "local", "bin", "capdb-server")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

func dbEngineUserData(engine database.DBEngine) string {
	switch engine {
	case database.EnginePostgres:
		return postgresUserData()
	case database.EngineRedis:
		return redisUserData()
	case database.EngineMariaDB:
		return mariadbUserData()
	case database.EngineCapDB:
		return capdbUserData()
	default:
		return ""
	}
}

func dbEngineEntrypoint(engine database.DBEngine) ([]string, []string) {
	switch engine {
	case database.EnginePostgres:
		return []string{"/bin/bash", "-lc"}, []string{postgresStartScript()}
	case database.EngineRedis:
		return []string{"/bin/bash", "-lc"}, []string{redisStartScript()}
	case database.EngineMariaDB:
		return []string{"/bin/bash", "-lc"}, []string{mariadbStartScript()}
	case database.EngineCapDB:
		return []string{"/bin/bash", "-lc"}, []string{capdbStartScript()}
	default:
		return nil, nil
	}
}

func postgresUserData() string {
	return `#!/bin/bash
set -e
apk add --no-cache --no-scripts postgresql16
`
}

func postgresStartScript() string {
	return `set -e
apk add --no-cache --no-scripts postgresql16
mkdir -p /run/postgresql /var/lib/postgresql/data
chown -R postgres:postgres /run/postgresql /var/lib/postgresql
if [ ! -f /var/lib/postgresql/data/PG_VERSION ]; then
  su postgres -c "initdb -D /var/lib/postgresql/data --auth=scram-sha-256"
fi
echo "host all all 0.0.0.0/0 scram-sha-256" >> /var/lib/postgresql/data/pg_hba.conf
echo "listen_addresses='*'" >> /var/lib/postgresql/data/postgresql.conf
su postgres -c "pg_ctl -D /var/lib/postgresql/data -l /var/log/postgresql.log start"
su postgres -c "psql -v ON_ERROR_STOP=1 postgres" <<EOF
DO $$ BEGIN
  CREATE USER capper WITH PASSWORD '$CAPPER_DB_PASSWORD';
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
CREATE DATABASE "$CAPPER_DB_NAME" OWNER capper;
EOF
exec tail -f /var/log/postgresql.log
`
}

func redisUserData() string {
	return `#!/bin/bash
set -e
apk add --no-cache --no-scripts redis
`
}

func redisStartScript() string {
	return `set -e
apk add --no-cache --no-scripts redis
mkdir -p /var/lib/redis
echo "requirepass $CAPPER_DB_PASSWORD" >> /etc/redis.conf
echo "bind 0.0.0.0" >> /etc/redis.conf
exec redis-server /etc/redis.conf --dir /var/lib/redis
`
}

func mariadbUserData() string {
	return `#!/bin/bash
set -e
apk add --no-cache --no-scripts mariadb mariadb-client
`
}

func mariadbStartScript() string {
	return "set -e\n" +
		"apk add --no-cache --no-scripts mariadb mariadb-client\n" +
		"mkdir -p /run/mysqld /var/lib/mysql\n" +
		"chown -R mysql:mysql /run/mysqld /var/lib/mysql\n" +
		"if [ ! -d /var/lib/mysql/mysql ]; then\n" +
		"  mysql_install_db --user=mysql --datadir=/var/lib/mysql\n" +
		"fi\n" +
		"mysqld --user=mysql --bind-address=0.0.0.0 &\n" +
		"for i in $(seq 1 30); do mysqladmin ping --silent && break; sleep 1; done\n" +
		"mysql -u root <<EOF\n" +
		"CREATE DATABASE IF NOT EXISTS `$CAPPER_DB_NAME`;\n" +
		"CREATE USER IF NOT EXISTS 'capper'@'%' IDENTIFIED BY '$CAPPER_DB_PASSWORD';\n" +
		"GRANT ALL ON `$CAPPER_DB_NAME`.* TO 'capper'@'%';\n" +
		"FLUSH PRIVILEGES;\n" +
		"EOF\n" +
		"exec tail -f /dev/null\n"
}

func capdbUserData() string {
	return `#!/bin/bash
set -e
mkdir -p /var/lib/capdb /etc/capdb
`
}

func capdbStartScript() string {
	return `set -e
mkdir -p /var/lib/capdb /etc/capdb
if [ -x /usr/local/bin/capdb-server ]; then
  echo "$CAPPER_DB_PASSWORD" > /etc/capdb/auth
  chmod 600 /etc/capdb/auth
  exec /usr/local/bin/capdb-server --listen 0.0.0.0:$CAPPER_DB_PORT --auth-file /etc/capdb/auth --db-root /var/lib/capdb
fi
apk add --no-cache --no-scripts postgresql16
` + postgresStartScript()
}
