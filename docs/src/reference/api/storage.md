---
title: "API reference — storage"
description: "Host pools, volumes, buckets, CSD, and the S3 server."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# API reference — storage

All control-plane paths are under `/api/v1` and require
[authentication](overview.md).

## Admin — host storage pools

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/admin/disks` | discovered host disks |
| `GET` | `/admin/storage-pools` | list pools |
| `POST` | `/admin/storage-pools` | register a pool |
| `DELETE` | `/admin/storage-pools/{id}` | delete empty pool |
| `GET/POST` | `/admin/storage-pools/{id}/allocations` | list/create allocations |
| `GET/PUT` | `/admin/storage/settings` | get/set `defaultInstancePool` |

Requires `admin:storage:*` permissions.

## Workload storage

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/storage/volumes` | list block volumes |
| `POST` | `/storage/volumes` | create volume (**requires default pool**) |
| `POST` | `/storage/volumes/{name}/attach` | attach to instance |
| `DELETE` | `/storage/volumes/{name}` | delete a volume |
| `GET` | `/storage/buckets` | list buckets |
| `POST` | `/storage/buckets` | create bucket (no pool required) |
| `GET` | `/csd/volumes` | list CSD shared volumes |

`POST /instances` also requires the default pool when the instance type allocates disk.

## S3 data plane

The S3-compatible object API is served separately (standard S3 verbs), authenticated
with S3 credentials. Create credentials with `capper storage bucket credentials`.

The [Go SDK](../sdk/go.md) `Storage` and `CSD` groups wrap control-plane endpoints.

## Related

- [API overview](overview.md) · [Manage storage](../../operator-guide/manage-storage.md)
  · [Storage model](../../concepts/storage-model.md) · [Admin section](../../operator-guide/admin-section.md)
