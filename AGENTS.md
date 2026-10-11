# o11y workbench

CLI `cmd/o11y-workbench` is traces-only batch generation. Web `cmd/o11y-workbench-web` is the workbench (logs/metrics, OTTL, pipeline, TAP). Shared engine `internal/gen`. Multi-signal coordinator: `executeRun` in `internal/server/run.go`. Do not invent a product vision or roadmap.

## Commands

```bash
./scripts/serve.sh                         # only approved web start (embeds pipeline-ui)
kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null
go test ./...
go test -run TestName ./internal/pkg
npx playwright test --config playwright.config.js
cd pipeline-ui && npm test
go run ./cmd/o11y-workbench --traces 3 --topology branching
```

`make serve` / `make test` / `make build-pipeline-ui` wrap the same. Go 1.26+. Dashboard: vanilla JS (no HTMX). Pipeline UI: Svelte 5.

## Read when needed

Do not load these unless the task matches. In chat, attach with `@path`.


| When                                      | File                                           |
| ----------------------------------------- | ---------------------------------------------- |
| Changing Go, API, OTTL, or TS types       | `docs/agents/coding.md`                        |
| Serving, Docker, pipeline-ui, OTTL workers | `docs/agents/gotchas.md`                       |
| Naming UI in prompts                      | `docs/agents/pages.md`                         |
| Generator unification (shape vs evidence knobs) | `docs/generator-unification-adr.md` |
| Bumping OTel collector / OTTL / image pins | `docs/otel-version-support.md`                 |
| Adding/checking HTTP routes               | `internal/server/server.go` (`registerRoutes`) |
| Generator ADR / no strategy pattern       | `internal/gen/generator.go`                    |
| Human overview                            | `README.md`                                    |




## Always

- JSON APIs, `net/http` Go 1.22+ routes (`GET /api/runs/{id}`), wrap errors with `%w`
- Go structs win over `pipeline-ui/src/lib/types.ts`
- Handlers call `store.StartRun()`; do not construct generators in handlers
- Versioned OTTL goes through `internal/ottlengine`; retain worker stoppers from `StartConfiguredWorkers` and stop them on Close/SIGTERM. Pins live in `versions/otel.env` (see `docs/agents/gotchas.md`)
- Dashboard and Pipeline Prototyper are one product — not two competing apps



## Learned Workspace Facts

- `cmd/o11y-workbench` generates traces only; logs, metrics, OTTL, pipeline, and Remote TAP live in the web binary via `executeRun`. Both share `internal/gen`.
- Pipeline Prototyper (`/pipeline/`) supports full multi-signal edit/import/export/validate/execute for traces, logs, and metrics; execute selects `TraceData`/`LogData`/`MetricData` via `?signal=`.
- README CLI flags `--signal-type`, `--log-count`, and `--metric-count` are not in `cmd/o11y-workbench/main.go`.

- One generator: `internal/gen`. Shape (topology/scenario) and WorkloadProfile overlays are orthogonal — see `docs/generator-unification-adr.md`.
