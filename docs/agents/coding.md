# Coding rules

Load this when changing Go, the API, OTTL, or `pipeline-ui` types. Follow these; they are architectural, not style.

## Interfaces

1. Prefer signal-specific interfaces. `LogGenerator.Run` needs `traceSpans`; `MetricGenerator.Run` does not. Do not unify them. ADR: `@internal/gen/generator.go` (`VERDICT: DO NOT IMPLEMENT a strategy pattern`). Dispatch in `executeRun` (`@internal/server/run.go`).
2. Define interfaces where consumed, not where implemented. `export.TraceExporter` lives in `@internal/export/exporter.go`.
3. Compose with named slices (`multiExporter` in `@internal/server/run.go`). No registry/factory layers.
4. Interface surface is `Export(ctx, data) error` + `Shutdown(ctx) error`. Config stays off the interface.

## Data and types

5. Telemetry transforms happen in Go. Frontend does visualization only (`pipeline-ui/src/lib/diff.ts`, `ResultsView.svelte`).
6. Cross the HTTP boundary as `json.RawMessage`. Typed fields: `TraceData`, `LogData`, `MetricData` on `Run`.
7. OTLP JSON → pdata: protojson → proto → pdata. Centralize in `@internal/ottl/serialize.go`. Do not duplicate in `filter_log.go` / `filter_metric.go` / `executor.go`.
8. Counts, histograms, aggregations belong in Go if any non-browser client might need them.
9. Go structs are the type source of truth (`json` tags). Mirror in `@pipeline-ui/src/lib/types.ts` as interfaces only.
10. TS types: no methods, constructors, or runtime validation.
11. On drift, the Go struct wins.

## Config, errors, handlers

12. One flat config struct per component, value-passed, `DefaultConfig()`. Nested configs are embedded values.
13. All config fields need `json` tags (`omitempty` when optional).
14. Wrap errors: `fmt.Errorf("verb resource: %w", err)`. No sentinel errors, no panics.
15. API errors: `writeJSON(w, status, map[string]string{"error": msg})` in `@internal/server/server.go`. OTTL validation uses `ValidateResult`.
16. Extend `@internal/ottl/serialize.go` for new log/metric serde; do not copy the dance.
17. Pipeline stage config is `map[string]interface{}` (collector schema). Type assertions in `@internal/ottl/pipeline_import.go` are approved.
18. Every handler is a method on `*Server`. Decode JSON, `writeJSON`. Canonical: `@internal/server/ottl_handlers.go`, `@internal/server/run_handlers.go`.
19. Handlers must not construct generators/exporters. Call `StartRun(ctx, store, cfg, endpoints, insecure, output)`; `executeRun` owns generator/exporter cleanup. The RunStore owns accepted jobs independently of request cancellation and joins them on Close.
20. Version-sensitive OTTL work goes through `@internal/ottlengine` (`Engine` + `Registry`). Handlers call `resolveEngine` / `ensureEngines` — do not import `internal/ottl` directly for execute/validate/transform/filter once an engine is selected.
21. Extra collector trains are worker subprocesses supervised by `@internal/ottlengine` (`StartConfiguredWorkers`). `server.New` must retain stoppers; `Close` / `Shutdown` / SIGTERM must stop them. Do not discard the stoppers slice.
22. OTTL `collector_version` keys are bare contrib train versions (`0.165.0`), not `pkg/ottl` Go module tags (`v1.4.1`).

## Tests

23. Table-driven: `tests []struct{name string; ...}`, `t.Run`, `want`/`got`.
24. Integration tests create real runs (`@internal/server/integration_test.go`).
25. Seeded generators are deterministic. Inline pipeline YAML/JSON in tests (`@internal/ottl/pipeline_executor_test.go`).
