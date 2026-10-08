---
title: "Overview of the Capper platform"
description: "What Capper is, its subsystems, and how the four interfaces fit together."
owner: "docs"
status: "stable"
reviewed: "2026-06-12"
outputs:
  - markdown
  - web
  - pdf
---

# Overview of the Capper platform

Capper is a **self-hosted, multi-tenant cloud control plane**. It began as a local
`.cap` capsule image runner and grew into a full platform: compute, networking,
storage, identity, topology, certificates, observability, serverless, and public
IP management — all driven by a single control plane and reachable through four
interfaces.

> **Security note:** Do not run untrusted `.cap` images with Capper v0. Capsule
> isolation is hardening, not a security boundary against hostile images.

## What you can run on it

| Area | What it provides |
| --- | --- |
| **Compute** | `.cap` capsule instances (bwrap/chroot/crun/runc), images, templates, instance types, GPU inventory, compute groups + autoscale |
| **Networking** | VPCs + subnets (required for instances/LBs), ENIs, route tables, security groups, firewalls, load balancers, DNS, ingress, public IPAM |
| **Storage** | admin host storage pools (required for instance/volume disks), block volumes, S3 buckets, CSD, backups |
| **Storage** | block volumes, an S3-compatible object store, snapshots, CSD shared/replicated volumes, backups |
| **Multi-tenancy** | organizations → accounts → projects, IAM (users/groups/roles/policies), managed policies, assume-role, quotas, governance, audit |
| **Topology** | realms → regions → zones → nodes, node pools, service roles, the `capper-agent` node daemon, a placement scheduler |
| **VPC Mobility** | plan → approve → execute → cutover migration of VPC workloads across realms/regions |
| **Certificates** | ACME / Let's Encrypt issuance, a renewal scheduler, bindings, an internal CA |
| **Observability** | unified resource inventory, config drift, metrics, resource events, alerts |
| **Serverless** | Lambda-style Functions (triggers, invocations) and managed MCP servers with per-tool IAM + approval gates |
| **Security** | KMS, secrets, image posture scanning, SBOM/attestation, image signing, marketplace review |

## The four interfaces

Every subsystem is exposed consistently across all four:

- **CLI** — `capper <subsystem> <verb>` (e.g. `capper compute instance list`,
  `capper org create`, `capper fn invoke`, `capper ip-pool create`). Run
  `capper --help`.
- **REST API** — `capper api start` serves `/api/v1/...` with bearer-token (or
  cookie-session) auth. See the [API reference](../reference/api/overview.md).
- **Go SDK** — `import cappersdk "capper/sdk/go"`; `c := cappersdk.New(url, token)`.
  See the [SDK guide](../developer-guide/sdk.md).
- **Web UI** — CapperWeb (Vite + React), served via
  `capper api start --console <dist>`.

## How it is put together

A single **control-plane daemon** owns all state in one database (embedded pure-Go
SQLite by default, or the networked [CapDB backend](../operator-guide/capdb-backend.md)).
Worker **nodes** run the `capper-agent` daemon, which heartbeats, reports
inventory, pushes metrics, and supervises services. The
[architecture overview](../concepts/architecture.md) and the
[control-plane concept](../concepts/control-plane.md) go deeper.

## Where to go next

- [Installation](installation.md) — build the binaries and prerequisites.
- [Quickstart](quickstart.md) — run a capsule and start the control plane.
- [Configuration](configuration.md) — environment variables, flags, and files.
- [Concepts](../concepts/architecture.md) — the model behind the platform.
- [Tutorials](../tutorials/index.md) — end-to-end walkthroughs by goal.
