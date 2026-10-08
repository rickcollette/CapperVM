#!/usr/bin/env bash
set -euo pipefail

bundle="${1:?usage: packaging/smoke-test.sh DIST/AIO/capper-aio-*.tgz}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

tar xzf "$bundle" -C "$tmp"
root="$(find "$tmp" -mindepth 1 -maxdepth 1 -type d | head -1)"
[ -n "$root" ] || { echo "bundle did not extract to a directory" >&2; exit 1; }

"$root/bin/capper" version
"$root/bin/capper-agent" --help >/dev/null
"$root/bin/capinit" --help >/dev/null || true
test -f "$root/manifest.json"
test -x "$root/install.sh"
bash -n "$root/install.sh"

echo "bundle smoke passed: $bundle"
