# NEXTUP — Admin section roadmap

> **Status (2026-06-18): all four features implemented + the deferred follow-ups completed.**
> - [x] **#1 Storage** — `internal/hoststorage` (disk discovery, pools, allocations); admin API; CLI `capper host-storage`; Admin → Storage.
> - [x] **#2 Limits** — `internal/adminconfig` KV; persisted host deploy cap; admin API; Admin → Limits.
> - [x] **#3 Fail2ban** — `internal/hostsec/fail2ban` exclusive worker; admin API; Admin → Fail2ban.
> - [x] **#4 UFW** — `internal/hostsec/ufw` exclusive worker; admin API; Admin → Firewall (UFW).
>
> Follow-ups now done:
> - [x] **LVM/thin-pool backend** — pools have a `directory` or `lvm` backend; LVM allocations are ext4 logical volumes (`lvcreate`/`lvremove`).
> - [x] **Auto-draw instance disks from pools** — set a default instance pool; instance disks are carved from it (directory image or LVM LV) and released on delete (`internal/manager/instance_manager.go`, `internal/diskquota`).
> - [x] **Storage reconciler** — `hostStorageReconciler` refreshes pool capacity and marks degraded pools.
> - [x] **fail2ban persistent blocklist reconcile + allowlist** — blocklist table + `fail2banReconciler`; ignoreip drop-in.
> - [x] **UFW default-policy editor** (allowlist = allow-from rules) — `GET/PUT /api/v1/admin/ufw/defaults`.
> - [x] **AIO/Enterprise execution** — process-wide exclusive-worker singleton (`internal/hostsec/provider`); per-node `capper-agent` runs + serves the same workers (`/hostsec/...`); node-aware control API (`?node=`, `GET /api/v1/admin/hostsec/nodes`).
>
> Docs: `docs/src/operator-guide/admin-section.md`.

Follow-on work for the admin-only **Admin** area (started with Routable IPs, IP
Exclusions, and Local Users). Four features, each admin-gated the same way the
exclusions work was: API actions in the `admin:*` namespace (only the `admin-all`
`*` policy satisfies them) and a CapperWeb nav section hidden unless `me.isAdmin`.

Existing anchors this builds on:

- Admin nav + gating: [`AppShell.tsx`](../CapperWeb/src/components/layout/AppShell.tsx) (`adminOnly` sections, `visibleSections`).
- Reconciler loop (in-process background workers): [`internal/control/reconciler.go`](internal/control/reconciler.go), registered in [`internal/control/daemon.go`](internal/control/daemon.go).
- Privileged host agent (per-node, runs as root): [`internal/agent/agent.go`](internal/agent/agent.go), local API socket at `/run/capper-agent.sock` ([`internal/agent/localapi.go`](internal/agent/localapi.go)).
- Host command exec precedent: [`internal/firewall/nftables.go`](internal/firewall/nftables.go), [`internal/network/iptfilter.go`](internal/network/iptfilter.go) shell out to `nft`/`iptables`.
- Volumes / disk: [`internal/storage`](internal/storage/) (directory-backed volumes), [`internal/diskquota/overlay.go`](internal/diskquota/overlay.go) (per-instance overlay size caps).
- Limits today: [`internal/quotas`](internal/quotas/) (per-account, persisted, `GET/POST /api/v1/quotas`) and [`internal/deploylimit`](internal/deploylimit/) (host-wide cap, env-driven).

A cross-cutting decision applies to features **3** and **4**: they manage the
**host OS**, so they require root and must run where Capper already has host
privileges. See "Host-privileged worker model" below.

---

## 1. Underlying storage manager (physical disk pool)

**Goal.** Discover disks/partitions physically present on the host but not yet
allocated, register them as a managed capacity pool, and let additional instance
disks and volumes draw from real backing storage instead of only the control
plane's directory tree.

**Why.** Today [`storage.Volume`](internal/storage/types.go) is directory-backed
(`VolumeBackendDirectory`) under the store path, and instance extra disk is an
overlay size cap ([`diskquota`](internal/diskquota/overlay.go)) — there is no
notion of "this 2 TB NVMe is unused; carve instance disks from it."

**Backend (new `internal/hoststorage`).**
- `Disk` model: device path (`/dev/sdb`), size, rotational/SSD, model/serial,
  filesystem (if any), mountpoint (if any), and a derived `state`
  (`unallocated`, `pool-member`, `in-use-by-host`, `excluded`).
