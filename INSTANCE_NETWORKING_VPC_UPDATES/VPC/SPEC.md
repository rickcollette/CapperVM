# Capper VPC — AWS-Style Redesign SPEC

Status: Draft for implementation  
Owner: Capper Control Plane / Networking / WebUI  
Target: Backend, API, SDK, CLI, WebUI, Docs, Tests  
Non-goal: This is not code. This is the product and engineering contract.

## 1. Purpose

Capper VPC must become the primary private network boundary for an account/project, modeled much closer to AWS VPC than the current lightweight VPC/network split. A VPC is not just a CIDR record. It owns subnets, route tables, gateways, security groups, network ACLs, DHCP/DNS settings, endpoints, peerings, flow logs, and IP address management.

The WebUI, backend API, SDK, CLI, database, docs, tests, and resource monitor must expose the same model. No hidden backend-only fields. No WebUI-only concepts. No CLI-only shortcuts that create resources the API cannot describe.

## 2. Current Capper Baseline

The uploaded Capper snapshot already exposes VPC, network, instance, IPAM, resource monitor, mobility, topology, quotas, and SDK pieces. The SDK has VPC, region, zone, node, scheduler, instance, network, IPAM, resource monitor, ingress, firewall, storage, load balancer, and migration groups. The current VPC model includes ID, realm ID, project, slug, name, CIDR, status, home region, mobility policy, labels, created/updated timestamps. The current network model is simpler: ID, name, subnet, gateway, and project. The test/doc tooling already expects `/api/v1/vpcs`, `/api/v1/networks`, and `/api/v1/instances` to exist, and generated docs are meant to stay source-accurate.

## 3. AWS Reference Behavior to Match Conceptually

Capper should mimic the AWS VPC mental model, not necessarily every AWS internal implementation detail.

AWS VPC concepts to copy into Capper:

- VPCs have IPv4 CIDR blocks and optionally IPv6 CIDR blocks.
- Subnets live inside VPCs and are associated with one availability zone / Capper zone.
- Route tables determine where traffic goes, and each subnet is associated with exactly one effective route table.
- Internet gateways attach to VPCs and allow public routing when route tables send traffic to them.
- NAT gateways provide outbound internet access for private subnets.
- Security groups are virtual firewalls for instances / ENIs.
- Network ACLs are subnet-level allow/deny rule sets.
- Public IPs / Elastic IPs can be allocated, associated, disassociated, and released.
- VPC endpoints/private service endpoints provide private connectivity to platform services.
- VPC peering/transit connectivity is a first-class relationship, not an ad-hoc route.
- Flow logs and reachability analysis are first-class operational tools.

## 4. Product Model

### 4.1 VPC

A VPC is the isolated L3 network boundary for a project/account.

Required VPC fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | Stable resource ID, for example `vpc_...`. |
| orgId | yes | Owning organization. |
| accountId | yes | Owning account. |
| projectId | yes | Owning project/workspace. |
| realmId | yes | Capper realm. |
| homeRegionId | yes | Default region for resource placement. |
| name | yes | Human display name. |
| slug | yes | Unique slug within account/project. |
| description | no | User-facing description. |
| status | yes | `pending`, `available`, `updating`, `deleting`, `deleted`, `error`, `retired`. |
| stateReason | no | Last failure/state message. |
| primaryIpv4Cidr | yes | Primary IPv4 CIDR. |
| secondaryIpv4Cidrs | no | Additional IPv4 CIDRs. |
| ipv6Cidrs | no | IPv6 CIDRs. |
| tenancy | yes | `default`, `dedicated-node`, `pinned-pool`. |
| dnsSupport | yes | Whether Capper DNS resolver is enabled. |
| dnsHostnames | yes | Whether instances receive private DNS names. |
| defaultSecurityGroupId | yes | Default VPC security group. |
| defaultNetworkAclId | yes | Default network ACL. |
| mainRouteTableId | yes | Main route table. |
| dhcpOptionsId | no | DHCP option set. |
| enableFlowLogs | yes | Whether VPC flow logs are enabled. |
| flowLogTarget | no | Local file, object bucket, external collector, or observability stream. |
| mobilityPolicy | yes | `disabled`, `copy-only`, `planned-move`, `failover-ready`. |
| tags | no | AWS-style key/value tags. |
| labels | no | Capper internal labels. |
| createdBy | yes | Principal. |
| createdAt | yes | RFC3339 timestamp. |
| updatedAt | yes | RFC3339 timestamp. |
| deletedAt | no | Soft deletion timestamp. |

