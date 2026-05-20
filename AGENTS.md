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

### Dashboard (Vanilla JS)

The main dashboard (`internal/server/web/`) is a single-page app with vanilla JS (no framework):
- `index.html` — shell with config form, run list, OTTL panels
- `styles.css` — all styling (no framework)
- `scripts.js` — fetch-driven interactions + party mode toggle (no HTMX, no framework)
- Server handlers return JSON; JS updates the DOM directly

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

### Coding Guidelines

These rules go beyond style (lintable) and codify architectural decisions for agents and contributors. Always follow them when adding or modifying code.

#### Interface Usage

1. **Prefer signal-specific interfaces over generic unification** — If two types have different method signatures (e.g., `LogGenerator.Run` needs a `traceSpans` parameter that `MetricGenerator.Run` doesn't), don't force them into a common interface. Use simple dispatch branching at one call site instead. See the ADR in `internal/gen/generator.go` (`// VERDICT: DO NOT IMPLEMENT a strategy pattern.`).

2. **Define interfaces where they are consumed, not where they are implemented** — The consumer defines the contract (minimal, 2-3 methods). The implementation remains anonymous. Example: `export.TraceExporter` is defined in `internal/export/exporter.go`, implemented by `file_exporter.go`, `otlp.go`, and `run.go`'s `bufferExporter`. There is no "exporter registry" or "exporter factory" abstraction layer.

3. **Use composition over interface hierarchy** — Combine multiple implementations via the `multiExporter` pattern (a named slice type that implements the interface by fanning out to each member):
   ```go
   type multiExporter []export.TraceExporter
   func (e multiExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
       for _, exp := range e {
           if err := exp.Export(ctx, spans); err != nil { return err }
       }
       return nil
   }
   ```
   See `internal/server/run.go` for `multiExporter`, `multiLogExporter`, `multiMetricExporter`. Do not add new interface abstraction layers.

4. **Interface methods should be minimal** — `Export(ctx, data) error` + `Shutdown(ctx) error` is the pattern. No lifecycle hooks, no configuration methods on the interface. Config lives on the concrete implementation or in a config struct.

#### Data Handling — Backend Priority

5. **All telemetry data transformation must happen in Go** — The backend is the single source of truth for data computation. The frontend (JS/TS) must only do visualization-only transformations (mapping data to display formats, computing diff views, managing layout state). Examples of valid frontend transformations: `diff.ts` computing span diffs from `intermediate_data` blobs, `ResultsView.svelte` formatting diff results for display.

6. **Data crosses the boundary as `json.RawMessage`** — Use Go structs with `json:"..."` tags for API request/response types. Store telemetry data as `*json.RawMessage` in the server, typed by field name (`TraceData`, `LogData`, `MetricData` on the `Run` struct in `internal/server/run.go`). The frontend receives OTLP JSON blobs directly.

7. **When converting between OTLP JSON and pdata types, use the protojson→proto→pdata sequence** — This is required because OTTL's `pdata` types only accept binary protobuf input. Trace serialization is centralized in `internal/ottl/serialize.go` (`UnmarshalTracesFromJSON`, `MarshalTracesToJSON`). If adding log or metric serialization, extend `serialize.go` — do not re-implement the `protojson.Unmarshal → proto.Marshal → pdata.ProtoUnmarshaler.UnmarshalLogs` sequence inline.

   **Do NOT duplicate** this pattern:
   ```
   filter_log.go — ❌ inline duplicate of the proto→pdata dance
   filter_metric.go — ❌ inline duplicate
   executor.go — ❌ inline duplicate
   serialize.go — ✅ centralized, extend here
   ```

8. **Don't compute semantic summaries or aggregations on the frontend** — If a computation (count, histogram, aggregation, derived metric) could be useful to a non-browser client, it belongs in Go. The frontend may recompute display-oriented diffs (e.g., "which spans changed between stage N and N+1") because those depend on UI state.

#### Type Boundary

9. **Go structs are the source of truth for all type definitions** — When adding a new API endpoint or response type, define the Go struct first (with `json:"..."` tags), then replicate in `pipeline-ui/src/lib/types.ts` as a pure TypeScript interface. Keep the TS types in sync with the Go originals.

10. **Keep TS types as pure interfaces, not classes** — No methods, no constructors, no runtime validation logic. They exist solely for IDE autocompletion and compile-time checking. Runtime validation is the backend's responsibility.

11. **Watch for type drift at the boundary** — The TS `Pipeline` type (`pipeline-ui/src/lib/types.ts`) currently has `receivers` and `exporters` fields that the Go `Pipeline` type (`internal/ottl/pipeline_types.go`) does not. When you see drift, either add the fields to the Go struct or remove them from TS — the Go struct wins.

#### Config Structs

12. **Use a single flat config struct per component** — Passed as a value to the constructor (e.g., `gen.NewGenerator(cfg)`). Nested configs are embedded fields, not pointers. Every config type provides a `DefaultConfig()` constructor function. Example: `gen.Config` embeds `LogConfig` and `MetricConfig` as value fields.

13. **Config structs must be JSON-serializable** — All fields need `json:"name"` tags (and `omitempty` for optional fields). This is required for sending configs through the API and storing them in run history. See `gen.Config` in `internal/gen/generator.go` for the canonical pattern.

#### Error Handling

14. **Always wrap errors with context** — Use `fmt.Errorf("verb resource: %w", err)` consistently. No sentinel errors (`errors.New` at package level), no custom error types except when the error carries structured data (e.g., `export.FormatError` has an `Input` field). No panics in production code.

15. **API handlers return errors as `{"error": "message"}`** — Use the `writeJSON(w, status, map[string]string{"error": msg})` pattern. See `internal/server/server.go` for `writeJSON`. OTTL validation uses a structured `ValidateResult` type (`Valid bool` + `Errors []string`) for multi-error responses.

#### OTTL Integration

16. **The protojson→proto→pdata serialization dance is a known pattern** — Always use the centralized helpers in `internal/ottl/serialize.go` for traces. For logs and metrics, add helpers to `serialize.go` rather than duplicating the sequence inline. If the duplication makes you hesitate, that's the signal to extend serialize.go.

17. **Pipeline stage configurations use `map[string]interface{}`** — This is deliberate because the shape is determined by the OTel collector config schema, not by Go's type system. The type assertions in `internal/ottl/pipeline_import.go` (`.(map[string]interface{})`) are the approved pattern for dynamic YAML/JSON deserialization. For known-shape data, use typed structs with `yaml:"..."` / `json:"..."` tags instead.

#### Server Handlers

18. **Every handler is a method on `*Server`** — Access `s.store` for data. Decode request bodies with `json.NewDecoder(r.Body).Decode(&req)`. Respond with `writeJSON(w, status, result)`. Error responses are `map[string]string{"error": msg}`. See `internal/server/ottl_handlers.go` and `internal/server/run_handlers.go` for the canonical pattern.

19. **Don't create exporters or generators directly inside handlers** — Generation runs are managed by the `executeRun()` function in `internal/server/run.go`. Handlers should call `store.StartRun()` and return a run ID, not construct exporters or generators themselves. The buffer/run pattern handles the full lifecycle (generation → buffer → serialize → store).

#### Tests

20. **Use table-driven tests** — `tests []struct{name string; ...; want ...}` pattern. Name each test case meaningfully. Use `t.Run(name, func(t *testing.T) { ... })` for subtests. The `want`/`got` naming convention is standard across the project.

21. **Integration tests create real runs** — `internal/server/integration_test.go` uses `store.StartRun()`, waits for completion, then asserts lifecycle transitions (`pending → running → completed`) and verifies telemetry data is populated. This is the pattern for testing run behavior.

22. **Use golden files or inline expected data for deterministic output** — Generators are seeded (`gen.Config.Seed`), so output is deterministic. `internal/gen/gen_test.go` verifies deterministic behavior. When testing pipeline execution, inline the YAML/JSON pipeline definition in the test (see `internal/ottl/pipeline_executor_test.go` for examples).

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

1. **Stale embedded assets** — After editing Svelte code, use the two-terminal dev workflow (see "Dev Workflow" below) for hot-reload during development, or run `./scripts/serve.sh` for a production-style rebuild that bakes assets into the binary.
2. **serve.sh writes binary to /tmp** — The binary at `/tmp/o11y-workbench-web` is what runs. The source tree `o11y-workbench-web` at root is NOT the binary.
3. **`--pipeline-prototyper` is NOT default in the binary** — `serve.sh` passes it, but if you run the binary directly you must add `--pipeline-prototyper` or set `O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true`.
4. **`internal/server/pipeline-ui/dist/` is gitignored but synced** — The embedded dist is copied here by the build. It's in `.gitignore`. Don't edit it directly.
5. **Go 1.26+ route syntax** — Routes use `METHOD /path/{param}` syntax (e.g., `GET /api/runs/{id}`). Not compatible with older Go versions.
6. **Dockerfile doesn't include pipeline-ui** — `Dockerfile.web` builds the Go binary but does NOT build the Svelte frontend. The embedded `pipeline-ui/dist/` must exist before docker build.
7. **Run IDs are auto-incremented** — `runIDCounter` in `internal/server/run.go`. Don't assume UUIDs.

---

## Dev Workflow (Svelte SPA)

For rapid iteration on the pipeline prototyper, use two terminals:

**Terminal 1** — Go backend:
```bash
go run ./cmd/o11y-workbench-web --pipeline-prototyper
```

**Terminal 2** — Vite dev server (HMR):
```bash
cd pipeline-ui && npm run dev
```

The Vite config proxies `/api/*` to `http://localhost:8080`, so the Svelte SPA development server talks to the live Go backend. No rebuild needed between changes.

For production builds, continue using `./scripts/serve.sh` which rebuilds everything atomically.

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

---

## Page Naming Reference (for agent communication)

When referencing pages/views in agent prompts, use these canonical names to avoid ambiguity.

### Dashboard (`/`) — HTMX

| Canonical Name | Description |
|---|---|
| **Dashboard** | Top-level HTMX app shell (nav, signal-eq) |
| **Config Panel** | Generation config form |
| **Jobs List** | Run list with filters, pause/stop |
| **Run Detail** | Expanded single-run telemetry data |
| **Pipeline Promo** | Card linking to Pipeline Prototyper |
| **Import Modal** | OTLP JSON upload dialog |
| **Scenario Upload Modal** | Custom scenario YAML/JSON upload |
| **OTTL Validate Panel** | Validates transform/filter statements |
| **OTTL Transform Panel** | Applies transform, shows diff |
| **OTTL Filter Panel** | Applies filter, shows diff |

### Pipeline Prototyper (`/pipeline/`) — Svelte SPA

| Canonical Name | Description |
|---|---|
| **Quick Tab** | Single-stage transform/filter, immediate exec |
| **Graph Builder** | Multi-signal swimlane graph editor |
| **Pipeline Builder** | Single-signal linear pipeline |
| **Config Import** | Modal — import OTel collector YAML |
| **Stage Editor** | Modal — edit a stage's processor config |
| **Execute & Results Panel** | Bottom panel: run selector + diff results |

Full reference with routes at `.sisyphus/pages-reference.md`.