- Discovery: enumerate via `lsblk -J -b -O` (JSON) + `/proc/mounts`; classify a
  device as `unallocated` only when it has no mounted filesystem, no partitions
  in use, and is not the root/boot disk. **Never** auto-touch a disk the host is
  using — mirror the "never silently pull a live resource" rule from IP exclusions.
- `StoragePool`: a named pool over one or more `unallocated` disks. Creating a
  pool is an explicit admin action (it may format/partition), so it is gated and
  requires the operator to name the exact device(s).
- Allocation backend: extend [`storage/manager.go`](internal/storage/manager.go)
  with a new volume backend (`VolumeBackendPool`) that carves a volume from a
  pool. Start with a directory-on-mounted-disk backend (simplest, reuses the
  existing bind-mount attach path); leave LVM/thin-pool as a later backend behind
  the same interface.
- Capacity accounting: pool `totalBytes`/`allocatedBytes`/`availableBytes`;
  refuse allocations that would over-commit. Wire instance "additional disk" and
  `diskquota` overlay sizing to the chosen pool so the cap reflects real free space.

**API (admin).** `internal/api/handlers_hoststorage.go`:
- `GET /api/v1/admin/disks` — discovered disks + state.
- `GET/POST /api/v1/admin/storage-pools`, `DELETE …/{id}`.
- `POST /api/v1/admin/storage-pools/{id}/disks` — add/remove member disks.
Authorize with `admin:storage:*` / resource `admin:system`.

**Worker.** Register a `hostStorageReconciler` (Reconciler interface) to refresh
disk inventory and recompute pool usage on the existing loop — read-only
discovery only; all formatting/allocation stays request-driven.

**UI.** Admin → **Storage** ([`src/pages/admin/HostStorage.tsx`]): disk inventory
table (state badges), pool create/manage, per-pool usage bars. Disk format/pool
creation behind a typed-device-name confirm dialog.

**Open questions.** LVM vs partition-per-volume vs directory-on-mount as the
first real backend; whether pools are host-scoped only (AIO) or node-scoped
(topology profile); encryption (tie into existing KMS volume encryption).

---

## 2. Configurable limits (instances, databases, …)

**Goal.** One admin surface to view and set the caps that today are split across
[`quotas`](internal/quotas/) (per-account, persisted) and
[`deploylimit`](internal/deploylimit/) (host-wide, env-only) — plus new
per-resource limits (e.g. databases, per-engine, per-instance-type counts).

**Backend.**
- Promote the host deploy cap out of env-only: add a persisted host-limits row
  (reuse the quotas store with a synthetic `host` scope, or a small
  `internal/limits` store) so `deploylimit.MaxDeployments()` reads config first,
  env second, derived default last — keeping current behavior as the fallback.
- Add new limit keys to [`quotas/models.go`](internal/quotas/models.go):
  `database.count.max`, `database.<engine>.count.max`, `volume.count.max`,
  `volume.bytes.max`, `instance.<type>.count.max`. Enforce in the existing
  admission/quota checker path ([`quotas/checker.go`](internal/quotas/checker.go)).
- Surface current usage alongside each limit (the checker already tracks usage).

**API (admin).** Reuse `GET/POST /api/v1/quotas`; add
`GET/PUT /api/v1/admin/limits/host` for the host-wide caps. Admin-gate the host
limits endpoints (`admin:limits:*`); keep account quotas under existing IAM gating.

**UI.** Admin → **Limits**: grouped editable table (Compute / Storage /
Databases / Network) showing limit + current usage + default, with inline edit.
This replaces the read-mostly System → Quotas page for admins (or links to it).