### 4.2 Subnet

A subnet is a zone-scoped CIDR slice of a VPC.

Required subnet fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `subnet_...`. |
| vpcId | yes | Parent VPC. |
| regionId | yes | Region. |
| zoneId | yes | Zone/failure domain. |
| name | yes | Display name. |
| cidr | yes | IPv4 CIDR within a VPC CIDR. |
| ipv6Cidr | no | Optional IPv6 CIDR. |
| subnetType | yes | `public`, `private`, `isolated`, `lb`, `service`, `storage`, `edge`. |
| routeTableId | yes | Effective route table association. |
| networkAclId | yes | Effective NACL association. |
| autoAssignPublicIp | yes | Whether new ENIs/instances receive public IP automatically. |
| mapPublicIpv6 | yes | Whether IPv6 addressing is automatic. |
| reservedIpRanges | no | Internal reservations for routers, DNS, gateways. |
| availableIpCount | yes | Derived count. |
| status | yes | Lifecycle state. |
| tags | no | User tags. |

Rules:

- A subnet CIDR must be fully contained by one VPC CIDR.
- Subnet CIDRs in the same VPC must not overlap.
- Every subnet must have exactly one effective route table.
- Every subnet must have exactly one effective network ACL.
- Public subnet means route table has default route to an internet gateway or equivalent public egress target.
- Private subnet means outbound default route may point to NAT but inbound public routing is absent.
- Isolated subnet means no default internet route.

### 4.3 Route Table

Route tables must be first-class resources.

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `rtb_...`. |
| vpcId | yes | Parent VPC. |
| name | yes | Display name. |
| isMain | yes | Whether this is the VPC main route table. |
| associations | yes | Subnet/gateway associations. |
| routes | yes | Ordered effective routes. |
| propagatingGateways | no | Future: VPN/transit propagation sources. |
| tags | no | User tags. |

Route fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | Stable route ID. |
| destination | yes | CIDR or prefix list ID. |
| targetType | yes | `local`, `internet-gateway`, `nat-gateway`, `eni`, `instance`, `vpc-peering`, `transit-gateway`, `endpoint`, `blackhole`. |
| targetId | yes | Target resource ID, except `local`. |
| origin | yes | `system`, `static`, `propagated`. |
| state | yes | `active`, `blackhole`, `pending`. |

Rules:

- Every VPC gets a main route table with local routes for all VPC CIDRs.
- Users may create custom route tables and associate them to subnets.
- A subnet may only be associated with one route table at a time.
- Deleting a route target must mark dependent routes `blackhole` or prevent deletion unless forced.

### 4.4 Internet Gateway

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `igw_...`. |
| name | yes | Display name. |
| accountId | yes | Owner. |
| attachedVpcId | no | One attached VPC at a time. |
| status | yes | `detached`, `attaching`, `attached`, `detaching`, `error`. |
| tags | no | User tags. |

Rules:

- A VPC may have at most one attached internet gateway unless the backend explicitly supports multi-gateway routing.
- Public subnet creation wizard may create/attach an internet gateway automatically.
- Deleting a VPC requires detaching/deleting the IGW or using cascade delete.

### 4.5 NAT Gateway

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `nat_...`. |
| vpcId | yes | Parent VPC. |
| subnetId | yes | Public subnet that hosts NAT. |
| connectivityType | yes | `public` or `private`. |
| allocationId | no | Elastic/public IP allocation for public NAT. |
| privateIp | yes | Private address in hosting subnet. |
| status | yes | Lifecycle state. |
| tags | no | User tags. |

Rules:

- Public NAT requires a public subnet and an allocated routable IP.
- Private NAT does not require a public IP and only routes to private connected networks.
- Route tables may target NAT gateways for `0.0.0.0/0` and selected CIDRs.

### 4.6 Security Group

Security groups are stateful instance/ENI firewalls.

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `sg_...`. |
| vpcId | yes | Parent VPC. |
| name | yes | Unique within VPC. |
| description | yes | Required human description. |
| ingressRules | yes | Inbound rule list. |
| egressRules | yes | Outbound rule list. |
| isDefault | yes | Default group marker. |
| tags | no | User tags. |

