# Agent gotchas

Load this before serving, Docker, or `pipeline-ui` rebuilds.

## Serve

- Always start the web server with `./scripts/serve.sh` (or `make serve`). It builds the Svelte UI, builds Go, kills the old process, `nohup`s `/tmp/o11y-workbench-web`, polls `/api/health`.
- Cleanup: `kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null`
- The running binary is `/tmp/o11y-workbench-web`, not a tree-local `o11y-workbench-web`.
- `//go:embed pipeline-ui/dist` — Svelte is baked in at Go build. Copying `internal/server/pipeline-ui/dist/` after the fact does nothing. After `pipeline-ui/` edits, run `./scripts/serve.sh`.
- `//go:embed capacity-ui/dist` — same bake-in for `/capacity/`. `make build-capacity-ui` runs `capacity-ui` build and syncs into `internal/server/capacity-ui/dist`. `./scripts/serve.sh` also builds/syncs capacity-ui when present.
- `--pipeline-prototyper` is **not** default on the binary. `serve.sh` passes it. Direct runs need the flag or `O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true`.
- Do not hand-edit gitignored `internal/server/pipeline-ui/dist/`.

## HMR vs production

Two-terminal HMR (no Go rebuild between Svelte edits):

```bash
go run ./cmd/o11y-workbench-web --pipeline-prototyper
cd pipeline-ui && npm run dev
```

Vite proxies `/api/*` to `:18080` (default listen addr; override with `--addr` / `WEB_PORT`). Production-style: `./scripts/serve.sh`. Host port defaults to **18080** so it does not clash with a local process on `:8080`.

## Docker / compose / remotetap

- `Dockerfile.web` builds `pipeline-ui` and `capacity-ui` in image stages, then copies dist into `internal/server/*/dist` before the Go build.
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

- README lists CLI `--signal-type`, `--log-count`, `--metric-count`. Those flags are **not** in `@cmd/o11y-workbench/main.go`.

- Capacity authority is **Assess**, not Experimental Plan. Plan-era design docs under `docs/superpowers/specs/` and related QA notes are deprecated/archive banners only — see `docs/capacity-plan-vs-assess.md`.
