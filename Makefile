.DEFAULT_GOAL := help
.PHONY: build-pipeline-ui build-all test serve serve-stop help otel-start otel-stop dev dev-remotetap clean clean-remotetap otel-demo-sender otel-demo-sender-stop build-images logs ps ottl-workers

.PHONY: ottl-workers
ottl-workers:
	cd workers/ottl-0.163 && go build -o ../../bin/ottl-worker-0.163.0 .
	cd workers/ottl-0.164 && go build -o ../../bin/ottl-worker-0.164.0 .

build-pipeline-ui:
	./scripts/build-ui.sh pipeline-ui

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
	@echo "Web server started at http://localhost:$(WEB_PORT)"
	@echo "Use 'make logs' to see logs"

# Stop the web server
.PHONY: serve-stop
serve-stop:
	@echo "Stopping o11y-workbench-web..."
	$(COMPOSE) -f docker-compose.yml stop o11y-workbench-web otel-collector 2>/dev/null || true
	$(COMPOSE) -f docker-compose.yml rm -f o11y-workbench-web otel-collector 2>/dev/null || true
	@echo "Web server stopped"

# Show available targets
help:
	@echo "Available targets:"
	@echo ""
	@echo "==> Containerized Workflow (Recommended):"
	@echo "  make build-images        Build all Docker images"
	@echo "  make dev                 Start otel-desktop-viewer + demo-sender (containerized, dev profile)"
	@echo "  make dev-remotetap       Start collector + web + remotetap demo-sender (no viewer)"
	@echo "  make serve               Start web server + collector only (containerized)"
	@echo "  make clean               Stop all containerized services"
	@echo "  make logs                Show logs from running containers"
	@echo "  make ps                  Show status of all containers"
	@echo ""
	@echo "==> Local Development:"
	@echo "  make build-pipeline-ui   Build the Svelte frontend locally"
	@echo "  make ottl-workers        Build extra-train OTTL worker binaries into bin/"
	@echo "  make build-all           Build everything locally (frontend + Go binary)"
	@echo "  make test                Run all tests"
	@echo ""
	@echo "==> Legacy/Binary Targets (use if not using containers):"
	@echo "  make otel-start          Start OTel collector with remotetap processor (podman/docker)"
	@echo "  make otel-stop           Stop OTel collector"
	@echo "  make otel-demo-sender    Start CLI binary sending traces"
	@echo "  make otel-demo-sender-stop Stop the demo sender binary"
	@echo "  make serve-stop          Stop the web server binary"
	@echo "  make clean-remotetap     Stop all services (both containerized and binary)"

# Host port for the web dashboard (container still listens on 8080).
# Override: make dev-remotetap WEB_PORT=9090
WEB_PORT ?= 18080
export WEB_PORT

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
COMPOSE := bash "$(CURDIR)/scripts/runtime.sh" compose $(COMPOSE)
endif

# OTel collector config for remotetap
OTEL_COLLECTOR_CONTRIB_VERSION ?= 0.165.0
OTEL_COLLECTOR_CONTRIB_DIGEST ?= sha256:e40d61f10c9a617cca1ffd1be4d7e2b3a4b0f0001798ff07e58cabe82e618cea
OTEL_COLLECTOR_IMAGE ?= docker.io/otel/opentelemetry-collector-contrib:$(OTEL_COLLECTOR_CONTRIB_VERSION)@$(OTEL_COLLECTOR_CONTRIB_DIGEST)
OTEL_COLLECTOR_CONFIG := config/otel-collector-remotetap.yaml

# Start OTel collector with remotetap processor
.PHONY: otel-start
otel-start:
	@echo "Using container runtime: $(CONTAINER_RUNTIME)"
	$(CONTAINER_RUNTIME) run -d --name otel-collector-remotetap --replace \
		-p 127.0.0.1:14317:14317 -p 127.0.0.1:14318:14318 -p 127.0.0.1:12001:12001 \
		--security-opt=no-new-privileges \
		--memory=512m --pids-limit=256 \
		-v $(CURDIR)/config/otel-collector-remotetap.yaml:/etc/otelcol-contrib/config.yaml:Z \
		$(OTEL_COLLECTOR_IMAGE)
	@echo "OTel collector started"

# Stop OTel collector
.PHONY: otel-stop
otel-stop:
	$(CONTAINER_RUNTIME) stop otel-collector-remotetap 2>/dev/null || true
	$(CONTAINER_RUNTIME) rm otel-collector-remotetap 2>/dev/null || true
	@echo "OTel collector stopped"

