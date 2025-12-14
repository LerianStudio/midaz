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
CONSOLE_DIR := ./components/console
CRM_DIR := ./components/crm
TESTS_DIR := ./tests

# Define component groups for easier management
BACKEND_COMPONENTS := $(ONBOARDING_DIR) $(TRANSACTION_DIR) $(CRM_DIR)

# Define a list of all component directories for easier iteration
COMPONENTS := $(INFRA_DIR) $(ONBOARDING_DIR) $(TRANSACTION_DIR) $(CONSOLE_DIR) $(CRM_DIR)

# Include shared utility functions
# Define common utility functions
define print_title
	@echo ""
	@echo "------------------------------------------"
	@echo "   📝 $(1)  "
	@echo "------------------------------------------"
endef

# Check if a command is available
define check_command
	@which $(1) > /dev/null || (echo "Error: $(1) is required but not installed. $(2)" && exit 1)
endef

# Check if environment files exist
define check_env_files
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/.env.example" ] && [ ! -f "$$dir/.env" ]; then \
			echo "Warning: $$dir/.env file is missing. Consider running 'make set-env'."; \
		fi; \
	done
endef

# Shell utility functions
define print_logo
	@cat $(PWD)/pkg/shell/logo.txt
endef

# Check if a command exists
define check_command
	@if ! command -v $(1) >/dev/null 2>&1; then \
		echo "Error: $(1) is not installed"; \
		echo "To install: $(2)"; \
		exit 1; \
	fi
endef

# Check if environment files exist
define check_env_files
	@missing=false; \
	for dir in $(COMPONENTS); do \
		if [ ! -f "$$dir/.env" ]; then \
			missing=true; \
			break; \
		fi; \
	done; \
	if [ "$$missing" = "true" ]; then \
		echo "Environment files are missing. Running set-env command first..."; \
		$(MAKE) set-env; \
	fi
endef

# Choose docker compose command depending on installed version
DOCKER_CMD := $(shell if docker compose version >/dev/null 2>&1; then echo "docker compose"; else echo "docker-compose"; fi)
export DOCKER_CMD

