# Deploy

End-to-end deployment of the Capper All-In-One stack to **cloud.cappervm.com**.

## What gets deployed
- **CapDB** (SQL storage engine) — `capdb-server`
- **Capper** control plane + node agent (`capper`, `capper-agent`, `capinit`)
- **CapperWeb** console (built with the `aio` profile)
- **nginx** reverse proxy terminating TLS
- **Let's Encrypt** ACME certificate (auto-renewing via the `certbot` timer)
- **oauth2-proxy** — Google SSO gate restricting access to allowed email domains

## Authentication (Google SSO)
When OAuth credentials are present, `oauth2-proxy` sits at the nginx edge and
gates the **entire** site (console + API) behind Google login, allowing only
emails in `ALLOWED_DOMAINS` (set via env / `deploy/local.env` / `oauth2.env`). After a user
authenticates, nginx injects a full-admin capper bearer token downstream, so the
existing cookie-based console works with no manual token step.

Set up credentials before deploying:

```bash
cp deploy/local.env.example deploy/local.env
$EDITOR deploy/local.env         # DEPLOY_USER, SSH_KEY, ACME_EMAIL, …
cp deploy/oauth2.env.example deploy/oauth2.env
$EDITOR deploy/oauth2.env        # paste Client ID + secret
```

`oauth2.env` is gitignored. The Google OAuth client (Web application) needs the
redirect URI `https://cloud.cappervm.com/oauth2/callback`. **If no credentials
are provided, the deploy still runs but prints a warning and the site is OPEN.**

> Note: all SSO-authorized users currently share one admin principal inside
> capper (the email is logged by nginx for audit). Per-user IAM mapping is a
> future refinement.

## Usage
From the repo root (or anywhere):

```bash
deploy/deploy.sh
```

`deploy.sh` runs locally: it builds the bundle with `scripts/build-aio.sh`,
ships the `.tgz` + `remote-setup.sh` over SSH, runs the remote install under
`sudo`, obtains the ACME cert, and verifies the public HTTPS endpoint.
`remote-setup.sh` is the half that runs **on the server** — you don't invoke it
directly.

## Configuration (env overrides)
| Var | Default | Meaning |
|-----|---------|---------|
| `DEPLOY_HOST` | `cloud.cappervm.com` | SSH host |
| `DEPLOY_USER` | `$USER` | SSH user (needs passwordless sudo); required |
| `SSH_KEY` | `$HOME/.ssh/deploy` | SSH private key |
| `DOMAIN` | `$DEPLOY_HOST` | public TLS domain for the cert |
| `ACME_EMAIL` | _(required)_ | Let's Encrypt contact (no in-repo default) |
| `ALLOWED_DOMAINS` | _(empty)_ | Google SSO allowed email domains |
| `ACME_STAGING` | `0` | `1` = LE staging (untrusted, for testing rate limits) |
| `BACKEND` | `capdb` | capper db backend |
| `VERSION` | auto-bump patch in `./VERSION` | release version stamped into binaries + console |
| `BUMP_VERSION` | `1` | set `0` to deploy/build without incrementing `./VERSION` |
| `SKIP_BUILD` | `0` | `1` = reuse an existing `DIST/AIO/*.tgz` |
| `SKIP_TESTS` | `0` | `1` = skip the build-time test gate |

## Prerequisites
- **Local:** `go`, `cmake`, `cc`, `git` (for the build), plus `ssh`/`scp`.
- **DNS:** `DOMAIN` must resolve to the server, and ports **80/443** open
  (certbot uses HTTP-01 via nginx).
- **Server:** Ubuntu 24.04 (amd64), passwordless `sudo` for `DEPLOY_USER`.

## Re-running / upgrades
Idempotent. Re-running restages binaries and restarts services; node state
(`/etc/capper`) is only initialised on the first run. The cert is reused until
near expiry (`--keep-until-expiring`).

## Verify after deploy
```bash
curl https://cloud.cappervm.com/api/v1/health
ssh -i "$HOME/.ssh/deploy" "$USER@cloud.cappervm.com" 'capper aio status'
```
