# Project Root Makefile.
# Coordinates all component Makefiles and provides centralized commands.
# Midaz Project Management.

# Define the root directory of the project
MIDAZ_ROOT := $(shell pwd)

# Component directories
INFRA_DIR := ./components/infra
ONBOARDING_DIR := ./components/onboarding
TRANSACTION_DIR := ./components/transaction
LEDGER_DIR := ./components/ledger
CRM_DIR := ./components/crm
TESTS_DIR := ./tests
PKG_DIR := ./pkg

# Define a list of all component directories for easier iteration
COMPONENTS := $(INFRA_DIR) $(ONBOARDING_DIR) $(TRANSACTION_DIR) $(CRM_DIR)

# Unified mode: true = ledger (combined), false = separate onboarding+transaction
UNIFIED ?= true

# Include shared utility functions
# Define common utility functions
define print_title
	@echo ""
	@echo "------------------------------------------"
	@echo "   📝 $(1)  "
	@echo "------------------------------------------"
endef

# Shell utility functions
define print_logo
	@cat $(PWD)/pkg/shell/logo.txt
endef

# Check if a command is available
define check_command
	@if ! command -v $(1) >/dev/null 2>&1; then \
		echo "Error: $(1) is required but not installed."; \
		echo "To install: $(2)"; \
		exit 1; \
	fi
endef

# Check if environment files exist
define check_env_files
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/.env.example" ] && [ ! -f "$$dir/.env" ]; then \
			echo "Warning: $$dir/.env file is missing. Consider running 'make set-env'."; \
		fi; \
	done
	@if [ ! -f "$(INFRA_DIR)/.env" ]; then \
		echo "Environment files are missing. Running set-env command first..."; \
		$(MAKE) set-env; \
	fi
endef

# Choose docker compose command depending on installed version
DOCKER_CMD := $(shell if docker compose version >/dev/null 2>&1; then echo "docker compose"; else echo "docker-compose"; fi)
export DOCKER_CMD

# Benchmark + k6 defaults
K6_DIR ?= ./tests/k6
K6_REPO_URL ?= https://github.com/LerianStudio/k6.git
K6_REPO_LOCAL ?= ../k6
K6_SCENARIO_DIR ?= tests/v3.x.x/tps_api_first_accounting
BENCH_PROFILE ?=
PROFILE ?= $(if $(BENCH_PROFILE),$(BENCH_PROFILE),load)
BENCH_ENVIRONMENT ?= midaz-bench
BENCH_AUTH_ENABLED ?= false
BENCH_LOG ?= OFF
BENCH_NAMESPACE ?=
BENCH_ORG_COUNT ?= 1
BENCH_LEDGERS_PER_ORG ?= 1
BENCH_ACCOUNTS_PER_TYPE ?= 500
BENCH_FUND_AMOUNT ?= 1000000.00
BENCH_TRANSACTION_AMOUNT ?= 1.00
# Bootstrap creates topology sequentially (no work partitioning across VUs),
# so parallelism only causes duplicate idempotent requests. Default to 1.
BENCH_BOOTSTRAP_VUS ?= 1
BENCH_FUND_VUS ?= 50

ifeq ($(PROFILE),smoke)
BENCH_PROFILE_TPS := 10
BENCH_PROFILE_DURATION := 10s
BENCH_PROFILE_PRE_VUS := 5
BENCH_PROFILE_MAX_VUS := 20
else ifeq ($(PROFILE),load)
BENCH_PROFILE_TPS := 500
BENCH_PROFILE_DURATION := 10s
BENCH_PROFILE_PRE_VUS := 200
BENCH_PROFILE_MAX_VUS := 1000
else ifeq ($(PROFILE),stress)
BENCH_PROFILE_TPS := 10000
BENCH_PROFILE_DURATION := 10s
BENCH_PROFILE_PRE_VUS := 2000
BENCH_PROFILE_MAX_VUS := 20000
else ifeq ($(PROFILE),100k)
BENCH_PROFILE_TPS := 100000
BENCH_PROFILE_DURATION := 10s
BENCH_PROFILE_PRE_VUS := 5000
BENCH_PROFILE_MAX_VUS := 20000
else
$(error Invalid PROFILE='$(PROFILE)'. Use smoke, load, stress, or 100k)
endif

BENCH_TPS ?= $(BENCH_PROFILE_TPS)
BENCH_DURATION ?= $(BENCH_PROFILE_DURATION)
BENCH_PRE_VUS ?= $(BENCH_PROFILE_PRE_VUS)
BENCH_MAX_VUS ?= $(BENCH_PROFILE_MAX_VUS)
BENCH_BUILD_STACK ?= false

MK_DIR := $(abspath mk)

COVERAGE_PACKAGES := ./...
include $(MK_DIR)/coverage-unit.mk
include $(MK_DIR)/tests.mk

