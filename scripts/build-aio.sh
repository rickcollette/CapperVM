#!/usr/bin/env bash
# Build, test, and (on success) package the Capper All-In-One bundle.
# Output: DIST/AIO/capper-aio-<version>-<platform>.tgz
#
# Usage:
#   scripts/build-aio.sh [VERSION]
#
# Environment overrides:
#   CAPDB_DIR      CapDB checkout dir (default ./CapDB; cloned via make capdb-fetch)
#   CAPDB_BUILD    CapDB cmake build dir (default ./build/capdb; matrix uses /tmp)
#   CAPPERWEB_DIR  CapperWeb checkout for the console (default ../CapperWeb)
#   SKIP_WEB=1     skip the npm console build (ships no console/)
#   SKIP_TESTS=1   skip the test gate (build + package only; not recommended)
#   BUMP_VERSION=1 when no VERSION arg: auto-increment patch in ./VERSION (default 1)
#   PLATFORM_SUFFIX artifact suffix (default linux-amd64; matrix sets
#                   ubuntu24.04-glibc2.39-x86_64, debian12-..., etc.)
set -euo pipefail

# ── Locations ─────────────────────────────────────────────────────────────────
ROOT="$(CDPATH="" cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"

CAPDB_DIR="${CAPDB_DIR:-CapDB}"
CAPPERWEB_DIR="${CAPPERWEB_DIR:-$ROOT/../CapperWeb}"
BUILD_CAPDB="${CAPDB_BUILD:-$ROOT/build/capdb}"

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  if [ "${BUMP_VERSION:-1}" = "1" ]; then
    VERSION="$(scripts/bump-version.sh patch)"
    echo "Bumped VERSION -> $VERSION"
  elif [ -f VERSION ]; then
    VERSION="$(tr -d ' \n\r' < VERSION)"
  else
    VERSION="0.0.0-$(date +%Y%m%d)"
  fi
fi

PLATFORM_SUFFIX="${PLATFORM_SUFFIX:-linux-amd64}"
PKG="capper-aio-${VERSION}-${PLATFORM_SUFFIX}"
OUT_DIR="$ROOT/DIST/AIO"
STAGE="$OUT_DIR/stage/$PKG"

say() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

# ── Preflight ─────────────────────────────────────────────────────────────────
say "Preflight"
for t in go cmake cc tar git; do
  command -v "$t" >/dev/null 2>&1 || { echo "error: missing tool '$t'" >&2; exit 1; }
done

# Ensure the CapDB engine is checked out (clone/update from GitHub into ./CapDB).
CAPDB_DIR="$CAPDB_DIR" make capdb-fetch
CAPDB_DIR_ABS="$(CDPATH="" cd -- "$CAPDB_DIR" && pwd)"
[ -d "$CAPDB_DIR_ABS/capdb/client" ] || { echo "error: CapDB tree not found at $CAPDB_DIR_ABS" >&2; exit 1; }

# cgo paths for the capdb build tag (mirrors the Makefile).
export CAPDB_DIR
export CGO_CFLAGS="-I${CAPDB_DIR_ABS}/capdb/client"
export CGO_LDFLAGS="${BUILD_CAPDB}/libcapdb_client.a"

echo "Go:        $(go version)"
echo "CapDB:     $CAPDB_DIR_ABS"
echo "Version:   $VERSION"

# ── Build ─────────────────────────────────────────────────────────────────────
# Version stamping (matches the Makefile). CAPPER_VERSION defaults to $VERSION.
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-X capper/internal/version.Version=${VERSION} -X capper/internal/version.Commit=${COMMIT} -X capper/internal/version.BuildDate=${BUILD_DATE}"

say "Building CapDB engine (server + client lib)"
make capdb CAPDB_DIR="$CAPDB_DIR" CAPDB_BUILD="$BUILD_CAPDB"

say "Building capper (cgo + capdb backend, version $VERSION)"
make build-capdb CAPPER_VERSION="$VERSION" CAPDB_DIR="$CAPDB_DIR" CAPDB_BUILD="$BUILD_CAPDB"

say "Building capper-agent and capinit (static, pure-Go, version $VERSION)"
mkdir -p bin
CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o bin/capper-agent ./cmd/capper-agent
CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o bin/capinit ./cmd/capinit

if [ "${SKIP_WEB:-0}" = "1" ]; then
  echo "SKIP_WEB=1 — not building the console"
elif [ -d "$CAPPERWEB_DIR" ]; then
  say "Building web console ($CAPPERWEB_DIR, profile=aio)"
  # VITE_PROFILE=aio strips cluster/multi-server features (topology, compute
  # groups, VPCs, marketplace, orgs, governance) from the single-node console.
  ( cd "$CAPPERWEB_DIR" && VITE_PROFILE=aio VITE_CAPPER_VERSION="$VERSION" scripts/build.sh )
else
  echo "warning: CapperWeb not found at $CAPPERWEB_DIR — packaging without a console" >&2
fi

# ── Test gate ─────────────────────────────────────────────────────────────────
if [ "${SKIP_TESTS:-0}" = "1" ]; then
  echo "SKIP_TESTS=1 — skipping the test gate"
else
  say "Tests: pure-Go suite"
  go build ./...
  go vet ./...
  go test ./...

  say "Tests: CapDB driver conformance + store integration"
  make test-capdb
  CAPDB_SERVER="$BUILD_CAPDB/capdb-server" \
    go test -tags capdb ./internal/store/ -run SelfHeal -count=1
