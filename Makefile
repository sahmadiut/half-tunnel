.PHONY: all build build-client build-server test lint clean docker help

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Go commands
GO := go
GOFMT := gofmt
GOLINT := golangci-lint

# Output directories
BIN_DIR := bin

# Default target
all: lint test build

# Build both client and server
build: build-client build-server

# Build client
build-client:
	@echo "Building client..."
	@mkdir -p $(BIN_DIR)
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/ht-client ./cmd/client

# Build server
build-server:
	@echo "Building server..."
	@mkdir -p $(BIN_DIR)
	$(GO) build $(LDFLAGS) -o $(BIN_DIR)/ht-server ./cmd/server

# Run tests
test:
	@echo "Running tests..."
	$(GO) test -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test
	@echo "Generating coverage report..."
	$(GO) tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	@echo "Running linter..."
	$(GOLINT) run ./...

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html

# Run go mod tidy
tidy:
	@echo "Tidying modules..."
	$(GO) mod tidy

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download

# Build Docker images
docker:
	@echo "Building Docker images..."
	docker build -t half-tunnel-client:$(VERSION) -f deployments/Dockerfile.client .
	docker build -t half-tunnel-server:$(VERSION) -f deployments/Dockerfile.server .

# Run client locally
run-client:
	$(GO) run ./cmd/client -config configs/config.example.yaml

# Run server locally
run-server:
	$(GO) run ./cmd/server -config configs/config.example.yaml

# Security scan
security:
	@echo "Running security scan..."
	govulncheck ./...

# Generate mocks (if needed)
generate:
	$(GO) generate ./...

# Help
help:
	@echo "Half-Tunnel Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  all           - Run lint, test, and build"
	@echo "  build         - Build both client and server"
	@echo "  build-client  - Build client binary"
	@echo "  build-server  - Build server binary"
	@echo "  test          - Run tests with race detection"
	@echo "  test-coverage - Run tests and generate coverage report"
	@echo "  lint          - Run golangci-lint"
	@echo "  fmt           - Format code"
	@echo "  clean         - Remove build artifacts"
	@echo "  tidy          - Run go mod tidy"
	@echo "  deps          - Download dependencies"
	@echo "  docker        - Build Docker images"
	@echo "  run-client    - Run client locally"
	@echo "  run-server    - Run server locally"
	@echo "  security      - Run security vulnerability scan"
	@echo "  help          - Show this help"
