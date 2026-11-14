#!/usr/bin/env bash
# scripts/serve.sh — Reliable o11y-workbench-web server launcher.
set -euo pipefail

ADDR="127.0.0.1:18080"
SKIP_BUILD=false
FOREGROUND=false
WAIT_SECONDS=30
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PIDFILE="/tmp/o11y-workbench-web.pid"
BINARY="/tmp/o11y-workbench-web"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --addr) ADDR="$2"; shift 2 ;;
    --skip-build) SKIP_BUILD=true; shift ;;
    --foreground) FOREGROUND=true; shift ;;
    --wait) WAIT_SECONDS="$2"; shift 2 ;;
    --help)
      printf '%s\n' \
        'Usage: ./scripts/serve.sh [--addr HOST:PORT] [--wait SECONDS] [--foreground] [--skip-build]' \
        'Default: rebuild both UIs, Go binary and OTTL workers; start loopback daemon on 127.0.0.1:18080.' \
        '--foreground: exec the server; caller owns readiness/shutdown. Does not touch the daemon PID file.' \
        '--skip-build: explicitly trust the existing /tmp/o11y-workbench-web binary and worker binaries.' \
        '--wait: background-daemon readiness timeout (default 30 seconds).'
      exit 0
      ;;
    *)
      echo "ERROR: unknown flag $1" >&2
      exit 1
      ;;
  esac
done

# On background-launch failure, wipe the pidfile so callers know the server is dead.
# On success (exit 0) we leave it — read the PID file when stopping the daemon.
cleanup() {
  local rc=$?
  if [ "$rc" -ne 0 ]; then
    rm -f "$PIDFILE"
  fi
}
if ! $FOREGROUND; then
  trap cleanup EXIT
fi

if ! $SKIP_BUILD; then
  "$PROJECT_DIR/scripts/build-ui.sh"

  echo "==> Building o11y-workbench-web (Go server with embedded assets)..."
  go build -o "$BINARY" "$PROJECT_DIR/cmd/o11y-workbench-web/" || {
    echo "ERROR: Go build failed" >&2
    exit 1
  }
  echo "==> Go binary rebuilt with embedded frontend assets"

  echo "==> Building OTTL worker binaries..."
  (cd "$PROJECT_DIR" && make ottl-workers) || {
    echo "ERROR: ottl-workers build failed" >&2
    exit 1
  }
else
  # Explicitly trust an existing binary. Timestamps cannot prove its embedded
  # assets or Go source match this checkout (and stat -c is not portable).
  if [ ! -x "$BINARY" ]; then
    echo "ERROR: --skip-build requires an existing executable at $BINARY" >&2
    exit 1
  fi
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

# Test runners own foreground process groups and SIGTERM/join themselves. Do not
# read, replace or remove the operator's daemon PID file in this mode.
if $FOREGROUND; then
  echo "==> Serving foreground on $ADDR (caller owns shutdown)..."
  exec "$BINARY" --addr "$ADDR" --pipeline-prototyper
fi

if [ -f "$PIDFILE" ]; then
  OLD_PID="$(cat "$PIDFILE")"
  if kill -0 "$OLD_PID" 2>/dev/null; then
    echo "==> Stopping existing server (PID $OLD_PID)..."
    kill "$OLD_PID" 2>/dev/null || true
    for i in $(seq 1 10); do
      if ! kill -0 "$OLD_PID" 2>/dev/null; then
        break
      fi
      sleep 0.5
    done
  fi
fi

echo "==> Starting o11y-workbench-web on $ADDR..."
nohup "$BINARY" --addr "$ADDR" --pipeline-prototyper </dev/null >/dev/null 2>&1 &
SERVER_PID=$!
echo "$SERVER_PID" > "$PIDFILE"
disown "$SERVER_PID" 2>/dev/null || true

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
    exit 1
  fi
  if curl -sf "$HEALTH_URL" >/dev/null 2>&1; then
    echo "==> Server ready on $ADDR (PID $SERVER_PID)"
    exit 0
  fi
  ELAPSED=$((SECONDS - START_TIME))
  if [ "$ELAPSED" -ge "$WAIT_SECONDS" ]; then
    echo "ERROR: server did not become ready within ${WAIT_SECONDS}s" >&2
    kill "$SERVER_PID" 2>/dev/null || true
    exit 1
  fi
  sleep 0.5
done