**Open questions.** Scope model — host vs account vs org precedence; whether
lowering a limit below current usage is allowed (warn, don't retroactively evict).

---

## 3. Fail2ban management (host OS)

**Goal.** Admin UI over the host's fail2ban: jail status, currently-banned IPs,
manual ban/unban, and whitelist/blacklist. **Host OS only** — not per-instance.

**Worker (dedicated, exclusive).** Per the requirement, a single Go worker owns
fail2ban exclusively. New `internal/hostsec/fail2ban`:
- A long-lived worker goroutine serializing **all** access through a command
  queue (channel) so concurrent admin requests never race `fail2ban-client`.
- Wraps `fail2ban-client`: `status`, `status <jail>`, `set <jail> banip <ip>`,
  `set <jail> unbanip <ip>`, and reads `banned` lists. Whitelist = manage
  `ignoreip` (via a Capper-owned drop-in in `jail.d/` + reload); blacklist =
  persistent manual bans Capper re-applies on start (it reconciles desired vs
  actual bans, like other reconcilers).
- Capper-owned config drop-in only (e.g. `/etc/fail2ban/jail.d/capper.local`);
  never rewrite operator-authored jails.
- Degrades cleanly when fail2ban is absent: worker reports `unavailable`, UI
  shows "not installed" rather than erroring.

**API (admin).** `internal/api/handlers_hostsec.go`:
- `GET /api/v1/admin/fail2ban/status` (jails + counts), `GET …/jails/{jail}`.
- `GET /api/v1/admin/fail2ban/banned`.
- `POST …/ban`, `POST …/unban` (`{jail, ip}`).
- `GET/POST/DELETE …/allowlist` (ignoreip), `…/blocklist` (persistent bans).
Authorize `admin:hostsec:fail2ban:*` / resource `admin:system`.

**UI.** Admin → **Security → Fail2ban**: jail status cards, banned-IP table with
unban, manual ban form, allowlist/blocklist editors. Reuse the IP-input + table
patterns from the IP Exclusions page.

---

## 4. UFW management (host OS)

**Goal.** Admin UI over the host's UFW: status, rule list, add/delete
allow/deny rules, enable/disable. **Host OS only** — distinct from per-instance
nftables firewalls in [`internal/firewall`](internal/firewall/).

**Worker (dedicated, exclusive).** A single Go worker owns UFW exclusively
(mirrors the fail2ban worker; the requirement's "handles Fail2Ban" on this item
is a typo — this worker handles UFW). New `internal/hostsec/ufw`:
- Worker goroutine serializing all `ufw` invocations through a command queue.
- Wraps `ufw status numbered`/`verbose` (parse to structured rules), `ufw allow
  …`, `ufw deny …`, `ufw delete <num>`, `ufw enable`/`disable`,
  `ufw default …`.
- **Guardrails:** enabling UFW or changing default-incoming can lock out the
  admin/SSH/Capper API. Require an explicit confirm, and before enabling,
  auto-ensure allow rules for the SSH port and the Capper control-plane port.
- Reconcile Capper-managed rules (tagged via `comment`) without clobbering
  operator-authored rules; degrade cleanly when UFW is absent.

**API (admin).** Same `handlers_hostsec.go`:
- `GET /api/v1/admin/ufw/status`, `GET …/rules`.
- `POST …/rules` (`{action, direction, port, proto, from}`), `DELETE …/rules/{num}`.
- `POST …/enable`, `POST …/disable`, `POST …/default`.
Authorize `admin:hostsec:ufw:*` / resource `admin:system`.

**UI.** Admin → **Security → Firewall (UFW)**: status toggle (guarded), numbered
rules table with delete, add-rule form, default-policy controls.

---

## Host-privileged worker model (applies to 3 & 4, and disk format in 1)

These touch the host OS and need root. Two deployment shapes:

- **AIO / single host (primary target):** the control daemon already runs
  privileged; register the fail2ban and UFW workers in-process alongside the
  reconcilers in [`daemon.go`](internal/control/daemon.go). Admin API handlers
  talk to the workers directly via their command queues. Simplest; ship this first.
- **Multi-node (topology profile):** host-OS firewall/fail2ban is per-node, so
  the workers belong in [`capper-agent`](internal/agent/agent.go) and are exposed
  on its local API socket; the control plane proxies `admin:hostsec:*` calls to
  the selected node's agent. Defer until needed.

Shared worker conventions:
- One worker = exclusive owner of one host tool; every mutation goes through a
  serialized command queue (no concurrent CLI invocations).
- All workers expose a `status`/availability probe so the UI can show
  "not installed / not permitted" instead of failing.
- Capper only ever edits its **own** config drop-ins and **tagged** rules; never
  rewrite operator-authored config. Reconcile desired-vs-actual idempotently.
- Audit every mutating action through the existing audit log.

## Suggested sequencing

1. **Limits (#2)** — smallest, mostly extends `quotas`/`deploylimit`; no host root.
2. **Underlying storage (#1)** — read-only disk discovery first, then pool
   allocation; highest design surface.
3. **Fail2ban (#3)** then **UFW (#4)** — share `internal/hostsec` + the
   host-privileged worker model; build the worker scaffolding once, reuse twice.

## Out of scope (for now)

- Cloud/remote block storage backends (this is local physical disks only).
- Replacing per-instance nftables firewalls (`internal/firewall`) — UFW here is
  strictly the host's own perimeter.
- Multi-node agent proxying for #3/#4 beyond the interface seam (AIO first).