Rule fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | Stable rule ID. |
| direction | yes | `ingress` or `egress`. |
| protocol | yes | `tcp`, `udp`, `icmp`, `icmpv6`, `all`, or numeric protocol. |
| fromPort | no | Start port. |
| toPort | no | End port. |
| cidrIpv4 | no | IPv4 source/destination. |
| cidrIpv6 | no | IPv6 source/destination. |
| prefixListId | no | Managed prefix/service list. |
| referencedSecurityGroupId | no | SG-to-SG reference. |
| description | no | Rule description. |

Rules:

- Security groups are stateful.
- Default security group allows internal same-group traffic and all egress by default unless account policy overrides it.
- User security groups default to no ingress and all egress unless configured otherwise.
- Instances and ENIs may attach multiple security groups.

### 4.7 Network ACL

NACLs are stateless subnet-level firewalls.

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `acl_...`. |
| vpcId | yes | Parent VPC. |
| name | yes | Display name. |
| isDefault | yes | Default NACL marker. |
| entries | yes | Ordered rules. |
| associations | yes | Subnets using this ACL. |
| tags | no | User tags. |

Entry fields:

| Field | Required | Description |
| --- | --- | --- |
| ruleNumber | yes | Evaluation order. |
| direction | yes | `ingress` or `egress`. |
| action | yes | `allow` or `deny`. |
| protocol | yes | Protocol. |
| cidr | yes | Source/destination CIDR. |
| fromPort | no | Start port. |
| toPort | no | End port. |

Rules:

- NACLs are stateless.
- Lower rule number wins.
- Default final behavior is deny.
- Each subnet has exactly one NACL association.

### 4.8 DHCP Options

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `dopt_...`. |
| domainName | no | Search domain. |
| domainNameServers | yes | DNS servers. |
| ntpServers | no | NTP servers. |
| netbiosNameServers | no | Optional legacy. |
| tags | no | User tags. |

### 4.9 VPC Endpoint

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `vpce_...`. |
| vpcId | yes | Parent VPC. |
| serviceName | yes | Capper service name, for example `capper.s3`, `capper.registry`, `capper.kms`. |
| endpointType | yes | `gateway`, `interface`, `load-balancer-service`. |
| subnetIds | conditional | Required for interface endpoints. |
| routeTableIds | conditional | Required for gateway endpoints. |
| securityGroupIds | conditional | Interface endpoint security groups. |
| privateDnsEnabled | yes | Private DNS aliasing. |
| policyDocument | no | Resource policy. |
| status | yes | Lifecycle state. |

### 4.10 VPC Peering / Transit Attachment

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `pcx_...` or `tgwatt_...`. |
| requesterVpcId | yes | Requesting VPC. |
| accepterVpcId | yes | Peer VPC. |
| requesterAccountId | yes | Owner. |
| accepterAccountId | yes | Owner. |
| status | yes | `pending-acceptance`, `active`, `rejected`, `deleted`, `expired`. |
| allowDnsResolution | yes | DNS behavior. |
| allowCrossAccount | yes | Policy-controlled. |

Rules:

- CIDRs must not overlap.
- Routes must be explicit; peering alone does not route traffic.
- Cross-account peering requires IAM permission and acceptance.

### 4.11 Flow Logs

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `flog_...`. |
| resourceType | yes | `vpc`, `subnet`, `eni`. |
| resourceId | yes | Target resource. |
| trafficType | yes | `accepted`, `rejected`, `all`. |
| destinationType | yes | `local-file`, `object-bucket`, `resourcemon`, `syslog`, `otlp`. |
| destination | yes | Destination reference. |
| aggregationIntervalSeconds | yes | 60 or 600. |
| status | yes | Lifecycle state. |

## 5. API Surface

All endpoints are under `/api/v1`. All responses use the standard Capper envelope. Every list endpoint must support pagination, sorting, filtering by project/account, tags, state, and region/zone where applicable.

