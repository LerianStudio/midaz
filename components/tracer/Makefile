# Tracer Makefile

# Component-specific variables
SERVICE_NAME := tracer
GOLANGCI_LINT_VERSION := v2.4.0
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts
POSTGRES_SERVICE ?= tracer-postgres

# Ensure artifacts directory exists
$(shell mkdir -p $(ARTIFACTS_DIR))

# Test reports directory (used by mk/tests.mk)
TEST_REPORTS_DIR ?= ./reports

DOCKER_CMD := $(shell \
	if [ "$(shell printf '%s\n' "$(DOCKER_MIN_VERSION)" "$(DOCKER_VERSION)" | sort -V | head -n1)" = "$(DOCKER_MIN_VERSION)" ]; then \
		echo "docker compose"; \
	else \
		echo "docker-compose"; \
	fi \
)

# Include shared color definitions and utility functions
include pkg/shell/makefile_colors.mk
include pkg/shell/makefile_utils.mk

# Include modular makefiles
include mk/database.mk
include mk/docker.mk
include mk/docs.mk
include mk/quality.mk
include mk/security.mk
include mk/tests.mk

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)Plugin Service Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make help                        - Display this help message"
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
	@echo "  make coverage-unit               - Run unit tests with coverage report (PKG=./path, uses .ignorecoverunit)"
	@echo "  make coverage-integration        - Run integration tests with coverage report (PKG=./path)"
	@echo "  make coverage                    - Run all coverage targets (unit + integration)"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run linting tools"
	@echo "  make format                      - Format code with go fmt"
	@echo "  make generate                    - Generate code (mocks, etc.)"
	@echo "  make tidy                        - Clean unused dependencies (go mod tidy)"
	@echo "  make update-deps                 - Update all deps to latest versions"
	@echo "  make sec                         - Run security checks (gosec + govulncheck)"
	@echo "  make sec SARIF=1                 - Run security checks with SARIF output"
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "  make up                          - Start services with Docker Compose"
	@echo "  make down                        - Stop services with Docker Compose"
	@echo "  make start                       - Start existing containers"
	@echo "  make stop                        - Stop running containers"
	@echo "  make restart                     - Restart all containers"
	@echo "  make logs                        - Show logs for all services"
	@echo "  make logs-api                    - Show logs for plugin service"
	@echo "  make ps                          - List container status"
	@echo "  make rebuild-up                  - Rebuild and restart services during development"
	@echo ""
	@echo "$(BOLD)Database Commands:$(NC)"
	@echo "  make migrate                     - Apply all pending migrations"
	@echo "  make migrate-down                - Rollback last migration"
	@echo "  make migrate-down-all            - Rollback all migrations (5s confirmation, FORCE=1 to skip)"
	@echo "  make migrate-version             - Show current migration version"
	@echo "  make migrate-force VERSION=N     - Force set migration version (use with caution)"
	@echo "  make seed                        - Load development seed data"
	@echo "  make seed-down                   - Remove development seed data"
	@echo ""
	@echo "$(BOLD)Tracer-Specific Commands:$(NC)"
	@echo "  make generate-docs               - Generate Swagger API documentation"
	@echo "  make verify-api-docs             - Verify API documentation coverage"
	@echo "  make validate-api-docs           - Validate API documentation"
	@echo ""
	@echo "$(BOLD)Test Tooling:$(NC)"
	@echo "  make tools                       - Install test tools (gotestsum)"
	@echo "  make wait-for-services           - Wait for backend services to be healthy"
	@echo ""
	@echo "$(BOLD)Test Parameters (env vars for test-* targets):$(NC)"
	@echo "  TEST_TRACER_URL               - default: http://localhost:\$$(SERVER_PORT) (auto-read from .env, fallback 4020)"
	@echo "  TEST_HEALTH_WAIT              - default: 60"
	@echo "  LOW_RESOURCE                  - 0|1 (default: 0) - reduces parallelism for CI"
	@echo "  RETRY_ON_FAIL                 - 0|1 (default: 0)"
	@echo "  E2E_SERVER                    - default: http://localhost:\$$(SERVER_PORT) (E2E target URL, auto-read from .env)"
	@echo "  E2E_API_KEY                   - default: dev_api_key_32chars_change_in_prod"
	@echo "  E2E_SKIP_RESET                - 0|1 (default: 0) - skip Docker volume reset"
	@echo ""
	@echo "$(BOLD)Developer Helper Commands:$(NC)"
	@echo "  make dev-setup                   - Set up development environment"
	@echo ""
	@echo "$(BOLD)Workflow Aggregators (run before push / merge):$(NC)"
	@echo "  make pre-push                    - Fast pre-push gate: tidy + format + lint + sec + test-unit (no Docker)"
	@echo "  make pre-merge                   - Full pre-merge gate: pre-push + verify-api-docs + test-integration + test-e2e (requires Docker, DESTRUCTIVE — resets Docker volumes)"
	@echo ""