#-------------------------------------------------------
# Help Command
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo ""
	@echo "Midaz Project Management Commands"
	@echo ""
	@echo ""
	@echo "Core Commands:"
	@echo "  make help                        - Display this help message"
	@echo "  make test                        - Run tests on all components"
	@echo "  make build                       - Build all components"
	@echo "  make clean                       - Clean all build artifacts"
	@echo "  make cover                       - Run test coverage"
	@echo ""
	@echo ""
	@echo "Code Quality Commands:"
	@echo "  make lint                        - Run linting on all components"
	@echo "  make format                      - Format code in all components"
	@echo "  make tidy                        - Clean dependencies in root directory"
	@echo "  make check-logs                  - Verify error logging in usecases"
	@echo "  make check-tests                 - Verify test coverage for components"
	@echo "  make sec                         - Run security checks (gosec + govulncheck)"
	@echo "  make sec SARIF=1                 - Run security checks with SARIF output"
	@echo ""
	@echo ""
	@echo "CI Commands:"
	@echo "  make ci                          - Run local CI pipeline (tidy, docs, lint, checks, unit tests, sec, build)"
	@echo "  make ci-full                     - Run 'ci' plus integration tests (requires Docker for testcontainers)"
	@echo ""
	@echo ""
	@echo "Git Hook Commands:"
	@echo "  make setup-git-hooks             - Install and configure git hooks"
	@echo "  make check-hooks                 - Verify git hooks installation status"
	@echo "  make check-envs                  - Check if github hooks are installed and secret env files are not exposed"
	@echo ""
	@echo ""
	@echo "Setup Commands:"
	@echo "  make set-env                     - Copy .env.example to .env for all components"
	@echo "  make clear-envs                  - Remove .env files from all components"
	@echo "  make dev-setup                   - Set up development environment for all components (includes git hooks)"
	@echo ""
	@echo ""
	@echo "Service Commands:"
	@echo "  make up                           - Start full dev stack (infra + ledger/onboarding+transaction + CRM)"
	@echo "  make down                         - Stop all services"
	@echo "  make start                        - Start all containers"
	@echo "  make stop                         - Stop all containers"
	@echo "  make restart                      - Restart all containers"
	@echo "  make rebuild-up                   - Rebuild and restart all services"
	@echo "  make bench-up                     - Start benchmark-only infra stack (4 ledgers + 2 authorizers + nginx)"
	@echo "  make bench-down                   - Stop benchmark infra stack"
	@echo "  make bench-restart                - Restart benchmark infra stack"
	@echo "  make clean-docker                 - Clean all Docker resources (containers, networks, volumes)"
	@echo "  make logs                         - Show logs for all services"
	@echo "  make infra COMMAND=<cmd>          - Run command in infra component"
	@echo "  make onboarding COMMAND=<cmd>     - Run command in onboarding component"
	@echo "  make transaction COMMAND=<cmd>    - Run command in transaction component"
	@echo "  make all-components COMMAND=<cmd> - Run command across all components"
	@echo "  make ledger COMMAND=<cmd>         - Run command in ledger component"
	@echo "  make k6-run PROFILE=<p>           - Run k6 benchmark suite (stack must be running; see 'make bench-up')"
	@echo "  make k6-bootstrap                 - Run k6 bootstrap only (topology + fund accounts)"
	@echo "  make k6-transactions PROFILE=<p>  - Run k6 transactions only (requires BENCH_NAMESPACE)"
	@echo "  make k6-validate                  - Validate persisted benchmark data + invariants"
	@echo "  make k6-monitor                   - Stream Redpanda consumer lag as CSV"
	@echo ""
	@echo "  k6-run vars (defaults):"
	@echo "    PROFILE=$(PROFILE) BENCH_ENVIRONMENT=$(BENCH_ENVIRONMENT)"
	@echo "    BENCH_TPS=$(BENCH_TPS) BENCH_DURATION=$(BENCH_DURATION) BENCH_PRE_VUS=$(BENCH_PRE_VUS) BENCH_MAX_VUS=$(BENCH_MAX_VUS)"
	@echo "    BENCH_AUTH_ENABLED=$(BENCH_AUTH_ENABLED) BENCH_BUILD_STACK=$(BENCH_BUILD_STACK)"
	@echo "    BENCH_NAMESPACE=<auto> BENCH_ORG_COUNT=$(BENCH_ORG_COUNT) BENCH_LEDGERS_PER_ORG=$(BENCH_LEDGERS_PER_ORG) BENCH_ACCOUNTS_PER_TYPE=$(BENCH_ACCOUNTS_PER_TYPE)"
	@echo "    BENCH_FUND_AMOUNT=$(BENCH_FUND_AMOUNT) BENCH_TRANSACTION_AMOUNT=$(BENCH_TRANSACTION_AMOUNT)"
	@echo "    BENCH_BOOTSTRAP_VUS=$(BENCH_BOOTSTRAP_VUS) BENCH_FUND_VUS=$(BENCH_FUND_VUS)"
	@echo "    profiles: smoke(10tps/10s) load(500tps/10s) stress(20000tps/10s) 100k(100000tps/10s)"
	@echo ""
	@echo ""
	@echo "Documentation Commands:"
	@echo "  make generate-docs               - Generate Swagger documentation for all services"
	@echo ""
	@echo ""
	@echo "Migration Commands:"
	@echo "  make migrate-lint                - Lint all migrations for dangerous patterns"
	@echo "  make migrate-create              - Create new migration files (requires COMPONENT, NAME)"
	@echo ""
	@echo ""
	@echo "Test Suite Aliases:"
	@echo "  make test-unit                   - Run Go unit tests"
	@echo "  make test-integration            - Run integration tests with testcontainers (RUN=<test>, CHAOS=1)"
	@echo "  make test-all                    - Run all tests (unit + integration)"
	@echo "  make test-bench                  - Run benchmark tests (BENCH=pattern, BENCH_PKG=./path)"
	@echo "  make test-fuzz                   - Run native Go fuzz tests (FUZZ=target, FUZZTIME=duration)"
	@echo "  make test-chaos-system           - Run chaos tests with full Docker stack"
	@echo ""
	@echo "Coverage Commands:"
	@echo "  make coverage-unit               - Run unit tests with coverage report (PKG=./path, uses .ignorecoverunit)"
	@echo "  make coverage-integration        - Run integration tests with coverage report (PKG=./path)"
	@echo "  make coverage                    - Run all coverage targets (unit + integration)"
	@echo ""
	@echo "Test Tooling:"
	@echo "  make tools                       - Install test tools (gotestsum)"
	@echo "  make wait-for-services           - Wait for backend services to be healthy"
	@echo ""
	@echo ""
	@echo "Test Parameters (env vars for test-* targets):"
	@echo "  TEST_ONBOARDING_URL           - default: $(TEST_ONBOARDING_URL)"
	@echo "  TEST_TRANSACTION_URL          - default: $(TEST_TRANSACTION_URL)"
	@echo "  TEST_HEALTH_WAIT              - default: $(TEST_HEALTH_WAIT)"
	@echo "  TEST_AUTH_URL                 - default: $(TEST_AUTH_URL)"
	@echo "  TEST_AUTH_USERNAME            - default: $(TEST_AUTH_USERNAME)"
	@sh -c 'if [ -n "$(TEST_AUTH_PASSWORD)" ]; then echo "  TEST_AUTH_PASSWORD            - [set]"; else echo "  TEST_AUTH_PASSWORD            - [unset]"; fi'
	@echo "  LOW_RESOURCE                  - 0|1 (default: 0) - reduces parallelism for CI"
	@echo "  RETRY_ON_FAIL                 - 0|1 (default: $(RETRY_ON_FAIL))"
	@echo ""
	@echo "Target usage (which vars each target honors):"
	@echo "  test-integration:  PKG, RUN, CHAOS=1, LOW_RESOURCE (testcontainers-based, no external services needed)"
	@echo "  test-chaos-system: TEST_ONBOARDING_URL, TEST_TRANSACTION_URL, TEST_AUTH_* (starts full stack)"
	@echo "  test-fuzz:         FUZZ, FUZZTIME (native Go fuzz testing)"
	@echo "  test-bench:        BENCH, BENCH_PKG (benchmark pattern and package filter)"

 

