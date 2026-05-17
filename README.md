# o11y workbench

**Deterministic mock OTLP telemetry generator** for testing distributed system scenarios, trace-ID-aware load balancing, [OpenTelemetry Transformation Language (OTTL)](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl) prototyping, and pipeline authoring.

Generate structurally valid OTLP traces, logs, and metrics with reproducible output — feed them into any collector (Jaeger, Datadog, Grafana, etc.), inspect locally as JSON/protobuf files, or prototype collector pipelines in the browser.

## Features

- **Deterministic generation** — same seed always produces identical traces, logs, and metrics. Reproduce bugs, share test cases.
- **Multi-signal** — generate traces, logs, and metrics together or independently, all in OTLP wire format.
- **Multiple topologies** — flat, branching, split trace shapes.
- **Built-in scenarios** — microservice, monolith, 3-tier-web-app, event-driven architectures with realistic service graphs.
- **User-defined scenarios** — define custom service graphs in YAML/JSON.
- **Flexible output** — write to files (JSON/protobuf) or stream via OTLP gRPC to any collector.
- **Multi-endpoint routing** — distribute telemetry across collector endpoints by region.
- **Rate-limited generation** — pace trace output to a target traces/second for load testing.
- **OTTL Prototyper** — web-based sandbox to write, validate, and test OTTL transform and filter statements against live generated telemetry.
- **Pipeline Prototyper** — visual, multi-signal pipeline builder that mirrors the OTel Collector's processor model. Chain transform/filter stages with `error_mode` support and per-stage result inspection.
- **Web dashboard** — browser UI for interactive generation, run management, scenario management, trace import, and OTTL prototyping.
- **Run lifecycle management** — start, stop, pause, and resume generation jobs with live progress tracking.
- **Trace import** — upload existing OTLP JSON traces for inspection and OTTL experimentation.
- **YAML-first configuration** — CLI flags, env vars, and config files all backed by the same schema.

## Quick Start

### CLI

```bash
# Generate 5 flat-topology traces, output as JSON files
go run ./cmd/o11y-workbench --traces 5 --topology flat --format json

# Generate traces using the microservice scenario, streamed via OTLP
go run ./cmd/o11y-workbench --scenario microservice --output otlp --endpoint localhost:4317

# Load a user-defined scenario file
go run ./cmd/o11y-workbench --scenario my-app --scenario-file ./scenarios.yaml

# Generate traces + logs + metrics together
go run ./cmd/o11y-workbench --traces 3 --scenario microservice \
  --signal-type both --log-count 20 --metric-count 30
```

### Web Dashboard

```bash
# Start the web server (builds frontend + Go binary, waits until ready)
./scripts/serve.sh

# Open http://localhost:8080
```

The dashboard lets you configure telemetry parameters, run generation jobs (including logs and metrics), inspect results, import traces, and experiment with OTTL transform/filter statements.

### Pipeline Prototyper

Enable the visual pipeline builder with the `--pipeline-prototyper` flag:

```bash
./scripts/serve.sh          # enabled by default
# Or via env var:
export O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true
```

