---
title: "Quickstart"
description: "Run a capsule, then bring up the control plane and Web console."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Quickstart

Two paths: run a single `.cap` capsule locally, or bring up the full control
plane. Start with whichever matches your goal.

> Do not run untrusted `.cap` images with Capper v0.

## Run a capsule

```bash
sh examples/alpine/bootstrap.sh
go run ./cmd/capper --store /tmp/capper-alpine create alpine.cap examples/alpine/capper.json
go run ./cmd/capper --store /tmp/capper-alpine run alpine.cap
go run ./cmd/capper --store /tmp/capper-alpine list instances
```

Capper prefers Bubblewrap (`bwrap`) with unprivileged user namespaces and falls
back to `chroot`. Choose explicitly with `--runtime bwrap|chroot|crun|runc|lxc|qemu`. Limit
resources:

```bash
go run ./cmd/capper --store /tmp/capper-alpine run \
  --memory 128M --cpu-time 60 --file-size 16M alpine.cap
```

## Run the control plane (local dev)

The fastest path builds a runnable bundle and starts the API with the daemon
embedded:

```bash
make capper-run            # serves http://127.0.0.1:8687
make capper-run-status
make capper-run-stop
```

Defaults written under `capper-run/`:

```text
URL:   http://127.0.0.1:8687
PID:   capper-run/run/api.pid
Log:   capper-run/logs/api.log
Store: capper-run/store
```

Override via environment:

```bash
CAPPER_RUN_API_ADDR=127.0.0.1:8690 make capper-run
CAPPER_RUN_CONSOLE=/path/to/CapperWeb/dist make capper-run   # serve the Web UI
```

## Start the API directly

```bash
capper api start --listen 127.0.0.1:8686 --console /path/to/CapperWeb/dist
```

For anything beyond loopback, terminate TLS — either built in
(`--tls-cert <file> --tls-key <file>`) or behind a proxy — and allow only your
console's origin for credentialed cross-origin requests
(`--allowed-origin https://console.example.com`). Loopback origins are always
allowed. See [Configuration](configuration.md).

## All-in-one dev node

```bash
capper aio init      # create storage layout + local topology
capper aio up        # start API, daemon, and local services
capper aio status
capper aio down
```

Add `--backend capdb` to `aio init` to provision the
[networked CapDB backend](../operator-guide/capdb-backend.md) instead of embedded
SQLite (requires a `-tags capdb` build).

### First-boot networking and storage

AIO deploy (`deploy/deploy.sh`) and `remote-setup.sh` automatically create:

- VPC `default-vpc` with subnet `default` (`10.88.1.0/24`)
- A directory-backed **default storage pool** for instance and volume disks

Before launching instances on a manual install, configure these via
**Network → VPCs** and **Admin → Storage**. See
[Manage VPCs](../operator-guide/manage-networks.md) and
[Admin section](../operator-guide/admin-section.md).

## Join a worker node

On a second machine:

```bash
capper node join my-node --token <join-token> --address 10.0.0.5 --role compute
# then, on the control plane:
capper node approve my-node
```

The `capper-agent` daemon runs on nodes: it sends heartbeats, reports inventory,
pushes host metrics, and supervises services.

## Next

- [Configuration](configuration.md) · [Deploy your first application](../tutorials/deploy-first-app.md)
  · [Concepts](../concepts/architecture.md)
