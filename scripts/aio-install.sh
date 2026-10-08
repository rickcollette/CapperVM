#!/usr/bin/env bash
# Installer shipped inside the Capper AIO tarball. Installs into a *versioned*
# layout so `capper aio upgrade` / `--rollback` are atomic symlink flips:
#
#   /usr/local/lib/capper/<version>/{bin,console}   staged release
#   /usr/local/lib/capper/current  -> <version>      active version (symlink)
#   /usr/local/bin/<bin>           -> current/bin/<bin>
#   /opt/capper/console            -> current/console
#
# Re-running on an existing install performs an in-place version add + flip
# (an upgrade) rather than overwriting files. Target: Ubuntu 24.04 (amd64).
set -euo pipefail

PREFIX="${PREFIX:-/usr/local}"
LIB_ROOT="${LIB_ROOT:-$PREFIX/lib/capper}"
CONSOLE_LINK="${CONSOLE_DIR:-/opt/capper/console}"

here="$(CDPATH="" cd -- "$(dirname -- "$0")" && pwd)"

if [ "$(id -u)" -ne 0 ]; then
  echo "error: run as root (sudo ./install.sh)" >&2
  exit 1
fi

VERSION="$(cat "$here/VERSION" 2>/dev/null || echo unknown)"
DEST="$LIB_ROOT/$VERSION"

# Runtime dependency: capper (cgo+capdb) and capdb-server link OpenSSL 3.
if ! ldconfig -p | grep -q 'libssl\.so\.3'; then
  echo "warning: libssl.so.3 not found. Install with: apt-get update && apt-get install -y openssl libssl3" >&2
fi

prev=""
if [ -L "$LIB_ROOT/current" ]; then
  prev="$(basename "$(readlink "$LIB_ROOT/current")")"
  echo "Existing install detected (version $prev) — staging $VERSION as an upgrade."
fi

echo "Staging release $VERSION -> $DEST"
install -d -m 0755 "$LIB_ROOT"
rm -rf "$DEST"
install -d -m 0755 "$DEST/bin"
for b in capper capper-agent capinit capdb-server; do
  [ -f "$here/bin/$b" ] && install -m 0755 "$here/bin/$b" "$DEST/bin/$b"
done
if [ -d "$here/console" ]; then
  install -d -m 0755 "$DEST/console"
  cp -a "$here/console/." "$DEST/console/"
fi

# Flip the active version atomically.
ln -sfn "$DEST" "$LIB_ROOT/current.tmp"
mv -Tf "$LIB_ROOT/current.tmp" "$LIB_ROOT/current"

# Point stable paths through current/.
install -d -m 0755 "$PREFIX/bin"
for b in capper capper-agent capinit capdb-server; do
  [ -f "$LIB_ROOT/current/bin/$b" ] && ln -sfn "$LIB_ROOT/current/bin/$b" "$PREFIX/bin/$b"
done
if [ -d "$LIB_ROOT/current/console" ]; then
  install -d -m 0755 "$(dirname "$CONSOLE_LINK")"
  ln -sfn "$LIB_ROOT/current/console" "$CONSOLE_LINK"
fi

# Drop-in so the control plane serves the console (the base unit has no --console).
if command -v systemctl >/dev/null 2>&1; then
  install -d -m 0755 /etc/systemd/system/capper-control.service.d
  cat > /etc/systemd/system/capper-control.service.d/10-console.conf <<EOF
[Service]
ExecStart=
ExecStart=$PREFIX/bin/capper api start --console $CONSOLE_LINK
EOF
  systemctl daemon-reload || true
fi

if [ -n "$prev" ]; then
  cat <<EOF

Upgraded $prev -> $VERSION. Apply it with:

  sudo capper aio upgrade --bundle <this-bundle>.tgz   # full orchestrated upgrade
  # or, since binaries are already staged, restart services:
  sudo systemctl restart capdb-server capper-control capper-agent

Roll back with: sudo capper aio upgrade --rollback
EOF
else
  cat <<EOF

Capper AIO $VERSION installed. Next steps (CapDB backend, TLS by default):

  sudo capper aio init --backend capdb
  sudo capper aio up
  capper aio status

The console is served by the control plane on http://localhost:8080.
Use 'capper aio doctor' for pre-flight checks and 'capper aio down' to stop.
EOF
fi