.PHONY: build
build:
	$(call print_title,Building all components)
	@if [ "$(UNIFIED)" = "true" ]; then \
		echo "Building unified backend (ledger)..."; \
		(cd $(LEDGER_DIR) && $(MAKE) build) || exit 1; \
	else \
		echo "Building separate backend services..."; \
		(cd $(ONBOARDING_DIR) && $(MAKE) build) || exit 1; \
		(cd $(TRANSACTION_DIR) && $(MAKE) build) || exit 1; \
	fi
	@echo "Building CRM..."
	@(cd $(CRM_DIR) && $(MAKE) build) || exit 1
	@echo "[ok] All components built successfully"

.PHONY: clean
clean:
	@./scripts/clean-artifacts.sh

.PHONY: cover
cover:
	$(call print_title,Generating test coverage report)
	@echo "Note: PostgreSQL repository tests are excluded from coverage metrics."
	@echo "See coverage report for details on why and what is being tested."
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@sh ./scripts/coverage.sh
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"
	@echo ""
	@echo "Coverage Summary:"
	@echo "----------------------------------------"
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'
	@echo "----------------------------------------"
	@echo "Open coverage.html in your browser to view detailed coverage report"
	@echo "[ok] Coverage report generated successfully ✔️"

#-------------------------------------------------------
# Code Quality Commands
#-------------------------------------------------------

.PHONY: lint
lint:
	$(call print_title,Running linters on all components)
	@for dir in $(COMPONENTS); do \
		echo "Checking for Go files in $$dir..."; \
		if find "$$dir" -name "*.go" -type f | grep -q .; then \
			echo "Linting in $$dir..."; \
			(cd $$dir && $(MAKE) lint) || exit 1; \
		else \
			echo "No Go files found in $$dir, skipping linting"; \
		fi; \
	done
	@echo "Checking for Go files in $(LEDGER_DIR)..."
	@if find "$(LEDGER_DIR)" -name "*.go" -type f | grep -q .; then \
		echo "Linting in $(LEDGER_DIR)..."; \
		(cd $(LEDGER_DIR) && $(MAKE) lint) || exit 1; \
	else \
		echo "No Go files found in $(LEDGER_DIR), skipping linting"; \
	fi
	@echo "Checking for Go files in $(TESTS_DIR)..."
	@if [ -d "$(TESTS_DIR)" ]; then \
		if find "$(TESTS_DIR)" -name "*.go" -type f | grep -q .; then \
			echo "Linting in $(TESTS_DIR)..."; \
			if ! command -v golangci-lint >/dev/null 2>&1; then \
				echo "golangci-lint not found, installing..."; \
				go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
			else \
				echo "golangci-lint already installed ✔️"; \
			fi; \
			(cd $(TESTS_DIR) && golangci-lint run --fix --build-tags=integration ./... --verbose) || exit 1; \
		else \
			echo "No Go files found in $(TESTS_DIR), skipping linting"; \
		fi; \
	else \
		echo "No tests directory found at $(TESTS_DIR), skipping linting"; \
	fi
	@echo "Checking for Go files in $(PKG_DIR)..."
	@if [ -d "$(PKG_DIR)" ]; then \
		if find "$(PKG_DIR)" -name "*.go" -type f | grep -q .; then \
			echo "Linting in $(PKG_DIR)..."; \
			if ! command -v golangci-lint >/dev/null 2>&1; then \
				echo "golangci-lint not found, installing..."; \
				go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
			else \
				echo "golangci-lint already installed ✔️"; \
			fi; \
			(cd $(PKG_DIR) && golangci-lint run --fix ./... --verbose) || exit 1; \
		else \
			echo "No Go files found in $(PKG_DIR), skipping linting"; \
		fi; \
	else \
		echo "No pkg directory found at $(PKG_DIR), skipping linting"; \
	fi
	@echo "[ok] Linting completed successfully"

.PHONY: format
format:
	$(call print_title,Formatting code in all components)
	@for dir in $(COMPONENTS); do \
		echo "Checking for Go files in $$dir..."; \
		if find "$$dir" -name "*.go" -type f | grep -q .; then \
			echo "Formatting in $$dir..."; \
			(cd $$dir && $(MAKE) format) || exit 1; \
		else \
			echo "No Go files found in $$dir, skipping formatting"; \
		fi; \
	done
	@echo "[ok] Formatting completed successfully"

