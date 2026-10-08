#!/usr/bin/env bash
# remote-setup.sh — runs ON cloud.cappervm.com under sudo, driven by deploy.sh.
#
# Installs the Capper AIO bundle, brings up the single-node stack (CapDB +
# control plane + agent), fronts it with nginx + a Let's Encrypt (ACME) cert,
# and — when Google OAuth credentials are supplied — gates the whole site behind
# Google SSO restricted to the allowed email domains via oauth2-proxy.
#
# Idempotent: safe to re-run for upgrades. It restages binaries, restarts
# services, and only initialises node state the first time.
#
# Expects (exported by deploy.sh):
#   DOMAIN            public TLS domain
#   ACME_EMAIL        Let's Encrypt contact email
#   ACME_STAGING      1 = LE staging environment, 0 = production
#   BACKEND           capper db backend (capdb|sqlite)
#   PKG               bundle basename (capper-aio-<ver>-linux-amd64)
#   REMOTE_TMP        dir on this host holding <PKG>.tgz + this script
#   OAUTH2_CLIENT_ID      Google OAuth client id      (empty => SSO disabled)
#   OAUTH2_CLIENT_SECRET  Google OAuth client secret  (empty => SSO disabled)
#   ALLOWED_DOMAINS       comma-separated email domains (e.g. example.com)
set -euo pipefail

[ "$(id -u)" -eq 0 ] || { echo "remote-setup.sh must run as root" >&2; exit 1; }

DOMAIN="${DOMAIN:?DOMAIN required}"
ACME_EMAIL="${ACME_EMAIL:?ACME_EMAIL required}"
ACME_STAGING="${ACME_STAGING:-0}"
BACKEND="${BACKEND:-capdb}"
PKG="${PKG:?PKG required}"
REMOTE_TMP="${REMOTE_TMP:?REMOTE_TMP required}"
OAUTH2_CLIENT_ID="${OAUTH2_CLIENT_ID:-}"
OAUTH2_CLIENT_SECRET="${OAUTH2_CLIENT_SECRET:-}"
ALLOWED_DOMAINS="${ALLOWED_DOMAINS:-}"

CONTROL_ADDR="127.0.0.1:8080"   # control plane HTTP; nginx terminates TLS in front
CONSOLE_LINK="/opt/capper/console"
OAUTH2_ADDR="127.0.0.1:4180"
WEBROOT="/var/www/letsencrypt"
SSO_ENABLED=0
[ -n "$OAUTH2_CLIENT_ID" ] && [ -n "$OAUTH2_CLIENT_SECRET" ] && SSO_ENABLED=1

export DEBIAN_FRONTEND=noninteractive
export PATH="/usr/local/bin:$PATH"
export HOME=/root

say() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
ok()  { printf '\033[1;32m  ✓ %s\033[0m\n' "$*"; }
die() { printf '\033[1;31m  ✗ %s\033[0m\n' "$*" >&2; exit 1; }

# ── Dependencies ──────────────────────────────────────────────────────────────
say "Installing OS dependencies"
apt-get update -qq
apt-get install -y -qq \
  openssl libssl3 ca-certificates curl tar python3 \
  nginx certbot \
  bubblewrap crun
ok "deps installed"

# Whitelist the deploy network in fail2ban so repeated SSH/scp sessions during a
# deploy can't get the operator banned mid-run (and proactively unban it).
if command -v fail2ban-client >/dev/null 2>&1 && [ -n "${DEPLOY_SRC_IP:-}" ]; then
  net24="$(echo "$DEPLOY_SRC_IP" | awk -F. 'NF==4{print $1"."$2"."$3".0/24"}')"
  cat > /etc/fail2ban/jail.d/capper-deploy.local <<EOF
[DEFAULT]
ignoreip = 127.0.0.1/8 ::1 ${DEPLOY_SRC_IP}/32 ${net24}
EOF
  systemctl reload fail2ban 2>/dev/null || systemctl restart fail2ban 2>/dev/null || true
  fail2ban-client set sshd unbanip "$DEPLOY_SRC_IP" >/dev/null 2>&1 || true
  ok "fail2ban whitelisted ${DEPLOY_SRC_IP} / ${net24}"
fi

# ── Unpack + verify ───────────────────────────────────────────────────────────
say "Unpacking bundle"
cd "$REMOTE_TMP"
if [ -f "${PKG}.tgz.sha256" ]; then
  sha256sum -c "${PKG}.tgz.sha256" || die "checksum mismatch on ${PKG}.tgz"
  ok "checksum verified"
