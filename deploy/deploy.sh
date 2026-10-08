#!/usr/bin/env bash
# deploy.sh — Build the full Capper All-In-One stack (CapDB + capper +
# CapperWeb console), package it, ship it to cloud.cappervm.com, install it
# end-to-end, obtain a Let's Encrypt (ACME) TLS certificate, and verify the
# running system.
#
# Run this from the Capper repo root (or anywhere — it locates the repo
# relative to itself). It builds locally, then drives the remote host over SSH
# using the deploy key.
#
#   deploy/deploy.sh
#
# ── What it does ──────────────────────────────────────────────────────────────
#   1. Preflight: required local tools + SSH reachability.
#   2. Build:     scripts/build-aio.sh -> DIST/AIO/capper-aio-<ver>-linux-amd64.tgz
#   3. Ship:      scp the tarball + remote-setup.sh to the host.
#   4. Install:   run remote-setup.sh under sudo on the host (deps, install.sh,
#                 aio init/up, nginx reverse proxy, certbot ACME cert).
#   5. Verify:    service health + public HTTPS endpoint with a real cert.
#
# ── Configuration (env overrides) ─────────────────────────────────────────────
#   DEPLOY_HOST    SSH host                 (default cloud.cappervm.com)
#   DEPLOY_USER    SSH user                 (default $USER)
#   SSH_KEY        SSH private key          (default $HOME/.ssh/deploy)
#   DOMAIN         public TLS domain        (default = DEPLOY_HOST)
#   ACME_EMAIL     Let's Encrypt contact    (required — set env or deploy/local.env)
#   ACME_STAGING   1 = LE staging (testing) (default 0 = production cert)
#   BACKEND        capper db backend        (default capdb)
#   VERSION        release version          (default: auto-bump patch in ./VERSION)
#   BUMP_VERSION   1 = bump patch before build (default); 0 = use VERSION as-is
#   SKIP_BUILD     1 = reuse existing tgz   (default 0)
#   SKIP_TESTS     1 = skip build test gate (passed through to build-aio.sh)
set -euo pipefail

# ── Locations ─────────────────────────────────────────────────────────────────
HERE="$(CDPATH="" cd -- "$(dirname -- "$0")" && pwd)"
ROOT="$(CDPATH="" cd -- "$HERE/.." && pwd)"
cd "$ROOT"

# ── Local overrides (gitignored) ──────────────────────────────────────────────
LOCAL_ENV_FILE="${LOCAL_ENV_FILE:-$HERE/local.env}"
if [ -f "$LOCAL_ENV_FILE" ]; then
  # shellcheck disable=SC1090
  set -a; . "$LOCAL_ENV_FILE"; set +a
fi

# ── Configuration ─────────────────────────────────────────────────────────────
DEPLOY_HOST="${DEPLOY_HOST:-cloud.cappervm.com}"
DEPLOY_USER="${DEPLOY_USER:-${USER:-}}"
SSH_KEY="${SSH_KEY:-${HOME}/.ssh/deploy}"
DOMAIN="${DOMAIN:-$DEPLOY_HOST}"
ACME_EMAIL="${ACME_EMAIL:-}"
ACME_STAGING="${ACME_STAGING:-0}"
BACKEND="${BACKEND:-capdb}"
SKIP_BUILD="${SKIP_BUILD:-0}"
BUMP_VERSION="${BUMP_VERSION:-1}"

# ── Version ───────────────────────────────────────────────────────────────────
# Auto-increment patch on each deploy build unless VERSION is preset or bump disabled.
if [ -z "${VERSION:-}" ]; then
  if [ "$SKIP_BUILD" = "1" ] || [ "$BUMP_VERSION" = "0" ]; then
    if [ -f VERSION ]; then VERSION="$(tr -d ' \n\r' < VERSION)"; else VERSION="0.0.0-$(date +%Y%m%d)"; fi
  else
    VERSION="$(scripts/bump-version.sh patch)"
  fi
fi
# When both id+secret are present, oauth2-proxy gates the site to ALLOWED_DOMAINS.
OAUTH_ENV_FILE="${OAUTH_ENV_FILE:-$HERE/oauth2.env}"
if [ -f "$OAUTH_ENV_FILE" ]; then
  # shellcheck disable=SC1090
  set -a; . "$OAUTH_ENV_FILE"; set +a
fi
OAUTH2_CLIENT_ID="${OAUTH2_CLIENT_ID:-}"
OAUTH2_CLIENT_SECRET="${OAUTH2_CLIENT_SECRET:-}"
ALLOWED_DOMAINS="${ALLOWED_DOMAINS:-}"
# Optional: ensure this email is an active admin on deploy (first administrator;
# no self-registration). Idempotent if already admin.
BOOTSTRAP_ADMIN="${BOOTSTRAP_ADMIN:-}"
SSO_ENABLED=0
[ -n "$OAUTH2_CLIENT_ID" ] && [ -n "$OAUTH2_CLIENT_SECRET" ] && SSO_ENABLED=1

