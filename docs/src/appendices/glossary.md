---
title: "Glossary"
description: "Definitions of Capper terms and subsystems."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Glossary

| Term | Definition |
| --- | --- |
| **`.cap` image / capsule** | Capper's container image format and the running instance built from it. |
| **`capinit`** | The PID-1 init process inside a capsule. |
| **Account** | A billing/isolation boundary inside an organization; holds IAM and resources. |
| **AIO (all-in-one)** | A single-node deployment running the API, daemon, and services together (`capper aio`). |
| **Autoscale** | Policy-driven resizing of a compute group between min/max. |
| **Bottle** | A declarative app deployment (`capper bottle`). |
| **CapDB** | A vendored SQLite fork with a TLS client/server protocol and native pool; the optional networked control-plane backend. |
| **Compute group** | A managed set of instances kept at a desired size. |
| **Control plane** | The authoritative daemon that owns state, serves the API, and runs reconcilers. |
| **CSD** | Capper shared/replicated volumes mountable across nodes. |
| **ENI** | Elastic network interface — VPC attachment with private IP(s) and security groups. |
| **Elastic / public IP** | A routable IP allocated from a pool and bound to an instance or load balancer (IPAM). |
| **Guardrail** | An org-level deny that overrides account policy and root (governance). |
| **Host storage pool** | Admin-registered physical capacity (directory or LVM) backing instance disks and block volumes. |
| **IAM** | Identity and access management: users, groups, roles, policies, tokens. |
| **Ingress** | Host/path routing in front of services. |
| **KMS** | Key management: envelope encryption of data keys under a master key. |
| **MCP** | Model Context Protocol; managed MCP servers expose tools to AI agents with per-tool IAM. |
| **Node** | A worker machine running `capper-agent` that joins the topology. |
| **Organization** | The top of the tenancy hierarchy (org → account → project). |
| **Placement / scheduler** | Decides which node runs a workload by role, capacity, zone, and failure domain. |
| **Posture scan** | A security/configuration scan of an image. |
| **Project** | A resource namespace inside an account. |
| **Realm / region / zone** | The physical topology hierarchy (realm → region → zone → node). |
| **Reconciler** | A control loop that converges actual state toward desired state. |
| **SBOM** | Software Bill of Materials generated for an image (`capper attest sbom`). |
| **Security group** | Instance-level allow rules within a VPC/subnet. |
| **Service account** | A non-human IAM principal. |
| **Stack** | A declared set of resources applied/destroyed as a unit (IaC). |
| **Store** | The single database (with ~45 sub-stores) holding all control-plane state. |
| **Subnet** | A CIDR slice inside a VPC; instances and load balancers require a subnet ID. |
| **VPC** | An isolated virtual private cloud of subnets with routing. |
| **VPC Mobility** | Migrating a VPC's workloads across realms/regions (plan → approve → execute → cutover). |

## Related

- [Overview](../getting-started/overview.md) · [Concepts](../concepts/architecture.md)