.PHONY: tidy
tidy:
	$(call print_title,Cleaning dependencies in root directory)
	@echo "Tidying root go.mod..."
	@go mod tidy
	@echo "[ok] Dependencies cleaned successfully"

.PHONY: check-logs
check-logs:
	$(call print_title,Verifying error logging in usecases)
	@sh ./scripts/check-logs.sh
	@echo "[ok] Error logging verification completed"

.PHONY: check-tests
check-tests:
	$(call print_title,Verifying test coverage for components)
	@sh ./scripts/check-tests.sh
	@echo "[ok] Test coverage verification completed"

# SARIF output for GitHub Security tab integration (optional)
# Usage: make sec SARIF=1
SARIF ?= 0

.PHONY: sec-gosec
sec-gosec:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@if find ./components ./pkg -name "*.go" -type f | grep -q .; then \
		echo "Running gosec (via golangci-lint) on components/ and pkg/ folders..."; \
		if [ "$(SARIF)" = "1" ]; then \
			echo "Generating SARIF output: gosec-report.sarif"; \
			golangci-lint run --enable-only=gosec --timeout=5m --output.sarif.path=gosec-report.sarif ./components/... ./pkg/...; \
			echo "[ok] SARIF report generated: gosec-report.sarif"; \
		else \
			golangci-lint run --enable-only=gosec --timeout=5m ./components/... ./pkg/...; \
		fi; \
	else \
		echo "No Go files found, skipping gosec"; \
	fi

.PHONY: sec-govulncheck
sec-govulncheck:
	@if ! command -v govulncheck >/dev/null 2>&1; then \
		echo "Installing govulncheck..."; \
		go install golang.org/x/vuln/cmd/govulncheck@latest; \
	fi
	@if find ./components ./pkg -name "*.go" -type f | grep -q .; then \
		echo "Running govulncheck on components/ and pkg/ folders..."; \
		govulncheck ./components/... ./pkg/...; \
	else \
		echo "No Go files found, skipping govulncheck"; \
	fi

.PHONY: sec
sec:
	$(call print_title,Running security checks)
	@$(MAKE) sec-gosec SARIF=$(SARIF)
	@$(MAKE) sec-govulncheck
	@echo "[ok] Security checks completed"

#-------------------------------------------------------
# CI Commands
#-------------------------------------------------------

.PHONY: ci ci-full

ci:
	$(call print_title,Running local CI verification pipeline)
	@$(MAKE) tidy
	@$(MAKE) generate-docs
	@$(MAKE) lint
	@$(MAKE) check-logs
	@$(MAKE) check-tests
	@$(MAKE) migrate-lint
	@$(MAKE) test
	@$(MAKE) sec
	@$(MAKE) build
	@echo ""
	@echo "=========================================="
	@echo "   [ok] Local CI verification passed"
	@echo "=========================================="

ci-full:
	$(call print_title,Running full CI verification pipeline [includes integration tests])
	@$(MAKE) ci
	@$(MAKE) test-integration
	@echo ""
	@echo "=========================================="
	@echo "   [ok] Full CI verification passed"
	@echo "=========================================="

#-------------------------------------------------------
# Git Hook Commands
#-------------------------------------------------------

.PHONY: setup-git-hooks
setup-git-hooks:
	$(call print_title,Installing and configuring git hooks)
	@git config core.hooksPath .githooks
	@echo "[ok] Git hooks configured (using .githooks/)"

