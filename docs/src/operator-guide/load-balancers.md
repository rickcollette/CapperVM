---
title: "Load balancers"
description: "ELB-style load balancers with listeners, target groups, and VIP placement."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Load balancers

Capper load balancers follow an **ELB-style model**: one load balancer has a
**scheme** (internal or internet-facing), a **VIP**, multiple **listeners**
(HTTP/HTTPS/TCP ports on the VIP), and **target groups** with registered
backends.

## Create and manage

Use the console wizard at **Load Balancers → Create LB** (`/lb/new`) or the API:

```bash
curl -X POST /api/v1/lb -d '{
  "name": "web-lb",
  "scheme": "internal",
  "type": "application",
  "subnetId": "sub_…",
  "autoVip": true,
  "listenerProtocol": "HTTP",
  "listenerPort": 80,
  "targetGroupName": "web-tg",
  "targetGroupPort": 8080,
  "initialTargetAddr": "10.0.1.5:8080"
}'
```

| Field | Purpose |
| --- | --- |
| `scheme` | `internal` or `internet-facing` |
| `type` | `application` (HTTP/HTTPS) or `network` (TCP) |
| `subnetId` | **Required** — VPC subnet for VIP placement |
| `vip` / `autoVip` | explicit VIP or auto-allocate (subnet IP or routable pool) |
| `poolId` | routable IP pool for internet-facing LBs |
| `listenerProtocol` / `listenerPort` | first front-end listener (optional) |
| `targetGroupName` / `targetGroupPort` | default target group for that listener |

Legacy single-listener create (`listenAddr` + `mode`) is still supported for
automation; existing LBs are migrated to listeners + target groups on upgrade.

![Load balancers](/assets/images/screenshots/08-load-balancers.png)

## Listeners and target groups

Each listener binds `vipAddress:port` and forwards to one target group.
Manage on the LB detail page tabs or via API:

```bash
# Listeners
GET/POST /api/v1/lb/{name}/listeners
PATCH/DELETE /api/v1/lb/{name}/listeners/{id}

# Target groups (LB-scoped)
GET/POST /api/v1/lb/{name}/target-groups
POST/DELETE /api/v1/lb/{name}/target-groups/{tgId}/targets
```

Internal VIP picker: `GET /api/v1/subnets/{id}/available-ips`

## TLS and ACME

- **HTTP** listeners serve ACME `http-01` at `/.well-known/acme-challenge/`.
- **HTTPS** listeners require a `certificateId` before the proxy starts.
- Attach per listener: `POST /api/v1/lb/{name}/listeners/{id}/certificates`

Issue certificates under [Certificates](certificates.md), then attach on the
**TLS** tab of the LB detail page.

## IP placement

| Scheme | VIP source |
| --- | --- |
| **internet-facing** | Routable IP from a pool whose `usage` includes `load-balancer` ([Public IPAM](routable-ips.md)) |
| **internal** | Private IP allocated in the subnet CIDR |

On create, the API reserves/attaches routable IPs or allocates subnet IPs
automatically when `autoVip` is true.

## Related

- [Networking model](../concepts/networking-model.md) · [Ingress](ingress.md)
  · [Manage VPCs](manage-networks.md) · [Public IPAM](routable-ips.md)
