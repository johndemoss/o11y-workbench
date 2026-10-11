# Page names

Load this when referring to UI in prompts or commits.

## Dashboard (`/`) — vanilla JS `@internal/server/web/`

| Name | What |
|---|---|
| Dashboard | App shell (nav, signal-eq) |
| Config Panel | Generation config form (topology/scenario/logs/metrics). Scenario `workload` selects flat evidence grammar (`workload.span`) |
| Jobs List | Run list with filters, pause/stop |
| Run Detail | Expanded single-run telemetry |
| Pipeline Promo | Link to Pipeline Prototyper |
| Import Modal | OTLP JSON upload |
| Scenario Upload Modal | Custom scenario YAML/JSON |
| OTTL Validate Panel | Validate transform/filter |
| OTTL Transform Panel | Apply transform, show diff |
| OTTL Filter Panel | Apply filter, show diff |

## Pipeline Prototyper (`/pipeline/`) — Svelte `@pipeline-ui/`

Full multi-signal authoring for traces, logs, and metrics (edit/import/export/validate/execute). Remote TAP and Pipeline Evaluation remain separate surfaces.

| Name | What |
|---|---|
| Quick Tab | Single-stage transform/filter |
| Graph Builder | Multi-signal swimlane graph (traces/logs/metrics editable) |
| Pipeline Builder | Per-signal linear pipeline (signal selector) |
| Config Import | Import OTel collector YAML (all signals) |
| Stage Editor | Edit a stage’s processor config |
| Execute & Results Panel | Run + signal selector; executes against TraceData/LogData/MetricData |
