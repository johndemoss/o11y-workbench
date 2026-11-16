<div align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo.svg">
    <img src="assets/logo-light.svg" alt="o11y workbench" width="420">
  </picture>
</div>

**Seeded mock OTLP telemetry generator and local development workbench** for testing distributed system scenarios, trace-ID-aware load balancing, [OpenTelemetry Transformation Language (OTTL)](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl) prototyping, and pipeline authoring.

Generate structurally valid OTLP traces with the CLI, or traces, logs, and metrics in the web dashboard. Feed them into a collector, inspect locally as JSON/protobuf files, or prototype collector pipelines in the browser. Seeds reproduce generation choices, not byte-identical telemetry: normal runs use fresh IDs and wall-clock timestamps.

This is an **unauthenticated, trusted single-user development tool**, not a public HTTP service. Keep it on loopback and use synthetic data. Read [SECURITY.md](SECURITY.md) before using real captures or changing network bindings.

See [CONTRIBUTING.md](CONTRIBUTING.md) for verified build/check commands and [AGENTS.md](AGENTS.md) for architecture rules. Project-owned code is [MIT licensed](LICENSE); third-party material retains its own terms.

## Product walkthrough

https://github.com/user-attachments/assets/11daae8b-9264-45f4-b803-7adb1b2e6ab2

## Features

- **Seeded generation** — reproduce topology and workload choices; normal runs retain fresh IDs and timestamps rather than byte-identical output.
- **Multi-signal web workflow** — generate traces, logs, and metrics together or independently, all in OTLP wire format. The batch CLI generates traces only.
- **Multiple topologies** — flat, branching, split trace shapes.
- **Built-in scenarios** — microservice, monolith, 3-tier-web-app, event-driven architectures with realistic service graphs.
- **User-defined scenarios** — define custom service graphs in YAML/JSON.
- **Flexible output** — write to files (JSON/protobuf) or stream via OTLP gRPC to any collector.
- **Multi-endpoint routing** — distribute telemetry across collector endpoints by region.
- **Rate-limited generation** — pace trace output to a target traces/second for load testing.
- **OTTL Prototyper** — web-based sandbox to write, validate, and test OTTL transform and filter statements against generated telemetry, including experimental lambda expressions in supported collector trains.
- **Pipeline Prototyper** — visual, multi-signal pipeline builder that mirrors the OTel Collector's processor model. Chain transform/filter stages with `error_mode` support and per-stage result inspection.
- **Web dashboard** — browser UI for interactive generation, run management, scenario management, trace import, and OTTL prototyping.
- **Run lifecycle management** — start, stop, pause, and resume generation jobs with live progress tracking.
- **Trace import** — upload existing OTLP JSON traces for inspection and OTTL experimentation.
- **Remote TAP** — connect to an OTel Collector `remotetap` WebSocket endpoint to stream live telemetry from collector deployments. Endpoint configuration, connect/disconnect, and live status are in the web dashboard.
- **YAML-first configuration** — CLI flags, env vars, and config files all backed by the same schema.

## Quick Start

Prerequisites: Go **1.26.11+** for the CLI; Node.js **22.22+** (or 24.15+, or 26+) and npm also required for the web UIs.

### CLI (traces only)

```bash
# Generate 5 flat-topology traces, output as JSON files
go run ./cmd/o11y-workbench --traces 5 --topology flat --format json

# Generate traces using the microservice scenario, streamed via OTLP
go run ./cmd/o11y-workbench --scenario microservice --output otlp --endpoint localhost:4317

# Load a user-defined scenario file
go run ./cmd/o11y-workbench --scenario my-app --scenario-file ./scenarios.yaml

```

### Web Dashboard

```bash
# Start the web server (builds frontend + Go binary, waits until ready)
./scripts/serve.sh

# Open http://localhost:18080
```

The dashboard lets you configure telemetry parameters, run generation jobs (including logs and metrics), inspect results, import traces, and experiment with OTTL transform/filter statements.

