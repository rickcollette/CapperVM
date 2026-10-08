---
title: "Configuration"
description: "Global flags, environment variables, config files, and the API security flags."
owner: "docs"
status: "stable"
reviewed: "2026-06-12"
outputs:
  - markdown
  - web
  - pdf
---

# Configuration

Capper is configured through **global CLI flags**, **environment variables**, and
a few **config files**. There is no single monolithic config file; each layer has
a clear job.

## Global CLI flags

Available on every `capper` command:

| Flag | Default | Purpose |
| --- | --- | --- |
| `--store <path>` | `~/.capper` | Control-plane store directory (holds `capper.db`, keys). |
| `--project <name>` | `default` | Project namespace for resources. |
| `--runtime <backend>` | `auto` | Capsule runtime: `auto`, `bwrap`, `chroot`, `crun`, `runc`, `lxc`, `qemu`. Host default when instance omits `runtimeMode`. |
| `--json` | off | Emit JSON output where applicable. |
| `--debug` | off | Enable debug logging. |

## API server flags (`capper api start`)

| Flag | Default | Purpose |
| --- | --- | --- |
| `--listen <addr>` | `127.0.0.1:8686` | Listen address. |
| `--console <dir>` | — | Serve a CapperWeb `dist/` as the Web console. |
| `--with-daemon` | off | Also run the control-plane daemon (supervisor) in-process. |
| `--tls-cert <file>` | — | TLS certificate; enables HTTPS (requires `--tls-key`). |
| `--tls-key <file>` | — | TLS private key (requires `--tls-cert`). |
| `--allowed-origin <origin>` | — | Add a CORS origin allowed credentialed cross-origin access. Repeatable. Loopback origins are always allowed. |

**Security defaults that matter:**

- Without `--tls-cert`/`--tls-key`, the API serves **plain HTTP**. Session/CSRF
  cookies are `Secure`, so a cookie-based browser session only works behind TLS.
  Binding a **non-loopback** address without TLS logs a warning — front it with a
  TLS terminator or use the built-in flags.
- CORS uses an **allowlist**: loopback origins (localhost / 127.0.0.1 / ::1, any
  port) are always permitted; any other origin must be passed with
  `--allowed-origin`. Arbitrary origins are never reflected.

## Environment variables

### Database backend

| Variable | Default | Purpose |
| --- | --- | --- |
| `CAPPER_DB_DRIVER` | `sqlite` | `sqlite` (embedded) or `capdb` (networked; needs a `-tags capdb` build). |
| `CAPPER_DB_DSN` | — | CapDB DSN, e.g. `capdb://token@host:5432/capper.db?ca=…`. |
| `CAPPER_DB_TOKEN_FILE` | — | Path to a file holding the DB token (keeps it out of the DSN/env). |
| `CAPPER_DB_MAX_OPEN_CONNS` | `8` | Client connection-pool size (match the server `--pool-max`). |
| `CAPPER_DB_MAX_IDLE_CONNS` | `8` | Idle pool size. |
| `CAPPER_DB_CONN_MAX_LIFETIME_SECS` | `300` | Max connection lifetime. |
| `CAPPER_DB_STARTUP_RETRIES` | — | Bounded startup `Ping` retries while the DB comes up. |
| `CAPPER_DB_SYNCHRONOUS` | `NORMAL` | SQLite durability: `OFF`, `NORMAL`, `FULL`, or `EXTRA`. `FULL` for strict durability (no commit loss on power failure). |

### Co-located CapDB (used by `aio`)

| Variable | Purpose |
| --- | --- |
| `CAPPER_CAPDB_SERVER` | Path to `capdb-server` (default: from `$PATH`). |
| `CAPPER_CAPDB_DB_ROOT` | Server data root (default: `~/.capper/capdb`). |
| `CAPPER_CAPDB_CERT` / `CAPPER_CAPDB_KEY` | Server TLS material (omit → `--insecure`, local only). |

### Metadata service (node-side)

| Variable | Purpose |
| --- | --- |
| `CAPPER_METADATA_URL` | Instance metadata (IMDS) endpoint. |
| `CAPPER_METADATA_TOKEN_FILE` | Token file for metadata auth. |

### Local dev runner (`make capper-run`)

| Variable | Default | Purpose |
| --- | --- | --- |
| `CAPPER_RUN_API_ADDR` | `127.0.0.1:8687` | API listen address for the bundle. |
| `CAPPER_RUN_CONSOLE` | — | Path to a CapperWeb `dist/` to serve. |

## Config files

| Path | Written by | Contents |
| --- | --- | --- |
| `~/.capper/` (or `--store`) | control plane | `capper.db`, the IAM signing key (`iam.key`), secret/KMS master keys — all `0600`. |
| `/etc/capper/agent.yaml` | `aio init` / `node join` | Node identity, roles, control-plane URL, heartbeat. |
| `/etc/capper/aio.yaml` | `aio init` | AIO node name, storage root, mode. |
| `/etc/capper/capdb.env` | `aio init --backend capdb` | `CAPPER_DB_DRIVER`/`CAPPER_DB_DSN`/`CAPPER_DB_TOKEN_FILE` for the control plane. |
| `/etc/capper/capdb.auth` | `aio init --backend capdb` | CapDB auth file (token or `user:password` per line, `0600`). |

## Key custody

The IAM token-signing key and the secret/KMS master keys are 32-byte files stored
`0600` in the store directory. Protect the store directory accordingly.

Set **`CAPPER_MASTER_PASSPHRASE`** to derive all three keys from a runtime-supplied
passphrase (PBKDF2-HMAC-SHA256, per-key salt) instead of storing plaintext keys on
disk. See [Secrets → Key custody](../operator-guide/secrets.md#key-custody) and the
[security model](../concepts/security-model.md).

## Observability endpoint

`GET /api/v1/db/stats` returns the database connection-pool statistics
(`sql.DB.Stats()`: open/in-use/idle connections, `waitCount`, `waitDuration`) for
SLOs and pool-saturation alerts. See [Observability](../operator-guide/observability.md).

## Next

- [CapDB backend](../operator-guide/capdb-backend.md) · [Security model](../concepts/security-model.md)
  · [Troubleshooting](troubleshooting.md)
