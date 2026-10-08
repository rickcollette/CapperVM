#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"

version="${1:?usage: scripts/release-matrix.sh VERSION [target...]}"
shift || true

targets=("$@")
if [ "${#targets[@]}" -eq 0 ]; then
  targets=(ubuntu24.04 debian12 rhel9 rocky-current ubuntu18.04)
fi

target_image() {
  case "$1" in
    ubuntu18.04) echo "ubuntu:18.04" ;;
    debian12) echo "debian:12" ;;
    ubuntu24.04) echo "ubuntu:24.04" ;;
    rocky-current) echo "rockylinux/rockylinux:10" ;;
    rhel9) echo "registry.access.redhat.com/ubi9/ubi" ;;
    *) echo "unknown target: $1" >&2; return 1 ;;
  esac
}

target_suffix() {
  case "$1" in
    ubuntu18.04) echo "ubuntu18.04-glibc2.27-x86_64" ;;
    debian12) echo "debian12-glibc2.36-x86_64" ;;
    ubuntu24.04) echo "ubuntu24.04-glibc2.39-x86_64" ;;
    rocky-current) echo "rocky10-glibc-detect-x86_64" ;;
    rhel9) echo "rhel9-glibc2.34-x86_64" ;;
    *) echo "unknown target: $1" >&2; return 1 ;;
  esac
}

for target in "${targets[@]}"; do
  image="$(target_image "$target")"
  suffix="$(target_suffix "$target")"
  echo "==> building $target ($image -> $suffix)"
  scripts/build-aio-platform.sh "$target" "$version" "$image" "$suffix"
  bundle="DIST/AIO/capper-aio-${version}-${suffix}.tgz"
  if [ -f "$bundle" ]; then
    docker run --rm \
      --entrypoint /bin/bash \
      -v "$ROOT:/src" \
      -w /src \
      "capper-release-${target}" \
      packaging/smoke-test.sh "$bundle"
  fi
done
