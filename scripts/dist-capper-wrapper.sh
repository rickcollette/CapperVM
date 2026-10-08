#!/bin/sh
set -eu

dir="$(CDPATH="" cd -- "$(dirname -- "$0")" && pwd)"
has_store=0

for arg in "$@"; do
  case "$arg" in
    --store|--store=*)
      has_store=1
      break
      ;;
  esac
done

if [ "$has_store" -eq 1 ]; then
  exec "$dir/lib/capper-bin" "$@"
fi

exec "$dir/lib/capper-bin" --store "$dir/store" "$@"
