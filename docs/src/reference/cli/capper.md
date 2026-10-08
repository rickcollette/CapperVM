---
title: "CLI reference"
description: "Complete capper command tree — every command, subcommand, and flag. Generated from source."
owner: "docs"
status: "stable"
reviewed: "2026-06-13"
outputs:
  - markdown
  - web
  - pdf
---

# CLI reference

> Generated from the `capper` command tree by `make docs-cli`. Do not edit by hand.

Run `capper <command> --help` for the same information at the terminal. Global persistent flags apply to every command.

## Global flags

| Flag | Default | Description |
| --- | --- | --- |
| `--debug` | — | enable debug logging |
| `--json` | — | emit JSON output when applicable |
| `--project` | `default` | project namespace for resources |
| `--runtime` | `auto` | runtime backend: auto, bwrap, chroot, crun, runc, lxc, or qemu |
| `--store` | — | Capper store path |

## Commands

- [`ai`](#capper-ai) — manage AI agents, sessions, and MCP servers
- [`aio`](#capper-aio) — All-in-one single-node Capper management
- [`alert`](#capper-alert) — manage alert rules and evaluate firing alerts
- [`alerts`](#capper-alerts) — resource monitor alerts
- [`api`](#capper-api) — Capper REST API and web console
- [`attest`](#capper-attest) — generate SBOM or provenance for a .cap image
- [`backup`](#capper-backup) — manage backups and backup policies
- [`bottle`](#capper-bottle) — manage Capper Bottles (declarative app deployments)
- [`cert`](#capper-cert) — manage TLS certificates signed by the local CA
- [`compute`](#capper-compute) — manage compute hosts, templates, groups, and instances
- [`config`](#capper-config) — resource configuration and drift
- [`connect`](#capper-connect) — connect to a running instance shell
- [`context`](#capper-context) — manage active org / account / project context
- [`create`](#capper-create) — create a .cap image
- [`daemon`](#capper-daemon) — run the Capper control plane daemon
- [`db`](#capper-db) — manage managed database services
- [`delete`](#capper-delete) — delete a local image
- [`dns`](#capper-dns) — manage private DNS zones, records, and service discovery
- [`event`](#capper-event) — view resource lifecycle events
- [`exec`](#capper-exec) — execute a command inside a running instance
- [`firewall`](#capper-firewall) — manage network firewall policies (nftables)
- [`fleet`](#capper-fleet) — manage rolling upgrades across a fleet of node agents
- [`fn`](#capper-fn) — manage serverless functions
- [`governance`](#capper-governance) — manage governance policies
- [`health`](#capper-health) — instance health check status
- [`host`](#capper-host) — manage host inventory and run capability checks
- [`host-storage`](#capper-host-storage) — manage host physical disks and capacity pools
- [`iam`](#capper-iam) — manage IAM users, roles, policies, and audit log
- [`igw`](#capper-igw) — manage internet gateways
- [`ingress`](#capper-ingress) — manage ingress rules
- [`inspect`](#capper-inspect) — inspect an image or instance
- [`ip`](#capper-ip) — manage routable IP addresses
- [`ip-exclusion`](#capper-ip-exclusion) — manage admin IP exclusions (unlist addresses)
- [`ip-pool`](#capper-ip-pool) — manage routable IP pools
- [`job`](#capper-job) — manage and run operational jobs
- [`keygen`](#capper-keygen) — generate an Ed25519 signing key pair
- [`kms`](#capper-kms) — manage local KMS keys (envelope encryption)
- [`lb`](#capper-lb) — manage load balancers
- [`list`](#capper-list) — list images or instances
- [`logs`](#capper-logs) — show instance logs (use --selector for multi-instance view)
- [`market`](#capper-market) — manage the image marketplace
- [`mcp`](#capper-mcp) — manage MCP servers
- [`metrics`](#capper-metrics) — resource metrics
- [`nat`](#capper-nat) — manage NAT gateways
- [`network`](#capper-network) — manage virtual networks
- [`node`](#capper-node) — manage topology nodes
- [`org`](#capper-org) — manage organizations and accounts
- [`placement`](#capper-placement) — manage placement policies
- [`posture`](#capper-posture) — run and review security posture checks
- [`project`](#capper-project) — manage projects (resource namespaces)
- [`queue`](#capper-queue) — manage message queues
- [`quota`](#capper-quota) — manage per-project resource quotas
- [`realm`](#capper-realm) — manage realms
- [`region`](#capper-region) — manage regions
- [`registry`](#capper-registry) — manage image and artifact registries
- [`resources`](#capper-resources) — unified resource inventory (capper-observe)
- [`rm`](#capper-rm) — remove a stopped or failed instance
- [`route-table`](#capper-route-table) — manage VPC route tables
- [`rule`](#capper-rule) — manage event rules
- [`run`](#capper-run) — run a .cap image
- [`schedule`](#capper-schedule) — manage cron-based schedules
- [`scheduler`](#capper-scheduler) — simulate and inspect the region scheduler
- [`schema`](#capper-schema) — inspect and snapshot the control-plane database schema
- [`secret`](#capper-secret) — manage encrypted secrets
- [`sg`](#capper-sg) — manage VPC security groups
- [`sign`](#capper-sign) — sign a .cap image with an Ed25519 private key
- [`stack`](#capper-stack) — manage infrastructure stacks
- [`stats`](#capper-stats) — show live cgroup resource metrics for running instances
- [`status`](#capper-status) — show daemon and subsystem status
- [`stop`](#capper-stop) — stop a running instance
- [`storage`](#capper-storage) — manage volumes, buckets, objects, and snapshots
- [`validate`](#capper-validate) — validate a config file or image
- [`verify`](#capper-verify) — verify the Ed25519 signature on a .cap image
- [`version`](#capper-version) — print the capper version, commit, and build info
- [`volume`](#capper-volume) — manage CSD shared volumes
- [`vpc`](#capper-vpc) — manage VPCs
- [`zone`](#capper-zone) — manage zones

## `capper ai`

manage AI agents, sessions, and MCP servers

**Subcommands:** `agent` · `mcp` · `session`

### `capper ai agent`

manage AI agents

**Subcommands:** `list` · `register` · `revoke`

#### `capper ai agent list`

list AI agents

```text
capper ai agent list
```

#### `capper ai agent register`

register an AI agent

```text
capper ai agent register NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--model` | — | model identifier, e.g. claude-opus-4 |
| `--owner` | — | IAM user owner (defaults to current principal) |

#### `capper ai agent revoke`

revoke an AI agent

```text
capper ai agent revoke NAME
```

### `capper ai mcp`

manage MCP servers

**Subcommands:** `list` · `register`

#### `capper ai mcp list`

list MCP servers

```text
capper ai mcp list
```

#### `capper ai mcp register`

register an MCP server

```text
capper ai mcp register NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | — | required IAM action to call this server |
| `--endpoint` | — | MCP server endpoint URL |

### `capper ai session`

manage AI sessions

**Subcommands:** `list`

#### `capper ai session list`

list AI sessions

```text
capper ai session list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--agent` | — | filter by agent ID or name |

## `capper aio`

All-in-one single-node Capper management

Example:

```bash
capper aio init --backend capdb && capper aio up
```

**Subcommands:** `check-update` · `doctor` · `down` · `init` · `logs` · `reset` · `status` · `up` · `upgrade`

### `capper aio check-update`

check the update feed for a newer version

```text
capper aio check-update [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--channel` | `stable` | update channel to check |
| `--feed` | — | channel feed URL (default $CAPPER_UPDATE_FEED) |

### `capper aio doctor`

Run AIO pre-flight checks

```text
capper aio doctor
```

### `capper aio down`

Stop AIO services

```text
capper aio down
```

### `capper aio init`

Initialise AIO node directories and config

```text
capper aio init [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--backend` | — | database backend: sqlite (default) or capdb |
| `--insecure` | — | capdb backend: disable TLS (dev only; default is TLS) |
| `--name` | — | node name slug (default: devbox) |
| `--storage` | — | storage root path (default: /var/lib/capper) |

### `capper aio logs`

Stream AIO service logs

```text
capper aio logs [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--service` | — | service to stream: control, agent (default: both) |
| `--tail` | `100` | number of recent lines to show |

### `capper aio reset`

Stop AIO, remove state, and clear node identity

```text
capper aio reset
```

### `capper aio status`

Show AIO node status

```text
capper aio status
```

### `capper aio up`

Start AIO services

```text
capper aio up
```

### `capper aio upgrade`

Upgrade (or roll back) the AIO node from a release bundle

Verifies and stages a release bundle into a versioned directory, snapshots the
database, swaps the active version, runs migrations, and health-gates the new
version — automatically rolling back binaries and database on failure.

```text
capper aio upgrade [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--bundle` | — | path to a capper-aio-*.tgz release bundle |
| `--channel` | — | update channel to pull from the feed (e.g. stable, edge) |
| `--feed` | — | channel feed URL (default $CAPPER_UPDATE_FEED) |
| `--rollback` | — | roll back to the previous version |
| `--sha256` | — | expected SHA-256 of the bundle (hex) |
| `--timeout` | `1m0s` | health-gate timeout before auto-rollback |
| `--url` | — | URL to download the release bundle from |
| `--version` | — | override the version label for the staged dir |
| `--yes` | — | do not prompt for confirmation |

## `capper alert`

manage alert rules and evaluate firing alerts

**Subcommands:** `create` · `delete` · `eval` · `list`

### `capper alert create`

create an alert rule

```text
capper alert create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--event-action` | — | event action prefix to match (event_count rules) |
| `--metric` | — | metric name: cpu_micros, memory_bytes, pid_count (metric_threshold rules) |
| `--threshold` | `1` | firing threshold (count or metric value) |
| `--type` | `event_count` | rule type: event_count or metric_threshold |
| `--window` | `60` | evaluation window in seconds |

### `capper alert delete`

delete an alert rule

```text
capper alert delete NAME
```

### `capper alert eval`

evaluate all alert rules and print firing alerts

```text
capper alert eval
```

### `capper alert list`

list alert rules

```text
capper alert list
```

## `capper alerts`

resource monitor alerts

**Subcommands:** `list` · `rules`

### `capper alerts list`

list alerts

```text
capper alerts list
```

### `capper alerts rules`

list alert rules

```text
capper alerts rules
```

## `capper api`

Capper REST API and web console

**Subcommands:** `start`

### `capper api start`

start the Capper API server

```text
capper api start [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--allowed-origin` | — | CORS allowlist origin permitted credentialed cross-origin access (repeatable; loopback always allowed) |
| `--console` | — | path to CapperWeb dist/ to serve as console |
| `--listen` | `127.0.0.1:8686` | listen address |
| `--tls-cert` | — | TLS certificate file (enables HTTPS; requires --tls-key) |
| `--tls-key` | — | TLS private key file (enables HTTPS; requires --tls-cert) |
| `--with-daemon` | — | also run control plane daemon (supervisor) |

## `capper attest`

generate SBOM or provenance for a .cap image

**Subcommands:** `provenance` · `sbom`

### `capper attest provenance`

generate a provenance record for a .cap image

```text
capper attest provenance IMAGE.cap [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--embed` | — | embed the provenance record inside the .cap archive |
| `--out` | — | output path (default: IMAGE.provenance.json) |

### `capper attest sbom`

generate an SPDX 2.3 SBOM for a .cap image

```text
capper attest sbom IMAGE.cap [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--embed` | — | embed the SBOM inside the .cap archive |
| `--out` | — | output path (default: IMAGE.sbom.spdx.json) |

## `capper backup`

manage backups and backup policies

Example:

```bash
capper backup policy-create --schedule '@daily' --retain 7 --resource <id>
```

**Subcommands:** `create` · `list` · `policy-create` · `policy-delete` · `policy-list` · `restore` · `test`

### `capper backup create`

create a backup of the store

```text
capper backup create [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--dest` | — | destination directory (default: <store-root>/backups) |

### `capper backup list`

list backups

```text
capper backup list
```

### `capper backup policy-create`

create a backup policy

```text
capper backup policy-create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--interval` | `3600` | backup interval in seconds |
| `--retention` | `5` | number of backups to retain |
| `--source` | — | backup source (database connection string for --type database) |
| `--target` | — | target path or resource |
| `--type` | `store` | backup type: store\|database |

### `capper backup policy-delete`

delete a backup policy

```text
capper backup policy-delete NAME
```

### `capper backup policy-list`

list backup policies

```text
capper backup policy-list
```

### `capper backup restore`

restore a backup

```text
capper backup restore ID
```

### `capper backup test`

restore into an isolated test namespace and verify DNS/endpoints

```text
capper backup test BACKUP_ID
```

## `capper bottle`

manage Capper Bottles (declarative app deployments)

**Subcommands:** `deploy` · `deployments` · `import` · `list` · `outputs` · `plan` · `remove` · `validate`

### `capper bottle deploy`

deploy a bottle (create all resources)

```text
capper bottle deploy NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--name` | — | deployment name (default: BOTTLE-deploy) |
| `--set` | — | parameter overrides (key=value) |

### `capper bottle deployments`

list bottle deployments

```text
capper bottle deployments
```

### `capper bottle import`

import a bottle definition from a JSON file

```text
capper bottle import FILE
```

### `capper bottle list`

list imported bottles

```text
capper bottle list
```

### `capper bottle outputs`

show outputs from a bottle deployment

```text
capper bottle outputs DEPLOYMENT
```

### `capper bottle plan`

show what a bottle deployment would create

```text
capper bottle plan NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--set` | — | parameter overrides (key=value) |

### `capper bottle remove`

remove a bottle deployment record

```text
capper bottle remove DEPLOYMENT
```

### `capper bottle validate`

validate a bottle file without importing it

```text
capper bottle validate FILE
```

## `capper cert`

manage TLS certificates signed by the local CA

Example:

```bash
capper cert issue --cn svc.internal --san DNS:svc.internal
```

**Subcommands:** `ca` · `issue` · `list` · `revoke`

### `capper cert ca`

print the local CA certificate (PEM)

```text
capper cert ca
```

### `capper cert issue`

issue a certificate signed by the local CA

```text
capper cert issue NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--cn` | — | certificate common name (defaults to NAME) |
| `--dns` | — | DNS SAN entries, repeatable |

### `capper cert list`

list certificates in the project

```text
capper cert list
```

### `capper cert revoke`

revoke a certificate

```text
capper cert revoke NAME
```

## `capper compute`

manage compute hosts, templates, groups, and instances

Example:

```bash
capper compute group create web --template web-tmpl --desired 3 --min 2 --max 10
```

**Subcommands:** `gpu` · `group` · `host` · `instance` · `instance-type` · `template`

### `capper compute gpu`

manage GPU devices

**Subcommands:** `assign` · `inspect` · `list` · `register` · `release` · `remove`

#### `capper compute gpu assign`

assign a GPU to an instance

```text
capper compute gpu assign GPU-ID INSTANCE-ID
```

#### `capper compute gpu inspect`

inspect a GPU device

```text
capper compute gpu inspect ID
```

#### `capper compute gpu list`

list GPU devices

```text
capper compute gpu list
```

#### `capper compute gpu register`

register a GPU device

```text
capper compute gpu register [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--device-path` | — | device path (e.g. /dev/nvidia0) |
| `--memory-mb` | — | GPU memory in megabytes |
| `--model` | — | GPU model (e.g. RTX 3090 24GB) |
| `--vendor` | — | GPU vendor (e.g. NVIDIA) |

#### `capper compute gpu release`

release a GPU assignment

```text
capper compute gpu release GPU-ID
```

#### `capper compute gpu remove`

remove a GPU device record (must not be assigned)

```text
capper compute gpu remove GPU-ID
```

### `capper compute group`

manage instance groups

**Subcommands:** `autoscale` · `create` · `delete` · `inspect` · `list` · `reconcile` · `scale`

#### `capper compute group autoscale`

manage autoscaling policies for a group

**Subcommands:** `disable` · `enable` · `evaluate` · `history` · `list`

##### `capper compute group autoscale disable`

disable all autoscaling policies for a group

```text
capper compute group autoscale disable GROUP
```

##### `capper compute group autoscale enable`

create or update an autoscaling policy for a group

```text
capper compute group autoscale enable POLICY_NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--group` | — | group name (required) |
| `--max` | `10` | maximum replicas |
| `--metric` | `group_cpu_avg_percent` | metric name to scale on |
| `--min` | `1` | minimum replicas |
| `--policy-type` | `target` | policy type: target, threshold, schedule, queue |
| `--queue` | — | queue name (for queue policy type) |
| `--scale-in-cooldown` | `300` | scale-in cooldown in seconds |
| `--scale-out-cooldown` | `60` | scale-out cooldown in seconds |
| `--target` | `60` | target metric value |

##### `capper compute group autoscale evaluate`

manually evaluate autoscaling policies for a group

```text
capper compute group autoscale evaluate GROUP
```

##### `capper compute group autoscale history`

show recent autoscaling decisions for a group

```text
capper compute group autoscale history GROUP [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--limit` | `20` | number of decisions to show |

##### `capper compute group autoscale list`

list autoscaling policies for a group

```text
capper compute group autoscale list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--group` | — | filter by group name |

#### `capper compute group create`

create an instance group

```text
capper compute group create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--desired` | `1` | desired replica count |
| `--max` | `1` | maximum replica count |
| `--min` | — | minimum replica count |
| `--template` | — | template name (required) |

#### `capper compute group delete`

delete a group

```text
capper compute group delete NAME
```

#### `capper compute group inspect`

show group details

```text
capper compute group inspect NAME
```

#### `capper compute group list`

list instance groups

```text
capper compute group list
```

#### `capper compute group reconcile`

ensure the group's actual replica count matches desired

```text
capper compute group reconcile NAME
```

#### `capper compute group scale`

change the desired replica count

```text
capper compute group scale NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--desired` | `1` | desired replica count |

### `capper compute host`

manage compute hosts

**Subcommands:** `drain` · `inspect` · `list` · `register` · `uncordon`

#### `capper compute host drain`

mark a host as drained (no new scheduling)

```text
capper compute host drain NAME
```

#### `capper compute host inspect`

show host details

```text
capper compute host inspect NAME
```

#### `capper compute host list`

list compute hosts

```text
capper compute host list
```

#### `capper compute host register`

register the local host or a remote host by address

```text
capper compute host register local|ADDR [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--label` | — | label key=value for placement matching |

#### `capper compute host uncordon`

mark a host as ready

```text
capper compute host uncordon NAME
```

### `capper compute instance`

manage compute instances

**Subcommands:** `run`

#### `capper compute instance run`

launch an instance from a template

```text
capper compute instance run [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--name` | — | custom instance name (optional) |
| `--template` | — | template name (required) |

### `capper compute instance-type`

manage capsule instance types

**Subcommands:** `create` · `delete` · `inspect` · `list` · `seed`

#### `capper compute instance-type create`

create a custom instance type

```text
capper compute instance-type create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--cpu` | `1` | CPU count |
| `--description` | — | human-readable description |
| `--family` | `compute` | type family (memory, compute, gpu) |
| `--gpu` | — | GPU eligible |
| `--gpu-count` | — | number of GPUs |
| `--memory-mb` | `1024` | memory in megabytes |
| `--pids` | `256` | PID limit |

#### `capper compute instance-type delete`

delete a custom instance type (locked types cannot be deleted)

```text
capper compute instance-type delete NAME
```

#### `capper compute instance-type inspect`

inspect an instance type

```text
capper compute instance-type inspect NAME
```

#### `capper compute instance-type list`

list instance types

```text
capper compute instance-type list
```

#### `capper compute instance-type seed`

seed built-in and standard instance types

```text
capper compute instance-type seed
```

### `capper compute template`

manage instance templates

**Subcommands:** `create` · `delete` · `inspect` · `list`

#### `capper compute template create`

create a template from a JSON document

```text
capper compute template create FILE
```

#### `capper compute template delete`

delete a template

```text
capper compute template delete NAME
```

#### `capper compute template inspect`

show template details

```text
capper compute template inspect NAME
```

#### `capper compute template list`

list instance templates

```text
capper compute template list
```

## `capper config`

resource configuration and drift

**Subcommands:** `drift`

### `capper config drift`

configuration drift

**Subcommands:** `list` · `repair`

#### `capper config drift list`

list resources whose observed config has drifted

```text
capper config drift list
```

#### `capper config drift repair`

repair drift by resetting the baseline to desired config

```text
capper config drift repair RESOURCE_ID
```

## `capper connect`

connect to a running instance shell

```text
capper connect INSTANCE_NAME|INSTANCE_ID
```

## `capper context`

manage active org / account / project context

**Subcommands:** `clear` · `show` · `use-account` · `use-org` · `use-project`

### `capper context clear`

clear all context (org, account, project)

```text
capper context clear
```

### `capper context show`

show active context

```text
capper context show
```

### `capper context use-account`

set the active account

```text
capper context use-account ACCOUNT_ID
```

### `capper context use-org`

set the active organization

```text
capper context use-org ORG_ID
```

### `capper context use-project`

set the active project

```text
capper context use-project PROJECT_ID
```

## `capper create`

create a .cap image

```text
capper create IMAGE_NAME CONFIG_FILE_NAME.json
```

## `capper daemon`

run the Capper control plane daemon

capper daemon starts the control plane: an instance supervisor (restart policy
enforcement) and a reconciler loop (host heartbeat and future reconcilers).

Press Ctrl-C to stop.

```text
capper daemon [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--interval` | `5` | supervisor poll interval in seconds |
| `--metrics-addr` | — | expose Prometheus metrics on this address, e.g. 127.0.0.1:9100 |

## `capper db`

manage managed database services

**Subcommands:** `create` · `delete` · `inspect` · `list` · `restore`

### `capper db create`

create a managed database

```text
capper db create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--engine` | — | database engine: postgres, redis, mariadb, or capdb (required) |
| `--network` | — | attach to virtual network (name or ID) |
| `--port` | — | database port (optional) |
| `--version` | — | engine version (optional) |

### `capper db delete`

delete a managed database

```text
capper db delete NAME
```

### `capper db inspect`

inspect a managed database

```text
capper db inspect NAME
```

### `capper db list`

list managed databases

```text
capper db list
```

### `capper db restore`

restore a database backup into a new managed database record

```text
capper db restore BACKUP_ID [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--conn` | — | target database connection string for pg_restore |
| `--engine` | `postgres` | database engine |
| `--network` | — | attach restored database record to virtual network |
| `--port` | — | database port |
| `--target` | — | new managed database name |
| `--version` | — | engine version |

## `capper delete`

delete a local image

```text
capper delete IMAGE_NAME
```

## `capper dns`

manage private DNS zones, records, and service discovery

Example:

```bash
capper dns zone create internal.example.
```

**Subcommands:** `healthcheck` · `query` · `record` · `serve` · `service` · `start` · `trace` · `zone`

### `capper dns healthcheck`

poll a DNS record's IP and mark unhealthy after 3 failures

```text
capper dns healthcheck ZONE NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--http` | — | HTTP path to poll, e.g. :8080/health |
| `--interval` | `30` | poll interval in seconds |

### `capper dns query`

resolve a DNS name against the local store (or a live daemon)

```text
capper dns query FQDN [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--type` | `A` | record type: A, AAAA, CNAME, TXT, MX, SRV |
| `--upstream` | — | upstream resolver to forward to (ip[:port]) |

### `capper dns record`

manage DNS records

**Subcommands:** `create` · `delete` · `list`

#### `capper dns record create`

add a DNS record to a zone

```text
capper dns record create ZONE NAME TYPE VALUE [VALUE...] [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--ttl` | — | TTL in seconds (0 = zone default) |

#### `capper dns record delete`

delete a DNS record

```text
capper dns record delete ZONE RECORD_ID
```

#### `capper dns record list`

list all records in a zone

```text
capper dns record list ZONE
```

### `capper dns serve`

start DNS daemon bound to a network's gateway address

```text
capper dns serve [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--addr` | — | listen address (default: GATEWAY:53) |
| `--network` | — | network name or ID (required) |

### `capper dns service`

manage service discovery records

**Subcommands:** `create` · `delete` · `list`

#### `capper dns service create`

create a selector-backed service discovery record

```text
capper dns service create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--port` | — | service port (required) |
| `--proto` | `tcp` | protocol: tcp or udp |
| `--selector` | — | selector: label:KEY=VALUE (required) |
| `--ttl` | `5` | TTL in seconds |
| `--zone` | — | zone name (required) |

#### `capper dns service delete`

delete a service discovery record

```text
capper dns service delete ZONE SERVICE_ID
```

#### `capper dns service list`

list service discovery records in a zone

```text
capper dns service list ZONE
```

### `capper dns start`

run the embedded DNS daemon in the foreground

```text
capper dns start [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--listen` | `127.0.0.1:1053` | address to listen on (UDP+TCP) |
| `--upstream` | — | comma-separated upstream resolvers (default: 8.8.8.8,8.8.4.4) |

### `capper dns trace`

trace a DNS resolution with timing

```text
capper dns trace FQDN [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--type` | `A` | record type: A, AAAA, CNAME, TXT |

### `capper dns zone`

manage DNS hosted zones

**Subcommands:** `create` · `delete` · `inspect` · `list`

#### `capper dns zone create`

create a private hosted zone

```text
capper dns zone create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--description` | — | zone description |
| `--ttl` | `30` | default TTL for records in this zone |

#### `capper dns zone delete`

delete a zone and all its records

```text
capper dns zone delete NAME
```

#### `capper dns zone inspect`

show zone details, records, and services

```text
capper dns zone inspect NAME
```

#### `capper dns zone list`

list hosted zones

```text
capper dns zone list
```

## `capper event`

view resource lifecycle events

**Subcommands:** `export` · `list` · `tail`

### `capper event export`

export events to a JSONL file

```text
capper event export [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--out` | — | output file path (default: stdout) |

### `capper event list`

list recent resource events

```text
capper event list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | — | filter by action prefix (e.g. instance) |
| `--limit` | `50` | maximum number of events to return |
| `--resource` | — | filter by resource ID |
| `--type` | — | filter by resource type (instance, network, firewall, image) |

### `capper event tail`

stream new resource events (Ctrl-C to stop)

```text
capper event tail [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--type` | — | filter by resource type |

## `capper exec`

execute a command inside a running instance

```text
capper exec INSTANCE_NAME|INSTANCE_ID COMMAND [ARGS...]
```

## `capper firewall`

manage network firewall policies (nftables)

Example:

```bash
capper firewall rule ... && capper firewall apply
```

**Subcommands:** `apply` · `delete` · `init` · `inspect` · `list` · `reset` · `rule`

### `capper firewall apply`

compile and apply (or dry-run) the firewall policy

```text
capper firewall apply NETWORK [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--dry-run` | — | print the nft script without applying |

### `capper firewall delete`

delete a firewall policy and remove nftables chain

```text
capper firewall delete NETWORK
```

### `capper firewall init`

initialize a firewall policy for a network

```text
capper firewall init NETWORK_ID [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--mode` | `strict` | firewall mode: strict, permissive, internal |

### `capper firewall inspect`

show firewall details and rules

```text
capper firewall inspect NETWORK
```

### `capper firewall list`

list firewall policies

```text
capper firewall list
```

### `capper firewall reset`

flush nftables chain and mark firewall as pending

```text
capper firewall reset NETWORK
```

### `capper firewall rule`

manage firewall rules

**Subcommands:** `add` · `delete` · `disable` · `enable`

#### `capper firewall rule add`

add a firewall rule

```text
capper firewall rule add NETWORK [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | `allow` | rule action: allow, deny, reject |
| `--description` | — | human-readable description |
| `--direction` | `forward` | direction: forward, ingress, egress, any |
| `--from` | `any` | source endpoint (any, internet, gateway, network, cidr:CIDR, instance:ID, label:KEY=VAL) |
| `--port` | — | destination port(s) |
| `--priority` | — | rule priority (0 = auto-assign) |
| `--proto` | `any` | protocol: any, tcp, udp, icmp |
| `--to` | `any` | destination endpoint |

#### `capper firewall rule delete`

delete a firewall rule

```text
capper firewall rule delete NETWORK RULE_ID
```

#### `capper firewall rule disable`

disable a rule without deleting it

```text
capper firewall rule disable NETWORK RULE_ID
```

#### `capper firewall rule enable`

enable a disabled rule

```text
capper firewall rule enable NETWORK RULE_ID
```

## `capper fleet`

manage rolling upgrades across a fleet of node agents

**Subcommands:** `status` · `upgrade`

### `capper fleet status`

show each node's agent version and drain state

```text
capper fleet status [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--control-url` | — | control-plane base URL (default $CAPPER_CONTROL_URL or http://localhost:8080) |
| `--token` | — | bearer token (default $CAPPER_TOKEN) |

### `capper fleet upgrade`

rolling-upgrade node agents to a target version, batch by batch

Drains each node, runs the per-node upgrade command (--exec), waits for the
node to re-report the target version, then uncordons it — N nodes at a time,
with a control-plane health gate between batches. Stops on the first failure.

```text
capper fleet upgrade --target VERSION [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--batch` | `1` | number of nodes to upgrade concurrently |
| `--control-url` | — | control-plane base URL (default $CAPPER_CONTROL_URL or http://localhost:8080) |
| `--dry-run` | — | print the plan without making changes |
| `--exec` | — | per-node upgrade command; {node} is substituted (e.g. 'ssh {node} capper-self-upgrade') |
| `--target` | — | target agent version (required) |
| `--timeout` | `5m0s` | per-node convergence timeout |
| `--token` | — | bearer token (default $CAPPER_TOKEN) |

## `capper fn`

manage serverless functions

**Subcommands:** `create` · `delete` · `invocations` · `invoke` · `list`

### `capper fn create`

create a function

```text
capper fn create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--command` | — | command to execute (repeatable) |
| `--image` | — | function image |
| `--runtime` | `native` | function runtime |

### `capper fn delete`

delete a function

```text
capper fn delete NAME
```

### `capper fn invocations`

list recent invocations of a function

```text
capper fn invocations NAME
```

### `capper fn invoke`

invoke a function synchronously

```text
capper fn invoke NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--payload` | — | payload sent to the function on stdin |

### `capper fn list`

list functions

```text
capper fn list
```

## `capper governance`

manage governance policies

**Subcommands:** `add` · `eval` · `list`

### `capper governance add`

add a governance policy rule

```text
capper governance add NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | `*` | action (e.g. create, delete, *) |
| `--condition` | — | optional label condition (e.g. label.env=prod) |
| `--effect` | `deny` | allow or deny |
| `--priority` | — | rule priority (higher = evaluated first) |
| `--resource` | `*` | resource type (e.g. instance, network, *) |

### `capper governance eval`

evaluate governance policies for a resource/action

```text
capper governance eval [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | — | action to evaluate (required) |
| `--label` | — | label key=value filters |
| `--resource` | — | resource type to evaluate (required) |

### `capper governance list`

list governance policies

```text
capper governance list
```

## `capper health`

instance health check status

**Subcommands:** `list`

### `capper health list`

list health check results

```text
capper health list
```

## `capper host`

manage host inventory and run capability checks

**Subcommands:** `doctor` · `inspect` · `label` · `list` · `register`

### `capper host doctor`

run capability checks on the local host

```text
capper host doctor
```

### `capper host inspect`

show detailed host information

```text
capper host inspect HOSTNAME
```

### `capper host label`

set labels on a host

```text
capper host label HOSTNAME KEY=VALUE [KEY=VALUE ...]
```

### `capper host list`

list registered hosts

```text
capper host list
```

### `capper host register`

register the local host in the inventory

```text
capper host register [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--no-gpu-detect` | — | skip automatic GPU detection |
| `--role` | `[compute]` | host roles (comma-separated) |

## `capper host-storage`

manage host physical disks and capacity pools

**Subcommands:** `disks` · `pool`

### `capper host-storage disks`

list discovered host disks and their allocation state

```text
capper host-storage disks
```

### `capper host-storage pool`

manage storage pools

**Subcommands:** `create` · `delete` · `list`

#### `capper host-storage pool create`

register a storage pool (directory over a mount, or an LVM volume group)

```text
capper host-storage pool create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--backend` | `directory` | pool backend: directory \| lvm |
| `--device` | — | directory backend: backing device path (for display) |
| `--mountpoint` | — | directory backend: mounted path the pool draws from |
| `--vg` | — | lvm backend: volume group name |

#### `capper host-storage pool delete`

delete an (empty) storage pool

```text
capper host-storage pool delete NAME
```

#### `capper host-storage pool list`

list storage pools with capacity usage

```text
capper host-storage pool list
```

## `capper iam`

manage IAM users, roles, policies, and audit log

Example:

```bash
capper iam user create alice --local-user alice
```

**Subcommands:** `audit` · `cross-account` · `grant` · `group` · `policy` · `role` · `service-account` · `token` · `user` · `whoami`

### `capper iam audit`

view the IAM audit log

```text
capper iam audit
```

**Subcommands:** `list` · `tail`

#### `capper iam audit list`

list IAM audit records

```text
capper iam audit list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | — | filter by action prefix (e.g. instance) |
| `--limit` | `100` | maximum number of records to return |
| `--principal` | — | filter by principal ID prefix |
| `--since` | — | show records at or after this RFC3339 timestamp |

#### `capper iam audit tail`

stream new IAM audit events (Ctrl-C to stop)

```text
capper iam audit tail [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | — | filter by action prefix |
| `--principal` | — | filter by principal ID prefix |

### `capper iam cross-account`

manage cross-account IAM policies

**Subcommands:** `create` · `delete` · `list`

#### `capper iam cross-account create`

create a cross-account IAM policy

```text
capper iam cross-account create [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--actions` | — | allowed actions (e.g. instance:run) |
| `--expires-at` | — | RFC3339 expiry (optional) |
| `--name` | — | policy name |
| `--principal-id` | — | principal ID |
| `--principal-type` | `user` | principal type (user\|service-account) |
| `--resources` | `[*]` | resource scopes |
| `--source-account` | — | account granting trust |
| `--target-account` | — | account being accessed |

#### `capper iam cross-account delete`

delete a cross-account IAM policy

```text
capper iam cross-account delete ID
```

#### `capper iam cross-account list`

list cross-account IAM policies

```text
capper iam cross-account list
```

### `capper iam grant`

manage IAM grants

**Subcommands:** `create` · `delete` · `list`

#### `capper iam grant create`

grant a role to a principal

```text
capper iam grant create [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--principal-id` | — | principal ID or name |
| `--principal-type` | `user` | principal type (user\|group\|service-account) |
| `--role` | — | role name or ID |
| `--scope` | `*` | resource scope ("*" = all) |

#### `capper iam grant delete`

```text
capper iam grant delete ID
```

#### `capper iam grant list`

```text
capper iam grant list
```

### `capper iam group`

manage IAM groups

**Subcommands:** `add-member` · `create` · `inspect` · `remove-member`

#### `capper iam group add-member`

```text
capper iam group add-member GROUP USER
```

#### `capper iam group create`

```text
capper iam group create NAME
```

#### `capper iam group inspect`

```text
capper iam group inspect NAME
```

#### `capper iam group remove-member`

```text
capper iam group remove-member GROUP USER
```

### `capper iam policy`

manage IAM policies

**Subcommands:** `create` · `delete` · `inspect` · `list`

#### `capper iam policy create`

create a policy from a JSON file

```text
capper iam policy create NAME POLICY_FILE
```

#### `capper iam policy delete`

```text
capper iam policy delete NAME
```

#### `capper iam policy inspect`

```text
capper iam policy inspect NAME
```

#### `capper iam policy list`

```text
capper iam policy list
```

### `capper iam role`

manage IAM roles

**Subcommands:** `attach-policy` · `create` · `detach-policy` · `list`

#### `capper iam role attach-policy`

```text
capper iam role attach-policy ROLE POLICY
```

#### `capper iam role create`

```text
capper iam role create NAME
```

#### `capper iam role detach-policy`

```text
capper iam role detach-policy ROLE POLICY
```

#### `capper iam role list`

```text
capper iam role list
```

### `capper iam service-account`

manage service accounts

**Subcommands:** `create` · `list`

#### `capper iam service-account create`

```text
capper iam service-account create NAME
```

#### `capper iam service-account list`

```text
capper iam service-account list
```

### `capper iam token`

issue and manage API tokens

**Subcommands:** `create` · `revoke`

#### `capper iam token create`

issue a new API token for the current principal

```text
capper iam token create [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--name` | — | human-readable name for the token |
| `--ttl` | `24h` | token lifetime (e.g. 1h, 24h, 7d) |

#### `capper iam token revoke`

revoke a token by ID

```text
capper iam token revoke ID
```

### `capper iam user`

manage IAM users

**Subcommands:** `create` · `delete` · `list`

#### `capper iam user create`

create an IAM user

```text
capper iam user create NAME [--local-user OSUSER] [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--local-user` | — | OS username to associate |

#### `capper iam user delete`

delete an IAM user

```text
capper iam user delete NAME
```

#### `capper iam user list`

list IAM users

```text
capper iam user list
```

### `capper iam whoami`

show the current IAM principal

```text
capper iam whoami
```

## `capper igw`

manage internet gateways

**Subcommands:** `create` · `delete` · `list`

### `capper igw create`

create an internet gateway

```text
capper igw create <name> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | VPC id or name (required) |

### `capper igw delete`

delete an internet gateway

```text
capper igw delete <name-or-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | VPC id or name (required) |

### `capper igw list`

list internet gateways

```text
capper igw list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | filter by VPC id |

## `capper ingress`

manage ingress rules

**Subcommands:** `create` · `delete` · `list`

### `capper ingress create`

create an ingress rule

```text
capper ingress create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--backend` | — | backend LB name |
| `--host` | — | hostname to match |
| `--path` | `/` | path prefix to match |
| `--rate-limit` | — | requests per minute (0 = unlimited) |
| `--tls-cert` | — | TLS cert name |

### `capper ingress delete`

delete an ingress rule

```text
capper ingress delete NAME
```

### `capper ingress list`

list ingress rules

```text
capper ingress list
```

## `capper inspect`

inspect an image or instance

**Subcommands:** `image` · `instance`

### `capper inspect image`

show detailed image information

```text
capper inspect image IMAGE_NAME
```

### `capper inspect instance`

show detailed instance information

```text
capper inspect instance INSTANCE_NAME|INSTANCE_ID
```

## `capper ip`

manage routable IP addresses

**Subcommands:** `list` · `release` · `reserve`

### `capper ip list`

list IP addresses

```text
capper ip list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--pool` | — | filter by pool |
| `--status` | — | filter by status |

### `capper ip release`

release a reserved IP

```text
capper ip release NAME
```

### `capper ip reserve`

reserve an IP from a pool

```text
capper ip reserve NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--address` | — | specific address to reserve |
| `--pool` | — | pool name or ID (required) |
| `--purpose` | — | purpose (load-balancer, egress, passthrough, …) |
| `--reserved` | — | mark as a reserved (Elastic) IP |

## `capper ip-exclusion`

manage admin IP exclusions (unlist addresses)

**Subcommands:** `add` · `list` · `remove`

### `capper ip-exclusion add`

unlist an address so it is never auto-allocated

```text
capper ip-exclusion add ADDRESS [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--pool` | — | scope to a pool (default: global) |
| `--reason` | — | why the address is excluded |

### `capper ip-exclusion list`

list IP exclusions

```text
capper ip-exclusion list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--pool` | — | filter to a pool (includes global) |

### `capper ip-exclusion remove`

re-list a previously excluded address

```text
capper ip-exclusion remove ID
```

## `capper ip-pool`

manage routable IP pools

**Subcommands:** `create` · `delete` · `list`

### `capper ip-pool create`

create a routable IP pool

```text
capper ip-pool create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--cidr` | — | pool CIDR (required) |
| `--gateway` | — | gateway address to exclude |
| `--interface` | — | host interface name |
| `--no-auto-allocate` | — | reserved-only pool (no auto allocation) |
| `--region` | — | region (when scope=region) |
| `--scope` | `global` | pool scope |
| `--usage` | — | comma-separated usage classes |

### `capper ip-pool delete`

delete an IP pool

```text
capper ip-pool delete NAME
```

### `capper ip-pool list`

list IP pools

```text
capper ip-pool list
```

## `capper job`

manage and run operational jobs

**Subcommands:** `create` · `delete` · `list` · `logs` · `run`

### `capper job create`

import a job spec from a JSON file

```text
capper job create FILE
```

### `capper job delete`

delete a job record

```text
capper job delete NAME
```

### `capper job list`

list jobs

```text
capper job list
```

### `capper job logs`

show logs from the last job run

```text
capper job logs NAME
```

### `capper job run`

execute a job's steps

```text
capper job run NAME
```

## `capper keygen`

generate an Ed25519 signing key pair

```text
capper keygen [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--key-out` | `capper.key` | output path for private key |
| `--pub-out` | `capper.pub` | output path for public key |

## `capper kms`

manage local KMS keys (envelope encryption)

Example:

```bash
capper kms key create app-key
```

**Subcommands:** `key`

### `capper kms key`

manage symmetric data keys

**Subcommands:** `create` · `decrypt` · `encrypt` · `list` · `rotate`

#### `capper kms key create`

create a new data key

```text
capper kms key create NAME
```

#### `capper kms key decrypt`

decrypt base64 ciphertext using a KMS data key

```text
capper kms key decrypt NAME CIPHERTEXT_BASE64
```

#### `capper kms key encrypt`

encrypt plaintext using a KMS data key (output: base64 ciphertext)

```text
capper kms key encrypt NAME PLAINTEXT
```

#### `capper kms key list`

list KMS keys in the project

```text
capper kms key list
```

#### `capper kms key rotate`

rotate a data key (generates new key, marks old as rotated)

```text
capper kms key rotate NAME
```

## `capper lb`

manage load balancers

Example:

```bash
capper lb create web-lb --listen 0.0.0.0:8080 --mode http --select tier=web
```

**Subcommands:** `backend` · `create` · `delete` · `inspect` · `list` · `logs` · `publish`

### `capper lb backend`

manage backends for a load balancer

**Subcommands:** `add` · `remove`

#### `capper lb backend add`

add a backend to a load balancer

```text
capper lb backend add LB INSTANCE:PORT
```

#### `capper lb backend remove`

remove a backend from a load balancer

```text
capper lb backend remove LB INSTANCE:PORT
```

### `capper lb create`

create a load balancer

```text
capper lb create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--algo` | — | balancing algorithm: round-robin or least-connections |
| `--listen` | — | listen address, e.g. 0.0.0.0:8080 |
| `--mode` | `tcp` | proxy mode: tcp or http |
| `--network` | — | attach to virtual network (name or ID) |
| `--select` | — | service selector label key=value |
| `--tls-cert` | — | TLS cert name from cert store |

### `capper lb delete`

delete a load balancer

```text
capper lb delete NAME
```

### `capper lb inspect`

inspect a load balancer and its backends

```text
capper lb inspect NAME
```

### `capper lb list`

list load balancers

```text
capper lb list
```

### `capper lb logs`

show request logs for a load balancer

```text
capper lb logs NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--follow, -f` | — | follow log output |

### `capper lb publish`

set the host listen address for a load balancer

```text
capper lb publish NAME HOST:PORT
```

## `capper list`

list images or instances

**Subcommands:** `images` · `instances`

### `capper list images`

list images

```text
capper list images
```

### `capper list instances`

list instances

```text
capper list instances
```

## `capper logs`

show instance logs (use --selector for multi-instance view)

```text
capper logs INSTANCE_NAME|INSTANCE_ID [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--selector` | — | filter by label KEY=VALUE, repeatable (combined log view) |
| `--stderr` | — | show stderr only |
| `--stdout` | — | show stdout only |

## `capper market`

manage the image marketplace

**Subcommands:** `approve` · `inspect` · `install` · `list` · `reject` · `scan` · `submit`

### `capper market approve`

approve a marketplace listing

```text
capper market approve ID
```

### `capper market inspect`

inspect a marketplace listing

```text
capper market inspect ID
```

### `capper market install`

install a marketplace listing as a stack

```text
capper market install ID [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--param` | — | key=value parameters passed to the stack template |

### `capper market list`

list marketplace listings

```text
capper market list
```

### `capper market reject`

reject a marketplace listing

```text
capper market reject ID
```

### `capper market scan`

run static scans on a marketplace listing

```text
capper market scan ID
```

### `capper market submit`

submit an image to the marketplace

```text
capper market submit IMAGE [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--description` | — | listing description |

## `capper mcp`

manage MCP servers

**Subcommands:** `approvals` · `deploy` · `list` · `tools`

### `capper mcp approvals`

list pending MCP tool approvals

```text
capper mcp approvals
```

### `capper mcp deploy`

deploy an MCP server

```text
capper mcp deploy NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--approval-policy` | `dangerous-only` | none\|dangerous-only\|all |
| `--runtime` | `mcp-go` | MCP server runtime |

### `capper mcp list`

list MCP servers

```text
capper mcp list
```

### `capper mcp tools`

manage MCP server tools

**Subcommands:** `list`

#### `capper mcp tools list`

list a server's tools

```text
capper mcp tools list SERVER
```

## `capper metrics`

resource metrics

**Subcommands:** `ingest` · `query`

### `capper metrics ingest`

record a single metric sample

```text
capper metrics ingest [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--metric` | — | metric name |
| `--resource-id` | — | resource id |
| `--resource-type` | — | resource type |
| `--unit` | — | unit |
| `--value` | — | metric value |

### `capper metrics query`

query metric samples for a resource

```text
capper metrics query [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--metric` | — | metric name (required) |
| `--range` | `1h` | time range (e.g. 1h, 24h) |
| `--resource-id` | — | resource id (required) |
| `--resource-type` | — | resource type (required) |

## `capper nat`

manage NAT gateways

**Subcommands:** `create` · `delete` · `list`

### `capper nat create`

create a NAT gateway

```text
capper nat create <name> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--public-ip` | — | public IP address |
| `--subnet` | — | subnet id or name (required) |
| `--vpc` | — | VPC id or name (required) |

### `capper nat delete`

delete a NAT gateway

```text
capper nat delete <name-or-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | VPC id or name (required) |

### `capper nat list`

list NAT gateways

```text
capper nat list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | filter by VPC id |

## `capper network`

manage virtual networks

Example:

```bash
capper network create app-net --mode nat --subnet 10.42.0.0/24 --dns
```

**Subcommands:** `connect` · `create` · `delete` · `disconnect` · `inspect` · `list`

### `capper network connect`

attach an instance to a network

```text
capper network connect INSTANCE [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--ip` | — | preferred IP address |
| `--network` | — | network name or ID (required) |

### `capper network create`

create a virtual network

```text
capper network create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--dns` | — | auto-create a .cap DNS zone with gateway and dns records |
| `--mode` | `nat` | network mode: nat, isolated, host-exposed |
| `--subnet` | `10.42.0.0/24` | subnet CIDR for the network |

### `capper network delete`

delete a virtual network

```text
capper network delete NAME
```

### `capper network disconnect`

detach an instance from a network

```text
capper network disconnect INSTANCE [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--network` | — | network name or ID (required) |

### `capper network inspect`

show network details and active leases

```text
capper network inspect NAME
```

### `capper network list`

list virtual networks

```text
capper network list
```

## `capper node`

manage topology nodes

Example:

```bash
capper node join my-node --token <join-token> --address 10.0.0.5 --role compute
```

**Subcommands:** `approve` · `cordon` · `delete` · `drain` · `get` · `join` · `list` · `register`

### `capper node approve`

approve a pending node

```text
capper node approve <node-id-or-slug>
```

### `capper node cordon`

cordon (or uncordon) a node

```text
capper node cordon <slug-or-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--uncordon` | — | uncordon instead of cordon |

### `capper node delete`

delete a node

```text
capper node delete <slug-or-id>
```

### `capper node drain`

drain a node

```text
capper node drain <slug-or-id>
```

### `capper node get`

get node details

```text
capper node get <slug-or-id>
```

### `capper node join`

register this node via a join token

```text
capper node join <name> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--address` | — | node address/IP |
| `--agent-version` | — | agent version string |
| `--cpu` | — | CPU count |
| `--disk` | — | disk in bytes |
| `--gpu` | — | GPU count |
| `--label` | — | key=value labels (repeatable) |
| `--memory` | — | memory in bytes |
| `--role` | — | node roles (repeatable) |
| `--token` | — | join token (required) |

### `capper node list`

list nodes

```text
capper node list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--region` | — | filter by region slug or ID |
| `--zone` | — | filter by zone slug or ID |

### `capper node register`

register a node

```text
capper node register <slug> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--address` | `127.0.0.1` | node address |
| `--cpu` | — | CPU count |
| `--disk` | — | disk in bytes |
| `--label` | — | labels key=value |
| `--memory` | — | memory in bytes |
| `--realm` | — | realm slug or ID |
| `--region` | — | region slug or ID |
| `--zone` | — | zone slug or ID |

## `capper org`

manage organizations and accounts

Example:

```bash
capper org create acme && capper org account-create --org acme prod
```

**Subcommands:** `account-create` · `account-inspect` · `account-list` · `create` · `delete` · `inspect` · `list`

### `capper org account-create`

create an account within an organization

```text
capper org account-create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--org` | — | organization ID or name |

### `capper org account-inspect`

show details of an account

```text
capper org account-inspect NAME
```

### `capper org account-list`

list accounts in an organization

```text
capper org account-list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--org` | — | organization ID or name |

### `capper org create`

create an organization

```text
capper org create NAME
```

### `capper org delete`

delete an organization

```text
capper org delete NAME
```

### `capper org inspect`

show details of an organization

```text
capper org inspect NAME
```

### `capper org list`

list organizations

```text
capper org list
```

## `capper placement`

manage placement policies

**Subcommands:** `create` · `delete` · `get` · `list`

### `capper placement create`

create a placement policy

```text
capper placement create <slug> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--min-regions` | `1` | minimum regions |
| `--min-zones` | `1` | minimum zones |
| `--name` | — | display name |
| `--scope` | `region` | scope (region, zone, realm) |
| `--strategy` | `spread-zones` | placement strategy |

### `capper placement delete`

delete a placement policy

```text
capper placement delete <slug-or-id>
```

### `capper placement get`

get placement policy details

```text
capper placement get <slug-or-id>
```

### `capper placement list`

list placement policies

```text
capper placement list
```

## `capper posture`

run and review security posture checks

**Subcommands:** `list` · `scan`

### `capper posture list`

list stored posture findings for the project

```text
capper posture list
```

### `capper posture scan`

run posture checks (open ports, world-writable paths, SUID files)

```text
capper posture scan [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--root` | — | filesystem root to scan for file-based checks (empty = skip) |

## `capper project`

manage projects (resource namespaces)

**Subcommands:** `create` · `delete` · `inspect` · `label` · `list`

### `capper project create`

create a new project

```text
capper project create NAME
```

### `capper project delete`

delete a project

```text
capper project delete NAME
```

### `capper project inspect`

show project details

```text
capper project inspect NAME
```

### `capper project label`

set labels on a project

```text
capper project label NAME KEY=VALUE [KEY=VALUE ...]
```

### `capper project list`

list all projects

```text
capper project list
```

## `capper queue`

manage message queues

**Subcommands:** `consume` · `create` · `delete` · `list` · `publish`

### `capper queue consume`

consume messages from a queue

```text
capper queue consume QUEUE [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--max` | `10` | maximum messages to consume |

### `capper queue create`

create a message queue

```text
capper queue create NAME
```

### `capper queue delete`

delete a queue

```text
capper queue delete NAME
```

### `capper queue list`

list queues

```text
capper queue list
```

### `capper queue publish`

publish a message to a queue

```text
capper queue publish QUEUE MESSAGE
```

## `capper quota`

manage per-project resource quotas

**Subcommands:** `list` · `set`

### `capper quota list`

list quotas for a project

```text
capper quota list
```

### `capper quota set`

set a quota for a project resource

```text
capper quota set [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--limit` | — | quota limit |
| `--resource` | — | resource type: instance, storage, network |

## `capper realm`

manage realms

**Subcommands:** `create` · `delete` · `get` · `list`

### `capper realm create`

create a realm

```text
capper realm create <slug> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--description` | — | description |
| `--name` | — | display name |

### `capper realm delete`

delete a realm

```text
capper realm delete <slug-or-id>
```

### `capper realm get`

get realm details

```text
capper realm get <slug-or-id>
```

### `capper realm list`

list realms

```text
capper realm list
```

## `capper region`

manage regions

**Subcommands:** `create` · `delete` · `drain` · `evacuate` · `get` · `list`

### `capper region create`

create a region

```text
capper region create <slug> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--country` | — | country code |
| `--location` | — | location description |
| `--name` | — | display name |
| `--realm` | — | realm slug or ID |
| `--region-code` | — | region code |

### `capper region delete`

delete a region

```text
capper region delete <slug-or-id>
```

### `capper region drain`

drain a region

```text
capper region drain <slug-or-id>
```

### `capper region evacuate`

evacuate a region to a target region

```text
capper region evacuate <slug-or-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--target-region` | — | target region slug or ID |

### `capper region get`

get region details

```text
capper region get <slug-or-id>
```

### `capper region list`

list regions

```text
capper region list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--realm` | — | filter by realm slug or ID |

## `capper registry`

manage image and artifact registries

**Subcommands:** `artifact` · `delete` · `gc` · `image` · `init` · `inspect` · `list` · `token`

### `capper registry artifact`

manage artifacts in a registry

**Subcommands:** `delete` · `get` · `list` · `put`

#### `capper registry artifact delete`

delete an artifact version from a registry

```text
capper registry artifact delete REGISTRY NAME:VERSION
```

#### `capper registry artifact get`

download an artifact from a registry

```text
capper registry artifact get REGISTRY NAME:VERSION DEST
```

#### `capper registry artifact list`

list artifacts (optionally filtered by registry)

```text
capper registry artifact list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--registry` | — | filter by registry name |

#### `capper registry artifact put`

upload a file as a versioned artifact

```text
capper registry artifact put REGISTRY FILE [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--label` | — | label KEY=VALUE (repeatable) |
| `--name` | — | artifact name (required) |
| `--type` | — | artifact type (inferred from filename if omitted) |
| `--version` | `latest` | artifact version |

### `capper registry delete`

delete a registry and all its contents

```text
capper registry delete NAME
```

### `capper registry gc`

remove unreferenced files from a registry

```text
capper registry gc NAME
```

### `capper registry image`

manage images in a registry

**Subcommands:** `delete` · `list` · `pull` · `push` · `scan` · `tag`

#### `capper registry image delete`

delete an image version from a registry

```text
capper registry image delete REGISTRY/NAME:VERSION
```

#### `capper registry image list`

list images in a registry (or all registries)

```text
capper registry image list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--registry` | — | filter by registry name |

#### `capper registry image pull`

pull an image from a registry to a local path

```text
capper registry image pull REGISTRY/NAME:VERSION DEST
```

#### `capper registry image push`

push a .cap image into a registry

```text
capper registry image push FILE REGISTRY/NAME:VERSION
```

#### `capper registry image scan`

run static security scans on a registry image and update its scan_status

```text
capper registry image scan REGISTRY/NAME:VERSION
```

#### `capper registry image tag`

tag an image with a new version

```text
capper registry image tag REGISTRY/NAME:VERSION NEW_VERSION
```

### `capper registry init`

create a local registry (idempotent)

```text
capper registry init NAME
```

### `capper registry inspect`

show registry details

```text
capper registry inspect NAME
```

### `capper registry list`

list registries

```text
capper registry list
```

### `capper registry token`

manage registry auth tokens

**Subcommands:** `create`

#### `capper registry token create`

issue a short-lived registry auth token

```text
capper registry token create [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--registry` | — | registry name (empty = all registries) |
| `--ttl` | `24h` | token TTL (e.g. 1h, 24h) |

## `capper resources`

unified resource inventory (capper-observe)

**Subcommands:** `get` · `list` · `sync`

### `capper resources get`

show a resource and its latest config

```text
capper resources get ID
```

### `capper resources list`

list resources in the inventory

```text
capper resources list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--health` | — | filter by health |
| `--project` | — | filter by project |
| `--status` | — | filter by status |
| `--type` | — | filter by resource type |

### `capper resources sync`

project live resources into the inventory

```text
capper resources sync [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--project` | — | project to sync (default: --project) |

## `capper rm`

remove a stopped or failed instance

```text
capper rm INSTANCE_NAME|INSTANCE_ID
```

## `capper route-table`

manage VPC route tables

**Subcommands:** `add-route` · `associate` · `create` · `delete` · `delete-route` · `list` · `list-routes`

### `capper route-table add-route`

add a route to a route table

```text
capper route-table add-route <route-table-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--dest` | — | destination CIDR (required) |
| `--target-id` | — | target resource id |
| `--target-type` | `local` | target type: igw, nat, local, instance |

### `capper route-table associate`

associate a subnet with a route table

```text
capper route-table associate <route-table-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--subnet` | — | subnet id (required) |

### `capper route-table create`

create a route table

```text
capper route-table create <name> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | VPC id or name (required) |

### `capper route-table delete`

delete a route table

```text
capper route-table delete <name-or-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | VPC id or name (required) |

### `capper route-table delete-route`

delete a route

```text
capper route-table delete-route <route-id>
```

### `capper route-table list`

list route tables

```text
capper route-table list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | filter by VPC id |

### `capper route-table list-routes`

list routes in a route table

```text
capper route-table list-routes <route-table-id>
```

## `capper rule`

manage event rules

**Subcommands:** `create` · `delete` · `list`

### `capper rule create`

create an event rule

```text
capper rule create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | `notify` | action: notify, webhook |
| `--args` | — | action arguments (e.g. webhook URL) |
| `--event` | — | event type pattern, e.g. instance.started |

### `capper rule delete`

delete an event rule

```text
capper rule delete NAME
```

### `capper rule list`

list event rules

```text
capper rule list
```

## `capper run`

run a .cap image

```text
capper run IMAGE_NAME.cap [flags]
```

Example:

```bash
capper run web.cap --name web-1 --memory 512M --network app-net \
  --publish 0.0.0.0:8080:8080/tcp --restart on-failure
```

| Flag | Default | Description |
| --- | --- | --- |
| `--cpu-time` | — | limit CPU time in seconds |
| `--file-size` | — | limit maximum file size written by the capsule, e.g. 64M |
| `--instance-type` | — | enforce an instance type envelope (e.g. cap-g1) |
| `--label` | — | attach a label: KEY=VALUE, repeatable |
| `--memory` | — | limit virtual memory/address space, e.g. 128M, 1G |
| `--mount` | — | bind mount in SOURCE:TARGET[:ro] format, repeatable |
| `--name` | — | assign a name to the instance |
| `--network` | — | attach instance to a virtual network (name or ID) |
| `--override-scan` | — | skip scan status check and run even if image has critical findings |
| `--pids` | — | limit number of processes for the capsule user |
| `--publish` | — | publish a container port as HOST:CONTAINER[/proto], repeatable |
| `--require-signature` | — | refuse to run if the image is not signed |
| `--restart` | — | restart policy: never, always, or on-failure |
| `--rm` | — | remove instance automatically after it stops |
| `--secret` | — | inject a secret as an env var: SECRET_NAME[=ENV_VAR], repeatable |
| `--trusted-key` | — | path to trusted public key; implies --require-signature |

## `capper schedule`

manage cron-based schedules

**Subcommands:** `create` · `delete` · `list`

### `capper schedule create`

create a schedule

```text
capper schedule create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | `backup` | action: backup, webhook, run-instance |
| `--args` | — | action arguments |
| `--cron` | `@1h` | cron interval: @5m, @1h, or standard cron expression |

### `capper schedule delete`

delete a schedule

```text
capper schedule delete NAME
```

### `capper schedule list`

list schedules

```text
capper schedule list
```

## `capper scheduler`

simulate and inspect the region scheduler

**Subcommands:** `capacity` · `simulate`

### `capper scheduler capacity`

show available capacity

```text
capper scheduler capacity [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--region` | — | filter by region |
| `--zone` | — | filter by zone |

### `capper scheduler simulate`

simulate a placement decision

```text
capper scheduler simulate [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--cpu` | — | minimum CPU count required |
| `--gpu` | — | require GPU |
| `--image` | — | image name |
| `--instance-type` | — | instance type |
| `--memory` | — | minimum memory bytes required |
| `--min-zones` | — | minimum zones to spread across |
| `--region` | — | preferred region |
| `--require-label` | — | required node labels key=value |
| `--strategy` | `spread-zones` | placement strategy |
| `--zone` | — | preferred zone |

## `capper schema`

inspect and snapshot the control-plane database schema

**Subcommands:** `backup` · `status`

### `capper schema backup`

write a consistent online snapshot of the control-plane database

```text
capper schema backup [DEST]
```

### `capper schema status`

show applied + pending schema migrations and the schema version

```text
capper schema status
```

## `capper secret`

manage encrypted secrets

Example:

```bash
capper secret create db-password --value "s3cr3t"
```

**Subcommands:** `create` · `delete` · `inspect` · `list`

### `capper secret create`

create or update an encrypted secret

```text
capper secret create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--description` | — | optional description |
| `--value` | — | plaintext secret value (required) |

### `capper secret delete`

delete a secret

```text
capper secret delete NAME
```

### `capper secret inspect`

show secret metadata (use --reveal to decrypt the value)

```text
capper secret inspect NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--reveal` | — | decrypt and print the secret value (requires secret:read) |

### `capper secret list`

list secrets in the project

```text
capper secret list
```

## `capper sg`

manage VPC security groups

**Subcommands:** `create` · `delete` · `list` · `rule`

### `capper sg create`

create a security group

```text
capper sg create <name> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--allow-all` | — | disable default-deny (allow by default) |
| `--desc` | — | description |
| `--vpc` | — | VPC id or name (required) |

### `capper sg delete`

delete a security group

```text
capper sg delete <name-or-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | VPC id or name (required) |

### `capper sg list`

list security groups

```text
capper sg list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--vpc` | — | filter by VPC id |

### `capper sg rule`

manage security group rules

**Subcommands:** `add` · `delete` · `list`

#### `capper sg rule add`

add a rule to a security group

```text
capper sg rule add <sg-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--action` | `allow` | allow or deny |
| `--cidr` | `0.0.0.0/0` | CIDR range |
| `--dir` | `ingress` | direction: ingress or egress |
| `--from` | — | from port |
| `--proto` | `tcp` | protocol: tcp, udp, icmp, all |
| `--to` | — | to port |

#### `capper sg rule delete`

delete a security group rule

```text
capper sg rule delete <rule-id>
```

#### `capper sg rule list`

list rules in a security group

```text
capper sg rule list <sg-id>
```

## `capper sign`

sign a .cap image with an Ed25519 private key

```text
capper sign IMAGE.cap [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--key` | `capper.key` | path to Ed25519 private key |

## `capper stack`

manage infrastructure stacks

**Subcommands:** `apply` · `destroy` · `diff` · `inspect` · `list` · `plan` · `update`

### `capper stack apply`

apply stack from a template file

```text
capper stack apply FILE
```

### `capper stack destroy`

destroy all resources in a stack

```text
capper stack destroy NAME
```

### `capper stack diff`

diff stack against live state

```text
capper stack diff NAME
```

### `capper stack inspect`

inspect a stack

```text
capper stack inspect NAME
```

### `capper stack list`

list stacks

```text
capper stack list
```

### `capper stack plan`

plan stack changes from a template file

```text
capper stack plan FILE
```

### `capper stack update`

update a stack to a new marketplace listing version

```text
capper stack update NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--market-version` | — | marketplace listing version to upgrade to |

## `capper stats`

show live cgroup resource metrics for running instances

```text
capper stats [INSTANCE...] [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--watch` | — | refresh interval in seconds (0 = one-shot) |

## `capper status`

show daemon and subsystem status

```text
capper status
```

## `capper stop`

stop a running instance

```text
capper stop INSTANCE_NAME|INSTANCE_ID [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--kill` | — | send SIGKILL immediately |
| `--timeout` | `5` | seconds to wait before SIGKILL |

## `capper storage`

manage volumes, buckets, objects, and snapshots

Example:

```bash
capper storage volume create data --size 20G --class local --encrypted
```

**Subcommands:** `bucket` · `object` · `s3` · `share` · `snapshot` · `volume`

### `capper storage bucket`

manage object storage buckets

**Subcommands:** `create` · `credentials` · `delete` · `inspect` · `list`

#### `capper storage bucket create`

create an object storage bucket

```text
capper storage bucket create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--encrypted` | — | mark bucket as encrypted |
| `--quota` | — | storage quota (e.g. 100G) |
| `--versioning` | — | enable object versioning |

#### `capper storage bucket credentials`

generate access credentials for a bucket

```text
capper storage bucket credentials NAME
```

#### `capper storage bucket delete`

delete a bucket and its objects

```text
capper storage bucket delete NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--force` | — | delete even if bucket is not empty |

#### `capper storage bucket inspect`

show bucket details

```text
capper storage bucket inspect NAME
```

#### `capper storage bucket list`

list buckets

```text
capper storage bucket list
```

### `capper storage object`

manage objects within buckets

**Subcommands:** `delete` · `get` · `list` · `put`

#### `capper storage object delete`

delete an object from a bucket

```text
capper storage object delete BUCKET KEY
```

#### `capper storage object get`

download an object to a file

```text
capper storage object get BUCKET KEY DEST
```

#### `capper storage object list`

list objects in a bucket

```text
capper storage object list BUCKET [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--prefix` | — | filter objects by key prefix |

#### `capper storage object put`

upload a file as an object

```text
capper storage object put BUCKET KEY FILE
```

### `capper storage s3`

S3-compatible object storage server

**Subcommands:** `credential` · `start`

#### `capper storage s3 credential`

manage S3 IAM service account credentials

**Subcommands:** `create` · `delete` · `list`

##### `capper storage s3 credential create`

generate an S3 access/secret key pair for a service account

```text
capper storage s3 credential create ACCOUNT_ID
```

##### `capper storage s3 credential delete`

delete an S3 credential by ID or access key

```text
capper storage s3 credential delete ID_OR_KEY
```

##### `capper storage s3 credential list`

list S3 credentials for a service account

```text
capper storage s3 credential list ACCOUNT_ID
```

#### `capper storage s3 start`

start the S3-compatible object storage server

Start the S3-compatible HTTP server for Capper object storage.

Third-party tools (AWS CLI, s3cmd, MinIO client) can connect using SigV4:

  aws s3 ls --endpoint-url http://127.0.0.1:9000

Use --no-auth to disable credential checking (development only).
Use --access-key and --secret-key to set static credentials.

```text
capper storage s3 start [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--access-key` | — | SigV4 access key (optional; uses IAM credentials when omitted) |
| `--listen` | `127.0.0.1:9000` | address:port to listen on |
| `--no-auth` | — | disable SigV4 authentication (development only) |
| `--secret-key` | — | SigV4 secret key (required when --access-key is set) |

### `capper storage share`

manage file storage shares

**Subcommands:** `create` · `delete` · `list`

#### `capper storage share create`

create a file storage share

```text
capper storage share create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--instance` | — | target instance ID or name |
| `--mount-path` | — | mount path inside the instance |
| `--path` | — | host filesystem path to share (required) |

#### `capper storage share delete`

delete a storage share

```text
capper storage share delete NAME
```

#### `capper storage share list`

list storage shares

```text
capper storage share list
```

### `capper storage snapshot`

manage volume snapshots

**Subcommands:** `create` · `delete` · `inspect` · `list` · `restore`

#### `capper storage snapshot create`

create a tar.zst snapshot of a volume

```text
capper storage snapshot create VOLUME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--name` | — | snapshot name (auto-generated if empty) |

#### `capper storage snapshot delete`

delete a snapshot and its archive file

```text
capper storage snapshot delete NAME
```

#### `capper storage snapshot inspect`

show snapshot details

```text
capper storage snapshot inspect NAME
```

#### `capper storage snapshot list`

list snapshots

```text
capper storage snapshot list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--source` | — | filter by source volume ID |

#### `capper storage snapshot restore`

restore a snapshot into a volume directory

```text
capper storage snapshot restore SNAPSHOT VOLUME
```

### `capper storage volume`

manage storage volumes

**Subcommands:** `attach` · `create` · `delete` · `detach` · `inspect` · `list`

#### `capper storage volume attach`

record a volume attachment to an instance

```text
capper storage volume attach VOLUME INSTANCE:PATH
```

#### `capper storage volume create`

create a directory-backed volume

```text
capper storage volume create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--class` | `local` | volume class |
| `--encrypted` | — | mark volume as encrypted |
| `--size` | — | volume size hint (e.g. 10G) |

#### `capper storage volume delete`

delete a volume and its directory

```text
capper storage volume delete NAME
```

#### `capper storage volume detach`

clear a volume's attachment record

```text
capper storage volume detach VOLUME
```

#### `capper storage volume inspect`

show volume details

```text
capper storage volume inspect NAME
```

#### `capper storage volume list`

list volumes

```text
capper storage volume list
```

## `capper validate`

validate a config file or image

**Subcommands:** `config` · `image`

### `capper validate config`

validate a create config file

```text
capper validate config FILE
```

### `capper validate image`

validate a .cap image file

```text
capper validate image IMAGE.cap
```

## `capper verify`

verify the Ed25519 signature on a .cap image

```text
capper verify IMAGE.cap [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--trusted-key` | `capper.pub` | path to trusted Ed25519 public key |

## `capper version`

print the capper version, commit, and build info

```text
capper version
```

## `capper volume`

manage CSD shared volumes

**Subcommands:** `attach` · `attachments` · `create` · `delete` · `detach` · `inspect` · `leases` · `list` · `repair` · `revoke-lease` · `snapshot` · `snapshot-delete` · `snapshots`

### `capper volume attach`

attach a volume to an instance

```text
capper volume attach VOLUME INSTANCE [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--access` | `rw` | access mode: rw, ro |
| `--mount` | `/mnt/csd` | mount path inside the instance |

### `capper volume attachments`

list attachments for a volume

```text
capper volume attachments VOLUME
```

### `capper volume create`

create a new CSD shared volume

```text
capper volume create NAME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--class` | `local` | storage class |
| `--enc-key` | — | KMS key name for encryption |
| `--encrypted` | — | encrypt at rest |
| `--mode` | `shared-fs` | volume mode: shared-fs, single-writer, shared-block |
| `--replicas` | `1` | replica count |
| `--size` | `1G` | volume size (e.g. 500M, 10G) |

### `capper volume delete`

delete a CSD volume

```text
capper volume delete NAME
```

### `capper volume detach`

detach a volume from an instance

```text
capper volume detach VOLUME INSTANCE
```

### `capper volume inspect`

show volume details

```text
capper volume inspect NAME
```

### `capper volume leases`

list active leases for a CSD volume

```text
capper volume leases VOLUME
```

### `capper volume list`

list CSD volumes

```text
capper volume list
```

### `capper volume repair`

replay journal and reset volume status to available

```text
capper volume repair VOLUME
```

### `capper volume revoke-lease`

revoke all leases held by a client on a CSD volume

```text
capper volume revoke-lease VOLUME [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--client` | — | client ID whose leases to revoke |

### `capper volume snapshot`

create a snapshot of a CSD volume

```text
capper volume snapshot VOLUME NAME
```

### `capper volume snapshot-delete`

delete a CSD volume snapshot

```text
capper volume snapshot-delete VOLUME SNAPSHOT
```

### `capper volume snapshots`

list snapshots for a CSD volume

```text
capper volume snapshots VOLUME
```

## `capper vpc`

manage VPCs

Example:

```bash
capper vpc create prod --cidr 10.0.0.0/16 --home-region local
```

**Subcommands:** `create` · `delete` · `get` · `list` · `subnet`

### `capper vpc create`

create a VPC

```text
capper vpc create <slug> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--cidr` | — | VPC CIDR block |
| `--home-region` | — | home region slug or ID |
| `--mobility` | `manual` | mobility policy |
| `--name` | — | display name |

### `capper vpc delete`

delete a VPC

```text
capper vpc delete <slug-or-id>
```

### `capper vpc get`

get VPC details

```text
capper vpc get <slug-or-id>
```

### `capper vpc list`

list VPCs

```text
capper vpc list
```

### `capper vpc subnet`

manage VPC subnets

**Subcommands:** `create` · `list`

#### `capper vpc subnet create`

create a subnet

```text
capper vpc subnet create <vpc> <slug> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--cidr` | — | subnet CIDR |
| `--gateway` | — | gateway IP |
| `--mode` | `nat` | subnet mode (nat, isolated, routed, overlay, host) |
| `--name` | — | display name |
| `--zone` | — | zone slug or ID |

#### `capper vpc subnet list`

list subnets in a VPC

```text
capper vpc subnet list <vpc>
```

## `capper zone`

manage zones

**Subcommands:** `cordon` · `create` · `delete` · `drain` · `get` · `list`

### `capper zone cordon`

cordon (or uncordon) a zone

```text
capper zone cordon <slug-or-id> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--uncordon` | — | uncordon instead of cordon |

### `capper zone create`

create a zone

```text
capper zone create <slug> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--control-url` | — | zone controller URL |
| `--failure-domain` | — | failure domain (rack, power group, etc.) |
| `--name` | — | display name |
| `--network-cidr` | — | zone network CIDR |
| `--realm` | — | realm slug or ID |
| `--region` | — | region slug or ID |

### `capper zone delete`

delete a zone

```text
capper zone delete <slug-or-id>
```

### `capper zone drain`

drain a zone

```text
capper zone drain <slug-or-id>
```

### `capper zone get`

get zone details

```text
capper zone get <slug-or-id>
```

### `capper zone list`

list zones

```text
capper zone list [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--region` | — | filter by region slug or ID |

