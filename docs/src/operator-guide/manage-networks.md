---
title: "Manage VPCs and networking"
description: "Create VPCs and subnets, launch instances into subnets, and expose services."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Manage VPCs and networking

This guide covers private connectivity. For the conceptual layering (traffic
control, exposure, public reachability) see the
[Networking model](../concepts/networking-model.md).

> **Note:** Flat virtual networks (`capper network`, `/api/v1/networks`) were
> removed. All workloads use **VPC subnets**.

## Web console

| Page | Purpose |
| --- | --- |
| **Network → VPCs** | List VPCs; link to create wizard and detail |
| **Network → VPCs → Create** | 3-step wizard: identity, initial subnets, IGW/NAT |
| **Network → VPC detail** | Tabbed management: subnets, route tables, security groups, NACLs, gateways, flow logs |
| **Network → Networking** | Dashboard: VPC/subnet counts, drift warnings, VPC links |
| **Compute → Instances → Launch** | Pick VPC + subnet (filtered by kind); multi-select security groups |

![VPCs](/assets/images/screenshots/03-vpcs.png)

![Create VPC wizard](/assets/images/screenshots/26-create-vpc-wizard.png)

![VPC detail](/assets/images/screenshots/34-vpc-detail.png)

![Launch wizard — VPC/subnet step](/assets/images/screenshots/33-launch-instance-networking.png)

## CLI — VPCs and subnets

```bash
capper vpc create my-vpc --cidr 10.0.0.0/16 --home-region local --mobility manual
capper vpc subnet create my-vpc --name app --cidr 10.0.1.0/24 --zone local-a
capper vpc list
capper vpc inspect my-vpc
capper vpc route-table ...        # routes between subnets / gateways
capper vpc igw ...                # internet gateway
capper vpc nat ...                # NAT gateway
```

| Flag (`vpc create`) | Purpose |
| --- | --- |
| `--cidr <cidr>` | VPC CIDR block |
| `--home-region <slug>` | home region for the VPC |
| `--mobility manual\|...` | mobility policy; see [VPC Mobility](vpc-mobility.md) |
| `--name <display>` | display name |

Security groups (`capper sg`) attach allow-rules to ENIs; node firewalls
(`capper firewall`) enforce network policy. See [Firewall](firewall.md).

## Launch instances into a subnet

**API** — `subnetId` is required:

```bash
curl -X POST /api/v1/instances \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "image": "alpine",
    "name": "web-1",
    "subnetId": "<subnet-id>",
    "securityGroupIds": ["<sg-id>"]
  }'
```

**CLI** — use the Web launch wizard or SDK with `subnetId`. The legacy `network`
field is rejected.

## AIO bootstrap

Fresh AIO installs (`deploy/remote-setup.sh`) automatically create:

- VPC `default-vpc` (`10.88.0.0/16`)
- Subnet `default` (`10.88.1.0/24`)
- A default **storage pool** (see [Admin section](admin-section.md#storage-physical-disks--pools))

## Exposing services

Put a [load balancer](load-balancers.md) (with `subnetId`) or [ingress](ingress.md)
in front of instances, give workloads names with [DNS](manage-dns.md), and make
them reachable from outside with [public IPs](routable-ips.md).

## Moving a VPC

To migrate a VPC's workloads across realms/regions, use the plan→approve→execute→
cutover flow in [VPC Mobility](vpc-mobility.md).

## Related

- [Networking model](../concepts/networking-model.md) · [Firewall](firewall.md)
  · [Load balancers](load-balancers.md) · [Ingress](ingress.md)
  · [Manage DNS](manage-dns.md) · [Public IPAM](routable-ips.md)
  · [VPC Mobility](vpc-mobility.md) · [Manage instances](manage-instances.md)