### 5.1 VPC Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/vpcs` | List VPCs. |
| POST | `/vpcs` | Create VPC. |
| GET | `/vpcs/{vpcId}` | Describe VPC. |
| PATCH | `/vpcs/{vpcId}` | Update mutable VPC fields. |
| DELETE | `/vpcs/{vpcId}` | Delete VPC. |
| POST | `/vpcs/{vpcId}/restore` | Restore soft-deleted VPC if possible. |
| GET | `/vpcs/{vpcId}/summary` | Full dashboard summary. |
| GET | `/vpcs/{vpcId}/dependencies` | Dependency graph for delete/move. |
| GET | `/vpcs/{vpcId}/reachability` | Reachability test input/output. |
| POST | `/vpcs/{vpcId}/reachability/analyze` | Run reachability analysis. |
| GET | `/vpcs/{vpcId}/flow-logs` | List flow logs for VPC. |
| POST | `/vpcs/{vpcId}/flow-logs` | Create flow log. |

### 5.2 Subnet Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/vpcs/{vpcId}/subnets` | List subnets. |
| POST | `/vpcs/{vpcId}/subnets` | Create subnet. |
| GET | `/subnets/{subnetId}` | Describe subnet. |
| PATCH | `/subnets/{subnetId}` | Update subnet settings. |
| DELETE | `/subnets/{subnetId}` | Delete subnet. |
| POST | `/subnets/{subnetId}/associate-route-table` | Associate route table. |
| POST | `/subnets/{subnetId}/associate-network-acl` | Associate NACL. |

### 5.3 Route Table Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/vpcs/{vpcId}/route-tables` | List route tables. |
| POST | `/vpcs/{vpcId}/route-tables` | Create route table. |
| GET | `/route-tables/{routeTableId}` | Describe route table. |
| PATCH | `/route-tables/{routeTableId}` | Update name/tags/main association. |
| DELETE | `/route-tables/{routeTableId}` | Delete route table. |
| POST | `/route-tables/{routeTableId}/routes` | Create route. |
| PATCH | `/route-tables/{routeTableId}/routes/{routeId}` | Replace route target. |
| DELETE | `/route-tables/{routeTableId}/routes/{routeId}` | Delete route. |
| POST | `/route-tables/{routeTableId}/associations` | Associate subnet/gateway. |
| DELETE | `/route-table-associations/{associationId}` | Disassociate. |

### 5.4 Gateway Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/internet-gateways` | List IGWs. |
| POST | `/internet-gateways` | Create IGW. |
| POST | `/internet-gateways/{igwId}/attach` | Attach to VPC. |
| POST | `/internet-gateways/{igwId}/detach` | Detach from VPC. |
| DELETE | `/internet-gateways/{igwId}` | Delete IGW. |
| GET | `/nat-gateways` | List NAT gateways. |
| POST | `/nat-gateways` | Create NAT gateway. |
| GET | `/nat-gateways/{natId}` | Describe NAT gateway. |
| DELETE | `/nat-gateways/{natId}` | Delete NAT gateway. |

### 5.5 Security Group / NACL Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/security-groups` | List security groups. |
| POST | `/security-groups` | Create security group. |
| GET | `/security-groups/{sgId}` | Describe security group. |
| PATCH | `/security-groups/{sgId}` | Update metadata. |
| DELETE | `/security-groups/{sgId}` | Delete security group. |
| POST | `/security-groups/{sgId}/rules` | Add ingress/egress rule. |
| DELETE | `/security-group-rules/{ruleId}` | Delete SG rule. |
| GET | `/network-acls` | List NACLs. |
| POST | `/network-acls` | Create NACL. |
| GET | `/network-acls/{aclId}` | Describe NACL. |
| DELETE | `/network-acls/{aclId}` | Delete NACL. |
| POST | `/network-acls/{aclId}/entries` | Add/replace entry. |
| DELETE | `/network-acls/{aclId}/entries/{ruleNumber}` | Delete entry. |

### 5.6 Endpoint / Peering / Flow Log Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/vpc-endpoints` | List endpoints. |
| POST | `/vpc-endpoints` | Create endpoint. |
| PATCH | `/vpc-endpoints/{endpointId}` | Update endpoint policy/settings. |
| DELETE | `/vpc-endpoints/{endpointId}` | Delete endpoint. |
| GET | `/vpc-peerings` | List peering connections. |
| POST | `/vpc-peerings` | Create peering request. |
| POST | `/vpc-peerings/{peeringId}/accept` | Accept peering. |
| POST | `/vpc-peerings/{peeringId}/reject` | Reject peering. |
| DELETE | `/vpc-peerings/{peeringId}` | Delete peering. |
| GET | `/flow-logs` | List flow logs. |
| POST | `/flow-logs` | Create flow log. |
| DELETE | `/flow-logs/{flowLogId}` | Delete flow log. |

