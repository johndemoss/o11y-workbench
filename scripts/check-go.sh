#!/usr/bin/env bash
# The root Go module omits the versioned OTTL worker modules.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
for tool in go staticcheck govulncheck; do
  command -v "$tool" >/dev/null || {
    echo "Missing $tool; see CONTRIBUTING.md for pinned tool installs." >&2
    exit 2
  }
done

go_files=()
while IFS= read -r -d '' path; do
  [[ -f "$path" ]] || continue
  go_files+=("$path")
done < <(git ls-files -z --cached --others --exclude-standard '*.go')

unformatted=""
if ((${#go_files[@]} > 0)); then
  unformatted="$(gofmt -l "${go_files[@]}")"
fi
if [[ -n "$unformatted" ]]; then
  printf 'Run gofmt on:\n%s\n' "$unformatted" >&2
  exit 1
fi
for module in . workers/ottl-0.163 workers/ottl-0.164; do
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
