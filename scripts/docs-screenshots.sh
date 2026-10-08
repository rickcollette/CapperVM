#!/usr/bin/env bash
# Build CapperWeb, start capper-run with console, seed a local docs user, capture screenshots.
set -euo pipefail

HERE="$(CDPATH="" cd -- "$(dirname -- "$0")" && pwd)"
ROOT="$(CDPATH="" cd -- "$HERE/.." && pwd)"
WEB="$ROOT/../CapperWeb"
OUT="$ROOT/docs/assets/images/screenshots"
LEGACY_OUT="$ROOT/docs/screenshots"
USER="${DOCS_SCREENSHOT_USER:-docs}"
PASS="${DOCS_SCREENSHOT_PASS:-docs-demo}"

cd "$ROOT"
say() { printf '\n==> %s\n' "$*"; }

say "Removing old screenshots"
rm -rf "$OUT" "$LEGACY_OUT"
mkdir -p "$OUT" "$LEGACY_OUT"

say "Building Capper + CapperWeb"
make dist web

say "Seeding screenshot user ($USER) into store"
go run ./tools/screenseed ./DIST/store --user "$USER:$PASS:admin" >/dev/null

say "Bundling console into DIST"
mkdir -p DIST/console
cp -a "$WEB/dist/." DIST/console/

say "Starting capper-run with console"
make capper-run-stop >/dev/null 2>&1 || true
# Kill orphaned API processes still bound to the default port.
pkill -9 -f 'capper-bin --store .*/capper-run/store api start' 2>/dev/null || true
sleep 0.5
RUN_DIR="capper-run" scripts/capper-run.sh start

say "Waiting for API"
for i in $(seq 1 30); do
  curl -fsS http://127.0.0.1:8687/api/v1/health >/dev/null 2>&1 && break
  sleep 0.5
done
curl -fsS http://127.0.0.1:8687/api/v1/health >/dev/null

say "Verifying console is served"
code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8687/)"
if [ "$code" != "200" ]; then
  echo "ERROR: console root returned HTTP $code (expected 200 SPA)" >&2
  exit 1
fi

say "Capturing screenshots with Playwright"
cd "$WEB"
DOCS_SCREENSHOT_USER="$USER" DOCS_SCREENSHOT_PASS="$PASS" \
  npx playwright test tests/e2e/screenshots.spec.ts --project=screenshots

say "Mirroring screenshots to docs/screenshots/"
cp -a "$OUT/." "$LEGACY_OUT/"

say "Done — $(ls -1 "$OUT"/*.png | wc -l) images in docs/assets/images/screenshots/ and docs/screenshots/"