fi
rm -rf -- "${REMOTE_TMP:?}/${PKG:?}"
tar xzf "${PKG}.tgz" -C "$REMOTE_TMP"
[ -d "$REMOTE_TMP/$PKG" ] || die "extracted dir not found: $PKG"

# ── Install binaries + console (versioned, atomic symlink flip) ───────────────
say "Installing release (install.sh)"
( cd "$REMOTE_TMP/$PKG" && ./install.sh )
ok "binaries + console installed"

# ── Initialise node state (first run only) ────────────────────────────────────
if [ ! -f /etc/capper/aio.yaml ]; then
  say "Initialising AIO node (backend: $BACKEND)"
  capper aio init --backend "$BACKEND"
  ok "node initialised"
else
  say "Existing node config detected — skipping init (upgrade path)"
fi

# ── Control-plane listen address + console (systemd drop-in) ──────────────────
# Override ExecStart so the control plane binds CONTROL_ADDR and serves the
# console; nginx reverse-proxies public TLS to it. Sorted after install.sh's
# 10-console.conf so this wins. HOME is required: capper resolves config/data
# dirs via os.UserHomeDir(), which fails with "$HOME is not defined" under
# systemd (no HOME by default).
say "Configuring control-plane service ($CONTROL_ADDR + console)"
install -d -m 0755 /etc/systemd/system/capper-control.service.d
cat > /etc/systemd/system/capper-control.service.d/20-listen.conf <<EOF
[Service]
Environment=HOME=/root
# Optional (leading '-'): RBAC/SSO settings (proxy secret + allowed domains).
# Written by the SSO step below; absent on non-SSO deploys.
EnvironmentFile=-/etc/capper/control.env
ExecStart=
# --runtime bwrap: use the bubblewrap runtime explicitly so a missing/broken
# runtime fails loudly instead of silently degrading to chroot.
ExecStart=/usr/local/bin/capper --runtime bwrap api start --with-daemon --listen ${CONTROL_ADDR} --console ${CONSOLE_LINK}
EOF
install -d -m 0755 /etc/systemd/system/capper-agent.service.d
cat > /etc/systemd/system/capper-agent.service.d/20-home.conf <<EOF
[Service]
Environment=HOME=/root
EOF
systemctl daemon-reload

# ── Start / restart services ──────────────────────────────────────────────────
say "Enabling and (re)starting services"
SVCS=(capper-control capper-agent)
systemctl list-unit-files | grep -q '^capdb-server\.service' && SVCS=(capdb-server "${SVCS[@]}")
systemctl enable "${SVCS[@]}" >/dev/null 2>&1 || true
for s in "${SVCS[@]}"; do
  systemctl restart "$s"
  ok "restarted $s"
done

# ── Wait for control-plane health ─────────────────────────────────────────────
say "Waiting for control plane (http://${CONTROL_ADDR}/api/v1/health)"
for i in $(seq 1 30); do
  if curl -fsS --max-time 3 "http://${CONTROL_ADDR}/api/v1/health" >/dev/null 2>&1; then
    ok "control plane healthy"; break
  fi
  [ "$i" -eq 30 ] && { journalctl -u capper-control --no-pager -n 40 || true; die "control plane did not become healthy"; }
  sleep 1
done

# ── Admin bearer (used for node bootstrap below + the nginx SSO bridge) ────────
# Minted via the local CLI against the SAME backend as the control plane (source
# its env), as root so the bootstrapped admin grant applies.
say "Minting capper admin token"
set -a; [ -f /etc/capper/capdb.env ] && . /etc/capper/capdb.env; set +a
CAPPER_BEARER="$(capper iam token create --json --name aio-bootstrap --ttl 87600h \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')"
[ -n "$CAPPER_BEARER" ] || die "failed to mint capper admin token"
printf '%s' "$CAPPER_BEARER" > /etc/capper/proxy-bearer
chmod 600 /etc/capper/proxy-bearer
ok "admin token minted"

# ── Bootstrap the local node so the agent can come online ─────────────────────
# AIO ships no automatic node bootstrap: the agent's join sends no join token but
# the server requires one, so join 401s forever. We register the node here via
# the API (join is auth-exempt; topology + join token created with the admin
# bearer) and pre-seed the agent's credential files, which makes the agent's
# loadOrJoin skip the HTTP join entirely.
say "Bootstrapping local node (agent registration)"
base="http://${CONTROL_ADDR}/api/v1"
AUTH=(-H "Authorization: Bearer ${CAPPER_BEARER}" -H "Content-Type: application/json")
jval() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }

