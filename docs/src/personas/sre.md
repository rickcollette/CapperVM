---
title: "DevOps / SRE"
description: "Guide for SREs managing deployments, traffic, and scaling on Capper."
owner: "docs"
status: "stable"
reviewed: "2026-06-10"
outputs:
  - markdown
  - web
  - pdf
---

# DevOps / SRE

You are responsible for keeping applications running and traffic flowing. You
deploy stacks, configure load balancers, tune autoscaling, and respond when
things go wrong.

## Load Balancers

Create a load balancer, set the listen port, select an algorithm (round-robin,
least-connections, or ip-hash), and attach upstream instances using a label
selector. The LB terminates connections and proxies them to healthy backends.

![Load Balancers](/assets/images/screenshots/08-load-balancers.png)

## Health Checks

Define HTTP or TCP health check probes to gate traffic. An upstream is marked
unhealthy after a configurable number of consecutive failures and is removed
from the pool until it recovers.

## Compute Groups and Autoscaling

A compute group is a named fleet of identical instances. Set `desired`,
`minSize`, and `maxSize` then attach an autoscale policy. The ±1 stepper in
the console lets you manually adjust desired count without touching the policy.

![Compute Groups](/assets/images/screenshots/14-compute-groups.png)

## Ingress

Map an external hostname to an upstream instance or load balancer. Attach a
certificate for HTTPS termination. Ingress rules are evaluated in priority
order; the most specific match wins.

![Ingress](/assets/images/screenshots/09-ingress.png)

## Stacks — Zero-Downtime Deployments

Apply a new stack version to update images, environment variables, or resource
counts atomically. Capper provisions the new resources before tearing down the
old ones.

![Stacks](/assets/images/screenshots/17-stacks.png)

## Networks

Segment traffic with isolated networks. Each network has its own CIDR range and
firewall policy. The Networks page shows connected instances and current traffic
state.

![VPCs and subnets](/assets/images/screenshots/03-vpcs.png)

## Backups

Configure backup policies with a schedule interval and retention count. Backups
cover instance volumes and database snapshots.

![Backups](/assets/images/screenshots/16-backups.png)

## Key Workflows

- [Deploy Your First Application](../tutorials/deploy-first-app.md) — image → instance → DNS → LB → Ingress
- [Scale a Workload](../tutorials/scale-workload.md) — Compute Groups + Autoscale + LB