# Fixed container_name values collide across worktrees: compose project names
# differ by directory, but these containers share global names. Reclaim them
# before up / during clean so leftover stacks from another checkout cannot block.
NAMED_CONTAINERS := otel-collector-remotetap o11y-workbench-web demo-sender remotetap-demo-sender otel-desktop-viewer

.PHONY: reclaim-named-containers
reclaim-named-containers:
	@for c in $(NAMED_CONTAINERS); do \
		$(CONTAINER_RUNTIME) rm -f $$c >/dev/null 2>&1 || true; \
	done

# Optional OTLP demo: official otel-desktop-viewer container + demo-sender
.PHONY: dev
dev: reclaim-named-containers
	@echo "Starting otel-desktop-viewer + demo-sender (containerized)..."
	$(COMPOSE) -f docker-compose.yml --profile dev up -d
	@echo ""
	@echo "==> Services running:"
	@echo "    otel-desktop-viewer UI: http://localhost:8000 (container)"
	@echo "    OTLP endpoint:          localhost:4317 (gRPC) / localhost:4318 (HTTP)"
	@echo ""
	@echo "==> Demo sender running in container"
	@echo "    Command: o11y-workbench --traces 0 --duration 300 --rate 5 --output otlp --endpoint otel-desktop-viewer:4317 --insecure"
	@echo "    Use 'make clean' to stop everything"
	@echo "    Use 'make logs' to see container logs"

# Remote TAP stack: collector + dashboard + sender (no otel-desktop-viewer)
.PHONY: dev-remotetap
dev-remotetap: reclaim-named-containers
	@echo "Starting remotetap stack (containerized)..."
	@echo "remotetap-demo-sender: o11y-workbench --traces 0 --duration 0 --rate 2 --output otlp --endpoint otel-collector:14317 --insecure --scenario microservice"
	$(COMPOSE) -f docker-compose.yml -f docker-compose.dev-remotetap.yml --profile remotetap up -d
	@echo ""
	@echo "==> All services running:"
	@echo "    o11y-workbench dashboard:  http://localhost:$(WEB_PORT) (containerized)"
	@echo "    otel-collector:            localhost:14317 (gRPC) / localhost:14318 (HTTP)"
	@echo "    remotetap WebSocket:       localhost:12001"
	@echo ""
	@echo "==> remotetap-demo-sender running in container, sending traces to collector"
	@echo "    Command: o11y-workbench --traces 0 --duration 0 --rate 2 --output otlp --endpoint otel-collector:14317 --insecure --scenario microservice"
	@echo "    Use 'make clean' to stop everything"
	@echo "    Use 'make logs' to see container logs"

# Start CLI sending traces to otel-desktop-viewer for demo purposes
.PHONY: otel-demo-sender
otel-demo-sender:
	@echo "Starting o11y-workbench CLI to send traces to otel-desktop-viewer..."
	./scripts/demo-sender.sh start

# Stop the demo sender
.PHONY: otel-demo-sender-stop
otel-demo-sender-stop:
	@echo "Stopping demo sender..."
	$(COMPOSE) -f docker-compose.yml --profile dev stop demo-sender 2>/dev/null || true
	$(COMPOSE) -f docker-compose.yml --profile dev rm -f demo-sender 2>/dev/null || true
	./scripts/demo-sender.sh stop
	@echo "Demo sender stopped"

# Stop all containerized services
.PHONY: clean
clean:
	@echo "Stopping all services..."
	$(COMPOSE) -f docker-compose.yml down 2>/dev/null || true
	$(COMPOSE) -f docker-compose.yml --profile dev down 2>/dev/null || true
	$(COMPOSE) -f docker-compose.yml -f docker-compose.dev-remotetap.yml --profile remotetap down 2>/dev/null || true
	@$(MAKE) --no-print-directory reclaim-named-containers
	@$(MAKE) --no-print-directory otel-stop
	@echo "All services stopped"

# Cleanup everything (remotetap-specific, includes collector)
.PHONY: clean-remotetap
clean-remotetap: clean
	@echo "Cleanup complete"

# Build all Docker images (web + CLI; viewer pulls official image at runtime)
.PHONY: build-images
build-images:
	@echo "Building all Docker images..."
	$(COMPOSE) -f docker-compose.yml --profile remotetap --profile dev build --no-cache
	@echo "All images built successfully"

# Show logs from running containers
.PHONY: logs
logs:
	$(COMPOSE) -f docker-compose.yml -f docker-compose.dev-remotetap.yml --profile dev --profile remotetap logs -f

# Show status of all containers
.PHONY: ps
ps:
	$(COMPOSE) -f docker-compose.yml -f docker-compose.dev-remotetap.yml --profile dev --profile remotetap ps