NODE_NAME="$(awk '/^[[:space:]]*name:/{print $2; exit}' /etc/capper/agent.yaml 2>/dev/null)"
[ -n "$NODE_NAME" ] || NODE_NAME="$(hostname -s)"
ROLES='["all-in-one","control-plane","compute","shared-disk","s3","network","ingress"]'

need_bootstrap=1
if [ -s /etc/capper/node-id ] && [ -s /etc/capper/agent-token ]; then
  nid="$(cat /etc/capper/node-id)"
  curl -fsS "${AUTH[@]}" "$base/nodes/$nid" >/dev/null 2>&1 \
    && { need_bootstrap=0; ok "node already registered ($nid)"; }
fi

if [ "$need_bootstrap" = "1" ]; then
  # Ensure topology exists (create-or-ignore, then read the assigned IDs).
  curl -fsS "${AUTH[@]}" -X POST "$base/realms"  -d '{"name":"Local","slug":"local"}' >/dev/null 2>&1 || true
  REALM_ID="$(curl -fsS "${AUTH[@]}" "$base/realms/local"   | jval '["data"]["id"]')"
  curl -fsS "${AUTH[@]}" -X POST "$base/regions" -d "{\"name\":\"Local\",\"slug\":\"local\",\"realmId\":\"$REALM_ID\"}" >/dev/null 2>&1 || true
  REGION_ID="$(curl -fsS "${AUTH[@]}" "$base/regions/local" | jval '["data"]["id"]')"
  curl -fsS "${AUTH[@]}" -X POST "$base/zones"   -d "{\"name\":\"Local A\",\"slug\":\"local-a\",\"realmId\":\"$REALM_ID\",\"regionId\":\"$REGION_ID\"}" >/dev/null 2>&1 || true
  ZONE_ID="$(curl -fsS "${AUTH[@]}" "$base/zones/local-a" | jval '["data"]["id"]')"
  [ -n "$REALM_ID" ] && [ -n "$REGION_ID" ] && [ -n "$ZONE_ID" ] || die "topology bootstrap failed (realm=$REALM_ID region=$REGION_ID zone=$ZONE_ID)"

  JT="$(curl -fsS "${AUTH[@]}" -X POST "$base/join-tokens" \
    -d "{\"realmId\":\"$REALM_ID\",\"regionId\":\"$REGION_ID\",\"zoneId\":\"$ZONE_ID\",\"roles\":$ROLES,\"ttl\":\"1h\",\"uses\":1}" \
    | jval '["data"]["token"]')"
  [ -n "$JT" ] || die "failed to create join token"

  # join is auth-exempt; the join token in the body authorizes registration.
  RESP="$(curl -fsS -H "Content-Type: application/json" -X POST "$base/nodes/join" \
    -d "{\"token\":\"$JT\",\"name\":\"$NODE_NAME\",\"address\":\"127.0.0.1\",\"roles\":$ROLES,\"agentVersion\":\"aio\"}")"
  NODE_ID="$(printf '%s' "$RESP"     | jval '["data"]["node"]["id"]')"
  NODE_BEARER="$(printf '%s' "$RESP" | jval '["data"]["bearer"]')"
  [ -n "$NODE_ID" ] && [ -n "$NODE_BEARER" ] || die "node join failed: $RESP"

  curl -fsS "${AUTH[@]}" -X POST "$base/nodes/$NODE_ID/approve" >/dev/null 2>&1 || true

  printf '%s' "$NODE_ID"     > /etc/capper/node-id;     chmod 600 /etc/capper/node-id
  printf '%s' "$NODE_BEARER" > /etc/capper/agent-token; chmod 600 /etc/capper/agent-token
  ok "node registered + approved ($NODE_ID)"
  systemctl restart capper-agent
fi

# Wait for the agent to come online now that it has credentials.
for i in $(seq 1 15); do
  systemctl is-active --quiet capper-agent && break
  sleep 1
done

# Default VPC + subnet so instances can reach the metadata service
# (169.254.169.254) for capinit (hostname, etc.).
if curl -fsS "${AUTH[@]}" "$base/vpcs" 2>/dev/null | grep -q '"slug":"default-vpc"'; then
  ok "default vpc present"