#-------------------------------------------------------
# Workflow Aggregator Commands
#-------------------------------------------------------
# These targets bundle the standard quality gates into named workflows so
# developers can run the same checks locally that CI runs on every PR. Each
# step is invoked via $(MAKE) so changes to the underlying targets propagate
# automatically.
#
# Failure semantics: Make stops at the first failing sub-target (default
# behavior). The aggregator prints a clear final status on success.

# Recipe bodies live in `define` blocks (not prerequisites) for two reasons:
#   1. Sequential semantics: with `make -j`, prerequisites can run in parallel.
#      lint and test-unit would race on the same workspace; the gates are
#      intentionally sequential.
#   2. Title banner: $(call title1, ...) belongs in the orchestrator recipe so
#      the user sees the framing BEFORE the first sub-step runs.
# This shape keeps recipes at <= 5 lines (checkmake maxbodylength) without
# losing either property.

# Sub-step invocations use shell `&&` chaining to guarantee strict sequential
# execution. A single `$(MAKE) goal1 goal2 goal3` recursive call would NOT be
# sequential under `make -j` — GNU Make's jobserver schedules multiple goals
# concurrently via job-slot tokens (gnu.org/s/make/manual/html_node/Parallel.html).
# `&&` runs each sub-make only after the previous one exited 0, short-circuiting
# on failure — which is exactly the semantics a quality gate needs.

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
# Setup Commands
#-------------------------------------------------------

.PHONY: set-env
set-env:
	$(call title1,"Setting up environment files")

	@if [ -f ".env.example" ] && [ ! -f ".env" ]; then \
		echo "$(CYAN)Creating .env in plugin from .env.example$(NC)"; \
		cp ".env.example" ".env"; \
	elif [ ! -f ".env.example" ]; then \
		echo "$(YELLOW)Warning: No .env.example found in plugin$(NC)"; \
	else \
		echo "$(GREEN).env already exists in plugin$(NC)"; \
	fi

	@echo "$(GREEN)$(BOLD)[ok]$(NC) Environment files set up successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call title1,"Building component")
	@mkdir -p $(BIN_DIR)
	@CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN_DIR)/$(SERVICE_NAME) ./cmd/app
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Build completed successfully - binary at $(BIN_DIR)/$(SERVICE_NAME)$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Clean Commands
#-------------------------------------------------------

.PHONY: clean
clean:
	$(call title1,"Cleaning build artifacts")
	@rm -rf $(BIN_DIR)/* $(ARTIFACTS_DIR)/*
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Artifacts cleaned successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call title1,"Setting up development environment")
	@echo "$(CYAN)Installing development tools...$(NC)"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing golangci-lint v2...$(NC)"; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	@if ! command -v swag >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing swag...$(NC)"; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
	fi
	@if ! command -v mockgen >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing mockgen...$(NC)"; \
		go install go.uber.org/mock/mockgen@v0.5.2; \
	fi
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing gosec...$(NC)"; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@echo "$(CYAN)Setting up environment...$(NC)"
	@if [ -f .env.example ] && [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "$(GREEN)Created .env file from template$(NC)"; \
	fi
	@make tidy
	@make check-tests
	@make sec
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully$(GREEN) ✔️$(NC)"
	@echo "$(CYAN)You're ready to start developing! Here are some useful commands:$(NC)"
	@echo "  make build         - Build the component"
	@echo "  make test          - Run tests"
	@echo "  make up            - Start services"
	@echo "  make rebuild-up    - Rebuild and restart services during development"