Open [http://localhost:8080/pipeline/](http://localhost:8080/pipeline/) for the Svelte SPA.

### Docker

```bash
docker build -f Dockerfile.web -t o11y-workbench-web .
docker run -p 8080:8080 o11y-workbench-web
```

## CLI Reference

### `o11y-workbench` — Telemetry Generator

```
Usage: o11y-workbench [flags]

Flags:
      --config string            Config file path (YAML, JSON, or TOML)
      --seed int                 RNG seed for deterministic generation (default 42)
      --traces int               Number of traces to generate (default 1)
      --rate int                 Target traces per second (0 = unlimited)
      --topology string          Trace shape: flat, branching, split (default "flat")
      --scenario string          Named scenario: microservice, monolith, 3-tier-web-app,
                                 event-driven, or a user-defined scenario
      --scenario-file string     Path to YAML/JSON with user-defined scenario definitions
      --scenario-span-count int  Per-service span budget for scenarios (0 uses --span-count)
      --span-count int           Target spans per trace (default 10)
      --max-depth int            Maximum span depth in branching/split topologies (default 5)
      --service-name string      Service name for resource attributes (default "mock-service")
      --signal-type string       Signals to generate: "traces", "logs", "metrics", "both" (default "traces")
      --log-count int            Number of log records to generate (default 10)
      --logs-per-span int        Log records per span (default 3)
      --metric-count int         Number of metric data points to generate (default 20)
      --output string            Output mode: file, otlp (default "file")
      --format string            Output format for file mode: json, protobuf (default "json")
      --output-dir string        Output directory for file mode (default "./traces")
      --endpoint strings         Collector endpoint(s) — repeatable (e.g. --endpoint localhost:4317)
      --insecure                 Use insecure gRPC connection (default true)
```

Precedence: CLI flag > environment variable (`O11Y_WORKBENCH_*`) > config file > built-in default.

### `o11y-workbench-web` — Web Dashboard Server

```
Usage: o11y-workbench-web [flags]

Flags:
      --addr string              HTTP listen address (default ":8080")
      --jobs string              Path to JSON file with startup job definitions
      --scenario-file string     Path to YAML/JSON with user-defined scenario definitions
      --pipeline-prototyper      Enable the pipeline prototyper UI and API
```

## Web Dashboard & API

The web dashboard at `http://localhost:8080` provides:

- **Configuration panel** — seed, trace count, rate, topology, span count, max depth, scenario selection, signal type (traces/logs/metrics), output mode, and endpoints.
- **Run management** — start, stop, pause, and resume trace generation runs with live progress tracking and real-time span/log/metric counts.
- **Trace viewer** — inspect generated trace JSON with syntax highlighting.
- **Trace import** — upload existing OTLP JSON traces for analysis and OTTL experimentation.
- **Scenario upload** — register user-defined scenarios via YAML/JSON.
- **OTTL Prototyper** — interactive sandbox for transform and filter statements against generated data.
- **Signal selection** — generate traces, logs, and metrics independently or together.

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Health check |
| `GET` | `/api/scenarios` | List built-in and user-defined scenarios |
| `POST` | `/api/scenarios` | Upload user-defined scenarios (YAML/JSON) |
| `GET` | `/api/config/defaults` | Get default configuration values |
| `POST` | `/api/runs` | Start a new trace generation run |
| `GET` | `/api/runs` | List all runs |
| `GET` | `/api/runs/{id}` | Get run details and telemetry data |
| `POST` | `/api/runs/import` | Import trace data from OTLP JSON |
| `POST` | `/api/runs/{id}/stop` | Stop a running job |
| `POST` | `/api/runs/{id}/pause` | Pause a running job |
| `POST` | `/api/runs/{id}/resume` | Resume a paused job |
| `POST` | `/api/ottl/validate` | Validate OTTL transform statements |
| `POST` | `/api/ottl/transform` | Execute OTTL transform against trace data |
| `POST` | `/api/ottl/filter` | Execute OTTL filter against generated traces |
| `POST` | `/api/ottl/filter-validate` | Validate OTTL filter conditions |
| `POST` | `/api/ottl/validate-html` | Validate OTTL statements (HTML response) |
| `POST` | `/api/ottl/transform-html` | Execute OTTL transform (HTML response) |
| `POST` | `/api/ottl/filter-html` | Execute OTTL filter (HTML response) |
| `POST` | `/api/ottl/pipeline/validate` | Validate a full pipeline definition |
| `POST` | `/api/ottl/pipeline/execute` | Execute a pipeline against trace data |
| `POST` | `/api/ottl/pipeline/import-config` | Import OTel Collector config YAML |
| `GET` | `/` | Serves the web dashboard (HTMX) |
| `GET` | `/pipeline/` | Serves the pipeline prototyper SPA |

## OTTL Prototyper

The **OTTL Prototyper** is a browser-based sandbox for writing and testing [OpenTelemetry Transformation Language](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl) statements — no collector setup required.

### Transform Mode

Write OTTL transform statements to modify span attributes, names, statuses, and more. The prototyper applies them against generated telemetry and shows before/after diffs.

Example transform config (YAML):

```yaml
trace_statements:
  - context: span
    statements:
      - set(attributes["env"], "staging")
      - set(name, "processed: " + name)
```

### Filter Mode

Write OTTL filter conditions to select or exclude spans. The prototyper evaluates conditions against generated traces and reports match statistics.

```yaml
filter:
  traces:
    span:
      - attributes["http.method"] == "GET"
      - IsRootSpan() == true
```

The YAML config can also be provided as a flat list of statements, a simple `statements:` block, or raw OTTL lines.

### HTML Endpoints

The prototyper also exposes `*-html` variants (`/api/ottl/transform-html`, `/api/ottl/filter-html`, `/api/ottl/validate-html`) that return rendered HTML fragments for server-driven UI updates via HTMX.

## Pipeline Prototyper

The **Pipeline Prototyper** is a visual pipeline builder (Svelte SPA) that mirrors the OTel Collector's pipeline model. It supports:

- **Multi-signal pipelines** — separate pipeline definitions for traces, logs, and metrics.
- **Visual builder** — drag-and-drop pipeline topology with a swimlane flow diagram.
- **Processor chain** — chain `transform` and `filter` processors in order with `error_mode` support.
- **Per-stage result inspection** — see the output after each processor stage.
- **OTel Config import** — import existing `pipelines:` blocks from collector config YAML.
- **Export to YAML** — download pipeline definitions as OTel-compatible config.
- **Statement snippets** — built-in library of common OTTL transform/filter statements.

### Architecture

- **Backend**: Pipeline data model and execution engine in `internal/ottl/`
- **API**: REST endpoints at `/api/ottl/pipeline/*`
- **Frontend**: Svelte SPA served at `/pipeline/`
- **Enabled via**: `--pipeline-prototyper` flag or `O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true` env var

### Build Prerequisites

- Node.js 18+ required for building the pipeline UI
- The `./scripts/serve.sh` command handles frontend builds automatically

## Scenarios

### Built-in Scenarios

| Scenario | Description |
|----------|-------------|
| `microservice` | Distributed request flowing through multiple services with internal calls |
| `monolith` | Single-service architecture with internal handler spans |
| `3-tier-web-app` | Three-tier topology (frontend → backend → database) |
| `event-driven` | Event publication and consumption across services |

### User-Defined Scenarios

Define custom service graphs in YAML/JSON and load them via `--scenario-file` or the web dashboard upload.

```yaml
my-api:
  roots:
    - service: api-gateway
      children:
        - service: auth-service
          children:
            - service: user-db
        - service: payment-service
          children:
            - service: ledger-db
```

Each node specifies a service name and optional nested children. Scenarios support configurable span budgets and produce realistic trace trees with timing, attributes, and random errors.

## Output Formats

### File Mode (default)

Writes each trace to `./traces/<traceID>/<endpoint>/spans.{json,pb}`. Each output file is a self-contained `ExportTraceServiceRequest` that can be sent directly to a collector.

```bash
# Send a generated trace to a collector
curl -X POST -H "Content-Type: application/x-protobuf" \
  --data-binary @./traces/abc123/default/spans.pb \
  http://collector:4318/v1/traces
```

### OTLP Mode

Streams telemetry via gRPC to one or more collector endpoints. The `--endpoint` flag supports multiple values:

```bash
o11y-workbench --output otlp \
  --endpoint localhost:4317 \
  --endpoint collector:4317
```

Telemetry is routed by `cloud.region` resource attribute — each region's spans go to the corresponding endpoint.

## Configuration

Configuration is resolved with this precedence (highest to lowest):

1. CLI flags
2. Environment variables (`O11Y_WORKBENCH_SEED`, `O11Y_WORKBENCH_TRACES`, etc.)
3. Config file (YAML, JSON, or TOML)
4. Built-in defaults

Example config file (`config.yaml`):

```yaml
seed: 42
traces: 10
topology: branching
span_count: 15
max_depth: 4
output: otlp
endpoints:
  - localhost:4317
  - collector:4317
```

## Development

### Building

```bash
# Build pipeline UI (Svelte SPA) + copy embedded assets
make build-pipeline-ui

# Build everything (frontend + Go binary)
make build-all

# Build + serve with auto-rebuild
./scripts/serve.sh
```

### Testing

```bash
# Go tests
go test ./...

# E2E smoke tests (requires running server)
make test        # headless
make test:headed # headed browser
```

### Project Structure

```
cmd/
  o11y-workbench/          # CLI generator binary
  o11y-workbench-web/      # Web dashboard server binary
internal/
  export/                  # Output exporters (file + OTLP gRPC)
    file.go                #   JSON/protobuf file export
    otlp.go                #   OTLP gRPC trace export
    otlp_log.go            #   OTLP gRPC log export
    otlp_metric.go         #   OTLP gRPC metric export
  gen/                     # Trace/log/metric generation engine
    generator.go           #   Trace generator
    log_generator.go       #   Log generator
    metric_generator.go    #   Metric generator
    topology.go            #   Trace topology shapes
    scenario.go            #   Built-in architecture scenarios
    scenario_file.go       #   User-defined scenario parser
    rng.go                 #   Deterministic seeded RNG
  ottl/                    # OTTL execution engine
    executor.go            #   OTTL transform executor
    filter.go              #   OTTL filter executor
    pipeline_executor.go   #   Multi-stage pipeline executor
    pipeline_types.go      #   Pipeline data model
  region/                  # Multi-endpoint routing
  server/                  # Web server + HTMX dashboard
    web/                   #   Static web assets (HTML/CSS/JS)
    pipeline-ui/           #   Embedded Svelte SPA (built)
    run.go                 #   Run lifecycle management
    import.go              #   Trace import handler
    pipeline_handlers.go   #   Pipeline prototyper API
pipeline-ui/               # Svelte SPA for pipeline prototyper
  src/                     #   Svelte components
  e2e/                     #   Playwright e2e tests
testdata/                  # Sample data for imports
tests/                     # Playwright smoke tests
scripts/
  serve.sh                 # Build and serve launcher
```
