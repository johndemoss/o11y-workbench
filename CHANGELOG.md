# Changelog

Notable changes to the o11y workbench. The project is developed on `main`; notes
here describe user-visible behavior rather than internal work.

## Unreleased

### Security

- The web server rejects requests whose `Host` header is not `localhost`, a
  loopback address, the address it was started with, or an explicit
  `--allowed-hosts` entry, so a page on another origin cannot reach the local
  API through DNS rebinding.
- Every response now carries a restricting `Content-Security-Policy` plus
  `X-Content-Type-Options`, `Referrer-Policy`, and `X-Frame-Options`. The
  dashboard no longer relies on inline event handlers or inline scripts.
- Dashboard fields that come from scenario names, run failures, and telemetry
  (log severity, log and metric attributes) are rendered as text instead of
  markup.
- Local launchers keep their built binaries, PID records, and locks in a private
  per-checkout `.run/` directory instead of predictable paths under the system
  temporary directory, verify process identity before stopping a recorded
  process, and refuse to signal a process they cannot verify.
- Updated `golang.org/x/net` to v0.62.0 in both OTTL worker modules
  ([GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860),
  [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866)).

### Added

- `--allowed-hosts` on the web binary and `./scripts/serve.sh` for additional
  exact hostnames or IP literals.
- `./scripts/serve.sh --stop`, which safely stops the daemon started from this
  checkout.
- `./scripts/demo-sender.sh start|stop` for the demo telemetry sender, with
  output kept in the private runtime directory.
- Weekly Dependabot updates for the Go modules, both npm manifests, and GitHub
  Actions, plus issue and pull-request templates, `CODEOWNERS`, and a code of
  conduct.

### Changed

- Updated the Pipeline Prototyper's Vite and Svelte build tooling.

### Fixed

- Native pipelines with configured receivers validate and execute as authored
  instead of being rejected as incomplete Collector configs; structured
  transforms honor their declared `error_mode`, and invalid group conditions
  fail validation on every supported Collector train.
- Pipeline Prototyper results stay honest when the selected run, signal, or
  execution surface changes, and Dashboard job cards show retained log and
  metric record counts, including zero.
- Statement and condition parse errors stay fatal regardless of `error_mode`,
  while evaluation errors keep honoring it; a run whose configured duration
  expires during an in-flight export completes cleanly instead of failing.
- Group conditions validate in their declared context (span, scope, resource)
  on every supported Collector train, and repeated pauses require a live
  acknowledged barrier. Remote TAP job cards report streaming progress and
  retained record counts.
- Correlated logs keep streaming throughout trace generation; finite runs finish
  their logs before generating metrics, and signal failures stop sibling work.
- Pause acknowledgement waits for in-flight exports, concurrent controls preserve
  the export barrier, and paused runs finish cleanly when their duration expires.
  Pausing also preserves trace partial-success accounting.

- Metrics filter validation recognizes datapoint conditions across all supported
  Collector versions, including configurations with `error_mode`.
- Trace filter stages report zero output spans when every span is removed.
- Party mode positions o11y's hat on the crown instead of over the forehead.
- Pipeline Prototyper stage additions and drag-and-drop target the correct signal
  lane and preserve existing OTTL edits. Graph controls remain accessible with the
  YAML and results panels open.
- Favorite transforms honor the selected lane for Add and preview, and the
  destination lane for drag-and-drop, with native log and metric contexts.
  Span-only filters are clearly unavailable outside traces. Pipeline-tab editors
  also preserve their selected signal.
- Template previews stay within the viewport, scroll long configurations internally,
  and keep their actions visible on narrow and short screens.
- File output no longer overwrites earlier batches of the same trace: each export
  batch is written to its own `spans-<unique>.{json,pb}` file under
  `<output-dir>/<traceID>/<endpoint>/`.
- Collector partial-success responses are reported through endpoint routing, so
  rejected items are no longer counted as accepted.
- OTLP JSON uses hexadecimal trace and span IDs for spans, links, log records and
  metric exemplars, and conversion no longer rounds large numeric values.
- The CLI bounds each OTLP export to 10 seconds, treats Ctrl-C as a clean stop,
  and always shuts its exporter down.
- Pipeline graph deletes and reorders keep stages and endpoints connected, stage
  cards can be dragged within their signal lane, graph update failures are shown
  and rolled back, and lanes with several receivers or exporters lay them out
  without overlap.
- Pipeline results compare logs and metrics with signal-aware diffs instead of
  span-only diffs, ignore stale responses, and show when a run's captured data
  was truncated.
- Corrected README scenario-file and config-file examples, the Vite proxy
  guidance, and the container filesystem hardening description.
- Pipeline filters apply Collector drop semantics across traces, logs, and
  metrics, and transform execution preserves context, statement groups, and
  group conditions on every supported Collector train.
- OTTL error modes apply per record, all metric datapoint types are transformed
  and filtered, and worker validation uses each signal's own context.
- OTTL execution honors request cancellation, and long-running jobs retain a
  bounded capture with visible truncation counts.
- Dashboard polling runs once per second only while jobs are active and backs off
  to a periodic recheck when idle, so external jobs still appear without constant
  list requests.
- Correlated logs stream from each generated trace while traces keep running, so
  unlimited trace runs no longer starve or defer log and metric generation.
  Cancelling or failing one signal does not discard the others' captured data.
- Standalone transform requests with structured processor configs execute through
  pipeline semantics, so contexts, statement groups, and group conditions match
  the Pipeline Prototyper and Collector behavior.
