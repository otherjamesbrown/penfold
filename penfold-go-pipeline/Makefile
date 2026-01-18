.PHONY: build build-worker run run-worker test lint clean tidy deps temporal-up temporal-down

# Build variables
BINARY_NAME=penfold-pipeline
WORKER_BINARY_NAME=penfold-worker
BUILD_DIR=bin

# Default target
all: build build-worker

# Build the pipeline (Redis-to-Temporal bridge)
build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/pipeline

# Build the Temporal worker
build-worker:
	go build -o $(BUILD_DIR)/$(WORKER_BINARY_NAME) ./cmd/worker

# Run the pipeline (Redis-to-Temporal bridge)
run:
	go run ./cmd/pipeline

# Run the Temporal worker
run-worker:
	go run ./cmd/worker

# Run both pipeline and worker (use in separate terminals)
run-all:
	@echo "Run these in separate terminals:"
	@echo "  make run        # Start the pipeline (Redis-to-Temporal bridge)"
	@echo "  make run-worker # Start the Temporal worker"

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-cover:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run linting
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Tidy dependencies
tidy:
	go mod tidy

# Download dependencies
deps:
	go mod download

# Format code
fmt:
	go fmt ./...

# Check code formatting
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Code not formatted, run 'make fmt'" && exit 1)

# Generate mocks (if needed)
generate:
	go generate ./...

# Docker build
docker-build:
	docker build -t penfold-go-pipeline:latest .

# Start MLX sidecar
sidecar:
	cd sidecar && uvicorn app:app --host 0.0.0.0 --port 8001

# Integration test
integration-test:
	go test -v -tags=integration ./...

# ============================================================================
# Temporal targets
# ============================================================================

# Start Temporal server (requires Docker)
temporal-up:
	docker-compose -f docker-compose.temporal.yml up -d
	@echo "Temporal server starting..."
	@echo "  Web UI: http://localhost:8088"
	@echo "  gRPC:   localhost:7233"

# Stop Temporal server
temporal-down:
	docker-compose -f docker-compose.temporal.yml down

# View Temporal logs
temporal-logs:
	docker-compose -f docker-compose.temporal.yml logs -f temporal

# Start Temporal admin tools (for tctl commands)
temporal-admin:
	docker-compose -f docker-compose.temporal.yml --profile tools up -d temporal-admin-tools

# Restart Temporal
temporal-restart: temporal-down temporal-up

# Help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build & Run:"
	@echo "  build          - Build the pipeline binary"
	@echo "  build-worker   - Build the Temporal worker binary"
	@echo "  run            - Run the pipeline (Redis-to-Temporal bridge)"
	@echo "  run-worker     - Run the Temporal worker"
	@echo "  run-all        - Show instructions for running both"
	@echo ""
	@echo "Testing:"
	@echo "  test           - Run tests"
	@echo "  test-cover     - Run tests with coverage"
	@echo "  integration-test - Run integration tests"
	@echo "  lint           - Run linting"
	@echo ""
	@echo "Temporal (requires Docker):"
	@echo "  temporal-up    - Start Temporal server (UI at http://localhost:8088)"
	@echo "  temporal-down  - Stop Temporal server"
	@echo "  temporal-logs  - View Temporal server logs"
	@echo "  temporal-admin - Start admin tools for tctl"
	@echo ""
	@echo "Utilities:"
	@echo "  clean          - Clean build artifacts"
	@echo "  tidy           - Tidy dependencies"
	@echo "  deps           - Download dependencies"
	@echo "  fmt            - Format code"
	@echo "  sidecar        - Start MLX sidecar service"
