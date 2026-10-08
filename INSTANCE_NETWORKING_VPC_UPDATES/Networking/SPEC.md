# Capper Networking — AWS-Style Redesign SPEC

Status: Draft for implementation  
Owner: Capper Networking / VPC / Ingress / IPAM / WebUI  
Target: Backend, API, SDK, CLI, WebUI, Docs, Tests  
Non-goal: This is not code. This is the product and engineering contract.

## 1. Purpose

Capper Networking must become the full cloud networking control plane that sits under VPCs, Instances, Load Balancing, DNS, IPAM, Firewalling, Ingress, Certificates, and Service Connectivity. The current Capper model has useful pieces, but they need to be unified into an AWS-like resource model where VPC constructs are coherent and every backend/WebUI/SDK/CLI path agrees.

This spec covers the broader Networking domain. The VPC spec covers VPC-specific resources. The Instances spec covers instance attachment points. This document binds them together as one networking system.

## 2. Current Capper Baseline

The uploaded Capper snapshot shows existing network, firewall, ingress, load balancer, DNS, certificate, IPAM, resource monitor, S3, IAM, topology, VPC mobility, and runtime network namespace work. The SDK includes Networks, VPCs, IPAM, Firewalls, DNS, LB, Ingress, Certificates, Resources, Scheduler, Regions, Zones, Nodes, and Migrations. Runtime code already supports network namespaces for instances. IPAM already models routable IP pools and reserved/allocated addresses.

The gap is product coherence: users need AWS-like networking options, not a collection of partially related features.

## 3. Networking Domain Boundaries

Networking owns these resource families:

- VPCs
- Subnets
- Route tables
- Routes
- Internet gateways
- NAT gateways
- Security groups
- Network ACLs
- ENIs
- Private IPs
- Public/Elastic IPs
- IP pools
- DNS zones and records
- Private hosted zones
- Load balancers
- Target groups
- Listeners
- Ingress rules
- WAF rules
- TLS certificates bindings
- VPC endpoints/private service endpoints
- VPC peering/transit attachments
- VPN/customer gateways/future hybrid connectivity
- Flow logs
- Reachability analyzer
- Network health checks

## 4. Product Principles

### 4.1 AWS-Like, Capper-Native

Capper should use AWS-like nouns and flows where they help users understand the system:

- VPC
- Subnet
- Route table
- Internet gateway
- NAT gateway
- Security group
- Network ACL
- Elastic IP / Public IP allocation
- Network interface
- Load balancer
- Target group
- Listener
- VPC endpoint
- Peering
- Flow logs

Capper-native concepts remain where they are useful:

- Realm
- Region
- Zone
- Node
- Project
- Capper Resource Name
- Capper labels
- Capper mobility/failover
- Local/private cloud backends

### 4.2 One Network Graph

All networking resources must be representable as a graph:

- VPC contains subnets.
- Subnets associate with route tables and NACLs.
- Instances attach ENIs in subnets.
- ENIs carry private/public IP associations and security groups.
- Route tables point to gateways, NAT, endpoints, peerings, ENIs, instances, or blackholes.
- Load balancers attach to subnets and route to target groups.
- Target groups point to instances, ENIs, IPs, functions, or MCP servers depending on service type.
- DNS records point to LBs, public IPs, instances, services, or static values.

The WebUI must display this graph and use it for delete/move/reachability decisions.

## 5. IPAM

### 5.1 Private IPAM

Private IPAM owns subnet address allocation.

Required behavior:

- Reserve system addresses per subnet.
- Allocate primary private IPs to ENIs.
- Allocate secondary private IPs to ENIs.
- Support manual IP selection.
- Prevent conflicts.
- Track available IP count.
- Track stale/orphaned reservations.
- Provide audit history.

Private IP statuses:

- `available`
- `reserved`
- `assigned`
- `stale`
- `quarantined`

### 5.2 Public IPAM / Elastic IPs

Public IPAM owns routable IP pools and public IP associations.

Required public IP fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `eipalloc_...` or `pubip_...`. |
| poolId | yes | Parent public IP pool. |
| address | yes | IPv4/IPv6 address. |
| status | yes | `available`, `reserved`, `associated`, `released`, `quarantined`. |
| scope | yes | `region`, `zone`, `global`, `edge`. |
| projectId | no | Owning project when allocated. |
| purpose | yes | `instance`, `load-balancer`, `nat-gateway`, `ingress`, `reserved`, `system`. |
| targetType | no | Associated resource type. |
| targetId | no | Associated resource ID. |
| associationId | no | Stable association ID. |
| reverseDns | no | PTR name. |
| tags | no | User tags. |