## 6. Backend Requirements

### 6.1 Single Source of Truth

The backend store owns canonical resource state. API, SDK, CLI, WebUI, docs, and tests consume this state. Generated docs must continue to introspect live routes so route docs cannot drift from source registrations.

### 6.2 Required Backend Packages

Recommended package ownership:

| Package | Responsibility |
| --- | --- |
| `internal/vpc` | VPC, subnet, route table, gateway, SG, NACL, endpoint model and validation. |
| `internal/ipam` | Private/public IP allocation, CIDR validation, exclusions, Elastic IPs. |
| `internal/network` | Linux bridge/netns/tap/veth application and dataplane wiring. |
| `internal/firewall` | nftables/iptables security group and NACL compilation. |
| `internal/topology` | Realms, regions, zones, nodes, capacity placement. |
| `internal/resourcemon` | Inventory, metrics, config drift, events, alerts, flow logs. |
| `internal/vpcmover` | Copy/move/delete/failover planning and execution. |
| `internal/quotas` | Account/project quota enforcement. |
| `internal/audit` | All mutating action audit records. |

### 6.3 Datastore Tables

Required tables:

- `vpcs`
- `vpc_cidr_blocks`
- `subnets`
- `route_tables`
- `route_table_associations`
- `routes`
- `internet_gateways`
- `nat_gateways`
- `security_groups`
- `security_group_rules`
- `network_acls`
- `network_acl_entries`
- `dhcp_options`
- `vpc_endpoints`
- `vpc_peerings`
- `flow_logs`
- `enis`
- `private_ips`
- `public_ips`
- `vpc_events`

Every table must include account/project ownership fields, created/updated timestamps, deletion state, and tags where relevant.

### 6.4 Validation Rules

Create/update validation must include:

- CIDR syntax and containment.
- CIDR overlap prevention.
- Subnet zone validity.
- Route target existence.
- Gateway attachment state.
- Security group reference validity.
- NACL rule number uniqueness by direction.
- No deletion of resources with dependents unless cascade is requested.
- Quota check before create.
- IAM authorization before every action.
- Audit record after every mutating action.

## 7. WebUI Requirements

The VPC WebUI must feel like a cloud console, not a low-level network table.

### 7.1 Navigation

Add top-level WebUI section: **Networking > VPCs**.

Pages:

- VPC list
- Create VPC wizard
- VPC detail dashboard
- Subnets tab
- Route tables tab
- Internet gateways tab
- NAT gateways tab
- Security groups tab
- Network ACLs tab
- Endpoints tab
- Peering tab
- IP addresses tab
- Flow logs tab
- Reachability analyzer tab
- Events / drift / audit tab
- Delete / mobility tab

### 7.2 Create VPC Wizard

Wizard modes:

1. Quick create
2. Production multi-zone
3. Custom

Quick create must offer:

- VPC name
- IPv4 CIDR
- Region
- Zones to use
- Public/private subnet count
- NAT gateway option: none, one per VPC, one per zone
- Internet gateway yes/no
- DNS support yes/no
- Flow logs yes/no
- Tags

Production multi-zone must default to:

- At least two zones where available.
- Public + private subnets per zone.
- Internet gateway enabled.
- NAT gateway per zone or clear warning if using single NAT.
- Flow logs enabled.
- Default deny-ish security posture, not wide-open inbound.

### 7.3 VPC Detail Dashboard

Must show:

- CIDRs
- Region/zone distribution
- Subnet map
- Route table summary
- Gateway status
- NAT status
- Security group count
- NACL count
- Instance count
- ENI count
- Public IP count
- Flow log status
- Health and drift status
- Recent events
- Dependency graph

### 7.4 UX Rules

