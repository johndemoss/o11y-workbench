# o11y workbench

CLI `cmd/o11y-workbench` is traces-only batch generation. Web `cmd/o11y-workbench-web` is the workbench (logs/metrics, OTTL, pipeline, TAP). Shared engine `internal/gen`. Multi-signal coordinator: `executeRun` in `internal/server/run.go`. Do not invent a product vision or roadmap.

## Commands

```bash
./scripts/serve.sh                         # only approved web start (embeds pipeline-ui)
kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null
go test ./...
go test -run TestName ./internal/pkg
npx playwright test --config playwright.config.js
cd pipeline-ui && npm test
go run ./cmd/o11y-workbench --traces 3 --topology branching
go run ./cmd/o11y-workbench capacity assess \
  --config path/to/collector.yaml \
  --collector-version vX.Y.Z \
  --distribution <distro> \
  --signal traces
go run ./cmd/o11y-workbench capacity run --scenario path/to/scenario.yaml
make build-otelcol-phase2                  # pinned Capacity Run collector binary
```

`make serve` / `make test` / `make build-pipeline-ui` wrap the same. Go 1.26+. Dashboard: vanilla JS (no HTMX). Pipeline UI: Svelte 5.

## Read when needed

Do not load these unless the task matches. In chat, attach with `@path`.


| When                                      | File                                           |
| ----------------------------------------- | ---------------------------------------------- |
| Changing Go, API, OTTL, or TS types       | `docs/agents/coding.md`                        |
| Serving, Docker, pipeline-ui / capacity-ui rebuild | `docs/agents/gotchas.md`                 |
| Naming UI in prompts                      | `docs/agents/pages.md`                         |
| Capacity Assess product rules             | `docs/capacity-plan-vs-assess.md`              |
| Capacity Run handoff                      | `docs/capacity-run-handoff.md` (vault canonical) |
| Generator unification (shape vs evidence knobs) | `docs/generator-unification-adr.md` |
| Bumping OTel collector / OTTL / image pins | `docs/otel-version-support.md`                 |
| Adding/checking HTTP routes               | `internal/server/server.go` (`registerRoutes`) |
| Generator ADR / no strategy pattern       | `internal/gen/generator.go`                    |
| Human overview                            | `README.md`                                    |




## Always

- JSON APIs, `net/http` Go 1.22+ routes (`GET /api/runs/{id}`), wrap errors with `%w`
- Go structs win over `pipeline-ui/src/lib/types.ts`
- Handlers call `store.StartRun()`; do not construct generators in handlers
- Two surfaces, one product — not two competing apps
- Assess is capacity authority; Experimental Plan is heuristic only
- Never treat queue→MiB / Plan results as validated capacity
- Capacity Run measures via `internal/gen` (+ Remote TAP for capture); do not use `phase2/workload`



## Learned Workspace Facts

- `cmd/o11y-workbench` generates traces only; logs, metrics, OTTL, pipeline, and Remote TAP live in the web binary via `executeRun`. Both share `internal/gen`.
- README CLI flags `--signal-type`, `--log-count`, and `--metric-count` are not in `cmd/o11y-workbench/main.go`.

## Capacity Assess (authoritative)

- Default capacity surface is **Assess** (`POST /api/capacity/assess`, `o11y-workbench capacity assess`, `/capacity/` Assess tab).
- Requires explicit `collector_version`, `distribution`, `signal`; fail closed on `INSUFFICIENT_EVIDENCE` / `ABSTAIN` / `REJECTED`.
- Use Phase 3 evaluator only (`internal/capacity/assess` + `internal/capacity/phase3`); never invent queue→heap formulas or silently invent fingerprint fields.
- Analytical **Plan** is experimental/demoted behind **Experimental Plan** — do not treat queue→MiB heuristics as validated capacity; never silently merge Plan into Assess.
- UI embed: `//go:embed capacity-ui/dist` — after capacity-ui edits run `make build-capacity-ui` or `./scripts/serve.sh`; compose/`dev-remotetap` needs an image rebuild (see `docs/agents/gotchas.md`).
- See `docs/capacity-plan-vs-assess.md` and `docs/collector-benchmark-integration-assessment.md`.

## Capacity Run (measurement)

- Product surface: `o11y-workbench capacity run`, `POST /api/capacity/run`, `GET /api/capacity/scenarios`, `/capacity/` **Capacity Run** tab.
- Library: `internal/capacity/bench` over Phase 2 harness (`internal/capacity/phase2/**`).
- Synthetic ingress: `internal/gen` via `phase2/ingress` (`WorkloadProfile` / `Stats`). Remote TAP stays captured ingress. `phase2/workload` is quarantined — not product authority.
- Pinned collector binary via `make build-otelcol-phase2` → `internal/capacity/phase2/bin/` (gitignored); fail closed if missing.
- Run output is evidence for Assess — not validated capacity. Traces-only synthetic ingress until `WorkloadProfile` grows.
- One generator: `internal/gen`. Shape (topology/scenario) and WorkloadProfile overlays are orthogonal — see `docs/generator-unification-adr.md`.
- Flat evidence grammar is explicit: `WorkloadProfile.Shape == "flat"` (Capacity Run pins this). Other WorkloadProfile knobs compose onto topology/scenario.
- Handoff: `docs/capacity-run-handoff.md` → vault `o11y-workbench-capacity-run-handoff.md`.
