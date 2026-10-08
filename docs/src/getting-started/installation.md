---
title: "Install and build Capper"
description: "Prerequisites, building the binaries, and the optional CapDB backend."
owner: "docs"
status: "stable"
reviewed: "2026-06-12"
outputs:
  - markdown
  - web
  - pdf
---

# Install and build Capper

Capper is distributed as source and built with the Go toolchain. There are three
binaries: `capper` (CLI + API + control plane), `capper-agent` (the node daemon),
and `capinit` (the capsule init/PID-1 helper).

## Prerequisites

- **Go 1.25+** (see `go.mod`).
- A Linux host. Capsule execution prefers **Bubblewrap** (`bwrap`) with
  unprivileged user namespaces; it can fall back to `chroot` (often needs `sudo`),
  or use `crun`/`runc` if installed.
- For the optional networked database backend: a **C toolchain + OpenSSL** and
  **CMake** (only needed for `-tags capdb`).
- For the Web UI: **Node.js** (to build CapperWeb).

Run the host capability checker any time:

```bash
capper host doctor
```

## Build the default (pure-Go) binaries

From the repository root:

```bash
make build        # writes bin/capper
make test         # go test ./...
make clean        # remove build output
```

`make build` produces a pure-Go binary with the embedded SQLite backend — no cgo,
no external services. This is the right build for laptops, CI, and single-node
installs.

To build everything (`capper`, `capper-agent`, `capinit`):

```bash
go build ./...
```

## Build with the CapDB networked backend (optional)

The [CapDB backend](../operator-guide/capdb-backend.md) is opt-in and links a cgo
client library. Build it only if you need a networked, connection-pooled database
shared by multiple processes:

```bash
make capdb-fetch   # clone CapDB 3.7.2 into ./CapDB
make build-capdb   # build the engine, then `go build -tags capdb`
```

The default backend remains pure-Go SQLite unless you both build with `-tags capdb`
and select it via `CAPPER_DB_DRIVER=capdb` (or `aio init --backend capdb`).

## Install the binaries

Copy the built binaries onto your `PATH`:

```bash
sudo install -m 0755 bin/capper /usr/local/bin/capper
# node hosts also need the agent:
go build -o /usr/local/bin/capper-agent ./cmd/capper-agent
```

## Verify

```bash
capper --help            # top-level command groups
capper status            # daemon + subsystem status (once a control plane runs)
capper host doctor       # host capability report
```

## Next

- [Quickstart](quickstart.md) — run a capsule and bring up the control plane.
- [Configuration](configuration.md) — env vars, flags, and config files.
