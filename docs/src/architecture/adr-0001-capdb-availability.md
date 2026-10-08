---
title: "ADR 0001: CapDB availability posture"
description: "The chosen availability/SLA posture for the CapDB control-plane backend, and the HA roadmap."
owner: "docs"
status: "stable"
reviewed: "2026-10-08"
outputs:
  - markdown
  - web
  - pdf
---

# ADR 0001: CapDB availability posture

- **Status:** Accepted (2026-06-16)
- **Context:** Closes the `R3` gap from the CapDB production review — the
  availability posture must be a written decision, not an implied one.

## Context

When the optional [CapDB backend](../operator-guide/capdb-backend.md) is selected,
`capdb-server` becomes a hard dependency for the entire control plane: it holds the
single database every control-plane process reads and writes. Today it is one
server — a single point of failure, like the embedded SQLite it replaces, but now
reachable over the network by multiple processes. There is no built-in
replication or automatic failover.

We need an explicit availability target and a roadmap, rather than discovering the
SLA during an incident.

## Options considered

1. **Single node, fast recovery (now).** One co-located `capdb-server` with
   `Restart=on-failure`, relying on the implemented resilience work (`R1`
   self-healing pool eviction, `R2` connect/handshake timeout + startup retry) so
   a restart is a multi-second blip, not a 5-minute outage. Control-plane
   availability is bounded by single-node DB availability.
2. **Active/standby.** Primary + warm standby over shared/replicated storage
   (CapDB RBU / remote-VFS primitives), a health-checked VIP failover, and
   client-side retry. Defines an RPO/RTO; meaningfully more moving parts.
3. **Read scaling.** Read-only replicas for list-heavy endpoints (the SQLite
   dialect + WAL already allow concurrent readers), orthogonal to write HA.

## Decision

Adopt **Option 1** as the supported posture for the current milestone:

- Co-locate `capdb-server` with the control plane (loopback or same host/cluster).
- Run it under systemd with `Restart=on-failure` and TLS by default.
- Depend on `R1`/`R2` (implemented) so a DB restart is a brief, self-healing blip.
- **Stated SLA:** control-plane availability is bounded by single-node CapDB
  availability. There is no automatic failover; recovery time on host loss is the
  time to restore the backup onto a new host (see Backups below).

Options 2 and 3 are deferred to a future milestone. They are not required for the
opt-in backend that exists today; they are the path from "safe to depend on,
single node" to "highly available."

## Consequences

- **Positive:** simple to operate and reason about; the resilience work already
  makes routine restarts non-events; no new infrastructure.
- **Negative:** host loss is an outage until restore; no zero-RPO guarantee
  (default `synchronous=NORMAL` can lose the last commit on power loss — set
  `CAPPER_DB_SYNCHRONOUS=FULL` where that matters).
- **Mitigations:** automated, tested backups with a defined RPO/RTO; co-location to
  shrink the failure surface; monitor pool saturation via `GET /api/v1/db/stats`.

## Follow-ups

- Automated online `.backup` (command + systemd timer) with a tested restore.
  The AIO bundle ships the `capdb` shell for this.
- Server-side auth/connection audit log (`S3`).
- CapDB 3.7.2 includes a volume store and WAL replication. Capper still does
  not configure that mode. Active/standby stays an operator procedure until
  the SLA target tightens.

## Related

- [CapDB backend](../operator-guide/capdb-backend.md) · [Control plane](../concepts/control-plane.md)
