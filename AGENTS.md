# o11y workbench — Agent Guide

Deterministic mock OTLP telemetry generator (traces, logs, metrics) for testing OTel collectors, OTTL prototyping, and visual pipeline authoring.

---

## Project Map

```
cmd/
  o11y-workbench/          CLI binary — generate telemetry from the shell
  o11y-workbench-web/      Web server binary — dashboard + API + pipeline prototyper
internal/
  export/                  Output exporters (file, OTLP gRPC) for traces/logs/metrics
  gen/                     Generation engine (traces, logs, metrics, scenarios, topologies)
  ottl/                    OTTL execution engine (transform, filter, pipeline executor)
  region/                  Multi-endpoint routing by cloud.region
  server/                  HTTP server, HTMX dashboard, run management, import
pipeline-ui/               Svelte SPA for the visual pipeline prototyper
  src/lib/                 Svelte components (FlowBuilder, EditorPanel, PipelineBuilder, etc.)
tests/                     Playwright smoke tests (JS)
testdata/                  Sample OTLP JSON for import testing
scripts/serve.sh           Agent-safe build+launch (the ONE approved way to start the server)
```

---

## Server Launch Protocol (CRITICAL)

**Always use `scripts/serve.sh` to start the web server.** Do not use `go run`, `go build && ./binary &`, or any other chaining pattern.

```bash
./scripts/serve.sh                          # build + start + wait for ready
./scripts/serve.sh --skip-build             # skip build if binary is current
./scripts/serve.sh --addr :9090             # custom port
```

What it does: builds the Svelte frontend → builds Go binary → kills old instance → starts with `nohup` + `disown` → polls `/api/health` every 500ms → exits 0 when ready, 1 on failure.

**Cleanup when done:**
```bash
kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null
```

### ⚠️ Go Embeds Frontend at Build Time

The Go server uses `//go:embed pipeline-ui/dist` — the frontend is **baked into the binary**. You cannot change Svelte code and restart the binary; you must rebuild Go. `serve.sh` handles this atomically.

After any `pipeline-ui/` change, you must run `./scripts/serve.sh` (which rebuilds everything). The binary at `/tmp/o11y-workbench-web` is independent of the source tree — copying `internal/server/pipeline-ui/dist/` after the fact does nothing.

---

## Build System

| Command | What it does |
|---------|-------------|
| `make build-pipeline-ui` | Build Svelte SPA + copy to embedded location |
| `make build-all` | Build everything (frontend + Go) |
| `make serve` | Alias for `./scripts/serve.sh` |
| `make test` | `go test ./...` |
| `go generate ./...` | Trigger `make build-pipeline-ui` via `//go:generate` |

**Go version**: 1.26+ (from `go.mod`). Uses standard library `embed`, `net/http`, `ServeMux` routing (Go 1.22+ pattern matching).

**Frontend**: Plain HTML/CSS/JS with HTMX 2.0 for the main dashboard. Svelte SPA (`pipeline-ui/`) with Vite for the pipeline prototyper. Node.js 18+ required.

---

## Code Conventions

### Go

- **No framework** — standard `net/http` with Go 1.22+ route patterns (`GET /api/runs/{id}`).
- **JSON everywhere** — all API responses use `encoding/json`. Protobuf only in the export layer.
- **Errors** — wrap with `fmt.Errorf("context: %w", err)`. No sentinel errors, no panic.
- **Config** — `github.com/spf13/cobra` + `viper` for CLI, env vars (`O11Y_WORKBENCH_*`), and config files.
- **Logging** — `log.Printf` / `log.Fatal` in main. `testing.T` logging in tests. No structured logger dependency at runtime (zap is transitive).
- **Exporter interface** — `export.Exporter` (traces), `export.LogExporter` (logs), `export.MetricExporter` (metrics). Each has `Export(ctx, data)` and `Shutdown(ctx)`.

### Dashboard (HTMX)

The main dashboard (`internal/server/web/`) is a single-page HTMX app with server-rendered HTML fragments:

- `index.html` — shell with config form, run list, OTTL panels
- `styles.css` — all styling (no framework)
- `scripts.js` — HTMX-driven interactions + party mode toggle
- Server handlers return HTML fragments for OTTL endpoints (`*-html` routes) and run progress updates

### Pipeline UI (Svelte)

The pipeline prototyper (`pipeline-ui/`) is a Svelte 5 SPA with:

