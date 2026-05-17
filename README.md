# o11y workbench

**Deterministic mock OTLP trace generator** for testing distributed system scenarios, trace-ID-aware load balancing, and OpenTelemetry Transformation Language (OTTL) prototyping.

Generate structurally valid OpenTelemetry traces with reproducible outputs — feed them into any trace collector (Jaeger, Datadog, Grafana, etc.) or inspect them locally as JSON/protobuf files.

## Features

- **Deterministic generation** — same seed always produces identical traces. Reproduce bugs, share test cases.
- **Multiple topologies** — flat, branching, split trace shapes.
- **Built-in scenarios** — microservice, monolith, 3-tier-web-app, event-driven architectures.
- **User-defined scenarios** — define custom service graphs in YAML.
- **Flexible output** — write to files (JSON/protobuf) or stream via OTLP gRPC to any collector.
- **Multi-endpoint routing** — distribute traces across collector endpoints by region.
- **OTTL Prototyper** — web-based sandbox to write, validate, and test OTTL transform and filter statements against live generated traces.
- **Web dashboard** — browser UI for interactive trace generation, scenario management, and OTTL prototyping.
- **YAML-first configuration** — CLI flags, env vars, config files, all with the same schema.

## Quick Start

### CLI

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
make serve
# Or: ./scripts/serve.sh

# Open http://localhost:8080
```

The dashboard lets you configure trace parameters, run generation jobs, inspect trace JSON, and experiment with OTTL transform/filter statements in real time.

### Docker

```bash
docker build -f Dockerfile.web -t o11y-workbench-web .
docker run -p 8080:8080 o11y-workbench-web
```

## CLI Reference

### `o11y-workbench` — Trace Generator

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
      --output string            Output mode: file, otlp (default "file")
      --format string            Output format for file mode: json, protobuf (default "json")
      --output-dir string        Output directory for file mode (default "./traces")
      --endpoint strings         Collector endpoint(s) — repeatable (e.g. --endpoint localhost:4317)
      --insecure                 Use insecure gRPC connection (default true)
```

Precedence: CLI flag > environment variable (`OTEL_WORKBENCH_*`) > config file > built-in default.

### `o11y-workbench-web` — Web Dashboard Server

```
Usage: o11y-workbench-web [flags]

Flags:
      --addr string           HTTP listen address (default ":8080")
      --jobs string           Path to JSON file with startup job definitions
      --scenario-file string  Path to YAML/JSON with user-defined scenario definitions
```

## Web Dashboard & API

The web dashboard at `http://localhost:8080` provides:

- **Configuration panel** — seed, trace count, rate, topology, span count, max depth, scenario selection
- **Run management** — start, stop, and inspect trace generation runs with live progress tracking
- **Trace viewer** — inspect generated trace JSON with syntax highlighting
- **Trace import** — upload existing trace JSON for analysis
- **Scenario upload** — register user-defined scenarios via YAML/JSON
- **OTTL Prototyper** — interactive sandbox for transform and filter statements

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/scenarios` | List built-in and user-defined scenarios |
| `POST` | `/api/scenarios` | Upload user-defined scenarios (YAML/JSON) |
| `GET` | `/api/config/defaults` | Get default configuration values |
| `POST` | `/api/runs` | Start a new trace generation run |
| `GET` | `/api/runs` | List all runs |
| `GET` | `/api/runs/{id}` | Get run details and trace data |
| `POST` | `/api/runs/import` | Import trace data from JSON |
| `POST` | `/api/runs/{id}/stop` | Stop a running job |
| `POST` | `/api/ottl/validate` | Validate OTTL transform statements |
| `POST` | `/api/ottl/transform` | Execute OTTL transform against trace data |
| `POST` | `/api/ottl/filter` | Execute OTTL filter against generated traces |
| `POST` | `/api/ottl/filter-validate` | Validate OTTL filter conditions |
| `GET` | `/` | Serves the web dashboard |

## OTTL Prototyper

The **OTTL Prototyper** is a browser-based sandbox for writing and testing [OpenTelemetry Transformation Language](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl) statements — no collector setup required.

### Transform Mode

Write OTTL transform statements to modify span attributes, names, statuses, and more. The prototyper applies them against generated trace data and shows before/after diffs.

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

Example filter conditions (YAML):

```yaml
filter:
  traces:
    span:
      - attributes["http.method"] == "GET"
      - IsRootSpan() == true
