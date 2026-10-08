#!/usr/bin/env bash
set -euo pipefail

cd /src

: "${VERSION:?VERSION is required}"
: "${PLATFORM_SUFFIX:?PLATFORM_SUFFIX is required}"

echo "Builder OS:"
cat /etc/os-release || true
echo "glibc: $(getconf GNU_LIBC_VERSION 2>/dev/null || echo unknown)"

if command -v git >/dev/null 2>&1; then
  git config --global --add safe.directory /src || true
  git config --global --add safe.directory /src/CapDB || true
  git config --global --add safe.directory /src/.release/CapperWeb-src || true
  git config --global --add safe.directory /tmp/capper-build/CapperWeb || true
fi

if [ "${SKIP_WEB:-0}" != "1" ] && [ -n "${CAPPERWEB_SRC_DIR:-}" ] && [ -d "$CAPPERWEB_SRC_DIR" ]; then
  rm -rf /tmp/capper-build/CapperWeb
  mkdir -p /tmp/capper-build
  cp -a "$CAPPERWEB_SRC_DIR" /tmp/capper-build/CapperWeb
fi

CAPDB_DIR="${CAPDB_DIR:-CapDB}" \
CAPDB_BUILD="${CAPDB_BUILD:-/tmp/capper-build/capdb}" \
CAPDB_JOBS="${CAPDB_JOBS:-1}" \
GO_PACKAGE_PARALLELISM="${GO_PACKAGE_PARALLELISM:-1}" \
PLATFORM_SUFFIX="$PLATFORM_SUFFIX" \
BUILD_IMAGE_DIGEST="${BUILD_IMAGE_DIGEST:-}" \
scripts/build-aio.sh "$VERSION"
