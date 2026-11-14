# Contributing

Read [AGENTS.md](AGENTS.md) for package boundaries and the linked coding rules.
The CLI is traces-only; multi-signal generation and Pipeline Evaluation are web
workflows. Do not add flags to documentation without checking the actual help.

## Toolchain and reproducible checks

Use Go **1.26.11** (the minimum in both modules), Node.js **22**, npm, Python 3 and
Git. Newer Go versions may require newer Staticcheck binaries. Install the tested
check tools outside the source tree:

```bash
GOTOOLCHAIN=go1.26.11 go install honnef.co/go/tools/cmd/staticcheck@v0.7.0
GOTOOLCHAIN=go1.26.11 go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
GOTOOLCHAIN=go1.26.11 go install github.com/zricethezav/gitleaks/v8@v8.24.3
export PATH="$(go env GOPATH)/bin:$PATH"

# Includes the nested OTTL worker module, formatting, tidy, vet, staticcheck,
# build, race tests and reachable-vulnerability checks.
GOTOOLCHAIN=go1.26.11 ./scripts/check-go.sh
./scripts/check-secrets.sh
./scripts/check-publication.sh

(cd pipeline-ui && npm ci && npm test && npm run build && npm audit --audit-level=moderate)
```

npm lockfiles are the sole JavaScript install authority (root tooling plus the
pipeline UI); the unused, stale pipeline Bun lock was removed. Use `npm ci` for builds, not
a dependency refresh. Some development dependencies
currently retain a low-severity upstream `deepmerge` advisory without a compatible
fix. The audit gate rejects moderate and higher findings; it is not a zero-risk
claim..

For intentional lockfile refreshes, npm 10.9.9 was observed to crash with
`Cannot read properties of null (reading 'edgesOut')`; npm 11.6.0 completed the
compatible update. Do not use `npm audit fix --force` to silently downgrade or
change major versions. Commit manifests and lockfiles together when needed.

## Local code graph (optional)

Graphify 0.9.108 was verified for local AST refreshes:

```bash
uv tool upgrade graphifyy     # existing uv-managed install; check graphify --version
GRAPHIFY_MAX_WORKERS=4 graphify update . --force
graphify explain routesForEndpoints --graph graphify-out/graph.json
```

`.graphifyignore` explicitly excludes credentials, local IDE or automation
session state,
compiled assets and test scaffolding. Graphify now merges it with `.gitignore`.
The generated graph/report/HTML stay local in ignored `graphify-out/`;
review them for sensitive content before sharing. `update` makes no LLM requests;
it preserves eligible prior semantic nodes but does **not** refresh documentation
semantics. Do not substitute `graphify extract` without `--code-only` or run
LLM labeling on private source without reviewing provider/data-sharing settings.
Historical semantic token counts can remain in the report after a local update;
they do not represent new model calls by the AST-only refresh.

## Running and exercising the app

```bash
./scripts/serve.sh             # builds and embeds both UIs plus OTTL workers
curl --fail http://127.0.0.1:18080/api/health
# Open http://127.0.0.1:18080/ in a browser.
kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null
```

The launcher replaces the instance named in its PID file; do not use it to start
an independent second server. Do not expose this unauthenticated workbench to
untrusted users; read [SECURITY.md](SECURITY.md).

`go test ./...` alone omits `workers/ottl-0.162`.

### Source-only versus distributable web builds

Compiled SPA assets are **not tracked**. A plain Go build from a fresh checkout
serves the dashboard/APIs, but `/pipeline/` returns an actionable
503 until the UI has been generated and the Go binary rebuilt. Missing static
assets return 404, never an HTML fallback. Do not distribute a source-only binary
as the complete web product.

```bash
# The same npm-ci/build/asset-validation/sync path used by Make and serve.sh:
go generate ./cmd/o11y-workbench-web
O11Y_REQUIRE_BUILT_UI=1 go test ./internal/server -run '^TestEmbeddedUI'
go build ./cmd/o11y-workbench-web
node --test scripts/check-ui-assets.test.mjs
```

`make build-pipeline-ui` builds the pipeline UI through `scripts/build-ui.sh`;
`make build-all` builds that UI and the web binary. Rebuilding must not modify
tracked files. `serve.sh --skip-build` explicitly trusts an already built
`/tmp/o11y-workbench-web` and does not verify freshness; use the normal launcher
after any source change. Docker validates the pipeline entry-point bundle before
embedding. CI checks source-only Go behavior and generated, actually embedded assets.

### Reproducible browser smoke tests

The root private npm manifest/lock pins Playwright; it is separate from the two
frontend manifests. Install the matching Chromium after the root `npm ci`:

```bash
npm ci
npm test                       # Node asset-validator regressions
npm audit --audit-level=moderate
npx playwright install chromium
# Stop any existing local daemon first: tests refuse to reuse port 18080.
kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null || true
npm run test:browser
```

On supported Ubuntu CI, `npx playwright install --with-deps chromium` also installs
OS browser libraries (requires apt privileges). On other hosts, use Playwright's
supported platform prerequisites; do not assume a browser download installs OS
libraries. Node, npm, Go, Make, Git and the OTTL worker build toolchain must be
available; the optional Phase2 collector is **not** needed for these smoke tests.

Playwright invokes only `./scripts/serve.sh --foreground`: locked frontend builds,
Go embedding and workers are rebuilt, then a fresh loopback server is health-checked.
The runner owns graceful SIGTERM/join, does not replace the operator's daemon PID
file and uses an isolated synthetic evaluation store. Existing servers are never
silently accepted. Both SPAs must execute their real embedded bundles; missing
static assets must return 404 rather than HTML. Reports, traces and screenshots
stay ignored/local. These tests complement, not replace, UI unit/Go race checks;
CI runs the full check workflow on every push and pull request.

## Changes and safety

- Keep changes focused, with regression tests that fail before a bug fix.
- Format Go with `gofmt`; handle/wrap errors; propagate cancellation and close
  owned resources. Avoid process-global mutable configuration in request paths.
- Use synthetic fixtures only. Never commit local `.env` files, credentials,
  screenshots of real telemetry, runtime evaluation stores or agent session state.
- Do not publish scanner match/context reports. A secret in history requires
  rotation and coordinated cleanup, not just a deletion commit.

## License

Project-owned contributions are licensed under the [MIT license](LICENSE).
Third-party dependencies and incorporated material retain their original terms;
do not remove attribution or assume this license relicenses their contents.