- Any resource created by wizard must be visible as a normal resource afterward.
- Every form field must map to an API field.
- Every API enum must be represented in UI constants generated from shared schema or validated by integration tests.
- Destructive actions must show dependencies before confirmation.
- Route edits must preview impact.
- Security group edits must validate dangerous rules and warn on `0.0.0.0/0` ingress.

## 8. SDK and CLI Requirements

SDK clients required:

- VPCs
- Subnets
- RouteTables
- InternetGateways
- NatGateways
- SecurityGroups
- NetworkACLs
- VpcEndpoints
- VpcPeerings
- FlowLogs
- Reachability

CLI groups required:

- `capper vpc`
- `capper subnet`
- `capper route-table`
- `capper igw`
- `capper nat-gateway`
- `capper security-group`
- `capper network-acl`
- `capper vpc-endpoint`
- `capper vpc-peering`
- `capper flow-log`

CLI must use the same API as WebUI, not direct store calls except in admin/offline recovery tools.

## 9. Observability Requirements

Every VPC resource must project into Resource Monitor with:

- resource type
- health
- status
- region/zone/node where applicable
- tags
- desired config hash
- observed config hash
- drift status
- metrics
- events

Metrics:

- IP utilization per subnet
- route blackhole count
- NAT active connections
- gateway status
- SG/NACL rule count
- flow log dropped write count
- endpoint health
- peering state

Events:

- create/update/delete
- route changed
- gateway attached/detached
- NAT gateway failed
- security group rule changed
- NACL rule changed
- flow log delivery failed
- CIDR association changed
- drift detected

## 10. IAM and Policy Actions

Required actions:

- `vpc:create`, `vpc:list`, `vpc:get`, `vpc:update`, `vpc:delete`, `vpc:restore`
- `subnet:*`
- `route-table:*`
- `internet-gateway:*`
- `nat-gateway:*`
- `security-group:*`
- `network-acl:*`
- `vpc-endpoint:*`
- `vpc-peering:*`
- `flow-log:*`
- `reachability:analyze`
- `ipam:allocate`, `ipam:associate`, `ipam:release`

Managed policies must include read-only, network operator, network admin, and VPC mobility operator variants.

## 11. Testing Requirements

Unit tests:

- CIDR containment and overlap.
- Route target validation.
- Subnet route association.
- Security group rule validation.
- NACL rule evaluation ordering.
- Gateway lifecycle.
- NAT gateway constraints.
- Endpoint policy validation.
- Peering overlap rejection.
- Deletion dependency behavior.

Integration tests:

- Create production VPC wizard payload through API.
- Create VPC, subnets, route tables, IGW, NAT, SG, NACL, endpoint.
- Launch instance into subnet with security groups.
- Verify instance gets private IP, route, DNS name, metadata, and ENI.
- Verify public subnet + public IP reaches expected ingress path.
- Verify private subnet egress through NAT.
- Verify NACL deny overrides path.
- Verify Resource Monitor sync sees all VPC resources.
- Verify WebUI form payloads match API schema.

E2E tests:

- Full WebUI create VPC wizard.
- Edit route table.
- Add security group rule.
- Create private instance behind load balancer.
- Delete VPC with dependency graph and cascade confirmation.

## 12. Migration Plan

Phase 1: Add canonical VPC schema and APIs while preserving existing `/vpcs` and `/networks` compatibility.

Phase 2: Convert existing Network objects into Subnets or legacy bridge networks under a compatibility adapter.

Phase 3: Update instance launch to require VPC/subnet/ENI semantics while still accepting old `network` fields through translation.

Phase 4: Update WebUI to use new VPC model exclusively.

Phase 5: Deprecate old flat network creation path or re-scope it as `legacy networks` / `simple networks`.

Phase 6: Add VPC mobility support for full topology copy using the new first-class resources.

## 13. Acceptance Criteria

This redesign is complete when:

- A user can create a multi-zone VPC with public/private subnets from WebUI.
- The same VPC can be fully described from API, SDK, CLI, and docs.
- Instances launch into subnets through ENIs.
- Security groups and NACLs are enforceable by the dataplane.
- Route tables determine subnet behavior.
- IGW/NAT/public IP behavior is explicit and inspectable.
- Resource Monitor shows health, drift, events, and metrics for every VPC subresource.
- Generated API/CLI docs include all VPC-related routes/commands.
- Tests prove WebUI/API/SDK/CLI stay aligned.