Required operations:

- Create pool.
- Exclude addresses.
- Reserve address.
- Allocate address.
- Associate address to ENI private IP, load balancer, NAT gateway, or ingress.
- Disassociate address.
- Release address.
- Quarantine address.
- Show usage and ownership.

Rules:

- A public IP can have only one active association.
- Public IPs associate to network attachment points, not arbitrary objects.
- NAT gateways and public load balancers require public IP support.
- Releasing an associated IP requires disassociation or force.

## 6. DNS

DNS must support public and private hosted zones.

### 6.1 Zone Fields

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `zone_...`. |
| name | yes | Zone name. |
| zoneType | yes | `public` or `private`. |
| vpcAssociations | conditional | Required for private zones. |
| accountId | yes | Owner. |
| projectId | no | Project scope. |
| status | yes | Lifecycle state. |
| tags | no | Tags. |

### 6.2 Record Fields

| Field | Required | Description |
| --- | --- | --- |
| id | yes | Stable record ID. |
| zoneId | yes | Parent zone. |
| name | yes | Record name. |
| type | yes | A, AAAA, CNAME, TXT, MX, SRV, CAA, PTR, ALIAS. |
| values | conditional | Literal values. |
| aliasTarget | conditional | Load balancer, ingress, public IP, service endpoint. |
| ttl | yes | TTL seconds. |
| routingPolicy | no | `simple`, `weighted`, `failover`, `latency`, future. |
| healthCheckId | no | Optional health check. |

Rules:

- Private zones only resolve inside associated VPCs.
- Public zones require explicit domain/delegation configuration.
- ALIAS records can target Capper LBs, ingress, public IPs, static sites, and service endpoints.
- DNS changes must be audited.
- DNS cutover must support lower TTL during VPC mobility.

## 7. Load Balancing

Capper load balancing should feel like AWS ELB concepts.

### 7.1 Load Balancer

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `lb_...`. |
| name | yes | Unique name. |
| type | yes | `application`, `network`, `gateway`, `internal`. |
| scheme | yes | `internet-facing` or `internal`. |
| vpcId | yes | Parent VPC. |
| subnetIds | yes | Subnets where LB is placed. |
| securityGroupIds | conditional | Required for application/internal HTTP LBs. |
| ipAddressType | yes | `ipv4`, `dualstack`, `ipv6`. |
| publicIpAllocationIds | conditional | Public scheme. |
| dnsName | yes | Generated DNS name. |
| status | yes | Lifecycle state. |
| tags | no | Tags. |

### 7.2 Listener

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `listener_...`. |
| loadBalancerId | yes | Parent LB. |
| protocol | yes | HTTP, HTTPS, TCP, TLS, UDP. |
| port | yes | Listener port. |
| certificateIds | conditional | HTTPS/TLS. |
| defaultAction | yes | Forward, redirect, fixed-response, reject. |
| rules | no | Listener rules. |

### 7.3 Target Group

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `tg_...`. |
| name | yes | Target group name. |
| protocol | yes | HTTP, HTTPS, TCP, TLS, UDP. |
| port | yes | Target port. |
| vpcId | yes | Parent VPC. |
| targetType | yes | `instance`, `ip`, `eni`, `function`, `mcp-server`. |
| healthCheck | yes | Health check config. |
| targets | yes | Registered targets. |
| stickiness | no | Optional. |
| deregistrationDelaySeconds | yes | Graceful drain. |

Rules:

- Public LBs require public subnet route path and public IP allocation.
- Internal LBs must not receive public IPs.
- Target health must be visible and queryable.
- Listener/TG changes must be auditable.

## 8. Ingress, WAF, and Certificates

Ingress is the higher-level app routing layer over LBs/DNS/certs.

### 8.1 Ingress Rule

Required fields:

- id
- name
- projectId
- host
- pathPrefix
- backend target type
- backend target ID
- tls certificate ID
- rate limit
- WAF policy ID
- priority
- status

### 8.2 WAF Policy

Required features:

- Rule priority.
- Match by IP/CIDR, path, method, header, query, body size, user agent.
- Actions: allow, block, count/log, challenge/future.
- Managed rule sets/future.
- Per-rule metrics.

### 8.3 Certificates

