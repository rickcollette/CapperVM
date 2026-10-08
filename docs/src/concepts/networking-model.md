---
title: "Networking model"
description: "VPCs, subnets, ENIs, firewalls, load balancers, DNS, ingress, and public IPs."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Networking model

Capper networking is layered: private connectivity (VPCs and subnets), traffic
control (security groups, network ACLs, and firewalls), service exposure (load
balancers, ingress, DNS), and reachability from outside (public IPAM / elastic IPs).

![Networking dashboard](/assets/images/screenshots/32-networking-dashboard.png)

## Canonical VPC model

The **canonical control plane** is `internal/vpc`, exposed uniformly via REST API,
SDK, CLI, and CapperWeb. **Every network-scoped workload launches into a VPC
subnet** — there is no separate flat “virtual network” layer anymore.

- **VPCs** — isolated clouds with subnets, route tables, IGW, NAT, security groups, and NACLs.
- **Subnets** — CIDR slices inside a VPC; each subnet gets a Linux bridge and gateway for instance dataplane.
- **ENIs** — attachable interfaces with private IPs allocated from subnet IPAM (gateway addresses are reserved).
- **Instances** — require `subnetId` (and optionally `securityGroupIds`) at launch; the control plane creates a primary ENI and attaches the instance to the subnet bridge.
- **Load balancers** — require `subnetId` so listeners are scoped to a VPC subnet.
- **DNS zones** — when scoped with `networkId`, that value must be a subnet ID.

Legacy flat virtual networks (`/api/v1/networks`, `capper network`) were removed in
v0.1. Migrate any automation to VPC subnets.

## Private connectivity

```bash
capper vpc create my-vpc --cidr 10.0.0.0/16
capper vpc subnet create my-vpc --name app --cidr 10.0.1.0/24 --zone local-a
```

On **AIO deploy**, `remote-setup.sh` bootstraps a `default-vpc` with a `default`
subnet (`10.88.1.0/24`) so instances can reach the metadata service
(`169.254.169.254`) for capinit.

![VPCs](/assets/images/screenshots/03-vpcs.png)

## Traffic control

- **Security groups** (`capper sg`) — instance-level allow rules attached to ENIs.
- **Network ACLs** — subnet-level stateless filters.
- **Firewalls** (`capper firewall`) — node policies programmed with **nftables**.
  See [Firewall](../operator-guide/firewall.md).

## Service exposure

- **Load balancers** (`capper lb`) — distribute traffic; require `subnetId`.
- **Ingress** (`capper ingress`) — host/path routing rules in front of services.
- **DNS** (`capper dns`) — private zones and records. See [Manage DNS](../operator-guide/manage-dns.md).

## Reachability from outside

- **Public IPAM / Elastic IPs** (`capper ip`, `capper ip-pool`) — allocate routable
  IPs from pools and bind them to instances or load balancers.
  See [Public IPAM](../operator-guide/routable-ips.md).

## How a packet finds a workload

1. A client resolves a name via DNS (or hits a public/elastic IP).
2. Traffic lands on an ingress rule or a load balancer listener in a subnet.
3. The LB/ingress selects a backend instance in the same VPC.
4. Security-group, NACL, and firewall rules permit (or drop) the flow.
5. The instance receives the packet on its primary ENI in the subnet.

## Related

- [Manage networks & VPCs](../operator-guide/manage-networks.md)
  · [Firewall](../operator-guide/firewall.md)
  · [Load balancers](../operator-guide/load-balancers.md)
  · [Ingress](../operator-guide/ingress.md)
  · [Manage DNS](../operator-guide/manage-dns.md)
  · [Public IPAM](../operator-guide/routable-ips.md)
  · [VPC Mobility](../operator-guide/vpc-mobility.md)
