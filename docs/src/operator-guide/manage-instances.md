---
title: "Manage instances"
description: "Create, run, inspect, and operate .cap capsule instances; templates, types, and groups."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Manage instances

Instances are running `.cap` capsules. Their lifecycle is driven by top-level
verbs; the `compute` group manages the building blocks (images via `create`,
templates, instance types, groups, GPUs, hosts).

## Lifecycle

```bash
capper create alpine.cap examples/alpine/capper.json   # build a .cap image
capper run alpine.cap                                   # start an instance
capper list instances                                   # list
capper inspect <instance>                               # details
capper logs <instance>                                  # logs (use --selector for many)
capper stats                                            # live cgroup metrics
capper exec <instance> -- <cmd>                         # run a command inside
capper connect <instance>                               # interactive shell
capper stop <instance>                                  # stop
capper rm <instance>                                    # remove a stopped/failed instance
capper health                                           # instance health checks
```

## Runtimes and isolation

Capper prefers Bubblewrap (`bwrap`) with unprivileged user namespaces, falling
back to `chroot`, `crun`, or `runc`. Pick explicitly with the global
`--runtime` flag, or per instance with `capper run --runtime-mode` / API
`runtimeMode`. LXC and QEMU are also available when host tools are installed —
see [LXC and QEMU runtimes](runtimes-lxc-qemu.md). `capinit` is PID 1 inside
process runtimes.

> Capsule isolation is hardening, not a security boundary — do not run untrusted
> images.

## Key `run` flags

`capper run` accepts the most options of any command. The ones you reach for most:

| Flag | Purpose |
| --- | --- |
| `--name <name>` | name the instance (otherwise auto-generated) |
| `--memory <size>` | cap address space, e.g. `128M`, `1G` |
| `--cpu-time <secs>` | cap CPU seconds |
| `--file-size <size>` | cap max file size written, e.g. `64M` |
| `--pids <n>` | cap process count |
| `--publish HOST:CONTAINER[/proto]` | publish a port (repeatable) |
| `--mount SOURCE:TARGET[:ro]` | bind-mount (repeatable) |
| `--secret NAME[=ENV]` | inject a [secret](secrets.md) as an env var (repeatable) |
| `--label KEY=VALUE` | attach a label (repeatable) |
| `--restart never\|always\|on-failure` | restart policy |
| `--runtime-mode <mode>` | per-instance runtime: auto, bwrap, chroot, crun, runc, lxc, qemu |
| `--rm` | remove the instance when it stops |
| `--require-signature` / `--trusted-key <path>` | refuse unsigned images |
| `--instance-type <type>` | enforce an instance-type envelope |

See the [full CLI reference](../reference/cli/capper.md#capper-run) for every flag.

### API / Web launch

Via **Compute → Instances → Launch** or `POST /api/v1/instances`:

- **`subnetId`** (required) — VPC subnet; creates a primary ENI with private IP.
- **`securityGroupIds`** — optional SGs on the ENI.
- **Default storage pool** — required (Admin → Storage); disk validated against pool capacity.

![Launch networking step](/assets/images/screenshots/33-launch-instance-networking.png)

### CLI one-shot run

```bash
capper run web.cap \
  --name web-1 \
  --memory 512M --cpu-time 0 --pids 256 \
  --publish 0.0.0.0:8080:8080/tcp \
  --secret DB_PASSWORD=DATABASE_PASSWORD \
  --label tier=web --label env=prod \
  --restart on-failure
```

For the full control plane, prefer the API/Web launch path with `subnetId` so ENI,
metadata, and pool-backed disks are provisioned correctly.

## Templates, instance types, and groups

- **Templates** (`capper compute template`) — reusable launch specs.
- **Instance types** (`capper compute instance-type`) — named size/shape profiles.
- **Compute groups** (`capper compute group`) — managed sets of instances with
  autoscale. See [Compute groups & autoscale](compute-groups.md).
- **GPUs / hosts** (`capper compute gpu`, `capper compute host`) — inventory the
  scheduler places against.

## Placement

The scheduler places instances onto nodes by topology (realm/region/zone), node
roles, capacity, and failure domain. See [Topology & nodes](topology.md).

## Related

- [Compute groups & autoscale](compute-groups.md) · [Topology & nodes](topology.md)
  · [Manage storage](manage-storage.md) · [Quickstart](../getting-started/quickstart.md)
