---
title: "Admin section"
description: "Admin-only platform configuration: routable-IP exclusions, local operators, host limits, host storage, and host security (fail2ban + UFW)."
owner: "docs"
status: "stable"
reviewed: "2026-06-18"
outputs:
  - markdown
  - web
  - pdf
---

# Admin section

The **Admin** area groups admin-only platform configuration. Every Admin endpoint
is gated by the `admin:*` action namespace, which only the built-in admin policy
(`*`) satisfies — non-admin principals receive `403`. In the Web UI the whole
**Admin** nav section is hidden unless the signed-in user is an admin.

## Routable IPs & exclusions

Surfaced under **Admin → Routable IPs** and **Admin → IP Exclusions**. See
[Public IPAM](routable-ips.md) for pools, reservations, and the admin-managed
exclusion list (unlisting addresses so the app stack never auto-allocates them).

## Local Users

**Admin → Local Users** manages platform operator accounts — the users holding
the `admin` role. All other users (members, SSO) are managed under
[IAM](manage-iam.md). Creating an operator here provisions a local
username/password account with the admin role.

## Limits

**Admin → Limits** sets host-wide caps for the control plane. Leaving a field
blank reverts to the built-in default. The host deployment cap (combined user +
system-managed capsules) is persisted here; an admin override takes precedence
over the `CAPPER_MAX_DEPLOYMENTS` environment variable and the RAM-derived
default. Per-account quotas remain under **System → Quotas**.

API: `GET/PUT /api/v1/admin/limits/host`.

## Storage (physical disks & pools)

**Admin → Storage** turns the host's physically-present disks into managed
capacity. Capper discovers disks read-only via `lsblk`; pools are registered with
one of two backends:

- **directory** — capacity is carved as subdirectories under an
  **already-mounted** path. Capper never formats or partitions the disk.
- **lvm** — capacity is carved as ext4 logical volumes from an existing LVM
  **volume group**; each allocation is its own block device.

Volumes and instance disks are carved from a pool as accounted allocations;
over-committing a pool is refused. A background reconciler refreshes each pool's
capacity from its backend and marks a pool **degraded** if its mountpoint or
volume group disappears.

**Auto-drawing instance disks.** A **default instance disk pool** is **required**
for instance launch and block volume creation (Admin → Storage, or
`storage.instance_pool`). New instance disks are drawn from that pool — a
size-capped image on a directory pool, or a logical volume on an LVM pool — and
released when the instance is deleted. AIO deploy registers and pins a default
pool automatically; manual installs must configure one before launching workloads.

Disk states: `unallocated` (no filesystem, mount, or partitions — safe to use),
`pool-member` (backs a registered pool), `in-use-by-host` (mounted or
operator-managed — never auto-touched).

```bash
capper host-storage disks
capper host-storage pool create data --backend directory --mountpoint /mnt/data --device /dev/sdb
capper host-storage pool create vgpool --backend lvm --vg capvg
capper host-storage pool list
```

API: `GET /api/v1/admin/disks`, `GET/POST /api/v1/admin/storage-pools`,
`DELETE …/{id}`, `GET/POST …/{id}/allocations`,
`DELETE /api/v1/admin/storage-allocations/{id}`,
`GET/PUT /api/v1/admin/storage/settings` (default instance pool).

## Host security: fail2ban

**Admin → Fail2ban** manages the host's fail2ban (host OS only). A single
exclusive worker drives `fail2ban-client` through a serialized queue, so Capper
never issues concurrent invocations. The page shows jail status and banned IPs
and supports:

- **Manual ban/unban** of an IP in a jail.
- A **persistent blocklist** — always-on bans that a background reconciler
  re-applies if fail2ban restarts or drops them.
- An **allowlist** (`ignoreip`) — IPs/CIDRs fail2ban will never ban, written to a
  Capper-owned drop-in (`jail.d/capper-allowlist.local`); loopback is always
  included. Operator-authored jails are never modified.

When fail2ban is not installed the page reports it as unavailable rather than
erroring.

API: `GET /api/v1/admin/fail2ban/status`, `POST /api/v1/admin/fail2ban/{ban,unban}`,
`GET/POST /api/v1/admin/fail2ban/blocklist`, `DELETE …/blocklist/{id}`,
`GET/PUT /api/v1/admin/fail2ban/allowlist`.

## Host security: UFW

**Admin → Firewall (UFW)** manages the host's UFW perimeter firewall (host OS
only — distinct from the per-instance [firewalls](firewall.md)). Like fail2ban, a
dedicated exclusive worker serializes all `ufw` invocations. The page supports:

- **Status** and **numbered rules** with add/delete (Capper tags its own rules
  with a comment so they are distinguishable from operator rules).
- **Default policies** for incoming/outgoing/routed traffic.
- A guarded **enable/disable** — enabling prompts a confirmation because it can
  drop existing connections; ensure an allow rule for SSH and the Capper API first.

API: `GET /api/v1/admin/ufw/status`, `POST /api/v1/admin/ufw/rules`,
`DELETE /api/v1/admin/ufw/rules/{num}`, `GET/PUT /api/v1/admin/ufw/defaults`,
`POST /api/v1/admin/ufw/{enable,disable}`.

## Execution model: AIO vs Enterprise

The fail2ban and UFW workers act on the host OS and require root, so they run
where host privilege lives. A process-wide singleton guarantees one exclusive
serialized worker per tool, shared by the admin API and the background
reconciler.

- **AIO (single host):** the control daemon is the host process and runs the
  workers in-process. Everything on this page operates on that host.
- **Enterprise (multi-node):** each node's `capper-agent` runs the same workers
  and serves host security for its own host on the agent API
  (`/hostsec/...`), including the exclusive-worker guarantee. The control plane's
  host-security API is node-aware: an optional `?node=<id>` selector targets a
  node; the control node executes in-process, and a request for a different node
  is reported as managed by that node's agent (`GET /api/v1/admin/hostsec/nodes`
  lists targetable nodes and flags the local one). Per-node host security is owned
  by that node's agent; central cross-node proxying is intentionally not exposed
  over the network.
