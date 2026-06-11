# Project Root Makefile.
# Coordinates all component Makefiles and provides centralized commands.
# Midaz Project Management.

# Define the root directory of the project
MIDAZ_ROOT := $(shell pwd)

# Component directories
INFRA_DIR := ./components/infra
LEDGER_DIR := ./components/ledger
TRACER_DIR := ./components/tracer
REPORTER_DIR := ./components/reporter
TESTS_DIR := ./tests
PKG_DIR := ./pkg

# The Go deploy units — the SINGLE list every fan-out target iterates.
# ledger absorbs crm + fees (collapsed), so they are NOT separate units.
# Adding a future component means editing ONLY this list, nothing else.
# Infra is config-only (no Go build, no image) and is sequenced separately
# by the service-lifecycle targets.
GO_COMPONENTS := $(LEDGER_DIR) $(TRACER_DIR) $(REPORTER_DIR)

# Pinned tool versions — single source of truth (P8-T01).
# Keep in sync with .github/workflows/go-combined-analysis.yml.
GO_VERSION := 1.26.4
GOLANGCI_LINT_VERSION := v2.12.2

# Shared color/title vocabulary + docker-compose detection.
MK_DIR := $(abspath mk)
include $(MK_DIR)/colors.mk
include $(MK_DIR)/utils.mk

# Legacy banner macro retained for the root targets and mk/tests.mk, which
# call $(print_title). New shared fragments use $(title1) from mk/utils.mk.
define print_title
	@echo ""
	@echo "------------------------------------------"
	@echo "   📝 $(1)  "
	@echo "------------------------------------------"
endef

# Check if a command is available
define check_command
	@if ! command -v $(1) >/dev/null 2>&1; then \
		echo "Error: $(1) is not installed"; \
		echo "To install: $(2)"; \
		exit 1; \
	fi
endef

# Warn about any missing .env files across all components + infra
define check_env_files
	@for dir in $(INFRA_DIR) $(GO_COMPONENTS); do \
		if [ -f "$$dir/.env.example" ] && [ ! -f "$$dir/.env" ]; then \
			echo "Warning: $$dir/.env file is missing. Consider running 'make set-env'."; \
		fi; \
	done
endef

# Shell utility functions
define print_logo
	@cat $(PWD)/pkg/shell/logo.txt
endef

COVERAGE_PACKAGES := ./...
include $(MK_DIR)/coverage-unit.mk
include $(MK_DIR)/tests.mk
include $(MK_DIR)/security.mk
include $(MK_DIR)/proto.mk

