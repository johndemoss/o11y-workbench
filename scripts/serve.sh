#!/usr/bin/env bash
# scripts/serve.sh — Reliable o11y-workbench-web server launcher for agents.
set -euo pipefail

ADDR=":18080"
SKIP_BUILD=false
WAIT_SECONDS=30
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PIDFILE="/tmp/o11y-workbench-web.pid"
BINARY="/tmp/o11y-workbench-web"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --addr) ADDR="$2"; shift 2 ;;
    --skip-build) SKIP_BUILD=true; shift ;;
    --wait) WAIT_SECONDS="$2"; shift 2 ;;
    --help)
      sed -n '3,25p' "$0"
      exit 0
      ;;
    *)
      echo "ERROR: unknown flag $1" >&2
      exit 1
      ;;
  esac
done

# On failure, wipe the pidfile so agents know the server is dead.
# On success (exit 0) we leave it — the agent reads it for cleanup.
cleanup() {
  local rc=$?
  if [ "$rc" -ne 0 ]; then
    rm -f "$PIDFILE"
  fi
}
trap cleanup EXIT

if ! $SKIP_BUILD; then
  if [ -d "$PROJECT_DIR/pipeline-ui" ]; then
    echo "==> Building pipeline-ui (frontend)..."
    (cd "$PROJECT_DIR/pipeline-ui" && npm install --silent && npm run build) || {
      echo "ERROR: pipeline-ui build failed" >&2
      exit 1
    }
    # Always sync dist to server - the Go build will embed this content
    rm -rf "$PROJECT_DIR/internal/server/pipeline-ui/dist"
    cp -r "$PROJECT_DIR/pipeline-ui/dist" "$PROJECT_DIR/internal/server/pipeline-ui/dist"
    echo "==> Frontend dist synced to embedded location"
  fi

  if [ -d "$PROJECT_DIR/capacity-ui" ]; then
    echo "==> Building capacity-ui (frontend)..."
    (cd "$PROJECT_DIR/capacity-ui" && npm install --silent && npm run build) || {
      echo "ERROR: capacity-ui build failed" >&2
      exit 1
    }
    rm -rf "$PROJECT_DIR/internal/server/capacity-ui/dist"
    cp -r "$PROJECT_DIR/capacity-ui/dist" "$PROJECT_DIR/internal/server/capacity-ui/dist"
    echo "==> Capacity UI dist synced to embedded location"
  fi

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
elif [ -d "$PROJECT_DIR/pipeline-ui" ]; then
  # Even with --skip-build, check if dist changed and rebuild Go if needed
  if [ -d "$PROJECT_DIR/pipeline-ui/dist" ] && [ -d "$PROJECT_DIR/internal/server/pipeline-ui/dist" ]; then
    FRONTEND_HASH=$(find "$PROJECT_DIR/pipeline-ui/dist" -type f -exec stat -c '%Y' {} + 2>/dev/null | sort | tail -1)
    EMBEDDED_HASH=$(find "$PROJECT_DIR/internal/server/pipeline-ui/dist" -type f -exec stat -c '%Y' {} + 2>/dev/null | sort | tail -1 2>/dev/null || echo "0")
    if [ "$FRONTEND_HASH" != "$EMBEDDED_HASH" ]; then
      echo "==> Detected frontend dist changed, rebuilding Go binary..."
      rm -rf "$PROJECT_DIR/internal/server/pipeline-ui/dist"
      cp -r "$PROJECT_DIR/pipeline-ui/dist" "$PROJECT_DIR/internal/server/pipeline-ui/dist"
      go build -o "$BINARY" "$PROJECT_DIR/cmd/o11y-workbench-web/" || {
        echo "ERROR: Go rebuild failed" >&2
        exit 1
      }
      echo "==> Go binary rebuilt with updated embedded assets"
    fi
  fi
fi

# Dual-train pins: in-process default comes from the Go binary (0.151.x);
# extra OTTL trains are worker subprocesses listed in versions/otel.env.
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

# Pipeline Evaluation Prepare needs a pinned Phase2 collector binary. Inventory
# is resolved at server start; missing binary yields empty /api/capacity/builds
# and Prepare fails closed with: build "phase2-admitted" is not admitted.
PHASE2_BIN="${O11Y_PHASE2_BIN:-}"
if [ -z "$PHASE2_BIN" ] && [ -n "${O11Y_PHASE2_BASE:-}" ]; then
  PHASE2_BIN="${O11Y_PHASE2_BASE%/}/bin/otelcol-phase2"
fi
if [ -z "$PHASE2_BIN" ]; then
  PHASE2_BIN="$PROJECT_DIR/internal/capacity/phase2/bin/otelcol-phase2"
fi
if [ ! -x "$PHASE2_BIN" ]; then
  echo "WARNING: pinned Phase2 collector missing at $PHASE2_BIN" >&2
  echo "WARNING: /api/capacity/builds will be empty; Prepare will fail with" >&2
  echo "WARNING:   build \"phase2-admitted\" is not admitted" >&2
  echo "WARNING: fix: make build-otelcol-phase2   # then restart this serve" >&2
  echo "WARNING: (pipeline prototyper / OTTL still work without it)" >&2
else
  echo "==> Phase2 admitted binary: $PHASE2_BIN"
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

HOST="localhost${ADDR}"
HEALTH_URL="http://${HOST#:}/api/health"
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