else
  say "Creating default VPC and subnet"
  curl -fsS "${AUTH[@]}" -X POST "$base/vpcs" \
    -d '{"slug":"default-vpc","name":"default","cidr":"10.88.0.0/16","status":"active"}' >/dev/null \
    && ok "default vpc created" || die "default vpc create failed"
  curl -fsS "${AUTH[@]}" -X POST "$base/vpcs/default-vpc/subnets" \
    -d '{"name":"default","cidr":"10.88.1.0/24","zone":"z1"}' >/dev/null \
    && ok "default subnet created" || die "default subnet create failed"
fi

# Default storage pool for instance and volume disks.
STORE_ROOT="${CAPPER_STORE:-/var/lib/capper}"
POOL_DIR="$STORE_ROOT/storage-pool"
mkdir -p "$POOL_DIR"
if curl -fsS "${AUTH[@]}" "$base/admin/storage/settings" 2>/dev/null | grep -q '"defaultInstancePool":"'; then
  ok "default storage pool configured"
else
  say "Registering default storage pool"
  POOL_JSON="$(curl -fsS "${AUTH[@]}" -X POST "$base/admin/storage-pools" \
    -d "{\"name\":\"default\",\"backend\":\"directory\",\"mountpoint\":\"$POOL_DIR\",\"totalBytes\":107374182400}")"
  POOL_ID="$(printf '%s' "$POOL_JSON" | grep -oP '"id"\s*:\s*"\K[^"]+' | head -1)"
  [ -n "$POOL_ID" ] || die "storage pool create failed"
  curl -fsS "${AUTH[@]}" -X PUT "$base/admin/storage/settings" \
    -d "{\"defaultInstancePool\":\"$POOL_ID\"}" >/dev/null \
    && ok "default storage pool configured" || die "storage settings update failed"
fi

# Seed the base images shipped in the bundle (alpine, alma, …). Always
# (re)upload so image updates ship; upsert by name.
for cap in "$REMOTE_TMP/$PKG"/images/*.cap "$REMOTE_TMP/$PKG"/*.cap; do
  [ -f "$cap" ] || continue
  nm="$(basename "$cap" .cap)"
  say "Uploading base image ($nm)"
  curl -fsS -H "Authorization: Bearer ${CAPPER_BEARER}" \
    -F file=@"$cap" -F name="$nm" "$base/images/upload" >/dev/null \
    && ok "image '$nm' registered" || die "image upload failed: $nm"
done

# ──────────────────────────────────────────────────────────────────────────────
# Google SSO via oauth2-proxy (only when credentials supplied)
# ──────────────────────────────────────────────────────────────────────────────
if [ "$SSO_ENABLED" = "1" ]; then
  say "Configuring Google SSO (oauth2-proxy) for domains: $ALLOWED_DOMAINS"

  # Install the oauth2-proxy binary (latest release) if missing.
  if ! command -v oauth2-proxy >/dev/null 2>&1; then
    ver="$(curl -fsSL https://api.github.com/repos/oauth2-proxy/oauth2-proxy/releases/latest \
            | grep -oP '"tag_name":\s*"\K[^"]+' || true)"
    ver="${ver:-v7.7.1}"
    url="https://github.com/oauth2-proxy/oauth2-proxy/releases/download/${ver}/oauth2-proxy-${ver}.linux-amd64.tar.gz"
    say "Downloading oauth2-proxy ${ver}"
    curl -fsSL "$url" -o "$REMOTE_TMP/o2p.tgz" || die "failed to download oauth2-proxy from $url"
    tar xzf "$REMOTE_TMP/o2p.tgz" -C "$REMOTE_TMP"
    install -m 0755 "$REMOTE_TMP"/oauth2-proxy-*/oauth2-proxy /usr/local/bin/oauth2-proxy
    ok "oauth2-proxy installed ($(oauth2-proxy --version 2>&1 | head -1))"
  else
    ok "oauth2-proxy present ($(oauth2-proxy --version 2>&1 | head -1))"
  fi

  install -d -m 0700 /etc/oauth2-proxy

  # Persistent cookie secret (rotating it would log everyone out on each deploy).
  if [ ! -s /etc/oauth2-proxy/cookie-secret ]; then
    python3 -c 'import secrets,base64;print(base64.urlsafe_b64encode(secrets.token_bytes(32)).decode())' \
      > /etc/oauth2-proxy/cookie-secret
    chmod 600 /etc/oauth2-proxy/cookie-secret
  fi
  COOKIE_SECRET="$(cat /etc/oauth2-proxy/cookie-secret)"

  # email_domains TOML array from the comma list.
  domains_toml=""
  IFS=',' read -ra _doms <<< "$ALLOWED_DOMAINS"
  for d in "${_doms[@]}"; do
    d="$(echo "$d" | xargs)"   # trim
    [ -n "$d" ] && domains_toml="${domains_toml}\"${d}\", "
  done
  domains_toml="[ ${domains_toml%, } ]"

  cat > /etc/oauth2-proxy/oauth2-proxy.cfg <<EOF
http_address      = "${OAUTH2_ADDR}"
reverse_proxy     = true
provider          = "google"
client_id         = "${OAUTH2_CLIENT_ID}"
client_secret     = "${OAUTH2_CLIENT_SECRET}"
cookie_secret     = "${COOKIE_SECRET}"
email_domains     = ${domains_toml}
redirect_url      = "https://${DOMAIN}/oauth2/callback"
cookie_secure     = true
cookie_domains    = ["${DOMAIN}"]
whitelist_domains = ["${DOMAIN}"]
set_xauthrequest  = true
skip_provider_button = true
upstreams         = ["static://202"]
EOF
  chmod 600 /etc/oauth2-proxy/oauth2-proxy.cfg

  cat > /etc/systemd/system/oauth2-proxy.service <<EOF
[Unit]
Description=oauth2-proxy (Google SSO gate for Capper)
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/oauth2-proxy --config=/etc/oauth2-proxy/oauth2-proxy.cfg
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable oauth2-proxy >/dev/null 2>&1 || true
  systemctl restart oauth2-proxy
  ok "oauth2-proxy running on ${OAUTH2_ADDR}"

  # Shared secret between nginx and capper for the trusted-proxy identity bridge.
  # nginx injects it (X-Capper-Proxy-Secret) only for oauth2-authenticated
  # requests; capper trusts the forwarded X-Auth-Request-Email when it matches.
  # Persisted so it stays stable across deploys.
  if [ ! -s /etc/capper/proxy-secret ]; then
    python3 -c 'import secrets;print(secrets.token_urlsafe(32))' > /etc/capper/proxy-secret
    chmod 600 /etc/capper/proxy-secret
  fi
  PROXY_SECRET="$(cat /etc/capper/proxy-secret)"

  # Control-plane RBAC env: per-user identity via the proxy secret + server-side
  # domain allowlist (defense-in-depth alongside oauth2-proxy).
  cat > /etc/capper/control.env <<EOF
CAPPER_PROXY_SECRET=${PROXY_SECRET}
CAPPER_ALLOWED_DOMAINS=${ALLOWED_DOMAINS}
CAPPER_BOOTSTRAP_ADMIN=${BOOTSTRAP_ADMIN:-}
EOF
  chmod 600 /etc/capper/control.env
  systemctl restart capper-control
  # Re-wait for health after the restart picks up the RBAC env.
  for i in $(seq 1 30); do
    curl -fsS --max-time 3 "http://${CONTROL_ADDR}/api/v1/health" >/dev/null 2>&1 && break
    [ "$i" -eq 30 ] && die "control plane unhealthy after RBAC env restart"
    sleep 1
  done
  ok "control plane configured for per-user SSO identity"
fi

# ──────────────────────────────────────────────────────────────────────────────
# nginx + ACME certificate
# ──────────────────────────────────────────────────────────────────────────────
say "Configuring nginx + ACME for $DOMAIN (SSO: $SSO_ENABLED)"
install -d -m 0755 "$WEBROOT"
rm -f /etc/nginx/sites-enabled/default

# WebSocket/SSE upgrade mapping (http context).
cat > /etc/nginx/conf.d/capper-upgrade.conf <<'EOF'
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}
EOF

