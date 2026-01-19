# Penfold Go Services Makefile

.PHONY: all build test lint vet proto clean help

# Go modules in the project
GO_MODULES := cmd/penf pkg api/proto services/ai services/content services/gateway services/gmail services/relationship services/review services/search services/worker

# Default target
all: lint vet build test

## Build targets

build: ## Build all Go services
	@echo "Building all Go modules..."
	@for mod in $(GO_MODULES); do \
		echo "Building $$mod..."; \
		(cd $$mod && go build ./...); \
	done

## Test targets

test: ## Run all tests
	@echo "Running tests for all Go modules..."
	@for mod in $(GO_MODULES); do \
		echo "Testing $$mod..."; \
		(cd $$mod && go test -v -race ./...); \
	done

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@for mod in $(GO_MODULES); do \
		echo "Testing $$mod with coverage..."; \
		(cd $$mod && go test -v -race -coverprofile=coverage.out ./...); \
	done

## Lint and vet targets

lint: ## Run golangci-lint on all modules
	@echo "Running golangci-lint..."
	@for mod in $(GO_MODULES); do \
		echo "Linting $$mod..."; \
		(cd $$mod && golangci-lint run --timeout=5m ./...); \
	done

vet: ## Run go vet on all modules
	@echo "Running go vet..."
	@for mod in $(GO_MODULES); do \
		echo "Vetting $$mod..."; \
		(cd $$mod && go vet ./...); \
	done

## Proto targets

proto: ## Generate protobuf code (placeholder)
	@echo "Generating protobuf code..."
	@if [ -f api/proto/buf.yaml ]; then \
		(cd api/proto && buf generate); \
	else \
		echo "No buf.yaml found, skipping proto generation"; \
	fi

proto-lint: ## Lint protobuf files
	@echo "Linting protobuf files..."
	@(cd api/proto && buf lint)

proto-breaking: ## Check for breaking changes in protos
	@echo "Checking for breaking changes..."
	@(cd api/proto && buf breaking --against ".git#branch=main,subdir=api/proto")

## Dependency management

deps: ## Download dependencies for all modules
	@echo "Downloading dependencies..."
	@for mod in $(GO_MODULES); do \
		echo "Downloading deps for $$mod..."; \
		(cd $$mod && go mod download); \
	done

tidy: ## Run go mod tidy for all modules
	@echo "Tidying modules..."
	@for mod in $(GO_MODULES); do \
		echo "Tidying $$mod..."; \
		(cd $$mod && go mod tidy); \
	done

## Clean targets

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@for mod in $(GO_MODULES); do \
		rm -f $$mod/coverage.out; \
	done

## Help

help: ## Show this help
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
