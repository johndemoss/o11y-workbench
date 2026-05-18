.DEFAULT_GOAL := help
.PHONY: build-pipeline-ui build-all test serve serve-stop help otel-start otel-stop otel-desktop-viewer-start otel-desktop-viewer-stop dev dev-remotetap clean clean-remotetap otel-demo-sender otel-demo-sender-stop

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

# Stop the web server
.PHONY: serve-stop
serve-stop:
	@echo "Stopping o11y-workbench-web..."
	pkill -f o11y-workbench-web 2>/dev/null || true
	kill $$(cat /tmp/o11y-workbench-web.pid 2>/dev/null) 2>/dev/null || true
	rm -f /tmp/o11y-workbench-web.pid
	@echo "Web server stopped"

# Show available targets
help:
	@echo "Available targets:"
	@echo "  make build-pipeline-ui   Build the Svelte frontend"
	@echo "  make build-all           Build everything (frontend + Go binary)"
	@echo "  make serve               Build + start web server (agent-safe, polls health endpoint)"
	@echo "  make serve-stop          Stop the web server"
	@echo "  make test                Run all tests"
	@echo "  make otel-start          Start OTel collector with remotetap processor"
	@echo "  make otel-stop           Stop OTel collector"
	@echo "  make dev                 Start CLI + otel-desktop-viewer only (no web server)"
	@echo "  make dev-remotetap       Start collector, workbench, and demo sender"
	@echo "  make otel-demo-sender    Start CLI sending traces to otel-desktop-viewer"
	@echo "  make otel-demo-sender-stop Stop the demo sender"
	@echo "  make otel-desktop-viewer-start Start otel-desktop-viewer (OTLP GUI)"
	@echo "  make clean               Stop all services (dev or dev-remotetap)"
	@echo "  make clean-remotetap     Stop all remotetap services + collector"

# Detect container runtime (docker or podman)
CONTAINER_RUNTIME := $(shell command -v podman > /dev/null 2>&1 && echo podman || echo docker)

# OTel collector config for remotetap
OTEL_COLLECTOR_CONFIG := config/otel-collector-remotetap.yaml

# Start OTel collector with remotetap processor
.PHONY: otel-start
otel-start:
	@echo "Using container runtime: $(CONTAINER_RUNTIME)"
	$(CONTAINER_RUNTIME) run -d --name otel-collector-remotetap --replace \
		-p 14317:14317 -p 14318:14318 -p 12001:12001 \
		-v $(CURDIR)/config/otel-collector-remotetap.yaml:/etc/otelcol-contrib/config.yaml:Z \
		docker.io/otel/opentelemetry-collector-contrib:latest
	@echo "OTel collector started"

# Stop OTel collector
.PHONY: otel-stop
otel-stop:
	$(CONTAINER_RUNTIME) stop otel-collector-remotetap 2>/dev/null || true
	$(CONTAINER_RUNTIME) rm otel-collector-remotetap 2>/dev/null || true
	@echo "OTel collector stopped"

# Build otel-desktop-viewer from local source (has logs + metrics support, newer than latest release)
ODV_SOURCE := workspace/github.com/ctrlspice/otel-desktop-viewer
ODV_VERSION := $(shell git -C $(ODV_SOURCE) describe --tags --always)
ODV_BINARY := $(shell go env GOPATH)/bin/otel-desktop-viewer

$(ODV_BINARY): $(ODV_SOURCE)
	cd $(ODV_SOURCE) && go build -ldflags="-s -w -X main.version=$(ODV_VERSION)" -o $(ODV_BINARY)

# Start otel-desktop-viewer (OTLP GUI viewer)
# Uses ports 4317/4318 (gRPC/HTTP) for OTLP and 8000 for the web UI
.PHONY: otel-desktop-viewer-start
otel-desktop-viewer-start: $(ODV_BINARY)
	@echo "Using otel-desktop-viewer $(ODV_VERSION) (from $(ODV_SOURCE))"
	$(ODV_BINARY) --host 0.0.0.0 &
	@echo "otel-desktop-viewer started"
	@echo "    UI:          http://localhost:8000"
	@echo "    OTLP gRPC:   localhost:4317"
	@echo "    OTLP HTTP:   localhost:4318"

# Stop otel-desktop-viewer
.PHONY: otel-desktop-viewer-stop
otel-desktop-viewer-stop:
	pkill -f otel-desktop-viewer 2>/dev/null || true
	@echo "otel-desktop-viewer stopped"

# Start everything for remotetap testing
.PHONY: dev-remotetap
dev-remotetap: otel-start otel-desktop-viewer-start serve otel-demo-sender
	@echo ""
	@echo "==> All services running:"
	@echo "    o11y-workbench dashboard:  http://localhost:8080"
	@echo "    otel-desktop-viewer UI:    http://localhost:8000"
	@echo "    otel-collector:            localhost:14317 (gRPC) / localhost:14318 (HTTP)"
	@echo "    remotetap WebSocket:       localhost:12001"
	@echo ""
	@echo "==> Demo sender running, sending traces to localhost:14317"
	@echo "    Use 'make clean-remotetap' to stop everything"

# Start CLI sending traces to otel-desktop-viewer for demo purposes
.PHONY: otel-demo-sender
otel-demo-sender:
	@echo "Starting o11y-workbench CLI to send traces to otel-desktop-viewer..."
	go run ./cmd/o11y-workbench --traces 0 --duration 300 --rate 5 --output otlp --endpoint localhost:4317 &
	@echo "CLI started in background"
	@echo "PID written to /tmp/o11y-workbench-demo.pid"
	@echo $$! > /tmp/o11y-workbench-demo.pid

# Start CLI + otel-desktop-viewer only (no web server, no collector)
.PHONY: dev
dev: otel-desktop-viewer-start otel-demo-sender
	@echo ""
	@echo "==> Services running:"
	@echo "    otel-desktop-viewer UI: http://localhost:8000"
	@echo "    OTLP endpoint:        localhost:4317 (gRPC) / localhost:4318 (HTTP)"
	@echo ""
	@echo "==> Demo sender running, sending traces to localhost:4317"
	@echo "    Use 'make clean' to stop everything"

# Stop the demo sender
.PHONY: otel-demo-sender-stop
otel-demo-sender-stop:
	@echo "Stopping demo sender..."
	kill $$(cat /tmp/o11y-workbench-demo.pid 2>/dev/null) 2>/dev/null || true
	rm -f /tmp/o11y-workbench-demo.pid
	@echo "Demo sender stopped"

# Stop all services (works for both 'make dev' and 'make dev-remotetap')
.PHONY: clean
clean:
	$(MAKE) otel-desktop-viewer-stop
	$(MAKE) otel-demo-sender-stop
	$(MAKE) serve-stop
	$(MAKE) otel-stop
	rm -f /tmp/o11y-workbench-web.pid /tmp/o11y-workbench-demo.pid
	@echo "All services stopped"

# Cleanup everything (remotetap-specific, includes collector)
.PHONY: clean-remotetap
clean-remotetap: clean
	@echo "Cleanup complete"