- `FlowBuilder.svelte` / `SwimlaneFlowBuilder.svelte` — visual pipeline graph via SVG
- `EditorPanel.svelte` — YAML/JSON editor for processor configs
- `PipelineBuilder.svelte` — single-signal pipeline view
- `RunSelector.svelte` — picks a completed run from the server to feed into the pipeline
- `ResultsView.svelte` — unified diff display for before/after pipeline execution
- `types.ts` — shared TypeScript types (`Pipeline`, `Stage`, `ProcessorConfig`, `PipelinesBySignal`)
- `export-yaml.ts` — OTel-compatible YAML export

### Generator

- `gen.Config` — all generation parameters, passed to `gen.NewGenerator(cfg)` then `generator.Run(ctx, exporter)`
- Topologies (`flat`, `branching`, `split`) are deterministic from seed. Trace/span IDs use a separate RNG so IDs are globally unique per run.
- Scenarios (`microservice`, `monolith`, `3-tier-web-app`, `event-driven`) produce service-graph-based traces.
- User-defined scenarios are YAML/JSON files parsed by `gen.ParseScenarioFile()`.

---

## API Overview (Key Endpoints)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/health` | Readiness check |
| `POST` | `/api/runs` | Start a generation run |
| `GET` | `/api/runs/{id}` | Get run result + telemetry data |
| `POST` | `/api/runs/{id}/stop` | Stop running job |
| `POST` | `/api/runs/{id}/pause` | Pause running job |
| `POST` | `/api/runs/{id}/resume` | Resume paused job |
| `POST` | `/api/runs/import` | Import OTLP JSON trace |
| `POST` | `/api/ottl/transform` | Apply OTTL transform to trace data |
| `POST` | `/api/ottl/filter` | Apply OTTL filter to traces |
| `POST` | `/api/ottl/transform-html` | Transform with HTML response (HTMX) |
| `POST` | `/api/ottl/pipeline/execute` | Execute multi-stage pipeline |
| `POST` | `/api/ottl/pipeline/import-config` | Import OTel collector config YAML |
| `GET` | `/` | HTMX dashboard |
| `GET` | `/pipeline/` | Svelte pipeline prototyper SPA |

Full list in `internal/server/server.go` (`registerRoutes`).

---

## Testing

```bash
go test ./...                                              # all Go tests
go test -run TestIntegration ./internal/server/            # run lifecycle integration tests
npx playwright test --config tests/playwright.config.js    # browser smoke tests (server must be running)
```

- Integration tests (`internal/server/integration_test.go`) create real runs with `StartRun()` and verify lifecycle transitions.
- Run unit tests (`internal/server/run_test.go` — if exists) for store operations.
- Generator tests (`internal/gen/gen_test.go`) verify deterministic output.
- Pipeline tests (`internal/ottl/pipeline_executor_test.go`) test multi-stage execution.

---

## Common Gotchas for Agents

1. **Stale embedded assets** — After editing Svelte code, you MUST run `./scripts/serve.sh` (not just restart the binary). The frontend won't update otherwise.
2. **serve.sh writes binary to /tmp** — The binary at `/tmp/o11y-workbench-web` is what runs. The source tree `o11y-workbench-web` at root is NOT the binary.
3. **`--pipeline-prototyper` is NOT default in the binary** — `serve.sh` passes it, but if you run the binary directly you must add `--pipeline-prototyper` or set `O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true`.
4. **`internal/server/pipeline-ui/dist/` is gitignored but synced** — The embedded dist is copied here by the build. It's in `.gitignore`. Don't edit it directly.
5. **Go 1.26+ route syntax** — Routes use `METHOD /path/{param}` syntax (e.g., `GET /api/runs/{id}`). Not compatible with older Go versions.
6. **HTMX + HTML endpoints** — OTTL handlers have dual JSON and HTML variants. The HTML variants return fragments for HTMX swapping. Always test both if you change OTTL handler logic.
7. **Dockerfile doesn't include pipeline-ui** — `Dockerfile.web` builds the Go binary but does NOT build the Svelte frontend. The embedded `pipeline-ui/dist/` must exist before docker build.
8. **Run IDs are auto-incremented** — `runIDCounter` in `internal/server/run.go`. Don't assume UUIDs.

---

## Quick Reference

```bash
# Build and launch everything
./scripts/serve.sh

# Run Go tests only (no server needed)
go test ./...

# Run just the CLI generator (no server)
go run ./cmd/o11y-workbench --traces 3 --topology branching

# Clean up server
kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null
```
