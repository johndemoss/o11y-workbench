#!/bin/bash
PID=$(cat /tmp/otel-desktop-viewer.pid 2>/dev/null)
[ -n "$PID" ] && kill $PID 2>/dev/null
rm -f /tmp/otel-desktop-viewer.pid
echo "otel-desktop-viewer stopped"