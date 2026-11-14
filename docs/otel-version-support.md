# OpenTelemetry Collector version support

The workbench supports the **latest three Collector minor trains** for OTTL validation, transform/filter execution, and pipeline prototyping. `collector_version` uses the bare Collector Contrib train (`0.N.0`), not the `pkg/ottl` Go module version. The Pipeline Prototyper loads available engines from `GET /api/collector-versions`; only successfully started workers appear in that response. The web app is not a replacement Collector distribution.

| Collector Contrib train | Core stable | Workbench engine | `pkg/ottl` module |
|---|---|---|---|
| `0.165.0` (default) | `1.71.0` | in-process | `v1.4.1` |
| `0.164.0` | `1.70.0` | `workers/ottl-0.164` | `v1.3.1` |
| `0.163.0` | `1.69.0` | `workers/ottl-0.163` | `v0.163.0` |

This window follows the [core releases](https://github.com/open-telemetry/opentelemetry-collector/releases/tag/v0.165.0) and their [0.164.0](https://github.com/open-telemetry/opentelemetry-collector/releases/tag/v0.164.0) and [0.163.0](https://github.com/open-telemetry/opentelemetry-collector/releases/tag/v0.163.0) predecessors. Contrib published [0.165.1](https://github.com/open-telemetry/opentelemetry-collector-contrib/releases/tag/v0.165.1) and [0.164.1](https://github.com/open-telemetry/opentelemetry-collector-contrib/releases/tag/v0.164.1) patch releases without corresponding core patch releases. OTTL became a v1 module after `0.163`; its published tags are `v1.4.1` and `v1.3.1`, **not** `v0.165.1` or `v0.164.1`. Keep the UI/API train names at `0.165.0`/`0.164.0` and the stable labels at `1.71.0`/`1.70.0`; do not derive a nonexistent core `.1` release from a contrib-only patch.

The default engine is compiled into the web binary from root `go.mod` and `internal/ottlengine/engine.go`. Older trains live in independent worker Go modules, supervised by `internal/ottlengine`. Worker executables must be present and healthy to advertise their train. Version-sensitive OTTL routes resolve an engine through `internal/ottlengine`; export, diff, and graph helpers are version-independent. The optional remotetap Collector sidecar is pinned to the **latest** Contrib image in the compose files and Makefile; it does not start three separate Collector daemons.

The workbench enables the upstream alpha `ottl.functions.enableLambda` gate in
each of its three OTTL engines and registers experimental `xottl` converters.
For example, a transform statement can use
`set(log.attributes["prod_tags"], Filter(log.attributes["tags"], (_, v) => v == "prod"))`.
Select a train in `/pipeline/` to validate and execute the statement against
sample telemetry. These functions are experimental and may change between
trains; the workbench does **not** enable the gate on an external Collector or
on its optional remotetap sidecar. Deployments must explicitly enable the
matching gate and functions in their own Collector. See the upstream
[lambda syntax and gate](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.165.0/pkg/ottl/LANGUAGE.md#lambda-expressions).

## Build and verify

- `versions/otel.env` pins the default train and the comma-separated worker trains. `make ottl-workers` builds the two isolated modules; `./scripts/serve.sh` builds the web UI, web binary, and workers and starts the server. `Dockerfile.web` builds and ships the same two worker binaries.
- `GET http://127.0.0.1:18080/api/collector-versions` should report default `0.165.0` and exactly three available trains in a complete local/container build. A missing worker does **not** become a substitute engine; fix its build/startup instead.
- Test the root module with `go test ./...` and each worker module with `go test ./...` in its directory. The browser picker must offer all three; validate and execute trace, log, and metric pipelines with each selected version. The repository's `./scripts/check-go.sh` and browser tests cover these build paths.

## Advancing the window

1. Check the [core release list](https://github.com/open-telemetry/opentelemetry-collector/releases) and [Contrib release list](https://github.com/open-telemetry/opentelemetry-collector-contrib/releases) for the newest aligned minor train and any contrib-only patch. Confirm the actual `pkg/ottl` module tag with `go list -m -versions github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl` rather than assuming it matches the Contrib train number.
2. Upgrade root OTTL/core module pins and `internal/ottlengine/engine.go` for the new default. Move the previous default into an isolated worker module; retain the other previous worker and retire the oldest. Update `versions/otel.env`, Makefile, `Dockerfile.web`, `scripts/check-go.sh`, CI Go sum caches, and the serve/docs version references together.
3. Update the remotetap image tag **and its verified OCI digest** in both compose files and the Makefile. Check the mounted Collector config against the new image.
4. Exercise validation, transform/filter, import, and per-signal pipeline execution on all three engines; verify worker startup and shutdown, the version API, and the Pipeline Prototyper picker. Do not claim a version supported merely because its tag appears in a config file.