# The root security scan covers the whole monorepo source, not a single module.
SEC_SCAN_PATHS := ./components/... ./pkg/...

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
	@echo "  make up                           - Start all services (infra first, then components)"
	@echo "  make down                         - Stop all services (components first, then infra)"
	@echo "  make start                        - Start all containers"
	@echo "  make stop                         - Stop all containers"
	@echo "  make restart                      - Restart all containers"
	@echo "  make rebuild-up                   - Rebuild and restart all services"
	@echo "  make wait-for-infra               - Block until infra services report healthy"
	@echo "  make clean-docker                 - Clean all Docker resources (containers, networks, volumes)"
	@echo "  make logs                         - Show logs for all services"
	@echo "  make infra COMMAND=<cmd>          - Run command in infra component"
	@echo "  make ledger COMMAND=<cmd>         - Run command in ledger component"
	@echo "  make tracer COMMAND=<cmd>         - Run command in tracer component"
	@echo "  make reporter COMMAND=<cmd>       - Run command in reporter component (unified api+worker)"
	@echo "  make all-components COMMAND=<cmd> - Run command across all components"
	@echo ""
	@echo ""
	@echo "Documentation Commands:"
	@echo "  make generate-docs               - Generate Swagger documentation for all services"
	@echo "  make check-docs                  - Verify OpenAPI spec metadata parity (CHECK_DOCS_REGEN=1 also checks drift)"
	@echo ""
	@echo ""
	@echo "Protobuf Commands:"
	@echo "  make proto                       - Regenerate gRPC stubs from proto/ into pkg/proto/"
	@echo "  make proto-lint                  - Lint protobuf sources (buf lint)"
	@echo "  make proto-check                 - Regenerate and fail if committed stubs are stale (CI gate)"
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
	@echo "  TEST_LEDGER_URL               - default: $(TEST_LEDGER_URL)"
	@echo "  TEST_HEALTH_WAIT              - default: $(TEST_HEALTH_WAIT)"
	@echo "  TEST_AUTH_URL                 - default: $(TEST_AUTH_URL)"
	@echo "  TEST_AUTH_USERNAME            - default: $(TEST_AUTH_USERNAME)"
	@sh -c 'if [ -n "$(TEST_AUTH_PASSWORD)" ]; then echo "  TEST_AUTH_PASSWORD            - [set]"; else echo "  TEST_AUTH_PASSWORD            - [unset]"; fi'
	@echo "  LOW_RESOURCE                  - 0|1 (default: 0) - reduces parallelism for CI"
	@echo "  RETRY_ON_FAIL                 - 0|1 (default: $(RETRY_ON_FAIL))"
	@echo ""
	@echo "Target usage (which vars each target honors):"
	@echo "  test-integration:  PKG, RUN, CHAOS=1, LOW_RESOURCE (testcontainers-based, no external services needed)"
	@echo "  test-chaos-system: TEST_LEDGER_URL, TEST_AUTH_* (starts full stack)"
	@echo "  test-fuzz:         FUZZ, FUZZTIME (native Go fuzz testing)"
	@echo "  test-bench:        BENCH, BENCH_PKG (benchmark pattern and package filter)"

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call print_title,Building all components)
	@for dir in $(GO_COMPONENTS); do \
		echo "Building $$(basename $$dir)..."; \
		(cd $$dir && $(MAKE) build) || exit 1; \
	done
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
	@for dir in $(GO_COMPONENTS); do \
		echo "Checking for Go files in $$dir..."; \
		if find "$$dir" -name "*.go" -type f | grep -q .; then \
			echo "Linting in $$dir..."; \
			(cd $$dir && $(MAKE) lint) || exit 1; \
		else \
			echo "No Go files found in $$dir, skipping linting"; \
		fi; \
	done
	@echo "Checking for Go files in $(TESTS_DIR)..."
	@if [ -d "$(TESTS_DIR)" ]; then \
		if find "$(TESTS_DIR)" -name "*.go" -type f | grep -q .; then \
			echo "Linting in $(TESTS_DIR)..."; \
			(cd $(TESTS_DIR) && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --build-tags=integration ./... --verbose) || exit 1; \
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
			(cd $(PKG_DIR) && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./... --verbose) || exit 1; \
		else \
			echo "No Go files found in $(PKG_DIR), skipping linting"; \
		fi; \
	else \
		echo "No pkg directory found at $(PKG_DIR), skipping linting"; \
	fi
	@echo "[ok] Linting completed successfully"

# lint-fix runs golangci-lint with --fix across the same scope as `lint`.
# This MUTATES source — it is a developer convenience, NOT a gate. The `lint`
# target above is read-only so a clean checkout is verified without mutation.
.PHONY: lint-fix
lint-fix:
	$(call print_title,Applying lint autofixes on all components)
	@for dir in $(GO_COMPONENTS); do \
		echo "Checking for Go files in $$dir..."; \
		if find "$$dir" -name "*.go" -type f | grep -q .; then \
			echo "Fixing in $$dir..."; \
			(cd $$dir && $(MAKE) lint-fix) || exit 1; \
		else \
			echo "No Go files found in $$dir, skipping"; \
		fi; \
	done
	@echo "Checking for Go files in $(TESTS_DIR)..."
	@if [ -d "$(TESTS_DIR)" ] && find "$(TESTS_DIR)" -name "*.go" -type f | grep -q .; then \
		echo "Fixing in $(TESTS_DIR)..."; \
		(cd $(TESTS_DIR) && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --fix --build-tags=integration ./... --verbose) || exit 1; \
	else \
		echo "No Go files found in $(TESTS_DIR), skipping"; \
	fi
	@echo "Checking for Go files in $(PKG_DIR)..."
	@if [ -d "$(PKG_DIR)" ] && find "$(PKG_DIR)" -name "*.go" -type f | grep -q .; then \
		echo "Fixing in $(PKG_DIR)..."; \
		(cd $(PKG_DIR) && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --fix ./... --verbose) || exit 1; \
	else \
		echo "No Go files found in $(PKG_DIR), skipping"; \
	fi
	@echo "[ok] Lint autofixes applied"

