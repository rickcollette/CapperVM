# Capper — Seamless Upgrade System Plan

Implementation status of the upgrade system for **AIO** (single host) and
**Enterprise** (multi-node). Most of the plan is implemented; remaining items are
larger infra/feature work and are marked below.

**Status (2026-06-18):** `go build`, `go vet`, `go test ./...` green. New surface:
`capper version`, `capper schema status|backup`, `capper aio upgrade|check-update`,
`capper fleet status|upgrade`, build-stamped binaries, channel feed, versioned AIO
layout with auto-rollback.

---

## Phase 0 — Versioning & compatibility foundation

- [x] **Build-stamp the version.** New [internal/version](internal/version/version.go)
  package (`Version`/`Commit`/`BuildDate`, `APIVersion`, `MinAgentVersion`), stamped
  via `-ldflags -X` from the `VERSION` file in the [Makefile](Makefile) and
  [build-aio.sh](scripts/build-aio.sh). `api.Version` now derives from it.
- [x] **`capper version`** (text + `--json`: version, commit, date, Go, platform, API).
- [x] **Enriched `/api/v1/version`** — returns version, commit, buildDate, goVersion,
  apiVersion, platform, **schemaVersion**, minAgentVersion
  ([handlers_health.go](internal/api/handlers_health.go)).
- [x] **Agent reports version in heartbeat + join**; control plane persists it to
  `node.AgentVersion` ([agent.go](internal/agent/agent.go),
  [handlers_topology.go](internal/api/handlers_topology.go)).
- [x] **Compatibility contract** encoded (`APIVersion`, `MinAgentVersion`, N‑1 policy)
  and documented in [upgrades.md](docs/src/operator-guide/upgrades.md).
  - [ ] **Remaining:** enforce a hard skew interlock (reject contract migrations while
    N‑1 nodes are live) — see Phase 3.

## Phase 1 — Migration & data safety

- [x] **Backup-before-migrate / snapshots.** `Store.SnapshotDB` (online `VACUUM INTO`,
  works for sqlite + CapDB) + `SnapshotPath` ([store/migrations.go](internal/store/migrations.go));
  `capper schema backup` and the AIO upgrade take a snapshot before migrating.
- [x] **Migration status.** `capper schema status` shows applied + pending migrations
  and the schema version; `Store.AppliedMigrations/PendingMigrations/SchemaVersion`.
- [x] **Expand/contract policy** documented as the rule for breaking changes.
  - [ ] **Remaining:** finish converting the inline additive `ALTER` block in
    [store/db.go](internal/store/db.go) into numbered registered migrations (currently
    one named migration + the additive block; `knownMigrations` is the registry seam).

## Phase 2 — AIO upgrade

- [x] **`capper aio upgrade`** ([aio_upgrade.go](internal/cli/aio_upgrade.go)):
  resolve bundle (`--bundle`/`--url`/`--channel`) → verify SHA‑256 → stage into a
  versioned dir → stop services → DB snapshot → flip `current` symlink → start (boot
  migrations) → **health-gate `/api/v1/health` with auto-rollback** of binaries + DB.
- [x] **Rollback** — `capper aio upgrade --rollback` (symlink flip to previous staged
  version) and automatic rollback on a failed health gate.
- [x] **Versioned layout** `/usr/local/lib/capper/<version>/` + `current` symlink +
  `/usr/local/bin/*` symlinks (atomic swap).
- [x] **Upgrade-aware installer** ([aio-install.sh](scripts/aio-install.sh)) — detects
  an existing install and stages a new version + flips, instead of overwriting.
- [x] **Console upgrades atomically** with the control plane (served from
  `current/console`).

## Phase 3 — Enterprise rolling upgrade

- [x] **`capper fleet upgrade`** ([fleet_upgrade.go](internal/cli/fleet_upgrade.go)):
  per-node cordon → drain → `--exec` swap (e.g. ssh) → wait for the node to re-report
  the target version → uncordon; `--batch N`, control-plane health gate between
  batches, `--dry-run`, stop-on-first-failure.
- [x] **`capper fleet status`** — per-node version + drain state (skew visibility).
- [x] **Order documented** — control plane first (maintenance window until HA), then
  rolling agents; CapDB sequencing + RPO/RTO in the docs.
  - [ ] **Remaining (feature work):** an agent **self-upgrade** channel so `--exec` ssh
    isn't required; the contract-migration **skew interlock**; surfacing per-node
    upgrade status in the console.

## Phase 4 — Distribution, packaging & rollback

- [x] **Release artifacts + feed.** [build-aio.sh](scripts/build-aio.sh) emits the
  versioned `.tgz` + `.sha256` and updates a `channels.json` feed entry
  (version/url/sha256/minUpgradeFrom).
- [x] **Verify before apply.** SHA‑256 verification + `minUpgradeFrom` downgrade guard
  (`compareVersions`) in the upgrade path.
  - [ ] **Remaining:** detached **signature** verification (wire the existing signing
    infra into bundle verification — currently checksum-only).
- [x] **First-class rollback** (Phase 2).
  - [ ] **Remaining:** version/backup **retention GC** (keep last N).

## Phase 5 — Safety, UX & observability

- [x] **Update-available check** — `capper aio check-update --channel` compares the
  running version to the feed.
- [x] **Docs** — operator [Upgrades guide](docs/src/operator-guide/upgrades.md) (AIO +
  enterprise runbooks), added to `nav.yml`.
- [x] **Tests** — unit tests for the upgrade helpers (tar extraction incl.
  path-traversal rejection, checksum verify, semver compare) in
  [aio_upgrade_test.go](internal/cli/aio_upgrade_test.go).
- [x] **CapperWeb version stamping** — `VITE_CAPPER_VERSION` → `consoleVersion`
  ([features.ts](../CapperWeb/src/lib/features.ts)).
  - [ ] **Remaining:** pre-flight checks integrated into `aio doctor` (disk-for-backup,
    schema diff preview); structured **upgrade audit events** + `upgrade history`; a
    console "update available" banner; a full vN→vN+1 integration test on a temp
    install (current tests cover the helpers, not a live systemd upgrade).

---

## Summary of what remains (all non-blocking, larger feature work)

1. Convert the residual additive migration block into numbered migrations.
2. Skew interlock for contract migrations; agent self-upgrade channel.
3. Detached signature verification + version/backup retention GC.
4. `aio doctor` pre-flight integration, upgrade audit events/history, console banner,
   and a live end-to-end upgrade integration test.

The core spine (versioning, migration safety, AIO upgrade with auto-rollback,
enterprise rolling driver, distribution feed) is implemented and tested.
