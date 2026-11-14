#!/usr/bin/env bash
# scripts/demo-sender.sh start|stop — build and control the local demo sender.
#
# State lives in the private runtime directory (see scripts/runtime.sh) rather
# than a predictable path under a shared temporary directory.
set -euo pipefail

ACTION="${1:-}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SENDER_NAME="o11y-workbench-demo"
# shellcheck source=scripts/runtime.sh
. "$SCRIPT_DIR/runtime.sh"

SENDER_ARGS=(--duration 300 --rate 5 --output otlp --endpoint localhost:4317)

usage() {
  printf '%s\n' \
    'Usage: ./scripts/demo-sender.sh start [sender args...] | stop' \
    'start: build the CLI and launch the demo sender in the background.' \
    '       Extra arguments replace the defaults (--duration 300 --rate 5 --output otlp --endpoint localhost:4317).' \
    'stop:  safely stop the sender recorded in the private runtime directory.' \
    'Runtime state lives in ./.run (override with O11Y_WORKBENCH_RUNTIME_DIR).'
}

case "$ACTION" in
  start) shift; if [ "$#" -gt 0 ]; then SENDER_ARGS=("$@"); fi ;;
  stop) ;;
  --help|-h)
    usage
    exit 0
    ;;
  *)
    echo "ERROR: expected 'start' or 'stop'" >&2
    usage >&2
    exit 1
    ;;
esac

runtime_prepare "$PROJECT_DIR"
PIDFILE="$RUN_DIR/$SENDER_NAME.pid"
BINARY="$RUN_DIR/$SENDER_NAME"
LOCK_DIR="$RUN_DIR/$SENDER_NAME.lock"

runtime_acquire_lock "$LOCK_DIR" demo-sender
trap 'runtime_release_lock "$LOCK_DIR"' EXIT

if [ "$ACTION" = "stop" ]; then
  runtime_stop "$PIDFILE" "$BINARY"
  echo "==> Demo sender stopped (or was not running)"
  exit 0
fi

if [ -e "$PIDFILE" ]; then
  echo "==> Stopping existing demo sender..."
  runtime_stop "$PIDFILE" "$BINARY"
fi

echo "==> Building o11y-workbench CLI..."
tmp_bin=$(mktemp "$RUN_DIR/$SENDER_NAME.build.XXXXXX")
if ! go build -o "$tmp_bin" "$PROJECT_DIR/cmd/o11y-workbench/"; then
  rm -f "$tmp_bin"
  echo "ERROR: Go build failed" >&2
  exit 1
fi
runtime_install_binary "$tmp_bin" "$BINARY"

echo "==> Starting demo sender (${SENDER_ARGS[*]})..."
SENDER_LOG="$RUN_DIR/$SENDER_NAME.log"
: > "$SENDER_LOG"
chmod 600 "$SENDER_LOG"
nohup "$BINARY" "${SENDER_ARGS[@]}" </dev/null >"$SENDER_LOG" 2>&1 &
SENDER_PID=$!
disown "$SENDER_PID" 2>/dev/null || true

if ! runtime_record_pid "$PIDFILE" "$SENDER_PID" "$BINARY" "${SENDER_ARGS[@]}"; then
  kill "$SENDER_PID" 2>/dev/null || true
  echo "       Last lines from $SENDER_LOG:" >&2
  tail -n 8 "$SENDER_LOG" >&2 || true
  exit 1
fi

echo "==> Demo sender running (PID $SENDER_PID); PID record: $PIDFILE"
