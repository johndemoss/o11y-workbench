# Page names

Load this when referring to UI in prompts or commits.

## Dashboard (`/`) — vanilla JS `@internal/server/web/`

| Name | What |
|---|---|
| Dashboard | App shell (nav, signal-eq) |
| Config Panel | Generation config form (topology/scenario/logs/metrics). WorkloadProfile overlays may be added without a mode switch (`docs/generator-unification-adr.md`) |
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

## Capacity Assess (`/capacity/`) — Svelte `@capacity-ui/`

| Name | What |
|---|---|
| Capacity Assess (default) | Evidence-constrained Phase 3 assess; capacity authority |
| Assess panel | Required fingerprint fields + fail-closed conclusion / next-steps CTA |
| Assess YAML rail | Collector YAML paste/drop + sample config (no Plan knobs) |
| Fingerprint fields | Required: `collector_version`, `distribution`, `signal`; optional memory limit / payload |
| Capacity Run | Measurement scenarios via `POST /api/capacity/run`; WorkloadProfile knobs + Stats; evidence for Assess — not capacity authority |
| Experimental Plan | Demoted analytical what-if; queue→MiB heuristic only — not capacity authority |
| Plan control rail | Traffic / what-if knobs + YAML compile (Plan mode only) |
| Plan IR canvas / inspector | Bottleneck graph + model presets (Plan mode only) |