PKG="capper-aio-${VERSION}-linux-amd64"
TGZ="DIST/AIO/${PKG}.tgz"
SHA="DIST/AIO/${PKG}.tgz.sha256"
REMOTE_TMP="/tmp/capper-deploy.$$"

# ── Output helpers ────────────────────────────────────────────────────────────
if [ -t 1 ]; then C='\033[1;36m'; G='\033[1;32m'; R='\033[1;31m'; Z='\033[0m'
else C=''; G=''; R=''; Z=''; fi
say()  { printf "\n${C}==> %s${Z}\n" "$*"; }
ok()   { printf "${G}  ✓ %s${Z}\n" "$*"; }
die()  { printf "${R}  ✗ %s${Z}\n" "$*" >&2; exit 1; }

# Required deploy identity (no personal defaults checked into the repo).
[ -n "$ACME_EMAIL" ] || die "ACME_EMAIL is required (export ACME_EMAIL=you@example.com)"
[ -n "$DEPLOY_USER" ] || die "DEPLOY_USER is required (export DEPLOY_USER=…)"

# Connection multiplexing: route every ssh/scp through ONE TCP connection so a
# fail2ban-style jail sees a single login instead of one per step (which can
# trip the sshd jail and ban us mid-deploy). IPQoS=none avoids QoS/DSCP-mark
# drops on fussy paths; keepalives hold the master open between steps.
CONTROL_DIR="${TMPDIR:-/tmp}/capper-deploy-ssh.$$"
mkdir -p "$CONTROL_DIR"
SSH_OPTS=(-i "$SSH_KEY"
  -o StrictHostKeyChecking=accept-new
  -o ConnectTimeout=15
  -o IPQoS=none
  -o ServerAliveInterval=15
  -o ControlMaster=auto
  -o ControlPath="$CONTROL_DIR/cm-%r@%h:%p"
  -o ControlPersist=120s)
TARGET="${DEPLOY_USER}@${DEPLOY_HOST}"
ssh_()  { ssh "${SSH_OPTS[@]}" "$TARGET" "$@"; }
scp_()  { scp "${SSH_OPTS[@]}" "$@"; }

# Tear down the shared master connection (and its socket dir) on exit.
cleanup_ssh() {
  ssh "${SSH_OPTS[@]}" -O exit "$TARGET" 2>/dev/null || true
  rm -rf "$CONTROL_DIR" 2>/dev/null || true
}
trap cleanup_ssh EXIT

# ── 1. Preflight ──────────────────────────────────────────────────────────────
say "Preflight"
for t in ssh scp tar; do command -v "$t" >/dev/null 2>&1 || die "missing local tool: $t"; done
[ -f "$SSH_KEY" ] || die "SSH key not found: $SSH_KEY"
[ "$SKIP_BUILD" = "1" ] || for t in go cmake cc git; do
  command -v "$t" >/dev/null 2>&1 || die "missing build tool: $t (or set SKIP_BUILD=1)"
done
echo "  host:    $TARGET"
echo "  domain:  $DOMAIN  (ACME email: $ACME_EMAIL, staging: $ACME_STAGING)"
echo "  backend: $BACKEND"
echo "  version: $VERSION"
if [ "$SSO_ENABLED" = "1" ]; then
  echo "  SSO:     Google, domains: $ALLOWED_DOMAINS"
else
  printf "${R}  SSO:     DISABLED — no OAuth creds (set OAUTH2_CLIENT_ID/SECRET in %s); site will be OPEN${Z}\n" "$OAUTH_ENV_FILE"
fi

# Best-effort: detect our public egress IP so the remote can whitelist this
# network in fail2ban (the deploy opens several SSH/scp sessions; without this a
# strict sshd jail can ban us mid-run). Override with DEPLOY_SRC_IP.
DEPLOY_SRC_IP="${DEPLOY_SRC_IP:-$(curl -fsS --max-time 8 https://api.ipify.org 2>/dev/null || true)}"
[ -n "$DEPLOY_SRC_IP" ] && echo "  src IP:  $DEPLOY_SRC_IP (will be fail2ban-whitelisted)"

say "Checking SSH reachability"
ssh "${SSH_OPTS[@]}" -o BatchMode=yes "$TARGET" true \
  || die "cannot SSH to $TARGET with key $SSH_KEY (host/port reachable? VPN up? key authorized?)"
