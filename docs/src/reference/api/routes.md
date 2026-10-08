---
title: "API reference — all routes"
description: "Every /api/v1 route, grouped by resource. Generated from source."
owner: "docs"
status: "stable"
reviewed: "2026-06-16"
outputs:
  - markdown
  - web
  - pdf
---

# API reference — all routes

> Generated from `internal/api` route registrations by `make docs-api`. Do not edit by hand.

All routes are under `/api/v1` and require [authentication](overview.md) unless listed as public there. Responses use the [standard envelope](overview.md#response-envelope). This deployment registers **542** routes across **84** groups.

## Groups

- [`accounts`](#accounts) — 31 routes
- [`admin`](#admin) — 33 routes
- [`ai`](#ai) — 6 routes
- [`alerts`](#alerts) — 7 routes
- [`auth`](#auth) — 5 routes
- [`autoscale`](#autoscale) — 5 routes
- [`backup-policies`](#backup-policies) — 3 routes
- [`backups`](#backups) — 3 routes
- [`capinit`](#capinit) — 7 routes
- [`capsule-types`](#capsule-types) — 6 routes
- [`certificates`](#certificates) — 15 routes
- [`certs`](#certs) — 3 routes
- [`config`](#config) — 1 routes
- [`csd`](#csd) — 13 routes
- [`daemon`](#daemon) — 1 routes
- [`databases`](#databases) — 4 routes
- [`db`](#db) — 1 routes
- [`dns`](#dns) — 10 routes
- [`events`](#events) — 1 routes
- [`factory`](#factory) — 7 routes
- [`firewalls`](#firewalls) — 8 routes
- [`flow-logs`](#flow-logs) — 2 routes
- [`functions`](#functions) — 12 routes
- [`governance`](#governance) — 3 routes
- [`gpu`](#gpu) — 5 routes
- [`groups`](#groups) — 11 routes
- [`health`](#health) — 1 routes
- [`health-checks`](#health-checks) — 2 routes
- [`iam`](#iam) — 16 routes
- [`images`](#images) — 11 routes
- [`ingress`](#ingress) — 3 routes
- [`instance-disk-capacity`](#instance-disk-capacity) — 1 routes
- [`instance-types`](#instance-types) — 2 routes
- [`instances`](#instances) — 22 routes
- [`internet-gateways`](#internet-gateways) — 3 routes
- [`ip-pools`](#ip-pools) — 4 routes
- [`ips`](#ips) — 6 routes
- [`join-tokens`](#join-tokens) — 3 routes
- [`key-pairs`](#key-pairs) — 4 routes
- [`kms`](#kms) — 6 routes
- [`launch-templates`](#launch-templates) — 5 routes
- [`lb`](#lb) — 10 routes
- [`load-balancers`](#load-balancers) — 1 routes
- [`marketplace`](#marketplace) — 7 routes
- [`mcp`](#mcp) — 11 routes
- [`metrics`](#metrics) — 3 routes
- [`migrations`](#migrations) — 3 routes
- [`nat-gateways`](#nat-gateways) — 4 routes
- [`network-acls`](#network-acls) — 6 routes
- [`network-interfaces`](#network-interfaces) — 7 routes
- [`networking`](#networking) — 3 routes
- [`node-pools`](#node-pools) — 8 routes
- [`nodes`](#nodes) — 16 routes
- [`openapi.json`](#openapi.json) — 1 routes
- [`orgs`](#orgs) — 22 routes
- [`placement`](#placement) — 4 routes
- [`posture`](#posture) — 2 routes
- [`public-ips`](#public-ips) — 5 routes
- [`queues`](#queues) — 5 routes
- [`quotas`](#quotas) — 2 routes
- [`reachability`](#reachability) — 1 routes
- [`realms`](#realms) — 5 routes
- [`regions`](#regions) — 9 routes
- [`resource-events`](#resource-events) — 2 routes
- [`resources`](#resources) — 7 routes
- [`route-tables`](#route-tables) — 3 routes
- [`s3`](#s3) — 6 routes
- [`scheduler`](#scheduler) — 3 routes
- [`search`](#search) — 1 routes
- [`secrets`](#secrets) — 4 routes
- [`security-group-rules`](#security-group-rules) — 1 routes
- [`security-groups`](#security-groups) — 5 routes
- [`service-nodes`](#service-nodes) — 2 routes
- [`stacks`](#stacks) — 5 routes
- [`storage`](#storage) — 14 routes
- [`subnets`](#subnets) — 4 routes
- [`target-groups`](#target-groups) — 2 routes
- [`topology`](#topology) — 2 routes
- [`users`](#users) — 10 routes
- [`version`](#version) — 1 routes
- [`vpc-endpoints`](#vpc-endpoints) — 2 routes
- [`vpc-peerings`](#vpc-peerings) — 2 routes
- [`vpcs`](#vpcs) — 29 routes
- [`zones`](#zones) — 10 routes

## accounts

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/accounts/{account}/audit` |
| `GET` | `/api/v1/accounts/{account}/iam/groups` |
| `POST` | `/api/v1/accounts/{account}/iam/groups` |
| `GET` | `/api/v1/accounts/{account}/iam/groups/{groupId}` |
| `PATCH` | `/api/v1/accounts/{account}/iam/groups/{groupId}` |
| `DELETE` | `/api/v1/accounts/{account}/iam/groups/{id}` |
| `POST` | `/api/v1/accounts/{account}/iam/groups/{id}/members` |
| `DELETE` | `/api/v1/accounts/{account}/iam/groups/{id}/members/{userID}` |
| `GET` | `/api/v1/accounts/{account}/iam/policies` |
| `POST` | `/api/v1/accounts/{account}/iam/policies` |
| `DELETE` | `/api/v1/accounts/{account}/iam/policies/{id}` |
| `GET` | `/api/v1/accounts/{account}/iam/policies/{id}` |
| `PUT` | `/api/v1/accounts/{account}/iam/policies/{id}` |
| `POST` | `/api/v1/accounts/{account}/iam/policies/{id}/attach` |
| `POST` | `/api/v1/accounts/{account}/iam/policies/{id}/detach` |
| `GET` | `/api/v1/accounts/{account}/iam/roles` |
| `POST` | `/api/v1/accounts/{account}/iam/roles` |
| `DELETE` | `/api/v1/accounts/{account}/iam/roles/{id}` |
| `GET` | `/api/v1/accounts/{account}/iam/roles/{roleId}` |
| `PATCH` | `/api/v1/accounts/{account}/iam/roles/{roleId}` |
| `POST` | `/api/v1/accounts/{account}/iam/roles/{roleId}/assume` |
| `GET` | `/api/v1/accounts/{account}/iam/service-accounts` |
| `POST` | `/api/v1/accounts/{account}/iam/service-accounts` |
| `DELETE` | `/api/v1/accounts/{account}/iam/service-accounts/{id}` |
| `POST` | `/api/v1/accounts/{account}/iam/service-accounts/{id}/tokens` |
| `POST` | `/api/v1/accounts/{account}/iam/simulate` |
| `GET` | `/api/v1/accounts/{account}/iam/users` |
| `POST` | `/api/v1/accounts/{account}/iam/users` |
| `DELETE` | `/api/v1/accounts/{account}/iam/users/{id}` |
| `GET` | `/api/v1/accounts/{account}/iam/users/{userId}` |
| `PATCH` | `/api/v1/accounts/{account}/iam/users/{userId}` |

## admin

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/admin/disks` |
| `GET` | `/api/v1/admin/fail2ban/allowlist` |
| `PUT` | `/api/v1/admin/fail2ban/allowlist` |
| `POST` | `/api/v1/admin/fail2ban/ban` |
| `GET` | `/api/v1/admin/fail2ban/blocklist` |
| `POST` | `/api/v1/admin/fail2ban/blocklist` |
| `DELETE` | `/api/v1/admin/fail2ban/blocklist/{id}` |
| `POST` | `/api/v1/admin/fail2ban/flush` |
| `POST` | `/api/v1/admin/fail2ban/reload` |
| `GET` | `/api/v1/admin/fail2ban/status` |
| `POST` | `/api/v1/admin/fail2ban/unban` |
| `POST` | `/api/v1/admin/fail2ban/unban-all` |
| `GET` | `/api/v1/admin/hostsec/nodes` |
| `GET` | `/api/v1/admin/ip-exclusions` |
| `POST` | `/api/v1/admin/ip-exclusions` |
| `DELETE` | `/api/v1/admin/ip-exclusions/{id}` |
| `GET` | `/api/v1/admin/limits/host` |
| `PUT` | `/api/v1/admin/limits/host` |
| `DELETE` | `/api/v1/admin/storage-allocations/{id}` |
| `GET` | `/api/v1/admin/storage-pools` |
| `POST` | `/api/v1/admin/storage-pools` |
| `DELETE` | `/api/v1/admin/storage-pools/{id}` |
| `GET` | `/api/v1/admin/storage-pools/{id}/allocations` |
| `POST` | `/api/v1/admin/storage-pools/{id}/allocations` |
| `GET` | `/api/v1/admin/storage/settings` |
| `PUT` | `/api/v1/admin/storage/settings` |
| `GET` | `/api/v1/admin/ufw/defaults` |
| `PUT` | `/api/v1/admin/ufw/defaults` |
| `POST` | `/api/v1/admin/ufw/disable` |
| `POST` | `/api/v1/admin/ufw/enable` |
| `POST` | `/api/v1/admin/ufw/rules` |
| `DELETE` | `/api/v1/admin/ufw/rules/{num}` |
| `GET` | `/api/v1/admin/ufw/status` |

## ai

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/ai/agents` |
| `POST` | `/api/v1/ai/agents` |
| `GET` | `/api/v1/ai/mcp` |
| `POST` | `/api/v1/ai/mcp` |
| `GET` | `/api/v1/ai/sessions` |
| `POST` | `/api/v1/ai/sessions` |

## alerts

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/alerts` |
| `GET` | `/api/v1/alerts/rules` |
| `POST` | `/api/v1/alerts/rules` |
| `DELETE` | `/api/v1/alerts/rules/{id}` |
| `PATCH` | `/api/v1/alerts/rules/{id}` |
| `POST` | `/api/v1/alerts/{id}/ack` |
| `POST` | `/api/v1/alerts/{id}/resolve` |

## auth

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/auth/google/callback` |
| `POST` | `/api/v1/auth/login` |
| `DELETE` | `/api/v1/auth/session` |
| `GET` | `/api/v1/auth/session` |
| `POST` | `/api/v1/auth/session` |

## autoscale

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/autoscale/policies` |
| `POST` | `/api/v1/autoscale/policies` |
| `DELETE` | `/api/v1/autoscale/policies/{policy}` |
| `GET` | `/api/v1/autoscale/policies/{policy}` |
| `PATCH` | `/api/v1/autoscale/policies/{policy}` |

## backup-policies

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/backup-policies` |
| `POST` | `/api/v1/backup-policies` |
| `DELETE` | `/api/v1/backup-policies/{name}` |

## backups

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/backups` |
| `POST` | `/api/v1/backups` |
| `POST` | `/api/v1/backups/{id}/restore` |

## capinit

| Method | Path |
| --- | --- |
| `POST` | `/api/v1/capinit/render` |
| `GET` | `/api/v1/capinit/status` |
| `GET` | `/api/v1/capinit/templates` |
| `POST` | `/api/v1/capinit/templates` |
| `DELETE` | `/api/v1/capinit/templates/{id}` |
| `GET` | `/api/v1/capinit/templates/{id}` |
| `PUT` | `/api/v1/capinit/templates/{id}` |

## capsule-types

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/capsule-types` |
| `POST` | `/api/v1/capsule-types` |
| `DELETE` | `/api/v1/capsule-types/{name}` |
| `GET` | `/api/v1/capsule-types/{name}` |
| `GET` | `/api/v1/capsule-types/{name}/audit` |
| `POST` | `/api/v1/capsule-types/{name}/deprecate` |

## certificates

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/certificates` |
| `POST` | `/api/v1/certificates` |
| `GET` | `/api/v1/certificates/acme/accounts` |
| `POST` | `/api/v1/certificates/acme/accounts` |
| `DELETE` | `/api/v1/certificates/acme/accounts/{acmeAccount}` |
| `GET` | `/api/v1/certificates/acme/accounts/{acmeAccount}` |
| `DELETE` | `/api/v1/certificates/{cert}` |
| `GET` | `/api/v1/certificates/{cert}` |
| `GET` | `/api/v1/certificates/{cert}/bindings` |
| `POST` | `/api/v1/certificates/{cert}/bindings` |
| `DELETE` | `/api/v1/certificates/{cert}/bindings/{binding}` |
| `POST` | `/api/v1/certificates/{cert}/reissue` |
| `POST` | `/api/v1/certificates/{cert}/renew` |
| `POST` | `/api/v1/certificates/{cert}/revoke` |
| `GET` | `/api/v1/certificates/{id}/monitoring` |

## certs

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/certs` |
| `POST` | `/api/v1/certs` |
| `DELETE` | `/api/v1/certs/{name}` |

## config

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/config/drift` |

## csd

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/csd/volumes` |
| `POST` | `/api/v1/csd/volumes` |
| `DELETE` | `/api/v1/csd/volumes/{vol}` |
| `GET` | `/api/v1/csd/volumes/{vol}` |
| `POST` | `/api/v1/csd/volumes/{vol}/attach` |
| `GET` | `/api/v1/csd/volumes/{vol}/attachments` |
| `POST` | `/api/v1/csd/volumes/{vol}/detach` |
| `GET` | `/api/v1/csd/volumes/{vol}/leases` |
| `POST` | `/api/v1/csd/volumes/{vol}/leases/revoke` |
| `POST` | `/api/v1/csd/volumes/{vol}/repair` |
| `GET` | `/api/v1/csd/volumes/{vol}/replicas` |
| `GET` | `/api/v1/csd/volumes/{vol}/snapshots` |
| `POST` | `/api/v1/csd/volumes/{vol}/snapshots` |

## daemon

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/daemon/status` |

## databases

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/databases` |
| `POST` | `/api/v1/databases` |
| `DELETE` | `/api/v1/databases/{name}` |
| `GET` | `/api/v1/databases/{name}` |

## db

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/db/stats` |

## dns

| Method | Path |
| --- | --- |
| `POST` | `/api/v1/dns/query` |
| `GET` | `/api/v1/dns/zones` |
| `POST` | `/api/v1/dns/zones` |
| `DELETE` | `/api/v1/dns/zones/{zone}` |
| `GET` | `/api/v1/dns/zones/{zone}` |
| `POST` | `/api/v1/dns/zones/{zone}/records` |
| `DELETE` | `/api/v1/dns/zones/{zone}/records/{id}` |
| `DELETE` | `/api/v1/dns/zones/{zone}/vpc-associations` |
| `GET` | `/api/v1/dns/zones/{zone}/vpc-associations` |
| `POST` | `/api/v1/dns/zones/{zone}/vpc-associations` |

## events

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/events` |

## factory

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/factory/images` |
| `POST` | `/api/v1/factory/images/{id}/push` |
| `POST` | `/api/v1/factory/images/{id}/rescan` |
| `GET` | `/api/v1/factory/jobs` |
| `GET` | `/api/v1/factory/jobs/{id}` |
| `GET` | `/api/v1/factory/status` |
| `GET` | `/api/v1/factory/sync/status` |

## firewalls

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/firewalls` |
| `POST` | `/api/v1/firewalls` |
| `DELETE` | `/api/v1/firewalls/{name}` |
| `GET` | `/api/v1/firewalls/{name}` |
| `POST` | `/api/v1/firewalls/{name}/apply` |
| `GET` | `/api/v1/firewalls/{name}/rules` |
| `POST` | `/api/v1/firewalls/{name}/rules` |
| `DELETE` | `/api/v1/firewalls/{name}/rules/{id}` |

## flow-logs

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/flow-logs` |
| `POST` | `/api/v1/flow-logs` |

## functions

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/functions` |
| `POST` | `/api/v1/functions` |
| `DELETE` | `/api/v1/functions/{id}` |
| `GET` | `/api/v1/functions/{id}` |
| `PATCH` | `/api/v1/functions/{id}` |
| `GET` | `/api/v1/functions/{id}/invocations` |
| `POST` | `/api/v1/functions/{id}/invoke` |
| `GET` | `/api/v1/functions/{id}/triggers` |
| `POST` | `/api/v1/functions/{id}/triggers` |
| `DELETE` | `/api/v1/functions/{id}/triggers/{triggerId}` |
| `GET` | `/api/v1/functions/{id}/versions` |
| `POST` | `/api/v1/functions/{id}/versions` |

## governance

| Method | Path |
| --- | --- |
| `POST` | `/api/v1/governance/evaluate` |
| `GET` | `/api/v1/governance/policies` |
| `POST` | `/api/v1/governance/policies` |

## gpu

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/gpu` |
| `POST` | `/api/v1/gpu` |
| `DELETE` | `/api/v1/gpu/{id}` |
| `POST` | `/api/v1/gpu/{id}/assign` |
| `POST` | `/api/v1/gpu/{id}/release` |

## groups

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/groups` |
| `POST` | `/api/v1/groups` |
| `DELETE` | `/api/v1/groups/{name}` |
| `GET` | `/api/v1/groups/{name}` |
| `GET` | `/api/v1/groups/{name}/autoscale` |
| `GET` | `/api/v1/groups/{name}/autoscale/decisions` |
| `POST` | `/api/v1/groups/{name}/autoscale/disable` |
| `POST` | `/api/v1/groups/{name}/autoscale/evaluate` |
| `GET` | `/api/v1/groups/{name}/instances` |
| `POST` | `/api/v1/groups/{name}/reconcile` |
| `POST` | `/api/v1/groups/{name}/scale` |

## health

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/health` |

## health-checks

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/health-checks` |
| `GET` | `/api/v1/health-checks/{instanceId}` |

## iam

| Method | Path |
| --- | --- |
| `POST` | `/api/v1/iam/assume-role` |
| `GET` | `/api/v1/iam/audit` |
| `GET` | `/api/v1/iam/groups` |
| `POST` | `/api/v1/iam/groups` |
| `POST` | `/api/v1/iam/groups/{group}/members` |
| `DELETE` | `/api/v1/iam/groups/{group}/members/{user}` |
| `GET` | `/api/v1/iam/policies` |
| `POST` | `/api/v1/iam/policies` |
| `GET` | `/api/v1/iam/roles` |
| `POST` | `/api/v1/iam/roles` |
| `POST` | `/api/v1/iam/simulate` |
| `GET` | `/api/v1/iam/tokens` |
| `POST` | `/api/v1/iam/tokens` |
| `GET` | `/api/v1/iam/users` |
| `POST` | `/api/v1/iam/users` |
| `DELETE` | `/api/v1/iam/users/{name}` |

## images

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/images` |
| `POST` | `/api/v1/images/import` |
| `POST` | `/api/v1/images/upload` |
| `DELETE` | `/api/v1/images/{name}` |
| `GET` | `/api/v1/images/{name}` |
| `GET` | `/api/v1/images/{name}/provenance` |
| `POST` | `/api/v1/images/{name}/provenance` |
| `POST` | `/api/v1/images/{name}/publish` |
| `GET` | `/api/v1/images/{name}/sbom` |
| `POST` | `/api/v1/images/{name}/sbom` |
| `POST` | `/api/v1/images/{name}/scan` |

## ingress

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/ingress` |
| `POST` | `/api/v1/ingress` |
| `DELETE` | `/api/v1/ingress/{name}` |

## instance-disk-capacity

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/instance-disk-capacity` |

## instance-types

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/instance-types` |
| `GET` | `/api/v1/instance-types/{name}` |

## instances

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/instances` |
| `POST` | `/api/v1/instances` |
| `DELETE` | `/api/v1/instances/{id}` |
| `GET` | `/api/v1/instances/{id}` |
| `PATCH` | `/api/v1/instances/{id}` |
| `POST` | `/api/v1/instances/{id}/attach-network-interface` |
| `POST` | `/api/v1/instances/{id}/detach-network-interface` |
| `GET` | `/api/v1/instances/{id}/events` |
| `GET` | `/api/v1/instances/{id}/logs` |
| `GET` | `/api/v1/instances/{id}/logs/stderr` |
| `GET` | `/api/v1/instances/{id}/logs/stdout` |
| `GET` | `/api/v1/instances/{id}/metadata` |
| `PUT` | `/api/v1/instances/{id}/metadata` |
| `GET` | `/api/v1/instances/{id}/metadata/{tab}` |
| `GET` | `/api/v1/instances/{id}/monitoring` |
| `DELETE` | `/api/v1/instances/{id}/protect-termination` |
| `POST` | `/api/v1/instances/{id}/protect-termination` |
| `POST` | `/api/v1/instances/{id}/reboot` |
| `POST` | `/api/v1/instances/{id}/restart` |
| `POST` | `/api/v1/instances/{id}/start` |
| `POST` | `/api/v1/instances/{id}/stop` |
| `GET` | `/api/v1/instances/{id}/terminal` |

## internet-gateways

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/internet-gateways` |
| `POST` | `/api/v1/internet-gateways` |
| `DELETE` | `/api/v1/internet-gateways/{igwId}` |

## ip-pools

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/ip-pools` |
| `POST` | `/api/v1/ip-pools` |
| `DELETE` | `/api/v1/ip-pools/{id}` |
| `GET` | `/api/v1/ip-pools/{id}` |

## ips

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/ips` |
| `POST` | `/api/v1/ips/reserve` |
| `GET` | `/api/v1/ips/{id}` |
| `POST` | `/api/v1/ips/{id}/attach` |
| `POST` | `/api/v1/ips/{id}/detach` |
| `POST` | `/api/v1/ips/{id}/release` |

## join-tokens

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/join-tokens` |
| `POST` | `/api/v1/join-tokens` |
| `DELETE` | `/api/v1/join-tokens/{id}` |

## key-pairs

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/key-pairs` |
| `POST` | `/api/v1/key-pairs` |
| `DELETE` | `/api/v1/key-pairs/{keyName}` |
| `GET` | `/api/v1/key-pairs/{keyName}` |

## kms

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/kms/keys` |
| `POST` | `/api/v1/kms/keys` |
| `DELETE` | `/api/v1/kms/keys/{name}` |
| `POST` | `/api/v1/kms/keys/{name}/decrypt` |
| `POST` | `/api/v1/kms/keys/{name}/encrypt` |
| `POST` | `/api/v1/kms/keys/{name}/rotate` |

## launch-templates

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/launch-templates` |
| `POST` | `/api/v1/launch-templates` |
| `GET` | `/api/v1/launch-templates/{templateId}` |
| `GET` | `/api/v1/launch-templates/{templateId}/versions` |
| `POST` | `/api/v1/launch-templates/{templateId}/versions` |

## lb

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/lb` |
| `POST` | `/api/v1/lb` |
| `POST` | `/api/v1/lb/{lb}/certificates` |
| `DELETE` | `/api/v1/lb/{lb}/certificates/{cert}` |
| `DELETE` | `/api/v1/lb/{name}` |
| `GET` | `/api/v1/lb/{name}` |
| `POST` | `/api/v1/lb/{name}/backends` |
| `DELETE` | `/api/v1/lb/{name}/backends/{address}` |
| `GET` | `/api/v1/lb/{name}/listeners` |
| `POST` | `/api/v1/lb/{name}/listeners` |

## load-balancers

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/load-balancers/{id}/monitoring` |

## marketplace

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/marketplace/images` |
| `GET` | `/api/v1/marketplace/images/{id}` |
| `POST` | `/api/v1/marketplace/images/{id}/approve` |
| `POST` | `/api/v1/marketplace/images/{id}/install` |
| `POST` | `/api/v1/marketplace/images/{id}/quarantine` |
| `POST` | `/api/v1/marketplace/images/{id}/reject` |
| `GET` | `/api/v1/marketplace/images/{id}/scans` |

## mcp

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/mcp/approvals` |
| `POST` | `/api/v1/mcp/approvals/{id}/approve` |
| `POST` | `/api/v1/mcp/approvals/{id}/deny` |
| `GET` | `/api/v1/mcp/servers` |
| `POST` | `/api/v1/mcp/servers` |
| `DELETE` | `/api/v1/mcp/servers/{id}` |
| `GET` | `/api/v1/mcp/servers/{id}` |
| `GET` | `/api/v1/mcp/servers/{id}/invocations` |
| `GET` | `/api/v1/mcp/servers/{id}/tools` |
| `POST` | `/api/v1/mcp/servers/{id}/tools/sync` |
| `POST` | `/api/v1/mcp/servers/{id}/tools/{toolName}/invoke` |

## metrics

| Method | Path |
| --- | --- |
| `POST` | `/api/v1/metrics/custom` |
| `POST` | `/api/v1/metrics/ingest` |
| `GET` | `/api/v1/metrics/query` |

## migrations

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/migrations` |
| `POST` | `/api/v1/migrations` |
| `GET` | `/api/v1/migrations/{plan}` |

## nat-gateways

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/nat-gateways` |
| `POST` | `/api/v1/nat-gateways` |
| `DELETE` | `/api/v1/nat-gateways/{natId}` |
| `GET` | `/api/v1/nat-gateways/{natId}` |

## network-acls

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/network-acls` |
| `POST` | `/api/v1/network-acls` |
| `DELETE` | `/api/v1/network-acls/{aclId}` |
| `GET` | `/api/v1/network-acls/{aclId}` |
| `POST` | `/api/v1/network-acls/{aclId}/entries` |
| `DELETE` | `/api/v1/network-acls/{aclId}/entries/{ruleNumber}` |

## network-interfaces

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/network-interfaces` |
| `POST` | `/api/v1/network-interfaces` |
| `DELETE` | `/api/v1/network-interfaces/{eniId}` |
| `GET` | `/api/v1/network-interfaces/{eniId}` |
| `POST` | `/api/v1/network-interfaces/{eniId}/attach` |
| `POST` | `/api/v1/network-interfaces/{eniId}/detach` |
| `POST` | `/api/v1/network-interfaces/{eniId}/private-ips` |

## networking

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/networking/dashboard` |
| `GET` | `/api/v1/networking/drift` |
| `GET` | `/api/v1/networking/topology` |

## node-pools

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/node-pools` |
| `POST` | `/api/v1/node-pools` |
| `DELETE` | `/api/v1/node-pools/{pool}` |
| `GET` | `/api/v1/node-pools/{pool}` |
| `PATCH` | `/api/v1/node-pools/{pool}` |
| `GET` | `/api/v1/node-pools/{pool}/members` |
| `POST` | `/api/v1/node-pools/{pool}/members` |
| `DELETE` | `/api/v1/node-pools/{pool}/members/{nodeID}` |

## nodes

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/nodes` |
| `POST` | `/api/v1/nodes` |
| `POST` | `/api/v1/nodes/join` |
| `GET` | `/api/v1/nodes/{id}/monitoring` |
| `DELETE` | `/api/v1/nodes/{node}` |
| `GET` | `/api/v1/nodes/{node}` |
| `PATCH` | `/api/v1/nodes/{node}` |
| `POST` | `/api/v1/nodes/{node}/approve` |
| `POST` | `/api/v1/nodes/{node}/cordon` |
| `POST` | `/api/v1/nodes/{node}/drain` |
| `POST` | `/api/v1/nodes/{node}/heartbeat` |
| `POST` | `/api/v1/nodes/{node}/inventory` |
| `GET` | `/api/v1/nodes/{node}/services` |
| `POST` | `/api/v1/nodes/{node}/services` |
| `POST` | `/api/v1/nodes/{node}/uncordon` |
| `POST` | `/api/v1/nodes/{node}/undrain` |

## openapi.json

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/openapi.json` |

## orgs

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/orgs` |
| `POST` | `/api/v1/orgs` |
| `DELETE` | `/api/v1/orgs/{org}` |
| `GET` | `/api/v1/orgs/{org}` |
| `PATCH` | `/api/v1/orgs/{org}` |
| `GET` | `/api/v1/orgs/{org}/accounts` |
| `POST` | `/api/v1/orgs/{org}/accounts` |
| `DELETE` | `/api/v1/orgs/{org}/accounts/{account}` |
| `GET` | `/api/v1/orgs/{org}/accounts/{account}` |
| `PATCH` | `/api/v1/orgs/{org}/accounts/{account}` |
| `POST` | `/api/v1/orgs/{org}/accounts/{account}/reactivate` |
| `GET` | `/api/v1/orgs/{org}/accounts/{account}/root-users` |
| `POST` | `/api/v1/orgs/{org}/accounts/{account}/root-users` |
| `DELETE` | `/api/v1/orgs/{org}/accounts/{account}/root-users/{userID}` |
| `POST` | `/api/v1/orgs/{org}/accounts/{account}/suspend` |
| `GET` | `/api/v1/orgs/{org}/guardrails` |
| `POST` | `/api/v1/orgs/{org}/guardrails` |
| `DELETE` | `/api/v1/orgs/{org}/guardrails/{id}` |
| `GET` | `/api/v1/orgs/{org}/guardrails/{id}` |
| `GET` | `/api/v1/orgs/{org}/root-users` |
| `POST` | `/api/v1/orgs/{org}/root-users` |
| `DELETE` | `/api/v1/orgs/{org}/root-users/{userID}` |

## placement

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/placement/policies` |
| `POST` | `/api/v1/placement/policies` |
| `DELETE` | `/api/v1/placement/policies/{policy}` |
| `GET` | `/api/v1/placement/policies/{policy}` |

## posture

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/posture/findings` |
| `POST` | `/api/v1/posture/scan` |

## public-ips

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/public-ips` |
| `POST` | `/api/v1/public-ips/allocate` |
| `DELETE` | `/api/v1/public-ips/{allocationId}` |
| `POST` | `/api/v1/public-ips/{allocationId}/associate` |
| `POST` | `/api/v1/public-ips/{associationId}/disassociate` |

## queues

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/queues` |
| `POST` | `/api/v1/queues` |
| `DELETE` | `/api/v1/queues/{name}` |
| `POST` | `/api/v1/queues/{name}/consume` |
| `POST` | `/api/v1/queues/{name}/publish` |

## quotas

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/quotas` |
| `POST` | `/api/v1/quotas` |

## reachability

| Method | Path |
| --- | --- |
| `POST` | `/api/v1/reachability/analyze` |

## realms

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/realms` |
| `POST` | `/api/v1/realms` |
| `DELETE` | `/api/v1/realms/{realm}` |
| `GET` | `/api/v1/realms/{realm}` |
| `PATCH` | `/api/v1/realms/{realm}` |

## regions

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/regions` |
| `POST` | `/api/v1/regions` |
| `DELETE` | `/api/v1/regions/{region}` |
| `GET` | `/api/v1/regions/{region}` |
| `PATCH` | `/api/v1/regions/{region}` |
| `POST` | `/api/v1/regions/{region}/drain` |
| `POST` | `/api/v1/regions/{region}/evacuate` |
| `POST` | `/api/v1/regions/{region}/promote` |
| `POST` | `/api/v1/regions/{region}/undrain` |

## resource-events

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/resource-events` |
| `POST` | `/api/v1/resource-events` |

## resources

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/resources` |
| `POST` | `/api/v1/resources/sync` |
| `GET` | `/api/v1/resources/{id}` |
| `GET` | `/api/v1/resources/{id}/config` |
| `POST` | `/api/v1/resources/{id}/drift/repair` |
| `GET` | `/api/v1/resources/{id}/events` |
| `GET` | `/api/v1/resources/{id}/metrics` |

## route-tables

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/route-tables/{routeTableId}` |
| `POST` | `/api/v1/route-tables/{routeTableId}/routes` |
| `DELETE` | `/api/v1/route-tables/{routeTableId}/routes/{routeId}` |

## s3

| Method | Path |
| --- | --- |
| `DELETE` | `/api/v1/s3/buckets/{bucket}/policy` |
| `GET` | `/api/v1/s3/buckets/{bucket}/policy` |
| `PUT` | `/api/v1/s3/buckets/{bucket}/policy` |
| `GET` | `/api/v1/s3/credentials` |
| `POST` | `/api/v1/s3/credentials` |
| `DELETE` | `/api/v1/s3/credentials/{id}` |

## scheduler

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/scheduler/capacity` |
| `GET` | `/api/v1/scheduler/placements` |
| `POST` | `/api/v1/scheduler/simulate` |

## search

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/search` |

## secrets

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/secrets` |
| `POST` | `/api/v1/secrets` |
| `DELETE` | `/api/v1/secrets/{name}` |
| `GET` | `/api/v1/secrets/{name}` |

## security-group-rules

| Method | Path |
| --- | --- |
| `DELETE` | `/api/v1/security-group-rules/{ruleId}` |

## security-groups

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/security-groups` |
| `POST` | `/api/v1/security-groups` |
| `DELETE` | `/api/v1/security-groups/{sgId}` |
| `GET` | `/api/v1/security-groups/{sgId}` |
| `POST` | `/api/v1/security-groups/{sgId}/rules` |

## service-nodes

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/service-nodes` |
| `GET` | `/api/v1/service-nodes/{role}` |

## stacks

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/stacks` |
| `POST` | `/api/v1/stacks` |
| `DELETE` | `/api/v1/stacks/{name}` |
| `GET` | `/api/v1/stacks/{name}` |
| `POST` | `/api/v1/stacks/{name}/diff` |

## storage

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/storage/buckets` |
| `POST` | `/api/v1/storage/buckets` |
| `DELETE` | `/api/v1/storage/buckets/{bucket}` |
| `GET` | `/api/v1/storage/buckets/{bucket}` |
| `GET` | `/api/v1/storage/buckets/{bucket}/objects` |
| `DELETE` | `/api/v1/storage/buckets/{bucket}/objects/{key...}` |
| `GET` | `/api/v1/storage/buckets/{bucket}/objects/{key...}` |
| `POST` | `/api/v1/storage/buckets/{bucket}/objects/{key...}` |
| `PUT` | `/api/v1/storage/buckets/{bucket}/objects/{key...}` |
| `GET` | `/api/v1/storage/volumes` |
| `POST` | `/api/v1/storage/volumes` |
| `DELETE` | `/api/v1/storage/volumes/{name}` |
| `POST` | `/api/v1/storage/volumes/{name}/attach` |
| `POST` | `/api/v1/storage/volumes/{name}/detach` |

## subnets

| Method | Path |
| --- | --- |
| `DELETE` | `/api/v1/subnets/{subnetId}` |
| `GET` | `/api/v1/subnets/{subnetId}` |
| `PATCH` | `/api/v1/subnets/{subnetId}` |
| `POST` | `/api/v1/subnets/{subnetId}/associate-route-table` |

## target-groups

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/target-groups` |
| `POST` | `/api/v1/target-groups` |

## topology

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/topology/health` |
| `POST` | `/api/v1/topology/health` |

## users

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/users` |
| `POST` | `/api/v1/users` |
| `GET` | `/api/v1/users/me` |
| `PATCH` | `/api/v1/users/me` |
| `POST` | `/api/v1/users/me/password` |
| `POST` | `/api/v1/users/{id}/approve` |
| `POST` | `/api/v1/users/{id}/disable` |
| `POST` | `/api/v1/users/{id}/password` |
| `POST` | `/api/v1/users/{id}/roles` |
| `DELETE` | `/api/v1/users/{id}/roles/{role}` |

## version

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/version` |

## vpc-endpoints

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/vpc-endpoints` |
| `POST` | `/api/v1/vpc-endpoints` |

## vpc-peerings

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/vpc-peerings` |
| `POST` | `/api/v1/vpc-peerings` |

## vpcs

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/vpcs` |
| `POST` | `/api/v1/vpcs` |
| `DELETE` | `/api/v1/vpcs/{vpc}` |
| `GET` | `/api/v1/vpcs/{vpc}` |
| `PATCH` | `/api/v1/vpcs/{vpc}` |
| `POST` | `/api/v1/vpcs/{vpc}/copy` |
| `GET` | `/api/v1/vpcs/{vpc}/dependencies` |
| `GET` | `/api/v1/vpcs/{vpc}/mobility/jobs` |
| `GET` | `/api/v1/vpcs/{vpc}/mobility/jobs/{job}` |
| `POST` | `/api/v1/vpcs/{vpc}/mobility/jobs/{job}/cancel` |
| `POST` | `/api/v1/vpcs/{vpc}/mobility/jobs/{job}/cutover` |
| `GET` | `/api/v1/vpcs/{vpc}/mobility/jobs/{job}/mappings` |
| `POST` | `/api/v1/vpcs/{vpc}/mobility/jobs/{job}/rollback` |
| `GET` | `/api/v1/vpcs/{vpc}/mobility/jobs/{job}/steps` |
| `GET` | `/api/v1/vpcs/{vpc}/mobility/plans` |
| `POST` | `/api/v1/vpcs/{vpc}/mobility/plans` |
| `GET` | `/api/v1/vpcs/{vpc}/mobility/plans/{plan}` |
| `POST` | `/api/v1/vpcs/{vpc}/mobility/plans/{plan}/approve` |
| `POST` | `/api/v1/vpcs/{vpc}/mobility/plans/{plan}/cancel` |
| `GET` | `/api/v1/vpcs/{vpc}/mobility/plans/{plan}/dry-run` |
| `POST` | `/api/v1/vpcs/{vpc}/mobility/plans/{plan}/execute` |
| `POST` | `/api/v1/vpcs/{vpc}/move` |
| `GET` | `/api/v1/vpcs/{vpc}/route-tables` |
| `POST` | `/api/v1/vpcs/{vpc}/route-tables` |
| `GET` | `/api/v1/vpcs/{vpc}/routes` |
| `POST` | `/api/v1/vpcs/{vpc}/routes` |
| `GET` | `/api/v1/vpcs/{vpc}/subnets` |
| `POST` | `/api/v1/vpcs/{vpc}/subnets` |
| `GET` | `/api/v1/vpcs/{vpc}/summary` |

## zones

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/zones` |
| `POST` | `/api/v1/zones` |
| `DELETE` | `/api/v1/zones/{zone}` |
| `GET` | `/api/v1/zones/{zone}` |
| `PATCH` | `/api/v1/zones/{zone}` |
| `POST` | `/api/v1/zones/{zone}/cordon` |
| `POST` | `/api/v1/zones/{zone}/drain` |
| `POST` | `/api/v1/zones/{zone}/evacuate` |
| `POST` | `/api/v1/zones/{zone}/uncordon` |
| `POST` | `/api/v1/zones/{zone}/undrain` |