Networking must integrate with certificate manager:

- Issue certificate.
- Import certificate.
- Bind certificate to listener/ingress/static site/S3 endpoint.
- Auto-renew when possible.
- Show expiry warnings.
- Prevent deleting in-use certs unless forced.

## 9. Service Connectivity

### 9.1 VPC Endpoints

Private service access must not require public routing.

Endpoint services:

- object storage / S3-compatible service
- registry / image marketplace
- KMS
- secrets
- databases
- queues
- functions
- MCP servers
- control-plane APIs where allowed

Endpoint types:

- Gateway endpoint: route table target for service prefix.
- Interface endpoint: ENI-backed private service IP.
- Load-balancer service endpoint: private LB-backed service.

### 9.2 Service Discovery

Every service endpoint may publish private DNS names into associated VPCs.

Required private names:

- service-scoped name
- project-scoped name
- optional custom private zone alias

## 10. Network Security Enforcement

### 10.1 Security Group Enforcement

Security groups must compile to the dataplane. Depending on backend mode:

- nftables preferred.
- iptables compatibility allowed.
- eBPF future option.
- OVS/bridge flow rules future option.

Rules:

- Stateful behavior is required.
- Multiple SGs on one ENI combine as allow-union.
- No ingress allowed unless rule allows it.
- Egress defaults must follow account policy.

### 10.2 Network ACL Enforcement

NACLs must compile to subnet-level stateless rules.

Rules:

- Ordered evaluation.
- Explicit allow/deny.
- Final deny.
- Separate ingress/egress chains.
- Changes apply safely with rollback if compile fails.

### 10.3 Route Enforcement

Route tables must compile to Linux routing/network namespace dataplane state.

Required support:

- Local VPC routes.
- Default route to IGW/NAT.
- Routes to ENI/instance appliances.
- Peering/transit routes.
- Endpoint prefix routes.
- Blackhole route visibility when target disappears.

## 11. Reachability Analyzer

Provide a control-plane reachability analyzer.

Inputs:

- source type: instance, ENI, IP, subnet, internet, load balancer.
- source ID/address.
- destination type: instance, ENI, IP, subnet, internet, service endpoint, load balancer.
- destination ID/address.
- protocol/port.

Output:

- Allowed or blocked.
- Path graph.
- Route table decisions.
- Security group decisions.
- NACL decisions.
- Gateway/NAT/LB decisions.
- Blocking rule/route if blocked.
- Warnings about asymmetric routing.

## 12. API Surface

All endpoints are under `/api/v1` and use the standard envelope.

### 12.1 IPAM/Public IPs

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/ip-pools` | List IP pools. |
| POST | `/ip-pools` | Create IP pool. |
| GET | `/ip-pools/{poolId}` | Describe pool. |
| PATCH | `/ip-pools/{poolId}` | Update pool. |
| DELETE | `/ip-pools/{poolId}` | Delete pool. |
| GET | `/public-ips` | List public IPs. |
| POST | `/public-ips/allocate` | Allocate/reserve IP. |
| POST | `/public-ips/{allocationId}/associate` | Associate IP. |
| POST | `/public-ips/{associationId}/disassociate` | Disassociate IP. |
| DELETE | `/public-ips/{allocationId}` | Release IP. |
| GET | `/admin/ip-exclusions` | List exclusions. |
| POST | `/admin/ip-exclusions` | Add exclusion. |
| DELETE | `/admin/ip-exclusions/{id}` | Remove exclusion. |

### 12.2 DNS

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/dns/zones` | List zones. |
| POST | `/dns/zones` | Create zone. |
| GET | `/dns/zones/{zoneId}` | Describe zone. |
| PATCH | `/dns/zones/{zoneId}` | Update zone. |
| DELETE | `/dns/zones/{zoneId}` | Delete zone. |
| GET | `/dns/zones/{zoneId}/records` | List records. |
| POST | `/dns/zones/{zoneId}/records` | Create record. |
| PATCH | `/dns/zones/{zoneId}/records/{recordId}` | Update record. |
| DELETE | `/dns/zones/{zoneId}/records/{recordId}` | Delete record. |
| POST | `/dns/zones/{zoneId}/associate-vpc` | Associate private zone. |
| POST | `/dns/zones/{zoneId}/disassociate-vpc` | Remove private zone association. |

