# ------------------------------------------------------
# Code quality and maintenance commands
# ------------------------------------------------------
# This file contains all code quality operations including
# linting, formatting, code generation, and dependency management.
#
# Requirements:
#   - golangci-lint v2 (auto-installed if missing)
#   - mockgen (auto-installed if missing)
#   - Go toolchain (go fmt, go generate, go mod)
#
# Variables:
#   None (uses Go standard tools)
#
# Usage:
#   make lint                        # Run linters with auto-fix
#   make format                      # Format Go code
#   make generate                    # Generate mocks and code
#   make tidy                        # Clean unused dependencies
#   make update-deps                 # Update all deps to latest versions
#   make quality                     # Run all quality checks (lint + test)
#
# Tools:
#   - golangci-lint v2: Comprehensive Go linter
#   - go fmt: Official Go code formatter
#   - mockgen: Mock generation tool
#   - go mod tidy: Dependency management
# ------------------------------------------------------

#-------------------------------------------------------
# Commands (alphabetically ordered)
#-------------------------------------------------------

# Format Go code with go fmt
# Applies official Go formatting standards to all .go files
.PHONY: format
format:
	$(call title1,"Formatting code")
	@go fmt ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Formatting completed successfully$(GREEN) ✔️$(NC)"

# Generate code (mocks, etc.)
# Runs go generate on all packages
# Auto-installs mockgen if not present
.PHONY: generate
generate:
	$(call title1,"Generating code (mocks, etc.)")
	@if ! command -v mockgen >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing mockgen...$(NC)"; \
		go install go.uber.org/mock/mockgen@v0.5.2; \
	fi
	@go generate ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Code generation completed successfully$(GREEN) ✔️$(NC)"

# Run golangci-lint with auto-fix
# Checks for common issues, errors, and code smells
# Auto-installs golangci-lint v2 if not present
.PHONY: lint
lint:
	$(call title1,"Running linters")
	@if find . -name "*.go" -type f | grep -q .; then \
		if ! command -v golangci-lint >/dev/null 2>&1; then \
			echo "$(YELLOW)Installing golangci-lint v2...$(NC)"; \
			go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
		fi; \
		golangci-lint run --fix ./... --verbose && \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Linting completed successfully$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping linting$(NC)"; \
	fi

# Run all quality checks (aggregator command)
# Executes lint and test sequentially
# Use before committing or creating pull requests
.PHONY: quality
quality: lint test
	$(call title1,"Quality checks complete")
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All quality checks passed$(GREEN) ✔️$(NC)"
	@echo ""
	@echo "Checks passed:"
	@echo "  ✅ Linting (errorlint, contextcheck)"
	@echo "  ✅ Unit tests"
	@echo ""
	@echo "$(GREEN)Ready to commit and push!$(NC)"

# Clean Go module dependencies (safe for frequent use)
# Runs go mod tidy to remove unused dependencies
.PHONY: tidy
tidy:
	$(call title1,"Cleaning dependencies")
	@go mod tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dependencies cleaned successfully$(GREEN) ✔️$(NC)"

# Update all dependencies to latest versions and clean
# Use intentionally: upgrades all transitive deps
.PHONY: update-deps
update-deps:
	$(call title1,"Updating all dependencies")
	@go get -u ./...
	@go mod tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dependencies updated and cleaned successfully$(GREEN) ✔️$(NC)"
