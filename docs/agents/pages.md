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

| Name | What |
|---|---|
| Quick Tab | Single-stage transform/filter |
| Graph Builder | Multi-signal swimlane graph |
| Pipeline Builder | Single-signal linear pipeline |
| Config Import | Import OTel collector YAML |
| Stage Editor | Edit a stage’s processor config |
| Execute & Results Panel | Run selector + diff results |
