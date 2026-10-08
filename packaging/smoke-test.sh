#!/usr/bin/env bash
set -euo pipefail

bundle="${1:?usage: packaging/smoke-test.sh DIST/AIO/capper-aio-*.tgz}"
[ -f "$bundle" ] || { echo "bundle missing: $bundle" >&2; exit 1; }
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

tar xzf "$bundle" -C "$tmp"
root="$(find "$tmp" -mindepth 1 -maxdepth 1 -type d | head -1)"
[ -n "$root" ] || { echo "bundle did not extract to a directory" >&2; exit 1; }

"$root/bin/capper" version
"$root/bin/capper-agent" --help >/dev/null
"$root/bin/capinit" --help >/dev/null || true
test -f "$root/manifest.json" || { echo "missing manifest.json in $root" >&2; ls -la "$root" >&2; exit 1; }
test -x "$root/install.sh"
bash -n "$root/install.sh"

echo "bundle smoke passed: $bundle"
