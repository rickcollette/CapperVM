---
title: "Troubleshooting"
description: "Common failures bringing up capsules, the control plane, nodes, and the database."
owner: "docs"
status: "stable"
reviewed: "2026-06-12"
outputs:
  - markdown
  - web
  - pdf
---

# Troubleshooting

First stops for diagnosis:

```bash
capper host doctor     # host capability + dependency report
capper status          # daemon and subsystem status
capper aio status      # AIO service status (if using aio)
capper aio logs        # stream AIO service logs
```

## Capsules

**`bwrap` fails / user namespaces unavailable.** Capper prefers Bubblewrap with
unprivileged user namespaces. If the host disallows them, either enable them, or
fall back with `--runtime chroot` (may require `sudo`), or use `--runtime crun`/`lxc`/`qemu` or 
`runc` if installed. `capper host doctor` reports what is available.

**A capsule is killed unexpectedly.** Check the resource limits you passed
(`--memory`, `--cpu-time`, `--file-size`); the cgroup enforcement kills overruns.
Inspect with `capper logs <instance>` and `capper stats`.

## Control plane / API

**`authentication required` (401).** The API requires a bearer token or a session
cookie on `/api/...`. Issue a token via the IAM commands and send
`Authorization: Bearer <token>`. Public paths are `/api/v1/health`, `…/openapi.json`,
`…/version`, `…/daemon/status`, and node join.

**`not authorized for account/org/project …` (403).** The `X-Capper-Account-ID` /
`-Org-ID` / `-Project-ID` headers are validated against the principal's
memberships — you cannot scope to a tenant you do not belong to. Use a principal
that is a member (or org/account root) of the target.

**Browser session does not stick.** Session cookies are `Secure`, so they are only
sent over HTTPS. Serve the API over TLS (`--tls-cert`/`--tls-key`) or behind a TLS
terminator. Over plain `http://` only bearer-token auth works.

**CORS errors from the Web console.** Cross-origin requests are allowlisted.
Loopback origins always work; for any other console origin pass
`--allowed-origin https://your-console`. See [Configuration](configuration.md).

## Nodes

**A joined node never becomes ready.** Joins require approval:
`capper node approve <name>`. Confirm the agent can reach the control-plane URL in
`/etc/capper/agent.yaml` and that heartbeats arrive (`capper node list`).

## Database backend

**`driver is not compiled in`.** You set `CAPPER_DB_DRIVER=capdb` on a binary built
without `-tags capdb`. Rebuild with `make build-capdb`.

**Startup hangs or fails to reach CapDB.** The control plane pings the server on
startup with bounded retries (`CAPPER_DB_STARTUP_RETRIES`) and a `connect_timeout`
DSN param. A black-holed host now fails fast rather than hanging. Verify the DSN,
that `capdb-server` is listening, and that the CA in `ca=` matches the server cert
SAN.

**Queries fail right after a DB restart.** Expected to self-heal: broken pooled
connections are evicted and the next query reconnects within seconds. If it does
not recover, check server logs and `capdb-server` health.

## Getting more detail

Add `--debug` to any command for verbose logging, and `--json` for
machine-readable output you can inspect or pipe to `jq`.
