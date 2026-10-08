#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"

target="${1:?usage: scripts/build-aio-platform.sh TARGET VERSION BASE_IMAGE PLATFORM_SUFFIX}"
version="${2:?usage: scripts/build-aio-platform.sh TARGET VERSION BASE_IMAGE PLATFORM_SUFFIX}"
base_image="${3:?usage: scripts/build-aio-platform.sh TARGET VERSION BASE_IMAGE PLATFORM_SUFFIX}"
platform_suffix="${4:?usage: scripts/build-aio-platform.sh TARGET VERSION BASE_IMAGE PLATFORM_SUFFIX}"

builder="capper-release-${target}"

DOCKER_BUILDKIT="${DOCKER_BUILDKIT:-1}" docker build \
  --build-arg "BASE_IMAGE=${base_image}" \
  -f packaging/Dockerfile.release \
  -t "$builder" .

CAPPERWEB_HOST="${CAPPERWEB_HOST:-$ROOT/../CapperWeb}"
if [ ! -d "$CAPPERWEB_HOST" ]; then
  echo "error: CapperWeb not found at $CAPPERWEB_HOST (set CAPPERWEB_HOST)" >&2
  exit 1
fi

docker run --rm \
  --privileged \
  --cgroupns=host \
  -e "VERSION=${version}" \
  -e "PLATFORM_SUFFIX=${platform_suffix}" \
  -e "CAPPERWEB_SRC_DIR=/src/.release/CapperWeb-src" \
  -e "CAPPERWEB_DIR=/tmp/capper-build/CapperWeb" \
  -e "SKIP_WEB=${SKIP_WEB:-0}" \
  -e "SKIP_TESTS=${SKIP_TESTS:-0}" \
  -e "SKIP_IMAGE=${SKIP_IMAGE:-0}" \
  -v "$ROOT:/src" \
  -v "$CAPPERWEB_HOST:/src/.release/CapperWeb-src:ro" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$builder"

if [ -d "$ROOT/DIST/AIO" ]; then
  docker run --rm \
    --entrypoint /bin/sh \
    -v "$ROOT/DIST/AIO:/out" \
    "$builder" \
    -c "chown -R $(id -u):$(id -g) /out"
fi
