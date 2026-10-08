# Capper — Remediation Plan

End-to-end review of the Capper control plane (Go) and CapperWeb (TS/React),
**excluding CapDB**. Checkboxes track changes for incomplete / hollow / simplified
/ dead / fail-open code.

**Status (2026-06-18):** the bounded, high-confidence fixes are done; `go build`,
`go vet`, and `go test ./...` are green. Two items remain as larger feature work
(IPAM dataplane programming, full VPC-mobility step wiring) and are scoped below.

---

## P1 — Hollow implementations (claim success, do nothing)

- [x] **VPC Mobility no longer fakes success.** All unimplemented steps in
  [internal/vpcmover/steps.go](internal/vpcmover/steps.go) now route to
  `stepUnimplemented`, which records an event and returns an explicit error, so a
  mobility job halts with a truthful failed status instead of reporting success
  while moving nothing. (The genuinely-wired steps — lock, inventory, capacity
  check, delete-policy, create-destination-VPC, mark-retired — are unchanged.)
  - [ ] **Remaining (feature work):** wire each step to its real subsystem
    (compute, storage, network, DNS, LB, firewall). The executor currently holds
    only the mobility `Store` + a topology-only `InventoryStore`; real moves need
    those managers threaded in via `NewExecutor`. Add an integration test that
    asserts destination resources are actually created.

- [ ] **Elastic-IP / IPAM bindings are persisted but never programmed.** `Attach`
  stores `IPBinding` rows (modes `snat`/`dnat`/`vip`,
  [internal/ipam/store.go:283](internal/ipam/store.go#L283),
  [internal/ipam/manager.go:145](internal/ipam/manager.go#L145)) but no node-agent
  path programs SNAT/DNAT/VIP into the dataplane. **Left for implementation** — it
  needs a node-agent change (nftables/iptables + VIP address programming, reconcile
  on startup) that is platform-sensitive and shouldn't be written blind. Scope first:
  is dataplane programming expected in AIO, or only on network-role nodes?

- [x] **Quota fail-open is now observable.** `CheckQuota` still fails open on a
  usage-table error (availability over hard-fail) but logs a `slog.Warn` with
  project/resource/limit/err so it is no longer silent.
  [internal/billing/types.go](internal/billing/types.go).

- [x] **`handleSchedulerPlacements` is now honest.** Placement decisions aren't
  persisted (the scheduler computes on demand); the endpoint now says so explicitly
  and returns the placement-eligible node inventory with a `placementHistory:false`
  marker instead of pretending to return a decision log.
  [internal/api/handlers_topology.go](internal/api/handlers_topology.go). (No CLI/SDK/
  web consumer — shape change is safe.)

## P2 — Dead / unwired / partial code

- [x] **Removed dead `internal/scanner` package** (579 LOC, no importers; heuristic
  name-string matching superseded by `registry.ScanImage` (trivy/sha256/SBOM) and
  `Store.Posture.Scan`).

- [x] **Posture scan endpoint confirmed wired.** `Store.Posture` is initialized by
  default in [internal/store/db.go:183](internal/store/db.go#L183); the `501` only
  fires defensively when it is nil. No change needed.

- [x] **Factory `501` confirmed deferred-by-design** (→ CapsuleBuilder). The
  CapperWeb Factory route is already feature-gated off in the AIO profile; in the
  full profile it is intentionally a deferred endpoint. No change needed.

## P3 — Quality

- [x] **Fixed a real swallowed-error bug (message loss).** `moveToDLQ`
  ([internal/eventing/queues.go](internal/eventing/queues.go)) deleted the source
  message even when the DLQ send failed → silent loss. Now it only deletes after a
  successful DLQ send and logs failures.
  - [ ] **Remaining:** the other swallowed-error sites surveyed are legitimate
    (HTTP `ResponseWriter.Write`, `hash.Hash.Write`, cleanup `Close`, idempotent
    "already gone" no-ops). Re-audit opportunistically; none are dropping a real
    failure today.

- [x] **Documented trivy as an operational dependency** for image vuln scanning
  ([docs/.../posture-sbom-signing.md](docs/src/operator-guide/posture-sbom-signing.md));
  absent trivy degrades to a non-fatal `warn`. Consider installing trivy in the AIO
  bundle.

- [ ] **IAM bootstrap returns the raw OS username as principal ID**
  ([internal/iam/manager.go:115](internal/iam/manager.go#L115)). Low risk; confirm
  it can't leak into a non-bootstrap path, then close.

---

## Notes

- **CapperWeb is clean** of stubs (the `placeholder=` hits are HTML input hints).
- **CSD replication is real** (working QUIC transport), not a stub.
- Re-run a fresh review after the two remaining feature items (IPAM dataplane, full
  VPC-mobility wiring) land.
