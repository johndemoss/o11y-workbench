# Agent gotchas

Load this before serving, Docker, or `pipeline-ui` rebuilds.

## Serve

- Always start the local web server with `./scripts/serve.sh` (`make serve` starts containers, not this launcher). It builds the Svelte UI, builds Go, builds OTTL worker binaries (`make ottl-workers`), kills the old process, `nohup`s `/tmp/o11y-workbench-web`, polls `/api/health`.
- Cleanup: `kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null` — SIGTERM now shuts down HTTP and supervised OTTL workers.
- The running binary is `/tmp/o11y-workbench-web`, not a tree-local `o11y-workbench-web`.
- `//go:embed pipeline-ui` — only the parent README.txt is tracked; all `dist/` output is generated. Source-only Go builds return a build-required 503 for the pipeline SPA. Generate assets with `go generate ./cmd/o11y-workbench-web` (or `./scripts/build-ui.sh`), then rebuild Go. Copying dist after Go build does nothing. After UI edits, use `./scripts/serve.sh`.
- Local Make/serve/go-generate builds share `scripts/build-ui.sh` (`npm ci`, build, entry-asset validation, sync). A rebuild must leave tracked files unchanged. `serve.sh --skip-build` trusts the existing binary without checking freshness; it is not a rebuild shortcut after edits.
- `--pipeline-prototyper` is **not** default on the binary. `serve.sh` passes it. Direct runs need the flag or `O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true`.
- Do not hand-edit gitignored `internal/server/pipeline-ui/dist/`.

## OTTL collector versions (dual-train)

- In-process default OTTL train is pinned by `go.mod` / `internal/ottlengine.DefaultContribVersion` (currently **0.151.0**).
- Extra trains run as worker subprocesses under `workers/ottl-<ver>/`, built to `bin/ottl-worker-<ver>` via `make ottl-workers`.
- Pin file: `versions/otel.env` (`OTEL_OTTL_DEFAULT_VERSION`, `OTEL_OTTL_WORKER_VERSIONS`). `serve.sh` sources it and exports `O11Y_OTTL_WORKERS=ver=path,...` unless already set.
- `Dockerfile.web` builds the worker into the image and sets `O11Y_OTTL_WORKERS` for compose.
- Pipeline UI picker hits `GET /api/collector-versions`. Version-sensitive OTTL calls pass `?collector_version=` (validate / execute / import-config). Export/diff/graph helpers stay version-agnostic.

## HMR vs production

Two-terminal HMR (no Go rebuild between Svelte edits):

```bash
go run ./cmd/o11y-workbench-web --pipeline-prototyper
cd pipeline-ui && npm run dev
```

Vite proxies `/api/*` to `:18080` (default listen addr; override with `--addr` / `WEB_PORT`). Production-style: `./scripts/serve.sh`. Host port defaults to **18080** so it does not clash with a local process on `:8080`.

## Docker / compose / remotetap

- Fixed `container_name` values (`otel-collector-remotetap`, `o11y-workbench-web`, `demo-sender`) are global. Compose project names differ by worktree directory, so `compose down` in one checkout does **not** remove containers started from another. `make clean` / `make clean-remotetap` and `make dev-remotetap` call `reclaim-named-containers` (`podman/docker rm -f`) for those names.
- If `make dev-remotetap` fails with "name is already in use", run `make clean-remotetap` (or `make reclaim-named-containers`) and retry. Only one worktree can own those names at a time.
- `Dockerfile.web` builds `pipeline-ui` in an image stage, then copies dist into `internal/server/pipeline-ui/dist` before the Go build. It also builds the OTTL worker binary and sets `O11Y_OTTL_WORKERS`.
- `make build-pipeline-ui` alone is **not** enough when UI is served from compose: `make dev-remotetap` (and hybrid compose) serves `localhost/o11y-workbench-web:latest`. Rebuild/recreate that image or UI changes are invisible:
  ```bash
  make build-pipeline-ui
  podman compose -f docker-compose.hybrid.yml build o11y-workbench-web
  podman compose -f docker-compose.hybrid.yml -f docker-compose.hybrid-remotetap.yml \
    --profile remotetap up -d --force-recreate o11y-workbench-web
  ```
  Then hard-refresh the browser (`Ctrl+Shift+R`).
- For local embedded iteration without containers, prefer `./scripts/serve.sh` and open `http://localhost:18080/pipeline/`.

## IDs / routes

- Run IDs are auto-incremented (`runIDCounter` in `@internal/server/run.go`), not UUIDs.
- Go 1.26+ route syntax: `GET /api/runs/{id}`.

## Docs drift

- The batch CLI is traces-only. Do not reintroduce the nonexistent `--signal-type`, `--log-count`, or `--metric-count` flags into README examples.

- `/capacity/` product workflow is **Pipeline Evaluation**; Assess remains advanced evidence authority inside evaluation, not a peer UI mode. Plan/Run/measured product HTTP routes are retired (T36). Plan-era design docs under `docs/superpowers/specs/` are archive banners only — see `docs/capacity-plan-vs-assess.md`.