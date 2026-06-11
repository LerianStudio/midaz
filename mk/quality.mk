# ------------------------------------------------------
# Code quality and maintenance commands
# ------------------------------------------------------
# This file contains all code quality operations including
# linting, formatting, code generation, and dependency management.
#
# Requirements:
#   - golangci-lint v2 (run via `go run` at the pinned version; never a
#     preinstalled binary, so the gate stays deterministic)
#   - mockgen (auto-installed if missing)
#   - Go toolchain (go fmt, go generate, go mod)
#
# Variables used from the including Makefile:
#   - GOLANGCI_LINT_VERSION: pinned golangci-lint version
#
# Usage:
#   make lint                        # Run linters (read-only gate)
#   make lint-fix                    # Apply lint autofixes (mutates source)
#   make format                      # Format Go code
#   make generate                    # Generate mocks and code
#   make tidy                        # Clean unused dependencies
# ------------------------------------------------------

#-------------------------------------------------------
# Commands (alphabetically ordered)
#-------------------------------------------------------

# Format Go code with go fmt
.PHONY: format
format:
	$(call title1,"Formatting code")
	@go fmt ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Formatting completed successfully$(GREEN) ✔️$(NC)"

# Generate code (mocks, etc.)
.PHONY: generate
generate:
	$(call title1,"Generating code (mocks, etc.)")
	@if ! command -v mockgen >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing mockgen...$(NC)"; \
		go install go.uber.org/mock/mockgen@v0.6.0; \
	fi
	@export PATH="$$(go env GOPATH)/bin:$$PATH"; go generate ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Code generation completed successfully$(GREEN) ✔️$(NC)"

# Run golangci-lint read-only (the gate; never mutates source)
# Pins the version via `go run @$(GOLANGCI_LINT_VERSION)` so a preinstalled
# binary of a different version cannot make the gate non-deterministic.
.PHONY: lint
lint:
	$(call title1,"Running linters")
	@if find . -name "*.go" -type f | grep -q .; then \
		go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./... --verbose && \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Linting completed successfully$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping linting$(NC)"; \
	fi

# Apply lint autofixes — MUTATES source. Developer convenience, NOT a gate.
.PHONY: lint-fix
lint-fix:
	$(call title1,"Applying lint autofixes (mutates source)")
	@if find . -name "*.go" -type f | grep -q .; then \
		go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --fix ./... --verbose; \
	else \
		echo "$(YELLOW)No Go files found, skipping$(NC)"; \
	fi

# Clean Go module dependencies (safe for frequent use)
.PHONY: tidy
tidy:
	$(call title1,"Cleaning dependencies")
	@go mod tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dependencies cleaned successfully$(GREEN) ✔️$(NC)"
