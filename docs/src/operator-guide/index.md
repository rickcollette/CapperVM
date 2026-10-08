---
title: "Operator guide"
description: "Day-to-day operation of every Capper subsystem."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Operator guide

How to operate each Capper subsystem. New here? Start with the
[Getting Started](../getting-started/overview.md) section and the
[Concepts](../concepts/architecture.md), then return for task-focused guides.

## Compute

- [Manage instances](manage-instances.md) — run, inspect, and operate capsules.
- [Compute groups & autoscale](compute-groups.md) — managed instance sets.
- [Bottles](bottles.md) — declarative app deployments.
- [Topology & nodes](topology.md) — realms/regions/zones/nodes, join & placement.
- [Host inventory](hosts.md) — host capabilities and `host doctor`.

## Networking

- [Manage VPCs & networking](manage-networks.md) — VPCs, subnets, ENIs, routing.
- [Networking dashboard](manage-networks.md) — drift and utilization.
- [Firewall](firewall.md) — nftables network policy.
- [Load balancers](load-balancers.md) · [Ingress](ingress.md)
- [Manage DNS](manage-dns.md) — zones, records, service discovery.
- [Public IPAM / Elastic IPs](routable-ips.md)
- [VPC Mobility](vpc-mobility.md) — migrate VPC workloads across realms/regions.

## Storage & data

- [Admin section](admin-section.md) — host storage pools, fail2ban, UFW, limits.
- [Manage storage](manage-storage.md) — volumes, object store, snapshots, CSD.
- [Manage backups](manage-backups.md) — policies and restore.
- [Managed databases](managed-databases.md)
- [CapDB storage backend](capdb-backend.md) — networked control-plane database.

## Identity, security & governance

- [Manage IAM](manage-iam.md) — users, roles, policies, tokens, audit.
- [Secrets](secrets.md) · [KMS](kms.md)
- [Posture, SBOM & signing](posture-sbom-signing.md)
- [Quotas](quotas.md) · [Governance](governance.md)

## Platform services

- [Certificates](certificates.md) — ACME/internal-CA issuance and renewal.
- [Serverless (Functions & MCP)](serverless.md)
- [Stacks (infrastructure as code)](stacks.md)
- [Marketplace](marketplace.md) · [Registries](registries.md)
- [Message queues](queues.md) · [Schedules](schedules.md) · [Operational jobs](jobs.md)
- [Events & rules](events-and-rules.md) · [Active context](context.md)
- [AI agents & MCP](ai.md)
- [Observability](observability.md) — metrics, events, drift, alerts.

## Reference

- [CLI reference](../reference/cli/capper.md) · [API reference](../reference/api/overview.md)
  · [Go SDK](../reference/sdk/go.md)
