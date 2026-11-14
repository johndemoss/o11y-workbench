#!/usr/bin/env bash
# scripts/serve.sh — Reliable o11y-workbench-web server launcher.
#
# Local process state (built binary, PID record, locks) lives in a private
# per-checkout directory so a shared, world-writable temporary path cannot be
# used to swap the binary or point the launcher at another process.
set -euo pipefail

ADDR="127.0.0.1:18080"
ALLOWED_HOSTS=""
SKIP_BUILD=false
FOREGROUND=false
STOP_ONLY=false
WAIT_SECONDS=30
ADDR_SET=false
WAIT_SET=false
ALLOWED_SET=false

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SERVER_NAME="o11y-workbench-web"
# shellcheck source=scripts/runtime.sh
. "$SCRIPT_DIR/runtime.sh"

usage() {
  printf '%s\n' \
    'Usage: ./scripts/serve.sh [--addr HOST:PORT] [--allowed-hosts HOSTS] [--wait SECONDS] [--foreground] [--skip-build]' \
    '       ./scripts/serve.sh --stop' \
    'Default: rebuild both UIs, Go binary and OTTL workers; start loopback daemon on 127.0.0.1:18080.' \
    '--addr: listen address passed to the server (default 127.0.0.1:18080).' \
    '--allowed-hosts: comma-separated extra hostnames/IPs the server accepts in the Host header.' \
    '--foreground: exec the server; caller owns readiness/shutdown. Does not touch the daemon PID record.' \
    '--skip-build: explicitly trust the existing runtime-directory binary and worker binaries.' \
    '--stop: safely stop the verified daemon without building; succeeds when none is running.' \
    '--wait: background-daemon readiness timeout (default 30 seconds).' \
    'Runtime state lives in ./.run (override with O11Y_WORKBENCH_RUNTIME_DIR).'
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --addr) ADDR="$2"; ADDR_SET=true; shift 2 ;;
    --allowed-hosts) ALLOWED_HOSTS="$2"; ALLOWED_SET=true; shift 2 ;;
    --skip-build) SKIP_BUILD=true; shift ;;
    --foreground) FOREGROUND=true; shift ;;
    --stop) STOP_ONLY=true; shift ;;
    --wait) WAIT_SECONDS="$2"; WAIT_SET=true; shift 2 ;;
    --help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown flag $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if $STOP_ONLY; then
  if $FOREGROUND || $SKIP_BUILD || $ADDR_SET || $WAIT_SET || $ALLOWED_SET; then
    echo "ERROR: --stop cannot be combined with start options." >&2
    exit 1
  fi
fi

runtime_prepare "$PROJECT_DIR"
PIDFILE="$RUN_DIR/$SERVER_NAME.pid"
BINARY="$RUN_DIR/$SERVER_NAME"
LOCK_DIR="$RUN_DIR/$SERVER_NAME.lock"
BUILD_LOCK_DIR="$RUN_DIR/build.lock"
BUILD_LOCK_HELD=false
SERVICE_LOCK_HELD=false

release_locks() {
  if $BUILD_LOCK_HELD; then
    runtime_release_lock "$BUILD_LOCK_DIR"
    BUILD_LOCK_HELD=false
  fi
  if $SERVICE_LOCK_HELD; then
    runtime_release_lock "$LOCK_DIR"
    SERVICE_LOCK_HELD=false
  fi
}

on_exit() {
  local rc=$?
  release_locks
  return "$rc"
}

trap on_exit EXIT

SERVER_ARGS=(--addr "$ADDR" --pipeline-prototyper)
if [ -n "$ALLOWED_HOSTS" ]; then
  SERVER_ARGS+=(--allowed-hosts "$ALLOWED_HOSTS")
fi

if $STOP_ONLY; then
  runtime_acquire_lock "$LOCK_DIR" server
  SERVICE_LOCK_HELD=true
  runtime_stop "$PIDFILE" "$BINARY"
  echo "==> Local daemon stopped (or was not running)"
  exit 0
fi

if $FOREGROUND; then
  # Playwright and other callers own this process group and drive shutdown
  # themselves. Never read, replace or remove the operator's daemon PID record.
  if ! $SKIP_BUILD; then
    runtime_acquire_lock "$BUILD_LOCK_DIR" build
    BUILD_LOCK_HELD=true
    "$PROJECT_DIR/scripts/build-ui.sh"
    tmp_bin=$(mktemp "$RUN_DIR/$SERVER_NAME.foreground.XXXXXX")
    if ! go build -o "$tmp_bin" "$PROJECT_DIR/cmd/o11y-workbench-web/"; then
      rm -f "$tmp_bin"
      echo "ERROR: Go build failed" >&2
      exit 1
    fi
    runtime_install_binary "$tmp_bin" "$BINARY"
    echo "==> Go binary rebuilt with embedded frontend assets"
    (cd "$PROJECT_DIR" && make ottl-workers)
    release_locks
  else
    runtime_verify_executable "$BINARY" || {
      echo "ERROR: --skip-build requires an existing executable at $BINARY" >&2
      exit 1
    }
    echo "WARNING: --skip-build reuses $BINARY without checking source/asset freshness" >&2
  fi
  echo "==> Serving foreground on $ADDR (caller owns shutdown)..."
  exec "$BINARY" "${SERVER_ARGS[@]}"
