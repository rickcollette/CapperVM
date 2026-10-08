---
title: "Manage DNS"
description: "Private DNS zones, records, service discovery, health checks, and query tracing."
owner: "docs"
status: "stable"
reviewed: "2026-06-12"
outputs:
  - markdown
  - web
  - pdf
---

# Manage DNS

`capper dns` provides private DNS zones, records, and service discovery. The
resolver answers from the **longest-matching zone**.

## Zones and records

```bash
capper dns zone create internal.example. --subnet-id <subnet-id> --ttl 30 --description "internal services"
capper dns zone list
capper dns record create internal.example. --name api --type A --value 10.0.1.10 --ttl 60
capper dns record list internal.example.
```

| Flag | Command | Purpose |
| --- | --- | --- |
| `--subnet-id <id>` | `zone create` | VPC subnet the private zone is attached to (required) |
| `--ttl <secs>` | `zone create` | default record TTL for the zone (default 30) |
| `--description <text>` | `zone create` | zone description |
| `--ttl <secs>` | `record create` | record TTL (0 = use the zone default) |

## Service discovery

```bash
capper dns service ...        # register/resolve services
capper dns healthcheck ...    # attach health checks to records/services
```

Health checks gate whether a record/service is returned, so unhealthy backends
drop out of resolution.

## Serving and debugging

```bash
capper dns serve              # run the resolver
capper dns query api.internal.example.   # resolve through Capper
capper dns trace api.internal.example.   # trace resolution path
```

## How it fits

DNS names front [load balancers](load-balancers.md) and [ingress](ingress.md)
rules; combine with [public IPs](routable-ips.md) for external reachability. See
the [Networking model](../concepts/networking-model.md).

## Related

- [Networking model](../concepts/networking-model.md) · [Load balancers](load-balancers.md)
  · [Ingress](ingress.md)
