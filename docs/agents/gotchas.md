# Agent gotchas

Load this before serving, Docker, or `pipeline-ui` rebuilds.

## Serve

- Always start the web server with `./scripts/serve.sh` (or `make serve`). It builds Svelte, builds Go, kills the old process, `nohup`s `/tmp/o11y-workbench-web`, polls `/api/health`.
- Cleanup: `kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null`
- The running binary is `/tmp/o11y-workbench-web`, not a tree-local `o11y-workbench-web`.
- `//go:embed pipeline-ui/dist` — Svelte is baked in at Go build. Copying `internal/server/pipeline-ui/dist/` after the fact does nothing. After `pipeline-ui/` edits, run `./scripts/serve.sh`.
- `--pipeline-prototyper` is **not** default on the binary. `serve.sh` passes it. Direct runs need the flag or `O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true`.
- Do not edit gitignored `internal/server/pipeline-ui/dist/`.

## HMR vs production

Two-terminal HMR (no Go rebuild between Svelte edits):

```bash
go run ./cmd/o11y-workbench-web --pipeline-prototyper
cd pipeline-ui && npm run dev
```

Vite proxies `/api/*` to `:8080`. Production-style: `./scripts/serve.sh`.

## Docker / IDs / routes

- `Dockerfile.web` does not build the Svelte app. `pipeline-ui/dist/` must exist before `docker build`.
- Run IDs are auto-incremented (`runIDCounter` in `@internal/server/run.go`), not UUIDs.
- Go 1.26+ route syntax: `GET /api/runs/{id}`.

## Docs drift

- README lists CLI `--signal-type`, `--log-count`, `--metric-count`. Those flags are **not** in `@cmd/o11y-workbench/main.go`.

