#!/bin/sh
set -eu

cmd="${1:-start}"

script_dir="$(CDPATH="" cd -- "$(dirname -- "$0")" && pwd)"
repo_root="$(CDPATH="" cd -- "$script_dir/.." && pwd)"
run_dir="${RUN_DIR:-capper-run}"
case "$run_dir" in
  /*) run_path="$run_dir" ;;
  *) run_path="$repo_root/$run_dir" ;;
esac

api_addr="${CAPPER_RUN_API_ADDR:-0.0.0.0:8687}"
console_dir="${CAPPER_RUN_CONSOLE:-$run_path/console}"
log_dir="$run_path/logs"
pid_dir="$run_path/run"
pid_file="$pid_dir/api.pid"
log_file="$log_dir/api.log"

host="${api_addr%:*}"
port="${api_addr##*:}"
health_url="http://127.0.0.1:$port/api/v1/health"

is_running() {
  [ -f "$pid_file" ] || return 1
  pid="$(cat "$pid_file" 2>/dev/null || true)"
  [ -n "$pid" ] || return 1
  kill -0 "$pid" 2>/dev/null || return 1
  return 0
}

stop_service() {
  if is_running; then
    pid="$(cat "$pid_file")"
    printf 'Stopping existing capper-run service (pid %s)\n' "$pid"
    kill "$pid" 2>/dev/null || true
    i=0
    while kill -0 "$pid" 2>/dev/null; do
      i=$((i + 1))
      if [ "$i" -ge 50 ]; then
        kill -9 "$pid" 2>/dev/null || true
        break
      fi
      sleep 0.1
    done
  fi
  rm -f "$pid_file"
}

status_service() {
  if is_running; then
    pid="$(cat "$pid_file")"
    printf 'capper-run is running\n'
    printf '  pid: %s\n' "$pid"
    printf '  url: http://%s\n' "$api_addr"
    printf '  folder: %s\n' "$run_path"
    return 0
  fi
  printf 'capper-run is not running\n'
  return 1
}

port_busy_by_other() {
  command -v ss >/dev/null 2>&1 || return 1
  ss -ltnp 2>/dev/null | grep -q "$host:$port" || return 1
  if is_running; then
    return 1
  fi
  return 0
}

wait_health() {
  i=0
  while [ "$i" -lt 50 ]; do
    if command -v curl >/dev/null 2>&1 && curl -fsS "$health_url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
    i=$((i + 1))
  done
  return 1
}

start_service() {
  stop_service

  if port_busy_by_other; then
    printf 'ERROR: %s is already in use by another process.\n' "$api_addr" >&2
    printf 'Set CAPPER_RUN_API_ADDR=127.0.0.1:PORT or stop the other process.\n' >&2
    return 1
  fi

  rm -rf "$run_path"
  mkdir -p "$run_path" "$log_dir" "$pid_dir"
  cp -a "$repo_root/DIST/." "$run_path/"
  mkdir -p "$log_dir" "$pid_dir"

  # cp strips file capabilities; re-apply them to the deployed binary.
  # Uses the wrapper so the sudoers entry is a clean path with no arguments.
  sudo "$script_dir/capper-setcap.sh" "$run_path/lib/capper-bin"

  # Create /run/capper/netns (tmpfs; lost on reboot) owned by this user so
  # capper-bin can bind-mount named network namespaces without root.
  sudo "$script_dir/capper-mkrundir.sh"

  console_args=""
  if [ -d "$console_dir" ]; then
    console_args="--console $console_dir"
  fi

  : > "$log_file"
  # One API process with --with-daemon avoids two independent processes fighting
  # over the same SQLite store.
  setsid -f "$run_path/capper" api start --listen "$api_addr" --with-daemon $console_args > "$log_file" 2>&1

  i=0
  pid=""
  while [ "$i" -lt 30 ]; do
    pid="$(pgrep -f "^$run_path/lib/capper-bin --store $run_path/store api start --listen $api_addr" | head -n 1 || true)"
    [ -n "$pid" ] && break
    sleep 0.1
    i=$((i + 1))
  done
  if [ -z "$pid" ]; then
    printf 'ERROR: capper-run failed to start. Log follows:\n' >&2
    sed -n '1,120p' "$log_file" >&2 || true
    return 1
  fi
  printf '%s\n' "$pid" > "$pid_file"

  if ! wait_health; then
    printf 'ERROR: capper-run started but health check failed. Log follows:\n' >&2
    sed -n '1,160p' "$log_file" >&2 || true
    stop_service
    return 1
  fi

  printf 'capper-run is ready\n'
  printf '  folder: %s\n' "$run_path"
  printf '  url: http://%s\n' "$api_addr"
  printf '  pid: %s\n' "$pid"
  printf '  log: %s\n' "$log_file"
}

case "$cmd" in
  start) start_service ;;
  stop) stop_service ;;
  status) status_service ;;
  *) printf 'usage: %s {start|stop|status}\n' "$0" >&2; exit 2 ;;
esac