MK_DIR := $(abspath mk)

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
	@echo "  make sec                         - Run security checks using gosec"
	@echo "  make panicguard                  - Run panic hardening linter (golangci-lint plugin)"
	@echo "  make panicguard-standalone       - Run panic hardening linter (standalone)"
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
	@echo "  make up                           - Start all services with Docker Compose"
	@echo "  make down                         - Stop all services with Docker Compose"
	@echo "  make start                        - Start all containers"
	@echo "  make stop                         - Stop all containers"
	@echo "  make restart                      - Restart all containers"
	@echo "  make rebuild-up                   - Rebuild and restart all services"
	@echo "  make clean-docker                 - Clean all Docker resources (containers, networks, volumes)"
	@echo "  make logs                         - Show logs for all services"
	@echo "  make infra COMMAND=<cmd>          - Run command in infra component"
	@echo "  make onboarding COMMAND=<cmd>     - Run command in onboarding component"
	@echo "  make transaction COMMAND=<cmd>    - Run command in transaction component"
	@echo "  make console COMMAND=<cmd>        - Run command in console component"
	@echo "  make all-components COMMAND=<cmd> - Run command across all components"
	@echo "  make up-backend                   - Start only backend services (onboarding, transaction and crm)"
	@echo "  make down-backend                 - Stop only backend services (onboarding, transaction and crm)"
	@echo "  make restart-backend              - Restart only backend services (onboarding, transaction and crm)"
	@echo "  make up-unified-backend           - Start unified ledger service (onboarding + transaction in one process)"
	@echo "  make down-unified-backend         - Stop unified ledger service"
	@echo "  make restart-unified-backend      - Restart unified ledger service"
	@echo "  make ledger COMMAND=<cmd>         - Run command in ledger component"
	@echo ""
	@echo ""
	@echo "Documentation Commands:"
	@echo "  make generate-docs               - Generate Swagger documentation for all services"
	@echo ""
	@echo ""
	@echo "API Testing Commands:"
	@echo "  make newman                      - Run complete API workflow tests with Newman"
	@echo "  make newman-install              - Install Newman CLI and reporters globally"
	@echo "  make newman-env-check            - Verify environment file exists"
	@echo ""
	@echo ""
	@echo "Migration Commands:"
	@echo "  make migrate-lint                - Lint all migrations for dangerous patterns"
	@echo "  make migrate-create              - Create new migration files (requires COMPONENT, NAME)"
	@echo ""
	@echo ""
	@echo "Test Suite Aliases:"
	@echo "  make test-unit                   - Run Go unit tests"
	@echo "  make test-integration            - Run Go integration tests (requires Docker stack)"
	@echo "  make test-integ                  - Run integration tests with testcontainers (no stack needed)"
	@echo "  make test-e2e                    - Run Apidog E2E tests"
	@echo "  make test-fuzzy                  - Run fuzz/robustness tests"
	@echo "  make test-fuzz-engine            - Run go fuzz engine on fuzzy tests"
	@echo "  make test-chaos                  - Run chaos/resilience tests"
	@echo "  make test-property               - Run property-based tests"
	@echo ""
	@echo "Coverage Commands:"
	@echo "  make coverage-unit               - Run unit tests and generate coverage report"
	@echo "  make coverage-integ              - Run testcontainers tests and generate coverage"
	@echo "  make coverage-combined           - Run unit + integ tests with merged coverage"
	@echo ""
	@echo ""
	@echo "Test Parameters (env vars for test-* targets):"
	@echo "  START_LOCAL_DOCKER            - 0|1 (default: $(START_LOCAL_DOCKER))"
	@echo "  TEST_ONBOARDING_URL           - default: $(TEST_ONBOARDING_URL)"
	@echo "  TEST_TRANSACTION_URL          - default: $(TEST_TRANSACTION_URL)"
	@echo "  TEST_HEALTH_WAIT              - default: $(TEST_HEALTH_WAIT)"
	@echo "  TEST_FUZZTIME                 - default: $(TEST_FUZZTIME)"
	@echo "  TEST_AUTH_URL                 - default: $(TEST_AUTH_URL)"
	@echo "  TEST_AUTH_USERNAME            - default: $(TEST_AUTH_USERNAME)"
	@sh -c 'if [ -n "$(TEST_AUTH_PASSWORD)" ]; then echo "  TEST_AUTH_PASSWORD            - [set]"; else echo "  TEST_AUTH_PASSWORD            - [unset]"; fi'
	@echo "  TEST_PARALLEL                 - go test -parallel (default: $(TEST_PARALLEL))"
	@echo "  TEST_GOMAXPROCS               - env GOMAXPROCS (default: $(TEST_GOMAXPROCS))"
	@echo "  RETRY_ON_FAIL                 - 0|1 (default: $(RETRY_ON_FAIL))"
	@echo ""
	@echo "Target usage (which vars each target honors):"
	@echo "  test-integration: START_LOCAL_DOCKER, TEST_ONBOARDING_URL, TEST_TRANSACTION_URL, TEST_AUTH_URL, TEST_AUTH_USERNAME, TEST_AUTH_PASSWORD"
	@echo "  test-fuzzy:       START_LOCAL_DOCKER, TEST_ONBOARDING_URL, TEST_TRANSACTION_URL, TEST_AUTH_URL, TEST_AUTH_USERNAME, TEST_AUTH_PASSWORD"
	@echo "  test-fuzz-engine: START_LOCAL_DOCKER, TEST_ONBOARDING_URL, TEST_TRANSACTION_URL, TEST_AUTH_URL, TEST_AUTH_USERNAME, TEST_AUTH_PASSWORD, TEST_FUZZTIME, TEST_PARALLEL, TEST_GOMAXPROCS"
	@echo "  test-chaos:       START_LOCAL_DOCKER, TEST_ONBOARDING_URL, TEST_TRANSACTION_URL, TEST_AUTH_URL, TEST_AUTH_USERNAME, TEST_AUTH_PASSWORD"

 

.PHONY: build
build:
	$(call print_title,Building all components)
	@for dir in $(COMPONENTS); do \
		echo "Building in $$dir..."; \
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
# Backend Commands
#-------------------------------------------------------

