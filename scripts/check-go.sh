#!/usr/bin/env bash
# Both Go modules are publication inputs; ./... at the root omits the worker.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
for tool in go staticcheck govulncheck; do
  command -v "$tool" >/dev/null || {
    echo "Missing $tool; see CONTRIBUTING.md for pinned tool installs." >&2
    exit 2
  }
done

unformatted="$(git ls-files -z '*.go' | xargs -0 gofmt -l)"
if [[ -n "$unformatted" ]]; then
  printf 'Run gofmt on:\n%s\n' "$unformatted" >&2
  exit 1
fi
for module in . workers/ottl-0.162; do
  echo "==> Checking Go module: $module"
  (
    cd "$module"
    go mod verify
    go mod tidy -diff
    go build ./...
    go vet ./...
    staticcheck ./...
    go test -race -timeout 10m ./...
    govulncheck ./...
  )
done
