---
title: "Platform Engineer"
description: "Guide for engineers who operate the Capper control plane."
owner: "docs"
status: "stable"
reviewed: "2026-06-10"
outputs:
  - markdown
  - web
  - pdf
---

# Platform Engineer

You own the Capper installation. Your job is to keep the platform healthy,
allocate capacity, and hand the right tools to every other persona.

## Dashboard

The dashboard is your first stop. It shows running and failed instance counts,
recent audit events, storage utilisation, and daemon health — all in a single
view.

![Capper Dashboard](/assets/images/screenshots/01-dashboard.png)

## Topology — Nodes and Zones

Register physical hosts, assign them to availability zones, and cordon or drain
nodes before maintenance. The Topology page provides four tabs: Realms,
Regions, Zones, and Nodes.

![Topology — Nodes & Zones](/assets/images/screenshots/13-topology.png)

**Typical tasks:**

- Register a new host with `POST /api/v1/nodes`
- Cordon a node before kernel upgrades; uncordon when done
- Create a new availability zone to reflect a rack move

## VPCs — Network Isolation

Create a VPC per tenant or environment. Attach subnets with explicit CIDR
blocks to control blast radius if a workload misbehaves.

![VPCs](/assets/images/screenshots/03-vpcs.png)

Each VPC row expands inline to show its subnets. Click **Create VPC** to
open the creation form; subnets can be added from the same expanded row.

## IAM Bootstrap

During first boot Capper creates a `bootstrap` IAM user with the built-in
`admin-all` policy. From the IAM Users page you can create operator accounts,
assign them to groups, and attach least-privilege policies before revoking the
bootstrap token.

![IAM Users](/assets/images/screenshots/11-iam.png)

The IAM section expands in the left nav to expose Users, Groups, Roles,
Policies, Simulate, Tokens, and Audit Log.

## Quotas

Set per-resource limits before handing a project to a tenant to prevent any
single workload from exhausting shared capacity.

![Quotas](/assets/images/screenshots/19-quotas.png)

## Settings

Global platform configuration: project metadata, default region, feature
flags, and API token management.

![Settings](/assets/images/screenshots/23-settings.png)

## Key Workflows

- [Onboard a New Team](../tutorials/onboard-team.md) — VPC + IAM + Quotas in one pass
- [Manage Topology](../operator-guide/index.md) — node registration, cordon, drain
