Source-only checkout: generated Pipeline Prototyper assets are intentionally not tracked.
Run ./scripts/build-ui.sh (or make build-pipeline-ui), then rebuild the Go web binary.
The tracked parent directory keeps go:embed valid without Node.js; the UI returns 503
with build instructions until dist/index.html and its assets have been generated.