### 12.3 Load Balancing

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/load-balancers` | List LBs. |
| POST | `/load-balancers` | Create LB. |
| GET | `/load-balancers/{lbId}` | Describe LB. |
| PATCH | `/load-balancers/{lbId}` | Update LB. |
| DELETE | `/load-balancers/{lbId}` | Delete LB. |
| GET | `/target-groups` | List target groups. |
| POST | `/target-groups` | Create target group. |
| GET | `/target-groups/{targetGroupId}` | Describe target group. |
| PATCH | `/target-groups/{targetGroupId}` | Update target group. |
| DELETE | `/target-groups/{targetGroupId}` | Delete target group. |
| POST | `/target-groups/{targetGroupId}/targets` | Register target. |
| DELETE | `/target-groups/{targetGroupId}/targets/{targetId}` | Deregister target. |
| GET | `/listeners` | List listeners. |
| POST | `/listeners` | Create listener. |
| PATCH | `/listeners/{listenerId}` | Update listener. |
| DELETE | `/listeners/{listenerId}` | Delete listener. |

### 12.4 Ingress/WAF/Reachability

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/ingress` | List ingress rules. |
| POST | `/ingress` | Create ingress rule. |
| GET | `/ingress/{id}` | Describe ingress. |
| PATCH | `/ingress/{id}` | Update ingress. |
| DELETE | `/ingress/{id}` | Delete ingress. |
| GET | `/waf/policies` | List WAF policies. |
| POST | `/waf/policies` | Create WAF policy. |
| GET | `/waf/policies/{id}` | Describe WAF policy. |
| PATCH | `/waf/policies/{id}` | Update WAF policy. |
| DELETE | `/waf/policies/{id}` | Delete WAF policy. |
| POST | `/reachability/analyze` | Analyze path. |

## 13. Backend Requirements

### 13.1 Required Packages

| Package | Responsibility |
| --- | --- |
| `internal/networking` | High-level networking service facade. |
| `internal/vpc` | VPC resources. |
| `internal/ipam` | IP allocation. |
| `internal/dns` | Zones/records/private DNS. |
| `internal/lb` | Load balancing. |
| `internal/ingress` | HTTP ingress/static site/WAF. |
| `internal/certificates` | TLS lifecycle. |
| `internal/firewall` | SG/NACL compilation. |
| `internal/network` | Linux dataplane primitives. |
| `internal/resourcemon` | Inventory/metrics/drift/events. |
| `internal/audit` | Mutating action audit. |
| `internal/quotas` | Limits. |

### 13.2 Dataplane Backends

Minimum required local backend:

- Linux bridge or equivalent per VPC/subnet.
- Network namespaces for instances.
- veth/tap attachment.
- nftables for firewall/NAT.
- local DNS resolver integration.
- IP forwarding and route management.
- optional HAProxy/Envoy/nginx for load balancing/ingress.

Future backend abstraction:

- Linux native.
- OVS.
- eBPF/Cilium-like.
- External appliance.
- Cloud/remote region adapter.

### 13.3 Transactions and Rollback

Network mutations must be transactional at the control-plane level:

1. Validate desired change.
2. Write desired state transaction.
3. Apply dataplane change.
4. Verify observed state.
5. Mark in sync or drifted.
6. Emit event/audit.

If dataplane apply fails:

- Preserve desired config.
- Mark resource `error` or `drifted`.
- Record failure reason.
- Attempt rollback if safe.
- Alert if production resource affected.

## 14. WebUI Requirements

Add top-level WebUI section: **Networking**.

Subsections:

- VPCs
- Subnets
- Route Tables
- Security Groups
- Network ACLs
- Elastic/Public IPs
- Load Balancers
- Target Groups
- DNS
- Ingress
- WAF
- Certificates
- VPC Endpoints
- Peering / Transit
- Flow Logs
- Reachability Analyzer
- Network Events

### 14.1 Network Dashboard

Must show:

- Total VPCs
- Total subnets
- Public/private subnet split
- Public IP pool usage
- NAT gateway health
- Load balancer health
- DNS zone count
- Flow log status
- Security warnings
- Drifted resources
- Recent network events

### 14.2 Topology View

Visual graph must show:

- VPCs
- Subnets by zone
- Instances/ENIs
- Route tables
- Gateways
- NAT gateways
- Load balancers
- Public IPs
- DNS targets
- Endpoints and peerings

### 14.3 Consistency Rule

Every WebUI page must be generated or validated from the API schema. Any field displayed in WebUI must exist in the API response. Any creation/edit form must submit only documented API fields.

