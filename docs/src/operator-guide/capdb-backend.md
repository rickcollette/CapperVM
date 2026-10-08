---
title: "CapDB networked storage backend"
description: "Run Capper's control-plane database as a networked, pooled CapDB service."
owner: "docs"
status: "stable"
reviewed: "2026-10-08"
outputs:
  - markdown
  - web
  - pdf
---

# CapDB Storage Backend (networked)

By default Capper stores all control-plane state in a single embedded SQLite
database via the pure-Go `modernc.org/sqlite` driver — no external process, ideal
for single-node and hermetic tests. For deployments that need a **networked,
connection-pooled** database without leaving the SQLite SQL dialect, Capper can
instead talk to **CapDB**, a hard fork of SQLite that adds a TLS client/server
protocol, a native connection pool, a volume store, and WAL replication
primitives. Capper pins **CapDB 3.7.2**. The sources are not in this
repository: `make capdb-fetch` clones
[rickcollette/CapDB](https://github.com/rickcollette/CapDB) into `./CapDB`
and detaches at `CAPDB_REF` (default `v3.7.2`).

The CapDB backend is **opt-in** and requires a binary built with the `capdb`
build tag (it links a cgo client library). The default build remains pure-Go.

## When to use it

- You need multiple Capper processes (or nodes) sharing one database.
- You want server-side connection pooling and TLS between the control plane and
  storage.
- You want to avoid rewriting any SQL (CapDB keeps the SQLite dialect).

Stay on the default `sqlite` backend for single-node installs, local
development, and CI.

## Build

The networked driver needs a C toolchain and OpenSSL. Build the CapDB client
library and server, then build Capper with the `capdb` tag:

```bash
make capdb-fetch                 # clone CapDB and detach at CAPDB_REF (v3.7.2)
make capdb                       # build/capdb: libcapdb_client.a, capdb-server, capdb
go build -tags capdb ./cmd/capper
```

## Run the server

```bash
# auth file: one token per line, or user:password per line
printf 'super-secret-token\n' > /etc/capper/capdb.auth
chmod 600 /etc/capper/capdb.auth

build/capdb/capdb-server \
  --listen 0.0.0.0:5432 \
  --auth-file /etc/capper/capdb.auth \
  --db-root /var/lib/capper/db \
  --cert /etc/capper/tls/server.crt \
  --key  /etc/capper/tls/server.key \
  --pool-min 1 --pool-max 8
```

For local development you can disable TLS with `--insecure` (and add
`&insecure=1` to the DSN). Never use `--insecure` in production.

## Production deployment (TLS + systemd)

Generate (or provision) a server certificate. For an internal CA you control:

```bash
sudo mkdir -p /etc/capper/tls && cd /etc/capper/tls
# self-signed example — replace with your CA-issued cert in real deployments
openssl req -x509 -newkey rsa:4096 -nodes -days 825 \
  -keyout server.key -out server.crt \
  -subj "/CN=db-host" -addext "subjectAltName=DNS:db-host"
sudo chmod 600 server.key
```

Run `capdb-server` under systemd:

```ini
# /etc/systemd/system/capdb-server.service
[Unit]
Description=CapDB SQL server
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/capdb-server \
  --listen 0.0.0.0:5432 \
  --auth-file /etc/capper/capdb.auth \
  --db-root /var/lib/capper/db \
  --cert /etc/capper/tls/server.crt \
  --key  /etc/capper/tls/server.key \
  --pool-min 2 --pool-max 16 \
  --max-clients 256
Restart=on-failure
User=capper
Group=capper
AmbientCapabilities=
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/capper/db
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload && sudo systemctl enable --now capdb-server
```

The client verifies the server certificate against the CA you pass via `ca=` in
the DSN (the hostname must match the cert SAN). For **mutual TLS**, also pass
`--ca <client-ca.pem>` to the server; it then requires a client certificate.

**Health check:** the client `Ping`s on startup; you can also probe liveness
with a TCP connect to the listen port, or `capdb 'capdb://…'` then `.quit`.

## All-in-one (co-located server)

`capper aio up` runs the default in-process modernc backend unless the CapDB
backend is selected. When `CAPPER_DB_DRIVER=capdb` is set, `aio up`
**auto-launches a co-located `capdb-server`** wired to a local db-root and an
auth file derived from the DSN token, then starts the control plane against it
(and stops it on `aio down`):

```bash
export CAPPER_DB_DRIVER=capdb
export CAPPER_DB_DSN='capdb://token@127.0.0.1:5432/capper.db?token=local&insecure=1'
# optional overrides:
export CAPPER_CAPDB_SERVER=/usr/local/bin/capdb-server   # default: from $PATH
export CAPPER_CAPDB_DB_ROOT=/var/lib/capper/db           # default: ~/.capper/capdb
export CAPPER_CAPDB_CERT=/etc/capper/tls/server.crt      # omit -> --insecure (local)
export CAPPER_CAPDB_KEY=/etc/capper/tls/server.key
capper aio up
```

## Point Capper at it

Selection is entirely by environment variable — no code or config-file change:

```bash
export CAPPER_DB_DRIVER=capdb
export CAPPER_DB_DSN='capdb://token@db-host:5432/capper.db?token=super-secret-token'
# optional client-side pool tuning (defaults shown)
export CAPPER_DB_MAX_OPEN_CONNS=16
export CAPPER_DB_MAX_IDLE_CONNS=8
export CAPPER_DB_CONN_MAX_LIFETIME_SECS=300

capper api start            # binary built with -tags capdb
```

### DSN format

```text
capdb://[user@]host[:port]/dbname?token=…&password=…&ca=…&insecure=1
```

- `token=` → token auth; `user@…&password=` → username/password auth.
- `token_file=` → read the token from a file instead of placing it in the DSN.
  `CAPPER_DB_TOKEN_FILE` does the same on the Capper side and injects `token=`
  only in memory.
- `ca=` → CA bundle to verify the server certificate (TLS).
- `insecure=1` → plain TCP, development only. CapDB 3.7 refuses to start TLS
  unless both `--cert` and `--key` are set.
- Default port is `5432`. `--listen` also accepts `[IPv6]:port` and an absolute
  Unix socket path (mode `0600`).

On startup Capper pings the server and fails fast with a clear error if it is
unreachable. If you set `CAPPER_DB_DRIVER=capdb` on a binary built **without**
`-tags capdb`, Capper reports that the driver is not compiled in.

## Behavioral notes / limitations

- **Transactions are atomic.** Outside a transaction, each statement runs in
  autocommit mode on a pooled connection. On `BEGIN` the server pins a write
  connection to the session and suppresses auto-commit until `COMMIT`/`ROLLBACK`,
  so transactions are fully atomic (`Rollback` undoes). Concurrent writers
  serialize via the pool (one writer at a time); reads run concurrently under
  WAL. Keep transactions short to avoid blocking other writers.
- **WAL / busy-timeout** pragmas are SQLite-local and do not apply here; the
  server owns the database file and its own pool (`--pool-min/--pool-max`).
- The CapDB engine is maintained at <https://github.com/rickcollette/CapDB>.

## Performance notes

- **Pooling.** The control plane opens up to `CAPPER_DB_MAX_OPEN_CONNS` (default 8)
  client connections; size the server's `--pool-max` to match so client
  connections never block on a server-side acquire. `capper aio` does this
  automatically.
- **Row batching.** The driver prefetches result rows in batches (256), so a
  `SELECT` returning N rows costs ~N/256 round-trips, not N.
- **Context cancellation.** Cancelling the request context aborts an in-flight
  query/exec (the socket is shut down and the connection discarded).

## Operations

### Backup

The server owns the database file at `<db-root>/capper.db`. Back it up with the
CapDB CLI's online backup (safe while the server runs):

```bash
capdb <db-root>/capper.db ".backup '/backups/capper-$(date +%F).db'"
```

Or snapshot the file with the server stopped. Restore by stopping the server,
replacing the file, and starting it again.

**Automate it with a systemd timer.** Schedule the online `.backup` and prune old
copies:

```ini
# /etc/systemd/system/capdb-backup.service
[Unit]
Description=CapDB online backup
After=capdb-server.service
Requires=capdb-server.service

[Service]
Type=oneshot
User=capper
Environment=DB_ROOT=/var/lib/capper/db BACKUP_DIR=/backups RETAIN_DAYS=14
ExecStart=/bin/sh -c 'mkdir -p "$BACKUP_DIR" && \
  capdb "$DB_ROOT/capper.db" ".backup '"'"'$BACKUP_DIR/capper-$(date +%%F-%%H%%M).db'"'"'" && \
  find "$BACKUP_DIR" -name "capper-*.db" -mtime +$RETAIN_DAYS -delete'
```

```ini
# /etc/systemd/system/capdb-backup.timer
[Unit]
Description=Run CapDB backup daily

[Timer]
OnCalendar=*-*-* 02:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

```bash
sudo systemctl enable --now capdb-backup.timer
```

Define and test your restore path: restore the latest backup onto a spare host,
point a control plane at it, and run a smoke flow. An untested backup is not a
recovery plan — the [availability ADR](../architecture/adr-0001-capdb-availability.md)
bounds recovery time on this procedure.

### Monitoring

Watch the **client-side** pool via the control plane: `GET /api/v1/db/stats`
exposes `sql.DB.Stats()` (`inUse`/`idle`/`waitCount`/`waitDurationMillis`). A
rising `waitCount` means the client pool or the server `--pool-max` is too small.
On the server, a growing number of sessions blocked on `BEGIN IMMEDIATE` indicates
write contention — keep transactions short.

### Audit logging

`capdb-server` emits structured stderr audit lines for connection lifecycle and
authentication, so failed AUTH (e.g. brute force) and unauthorized access attempts
are visible:

```text
capdb audit event=conn.accept peer=10.0.0.5:51234
capdb audit event=auth.ok peer=10.0.0.5:51234 detail=token
capdb audit event=auth.fail peer=10.0.0.9:40021 detail=token
capdb audit event=conn.close peer=10.0.0.5:51234
```

Ship the server's journal to your log pipeline and alert on `auth.fail` rate.

### Schema migrations

Capper's additive `ALTER TABLE`/`CREATE TABLE IF NOT EXISTS` startup migrations
run through the normal `EXEC` path and work unchanged against the network
backend; no migration tooling change is needed.

### Availability posture

Capper's supported posture is still **one `capdb-server`** on `--db-root` — a
single point of failure, like the embedded SQLite it replaces, but reachable by
multiple Capper processes. Run it close to the control plane, back it up with
the `capdb` shell, and let systemd restart it (`Restart=on-failure`).

CapDB 3.7 can also run a volume store with WAL replication (`--storage volume`).
Capper does not configure that mode. Failover stays an operator action. See
[ADR 0001](../architecture/adr-0001-capdb-availability.md).

## Verify

Run the driver conformance suite against a real server:

```bash
make test-capdb     # builds capdb, runs internal/capdbdriver integration tests
```