# Stage 1 — minimal HTTP server so nginx can serve the ACME http-01 challenge
# (and the app, pre-cert). Lets certbot --webroot succeed without TLS existing.
cat > /etc/nginx/sites-available/capper.conf <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};
    location /.well-known/acme-challenge/ { root ${WEBROOT}; }
    location / {
        proxy_pass http://${CONTROL_ADDR};
        proxy_set_header Host \$host;
    }
}
EOF
ln -sfn /etc/nginx/sites-available/capper.conf /etc/nginx/sites-enabled/capper.conf
nginx -t || die "nginx (stage 1) config test failed"
systemctl enable nginx >/dev/null 2>&1 || true
systemctl restart nginx

# Obtain / renew the certificate via the webroot (does NOT rewrite our config).
say "Obtaining ACME certificate"
CB_ARGS=(certonly --webroot -w "$WEBROOT" -d "$DOMAIN"
         --non-interactive --agree-tos -m "$ACME_EMAIL" --keep-until-expiring)
[ "$ACME_STAGING" = "1" ] && CB_ARGS+=(--staging)
certbot "${CB_ARGS[@]}" || die "certbot failed (DNS for $DOMAIN -> here? ports 80/443 open?)"
CERT_DIR="/etc/letsencrypt/live/${DOMAIN}"
[ -f "$CERT_DIR/fullchain.pem" ] || die "certificate not found at $CERT_DIR"
ok "certificate ready"