Compiled SPA assets are generated, not tracked. For a complete web build without
starting a server, run `go generate ./cmd/o11y-workbench-web` followed by
`go build ./cmd/o11y-workbench-web` (or `make build-all`). A plain source-only Go
build keeps dashboard/APIs available but returns an explicit build-required 503
for `/pipeline/`, rather than serving stale or incomplete bundles.
See [CONTRIBUTING.md](CONTRIBUTING.md#source-only-versus-distributable-web-builds).

### Pipeline Prototyper

Enable the visual pipeline builder with the `--pipeline-prototyper` flag:

```bash
./scripts/serve.sh          # enabled by default
# Or via env var:
export O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true
```

Open [http://localhost:18080/pipeline/](http://localhost:18080/pipeline/) for the Svelte SPA.

### Docker

```bash
docker build -f Dockerfile.web -t o11y-workbench-web .
# Host 18080 → container 8080, published on loopback only.
docker run --read-only --tmpfs /tmp -p 127.0.0.1:18080:8080 o11y-workbench-web
```

Docker Compose offers two independent local fixtures:

```bash
# Optional upstream OTel Desktop Viewer + demo sender (viewer at localhost:8000).
docker compose -f docker-compose.yml --profile dev up -d

# Remote TAP: collector + workbench + demo sender; no desktop viewer required.
docker compose -f docker-compose.yml -f docker-compose.dev-remotetap.yml --profile remotetap up -d
```

`make dev` and `make dev-remotetap` provide the corresponding managed workflows.

## CLI Reference

### `o11y-workbench` — Telemetry Generator

```
Usage: o11y-workbench [flags]

Flags:
      --config string            Config file path (YAML, JSON, or TOML)
      --seed int                 RNG seed for deterministic generation (default 42)
      --traces int               Number of traces (0 = unlimited; mutually exclusive with --duration)
      --duration int             Maximum duration in seconds (default 10; mutually exclusive with --traces)
      --rate int                 Target traces per second (0 = unlimited, default 1)
      --topology string          Trace shape: flat, branching, split (default "flat")
      --scenario string          Named scenario: microservice, monolith, 3-tier-web-app,
                                 event-driven, or a user-defined scenario
      --scenario-file string     Path to YAML/JSON with user-defined scenario definitions
      --scenario-span-count int  Per-service span budget for scenarios (0 uses --span-count)
      --span-count int           Target spans per trace (default 10)
      --max-depth int            Maximum span depth in branching/split topologies (default 5)
      --service-name string      Service name for resource attributes (default "mock-service")
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
      --addr string              HTTP listen address (default "127.0.0.1:18080")
      --jobs string              Path to JSON file with startup job definitions
      --scenario-file string     Path to YAML/JSON with user-defined scenario definitions
      --pipeline-prototyper      Enable the pipeline prototyper UI and API
```

## Web Dashboard & API

The web dashboard at `http://localhost:18080` provides:

- **Configuration panel** — seed, trace count, rate, topology, span count, max depth, scenario selection, signal type (traces/logs/metrics), output mode, and endpoints.
- **Run management** — start, stop, pause, and resume trace generation runs with live progress tracking and real-time span/log/metric counts.
- **Trace viewer** — inspect generated trace JSON with syntax highlighting.
- **Trace import** — upload existing OTLP JSON traces for analysis and OTTL experimentation.
- **Scenario upload** — register user-defined scenarios via YAML/JSON.
- **OTTL Prototyper** — interactive sandbox for transform and filter statements against generated data.
- **Remote TAP** — stream live telemetry from an OTel Collector `remotetap` WebSocket endpoint, with connection status and usage monitoring.
- **Signal selection** — generate traces, logs, and metrics independently or together.

### Multi-endpoint routing

When several OTLP endpoints are configured, each signal fans out to all of
them, deliberately not by trace:

- **Spans** are assigned per span, so the spans of one trace intentionally land
  on different collectors. Use this to test trace-ID-aware load balancing and
  backend aggregation: a balancer that keys on trace ID must re-home the pieces,
  and a backend that never sees a whole trace exercises partial-view handling.
- **Correlated log records** follow their span, so every log lands on the exact
  collector that received the span it references.
- **Independent logs and metric series** spread deterministically across the
  configured endpoints, one home per series.

Each endpoint is labeled with a generated region name (`local`,
`endpoint-1`, …) in the `cloud.region` resource attribute, which is also how
runs report their routing in the API.

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Health check (includes registered OTTL trains) |
| `GET` | `/api/collector-versions` | List default + registered OTTL collector trains |
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
| `POST` | `/api/ottl/pipeline/validate` | Validate a full pipeline definition |
| `POST` | `/api/ottl/pipeline/execute` | Execute a pipeline against trace data |
| `POST` | `/api/ottl/pipeline/import-config` | Import OTel Collector config YAML |
| `POST` | `/api/remotetap/connect` | Connect to a collector `remotetap` WebSocket endpoint |
| `POST` | `/api/remotetap/disconnect` | Disconnect from the remote endpoint |
| `GET` | `/api/remotetap/status` | Get remote TAP connection status |
| `GET` | `/api/remotetap/usage/{runID}` | Get remote TAP usage stats for a run |
| `GET` | `/` | Serves the web dashboard |
| `GET` | `/pipeline/` | Serves the pipeline prototyper SPA |

## OTTL Prototyper

The **OTTL Prototyper** is a browser-based sandbox for writing and testing [OpenTelemetry Transformation Language](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl) statements — no collector setup required.

OTTL execution supports Collector Contrib `0.165.0` (in-process default), `0.164.0`, and `0.163.0` (versioned workers). The Pipeline Prototyper selects available trains via `collector_version`; `./scripts/serve.sh` builds the workers and sets `O11Y_OTTL_WORKERS` from `versions/otel.env`. See the [version support policy](docs/otel-version-support.md) for module pins and upgrade steps.

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
- **Remote TAP**: `internal/remotetap/` client connects to the collector `remotetap` WebSocket; it does not require OTel Desktop Viewer
- **Enabled via**: `--pipeline-prototyper` flag or `O11Y_WORKBENCH_PIPELINE_PROTOTYPER=true` env var

### Build Prerequisites

- Node.js 22.22+ required for building the pipeline UI
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
    - service_name: api-gateway
      name: GET /checkout
      kind: SERVER
      children:
        - service_name: auth-service
          name: authorize
          kind: SERVER
          children:
            - service_name: user-db
              name: SELECT users
              kind: CLIENT
        - service_name: payment-service
          name: charge
          kind: SERVER
          children:
            - service_name: ledger-db
              name: INSERT ledger
              kind: CLIENT
```

Every node requires `service_name` and `name`; `kind`, `children`, `min_duration`, `max_duration`, `is_error`, and `attributes` are optional. Scenarios support configurable span budgets and produce realistic trace trees with timing, attributes, and random errors.

## Output Formats

### File Mode (default)

Writes each exported batch to `./traces/<traceID>/<endpoint>/spans-<unique>.{json,pb}`. A trace that spans several services or batches produces several files in its endpoint directory; none overwrites another. Each file is a self-contained `ExportTraceServiceRequest` that can be sent directly to a collector.

```bash
# Send every batch of a generated trace to a collector
for f in ./traces/abc123/default/spans-*.pb; do
  curl -X POST -H "Content-Type: application/x-protobuf" \
    --data-binary @"$f" http://collector:4318/v1/traces
done
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
span-count: 15
max-depth: 4
output: otlp
endpoint:
  - localhost:4317
  - collector:4317
```

Config keys use the same names as the CLI flags (for example `span-count`, `max-depth`, and `endpoint`).

## Development

### Building

```bash
# Build pipeline UI (Svelte SPA) + copy embedded assets
make build-pipeline-ui

# Build everything (frontend + Go binary)
make build-all

# Build + serve (rerun after edits to rebuild)
./scripts/serve.sh
```

### Branding

Olly's wordmark lives in [`assets/logo.svg`](assets/logo.svg) for dark backgrounds and
[`assets/logo-light.svg`](assets/logo-light.svg) for light backgrounds. The square
mascot mark is [`assets/favicon.svg`](assets/favicon.svg). When updating the artwork,
keep the Dashboard copies in `internal/server/web/assets/` and the Pipeline
Prototyper copies in `pipeline-ui/public/` aligned, then rebuild with
`./scripts/serve.sh` to refresh the embedded UI assets.

### Testing

```bash
# Go tests
go test ./...

# Locked browser tooling; installs the Chromium version paired with Playwright
npm ci
npx playwright install chromium
# Stop any existing daemon first; tests build/start/join their own loopback server
npm run test:browser

# Pipeline UI unit tests
(cd pipeline-ui && npm ci && npm test)
```

See [CONTRIBUTING.md](CONTRIBUTING.md#reproducible-browser-smoke-tests) for both-module
checks, platform/browser prerequisites and fresh-checkout verification.

### Project Structure

```
cmd/
  o11y-workbench/          # CLI generator binary
  o11y-workbench-web/      # Web dashboard server binary
internal/
  export/                  # Output exporters (file + OTLP gRPC)
    exporter.go            #   Exporter interface definitions
    file_exporter.go       #   JSON/protobuf file exporter
    file_trace.go          #   Trace file export (JSON/protobuf)
    file_metric.go         #   Metric file export (JSON/protobuf)
    trace_file.go          #   Per-trace file writer
    otlp.go                #   OTLP gRPC trace exporter
    otlpjson.go            #   OTLP JSON marshal/unmarshal utilities
    log_exporter.go        #   OTLP gRPC log exporter
    metric_exporter.go     #   OTLP gRPC metric exporter
    endpoint.go            #   Endpoint configuration
    endpoint_log.go        #   Log-specific endpoint config
    endpoint_metric.go     #   Metric-specific endpoint config
  gen/                     # Trace/log/metric generation engine
    generator.go           #   Trace generator
    log_generator.go       #   Log generator
    metric_generator.go    #   Metric generator
    topology.go            #   Trace topology shapes
    scenario.go            #   Built-in architecture scenarios
    scenario_file.go       #   User-defined scenario parser
    rng.go                 #   Deterministic seeded RNG
  ottl/                    # OTTL execution engine
    config.go              #   OTTL config parsing
    executor.go            #   OTTL transform executor
    filter.go              #   OTTL filter executor
    serialize.go           #   OTLP JSON serialization helpers
    pipeline_executor.go   #   Multi-stage pipeline executor
    pipeline_import.go     #   OTel Collector YAML config import
    pipeline_result.go     #   Pipeline execution results
    pipeline_types.go      #   Pipeline data model
  region/                  # Multi-endpoint routing
  remotetap/               # Remote TAP WebSocket client
    client.go              #   Collector remotetap client
    mock.go                #   Mock remote endpoint for testing
  server/                  # Web server + vanilla JS dashboard
    server.go              #   Route registration, HTTP server setup
    web/                   #   Static web assets (HTML/CSS/JS)
    pipeline-ui/           #   Embedded Svelte SPA (built)
    run.go                 #   Run lifecycle management
    run_handlers.go        #   Run API handlers
    import.go              #   Trace import handler
    ottl_handlers.go       #   OTTL transform/filter API handlers
    pipeline_handlers.go   #   Pipeline prototyper API handlers
    remotetap_handlers.go  #   Remote TAP API handlers
    web_handlers.go        #   Web dashboard handlers
pipeline-ui/               # Svelte SPA for pipeline prototyper
  src/                     #   Svelte components
  e2e/                     #   Playwright e2e tests
testdata/                  # Sample data for imports
tests/                     # Playwright smoke tests
scripts/
  serve.sh                 # Build and serve launcher
```