.PHONY: format
format:
	$(call print_title,Formatting code in all components)
	@for dir in $(GO_COMPONENTS); do \
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

# Telemetry/observability enforcement gates (rg-based). Each gate prints any
# violations and the whole target exits non-zero if ANY gate fires. Scope is
# production source only (non-test, non-mock) under components/ and pkg/.
# Gates encode the logging/observability rules in CLAUDE.md + docs/standards/telemetry.md:
#   a. no fmt.Sprintf as the log MESSAGE arg (structured fields may still format
#      values, e.g. libLog.String("type", fmt.Sprintf("%T", x)))
#   b. no nil-Redactor span dumps via SetSpanAttributesFromValue (must use explicit attrs)
#   c. no prefixed wire codes (FEE/TRC/TPL/REP-NNNN) — the canonical numeric registry is the only code family
#   d. no per-request Info narration (boot-time context.Background() milestones are allowed)
#   e. no reflect.TypeOf(mmodel.*) for entity names — use constant.Entity* (comments excepted)
.PHONY: check-telemetry
check-telemetry:
	$(call print_title,Verifying telemetry/observability gates)
	$(call check_command,rg,"Install ripgrep from https://github.com/BurntSushi/ripgrep")
	@FAILED=0; \
	run_gate() { \
		desc="$$1"; shift; \
		hits=$$("$$@" 2>/dev/null); \
		if [ -n "$$hits" ]; then \
			echo "[FAIL] $$desc"; \
			echo "$$hits" | sed 's/^/    /'; \
			echo ""; \
			FAILED=1; \
		else \
			echo "[ok] $$desc"; \
		fi; \
	}; \
	run_gate "no fmt.Sprintf as a log message argument" \
		rg -n 'logger\.(Log|Info|Warn|Error|Debug|Fatal)\([^"]*(Level[A-Za-z]+|"[a-z]+"),\s*fmt\.Sprint' --glob '*.go' --glob '!*_test.go' --glob '!*mock*' components/ pkg/; \
	run_gate "no nil-Redactor span dumps (SetSpanAttributesFromValue)" \
		rg -Un 'SetSpanAttributesFromValue\(' --glob '*.go' --glob '!*_test.go' --glob '!*mock*' components/ pkg/; \
	run_gate "no prefixed wire codes (FEE/TRC/TPL/REP-NNNN)" \
		rg -n '"(FEE|TRC|TPL|REP)-[0-9]' --glob '*.go' --glob '!*_test.go' components/ pkg/; \
	run_gate "no per-request Info narration" \
		rg -nP '^(?!.*context\.Background\(\)).*(LevelInfo|\.Info\().*"(Initiating|Retrieving|Trying to|Successfully)' --glob '*.go' --glob '!*_test.go' components/ pkg/; \
	run_gate "no reflect.TypeOf(mmodel.*) for entity names" \
		rg -nP '^(?:[^/]|/(?!/))*reflect\.TypeOf\(mmodel\.' --glob '!*_test.go' --glob '!*mock*' components/ pkg/; \
	if [ "$$FAILED" -ne 0 ]; then \
		echo "[error] One or more telemetry gates failed"; \
		exit 1; \
	fi
	@echo "[ok] Telemetry gate verification completed"

