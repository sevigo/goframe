.DEFAULT_GOAL := help
.PHONY: help lint lint-fix test test-race test-cover build-examples tidy clean pre-push

# Local tooling directory
BIN_DIR := ./bin
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
GOLANGCI_LINT_VERSION := v2.5.0

# Docker environment for testcontainers-go
DOCKER_ENV := DOCKER_HOST=$$(docker context inspect -f '{{.Endpoints.docker.Host}}' 2>/dev/null || echo "unix:///var/run/docker.sock") \
              TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE="/var/run/docker.sock"

help:
	@echo "GoFrame Makefile"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Available targets:"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint           - Run all linters"
	@echo "  lint-fix       - Run all linters and apply automatic fixes"
	@echo ""
	@echo "Testing:"
	@echo "  test           - Run all tests"
	@echo "  test-race      - Run tests with race detector enabled"
	@echo "  test-cover     - Run tests and generate coverage report"
	@echo ""
	@echo "Build & Dependencies:"
	@echo "  build-examples - Build all examples to ensure they compile"
	@echo "  tidy           - Tidy Go module dependencies in root and examples"
	@echo ""
	@echo "Other:"
	@echo "  clean          - Clean up build artifacts and caches"
	@echo "  pre-push       - Run pre-push checks (lint + test)"
	@echo "  help           - Show this help message"

test:
	@echo "Running tests..."
	@$(DOCKER_ENV) go test ./...

test-race:
	@echo "Running tests with race detector..."
	@$(DOCKER_ENV) go test -race ./...

test-cover:
	@echo "Running tests with coverage..."
	@$(DOCKER_ENV) go test -coverprofile=coverage.out ./...
	@echo "Opening coverage report in browser..."
	@go tool cover -html=coverage.out

lint: $(GOLANGCI_LINT)
	@echo "Running linter..."
	@$(GOLANGCI_LINT) run ./...

lint-fix: $(GOLANGCI_LINT)
	@echo "Running linter with auto-fix..."
	@$(GOLANGCI_LINT) run --fix ./...

$(GOLANGCI_LINT):
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@mkdir -p $(BIN_DIR)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(BIN_DIR) $(GOLANGCI_LINT_VERSION)

build-examples:
	@echo "Building all examples..."
	@for example in $$(find ./examples -mindepth 1 -maxdepth 1 -type d); do \
		echo "--> Building $$example"; \
		(cd $$example && go mod tidy && go build -o /dev/null) || exit 1; \
	done
	@echo "All examples built successfully"

tidy:
	@echo "Tidying go.mod files..."
	@go mod tidy
	@for example in $$(find ./examples -mindepth 1 -maxdepth 1 -type d); do \
		echo "--> Tidying $$example"; \
		(cd $$example && go mod tidy) || exit 1; \
	done

clean:
	@echo "Cleaning up..."
	@rm -f coverage.out
	@rm -rf $(BIN_DIR)
	@go clean -testcache
	@if [ -f $(GOLANGCI_LINT) ]; then $(GOLANGCI_LINT) cache clean; fi

pre-push: lint test
	@echo "✅ Pre-push checks passed!"