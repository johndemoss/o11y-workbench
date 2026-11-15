# Agent gotchas

Load this before serving, Docker, or `pipeline-ui` rebuilds.

## Serve

- Always start the local web server with `./scripts/serve.sh` (`make serve` starts containers, not this launcher). It builds the Svelte UI, builds Go, builds OTTL worker binaries (`make ottl-workers`), stops the daemon it previously started, `nohup`s the binary from its private runtime directory `./.run/`, polls `/api/health`.
- Cleanup: `./scripts/serve.sh --stop` — SIGTERM shuts down HTTP, cancels/joins generated jobs, finalizes TAP capture and stops supervised OTTL workers. The launcher stores a JSON PID record in `./.run/` and verifies process identity before signalling, so do not hand-edit that file.
- Root browser bootstrap: `npm ci`, `npx playwright install chromium`, then `npm run test:browser`. Playwright owns `./scripts/serve.sh --foreground`, health-checks it and SIGTERM/joins it. Foreground mode never changes the daemon PID record. Tests refuse existing servers on port 18080; stop the daemon explicitly first.
- The running binary is `./.run/o11y-workbench-web` (override the directory with `O11Y_WORKBENCH_RUNTIME_DIR`), not a tree-local `o11y-workbench-web`.
- `//go:embed pipeline-ui` — only the parent README.txt is tracked; all `dist/` output is generated. Source-only Go builds return a build-required 503 for the pipeline SPA. Generate assets with `go generate ./cmd/o11y-workbench-web` (or `./scripts/build-ui.sh`), then rebuild Go. Copying dist after Go build does nothing. After UI edits, use `./scripts/serve.sh`.
- Local Make/serve/go-generate builds share `scripts/build-ui.sh` (`npm ci`, build, entry-asset validation, sync). A rebuild must leave tracked files unchanged. `serve.sh --skip-build` trusts the existing binary without checking freshness; it is not a rebuild shortcut after edits.
- `--pipeline-prototyper` is **not** default on the binary. `serve.sh` passes it. Direct runs need the flag or `O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true`.
- Do not hand-edit gitignored `internal/server/pipeline-ui/dist/`.

## OTTL collector versions (three trains)

- In-process default OTTL train is pinned by `go.mod` / `internal/ottlengine.DefaultContribVersion` (currently **0.165.0**, `pkg/ottl v1.4.1`).
- Previous trains **0.164.0** and **0.163.0** run as worker subprocesses under `workers/ottl-<ver>/`, built to `bin/ottl-worker-<ver>` via `make ottl-workers`.
- Pin file: `versions/otel.env` (`OTEL_OTTL_DEFAULT_VERSION`, `OTEL_OTTL_WORKER_VERSIONS`). `serve.sh` sources it and exports `O11Y_OTTL_WORKERS=ver=path,...` unless already set.
- `Dockerfile.web` builds both workers into the image and sets `O11Y_OTTL_WORKERS` for compose.
- Pipeline UI picker hits `GET /api/collector-versions`. Version-sensitive OTTL calls pass `?collector_version=` (validate / execute / import-config). Export/diff/graph helpers stay version-agnostic. See `docs/otel-version-support.md` for the train/module version distinction.
- Lambda expressions are upstream alpha: all three workbench OTTL engines enable `ottl.functions.enableLambda` and register experimental `xottl` converters (e.g. `Filter`). This affects local prototyping only, not the optional remotetap Collector container or an external Collector.

## HMR vs production

Two-terminal HMR (no Go rebuild between Svelte edits):

```bash
go run ./cmd/o11y-workbench-web --pipeline-prototyper
cd pipeline-ui && npm run dev
```

Vite proxies `/api/*` to the fixed target `http://127.0.0.1:18080` (`pipeline-ui/vite.config.ts`); run the Go server on that address for HMR. Production-style: `./scripts/serve.sh`. Host port defaults to **18080** so it does not clash with other listeners on `:8080`.

## Docker / compose / remotetap

- Fixed `container_name` values (`otel-collector-remotetap`, `o11y-workbench-web`, `demo-sender`, `remotetap-demo-sender`, `otel-desktop-viewer`) are global. Compose project names differ by worktree directory, so `compose down` in one checkout does **not** remove containers started from another. `make clean` / `make clean-remotetap` and `make dev-remotetap` call `reclaim-named-containers` (`podman/docker rm -f`) for those names.
- If `make dev-remotetap` fails with "name is already in use", run `make clean-remotetap` (or `make reclaim-named-containers`) and retry. Only one worktree can own those names at a time.
- `make dev` opts into the pinned upstream desktop-viewer image and a demo sender. `make dev-remotetap` runs the collector, web workbench, and demo sender without the viewer or a downloaded host binary.
- `Dockerfile.web` builds `pipeline-ui` in an image stage, then copies dist into `internal/server/pipeline-ui/dist` before the Go build. It also builds the OTTL worker binary and sets `O11Y_OTTL_WORKERS`.
- `make build-pipeline-ui` alone is **not** enough when UI is served from compose: `make dev-remotetap` serves `localhost/o11y-workbench-web:latest`. Rebuild/recreate that image or UI changes are invisible:
  ```bash
  make build-pipeline-ui
  podman compose -f docker-compose.yml build o11y-workbench-web
  podman compose -f docker-compose.yml -f docker-compose.dev-remotetap.yml \
    --profile remotetap up -d --force-recreate o11y-workbench-web
  ```
  Then hard-refresh the browser (`Ctrl+Shift+R`).
- For local embedded iteration without containers, prefer `./scripts/serve.sh` and open `http://localhost:18080/pipeline/`.

## IDs / routes

- Run IDs are auto-incremented (`runIDCounter` in `@internal/server/run.go`), not UUIDs.
- Go 1.26+ route syntax: `GET /api/runs/{id}`.

## Docs drift

- The batch CLI is traces-only. Do not reintroduce the nonexistent `--signal-type`, `--log-count`, or `--metric-count` flags into README examples.