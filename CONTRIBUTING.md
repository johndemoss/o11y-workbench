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

(cd pipeline-ui && npm ci && npm test && npm run build && npm audit --audit-level=moderate)
```

Use `npm ci` for builds, not a dependency refresh. Some development dependencies
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

`go test ./...` alone omits `workers/ottl-0.162`. A Go build alone also does not
rebuild the embedded SPAs. For Pipeline Evaluation execution, separately build the
pinned collector with `make build-otelcol-phase2`; without it, Prepare must fail
closed. Capacity measurement integration tests may exercise that local collector
when installed and can take several minutes.

Browser tests use `npx playwright test --config playwright.config.js` after
installing the matching Playwright browser/runtime dependencies. They are
additional evidence, not replaced by UI unit tests or the Go race suite.

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
