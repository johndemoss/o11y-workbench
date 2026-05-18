.DEFAULT_GOAL := help
.PHONY: build-pipeline-ui build-all test serve help otel-start otel-stop dev-remotetap clean-remotetap otel-demo-sender

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
	@echo "  make otel-start          Start OTel collector with remotetap processor"
	@echo "  make otel-stop           Stop OTel collector"
	@echo "  make dev-remotetap       Start collector, workbench, and demo sender"
	@echo "  make otel-demo-sender    Start CLI sending traces to collector for demo"
	@echo "  make clean-remotetap     Cleanup everything"

# Detect container runtime (docker or podman)
CONTAINER_RUNTIME := $(shell command -v podman > /dev/null 2>&1 && echo podman || echo docker)

# OTel collector config for remotetap
OTEL_COLLECTOR_CONFIG := /tmp/otel-collector-remotetap.yaml

# OTel collector YAML config (exported for recipe use)
define OTEL_COLLECTOR_YAML
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
processors:
  remotetap:
    endpoint: 0.0.0.0:12001
    limit: 10
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [remotetap]
      exporters: [debug]
    logs:
      receivers: [otlp]
      processors: [remotetap]
      exporters: [debug]
endef
export OTEL_COLLECTOR_YAML

# Start OTel collector with remotetap processor
.PHONY: otel-start
otel-start:
	@echo "Using container runtime: $(CONTAINER_RUNTIME)"
	@mkdir -p /tmp
	@echo "$$OTEL_COLLECTOR_YAML" > $(OTEL_COLLECTOR_CONFIG)
	$(CONTAINER_RUNTIME) run -d --name otel-collector-remotetap --replace \
		-p 4317:4317 -p 4318:4318 -p 12001:12001 \
		-v $(OTEL_COLLECTOR_CONFIG):/etc/otelcol-contrib/config.yaml:Z \
		docker.io/otel/opentelemetry-collector-contrib:latest
	@echo "OTel collector started"

# Stop OTel collector
.PHONY: otel-stop
otel-stop:
	$(CONTAINER_RUNTIME) stop otel-collector-remotetap 2>/dev/null || true
	$(CONTAINER_RUNTIME) rm otel-collector-remotetap 2>/dev/null || true
	rm -f $(OTEL_COLLECTOR_CONFIG)
	@echo "OTel collector stopped"

# Start everything for remotetap testing
.PHONY: dev-remotetap
dev-remotetap: otel-start serve otel-demo-sender

# Start CLI sending traces to collector for demo purposes
.PHONY: otel-demo-sender
otel-demo-sender:
	@echo "Starting o11y-workbench CLI to send traces to collector..."
	go run ./cmd/o11y-workbench --traces 0 --duration 300 --rate 5 --output otlp --endpoint localhost:4317 &
	@echo "CLI started in background"
	@echo "PID written to /tmp/o11y-workbench-demo.pid"
	@echo $$! > /tmp/o11y-workbench-demo.pid

# Cleanup everything
.PHONY: clean-remotetap
clean-remotetap:
	$(MAKE) otel-stop
	pkill -f o11y-workbench-web 2>/dev/null || true
	kill $$(cat /tmp/o11y-workbench-demo.pid 2>/dev/null) 2>/dev/null || true
	rm -f /tmp/o11y-workbench-demo.pid
