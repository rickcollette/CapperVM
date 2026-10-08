---
title: "API reference — VPCs and networking"
description: "VPC, subnet, ENI, and networking endpoints (legacy /networks removed)."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# API reference — VPCs and networking

All paths are under `/api/v1` and require [authentication](overview.md).

> **Removed:** `GET/POST /networks`, attach/detach, and `migrate-legacy-networks`.
> Use VPC subnets instead.

## Core VPC routes

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/vpcs` | list VPCs |
| `POST` | `/vpcs` | create a VPC |
| `GET` | `/vpcs/{vpc}` | inspect a VPC |
| `POST` | `/vpcs/{vpc}/subnets` | create a subnet |
| `GET` | `/subnets/{subnetId}` | inspect a subnet |
| `GET` | `/network-interfaces` | list ENIs (`?vpcId=`, `?subnetId=`) |
| `POST` | `/network-interfaces` | create an ENI |
| `GET` | `/networking/dashboard` | VPC/subnet summary and drift |
| `GET` | `/networking/topology` | topology graph |

Additional groups cover route tables, IGW/NAT, security groups, NACLs, VPC
peerings, flow logs, target groups, load balancers, ingress, DNS, and public IPAM.
See [All routes](routes.md) for the full list.

## Instance networking

`POST /instances` requires `subnetId`. Optional: `vpcId`, `securityGroupIds`,
`privateIpAddress`, `publicIpBehavior`.

## Load balancers

`POST /lb` requires `subnetId` (stored as the LB's network scope).

## DNS scoping

When creating zones or records with `networkId`, the value must be a **subnet ID**.

## Related

- [API overview](overview.md) · [Manage VPCs](../../operator-guide/manage-networks.md)
  · [Networking model](../../concepts/networking-model.md) · [All routes](routes.md)
