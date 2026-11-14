package main

// Generate both SPAs before building a distributable web binary. A plain Go build
// remains usable for APIs/dashboard, but source-only SPA routes return an explicit
// build-required 503 instead of shipping stale or incomplete compiled bundles.
//
// Run from the repository root: go generate ./cmd/o11y-workbench-web
//go:generate ../../scripts/build-ui.sh
