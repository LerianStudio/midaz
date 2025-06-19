# Demo Data Generator Makefile

.PHONY: build test clean fmt vet lint cross-compile integration-test e2e-test help

# Build variables
BINARY_NAME=demo-data
VERSION?=dev
COMMIT?=$(shell git rev-parse --short HEAD)
DATE?=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO_VERSION?=$(shell go version | cut -d' ' -f3)

# Build flags
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE} -X main.goVersion=${GO_VERSION}"

# Default target
all: build

# Build the application
build:
	@echo "Building ${BINARY_NAME}..."
	@go build ${LDFLAGS} -o bin/${BINARY_NAME} ./cmd/demo-data

# Build for production (optimized)
build-prod:
	@echo "Building ${BINARY_NAME} for production..."
	@CGO_ENABLED=0 go build -a -installsuffix cgo ${LDFLAGS} -o bin/${BINARY_NAME} ./cmd/demo-data

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html

# Run integration tests
integration-test:
	@echo "Running integration tests..."
	@go test -v -tags=integration ./tests/integration/...

# Run end-to-end tests
e2e-test:
	@echo "Running end-to-end tests..."
	@go test -v ./tests/integration/mt001/e2e_test.go

# Run cross-platform tests
cross-platform-test:
	@echo "Running cross-platform build tests..."
	@./tests/integration/mt001/cross_platform_test.sh

# Run performance benchmarks
benchmark:
	@echo "Running performance benchmarks..."
	@go test -v -bench=. -benchmem ./tests/integration/mt001/performance_test.go

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Run golangci-lint (if available)
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

# Security scan
security:
	@echo "Running security checks..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not installed, install with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"; \
	fi

# Vulnerability check
vuln-check:
	@echo "Running vulnerability check..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not installed, install with: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi

# Cross-compile for multiple platforms
cross-compile:
	@echo "Cross-compiling for multiple platforms..."
	@mkdir -p dist
	@GOOS=linux GOARCH=amd64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-linux-amd64 ./cmd/demo-data
	@GOOS=darwin GOARCH=amd64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-darwin-amd64 ./cmd/demo-data
	@GOOS=darwin GOARCH=arm64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-darwin-arm64 ./cmd/demo-data
	@GOOS=windows GOARCH=amd64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-windows-amd64.exe ./cmd/demo-data
	@echo "Cross-compilation complete! Binaries in dist/"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

# Install development tools
install-tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	@go install golang.org/x/vuln/cmd/govulncheck@latest

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/ dist/ coverage.out coverage.html

# Run all quality checks
quality: fmt vet lint security vuln-check test

# Run comprehensive test suite
test-all: test integration-test e2e-test benchmark

# Run MT-001 validation suite
validate-mt001: quality integration-test cross-platform-test
	@echo "MT-001 validation complete!"

# Development workflow
dev: deps quality build
	@echo "Development build complete!"

# Production workflow
prod: deps quality build-prod cross-compile
	@echo "Production build complete!"

# Help
help:
	@echo "Available targets:"
	@echo "  build              - Build the application"
	@echo "  build-prod         - Build optimized production binary"
	@echo "  test               - Run unit tests"
	@echo "  test-coverage      - Run tests with coverage report"
	@echo "  integration-test   - Run integration tests"
	@echo "  e2e-test           - Run end-to-end tests"
	@echo "  cross-platform-test- Run cross-platform build tests"
	@echo "  benchmark          - Run performance benchmarks"
	@echo "  test-all           - Run all test suites"
	@echo "  fmt                - Format code"
	@echo "  vet                - Run go vet"
	@echo "  lint               - Run golangci-lint"
	@echo "  security           - Run security scan with gosec"
	@echo "  vuln-check         - Run vulnerability check"
	@echo "  cross-compile      - Build for multiple platforms"
	@echo "  deps               - Download and tidy dependencies"
	@echo "  install-tools      - Install development tools"
	@echo "  clean              - Clean build artifacts"
	@echo "  quality            - Run all quality checks"
	@echo "  validate-mt001     - Run MT-001 validation suite"
	@echo "  dev                - Full development workflow"
	@echo "  prod               - Full production workflow"
	@echo "  help               - Show this help message"