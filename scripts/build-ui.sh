#!/usr/bin/env bash
# One local build/sync path; all dist files are generated and ignored.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
case "${1:-all}" in
  all|pipeline-ui) targets=(pipeline-ui) ;;
  *) echo "Usage: $0 [all|pipeline-ui]" >&2; exit 2 ;;
esac
for ui in "${targets[@]}"; do
  prefix=/pipeline/
  echo "==> Building $ui from its npm lockfile..."
  (cd "$ROOT/$ui" && npm ci && npm run build)
  node "$ROOT/scripts/check-ui-assets.mjs" "$ROOT/$ui/dist" "$prefix"
  # Validate before replacing the embedded copy; never leave stale hashed chunks.
  rm -rf "$ROOT/internal/server/$ui/dist"
  cp -R "$ROOT/$ui/dist" "$ROOT/internal/server/$ui/dist"
done
