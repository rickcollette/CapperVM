---
title: "Storage model"
description: "Host storage pools, block volumes, object store, snapshots, CSD, and backups."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Storage model

Capper separates **host storage pools** (physical capacity on the node) from
**logical volumes** (tenant-facing disks) and **object storage**.

## Host storage pools (required)

Before creating instances or block volumes, an admin must register at least one
**storage pool** and set it as the **default instance pool**:

1. **Admin → Storage** — discover disks, register a pool (`directory` or `lvm` backend).
2. Set **default instance pool** — all new instance root disks and block volumes are carved from this pool.

![Admin storage pools](/assets/images/screenshots/31-admin-storage.png)

AIO deploy (`remote-setup.sh`) registers a directory pool under the store path and
pins it as the default automatically.

Pools track **available bytes**; over-committing is refused. Instance launches
validate the instance type's disk size against pool capacity.

## Block volumes

```bash
capper storage volume create my-vol --size 20G
capper storage volume attach my-vol --instance <id>
capper storage snapshot create my-vol
```

Volumes require the default pool to be configured. See
[Manage storage](../operator-guide/manage-storage.md).

## Object store (S3-compatible)

Buckets and objects do **not** use host pools — they live under the control-plane
object store path:

```bash
capper storage bucket create my-bucket
capper storage object put my-bucket ./file.txt
```

## CSD shared volumes

CSD provides shared/replicated volumes mountable across nodes (FUSE on the node).
The control plane runs a CSD server and tracks attachments.

## Backups

`capper backup` manages backups and **backup policies** (scheduled, retained).
Backups cover platform resources; for the control-plane **database**, see
[CapDB backup](../operator-guide/capdb-backend.md#operations).

## Control-plane state vs workload storage

**Workload storage** (pooled disks, buckets, CSD) is what instances use.
**Control-plane state** is the database recording platform metadata — backed up
separately. See [Architecture](architecture.md) and [CapDB backend](../operator-guide/capdb-backend.md).

## Related

- [Manage storage](../operator-guide/manage-storage.md)
  · [Admin section](../operator-guide/admin-section.md#storage-physical-disks--pools)
  · [Manage backups](../operator-guide/manage-backups.md)
  · [CapDB backend](../operator-guide/capdb-backend.md)