fi

runtime_acquire_lock "$LOCK_DIR" server
SERVICE_LOCK_HELD=true

if ! $SKIP_BUILD; then
  # Generated pipeline assets and worker binaries live in shared checkout paths,
  # so serialize the whole build phase across launchers.
  runtime_acquire_lock "$BUILD_LOCK_DIR" build
  BUILD_LOCK_HELD=true
  "$PROJECT_DIR/scripts/build-ui.sh"
  echo "==> Building o11y-workbench-web (Go server with embedded assets)..."
  tmp_bin=$(mktemp "$RUN_DIR/$SERVER_NAME.build.XXXXXX")
  if ! go build -o "$tmp_bin" "$PROJECT_DIR/cmd/o11y-workbench-web/"; then
    rm -f "$tmp_bin"
    echo "ERROR: Go build failed" >&2
    exit 1
  fi
  runtime_install_binary "$tmp_bin" "$BINARY"
  echo "==> Go binary rebuilt with embedded frontend assets"
  echo "==> Building OTTL worker binaries..."
  (cd "$PROJECT_DIR" && make ottl-workers)
  release_locks
else
  runtime_verify_executable "$BINARY" || {
    echo "ERROR: --skip-build requires an existing executable at $BINARY" >&2
    exit 1
  }
  echo "WARNING: --skip-build reuses $BINARY without checking source/asset freshness" >&2
fi

# The in-process OTTL default is pinned in the Go binary; extra supported
# collector trains run in workers listed in versions/otel.env.
if [ -f "$PROJECT_DIR/versions/otel.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$PROJECT_DIR/versions/otel.env"
  set +a
fi
if [ -z "${O11Y_OTTL_WORKERS:-}" ]; then
  bindings=()
  IFS=',' read -r -a worker_versions <<< "${OTEL_OTTL_WORKER_VERSIONS:-}"
  for ver in "${worker_versions[@]}"; do
    ver="${ver//[[:space:]]/}"
    [ -z "$ver" ] && continue
    bindings+=("$ver=$PROJECT_DIR/bin/ottl-worker-$ver")
  done
  if [ "${#bindings[@]}" -gt 0 ]; then
    old_ifs=$IFS
    IFS=,
    export O11Y_OTTL_WORKERS="${bindings[*]}"
    IFS=$old_ifs
  fi
fi
if [ -n "${O11Y_OTTL_WORKERS:-}" ]; then
  echo "==> OTTL workers: $O11Y_OTTL_WORKERS"
fi

if [ -e "$PIDFILE" ]; then
  echo "==> Stopping existing server..."
  runtime_stop "$PIDFILE" "$BINARY"
fi

echo "==> Starting o11y-workbench-web on $ADDR..."
nohup "$BINARY" "${SERVER_ARGS[@]}" </dev/null >/dev/null 2>&1 &
SERVER_PID=$!
disown "$SERVER_PID" 2>/dev/null || true

if ! runtime_record_pid "$PIDFILE" "$SERVER_PID" "$BINARY" "${SERVER_ARGS[@]}"; then
  # This is our own just-spawned child, so signalling it directly is safe here.
  kill "$SERVER_PID" 2>/dev/null || true
  exit 1
fi

HEALTH_ADDR="$ADDR"
case "$ADDR" in
  :*) HEALTH_ADDR="127.0.0.1$ADDR" ;;
  0.0.0.0:*) HEALTH_ADDR="127.0.0.1:${ADDR##*:}" ;;
  \[::\]:*) HEALTH_ADDR="[::1]:${ADDR##*:}" ;;
esac
HEALTH_URL="http://${HEALTH_ADDR}/api/health"
echo "==> Waiting for server (PID $SERVER_PID) to accept connections (up to ${WAIT_SECONDS}s)..."
START_TIME=$SECONDS
while true; do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "ERROR: server process died unexpectedly" >&2
    runtime_stop "$PIDFILE" "$BINARY" || true
    exit 1
  fi
  if curl -sf "$HEALTH_URL" >/dev/null 2>&1; then
    echo "==> Server ready on $ADDR (PID $SERVER_PID)"
    exit 0
  fi
  ELAPSED=$((SECONDS - START_TIME))
  if [ "$ELAPSED" -ge "$WAIT_SECONDS" ]; then
    echo "ERROR: server did not become ready within ${WAIT_SECONDS}s" >&2
    runtime_stop "$PIDFILE" "$BINARY" || kill "$SERVER_PID" 2>/dev/null || true
    exit 1
  fi
  sleep 0.5
done