```

The YAML config can also be provided as a flat list of statements/conditions, a simple `statements:` block, or raw OTTL lines.

## Pipeline Prototyper

The **Pipeline Prototyper** is a visual pipeline builder that mirrors the OTel Collector's pipeline model. Chain `transform` and `filter` processors in order with `error_mode` support and per-stage result inspection.

### Enabling the Feature

Start the web server with the `--pipeline-prototyper` flag:

```bash
make serve
```

The `serve` target builds the frontend, compiles the server, and starts it with pipeline prototyper flag handling.

Or via environment variable:

```bash
export OTEL_WORKBENCH_PIPELINE_PROTOTYPER=true
./scripts/serve.sh
```

### Build Prerequisites

- Node.js 18+ required for building the pipeline UI
- The `make serve` and `./scripts/serve.sh` commands handle frontend builds automatically

### Architecture

- **Backend**: Pipeline data model and execution engine in `internal/ottl/`
- **API**: REST endpoints at `/api/ottl/pipeline/*`
- **Frontend**: Svelte SPA served at `/pipeline/`

## Scenarios

### Built-in Scenarios

| Scenario | Description |
|----------|-------------|
| `microservice` | Distributed request flowing through multiple services with internal calls |
| `monolith` | Single-service architecture with internal handler spans |
| `3-tier-web-app` | Three-tier topology (frontend → backend → database) |
| `event-driven` | Event publication and consumption across services |

### User-Defined Scenarios

Define custom service graphs in YAML and load them via `--scenario-file` or the web dashboard upload.

```yaml
my-api:
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

Streams traces via gRPC to one or more collector endpoints. The `--endpoint` flag supports multiple values:

```bash
o11y-workbench --output otlp \
  --endpoint localhost:4317 \
  --endpoint collector:4317
```

Traces are routed by `cloud.region` resource attribute — each region's spans go to the corresponding endpoint.

## Configuration

Configuration is resolved with this precedence (highest to lowest):

1. CLI flags
2. Environment variables (`OTEL_WORKBENCH_SEED`, `OTEL_WORKBENCH_TRACES`, etc.)
3. Config file (YAML, JSON, or TOML)
4. Built-in defaults

Example config file (`config.yaml`):

```yaml
seed: 42
traces: 100
rate: 10
topology: branching
span-count: 15
max-depth: 6
scenario: microservice
output: otlp
endpoint:
  - localhost:4317
  - collector:4317
```

## Project Structure

```
├── cmd/
│   ├── o11y-workbench/          # CLI trace generator
│   └── o11y-workbench-web/      # Web dashboard server
├── internal/
│   ├── export/                  # Output exporters (file, OTLP gRPC, JSON)
│   ├── gen/                     # Trace generation engine, scenarios, topology
│   ├── ottl/                    # OTTL parser, executor, filter, config
│   ├── region/                  # Multi-endpoint region routing
│   └── server/                  # Web server, API handlers, run store
├── brand/                       # Brand concept exploration assets
├── Dockerfile.web
├── go.mod
└── go.sum
```

## Build from Source

### Prerequisites

- Go 1.26+
- Make sure your `go.mod` is in sync: `go mod tidy`

### Build

```bash
go build ./cmd/o11y-workbench
go build ./cmd/o11y-workbench-web
```

### Test

```bash
go test ./... -count=1
```

All tests are deterministic (seeded RNG) — output is reproducible across runs.

## Dependencies

- [OpenTelemetry Collector Contrib OTTL](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl)
- [OpenTelemetry Protocol (OTLP)](https://github.com/open-telemetry/opentelemetry-proto)
- [Cobra](https://github.com/spf13/cobra) — CLI framework
- [Viper](https://github.com/spf13/viper) — configuration
- [gRPC](https://google.golang.org/grpc) — OTLP export
