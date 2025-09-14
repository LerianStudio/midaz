# Project Root Makefile.
# Coordinates all component Makefiles and provides centralized commands.
# Midaz Project Management.

# Define the root directory of the project
MIDAZ_ROOT := $(shell pwd)

# Component directories
INFRA_DIR := ./components/infra
MDZ_DIR := ./components/mdz
ONBOARDING_DIR := ./components/onboarding
TRANSACTION_DIR := ./components/transaction
CONSOLE_DIR := ./components/console

# Define component groups for easier management
BACKEND_COMPONENTS := $(ONBOARDING_DIR) $(TRANSACTION_DIR)

# Define a list of all component directories for easier iteration
COMPONENTS := $(INFRA_DIR) $(MDZ_DIR) $(ONBOARDING_DIR) $(TRANSACTION_DIR) $(CONSOLE_DIR)

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

# ------------------------------------------------------
# Test configuration
# ------------------------------------------------------
TEST_ONBOARDING_URL ?= http://localhost:3000
TEST_TRANSACTION_URL ?= http://localhost:3001
TEST_HEALTH_WAIT ?= 60
TEST_FUZZTIME ?= 30s

define wait_for_services
	echo "Waiting for services to become healthy..."
	bash -c 'for i in $$(seq 1 $(TEST_HEALTH_WAIT)); do \
	  if curl -fsS $(TEST_ONBOARDING_URL)/health >/dev/null 2>&1 && curl -fsS $(TEST_TRANSACTION_URL)/health >/dev/null 2>&1; then \
	    echo "Services are up"; exit 0; \
	  fi; \
	  sleep 1; \
	done; echo "[error] Services not healthy after $(TEST_HEALTH_WAIT)s"; exit 1'
endef

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
	@echo "  make mdz COMMAND=<cmd>            - Run command in mdz component"
	@echo "  make onboarding COMMAND=<cmd>     - Run command in onboarding component"
	@echo "  make transaction COMMAND=<cmd>    - Run command in transaction component"
	@echo "  make console COMMAND=<cmd>        - Run command in console component"
	@echo "  make all-components COMMAND=<cmd> - Run command across all components"
	@echo "  make up-backend                   - Start only backend services (onboarding and transaction)"
	@echo "  make down-backend                 - Stop only backend services (onboarding and transaction)"
	@echo "  make restart-backend              - Restart only backend services (onboarding and transaction)"
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
	@echo "Test Suite Aliases:"
	@echo "  make test-unit                   - Run Go unit tests (exclude ./tests/**)"
	@echo "  make test-integration            - Run Go integration tests (brings up backend)"
	@echo "  make test-e2e                    - Run Go E2E tests (brings up backend)"
	@echo "  make test-fuzzy                  - Run fuzz/robustness tests (brings up backend)"
	@echo "  make test-fuzz-engine            - Run go fuzz engine on fuzzy tests (brings up backend)"
	@echo "  make test-chaos                  - Run chaos/resilience tests (brings up backend)"
	@echo "  make test-property               - Run property-based tests"
	@echo ""
	@echo ""

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: test
test:
	@./scripts/run-tests.sh

#-------------------------------------------------------
# Test Suite Aliases
#-------------------------------------------------------

# Unit tests (exclude ./tests/** packages)
.PHONY: test-unit
test-unit:
	$(call print_title,Running Go unit tests (excluding ./tests/**))
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@set -e; \
	pkgs=$$(go list ./... | rg -v '/tests(/|$$)'); \
	if [ -z "$$pkgs" ]; then \
	  echo "No unit test packages found (outside ./tests)**"; \
	else \
	  go test -v -race -count=1 $$pkgs; \
	fi

# Integration tests (Go) – spins up stack, runs tests/integration
.PHONY: test-integration
test-integration:
	$(call print_title,Running Go integration tests (with Docker stack))
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@set -e; \
	trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	$(MAKE) up-backend; \
	$(call wait_for_services); \
	ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) go test -v -race -count=1 ./tests/integration


# E2E tests (Go) – expects stack running; will bring it up if not
.PHONY: test-e2e-go
test-e2e-go:
	$(call print_title,Running Go E2E tests (with Docker stack))
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@set -e; \
	trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	$(MAKE) up-backend; \
	$(call wait_for_services); \
	ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) go test -v -race -count=1 ./tests/e2e

# Simple alias for the E2E suite
.PHONY: test-e2e
test-e2e: test-e2e-go

# Combined Go integration + E2E tests
.PHONY: test-integration-e2e
test-integration-e2e:
	$(call print_title,Running Go integration + E2E tests (with Docker stack))
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@set -e; \
	trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	$(MAKE) up-backend; \
	$(call wait_for_services); \
	ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) go test -v -race -count=1 ./tests/integration; \
	ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) go test -v -race -count=1 ./tests/e2e

# Property tests (model-level)
.PHONY: test-property
test-property:
	$(call print_title,Running property-based model tests)
	go test -v -race -failfast -timeout 120s -count=1 ./tests/property

# Chaos tests
.PHONY: test-chaos
test-chaos:
	$(call print_title,Running chaos tests - requires Docker stack)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@set -e; \
	trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	$(MAKE) up-backend; \
	$(call wait_for_services); \
	ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) go test -v -race -count=1 ./tests/chaos

# Fuzzy/robustness tests
.PHONY: test-fuzzy
test-fuzzy:
	$(call print_title,Running fuzz/robustness tests - requires Docker stack)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@set -e; \
	trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	$(MAKE) up-backend; \
	$(call wait_for_services); \
	ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) go test -v -race ./tests/fuzzy -count=1

# Fuzz engine run (uses Go's built-in fuzzing). Adjust TEST_FUZZTIME to control duration.
.PHONY: test-fuzz-engine
test-fuzz-engine:
	$(call print_title,Running Go fuzz engine on fuzzy tests - requires Docker stack)
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@set -e; \
	trap '$(MAKE) -s down-backend >/dev/null 2>&1 || true' EXIT; \
	$(MAKE) up-backend; \
	$(call wait_for_services); \
	ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) go test -v -race -fuzz=Fuzz -run=^$$ -fuzztime=$(TEST_FUZZTIME) ./tests/fuzzy

# Security tests (run only when auth plugin enabled)
.PHONY: test-security
test-security:
	$(call print_title,Running security tests (requires PLUGIN_AUTH_ENABLED=true))
	@echo "Note: set TEST_REQUIRE_AUTH=true and TEST_AUTH_HEADER=\"Bearer <token>\" when plugin is enabled."
	ONBOARDING_URL=$(TEST_ONBOARDING_URL) TRANSACTION_URL=$(TEST_TRANSACTION_URL) go test -v -race -count=1 ./tests/integration -run Security

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
	@echo "[ok] Environment files set up successfully"

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
.PHONY: infra mdz onboarding transaction console all-components
infra:
	$(call print_title,"Running command in infra component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(INFRA_DIR) && $(MAKE) $(COMMAND)

mdz:
	$(call print_title,"Running command in mdz component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(MDZ_DIR) && $(MAKE) $(COMMAND)

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
