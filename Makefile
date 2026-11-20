# Run `make` or `make help` to list the targets.
.DEFAULT_GOAL := help
SHELL := /bin/bash

GO_PACKAGES := ./...
INTEGRATION := -tags integration -p 1

## ---- Run ---------------------------------------------------------------

.PHONY: up
up: ## Build and start the whole stack (console on http://localhost:3000)
	docker compose up --build -d --wait
	@echo
	@echo "  Console        http://localhost:3000"
	@echo "  API docs       http://localhost:8080/docs"
	@echo "  Traces         http://localhost:16686"
	@echo "  Mock provider  http://localhost:8090/admin/stats"

.PHONY: down
down: ## Stop the stack and delete its data
	docker compose down -v --remove-orphans

.PHONY: logs
logs: ## Follow the API and worker logs
	docker compose logs -f api worker

.PHONY: demo
demo: ## Send a mixed burst of 1,000 notifications to the running stack
	./scripts/demo.sh

## ---- Test --------------------------------------------------------------

.PHONY: test
test: ## Run every test: unit, integration and end-to-end (needs Docker)
	go test $(INTEGRATION) -race -count=1 $(GO_PACKAGES)

.PHONY: test-unit
test-unit: ## Run the unit tests only (no Docker needed)
	go test -race -count=1 $(GO_PACKAGES)

.PHONY: test-integration
test-integration: ## Run the PostgreSQL, Redis and end-to-end tests (needs Docker)
	go test $(INTEGRATION) -race -count=1 ./internal/storage/... ./internal/ratelimit/... ./test/...

.PHONY: test-docker
test-docker: ## Run every test inside Docker, nothing else installed
	docker compose --profile test run --rm --build test

.PHONY: cover
cover: ## Run every test with coverage and open the HTML report
	mkdir -p coverage
	go test $(INTEGRATION) -count=1 -coverpkg=./internal/... -coverprofile=coverage/coverage.out $(GO_PACKAGES)
	go tool cover -func=coverage/coverage.out | tail -1
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "Report: coverage/coverage.html"

.PHONY: test-web
test-web: ## Type-check, lint and test the console
	cd web && npm ci && npm run typecheck && npm run lint && npm test

## ---- Code quality ------------------------------------------------------

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: fmt
fmt: ## Format the Go code
	golangci-lint fmt

.PHONY: generate
generate: ## Regenerate the console's API types from api/openapi.yaml
	cd web && npm run generate:api

## ---- Local development -------------------------------------------------

.PHONY: build
build: ## Build every binary into ./bin
	CGO_ENABLED=0 go build -trimpath -o bin/ ./cmd/...

.PHONY: deps
deps: ## Start only PostgreSQL, Redis, the mock provider and Jaeger
	docker compose up -d --wait postgres redis mockprovider jaeger

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*?## "} /^## ----/ {printf "\n%s\n", substr($$0, 4)} /^[a-z-]+:.*?## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
