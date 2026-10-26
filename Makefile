# Developer shortcuts. CI runs the same steps.
GO ?= go
BIN ?= PodcastAIStudio

.PHONY: help fmt vet lint test ui remotion smoke build run check clean

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

fmt: ## Format Go code
	gofmt -w $$(find . -name '*.go' -not -path './remotion/*')

vet: ## go vet
	$(GO) vet ./...

lint: vet ## Static analysis
	$(GO) run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

test: ## Go tests with the race detector
	$(GO) test -race -count=1 ./...

ui: ## Front-end checks
	node scripts/check-ui-ids.mjs

remotion: ## Install and type-check the Remotion project
	cd remotion && npm ci --no-audit --no-fund && npm run typecheck

smoke: ## Remotion render smoke test
	cd remotion && npm run smoke

build: ## Build the executable for the current platform
	$(GO) build -trimpath -ldflags "-s -w -X main.version=dev -X main.commit=$$(git rev-parse --short HEAD)" -o $(BIN) ./cmd/studio

run: ## Run from source without a session token
	$(GO) run ./cmd/studio -remotion ./remotion -no-token -log-level debug

check: lint test ui remotion smoke ## Everything CI runs on Linux

clean: ## Remove build outputs
	rm -rf $(BIN) $(BIN).exe dist remotion/out remotion/public/jobs/clip-*
