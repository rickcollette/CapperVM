# Delete / dependency-cleanup audit

A pass over every subsystem's delete path, checking that deleting a resource
cleans up its dependent records and host resources instead of leaving orphans.

## Root cause class

The SQLite connection ([`internal/store/open.go`](internal/store/open.go)) sets
`busy_timeout`, `journal_mode`, and `synchronous` but **not `foreign_keys`**, so
SQLite foreign-key enforcement is **off**. Every `... REFERENCES ... ON DELETE
CASCADE` in the schema is therefore a no-op — those cascades never fire, and
deletes must clean children explicitly. Enabling the pragma globally is risky on
existing data (any pre-existing FK violation would start erroring), so the fixes
below cascade explicitly in code.

## Fixed in this pass

| Subsystem | Delete | Was orphaning | Fix |
|---|---|---|---|
| Network | instance delete | the instance's **IP lease(s)** when `NetworkID` was unset/stale → blocked network deletion forever | `detachNetwork` now releases **all** leases held by the instance ([instance_manager.go](internal/manager/instance_manager.go)) |
| Network | network delete | leases of **already-deleted** instances permanently blocked deletion | delete now prunes orphaned leases and blocks only on **live** instances, naming them ([handlers_storage_network.go](internal/api/handlers_storage_network.go)) |
| Network | (ongoing) | — | `orphanLeaseReconciler` continuously prunes leases whose instance is gone ([daemon.go](internal/control/daemon.go)); `PruneOrphanedNetworkLeases` ([store/network_leases.go](internal/store/network_leases.go)) |
| IPAM | `ip-pool` delete | `routable_ip_bindings` of the pool's addresses + pool-scoped `ipam_exclusions` | cascade both ([ipam/store.go](internal/ipam/store.go)) |
| Load balancer | `lb` delete | `lb_backends` | cascade backends ([lb/store.go](internal/lb/store.go)) |
| VPC | `vpc` delete | **all** children: subnets, route tables, routes, security groups, SG rules, RT associations, internet/NAT gateways | explicit cascade in dependency order ([vpc/store.go](internal/vpc/store.go)) |
| IAM | user delete | role **grants**, **group memberships**, and API **tokens** (orphaned tokens keep authenticating — security hole) | cascade all three ([iam/store.go](internal/iam/store.go)) |
| IAM | group delete | group **memberships** and grants made **to** the group | cascade both ([iam/store.go](internal/iam/store.go)) |

## Verified already-correct (no change)

- **Instance delete** — tears down netns, veth, cgroup, overlay/diskquota mounts,
  CSD FUSE mounts, pool-backed disk allocation (`ReleaseByOwner`), and `instDir`.
- **Network delete** — removes bridge, metadata DNAT (host resources).
- **DNS zone delete** — cascades records + services.
- **Firewall delete** — cascades rules; also tears down the nft chain.
- **Storage bucket delete** — `os.RemoveAll` of the object directory (refuses
  non-empty unless `--force`).
- **Storage volume delete** — refuses while attached; removes the volume dir.
- **Storage pool (host) delete** — refuses while allocations exist.
- **IPAM** reserve/release/detach — releases bindings on the address.

## Remaining findings (need a policy decision, not yet changed)

1. **Managed database delete** ([database/manager.go](internal/database/manager.go))
   only deletes the `managed_databases` row. It does **not** stop/remove the
   backing managed instance (a running capsule → orphaned host workload) nor its
   `*_backups`. Fix needs the manager to tear down the instance (like
   `Instances.Delete`) and cascade backups. *Higher-touch; deferred.*
2. **Org / account delete** ([org/store.go](internal/org/store.go)) deletes only
   the org/account row, orphaning child accounts, root-users, guardrails, and the
   account's IAM objects. Recommended policy: **block** deletion while children
   exist (safer than cascading an entire tenant) — needs a product decision.
3. **Global `foreign_keys` pragma**: enabling it would make the declared
   `ON DELETE CASCADE`s enforce automatically and catch future orphans, but must
   be done with a migration that first repairs any existing violations. Tracked
   as a follow-up rather than flipped blindly here.

## How the network-lease symptom is resolved

The reported error — *network has 2 active lease(s); disconnect all instances
first* after the instances were already deleted — is now fixed three ways:
deletes release all leases, the network-delete path prunes orphans and reports
only live instances by name, and a reconciler sweeps stragglers. Stale leases no
longer block network deletion or starve the IP pool (which also unblocks
attaching to a network whose addresses were stuck behind dead leases).
