.DEFAULT_GOAL := help
.PHONY: build-pipeline-ui build-all test serve serve-stop help otel-start otel-stop otel-desktop-viewer-download otel-desktop-viewer-start otel-desktop-viewer-stop dev dev-remotetap clean clean-remotetap otel-demo-sender otel-demo-sender-stop build-images logs ps

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

# Start web server only (containerized)
serve:
	@echo "Starting o11y-workbench-web (containerized)..."
	$(COMPOSE) up -d o11y-workbench-web otel-collector
	@echo "Web server started at http://localhost:8080"
	@echo "Use 'make logs' to see logs"

# Stop the web server
.PHONY: serve-stop
serve-stop:
	@echo "Stopping o11y-workbench-web..."
	$(COMPOSE) -f docker-compose.yml stop o11y-workbench-web otel-collector 2>/dev/null || true
	$(COMPOSE) -f docker-compose.yml rm -f o11y-workbench-web otel-collector 2>/dev/null || true
	@rm -f /tmp/o11y-workbench-web.pid
	@echo "Web server stopped"

# Show available targets
help:
	@echo "Available targets:"
	@echo ""
	@echo "==> Containerized Workflow (Recommended):"
	@echo "  make build-images        Build all Docker images"
	@echo "  make dev                 Start otel-desktop-viewer + demo-sender (containerized)"
	@echo "  make dev-remotetap       Start full stack: collector + viewer + web + sender (containerized)"
	@echo "  make serve               Start web server + collector only (containerized)"
	@echo "  make clean               Stop all containerized services"
	@echo "  make logs                Show logs from running containers"
	@echo "  make ps                  Show status of all containers"
	@echo ""
	@echo "==> Local Development:"
	@echo "  make build-pipeline-ui   Build the Svelte frontend locally"
	@echo "  make build-all           Build everything locally (frontend + Go binary)"
	@echo "  make test                Run all tests"
	@echo ""
	@echo "==> Legacy/Binary Targets (use if not using containers):"
	@echo "  make otel-start          Start OTel collector with remotetap processor (podman/docker)"
	@echo "  make otel-stop           Stop OTel collector"
	@echo "  make otel-desktop-viewer-download Download pre-built binaries from GitHub Actions"
	@echo "  make otel-desktop-viewer-start Start otel-desktop-viewer binary (requires download first)"
	@echo "  make otel-desktop-viewer-stop  Stop otel-desktop-viewer binary"
	@echo "  make otel-demo-sender    Start CLI binary sending traces"
	@echo "  make otel-demo-sender-stop Stop the demo sender binary"
	@echo "  make serve-stop          Stop the web server binary"
	@echo "  make clean-remotetap     Stop all services (both containerized and binary)"

# Detect container runtime (docker or podman)
CONTAINER_RUNTIME := $(shell command -v podman > /dev/null 2>&1 && echo podman || echo docker)

# Detect compose command (prefer docker-compose, then podman-compose, fallback to 'docker compose')
COMPOSE := $(shell command -v docker-compose > /dev/null 2>&1 && echo "docker-compose" || (command -v podman-compose > /dev/null 2>&1 && echo "podman-compose" || echo "docker compose"))

# Set DOCKER_HOST for docker-compose compatibility with podman socket
# Auto-detect based on OS: macOS uses podman machine, Linux uses rootless socket
ifeq ($(shell uname -s),Darwin)
# macOS: Get socket from podman machine
PODMAN_SOCK := $(shell podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null)
ifneq ($(PODMAN_SOCK),)
export DOCKER_HOST := unix://$(PODMAN_SOCK)
endif
else
# Linux: Use standard rootless podman socket
PODMAN_SOCK := /run/user/$(shell id -u)/podman/podman.sock
ifneq ($(wildcard $(PODMAN_SOCK)),)
export DOCKER_HOST := unix://$(PODMAN_SOCK)
endif
endif

