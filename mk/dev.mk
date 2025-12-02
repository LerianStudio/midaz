# ------------------------------------------------------
# Development Tools Configuration
# ------------------------------------------------------
# This file contains development-related commands for code
# quality, formatting, and environment setup.

# Tool binaries (auto-detected or can be overridden)
GOFUMPT := $(shell command -v gofumpt 2>/dev/null)
GOIMPORTS := $(shell command -v goimports 2>/dev/null)
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null)

# Installation URLs for error messages
GOFUMPT_INSTALL := go install mvdan.cc/gofumpt@latest
GOIMPORTS_INSTALL := go install golang.org/x/tools/cmd/goimports@latest
GOLANGCI_LINT_INSTALL := go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

#-------------------------------------------------------
# Tool Installation
#-------------------------------------------------------

.PHONY: tools-dev tools-gofumpt tools-goimports tools-golangci-lint

tools-dev: tools-gofumpt tools-goimports tools-golangci-lint ## Install all development tools
	@echo "[ok] All development tools installed"

tools-gofumpt: ## Install gofumpt (stricter gofmt)
	@if [ -z "$(GOFUMPT)" ]; then \
		echo "Installing gofumpt..."; \
		$(GOFUMPT_INSTALL); \
		echo "[ok] gofumpt installed"; \
	else \
		echo "[ok] gofumpt already installed: $(GOFUMPT)"; \
	fi

tools-goimports: ## Install goimports (import organizer)
	@if [ -z "$(GOIMPORTS)" ]; then \
		echo "Installing goimports..."; \
		$(GOIMPORTS_INSTALL); \
		echo "[ok] goimports installed"; \
	else \
		echo "[ok] goimports already installed: $(GOIMPORTS)"; \
	fi

tools-golangci-lint: ## Install golangci-lint
	@if [ -z "$(GOLANGCI_LINT)" ]; then \
		echo "Installing golangci-lint..."; \
		$(GOLANGCI_LINT_INSTALL); \
		echo "[ok] golangci-lint installed"; \
	else \
		echo "[ok] golangci-lint already installed: $(GOLANGCI_LINT)"; \
	fi

#-------------------------------------------------------
# Code Formatting
#-------------------------------------------------------

.PHONY: gofumpt goimports format

gofumpt: tools-gofumpt ## Run gofumpt on all Go files (stricter formatting)
	$(call print_title,Running gofumpt)
	@echo "Formatting Go files with gofumpt..."
	@gofumpt -l -w .
	@echo "[ok] gofumpt completed"

goimports: tools-goimports ## Run goimports on all Go files (organize imports)
	$(call print_title,Running goimports)
	@echo "Organizing imports with goimports..."
	@goimports -l -w .
	@echo "[ok] goimports completed"

format: goimports gofumpt ## Run all formatters (goimports + gofumpt)
	$(call print_title,Formatting code in all components)
	@for dir in $(COMPONENTS); do \
		echo "Checking for Go files in $$dir..."; \
		if find "$$dir" -name "*.go" -type f | grep -q .; then \
			echo "Formatting in $$dir..."; \
			(cd $$dir && $(MAKE) format 2>/dev/null) || true; \
		else \
			echo "No Go files found in $$dir, skipping formatting"; \
		fi; \
	done
	@echo "[ok] Formatting completed successfully"

#-------------------------------------------------------
# Linting
#-------------------------------------------------------

.PHONY: lint lint-fix

lint: tools-golangci-lint ## Run linters on all Go code
	$(call print_title,Running linters on all components)
	@for dir in $(COMPONENTS); do \
		echo "Checking for Go files in $$dir..."; \
		if find "$$dir" -name "*.go" -type f | grep -q .; then \
			echo "Linting in $$dir..."; \
			(cd $$dir && $(MAKE) lint 2>/dev/null) || (cd $$dir && golangci-lint run ./... --verbose) || exit 1; \
		else \
			echo "No Go files found in $$dir, skipping linting"; \
		fi; \
	done
	@echo "Checking for Go files in $(TESTS_DIR)..."
	@if [ -d "$(TESTS_DIR)" ]; then \
		if find "$(TESTS_DIR)" -name "*.go" -type f | grep -q .; then \
			echo "Linting in $(TESTS_DIR)..."; \
			(cd $(TESTS_DIR) && golangci-lint run ./... --verbose) || exit 1; \
		else \
			echo "No Go files found in $(TESTS_DIR), skipping linting"; \
		fi; \
	else \
		echo "No tests directory found at $(TESTS_DIR), skipping linting"; \
	fi
	@echo "[ok] Linting completed successfully"

