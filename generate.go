//go:build ignore

// Package main enables `go generate` support for the o11y-workbench project.
//
// This file triggers the Svelte UI build before the Go build so that the
// embedded filesystem includes the latest frontend assets.
//
// Usage:
//
//	go generate ./...
//	go build ./cmd/o11y-workbench-web/...
package main

//go:generate make build-pipeline-ui
//go:generate make build-capacity-ui
