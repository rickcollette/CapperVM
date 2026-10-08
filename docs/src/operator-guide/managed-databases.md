---
title: "Managed databases"
description: "Provision and operate managed database services."
owner: "docs"
status: "stable"
reviewed: "2026-06-12"
outputs:
  - markdown
  - web
  - pdf
---

# Managed databases

`capper db` provisions and operates **managed database services** for your
workloads — distinct from the [CapDB backend](capdb-backend.md), which is the
control plane's *own* state store.

## Lifecycle

```bash
capper db create my-db --engine postgres --version 16 --subnet-id <subnet-id>
capper db list
capper db inspect my-db
capper db restore my-db <backup>
capper db delete my-db
```

| Flag (`db create`) | Purpose |
| --- | --- |
| `--engine <engine>` | `postgres`, `redis`, or `mariadb` (required) |
| `--version <ver>` | engine version (optional) |
| `--port <n>` | database port (optional) |
| `--subnet-id <id>` | VPC subnet to place the database in (**required**) |
| `--vpc-id <id>` | VPC ID (optional; must match the subnet) |

## Backups and restore

Managed databases integrate with the [backup](manage-backups.md) subsystem for
scheduled backups and point-in-time restore (`capper db restore`). Test restores
regularly.

## Managed databases vs the CapDB backend

- **Managed databases (`capper db`)** — databases you run *for applications*.
- **[CapDB backend](capdb-backend.md)** — the optional networked store for
  *Capper's own* control-plane state.

Don't conflate the two; they are backed up and operated separately.

## Related

- [Manage backups](manage-backups.md) · [CapDB backend](capdb-backend.md)
