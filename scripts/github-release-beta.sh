#!/usr/bin/env bash
# Publish ignored DIST/AIO artifacts to a GitHub prerelease.
set -euo pipefail

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"

version="${1:-$(tr -d ' \n\r' < VERSION 2>/dev/null || true)}"
beta="${2:-1}"

if [ -z "$version" ]; then
  echo "usage: scripts/github-release-beta.sh VERSION [BETA_NUMBER]" >&2
  exit 2
fi

tag="v${version}-beta.${beta}"
title="CapperVM ${version} beta ${beta}"

command -v gh >/dev/null 2>&1 || {
  echo "error: GitHub CLI 'gh' is required" >&2
  exit 1
}

artifacts=()
while IFS= read -r -d '' file; do
  artifacts+=("$file")
done < <(find DIST/AIO -maxdepth 1 -type f \( -name 'capper-aio-*.tgz' -o -name 'capper-aio-*.tgz.sha256' -o -name 'channels.json' \) -print0 | sort -z)

if [ "${#artifacts[@]}" -eq 0 ]; then
  echo "error: no DIST/AIO release artifacts found" >&2
  exit 1
fi

notes="$(mktemp)"
trap 'rm -f "$notes"' EXIT
{
  echo "Beta release artifacts for CapperVM ${version}."
  echo
  echo "Artifacts:"
  for file in "${artifacts[@]}"; do
    echo "- $(basename "$file")"
  done
  echo
  echo "Install:"
  echo '```bash'
  echo 'sha256sum -c capper-aio-<version>-<platform>.tgz.sha256'
  echo 'tar xzf capper-aio-<version>-<platform>.tgz'
  echo 'cd capper-aio-<version>-<platform>'
  echo 'sudo ./install.sh --check-only'
  echo 'sudo ./install.sh --yes'
  echo 'sudo capper aio doctor'
  echo 'sudo capper aio init --backend capdb'
  echo 'sudo capper aio up'
  echo '```'
} > "$notes"

if gh release view "$tag" >/dev/null 2>&1; then
  gh release upload "$tag" "${artifacts[@]}" --clobber
else
  gh release create "$tag" "${artifacts[@]}" \
    --target "$(git rev-parse --abbrev-ref HEAD)" \
    --title "$title" \
    --notes-file "$notes" \
    --prerelease
fi

echo "Published $tag with ${#artifacts[@]} artifacts"
