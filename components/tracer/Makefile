# Tracer Component Makefile
#
# Thin component Makefile built from the shared monorepo template: it sets the
# component identity (SERVICE_NAME, MIDAZ_ROOT, scoped coverage) and includes the
# root mk/*.mk fragments rather than re-implementing docker/quality/security
# logic. Component-specific targets (build flags, BDD e2e, docs, workflow
# aggregators) live here.

# Component identity
SERVICE_NAME := tracer
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts
TEST_REPORTS_DIR ?= ./reports

# Tracer points at the shared infra Postgres (no standalone tracer-postgres).
POSTGRES_SERVICE ?= midaz-postgres-primary

# Pinned tool versions — keep in sync with .github/workflows/go-combined-analysis.yml
GOLANGCI_LINT_VERSION := v2.12.2

# Ensure build/output directories exist
$(shell mkdir -p $(BIN_DIR) $(ARTIFACTS_DIR))

# Root of the monorepo (for shared mk fragments and scoped coverage)
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)
COVERAGE_PACKAGES := ./components/tracer/...

# Shared monorepo fragments — one template, included by every component.
include $(MIDAZ_ROOT)/mk/colors.mk
include $(MIDAZ_ROOT)/mk/utils.mk
include $(MIDAZ_ROOT)/mk/docker.mk
include $(MIDAZ_ROOT)/mk/database.mk
include $(MIDAZ_ROOT)/mk/docs.mk
include $(MIDAZ_ROOT)/mk/quality.mk
include $(MIDAZ_ROOT)/mk/security.mk
# tracer's component-local tests.mk owns the test/coverage targets (test-unit,
# test-integration, test-e2e/godog, coverage-unit) — it is NOT promoted to root.
include mk/tests.mk

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)Tracer Service Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make build                       - Build the binary to .bin/tracer"
	@echo "  make test                        - Run all tests"
	@echo "  make clean                       - Clean build artifacts"
	@echo "  make run                         - Run the application with .env config"
	@echo ""
	@echo "$(BOLD)Test Suite Aliases:$(NC)"
	@echo "  make test-unit                   - Run Go unit tests"
	@echo "  make test-integration            - Run integration tests with testcontainers (RUN=<test>)"
	@echo "  make test-e2e                    - Run E2E BDD tests (resets DB, runs Godog scenarios)"
	@echo "  make test-all                    - Run all tests (unit + integration)"
	@echo "  make test-bench                  - Run benchmark tests (BENCH=pattern, BENCH_PKG=./path)"
	@echo ""
	@echo "$(BOLD)Coverage Commands:$(NC)"
	@echo "  make coverage-unit               - Run unit tests with coverage report"
	@echo "  make coverage-integration        - Run integration tests with coverage report"
	@echo "  make coverage                    - Run all coverage targets"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run golangci-lint (read-only gate)"
	@echo "  make format                      - Format code with go fmt"
	@echo "  make generate                    - Generate code (mocks, etc.)"
	@echo "  make tidy                        - Clean unused dependencies"
	@echo "  make sec                         - Run security checks (gosec + govulncheck)"
	@echo "  make sec SARIF=1                 - Run security checks with SARIF output"
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "  make up / down / start / stop / restart / rebuild-up / logs / ps"
	@echo ""
	@echo "$(BOLD)Database Commands:$(NC)"
	@echo "  make migrate / migrate-down / migrate-version / seed / seed-down"
	@echo ""
	@echo "$(BOLD)Documentation Commands:$(NC)"
	@echo "  make verify-api-docs             - Verify API documentation coverage"
	@echo "  (API docs are generated from the repo root: make generate-docs)"
	@echo ""
	@echo "$(BOLD)Developer Helper Commands:$(NC)"
	@echo "  make dev-setup                   - Set up development environment"
	@echo ""
	@echo "$(BOLD)Workflow Aggregators (run before push / merge):$(NC)"
	@echo "  make pre-push                    - Fast pre-push gate: tidy + format + lint + sec + test-unit (no Docker)"
	@echo "  make pre-merge                   - Full pre-merge gate: pre-push + verify-api-docs + test-integration + test-e2e (requires Docker, DESTRUCTIVE)"
	@echo ""

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call title1,"Building $(SERVICE_NAME) component")
	@mkdir -p $(BIN_DIR)
	@CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN_DIR)/$(SERVICE_NAME) ./cmd/app
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Build completed successfully - binary at $(BIN_DIR)/$(SERVICE_NAME)$(GREEN) ✔️$(NC)"

