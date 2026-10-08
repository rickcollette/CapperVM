---
title: "Upgrades"
description: "Seamless upgrades for AIO and enterprise deployments — versioning, migrations, rollback."
status: stable
---

# Upgrades

Capper supports low-downtime, auto-rollback upgrades for both deployment shapes.
Every binary is build-stamped; check it with:

```bash
capper version            # capper 1.0.1 (commit …, built …, go…, linux/amd64)
capper version --json
curl -s localhost:8080/api/v1/version   # version, commit, schemaVersion, apiVersion
```

## Versioning & compatibility

- Binaries are stamped at build time (`internal/version`, via `-ldflags -X`); the
  `VERSION` file is the release source of truth.
- **Control plane is upgraded first** and supports node agents at the current and
  previous release (N and N‑1).
- Schema migrations are **forward-only and additive**; a single upgrade is
  reversible (expand/contract). Inspect them with `capper schema status`.

## Database safety

Before applying migrations, take a consistent online snapshot:

```bash
capper schema status                 # applied + pending migrations, schema version
capper schema backup /var/backups/capper-pre-upgrade.db
```

`capper aio upgrade` does this automatically and restores it on rollback.

## AIO upgrade

Binaries install into a versioned layout (`/usr/local/lib/capper/<version>/` with
a `current` symlink), so activation and rollback are atomic symlink flips.

```bash
# From a downloaded bundle (verifies checksum):
sudo capper aio upgrade --bundle capper-aio-0.2.0-linux-amd64.tgz \
  --sha256 "$(cut -d' ' -f1 capper-aio-0.2.0-linux-amd64.tgz.sha256)"

# Or from an update channel feed:
export CAPPER_UPDATE_FEED=https://downloads.example.com/capper/aio/channels.json
capper aio check-update --channel stable
sudo capper aio upgrade --channel stable

# Roll back to the previous version:
sudo capper aio upgrade --rollback
```

The upgrade: verifies the bundle → stages the new version → stops services →
snapshots the DB → flips `current` → starts the new version (migrations run on
boot) → health-gates `/api/v1/health`. If health does not go green within
`--timeout`, it **automatically restores the prior binaries and database**.

## Enterprise rolling upgrade

Upgrade the control plane (and CapDB) first, within a maintenance window (until
control-plane HA lands — see [ADR 0001](../architecture/adr-0001-capdb-availability.md)),
then roll the agents node-by-node:

```bash
export CAPPER_CONTROL_URL=https://control.example.com
export CAPPER_TOKEN=…

capper fleet status                              # per-node version + drain state
capper fleet upgrade --target 0.2.0 --batch 1 \
  --exec 'ssh {node} sudo capper aio upgrade --channel stable --yes'
```

For each node the driver cordons + drains it, runs the `--exec` upgrade command,
waits for the node to re-report the target version in its heartbeat, then
uncordons — `--batch N` at a time, with a control-plane health gate between
batches. It stops on the first failure. Use `--dry-run` to preview the plan.

## Building a release bundle

```bash
CAPPER_VERSION=0.2.0 scripts/build-aio.sh 0.2.0
# -> DIST/AIO/capper-aio-0.2.0-linux-amd64.tgz (+ .sha256, + channels.json entry)
```

Publish the tarball, its `.sha256`, and the `channels.json` feed entry to your
download host; point `CAPPER_UPDATE_FEED` at the feed.
