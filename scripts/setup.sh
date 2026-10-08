#!/usr/bin/env bash
# setup.sh — First-time and rebuild setup for Capper
#
# Usage:
#   ./scripts/setup.sh              full setup: deps → build → caps → start
#   ./scripts/setup.sh --caps-only  re-apply setcap after a manual rebuild
#   ./scripts/setup.sh --no-start   build and configure but don't start the service
#   ./scripts/setup.sh --check      verify prerequisites only
#
# The only operation that requires sudo is the single setcap call that grants
# CAP_NET_ADMIN to the binary.  Everything else runs as the current user.

set -euo pipefail

# ── Colours ──────────────────────────────────────────────────────────────────

if [ -t 1 ]; then
  RED='\033[0;31m'; YELLOW='\033[0;33m'; GREEN='\033[0;32m'
  CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
else
  RED=''; YELLOW=''; GREEN=''; CYAN=''; BOLD=''; RESET=''
fi

info()  { printf "${CYAN}  →${RESET} %s\n" "$*"; }
ok()    { printf "${GREEN}  ✓${RESET} %s\n" "$*"; }
warn()  { printf "${YELLOW}  !${RESET} %s\n" "$*"; }
die()   { printf "${RED}${BOLD}  ✗ %s${RESET}\n" "$*" >&2; exit 1; }
header(){ printf "\n${BOLD}%s${RESET}\n" "$*"; }

# ── Paths ─────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
CAPPERWEB_DIR="${CAPPERWEB_DIR:-$REPO_ROOT/../CapperWeb}"
DIST_BIN="$REPO_ROOT/DIST/lib/capper-bin"
RUN_DIR="${RUN_DIR:-$REPO_ROOT/capper-run}"

cd "$REPO_ROOT"

# ── Flags ─────────────────────────────────────────────────────────────────────

MODE="full"
for arg in "$@"; do
  case "$arg" in
    --caps-only) MODE="caps" ;;
    --no-start)  MODE="no-start" ;;
    --check)     MODE="check" ;;
    --help|-h)
      sed -n '3,10p' "$0"
      exit 0 ;;
    *) die "Unknown argument: $arg" ;;
  esac
done

# ── Prerequisite checks ───────────────────────────────────────────────────────

MIN_GO_MAJOR=1; MIN_GO_MINOR=22
MIN_NODE_MAJOR=18

check_go() {
  header "Go toolchain"
  if ! command -v go &>/dev/null; then
    die "Go not found. Install from https://go.dev/dl/ (need ≥ ${MIN_GO_MAJOR}.${MIN_GO_MINOR})"
  fi
  read -r go_maj go_min < <(go version | grep -oP 'go\K[0-9]+\.[0-9]+' | awk -F. '{print $1, $2}')
  if [[ "$go_maj" -lt "$MIN_GO_MAJOR" ]] || \
     [[ "$go_maj" -eq "$MIN_GO_MAJOR" && "$go_min" -lt "$MIN_GO_MINOR" ]]; then
    die "Go ${go_maj}.${go_min} is too old; need ≥ ${MIN_GO_MAJOR}.${MIN_GO_MINOR}"
  fi
  ok "Go $(go version | grep -oP 'go\K[0-9]+\.[0-9.]+')"
}

check_node() {
  header "Node / npm"
  # nvm installs node outside the default PATH — source nvm if available
  if ! command -v node &>/dev/null; then
    for nvm_script in "$HOME/.nvm/nvm.sh" "/usr/local/share/nvm/nvm.sh"; do
      [ -s "$nvm_script" ] && source "$nvm_script" --no-use && break
    done
  fi
  if ! command -v node &>/dev/null; then
    die "Node not found. Install via nvm (https://github.com/nvm-sh/nvm) or apt (node ≥ ${MIN_NODE_MAJOR})"
  fi
  node_maj=$(node --version | grep -oP 'v\K[0-9]+')
  if [[ "$node_maj" -lt "$MIN_NODE_MAJOR" ]]; then
    die "Node v${node_maj} is too old; need ≥ v${MIN_NODE_MAJOR}"
  fi
  ok "Node $(node --version)  npm $(npm --version)"
}