.PHONY: clean
clean:
	$(call title1,"Cleaning build artifacts")
	@rm -rf $(BIN_DIR)/* $(ARTIFACTS_DIR)/*
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Artifacts cleaned successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Workflow Aggregator Commands
#-------------------------------------------------------
# Sub-step invocations use shell `&&` chaining to guarantee strict sequential
# execution under `make -j` (a single recursive `$(MAKE) a b c` would let the
# jobserver schedule the goals concurrently).

define pre_push_steps
$(call title1,"Running pre-push gate (fast static checks, no Docker required)")
@$(MAKE) tidy && $(MAKE) format && $(MAKE) lint && $(MAKE) sec && $(MAKE) test-unit
@echo "$(GREEN)$(BOLD)[ok]$(NC) pre-push gate completed successfully — safe to push$(GREEN) ✔️$(NC)"
endef

.PHONY: pre-push
pre-push:
	$(pre_push_steps)

define pre_merge_steps
$(call title1,"Running pre-merge gate (full CI parity — requires Docker, DESTRUCTIVE: resets Docker volumes via test-e2e)")
@$(MAKE) pre-push && $(MAKE) verify-api-docs && $(MAKE) test-integration && $(MAKE) test-e2e
@echo "$(GREEN)$(BOLD)[ok]$(NC) pre-merge gate completed successfully — safe to merge$(GREEN) ✔️$(NC)"
endef

.PHONY: pre-merge
pre-merge:
	$(pre_merge_steps)

#-------------------------------------------------------
# Git Hook Commands
#-------------------------------------------------------

.PHONY: setup-git-hooks
setup-git-hooks:
	$(call title1,"Installing and configuring git hooks")
	@sh ./scripts/setup-git-hooks.sh
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Git hooks installed successfully$(GREEN) ✔️$(NC)"

.PHONY: check-hooks
check-hooks:
	$(call title1,"Verifying git hooks installation status")
	@err=0; \
	for hook_dir in .githooks/*; do \
		hook_name=$$(basename $$hook_dir); \
		if [ ! -f ".git/hooks/$$hook_name" ]; then \
			echo "$(RED)Git hook $$hook_name is not installed$(NC)"; \
			err=1; \
		else \
			echo "$(GREEN)Git hook $$hook_name is installed$(NC)"; \
		fi; \
	done; \
	if [ $$err -eq 0 ]; then \
		echo "$(GREEN)$(BOLD)[ok]$(NC) All git hooks are properly installed$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(RED)$(BOLD)[error]$(NC) Some git hooks are missing. Run 'make setup-git-hooks' to fix.$(RED) ❌$(NC)"; \
		exit 1; \
	fi

.PHONY: check-envs
check-envs:
	$(call title1,"Checking if github hooks are installed and secret env files are not exposed")
	@sh ./scripts/check-envs.sh
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Environment check completed$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Setup / Developer Helper Commands
#-------------------------------------------------------

.PHONY: set-env
set-env:
	$(call title1,"Setting up environment files")
	@if [ -f ".env.example" ] && [ ! -f ".env" ]; then \
		echo "$(CYAN)Creating .env from .env.example$(NC)"; \
		cp ".env.example" ".env"; \
	elif [ ! -f ".env.example" ]; then \
		echo "$(YELLOW)Warning: No .env.example found$(NC)"; \
	else \
		echo "$(GREEN).env already exists$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Environment files set up successfully$(GREEN) ✔️$(NC)"

.PHONY: dev-setup
dev-setup:
	$(call title1,"Setting up development environment")
	@echo "$(CYAN)Installing development tools...$(NC)"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	@if ! command -v swag >/dev/null 2>&1; then \
		go install github.com/swaggo/swag/cmd/swag@latest; \
	fi
	@if ! command -v mockgen >/dev/null 2>&1; then \
		go install go.uber.org/mock/mockgen@v0.6.0; \
	fi
	@if ! command -v gosec >/dev/null 2>&1; then \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@$(MAKE) set-env
	@$(MAKE) tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully$(GREEN) ✔️$(NC)"