.PHONY: up-backend
up-backend:
	$(call print_title,Starting backend services)
	$(call check_env_files)
	@echo "Starting infrastructure services first..."
	@cd $(INFRA_DIR) && $(MAKE) up
	@echo "Starting backend components..."
	@for dir in $(BACKEND_COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Starting services in $$dir..."; \
			(cd $$dir && $(MAKE) up) || exit 1; \
		fi \
	done
	@echo "[ok] Backend services started successfully ✔️"

.PHONY: down-backend
down-backend:
	$(call print_title,Stopping backend services)
	@echo "Stopping backend components..."
	@for dir in $(BACKEND_COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Stopping services in $$dir..."; \
			(cd $$dir && $(MAKE) down) || exit 1; \
		fi \
	done
	@echo "Stopping infrastructure services..."
	@cd $(INFRA_DIR) && $(MAKE) down
	@echo "[ok] Backend services stopped successfully ✔️"

.PHONY: restart-backend
restart-backend:
	$(call print_title,Restarting backend services)
	@make down-backend && make up-backend
	@echo "[ok] Backend services restarted successfully ✔️"

#-------------------------------------------------------
# Unified Backend Commands
#-------------------------------------------------------

.PHONY: up-unified-backend
up-unified-backend:
	$(call print_title,Starting unified backend service)
	$(call check_env_files)
	@echo "Starting infrastructure services first..."
	@cd $(INFRA_DIR) && $(MAKE) up
	@echo "Starting unified backend (onboarding + transaction)..."
	@cd $(LEDGER_DIR) && $(MAKE) up
	@echo "[ok] Unified backend service started successfully ✔️"

.PHONY: down-unified-backend
down-unified-backend:
	$(call print_title,Stopping unified backend service)
	@echo "Stopping unified backend..."
	@cd $(LEDGER_DIR) && $(MAKE) down
	@echo "Stopping infrastructure services..."
	@cd $(INFRA_DIR) && $(MAKE) down
	@echo "[ok] Unified backend service stopped successfully ✔️"

.PHONY: restart-unified-backend
restart-unified-backend:
	$(call print_title,Restarting unified backend service)
	@make down-unified-backend && make up-unified-backend
	@echo "[ok] Unified backend service restarted successfully ✔️"

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
			(cd $(TESTS_DIR) && golangci-lint run --fix ./...) || exit 1; \
		else \
			echo "No Go files found in $(TESTS_DIR), skipping linting"; \
		fi; \
	else \
		echo "No tests directory found at $(TESTS_DIR), skipping linting"; \
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

.PHONY: sec
sec:
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

.PHONY: panicguard
panicguard:
	$(call print_title,Running panic hardening linter)
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found, installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.7.2; \
	fi
	@if [ ! -f ./custom-gcl ]; then \
		echo "Building custom golangci-lint with panicguard plugins..."; \
		golangci-lint custom; \
	fi
	@echo "Running panicguard linters..."
	@./custom-gcl run --enable-only panicguard,panicguardwarn --timeout 5m ./components/... ./pkg/...
	@echo "[ok] Panic hardening linter completed"

.PHONY: panicguard-standalone
panicguard-standalone:
	$(call print_title,Running standalone panicguard linter)
	@go run ./cmd/panicguard/main.go ./components/... ./pkg/...
	@echo "[ok] Standalone panicguard linter completed"

#-------------------------------------------------------
# Git Hook Commands
#-------------------------------------------------------

.PHONY: setup-git-hooks
setup-git-hooks:
	$(call print_title,Installing and configuring git hooks)
	@sh ./scripts/setup-git-hooks.sh
	@echo "[ok] Git hooks installed successfully"

.PHONY: check-hooks
check-hooks:
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
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Starting services in $$dir..."; \
			(cd $$dir && $(MAKE) up) || exit 1; \
		fi; \
	done
	@echo "[ok] All services started successfully"

.PHONY: down
down:
	$(call print_title,Stopping all services with Docker Compose)
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Stopping services in component: $$component_name"; \
			(cd $$dir && ($(DOCKER_CMD) -f docker-compose.yml down 2>/dev/null || $(DOCKER_CMD) -f docker-compose.yml down)) || exit 1; \
		else \
			echo "No docker-compose.yml found in $$component_name, skipping"; \
		fi; \
	done
	@echo "[ok] All services stopped successfully"

.PHONY: start
start:
	$(call print_title,Starting all containers)
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Starting containers in $$dir..."; \
			(cd $$dir && $(MAKE) start) || exit 1; \
		fi; \
	done
	@echo "[ok] All containers started successfully"

.PHONY: stop
stop:
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Stopping containers in $$dir..."; \
			(cd $$dir && $(MAKE) stop) || exit 1; \
		fi; \
	done
	@echo "[ok] All containers stopped successfully"

.PHONY: restart
restart:
	@make stop && make start
	@echo "[ok] All containers restarted successfully"

.PHONY: rebuild-up
rebuild-up:
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Rebuilding and restarting services in $$dir..."; \
			(cd $$dir && $(MAKE) rebuild-up) || exit 1; \
		fi; \
	done
	@echo "[ok] All services rebuilt and restarted successfully"

.PHONY: clean-docker
clean-docker:
	$(call print_title,"Cleaning all Docker resources")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Cleaning Docker resources in $$dir..."; \
			(cd $$dir && $(MAKE) clean-docker) || exit 1; \
		fi; \
	done
	@echo "Pruning system-wide Docker resources..."
	@docker system prune -f
	@echo "Pruning system-wide Docker volumes..."
	@docker volume prune -f
	@echo "[ok] All Docker resources cleaned successfully"

.PHONY: logs
logs:
	$(call print_title,"Showing logs for all services")
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Logs for component: $$component_name"; \
			(cd $$dir && ($(DOCKER_CMD) -f docker-compose.yml logs --tail=50 2>/dev/null || $(DOCKER_CMD) -f docker-compose.yml logs --tail=50)) || exit 1; \
			echo ""; \
		fi; \
	done

# Component-specific command execution
.PHONY: infra onboarding transaction ledger console all-components
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

console:
	$(call print_title,"Running command in console component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(CONSOLE_DIR) && $(MAKE) $(COMMAND)

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
# Newman / API Testing Commands
#-------------------------------------------------------

.PHONY: newman newman-install newman-env-check

# Install Newman globally if not already installed
newman-install:
	$(call print_title,"Installing Newman CLI")
	@if ! command -v newman >/dev/null 2>&1; then \
		echo "📦 Newman not found. Installing globally..."; \
		npm install -g newman newman-reporter-html newman-reporter-htmlextra; \
		echo "✅ Newman installed successfully"; \
	else \
		echo "✅ Newman already installed: $$(newman --version)"; \
	fi

# Check environment file exists and has required variables
newman-env-check:
	@if [ ! -f "./postman/MIDAZ.postman_environment.json" ]; then \
		echo "❌ Environment file not found: ./postman/MIDAZ.postman_environment.json"; \
		echo "💡 Run 'make generate-docs' first to create the environment file"; \
		exit 1; \
	fi
	@echo "✅ Environment file found: ./postman/MIDAZ.postman_environment.json"

# Main Newman target - runs the complete API workflow (65 steps)
newman: newman-install newman-env-check
	$(call print_title,"Running Complete API Workflow with Newman")
	@if [ ! -f "./postman/MIDAZ.postman_collection.json" ]; then \
		echo "❌ Collection file not found. Running documentation generation first..."; \
		$(MAKE) generate-docs; \
	fi
	@echo "🚀 Starting complete API workflow test (65 steps)..."
	@mkdir -p ./reports/newman
	newman run "./postman/MIDAZ.postman_collection.json" \
		--environment "./postman/MIDAZ.postman_environment.json" \
		--folder "Complete API Workflow" \
		--reporters cli,json \
		--reporter-json-export "./reports/newman/workflow-json.json" \
		--timeout-request 30000 \
		--timeout-script 10000 \
		--delay-request 100 \
		--color on \
		--verbose
	@echo ""
	@echo "📊 Test Reports Generated:"
	@echo "  - CLI Summary: displayed above"
	@echo "  - JSON Report: ./reports/newman/workflow-json.json"

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call print_title,"Setting up development environment for all components")
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