.PHONY: check-hooks
check-hooks:
	$(call print_title,Verifying git hooks installation status)
	@HOOKS_PATH=$$(git config --get core.hooksPath); \
	if [ "$$HOOKS_PATH" = ".githooks" ]; then \
		echo "[ok] Git hooks are configured (core.hooksPath = .githooks)"; \
		echo "Available hooks:"; \
		for hook in .githooks/*; do \
			if [ -x "$$hook" ]; then \
				echo "  - $$(basename $$hook)"; \
			fi; \
		done; \
	else \
		echo "[error] Git hooks not configured. Run 'make setup-git-hooks' to fix."; \
		exit 1; \
	fi

.PHONY: check-envs
check-envs:
	$(call print_title,Checking if github hooks are installed and secret env files are not exposed)
	@sh ./scripts/check-envs.sh
	@echo "[ok] Environment check completed"

#-------------------------------------------------------
# Setup Commands
#-------------------------------------------------------

.PHONY: set-env
set-env:
	$(call print_title,Setting up environment files)
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/.env.example" ] && [ ! -f "$$dir/.env" ]; then \
			echo "Creating .env in $$dir from .env.example"; \
			cp "$$dir/.env.example" "$$dir/.env"; \
		elif [ ! -f "$$dir/.env.example" ]; then \
			echo "Warning: No .env.example found in $$dir"; \
		else \
			echo ".env already exists in $$dir"; \
		fi; \
	done
	@if [ -f "$(LEDGER_DIR)/.env.example" ] && [ ! -f "$(LEDGER_DIR)/.env" ]; then \
		echo "Creating .env in $(LEDGER_DIR) from .env.example"; \
		cp "$(LEDGER_DIR)/.env.example" "$(LEDGER_DIR)/.env"; \
	elif [ ! -f "$(LEDGER_DIR)/.env.example" ]; then \
		echo "Warning: No .env.example found in $(LEDGER_DIR)"; \
	else \
		echo ".env already exists in $(LEDGER_DIR)"; \
	fi
	@# Generate crypto keys for CRM component if .env exists
	@if [ -f "$(CRM_DIR)/.env" ]; then \
		$(MAKE) -C $(CRM_DIR) generate-keys; \
	fi
	@echo "[ok] Environment files set up successfully"

.PHONY: clear-envs
clear-envs:
	$(call print_title,Removing environment files)
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/.env" ]; then \
			echo "Removing .env in $$dir"; \
			rm "$$dir/.env"; \
		else \
			echo "No .env found in $$dir"; \
		fi; \
	done
	@if [ -f "$(LEDGER_DIR)/.env" ]; then \
		echo "Removing .env in $(LEDGER_DIR)"; \
		rm "$(LEDGER_DIR)/.env"; \
	else \
		echo "No .env found in $(LEDGER_DIR)"; \
	fi
	@echo "[ok] Environment files removed successfully"

#-------------------------------------------------------
# Service Commands
#-------------------------------------------------------

.PHONY: up
up:
	$(call print_title,Starting all services with Docker Compose)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@echo "Starting infrastructure services..."
	@cd $(INFRA_DIR) && $(MAKE) up
	@if [ "$(UNIFIED)" = "true" ]; then \
		echo "Starting unified backend (ledger)..."; \
		(cd $(LEDGER_DIR) && $(MAKE) up); \
	else \
		echo "Starting separate backend services..."; \
		(cd $(ONBOARDING_DIR) && $(MAKE) up); \
		(cd $(TRANSACTION_DIR) && $(MAKE) up); \
	fi
	@echo "Starting CRM service..."
	@cd $(CRM_DIR) && $(MAKE) up
	@echo "[ok] All services started successfully"

.PHONY: down
down:
	$(call print_title,Stopping all services with Docker Compose)
	@echo "Stopping CRM service..."
	@if [ -f "$(CRM_DIR)/docker-compose.yml" ]; then \
		(cd $(CRM_DIR) && $(DOCKER_CMD) -f docker-compose.yml down 2>/dev/null) || (cd $(CRM_DIR) && $(DOCKER_CMD) -f docker-compose.yml down); \
	fi
	@if [ "$(UNIFIED)" = "true" ]; then \
		echo "Stopping unified backend (ledger)..."; \
		if [ -f "$(LEDGER_DIR)/docker-compose.yml" ]; then \
			(cd $(LEDGER_DIR) && $(DOCKER_CMD) -f docker-compose.yml down 2>/dev/null) || (cd $(LEDGER_DIR) && $(DOCKER_CMD) -f docker-compose.yml down); \
		fi; \
	else \
		echo "Stopping separate backend services..."; \
		if [ -f "$(TRANSACTION_DIR)/docker-compose.yml" ]; then \
			(cd $(TRANSACTION_DIR) && $(DOCKER_CMD) -f docker-compose.yml down 2>/dev/null) || (cd $(TRANSACTION_DIR) && $(DOCKER_CMD) -f docker-compose.yml down); \
		fi; \
		if [ -f "$(ONBOARDING_DIR)/docker-compose.yml" ]; then \
			(cd $(ONBOARDING_DIR) && $(DOCKER_CMD) -f docker-compose.yml down 2>/dev/null) || (cd $(ONBOARDING_DIR) && $(DOCKER_CMD) -f docker-compose.yml down); \
		fi; \
	fi
	@echo "Stopping infrastructure services..."
	@if [ -f "$(INFRA_DIR)/docker-compose.yml" ]; then \
		(cd $(INFRA_DIR) && $(DOCKER_CMD) -f docker-compose.yml down 2>/dev/null) || (cd $(INFRA_DIR) && $(DOCKER_CMD) -f docker-compose.yml down); \
	fi
	@echo "[ok] All services stopped successfully"

.PHONY: start
start:
	$(call print_title,Starting all containers)
	@echo "Starting infrastructure containers..."
	@cd $(INFRA_DIR) && $(MAKE) start
	@if [ "$(UNIFIED)" = "true" ]; then \
		echo "Starting unified backend containers (ledger)..."; \
		(cd $(LEDGER_DIR) && $(MAKE) start); \
	else \
		echo "Starting separate backend containers..."; \
		(cd $(ONBOARDING_DIR) && $(MAKE) start); \
		(cd $(TRANSACTION_DIR) && $(MAKE) start); \
	fi
	@echo "Starting CRM containers..."
	@cd $(CRM_DIR) && $(MAKE) start
	@echo "[ok] All containers started successfully"

.PHONY: stop
stop:
	$(call print_title,Stopping all containers)
	@echo "Stopping CRM containers..."
	@cd $(CRM_DIR) && $(MAKE) stop 2>/dev/null || true
	@if [ "$(UNIFIED)" = "true" ]; then \
		echo "Stopping unified backend containers (ledger)..."; \
		(cd $(LEDGER_DIR) && $(MAKE) stop 2>/dev/null || true); \
	else \
		echo "Stopping separate backend containers..."; \
		(cd $(TRANSACTION_DIR) && $(MAKE) stop 2>/dev/null || true); \
		(cd $(ONBOARDING_DIR) && $(MAKE) stop 2>/dev/null || true); \
	fi
	@echo "Stopping infrastructure containers..."
	@cd $(INFRA_DIR) && $(MAKE) stop 2>/dev/null || true
	@echo "[ok] All containers stopped successfully"

.PHONY: restart
restart:
	@$(MAKE) down UNIFIED=$(UNIFIED) && $(MAKE) up UNIFIED=$(UNIFIED)
	@echo "[ok] All containers restarted successfully"

.PHONY: bench-up
bench-up:
	$(call print_title,Starting benchmark infrastructure stack)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@echo "Starting benchmark-ready infrastructure stack (4 ledgers + 2 authorizers + nginx)..."
	@cd $(INFRA_DIR) && $(MAKE) up
	@echo "[ok] Benchmark-ready stack started successfully"

.PHONY: bench-down
bench-down:
	$(call print_title,Stopping benchmark infrastructure stack)
	@echo "Stopping benchmark-ready infrastructure stack..."
	@if [ -f "$(INFRA_DIR)/docker-compose.yml" ]; then \
		(cd $(INFRA_DIR) && $(DOCKER_CMD) -f docker-compose.yml down 2>/dev/null) || (cd $(INFRA_DIR) && $(DOCKER_CMD) -f docker-compose.yml down); \
	fi
	@echo "[ok] Benchmark-ready stack stopped successfully"

.PHONY: bench-restart
bench-restart:
	@$(MAKE) bench-down && $(MAKE) bench-up
	@echo "[ok] Benchmark-ready stack restarted successfully"

.PHONY: rebuild-up
rebuild-up:
	$(call print_title,Rebuilding and restarting benchmark-ready stack)
	@echo "Rebuilding infrastructure stack..."
	@cd $(INFRA_DIR) && $(MAKE) rebuild-up
	@echo "[ok] Benchmark-ready stack rebuilt and restarted successfully"

.PHONY: clean-docker
clean-docker:
	$(call print_title,"Cleaning all Docker resources")
	@echo "Cleaning benchmark-ready infrastructure Docker resources..."
	@cd $(INFRA_DIR) && $(MAKE) clean-docker 2>/dev/null || true
	@echo "Pruning system-wide Docker resources..."
	@docker system prune -f
	@echo "Pruning system-wide Docker volumes..."
	@docker volume prune -f
	@echo "[ok] All Docker resources cleaned successfully"

.PHONY: logs
logs:
	$(call print_title,"Showing logs for all services")
	@echo "=== Benchmark-ready infrastructure logs ==="
	@cd $(INFRA_DIR) && $(DOCKER_CMD) -f docker-compose.yml logs --tail=50 2>/dev/null || true

.PHONY: k6-run k6-bootstrap k6-transactions k6-validate k6-monitor bench-run bench-validate bench-monitor

k6-bootstrap:
	$(call print_title,Running k6 bootstrap [topology + fund])
	$(call check_command,k6,"Install k6 with: brew install k6")
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_command,curl,"Install curl from https://curl.se/")
	@# ── run k6 bootstrap (001 + 002) ────────────────────────────
	@if [ ! -d "$(K6_DIR)/$(K6_SCENARIO_DIR)" ]; then \
		echo "Error: k6 scenario directory not found: $(K6_DIR)/$(K6_SCENARIO_DIR)"; \
		exit 1; \
	fi
	@namespace="$(BENCH_NAMESPACE)"; \
	if [ -z "$$namespace" ]; then namespace="api_$$(date +%s)"; fi; \
	echo ""; \
	echo "┌──────────────────────────────────────────────┐"; \
	echo "│  k6 bootstrap: topology + fund               │"; \
	echo "├──────────────────────────────────────────────┤"; \
	echo "│  Namespace       = $$namespace"; \
	echo "│  Accounts/type   = $(BENCH_ACCOUNTS_PER_TYPE)"; \
	echo "│  Fund amount     = $(BENCH_FUND_AMOUNT)"; \
	echo "│  Orgs            = $(BENCH_ORG_COUNT)"; \
	echo "│  Ledgers/org     = $(BENCH_LEDGERS_PER_ORG)"; \
	echo "│  Bootstrap VUs   = $(BENCH_BOOTSTRAP_VUS)"; \
	echo "│  Fund VUs        = $(BENCH_FUND_VUS)"; \
	echo "└──────────────────────────────────────────────┘"; \
	echo ""; \
	( cd "$(K6_DIR)/$(K6_SCENARIO_DIR)" && \
	for kv in $$(env); do name=$${kv%%=*}; case "$$name" in K6_*) unset $$name ;; esac; done; \
	k6 run \
	-e ENVIRONMENT=$(BENCH_ENVIRONMENT) \
	-e AUTH_ENABLED=$(BENCH_AUTH_ENABLED) \
	-e LOG=$(BENCH_LOG) \
	-e BENCH_NAMESPACE=$$namespace \
	-e ORG_COUNT=$(BENCH_ORG_COUNT) \
	-e LEDGERS_PER_ORG=$(BENCH_LEDGERS_PER_ORG) \
	-e ACCOUNTS_PER_TYPE=$(BENCH_ACCOUNTS_PER_TYPE) \
	-e BOOTSTRAP_VUS=$(BENCH_BOOTSTRAP_VUS) \
	001_bootstrap_topology.js ) && \
	( cd "$(K6_DIR)/$(K6_SCENARIO_DIR)" && \
	for kv in $$(env); do name=$${kv%%=*}; case "$$name" in K6_*) unset $$name ;; esac; done; \
	k6 run \
	-e ENVIRONMENT=$(BENCH_ENVIRONMENT) \
	-e AUTH_ENABLED=$(BENCH_AUTH_ENABLED) \
	-e LOG=$(BENCH_LOG) \
	-e BENCH_NAMESPACE=$$namespace \
	-e ORG_COUNT=$(BENCH_ORG_COUNT) \
	-e LEDGERS_PER_ORG=$(BENCH_LEDGERS_PER_ORG) \
	-e ACCOUNTS_PER_TYPE=$(BENCH_ACCOUNTS_PER_TYPE) \
	-e FUND_AMOUNT=$(BENCH_FUND_AMOUNT) \
	-e FUND_VUS=$(BENCH_FUND_VUS) \
	002_fund_accounts.js ) && \
	echo "" && \
	echo "┌──────────────────────────────────────────────┐" && \
	echo "│  Bootstrap complete                          │" && \
	echo "│                                              │" && \
	echo "│  Reuse this namespace for transactions:      │" && \
	echo "│                                              │" && \
	echo "│  make k6-transactions \\                      │" && \
	echo "│    BENCH_NAMESPACE=$$namespace \\              " && \
	echo "│    PROFILE=smoke                             │" && \
	echo "└──────────────────────────────────────────────┘"
	@echo "[ok] k6 bootstrap complete"

k6-transactions:
	$(call print_title,Running k6 transactions [$(PROFILE)])
	$(call check_command,k6,"Install k6 with: brew install k6")
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_command,curl,"Install curl from https://curl.se/")
	@# ── validate namespace ──────────────────────────────────────
	@if [ -z "$(BENCH_NAMESPACE)" ]; then \
		echo ""; \
		echo "Error: BENCH_NAMESPACE is required for k6-transactions."; \
		echo ""; \
		echo "Run bootstrap first to create accounts:"; \
		echo "  make k6-bootstrap BENCH_NAMESPACE=my_bench"; \
		echo ""; \
		echo "Then run transactions with that namespace:"; \
		echo "  make k6-transactions BENCH_NAMESPACE=my_bench PROFILE=smoke"; \
		echo ""; \
		exit 1; \
	fi
	@# ── run k6 transactions (003) ───────────────────────────────
	@if [ ! -d "$(K6_DIR)/$(K6_SCENARIO_DIR)" ]; then \
		echo "Error: k6 scenario directory not found: $(K6_DIR)/$(K6_SCENARIO_DIR)"; \
		exit 1; \
	fi
	@echo ""; \
	echo "┌──────────────────────────────────────────────┐"; \
	echo "│  k6 transactions: profile=$(PROFILE)"; \
	echo "├──────────────────────────────────────────────┤"; \
	echo "│  Namespace       = $(BENCH_NAMESPACE)"; \
	echo "│  TPS             = $(BENCH_TPS)"; \
	echo "│  Duration        = $(BENCH_DURATION)"; \
	echo "│  VUs             = $(BENCH_PRE_VUS) pre / $(BENCH_MAX_VUS) max"; \
	echo "│  Tx amount       = $(BENCH_TRANSACTION_AMOUNT)"; \
	echo "└──────────────────────────────────────────────┘"; \
	echo ""; \
	( cd "$(K6_DIR)/$(K6_SCENARIO_DIR)" && \
	for kv in $$(env); do name=$${kv%%=*}; case "$$name" in K6_*) unset $$name ;; esac; done; \
	k6 run \
	-e ENVIRONMENT=$(BENCH_ENVIRONMENT) \
	-e AUTH_ENABLED=$(BENCH_AUTH_ENABLED) \
	-e LOG=$(BENCH_LOG) \
	-e BENCH_NAMESPACE=$(BENCH_NAMESPACE) \
	-e ORG_COUNT=$(BENCH_ORG_COUNT) \
	-e LEDGERS_PER_ORG=$(BENCH_LEDGERS_PER_ORG) \
	-e ACCOUNTS_PER_TYPE=$(BENCH_ACCOUNTS_PER_TYPE) \
	-e FUND_AMOUNT=$(BENCH_FUND_AMOUNT) \
	-e TRANSACTION_AMOUNT=$(BENCH_TRANSACTION_AMOUNT) \
	-e TPS=$(BENCH_TPS) \
	-e DURATION=$(BENCH_DURATION) \
	-e PRE_VUS=$(BENCH_PRE_VUS) \
	-e MAX_VUS=$(BENCH_MAX_VUS) \
	003_transactions.js )
	@echo "[ok] k6 transactions complete (profile=$(PROFILE))"

k6-run:
	$(call print_title,Running k6 benchmark [$(PROFILE)])
	$(call check_command,k6,"Install k6 with: brew install k6")
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_command,curl,"Install curl from https://curl.se/")
	@# ── run k6 suite ──────────────────────────────────────────────
	@if [ ! -d "$(K6_DIR)/$(K6_SCENARIO_DIR)" ]; then \
		echo "Error: k6 scenario directory not found: $(K6_DIR)/$(K6_SCENARIO_DIR)"; \
		exit 1; \
	fi
	@namespace="$(BENCH_NAMESPACE)"; \
	if [ -z "$$namespace" ]; then namespace="api_$$(date +%s)"; fi; \
	echo ""; \
	echo "Profile   = $(PROFILE)"; \
	echo "TPS       = $(BENCH_TPS)  Duration = $(BENCH_DURATION)"; \
	echo "VUs       = $(BENCH_PRE_VUS) pre / $(BENCH_MAX_VUS) max"; \
	echo "Namespace = $$namespace"; \
	echo ""; \
	( cd "$(K6_DIR)/$(K6_SCENARIO_DIR)" && \
	for kv in $$(env); do name=$${kv%%=*}; case "$$name" in K6_*) unset $$name ;; esac; done; \
	k6 run \
	-e ENVIRONMENT=$(BENCH_ENVIRONMENT) \
	-e AUTH_ENABLED=$(BENCH_AUTH_ENABLED) \
	-e LOG=$(BENCH_LOG) \
	-e BENCH_NAMESPACE=$$namespace \
	-e ORG_COUNT=$(BENCH_ORG_COUNT) \
	-e LEDGERS_PER_ORG=$(BENCH_LEDGERS_PER_ORG) \
	-e ACCOUNTS_PER_TYPE=$(BENCH_ACCOUNTS_PER_TYPE) \
	-e BOOTSTRAP_VUS=$(BENCH_BOOTSTRAP_VUS) \
	001_bootstrap_topology.js ) && \
	( cd "$(K6_DIR)/$(K6_SCENARIO_DIR)" && \
	for kv in $$(env); do name=$${kv%%=*}; case "$$name" in K6_*) unset $$name ;; esac; done; \
	k6 run \
	-e ENVIRONMENT=$(BENCH_ENVIRONMENT) \
	-e AUTH_ENABLED=$(BENCH_AUTH_ENABLED) \
	-e LOG=$(BENCH_LOG) \
	-e BENCH_NAMESPACE=$$namespace \
	-e ORG_COUNT=$(BENCH_ORG_COUNT) \
	-e LEDGERS_PER_ORG=$(BENCH_LEDGERS_PER_ORG) \
	-e ACCOUNTS_PER_TYPE=$(BENCH_ACCOUNTS_PER_TYPE) \
	-e FUND_AMOUNT=$(BENCH_FUND_AMOUNT) \
	-e FUND_VUS=$(BENCH_FUND_VUS) \
	002_fund_accounts.js ) && \
	( cd "$(K6_DIR)/$(K6_SCENARIO_DIR)" && \
	for kv in $$(env); do name=$${kv%%=*}; case "$$name" in K6_*) unset $$name ;; esac; done; \
	k6 run \
	-e ENVIRONMENT=$(BENCH_ENVIRONMENT) \
	-e AUTH_ENABLED=$(BENCH_AUTH_ENABLED) \
	-e LOG=$(BENCH_LOG) \
	-e BENCH_NAMESPACE=$$namespace \
	-e ORG_COUNT=$(BENCH_ORG_COUNT) \
	-e LEDGERS_PER_ORG=$(BENCH_LEDGERS_PER_ORG) \
	-e ACCOUNTS_PER_TYPE=$(BENCH_ACCOUNTS_PER_TYPE) \
	-e FUND_AMOUNT=$(BENCH_FUND_AMOUNT) \
	-e TRANSACTION_AMOUNT=$(BENCH_TRANSACTION_AMOUNT) \
	-e TPS=$(BENCH_TPS) \
	-e DURATION=$(BENCH_DURATION) \
	-e PRE_VUS=$(BENCH_PRE_VUS) \
	-e MAX_VUS=$(BENCH_MAX_VUS) \
	003_transactions.js )
	@echo "[ok] k6 benchmark complete (profile=$(PROFILE))"

k6-validate:
	@echo "Validating benchmark results..."
	@bash tests/k6/scripts/validate_bench.sh

k6-monitor:
	@echo "Monitoring consumer lag (Ctrl+C to stop)..."
	@bash tests/k6/scripts/monitor_lag.sh

bench-run: k6-run

bench-bootstrap: k6-bootstrap

bench-transactions: k6-transactions

bench-validate: k6-validate

bench-monitor: k6-monitor

# Component-specific command execution
.PHONY: infra onboarding transaction ledger all-components
infra:
	$(call print_title,"Running command in infra component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(INFRA_DIR) && $(MAKE) $(COMMAND)

onboarding:
	$(call print_title,"Running command in onboarding component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(ONBOARDING_DIR) && $(MAKE) $(COMMAND)

transaction:
	$(call print_title,"Running command in transaction component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(TRANSACTION_DIR) && $(MAKE) $(COMMAND)

ledger:
	$(call print_title,"Running command in ledger component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(LEDGER_DIR) && $(MAKE) $(COMMAND)

all-components:
	$(call print_title,"Running command across all components")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@for dir in $(COMPONENTS); do \
		echo "Running '$(COMMAND)' in $$dir..."; \
		(cd $$dir && $(MAKE) $(COMMAND)) || exit 1; \
	done
	@echo "[ok] Command '$(COMMAND)' executed successfully across all components"

#-------------------------------------------------------
# Development Commands
#-------------------------------------------------------

.PHONY: generate-docs
generate-docs:
	@./scripts/generate-docs.sh

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call print_title,"Setting up development environment for all components")
	@echo "Installing development tools..."
	@command -v gitleaks >/dev/null 2>&1 || (echo "Installing gitleaks..." && go install github.com/zricethezav/gitleaks/v8@latest) || echo "⚠️  Failed to install gitleaks"
	@command -v gofumpt >/dev/null 2>&1 || (echo "Installing gofumpt..." && go install mvdan.cc/gofumpt@latest) || echo "⚠️  Failed to install gofumpt"
	@command -v goimports >/dev/null 2>&1 || (echo "Installing goimports..." && go install golang.org/x/tools/cmd/goimports@latest) || echo "⚠️  Failed to install goimports"
	@echo "Setting up git hooks..."
	@$(MAKE) setup-git-hooks
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		echo "Setting up development environment for component: $$component_name"; \
		(cd $$dir && $(MAKE) dev-setup) || exit 1; \
		echo ""; \
	done
	@echo "[ok] Development environment set up successfully for all components"


.PHONY: grpc-gen
grpc-gen:
	@protoc --proto_path=./pkg/mgrpc --go-grpc_out=./pkg/mgrpc --go_out=./pkg/mgrpc ./pkg/mgrpc/balance/balance.proto

#-------------------------------------------------------
# Migration Commands
#-------------------------------------------------------

.PHONY: migrate-lint migrate-create

migrate-lint:
	$(call print_title,"Linting database migrations")
	@go build -o ./bin/migration-lint ./scripts/migration_linter
	@echo "Checking onboarding migrations..."
	@./bin/migration-lint ./components/onboarding/migrations
	@echo ""
	@echo "Checking transaction migrations..."
	@./bin/migration-lint ./components/transaction/migrations
	@echo "[ok] All migrations passed validation"

migrate-create:
	$(call print_title,"Creating new migration")
	@if [ -z "$(COMPONENT)" ]; then \
		echo "Error: COMPONENT not specified."; \
		echo "Usage: make migrate-create COMPONENT=<onboarding|transaction> NAME=<migration_name>"; \
		exit 1; \
	fi
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME not specified."; \
		echo "Usage: make migrate-create COMPONENT=<onboarding|transaction> NAME=<migration_name>"; \
		exit 1; \
	fi
	$(call check_command,migrate,"Install from https://github.com/golang-migrate/migrate")
	@migrate create -ext sql -dir ./components/$(COMPONENT)/migrations -seq $(NAME)
	@echo "[ok] Migration files created"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Edit the .up.sql file with your changes"
	@echo "  2. Edit the .down.sql file with the rollback"
	@echo "  3. Run 'make migrate-lint' to validate"
	@echo "  4. Follow the guidelines in scripts/migration_linter/docs/MIGRATION_GUIDELINES.md"