# Only disable BuildKit when using podman (for compatibility)
# Docker users keep BuildKit for faster parallel builds
ifeq ($(CONTAINER_RUNTIME),podman)
export DOCKER_BUILDKIT := 0
export COMPOSE_DOCKER_CLI_BUILD := 0
# Create empty docker config to prevent credential store prompts
# docker-compose reads ~/.docker/config.json which triggers osxkeychain auth
EMPTY_DOCKER_CONFIG := $(shell mkdir -p /tmp/docker-config-empty && echo '{}' > /tmp/docker-config-empty/config.json && echo /tmp/docker-config-empty)
export DOCKER_CONFIG := $(EMPTY_DOCKER_CONFIG)
endif

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

# otel-desktop-viewer configuration
# ODV_SOURCE := workspace/github.com/ctrlspice/otel-desktop-viewer  # Not used - builds from downloaded binaries
ODV_VERSION := latest
ODV_BINDIR := $(CURDIR)/bin
ODV_PLATFORM := $(shell go env GOOS)-$(shell go env GOARCH)
ODV_ARTIFACT_URL := https://github.com/johndemoss/o11y-workbench/actions/workflows/build-otel-desktop-viewer.yml

# Download otel-desktop-viewer artifacts from latest successful GitHub Actions run
.PHONY: otel-desktop-viewer-download
otel-desktop-viewer-download:
	@mkdir -p $(ODV_BINDIR)
	@echo "Downloading otel-desktop-viewer artifacts from GitHub Actions..."
	@RUN_ID=$$(gh run list --workflow=build-otel-desktop-viewer.yml --status=success --json databaseId --jq '.[0].databaseId') && \
	if [ -z "$$RUN_ID" ]; then \
		echo "No successful workflow run found. Trigger the workflow at $(ODV_ARTIFACT_URL)"; \
		exit 1; \
	fi && \
	echo "Downloading from run $$RUN_ID..." && \
	rm -rf $(ODV_BINDIR)/otel-desktop-viewer-tmp && \
	mkdir -p $(ODV_BINDIR)/otel-desktop-viewer-tmp && \
	cd $(ODV_BINDIR)/otel-desktop-viewer-tmp && \
	gh run download "$$RUN_ID" --pattern "otel-desktop-viewer-*" && \
	for dir in otel-desktop-viewer-*/; do \
		cp "$$dir"otel-desktop-viewer-* ../tmp-$$(basename $$dir) && rm -rf "$$dir"; \
	done && \
	cd .. && \
	rm -rf otel-desktop-viewer-tmp && \
	for f in tmp-otel-desktop-viewer-*; do mv "$$f" "$${f#tmp-}"; done && \
	ls -lh $(ODV_BINDIR)/otel-desktop-viewer-linux-amd64 $(ODV_BINDIR)/otel-desktop-viewer-darwin-*

# Current platform binary
ODV_CURRENT_BINARY := $(ODV_BINDIR)/otel-desktop-viewer-$(ODV_PLATFORM)

# Start otel-desktop-viewer (OTLP GUI viewer)
# Uses ports 4317/4318 (gRPC/HTTP) for OTLP and 8000 for the web UI
.PHONY: otel-desktop-viewer-start
otel-desktop-viewer-start: $(ODV_CURRENT_BINARY)
	@echo "Using otel-desktop-viewer $(ODV_VERSION) ($(ODV_PLATFORM))"
	@($(ODV_CURRENT_BINARY) --host 0.0.0.0 > /tmp/otel-desktop-viewer.log 2>&1 &); sleep 1; echo $$! > /tmp/otel-desktop-viewer.pid
	@echo "otel-desktop-viewer started (pid: $$(cat /tmp/otel-desktop-viewer.pid))"
	@echo "    UI:          http://localhost:8000"
	@echo "    OTLP gRPC:   localhost:4317"
	@echo "    OTLP HTTP:   localhost:4318"

# Stop otel-desktop-viewer
.PHONY: otel-desktop-viewer-stop
otel-desktop-viewer-stop:
	@bash scripts/stop-odv.sh

