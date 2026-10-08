---
title: "API reference — instances"
description: "Endpoints for listing, creating, and operating instances."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# API reference — instances

All paths are under `/api/v1` and require [authentication](overview.md).

## Read and lifecycle

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/instances` | list instances |
| `POST` | `/instances` | launch an instance (**requires `subnetId`**) |
| `GET` | `/instances/{id}` | inspect an instance |
| `PATCH` | `/instances/{id}` | update limits, labels, restart policy |
| `DELETE` | `/instances/{id}` | remove an instance |
| `GET` | `/instances/{id}/logs` | combined logs |
| `GET` | `/instances/{id}/events` | lifecycle events |
| `GET` | `/instances/{id}/metadata` | capinit user-data metadata |
| `GET` | `/instances/{id}/monitoring` | metrics |
| `GET` | `/instances/{id}/terminal` | interactive terminal (WebSocket) |
| `GET` | `/instance-disk-capacity` | pool capacity for new instance disks |

## Create request

```json
{
  "image": "alpine",
  "name": "web-1",
  "subnetId": "sub_…",
  "vpcId": "vpc_…",
  "securityGroupIds": ["sg_…"],
  "instanceType": "cap-micro",
  "diskBytes": 10737418240,
  "runtimeMode": "lxc",
  "labels": { "tier": "web" }
}
```

`runtimeMode` (optional): `auto`, `bwrap`, `chroot`, `crun`, `runc`, `lxc`, or `qemu`.
When omitted, Capper uses the capsule `preferredRuntime` (if set) else the host `--runtime` default.

**Requirements:**

- `subnetId` — mandatory; instance attaches via primary ENI in that subnet.
- **Default storage pool** — must be configured under Admin → Storage; disk size is validated against pool capacity.
- Legacy `network` field — rejected with `400`.

Compute groups expose `GET /groups/{name}/instances`. Placement and autoscale have
their own route groups.

Responses use the [standard envelope](overview.md#response-envelope).

## Related

- [API overview](overview.md) · [Manage instances](../../operator-guide/manage-instances.md)
  · [Manage VPCs](../../operator-guide/manage-networks.md)
  · [Go SDK](../sdk/go.md)
