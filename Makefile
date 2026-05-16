.PHONY: build-pipeline-ui build-all test

# Build the Svelte pipeline UI and copy to server embed location
build-pipeline-ui:
	cd pipeline-ui && npm install && npm run build
	cp -r pipeline-ui/dist/* internal/server/pipeline-ui/dist/

# Build everything
build-all: build-pipeline-ui
	go build ./cmd/otel-workbench-web/...

# Run tests
test:
	go test ./...
