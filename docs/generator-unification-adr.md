# ADR — Unify `internal/gen` shape and evidence knobs

Status: **accepted / implemented** (2026-10-10)
Owner surface: `internal/gen`
Related: Dashboard (`/`)

## Context

Product goal: **one generator** — `internal/gen` is the only synthetic ingress authority. Remote TAP remains captured ingress. `phase2/workload` is quarantined.

During Capacity Run productization we added `WorkloadProfile` + `Stats` to `internal/gen`, then wired Capacity Run through `phase2/ingress`. To ship harness parity quickly, `Generator.Run` gained an early return:

```go
if g.config.Workload.Active() {
    return g.runWorkloadProfile(ctx, exp)
}
```

That path emits flat `workload.span` ResourceSpans (one export request, no region fan-out) with evidence attributes (`experiment.id`, `payload.id`, controlled attrs/cardinality) and evidence pacing (items/request, burst, continue-on-export-err).

The main `/` dashboard still only exposes topology/scenario knobs. An interim note said “do not put WorkloadProfile on the dashboard” because activating the profile **bypasses** topology/scenario today.

That interim note overstated a product split. The early return is an **implementation fork**, not a required dual-mode architecture.

## Decision drivers

Capacity Run measurement needs (from Phase 2 harness assumptions):

1. Controlled **export batch size** (`items_per_request`)
2. Controlled **payload complexity** (attribute count/bytes, cardinality)
3. Correlation tags (`experiment.id`, `payload.id`)
4. **Burst** on/off schedule
5. **Continue on export error** so backpressure is measurable, not fatal
6. Often **single-region / single ResourceSpans per request** so request sizing is not obscured by region fan-out

Dashboard / exploratory generation needs:

1. Realistic **shape**: topology trees or named scenarios
2. Multi-service / multi-region distribution when desired
3. Existing rate/duration/count controls
4. Logs/metrics coordination via `executeRun`

These are not mutually exclusive product goals. They are different dimensions of one generator.

## Diagnosis

| Dimension | What it controls | Current topology/scenario path | Current `runWorkloadProfile` path |
|---|---|---|---|
| **Shape** | Span graph / services / names | Topology or scenario trees | Flat `workload.span` only |
| **Pacing** | Requests/sec, duration, burst | `Rate` / `Duration` / `TraceCount` | `RateItemsPerSec` / burst / `TraceCount` as request budget |
| **Batching** | Items per export request | Implicit (spans in ResourceSpans after region split) | Explicit `ItemsPerRequest` |
| **Evidence tags** | `experiment.id`, `payload.id`, attr size/cardinality | Absent | Present |
| **Export policy** | Fail-fast vs continue | Fail-fast | `ContinueOnExportErr` |
| **Ledger** | attempted/acked/failed | `Stats` already recorded | `Stats` recorded |

**Root cause of “two modes”:** evidence knobs were implemented by replacing shape generation, instead of composing knobs onto the existing Run loop.

They are **orthogonal**. Incompatibility is coded, not inherent.

## Decision (proposed)

1. Treat `internal/gen` as one generator with composable controls:
   - **Shape** = topology | scenario | (optional) flat single-span helper
   - **WorkloadProfile** = pacing/batching/evidence tags/export policy overlays
2. Remove the product claim that dashboard and Capacity Run are different generator modes.
3. Keep Capacity Run free to **pin** shape defaults that preserve measurement purity (e.g. flat + single resource / no region fan-out) without forbidding composition.
4. Keep Assess as claim authority; generator output remains evidence/input, never validated capacity by itself.

## Consequences if accepted

### Must change

- Refactor `Generator.Run` so `Workload.Active()` does **not** early-return into a second world.
- Apply WorkloadProfile overlays on the shared path:
  - stamp evidence attrs on generated spans (or resource)
  - honor items/request as export batching unit where feasible
  - honor burst + continue-on-export-err
  - keep `Stats` as the single ledger
- Pin shape defaults at the caller when a measurement needs a flat single-endpoint run, not via a hard fork inside `gen.Run`.
- Update AGENTS/pages/handoff language that currently says WorkloadProfile “bypasses” topology/scenario as if that were desired product behavior.

### Must not change

- Do not revive `phase2/workload` as authority.
- Do not invent a third generator app.
- Do not treat dashboard runs as Assessed capacity.
- Do not silently invent collector_version / distribution / signal for Assess.

### UI implication (after unify)

Dashboard may expose WorkloadProfile knobs (or a subset) **without** a mode switch, because they no longer destroy topology/scenario.
Capacity Run remains the operator surface for pinned-collector experiments; it is not a second generator.

## Non-goals for the first unify PR

- Metrics/logs WorkloadProfile (still traces-first / fail closed for synthetic Capacity Run ingress)
- Making every topology run single-region
- Teaching Assess to consume raw dashboard jobs as fingerprints

## Acceptance tests (for the later implementation PR)

1. Topology/scenario run with empty WorkloadProfile: byte-stable shape behavior vs pre-unify (aside from intentional Stats fields already present).
2. Topology/scenario run with WorkloadProfile tags/attrs set: still emits topology/scenario span names/structure; spans carry `experiment.id` / `payload.id` when configured.
3. WorkloadProfile burst + continue-on-export-err still measurable via `Stats`.
4. Capacity Run smoke (`UI-SMOKE-traces` / harness) still completes with ledger integrity under pinned flat defaults.
5. Regression: activating any WorkloadProfile field must **not** drop the operator into an undocumented alternate span grammar unless shape was explicitly set to flat.

## Implementation status

- `Generator.Run` branches on **explicit** `WorkloadProfile.Shape == "flat"` only (Capacity Run pins this via `phase2/ingress`).
- Topology/scenario path composes `SingleResource`, `ItemsPerRequest` chunking, evidence attrs, continue-on-export-err, and optional workload pacing.
- Dashboard may expose WorkloadProfile knobs without a mode switch; they no longer destroy topology/scenario unless Shape is set to flat.

## Locked answers (2026-10-10 review)

1. **Batch unit = spans.** `ItemsPerRequest` is the number of spans in each `Export()` call. Matches current `runWorkloadProfile` / Capacity Run ledger semantics (`Stats` item counters = spans).
2. **Add explicit `single_resource` (name TBD) flag.** No-region-fanout is a first-class control, not an implied Capacity Run side effect of the early-return fork.
3. **Stamp evidence attrs on every span.** `experiment.id` / `payload.id` / controlled attrs follow current flat-path behavior.

## Locked answers (controller defaults)

4. **Field:** `WorkloadProfile.SingleResource bool` (`json:"single_resource,omitempty"`).
5. **Flat grammar is explicit:** `WorkloadProfile.Shape == "flat"` selects the flat `workload.span` helper. Empty Shape uses Config topology/scenario. WorkloadProfile knobs alone must not switch grammar.
6. **Chunking:** when a ResourceSpans has more spans than `ItemsPerRequest`, split into consecutive span chunks of that size and `Export` each chunk. Undersized final chunk is exported as-is (no pad, no reject).
