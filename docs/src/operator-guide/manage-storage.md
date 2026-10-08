---
title: "Manage storage"
description: "Host storage pools, block volumes, S3 buckets, snapshots, and CSD."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Manage storage

`capper storage` manages block volumes, buckets/objects, snapshots, and CSD shares.
For the conceptual model see the [Storage model](../concepts/storage-model.md).

## Prerequisites: host storage pool

**Instance root disks and block volumes require a default storage pool.**

1. Open **Admin → Storage**.
2. Register a pool (`directory` on a mounted path, or `lvm` on an existing volume group).
3. Set **Default instance pool** (persists as `storage.instance_pool`).

![Admin storage](/assets/images/screenshots/31-admin-storage.png)

Until a default pool is set, the Web UI blocks volume creation and instance launch,
and the API returns `400` for `POST /instances` and `POST /storage/volumes`.

```bash
capper host-storage disks
capper host-storage pool create data --backend directory --mountpoint /mnt/data
capper host-storage pool list
# set default via API:
curl -X PUT /api/v1/admin/storage/settings \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"defaultInstancePool":"<pool-id>"}'
```

See [Admin section — Storage](admin-section.md#storage-physical-disks--pools) for backends, disk states, and allocation APIs.

## Block volumes

```bash
capper storage volume create my-vol --size 20G --class local
capper storage volume list
capper storage volume inspect my-vol
capper storage volume attach my-vol --instance <id>
capper storage volume detach my-vol --instance <id>
capper storage volume delete my-vol
```

| Flag (`volume create`) | Purpose |
| --- | --- |
| `--size <size>` | size hint, e.g. `20G` (validated against pool capacity) |
| `--class <class>` | volume class (default `local`) |

![Storage dashboard](/assets/images/screenshots/05-storage.png)

Detach before deleting; snapshot before destructive operations.

## Snapshots

```bash
capper storage snapshot create my-vol
capper storage snapshot list
# restore a snapshot to a new volume, then attach it
```

## Object store (S3-compatible)

Buckets do not require a host pool:

```bash
capper storage bucket create my-bucket
capper storage bucket credentials create my-bucket   # S3 access/secret keys
capper storage object put my-bucket ./file.txt
capper storage object list my-bucket
capper storage s3 ...                                 # S3 server admin
```

## Shared/replicated volumes (CSD)

`capper storage share` manages CSD volumes that multiple nodes can mount (realized
on the node via FUSE) for clustered workloads.

## Backups

Protect data with [backup policies](manage-backups.md). For the control-plane
**database**, back it up separately — see the
[CapDB backup section](capdb-backend.md#operations).

## Related

- [Storage model](../concepts/storage-model.md) · [Admin section](admin-section.md)
  · [Manage backups](manage-backups.md) · [Manage instances](manage-instances.md)