## 15. SDK and CLI Requirements

SDK groups:

- Networking
- IPAM
- DNS
- LoadBalancers
- TargetGroups
- Listeners
- Ingress
- WAF
- Certificates
- Reachability
- FlowLogs

CLI groups:

- `capper network dashboard`
- `capper ip-pool`
- `capper public-ip`
- `capper dns`
- `capper lb`
- `capper target-group`
- `capper listener`
- `capper ingress`
- `capper waf`
- `capper cert`
- `capper reachability`
- `capper flow-log`

All CLI commands must call API endpoints unless explicitly marked as local recovery/admin tooling.

## 16. Observability Requirements

Resource Monitor must ingest:

Resource types:

- vpc
- subnet
- route-table
- route
- security-group
- network-acl
- eni
- public-ip
- ip-pool
- internet-gateway
- nat-gateway
- dns-zone
- dns-record
- load-balancer
- target-group
- listener
- ingress
- waf-policy
- certificate-binding
- vpc-endpoint
- peering
- flow-log

Metrics:

- IP pool utilization
- subnet utilization
- dropped packets by SG/NACL
- NAT connection count
- LB request count
- LB error count
- target health count
- DNS query count
- flow log delivery errors
- route blackhole count
- endpoint health

Alerts:

- Public IP pool nearly exhausted.
- Subnet nearly exhausted.
- NAT gateway down.
- Load balancer has no healthy targets.
- DNS zone misconfigured.
- Certificate expiring.
- Flow logs failing.
- Route blackhole exists.
- Security rule overly permissive.
- Dataplane drift detected.

## 17. IAM and Policy Actions

Required action groups:

- `network:*`
- `ipam:*`
- `dns:*`
- `load-balancer:*`
- `target-group:*`
- `listener:*`
- `ingress:*`
- `waf:*`
- `certificate:*`
- `flow-log:*`
- `reachability:*`
- `vpc-endpoint:*`
- `vpc-peering:*`

Managed policies:

- CapperNetworkReadOnly
- CapperNetworkOperator
- CapperNetworkAdministrator
- CapperDNSOperator
- CapperLoadBalancerOperator
- CapperSecurityGroupOperator
- CapperIPAMAdministrator

## 18. Testing Requirements

Unit tests:

- IPAM allocations.
- Public IP association constraints.
- DNS record validation.
- Load balancer target validation.
- Listener rule priority.
- WAF match behavior.
- SG/NACL compilation model.
- Route conflict/blackhole behavior.
- Reachability analyzer decisions.

Integration tests:

- Create VPC and full network stack.
- Launch instance in public subnet.
- Launch instance in private subnet with NAT egress.
- Associate public IP to ENI.
- Create public load balancer with target group.
- Create private load balancer.
- Create DNS ALIAS to LB.
- Create ingress with TLS certificate.
- Verify WAF block rule.
- Verify Resource Monitor sync.
- Verify WebUI forms submit valid API payloads.

E2E tests:

- WebUI creates production network stack.
- WebUI launches instance behind load balancer.
- Reachability analyzer explains blocked traffic.
- Delete flow shows full dependency graph and cascade options.

## 19. Migration Plan

Phase 1: Introduce canonical Networking API and resource names while maintaining existing `/networks`, `/lb`, `/dns`, `/ingress`, `/firewalls`, `/ip-pools`, and `/ips` compatibility.

Phase 2: Normalize existing Network objects into VPC subnets or legacy simple networks.

Phase 3: Move load balancer and ingress models onto VPC/subnet/target group/listener concepts.

Phase 4: Move firewall model onto security groups and NACLs.

Phase 5: Update WebUI Networking section.

Phase 6: Update SDK/CLI/generated docs.

Phase 7: Add reachability analyzer and topology graph.

Phase 8: Add full drift detection and dataplane reconciliation.

## 20. Acceptance Criteria

This redesign is complete when:

- Networking has one coherent AWS-like model across VPC, instances, IPAM, DNS, LB, ingress, security, and endpoints.
- WebUI/API/SDK/CLI expose the same fields and actions.
- Instances use ENIs, subnets, security groups, NACLs, route tables, and IPAM.
- Public and private load balancers work through explicit subnet and IP configuration.
- DNS can target Capper resources cleanly.
- Reachability analyzer explains allowed/blocked traffic.
- Resource Monitor sees every network resource with health, drift, metrics, and events.
- Generated docs and tests prevent drift.