# Top-level CI gate — static checks + fast unit tests, one command, one exit code.
# Sequences the deterministic gates that need no live stack: lint (all components
# + tests/ + pkg/), the telemetry/observability gates, then the race-enabled unit
# suite (test-unit already exports ALLOW_INSECURE_TLS=true). Each leg is a separate
# $(MAKE) under `set -e`, so the first failure aborts and `make ci` exits non-zero.
# For the heavier integration/property/chaos matrix run `make ci-tests`.
.PHONY: ci
ci:
	$(call print_title,Running CI gate (lint + telemetry + unit tests))
	@set -e; \
	$(MAKE) lint; \
	$(MAKE) check-telemetry; \
	$(MAKE) test-unit
	@echo "[ok] CI gate completed successfully"

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
	@for dir in $(INFRA_DIR) $(GO_COMPONENTS); do \
		if [ -f "$$dir/.env.example" ] && [ ! -f "$$dir/.env" ]; then \
			echo "Creating .env in $$dir from .env.example"; \
			cp "$$dir/.env.example" "$$dir/.env"; \
		elif [ ! -f "$$dir/.env.example" ]; then \
			echo "Warning: No .env.example found in $$dir"; \
		else \
			echo ".env already exists in $$dir"; \
		fi; \
	done
	@# Generate the collapsed holder-crypto keys into the ledger .env (migrated
	@# from the standalone holder service at COLLAPSE time — keys keep the bare
	@# LCRYPTO_* names the ledger binary reads).
	@if [ -f "$(LEDGER_DIR)/.env" ]; then \
		$(MAKE) -C $(LEDGER_DIR) generate-keys; \
	fi
	@echo "[ok] Environment files set up successfully"

.PHONY: clear-envs
clear-envs:
	$(call print_title,Removing environment files)
	@for dir in $(INFRA_DIR) $(GO_COMPONENTS); do \
		if [ -f "$$dir/.env" ]; then \
			echo "Removing .env in $$dir"; \
			rm "$$dir/.env"; \
		else \
			echo "No .env found in $$dir"; \
		fi; \
	done
	@echo "[ok] Environment files removed successfully"

#-------------------------------------------------------
# Service Commands
#-------------------------------------------------------

# Block until the shared infra services report healthy. Adopted from
# reporter — midaz's cross-compose `depends_on` does not work across
# separate compose projects, so the components must wait explicitly.
# Covers the SeaweedFS object store and KEDA autoscaler added in P8-T09.
.PHONY: wait-for-infra
wait-for-infra:
	$(call print_title,Waiting for infrastructure services to be healthy)
	@echo "Waiting for Postgres / Mongo / Valkey / RabbitMQ / SeaweedFS ..."
	@timeout=120; elapsed=0; \
	services="midaz-postgres-primary midaz-mongodb midaz-valkey midaz-rabbitmq midaz-seaweedfs"; \
	for svc in $$services; do \
		printf "  %-28s" "$$svc"; \
		while true; do \
			cid=$$(docker ps --filter "name=$$svc" --format '{{.ID}}' 2>/dev/null | head -n1); \
			if [ -n "$$cid" ]; then \
				status=$$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$$cid" 2>/dev/null); \
				if [ "$$status" = "healthy" ] || [ "$$status" = "running" ]; then \
					echo "[ok] $$status"; \
					break; \
				fi; \
			fi; \
			if [ $$elapsed -ge $$timeout ]; then \
				echo "[timeout]"; \
				echo "[error] $$svc did not become healthy within $${timeout}s"; \
				exit 1; \
			fi; \
			sleep 2; elapsed=$$((elapsed + 2)); \
		done; \
	done
	@echo "[ok] Infrastructure services are healthy"

.PHONY: up
up:
	$(call print_title,Starting all services with Docker Compose)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@echo "Starting infrastructure services..."
	@(cd $(INFRA_DIR) && $(MAKE) up) || exit 1
	@$(MAKE) wait-for-infra
	@for dir in $(GO_COMPONENTS); do \
		echo "Starting $$(basename $$dir) service..."; \
		(cd $$dir && $(MAKE) up) || exit 1; \
	done
	@echo "[ok] All services started successfully"

