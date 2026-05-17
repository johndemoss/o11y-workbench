.DEFAULT_GOAL := help
.PHONY: build-pipeline-ui build-all test serve help

build-pipeline-ui:
	cd pipeline-ui && npm install && npm run build
	rm -rf internal/server/pipeline-ui/dist
	cp -r pipeline-ui/dist internal/server/pipeline-ui/dist

# Build everything
build-all: build-pipeline-ui
	go build ./cmd/o11y-workbench-web/...

# Run tests
test:
	go test ./...

# Build and start the web server, waiting for it to be ready.
# Designed for agent/automation use - exits only when the server is accepting connections.
# The PID is written to /tmp/o11y-workbench-web.pid for cleanup.
serve:
	./scripts/serve.sh

# Show available targets
help:
	@echo "Available targets:"
	@echo "  make build-pipeline-ui   Build the Svelte frontend"
	@echo "  make build-all           Build everything (frontend + Go binary)"
	@echo "  make serve               Build + start web server (agent-safe, polls health endpoint)"
	@echo "  make test                Run all tests"