# Stage 2 — final config: HTTP->HTTPS redirect (+ ACME), HTTPS terminating TLS,
# optionally gated by oauth2-proxy with the capper bearer injected downstream.
proxy_common='        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $connection_upgrade;
        proxy_read_timeout 3600s;
        proxy_buffering off;'

{
cat <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};
    location /.well-known/acme-challenge/ { root ${WEBROOT}; }
    location / { return 301 https://\$host\$request_uri; }
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name ${DOMAIN};

    ssl_certificate     ${CERT_DIR}/fullchain.pem;
    ssl_certificate_key ${CERT_DIR}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;

    client_max_body_size 0;
EOF

if [ "$SSO_ENABLED" = "1" ]; then
cat <<EOF

    # oauth2-proxy endpoints (Google login, callback, sign-in/out).
    location /oauth2/ {
        proxy_pass       http://${OAUTH2_ADDR};
        proxy_set_header Host                    \$host;
        proxy_set_header X-Real-IP               \$remote_addr;
        proxy_set_header X-Scheme                \$scheme;
        proxy_set_header X-Auth-Request-Redirect \$request_uri;
    }
    location = /oauth2/auth {
        proxy_pass       http://${OAUTH2_ADDR};
        proxy_set_header Host             \$host;
        proxy_set_header X-Real-IP        \$remote_addr;
        proxy_set_header X-Scheme         \$scheme;
        proxy_set_header Content-Length   "";
        proxy_pass_request_body           off;
    }

    # Google login completion: requires an oauth2-proxy session (start the flow
    # if absent), then capper maps the verified email to an existing user and
    # mints a capper session. Only THIS path forwards the proxy identity.
    location = /api/v1/auth/google/callback {
        auth_request /oauth2/auth;
        error_page 401 = /oauth2/sign_in?rd=%2Fapi%2Fv1%2Fauth%2Fgoogle%2Fcallback;
        auth_request_set \$email \$upstream_http_x_auth_request_email;
        proxy_set_header  X-Auth-Request-Email  \$email;
        proxy_set_header  X-Capper-Proxy-Secret "${PROXY_SECRET}";
        proxy_pass http://${CONTROL_ADDR};
${proxy_common}
    }

    # App + API are publicly reachable (so the login page loads); capper enforces
    # authentication per request (401 without a session). Strip any inbound proxy
    # secret so only the callback path above can assert an SSO identity.
    location / {
        proxy_set_header X-Capper-Proxy-Secret "";
        proxy_pass http://${CONTROL_ADDR};
${proxy_common}
    }
}
EOF
else
cat <<EOF

    location / {
        proxy_pass http://${CONTROL_ADDR};
${proxy_common}
    }
}
EOF
fi
} > /etc/nginx/sites-available/capper.conf

nginx -t || die "nginx (final) config test failed"
systemctl reload nginx
systemctl enable --now certbot.timer >/dev/null 2>&1 || true
ok "nginx serving https://${DOMAIN}/"

# ── Local verification ────────────────────────────────────────────────────────
say "Verifying locally"
capper aio status || true
for s in "${SVCS[@]}"; do
  systemctl is-active --quiet "$s" && ok "$s active" || die "$s not active"
done
if [ "$SSO_ENABLED" = "1" ]; then
  systemctl is-active --quiet oauth2-proxy && ok "oauth2-proxy active" || die "oauth2-proxy not active"
fi
curl -fsS --max-time 5 "http://${CONTROL_ADDR}/api/v1/health" >/dev/null \
  && ok "local control-plane health OK" || die "local health failed"

say "Remote setup complete for $DOMAIN"