# Start everything for remotetap testing - HYBRID MODE
# Web server and collector in containers, viewer as binary
.PHONY: dev-remotetap
dev-remotetap: otel-desktop-viewer-start
	@echo "Starting full remotetap stack (hybrid)..."
	$(COMPOSE) -f docker-compose.hybrid.yml -f docker-compose.hybrid-remotetap.yml --profile remotetap up -d
	@echo ""
	@echo "==> All services running:"
	@echo "    o11y-workbench dashboard:  http://localhost:8080 (containerized)"
	@echo "    otel-desktop-viewer UI:    http://localhost:8000 (binary on host)"
	@echo "    otel-collector:            localhost:14317 (gRPC) / localhost:14318 (HTTP)"
	@echo "    remotetap WebSocket:       localhost:12001"
	@echo ""
	@echo "==> Demo sender running in container, sending traces to collector"
	@echo "    Use 'make clean' to stop everything"
	@echo "    Use 'make logs' to see container logs"

# Start CLI sending traces to otel-desktop-viewer for demo purposes
.PHONY: otel-demo-sender
otel-demo-sender:
	@echo "Starting o11y-workbench CLI to send traces to otel-desktop-viewer..."
	go run ./cmd/o11y-workbench --traces 0 --duration 300 --rate 5 --output otlp --endpoint localhost:4317 &
	@echo "CLI started in background"
	@echo "PID written to /tmp/o11y-workbench-demo.pid"
	@echo $$! > /tmp/o11y-workbench-demo.pid

# Start CLI + otel-desktop-viewer - HYBRID MODE (for macOS ARM64)
# Runs otel-desktop-viewer as binary on host, demo-sender in container
.PHONY: dev
dev: otel-desktop-viewer-start
	@echo "Starting demo sender (containerized)..."
	$(COMPOSE) -f docker-compose.hybrid.yml --profile dev up -d
	@echo ""
	@echo "==> Services running:"
	@echo "    otel-desktop-viewer UI: http://localhost:8000 (binary on host)"
	@echo "    OTLP endpoint:          localhost:4317 (gRPC) / localhost:4318 (HTTP)"
	@echo ""
	@echo "==> Demo sender running in container, sending traces to host"
	@echo "    Use 'make clean' to stop everything"
	@echo "    Use 'make logs' to see container logs"

# Stop the demo sender
.PHONY: otel-demo-sender-stop
otel-demo-sender-stop:
	@echo "Stopping demo sender..."
	$(COMPOSE) -f docker-compose.hybrid.yml --profile dev stop demo-sender 2>/dev/null || true
	$(COMPOSE) -f docker-compose.hybrid.yml --profile dev rm -f demo-sender 2>/dev/null || true
	kill $$(cat /tmp/o11y-workbench-demo.pid 2>/dev/null) 2>/dev/null || true
	rm -f /tmp/o11y-workbench-demo.pid
	@echo "Demo sender stopped"

# Stop all containerized services and binaries
.PHONY: clean
clean:
	@echo "Stopping all services..."
	$(COMPOSE) -f docker-compose.yml down 2>/dev/null || true
	$(COMPOSE) -f docker-compose.hybrid.yml --profile dev down 2>/dev/null || true
	$(COMPOSE) -f docker-compose.hybrid.yml --profile remotetap down 2>/dev/null || true
	$(MAKE) otel-desktop-viewer-stop
	@echo "All services stopped"

# Cleanup everything (remotetap-specific, includes collector)
.PHONY: clean-remotetap
clean-remotetap: clean
	@echo "Cleanup complete"

# Build all Docker images
.PHONY: build-images
build-images:
	@echo "Building all Docker images..."
	$(COMPOSE) -f docker-compose.hybrid.yml build --no-cache
	@echo "All images built successfully"

# Show logs from running containers
.PHONY: logs
logs:
	$(COMPOSE) logs -f

# Show status of all containers
.PHONY: ps
ps:
	$(COMPOSE) ps