ok "SSH OK"
# Confirm sudo is usable non-interactively (deploy runs unattended over SSH).
ssh_ "sudo -n true" 2>/dev/null || die "passwordless sudo not available for $DEPLOY_USER on $DEPLOY_HOST"
ok "sudo OK"

# ── 2. Build ──────────────────────────────────────────────────────────────────
if [ "$SKIP_BUILD" = "1" ]; then
  say "SKIP_BUILD=1 — reusing $TGZ"
  [ -f "$TGZ" ] || die "no prebuilt tarball at $TGZ"
else
  say "Building AIO bundle (CapDB + capper + console) via scripts/build-aio.sh"
  if [ "$BUMP_VERSION" = "1" ]; then
    ok "VERSION -> $VERSION (patch bump)"
  fi
  SKIP_TESTS="${SKIP_TESTS:-0}" scripts/build-aio.sh "$VERSION"
fi
[ -f "$TGZ" ] || die "expected tarball missing: $TGZ"
ok "bundle: $TGZ ($(du -h "$TGZ" | cut -f1))"

# ── 3. Ship ───────────────────────────────────────────────────────────────────
say "Shipping bundle to $TARGET:$REMOTE_TMP"
ssh_ "mkdir -p $REMOTE_TMP"
scp_ "$TGZ" "$HERE/remote-setup.sh" "$TARGET:$REMOTE_TMP/"
[ -f "$SHA" ] && scp_ "$SHA" "$TARGET:$REMOTE_TMP/"
ok "uploaded"

# ── 4. Install end-to-end (remote, under sudo) ────────────────────────────────
say "Running remote install + ACME on $TARGET"
ssh_ "sudo \
  DOMAIN='$DOMAIN' \
  ACME_EMAIL='$ACME_EMAIL' \
  ACME_STAGING='$ACME_STAGING' \
  BACKEND='$BACKEND' \
  PKG='$PKG' \
  REMOTE_TMP='$REMOTE_TMP' \
  OAUTH2_CLIENT_ID='$OAUTH2_CLIENT_ID' \
  OAUTH2_CLIENT_SECRET='$OAUTH2_CLIENT_SECRET' \
  ALLOWED_DOMAINS='$ALLOWED_DOMAINS' \
  BOOTSTRAP_ADMIN='$BOOTSTRAP_ADMIN' \
  DEPLOY_SRC_IP='$DEPLOY_SRC_IP' \
  bash $REMOTE_TMP/remote-setup.sh"
ok "remote install complete"

# ── 5. Verify from the outside ────────────────────────────────────────────────
say "Verifying public HTTPS endpoint"
CURL_OPTS=(-sS --max-time 20)
[ "$ACME_STAGING" = "1" ] && CURL_OPTS+=(-k)  # staging certs aren't publicly trusted

# App-level auth: the login page is public (200 HTML), but the API must reject
# unauthenticated requests (401). Health stays open for probes.
curl "${CURL_OPTS[@]}" -f "https://${DOMAIN}/" | grep -qiE '<html|<!doctype' \
  && ok "login page served at https://${DOMAIN}/" \
  || printf "${R}  ! site did not return HTML (continuing)${Z}\n"

api_code="$(curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' "https://${DOMAIN}/api/v1/images")"
echo "  unauth GET /api/v1/images -> HTTP $api_code"
case "$api_code" in
  401) ok "API requires authentication (unauthenticated request rejected)" ;;
  200) die "API returned data WITHOUT auth — protection is broken" ;;
  *)   printf "${R}  ! unexpected API response: %s${Z}\n" "$api_code" ;;
esac

if [ "$SSO_ENABLED" = "1" ]; then
  # The Google login entrypoint should bounce an unauthenticated user to Google.
  gcode="$(curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code} %{redirect_url}' "https://${DOMAIN}/api/v1/auth/google/callback")"
  echo "  GET /api/v1/auth/google/callback -> $gcode"
  case "$gcode" in
    30[0-9]\ *) ok "Google login entrypoint redirects to sign-in" ;;
    *) printf "${R}  ! google callback did not redirect: %s${Z}\n" "$gcode" ;;
  esac
fi

say "Certificate"
echo | openssl s_client -servername "$DOMAIN" -connect "${DOMAIN}:443" 2>/dev/null \
  | openssl x509 -noout -issuer -subject -dates 2>/dev/null || true

# ── Cleanup ───────────────────────────────────────────────────────────────────
ssh_ "rm -rf $REMOTE_TMP" || true

say "Done"
ok "Capper AIO $VERSION deployed to $DEPLOY_HOST"
echo "  Console:  https://${DOMAIN}/"
echo "  Health:   https://${DOMAIN}/api/v1/health"
echo "  Manage:   ssh -i $SSH_KEY $TARGET 'capper aio status'"
