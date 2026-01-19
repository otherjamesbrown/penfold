.PHONY: build run test lint clean tidy deps fmt proto

# Build variables
BINARY_NAME=ai-coordinator
BUILD_DIR=bin

# Default target
all: build

# Build the AI coordinator service
build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

# Run the service
run:
	go run .

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

# Run go vet
vet:
	go vet ./...

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

# Generate proto files
proto:
	cd ../../api/proto && buf generate --path ai/v1/ai.proto

# Docker build
docker-build:
	docker build -t $(BINARY_NAME):latest .

# Help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build & Run:"
	@echo "  build       - Build the AI coordinator binary"
	@echo "  run         - Run the AI coordinator service"
	@echo ""
	@echo "Testing:"
	@echo "  test        - Run tests"
	@echo "  test-cover  - Run tests with coverage"
	@echo "  lint        - Run linting"
	@echo "  vet         - Run go vet"
	@echo ""
	@echo "Utilities:"
	@echo "  clean       - Clean build artifacts"
	@echo "  tidy        - Tidy dependencies"
	@echo "  deps        - Download dependencies"
	@echo "  fmt         - Format code"
	@echo "  proto       - Regenerate proto files"