lint-fix: tools-golangci-lint ## Run linters with auto-fix enabled
	$(call print_title,Running linters with auto-fix)
	@for dir in $(COMPONENTS); do \
		echo "Checking for Go files in $$dir..."; \
		if find "$$dir" -name "*.go" -type f | grep -q .; then \
			echo "Linting (with fix) in $$dir..."; \
			(cd $$dir && golangci-lint run --fix ./... --verbose) || exit 1; \
		else \
			echo "No Go files found in $$dir, skipping linting"; \
		fi; \
	done
	@if [ -d "$(TESTS_DIR)" ]; then \
		if find "$(TESTS_DIR)" -name "*.go" -type f | grep -q .; then \
			echo "Linting (with fix) in $(TESTS_DIR)..."; \
			(cd $(TESTS_DIR) && golangci-lint run --fix ./... --verbose) || exit 1; \
		fi; \
	fi
	@echo "[ok] Linting with auto-fix completed"

#-------------------------------------------------------
# Code Quality Checks
#-------------------------------------------------------

.PHONY: tidy check-logs check-tests sec

tidy: ## Clean dependencies in root directory
	$(call print_title,Cleaning dependencies in root directory)
	@echo "Tidying root go.mod..."
	@go mod tidy
	@echo "[ok] Dependencies cleaned successfully"

check-logs: ## Verify error logging in usecases
	$(call print_title,Verifying error logging in usecases)
	@sh ./scripts/check-logs.sh
	@echo "[ok] Error logging verification completed"

check-tests: ## Verify test coverage for components
	$(call print_title,Verifying test coverage for components)
	@sh ./scripts/check-tests.sh
	@echo "[ok] Test coverage verification completed"

sec: ## Run security checks using gosec
	$(call print_title,Running security checks using gosec)
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "Installing gosec..."; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@if find ./components ./pkg -name "*.go" -type f | grep -q .; then \
		echo "Running security checks on components/ and pkg/ folders..."; \
		gosec ./components/... ./pkg/...; \
		echo "[ok] Security checks completed"; \
	else \
		echo "No Go files found, skipping security checks"; \
	fi

#-------------------------------------------------------
# Git Hooks
#-------------------------------------------------------

.PHONY: setup-git-hooks check-hooks check-envs

setup-git-hooks: ## Install and configure git hooks
	$(call print_title,Installing and configuring git hooks)
	@sh ./scripts/setup-git-hooks.sh
	@echo "[ok] Git hooks installed successfully"

check-hooks: ## Verify git hooks installation status
	$(call print_title,Verifying git hooks installation status)
	@err=0; \
	for hook_dir in .githooks/*; do \
		hook_name=$$(basename $$hook_dir); \
		if [ ! -f ".git/hooks/$$hook_name" ]; then \
			echo "Git hook $$hook_name is not installed"; \
			err=1; \
		else \
			echo "Git hook $$hook_name is installed"; \
		fi; \
	done; \
	if [ $$err -eq 0 ]; then \
		echo "[ok] All git hooks are properly installed"; \
	else \
		echo "[error] Some git hooks are missing. Run 'make setup-git-hooks' to fix."; \
		exit 1; \
	fi

check-envs: ## Check if github hooks are installed and secret env files are not exposed
	$(call print_title,Checking if github hooks are installed and secret env files are not exposed)
	@sh ./scripts/check-envs.sh
	@echo "[ok] Environment check completed"

#-------------------------------------------------------
# Development Environment Setup
#-------------------------------------------------------

.PHONY: dev-setup

dev-setup: setup-git-hooks tools-dev ## Set up complete development environment
	$(call print_title,Setting up development environment for all components)
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		echo "Setting up development environment for component: $$component_name"; \
		(cd $$dir && $(MAKE) dev-setup 2>/dev/null) || true; \
		echo ""; \
	done
	@echo "[ok] Development environment set up successfully for all components"

#-------------------------------------------------------
# Pre-commit Workflow
#-------------------------------------------------------

.PHONY: pre-commit

pre-commit: format lint ## Run pre-commit checks (format + lint)
	$(call print_title,Running pre-commit checks)
	@echo "[ok] Pre-commit checks passed"