.PHONY: down
down:
	$(call print_title,Stopping all services with Docker Compose)
	@for dir in $(GO_COMPONENTS); do \
		echo "Stopping $$(basename $$dir) service..."; \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			(cd $$dir && $(DOCKER_CMD) -f docker-compose.yml down 2>/dev/null) || (cd $$dir && $(DOCKER_CMD) -f docker-compose.yml down); \
		fi; \
	done
	@echo "Stopping infrastructure services..."
	@if [ -f "$(INFRA_DIR)/docker-compose.yml" ]; then \
		(cd $(INFRA_DIR) && $(DOCKER_CMD) -f docker-compose.yml down 2>/dev/null) || (cd $(INFRA_DIR) && $(DOCKER_CMD) -f docker-compose.yml down); \
	fi
	@echo "[ok] All services stopped successfully"

.PHONY: start
start:
	$(call print_title,Starting all containers)
	@echo "Starting infrastructure containers..."
	@(cd $(INFRA_DIR) && $(MAKE) start) || exit 1
	@$(MAKE) wait-for-infra
	@for dir in $(GO_COMPONENTS); do \
		echo "Starting $$(basename $$dir) containers..."; \
		(cd $$dir && $(MAKE) start) || exit 1; \
	done
	@echo "[ok] All containers started successfully"

.PHONY: stop
stop:
	$(call print_title,Stopping all containers)
	@for dir in $(GO_COMPONENTS); do \
		echo "Stopping $$(basename $$dir) containers..."; \
		(cd $$dir && $(MAKE) stop 2>/dev/null) || true; \
	done
	@echo "Stopping infrastructure containers..."
	@(cd $(INFRA_DIR) && $(MAKE) stop 2>/dev/null) || true
	@echo "[ok] All containers stopped successfully"

.PHONY: restart
restart:
	@$(MAKE) down && $(MAKE) up
	@echo "[ok] All containers restarted successfully"

.PHONY: rebuild-up
rebuild-up:
	$(call print_title,Rebuilding and restarting all services)
	@echo "Rebuilding infrastructure..."
	@(cd $(INFRA_DIR) && $(MAKE) rebuild-up) || exit 1
	@$(MAKE) wait-for-infra
	@for dir in $(GO_COMPONENTS); do \
		echo "Rebuilding $$(basename $$dir)..."; \
		(cd $$dir && $(MAKE) rebuild-up) || exit 1; \
	done
	@echo "[ok] All services rebuilt and restarted successfully"

.PHONY: clean-docker
clean-docker:
	$(call print_title,Cleaning all Docker resources)
	@for dir in $(GO_COMPONENTS); do \
		echo "Cleaning $$(basename $$dir) Docker resources..."; \
		(cd $$dir && $(MAKE) clean-docker 2>/dev/null) || true; \
	done
	@echo "Cleaning infrastructure Docker resources..."
	@(cd $(INFRA_DIR) && $(MAKE) clean-docker 2>/dev/null) || true
	@echo "Pruning system-wide Docker resources..."
	@docker system prune -f
	@echo "Pruning system-wide Docker volumes..."
	@docker volume prune -f
	@echo "[ok] All Docker resources cleaned successfully"

.PHONY: logs
logs:
	$(call print_title,Showing logs for all services)
	@echo "=== Infrastructure logs ==="
	@(cd $(INFRA_DIR) && $(DOCKER_CMD) -f docker-compose.yml logs --tail=50 2>/dev/null) || true
	@for dir in $(GO_COMPONENTS); do \
		echo ""; \
		echo "=== $$(basename $$dir) logs ==="; \
		(cd $$dir && $(DOCKER_CMD) -f docker-compose.yml logs --tail=50 2>/dev/null) || true; \
	done

# Component-specific command execution
.PHONY: infra ledger tracer reporter all-components
infra:
	$(call print_title,Running command in infra component)
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(INFRA_DIR) && $(MAKE) $(COMMAND)

