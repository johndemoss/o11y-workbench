// Package main enables `go generate` support for the otel-workbench project.
//
// This file triggers the Svelte UI build before the Go build so that the
// embedded filesystem includes the latest frontend assets.
//
// Usage:
//
//	go generate ./...
//	go build ./cmd/otel-workbench-web/...
package main

//go:generate make build-pipeline-ui