check_system_deps() {
  header "System packages"

  local missing=()
  declare -A bins=(
    [bwrap]="bubblewrap"
    [ip]="iproute2"
    [iptables]="iptables"
    [setcap]="libcap2-bin"
    [getcap]="libcap2-bin"
  )

  for bin in "${!bins[@]}"; do
    if ! command -v "$bin" &>/dev/null && ! command -v "/usr/sbin/$bin" &>/dev/null; then
      missing+=("${bins[$bin]}")
      warn "$bin not found (package: ${bins[$bin]})"
    else
      ok "$bin"
    fi
  done

  if [[ ${#missing[@]} -gt 0 ]]; then
    local unique_pkgs
    unique_pkgs=$(printf '%s\n' "${missing[@]}" | sort -u | tr '\n' ' ')
    if command -v apt-get &>/dev/null; then
      info "Installing: $unique_pkgs"
      sudo apt-get install -y $unique_pkgs
      ok "Packages installed"
    else
      die "Missing packages: $unique_pkgs — install them and re-run setup."
    fi
  fi
}

check_web_dir() {
  header "CapperWeb"
  if [[ ! -d "$CAPPERWEB_DIR" ]]; then
    die "CapperWeb directory not found at $CAPPERWEB_DIR — set CAPPERWEB_DIR=<path> to override"
  fi
  if [[ ! -f "$CAPPERWEB_DIR/package.json" ]]; then
    die "$CAPPERWEB_DIR/package.json missing — is this the right CapperWeb directory?"
  fi
  ok "$CAPPERWEB_DIR"
}

# ── Build steps ───────────────────────────────────────────────────────────────

build_backend() {
  header "Backend (Go)"
  info "Compiling…"
  go build -o bin/capper ./cmd/capper
  ok "bin/capper"
}

bootstrap_alpine() {
  header "Alpine rootfs"
  if [[ -f examples/alpine/rootfs/bin/sh ]]; then
    ok "Rootfs already present — skipping download"
    return
  fi
  info "Downloading Alpine rootfs…"
  sh examples/alpine/bootstrap.sh
  ok "examples/alpine/rootfs"
}

build_dist() {
  header "Distribution bundle"
  info "Assembling DIST/…"

  rm -rf DIST
  mkdir -p DIST/store DIST/lib

  cp bin/capper DIST/lib/capper-bin
  cp scripts/dist-capper-wrapper.sh DIST/capper
  chmod +x DIST/capper

  for f in README.md go.mod go.sum; do
    [[ -f "$f" ]] && cp "$f" DIST/
  done
  for d in schemas docs examples; do
    [[ -d "$d" ]] && cp -a "$d" DIST/
  done

  info "Seeding default Alpine image…"
  cd DIST && ./capper --store ./store create alpine.cap examples/alpine/capper.json
  cp store/images/alpine.cap alpine.cap
  cd ..
  ok "DIST/lib/capper-bin"
}

apply_setcap() {
  header "Binary capabilities"

  if [[ ! -f "$DIST_BIN" ]]; then
    die "$DIST_BIN not found — run setup without --caps-only first"
  fi

  local current
  current=$(getcap "$DIST_BIN" 2>/dev/null || true)
  local want="cap_net_admin,cap_net_raw,cap_sys_admin,cap_net_bind_service+eip"

  if echo "$current" | grep -q "cap_net_bind_service"; then
    ok "capabilities already set ($current)"
    return
  fi

  info "Granting $want to $(basename "$DIST_BIN")…"
  info "  (this is the only sudo operation in setup)"
  sudo setcap "$want" "$DIST_BIN"

  local result
  result=$(getcap "$DIST_BIN" 2>/dev/null || true)
  ok "Capabilities: $result"
}

configure_setcap_sudoers() {
  header "Sudoers — passwordless setcap + runtime dir"

  local setcap_wrapper="$REPO_ROOT/scripts/capper-setcap.sh"
  local mkrundir_wrapper="$REPO_ROOT/scripts/capper-mkrundir.sh"
  local sudoers_setcap="/etc/sudoers.d/capper-setcap"
  local sudoers_mkrundir="/etc/sudoers.d/capper-mkrundir"

  local rule_setcap rule_mkrundir
  rule_setcap="$(id -un) ALL=(root) NOPASSWD: $setcap_wrapper"
  rule_mkrundir="$(id -un) ALL=(root) NOPASSWD: $mkrundir_wrapper"

  if [ -f "$sudoers_setcap" ] && grep -qF "$rule_setcap" "$sudoers_setcap" 2>/dev/null; then
    ok "Sudoers setcap entry already present ($sudoers_setcap)"
  else
    info "Writing $sudoers_setcap…"
    printf '%s\n' "$rule_setcap" | sudo tee "$sudoers_setcap" > /dev/null
    sudo chmod 0440 "$sudoers_setcap"
    if ! sudo visudo -cf "$sudoers_setcap" > /dev/null 2>&1; then
      sudo rm -f "$sudoers_setcap"
      die "Sudoers file failed validation — check that $setcap_wrapper is an absolute path."
    fi
    ok "Sudoers entry written: $sudoers_setcap"
  fi

  if [ -f "$sudoers_mkrundir" ] && grep -qF "$rule_mkrundir" "$sudoers_mkrundir" 2>/dev/null; then
    ok "Sudoers mkrundir entry already present ($sudoers_mkrundir)"
  else
    info "Writing $sudoers_mkrundir…"
    printf '%s\n' "$rule_mkrundir" | sudo tee "$sudoers_mkrundir" > /dev/null
    sudo chmod 0440 "$sudoers_mkrundir"
    if ! sudo visudo -cf "$sudoers_mkrundir" > /dev/null 2>&1; then
      sudo rm -f "$sudoers_mkrundir"
      die "Sudoers file failed validation — check that $mkrundir_wrapper is an absolute path."
    fi
    ok "Sudoers entry written: $sudoers_mkrundir"
  fi
}

configure_sysctl() {
  header "Kernel — IP forwarding"

  local conf="/etc/sysctl.d/80-capper.conf"
  if [[ -f "$conf" ]] && grep -q "net.ipv4.ip_forward" "$conf"; then
    ok "IP forwarding already persisted ($conf)"
  else
    info "Writing $conf (persists ip_forward across reboots)…"
    printf '# Capper: required for NAT network mode\nnet.ipv4.ip_forward = 1\n' \
      | sudo tee "$conf" > /dev/null
    sudo sysctl -q -p "$conf"
    ok "IP forwarding enabled and persisted"
  fi
}

build_web() {
  header "Web console (Node)"
  check_node
  info "npm install + audit fix + build…"
  cd "$CAPPERWEB_DIR"
  scripts/build.sh
  cd "$REPO_ROOT"
  ok "CapperWeb built"
}

stage_web() {
  header "Staging web assets"
  mkdir -p DIST/console
  cp -a "$CAPPERWEB_DIR/dist/." DIST/console/
  ok "DIST/console/"
}

start_service() {
  header "Starting Capper"
  RUN_DIR="$RUN_DIR" scripts/capper-run.sh start
}

# ── Main ──────────────────────────────────────────────────────────────────────

printf "${BOLD}Capper Setup${RESET}  (mode: %s)\n" "$MODE"
printf "  repo:    %s\n" "$REPO_ROOT"
printf "  web:     %s\n" "$CAPPERWEB_DIR"

case "$MODE" in

  check)
    check_go
    check_node
    check_system_deps
    check_web_dir
    printf "\n${GREEN}${BOLD}All prerequisites satisfied.${RESET}\n"
    ;;

  caps)
    printf "\n${YELLOW}Note:${RESET} Capabilities are now applied automatically by capper-run.sh on each start.\n"
    printf "Just run: make capper-run\n"
    ;;

  no-start|full)
    check_go
    check_system_deps
    check_web_dir
    build_backend
    bootstrap_alpine
    build_dist
    configure_setcap_sudoers
    configure_sysctl
    build_web
    stage_web

    if [[ "$MODE" == "full" ]]; then
      start_service
      printf "\n${GREEN}${BOLD}Setup complete.${RESET}\n"
      printf "  API:     http://0.0.0.0:8687\n"
      printf "  Logs:    %s/logs/api.log\n" "$RUN_DIR"
      printf "  Stop:    make capper-run-stop\n"
      printf "  Rebuild: make capper-run  (then re-run: scripts/setup.sh --caps-only)\n\n"
    else
      printf "\n${GREEN}${BOLD}Build complete.${RESET}  Run 'make capper-run' to start.\n\n"
    fi
    ;;

esac