ledger:
	$(call print_title,Running command in ledger component)
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(LEDGER_DIR) && $(MAKE) $(COMMAND)

tracer:
	$(call print_title,Running command in tracer component)
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(TRACER_DIR) && $(MAKE) $(COMMAND)

reporter:
	$(call print_title,Running command in reporter component)
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(REPORTER_DIR) && $(MAKE) $(COMMAND)

all-components:
	$(call print_title,Running command across all components)
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@for dir in $(GO_COMPONENTS); do \
		echo "Running '$(COMMAND)' in $$dir..."; \
		(cd $$dir && $(MAKE) $(COMMAND)) || exit 1; \
	done
	@echo "[ok] Command '$(COMMAND)' executed successfully across all components"

#-------------------------------------------------------
# Development Commands
#-------------------------------------------------------

.PHONY: generate-docs
generate-docs:
	@./postman/generator/generate-docs.sh

.PHONY: check-docs
check-docs:
	@./postman/generator/check-docs.sh

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call print_title,Setting up development environment for all components)
	@echo "Installing development tools..."
	@GOBIN="$$(go env GOPATH)/bin"; \
	check_or_install() { \
		local name="$$1" pkg="$$2"; \
		if command -v "$$name" >/dev/null 2>&1 || [ -x "$$GOBIN/$$name" ]; then \
			echo "  ✓ $$name already installed"; \
		else \
			echo "  ⏳ Installing $$name..."; \
			go install "$$pkg" || echo "  ⚠️  Failed to install $$name"; \
		fi; \
	}; \
	check_or_install gitleaks github.com/zricethezav/gitleaks/v8@latest; \
	check_or_install gofumpt mvdan.cc/gofumpt@latest; \
	check_or_install goimports golang.org/x/tools/cmd/goimports@latest; \
	check_or_install gosec github.com/securego/gosec/v2/cmd/gosec@latest; \
	check_or_install mockgen go.uber.org/mock/mockgen@v0.6.0; \
	check_or_install golangci-lint github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@GOBIN="$$(go env GOPATH)/bin"; \
	if ! echo "$$PATH" | tr ':' '\n' | grep -qx "$$GOBIN"; then \
		echo ""; \
		echo "  ⚠️  $$GOBIN is not in your PATH."; \
		echo "  Add to your shell profile: export PATH=\"\$$PATH:$$GOBIN\""; \
		echo "  Without this, git hooks (gofumpt, goimports) will be silently skipped."; \
		echo ""; \
	fi
	@echo "Setting up git hooks..."
	@$(MAKE) setup-git-hooks
	@for dir in $(GO_COMPONENTS); do \
		component_name=$$(basename $$dir); \
		echo "Setting up development environment for component: $$component_name"; \
		(cd $$dir && $(MAKE) dev-setup) || exit 1; \
		echo ""; \
	done
	@echo "[ok] Development environment set up successfully for all components"

#-------------------------------------------------------
# Migration Commands
#-------------------------------------------------------

.PHONY: migrate-lint migrate-create

migrate-lint:
	$(call print_title,Linting database migrations)
	@go build -o ./bin/migration-lint ./scripts/migration_linter
	@echo "Checking onboarding migrations..."
	@./bin/migration-lint ./components/ledger/migrations/onboarding
	@echo ""
	@echo "Checking transaction migrations..."
	@./bin/migration-lint ./components/ledger/migrations/transaction
	@echo "[ok] All migrations passed validation"

migrate-create:
	$(call print_title,Creating new migration)
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
	@migrate create -ext sql -dir ./components/ledger/migrations/$(COMPONENT) -seq $(NAME)
	@echo "[ok] Migration files created"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Edit the .up.sql file with your changes"
	@echo "  2. Edit the .down.sql file with the rollback"
	@echo "  3. Run 'make migrate-lint' to validate"
	@echo "  4. Follow the guidelines in scripts/migration_linter/docs/MIGRATION_GUIDELINES.md"
