---
title: "Repository layout"
description: "Where the binaries, subsystems, SDK, and docs live."
owner: "docs"
status: "stable"
reviewed: "2026-06-12"
outputs:
  - markdown
  - web
  - pdf
---

# Repository layout

```text
cmd/            entrypoints: capper, capper-agent, capinit
internal/       all subsystems (one package per area) + the control plane
  api/            REST API server, handlers, middleware, auth
  controller/     wraps the store + managers for handlers/CLI
  control/        the control-plane daemon, reconcilers, supervisor
  store/          the single database + ~45 sub-stores, migrations, paths
  iam/ authz/     identity, tokens, deny-by-default authorization
  org/            organizations, accounts, projects, memberships
  compute/ runtime/ oci/ loader/   capsule build + execution
  network/ vpc/ firewall/ lb/ ingress/ dns/ ipam/   networking
  storage/ s3server/ csd/ backup/   storage & data
  kms/ secret/ sign/ posture/ sbom/ scanner/ marketplace/   security supply chain
  topology/ host/ agent/ supervisor/   nodes & placement
  functions/ mcpserver/ ai/ queue/ eventing/   serverless & messaging
  observability/ resourcemon/ metrics/ alert/ health/ audit/   observability
  capdbdriver/    the cgo CapDB client driver (-tags capdb)
  cli/            cobra command tree (all `capper` commands)
sdk/go/         the Go SDK client (one group per subsystem)
CapDB/          gitignored checkout of CapDB (make capdb-fetch, CAPDB_REF)
docs/           this documentation (src/ authored, dist/ + generated/ built)
tools/docgen/   the documentation generator (check/inventory/markdown/web/pdf)
examples/ testdata/ schemas/   samples and fixtures
```

## The subsystem pattern

Most `internal/<subsystem>` packages follow the same shape:

- a **store** (`store.go`) — typed CRUD over a table in the shared database;
- a **manager** (`manager.go`) — business logic and validation;
- **types** (`types.go`/`*.go`) — the domain structs;
- exposed through `internal/api` handlers, the `internal/cli` cobra tree, and a
  `sdk/go` group.

This consistency is what lets every subsystem appear identically across CLI, API,
SDK, and the Web UI.

## Binaries

| Binary | Package | Role |
| --- | --- | --- |
| `capper` | `cmd/capper` | CLI + API + control plane |
| `capper-agent` | `cmd/capper-agent` | node daemon (heartbeat, inventory, supervise) |
| `capinit` | `cmd/capinit` | in-capsule PID 1 |

## Related

- [Adding a module](adding-a-module.md) · [Build and test](build-and-test.md)
  · [Architecture](../concepts/architecture.md)