fi

# ── Package ───────────────────────────────────────────────────────────────────
say "Packaging $PKG"
rm -rf "$OUT_DIR/stage"
mkdir -p "$STAGE/bin"

install -m 0755 bin/capper            "$STAGE/bin/capper"
install -m 0755 bin/capper-agent      "$STAGE/bin/capper-agent"
install -m 0755 bin/capinit           "$STAGE/bin/capinit"
install -m 0755 "$BUILD_CAPDB/capdb-server" "$STAGE/bin/capdb-server"

if [ -d "$CAPPERWEB_DIR/dist" ] && [ "${SKIP_WEB:-0}" != "1" ]; then
  mkdir -p "$STAGE/console"
  cp -a "$CAPPERWEB_DIR/dist/." "$STAGE/console/"
fi

install -m 0755 scripts/aio-install.sh "$STAGE/install.sh"
printf '%s\n' "$VERSION" > "$STAGE/VERSION"

# Sample image: build alpine.cap so a fresh node ships with at least one
# launchable image. The .cap is backend-agnostic; built with the default
# (sqlite) store in a throwaway dir, then staged into the bundle.
# Sample base images: build alpine.cap and alma.cap (both require docker) so a
# fresh node ships with launchable base images. capinit is staged into each
# rootfs so it runs on boot. Built with the default (sqlite) store.
build_sample_image() {
  local key="$1" dir="examples/$1" cap="$1.cap"
  [ -f "$dir/capper.json" ] || { echo "warning: $dir/capper.json missing — skipping $cap" >&2; return; }
  say "Building sample image $cap"
  sh "$dir/bootstrap.sh"
  install -m 0755 bin/capinit "$dir/rootfs/sbin/capinit"
  local work="$OUT_DIR/capwork"
  rm -rf "$work"; mkdir -p "$work/store"
  ./bin/capper --store "$work/store" create "$key" "$dir/capper.json"
  cp "$work/store/images/$cap" "$STAGE/$cap"
  rm -rf "$work"
  echo "staged sample image: $cap ($(du -h "$STAGE/$cap" | cut -f1))"
}

if [ "${SKIP_IMAGE:-0}" = "1" ]; then
  echo "SKIP_IMAGE=1 — not building sample images"
else
  if command -v docker >/dev/null 2>&1; then
    build_sample_image alpine
    build_sample_image alma
  else
    echo "warning: docker not found — skipping alpine.cap and alma.cap sample images" >&2
  fi
fi

cat > "$STAGE/README.md" <<EOF
# Capper All-In-One — $VERSION ($PLATFORM_SUFFIX)

Single-node Capper: control plane, node agent, and CapDB SQL backend.

## Install
\`\`\`
tar xzf $PKG.tgz
cd $PKG
sudo ./install.sh
\`\`\`
Installs \`capper\`, \`capper-agent\`, \`capinit\`, \`capdb-server\` to
\`/usr/local/bin\` and the console to \`/opt/capper/console\`.

## Run
\`\`\`
sudo capper aio init --backend capdb   # provisions /etc/capper, TLS, systemd units
sudo capper aio up                     # starts capdb-server + control plane + agent
capper aio status
\`\`\`

## Runtime requirements
- Ubuntu 24.04 (amd64), systemd, cgroup v2
- OpenSSL 3 (\`libssl.so.3\`): \`sudo apt-get install -y openssl libssl3\`
EOF

say "Creating tarball"
mkdir -p "$OUT_DIR"
tar czf "$OUT_DIR/$PKG.tgz" -C "$OUT_DIR/stage" "$PKG"
( cd "$OUT_DIR" && sha256sum "$PKG.tgz" > "$PKG.tgz.sha256" )
rm -rf "$OUT_DIR/stage"

# Update channel feed (consumed by `capper aio upgrade --channel`). Defaults to
# the "stable" channel; override CHANNEL / FEED_BASE_URL for releases.
CHANNEL="${CHANNEL:-stable}"
SHA="$(cut -d' ' -f1 < "$OUT_DIR/$PKG.tgz.sha256")"
FEED_BASE_URL="${FEED_BASE_URL:-https://downloads.example.com/capper/aio}"
say "Writing channel feed entry ($CHANNEL -> $VERSION)"
python3 - "$OUT_DIR/channels.json" "$CHANNEL" "$VERSION" "$FEED_BASE_URL/$PKG.tgz" "$SHA" <<'PY' || \
  printf '{"%s":{"version":"%s","url":"%s","sha256":"%s","minUpgradeFrom":"0.0.0"}}\n' \
    "$CHANNEL" "$VERSION" "$FEED_BASE_URL/$PKG.tgz" "$SHA" > "$OUT_DIR/channels.json"
import json, os, sys
path, channel, version, url, sha = sys.argv[1:6]
feed = {}
if os.path.exists(path):
    with open(path) as f:
        feed = json.load(f)
feed[channel] = {"version": version, "url": url, "sha256": sha, "minUpgradeFrom": "0.0.0"}
with open(path, "w") as f:
    json.dump(feed, f, indent=2)
    f.write("\n")
PY

say "Done"
ls -lh "$OUT_DIR/$PKG.tgz" "$OUT_DIR/$PKG.tgz.sha256" "$OUT_DIR/channels.json"
