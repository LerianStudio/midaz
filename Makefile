# Project Root Makefile.
# Coordinates all component Makefiles and provides centralized commands.
# Midaz Project Management.

# Define the root directory of the project
MIDAZ_ROOT := $(shell pwd)

# Component directories
INFRA_DIR := ./components/infra
LEDGER_DIR := ./components/ledger
CONSOLE_DIR := ./components/console
CRM_DIR := ./components/crm
TESTS_DIR := ./tests

# Define component groups for easier management
BACKEND_COMPONENTS := $(LEDGER_DIR)

# Define a list of all component directories for easier iteration
COMPONENTS := $(INFRA_DIR) $(LEDGER_DIR) $(CONSOLE_DIR) $(CRM_DIR)

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
include $(MK_DIR)/dev.mk

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
	@echo "Development Tools (mk/dev.mk):"
	@echo "  make dev-setup                   - Set up complete development environment (hooks + tools)"
	@echo "  make tools-dev                   - Install all development tools"
	@echo "  make pre-commit                  - Run pre-commit checks (format + lint)"
	@echo ""
	@echo ""
	@echo "Code Formatting:"
	@echo "  make format                      - Run all formatters (goimports + gofumpt)"
	@echo "  make gofumpt                     - Run gofumpt (stricter formatting)"
	@echo "  make goimports                   - Run goimports (organize imports)"
	@echo ""
	@echo ""
	@echo "Code Quality Commands:"
	@echo "  make lint                        - Run linters on all components"
	@echo "  make lint-fix                    - Run linters with auto-fix enabled"
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
	@echo "  make ledger COMMAND=<cmd>         - Run command in ledger component"
	@echo "  make console COMMAND=<cmd>        - Run command in console component"
	@echo "  make all-components COMMAND=<cmd> - Run command across all components"
	@echo "  make up-backend                   - Start only backend services (ledger and crm)"
	@echo "  make down-backend                 - Stop only backend services (ledger and crm)"
	@echo "  make restart-backend              - Restart only backend services (ledger and crm)"
	@echo ""
	@echo ""
	@echo "Documentation Commands:"
	@echo "  make generate-docs               - Generate Swagger documentation for all services"
	@echo ""
	@echo ""
	@echo "Test Suite Aliases:"
	@echo "  make test-unit                   - Run Go unit tests"
	@echo "  make test-integration            - Run Go integration tests"
	@echo "  make test-fuzzy                  - Run fuzz/robustness tests"
	@echo "  make test-fuzz-engine            - Run go fuzz engine on fuzzy tests"
	@echo "  make test-chaos                  - Run chaos/resilience tests"
	@echo "  make test-property               - Run property-based tests"
	@echo ""
	@echo ""
	@echo "Test Parameters (env vars for test-* targets):"
	@echo "  START_LOCAL_DOCKER            - 0|1 (default: $(START_LOCAL_DOCKER))"
	@echo "  TEST_ONBOARDING_URL           - default: $(TEST_ONBOARDING_URL)"
	@echo "  TEST_LEDGER_URL               - default: $(TEST_LEDGER_URL)"
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
	@echo "  test-integration: START_LOCAL_DOCKER, TEST_ONBOARDING_URL, TEST_LEDGER_URL, TEST_AUTH_URL, TEST_AUTH_USERNAME, TEST_AUTH_PASSWORD"
	@echo "  test-fuzzy:       START_LOCAL_DOCKER, TEST_ONBOARDING_URL, TEST_LEDGER_URL, TEST_AUTH_URL, TEST_AUTH_USERNAME, TEST_AUTH_PASSWORD"
	@echo "  test-fuzz-engine: START_LOCAL_DOCKER, TEST_ONBOARDING_URL, TEST_LEDGER_URL, TEST_AUTH_URL, TEST_AUTH_USERNAME, TEST_AUTH_PASSWORD, TEST_FUZZTIME, TEST_PARALLEL, TEST_GOMAXPROCS"
	@echo "  test-chaos:       START_LOCAL_DOCKER, TEST_ONBOARDING_URL, TEST_LEDGER_URL, TEST_AUTH_URL, TEST_AUTH_USERNAME, TEST_AUTH_PASSWORD"

 

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
.PHONY: infra ledger console all-components
infra:
	$(call print_title,"Running command in infra component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(INFRA_DIR) && $(MAKE) $(COMMAND)

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

