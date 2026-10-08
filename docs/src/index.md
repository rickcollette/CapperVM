---
title: "Capper Documentation"
description: "Documentation for the Capper private cloud platform."
owner: "docs"
status: "stable"
reviewed: "2026-06-10"
outputs:
  - markdown
  - web
  - pdf
---

# Capper Documentation

Capper is a Go-based homelab and enterprise private cloud platform — a control plane and runtime stack you own and operate.

## What is Capper?

Capper provides:

- **Compute** — lifecycle management for Linux instances with cgroup isolation
- **Networking** — VPCs, subnets, firewall rules, DNS, load balancers, ingress
- **Storage** — object storage (S3-compatible), block volumes, distributed filesystem
- **IAM** — identity, access policies, token management, role assumption
- **Databases** — managed Postgres, Redis, MariaDB with backup and replication
- **Marketplace** — image approval, signing, provenance, SBOM
- **Observability** — unified log store, alert rules, metrics
- **AI** — secure agent control plane, approval gates, immutable ledger

## Quick Navigation

- [Getting Started](getting-started/overview.md)
- [Operator Guide](operator-guide/index.md)
- [Developer Guide](developer-guide/index.md)
- [API Reference](reference/api/overview.md)
- [CLI Reference](reference/cli/capper.md)
