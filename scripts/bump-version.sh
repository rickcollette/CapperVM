#!/usr/bin/env bash
# Bump the patch component in ./VERSION (semver major.minor.patch).
# Usage: scripts/bump-version.sh [patch|minor|major]
# Prints the new version and writes it to VERSION.
set -euo pipefail

ROOT="$(CDPATH="" cd -- "$(dirname -- "$0")/.." && pwd)"
VER_FILE="$ROOT/VERSION"
KIND="${1:-patch}"

current="$(tr -d ' \n\r' < "$VER_FILE" 2>/dev/null || echo "0.1.0")"

if [[ "$current" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)(.*)$ ]]; then
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[3]}"
  suffix="${BASH_REMATCH[4]}"
else
  major=0
  minor=1
  patch=0
  suffix=""
fi

case "$KIND" in
  major) major=$((major + 1)); minor=0; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
  *)
    echo "error: unknown bump kind '$KIND' (use patch, minor, or major)" >&2
    exit 1
    ;;
esac

new="${major}.${minor}.${patch}${suffix}"
printf '%s\n' "$new" > "$VER_FILE"
echo "$new"
