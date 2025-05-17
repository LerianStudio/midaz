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
include $(MIDAZ_ROOT)/pkg/shell/makefile_utils.mk

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
	@echo "Demo Data Commands:"
	@echo "  make demo-data                   - Generate demo data with small volume"
	@echo "  make demo-data-medium            - Generate demo data with medium volume"
	@echo "  make demo-data-large             - Generate demo data with large volume"
	@echo "  make demo-data-test              - Run simplified tests for demo data generator"
	@echo "  make demo-data-test-full         - Run full Jest tests for demo data generator"
	@echo "  make demo-data-test-mode         - Test demo data generator in test mode"
	@echo ""
	@echo ""

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: test
test:
	$(call title1,"Running tests on all components")
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	$(call check_command,npm,"Install Node.js and npm from https://nodejs.org/")
	
	@echo "Starting tests at $$(date)"
	@start_time=$$(date +%s)
	@overall_exit_code=0
	
	@echo "\nRunning core Go tests..."
	@go test -v ./... || overall_exit_code=1
	
	@echo "\nRunning component tests..."
	
	@echo "\nTesting console component..."
	@if [ -d "components/console" ]; then \
		(cd components/console && $(MAKE) test) || overall_exit_code=1; \
	fi
	
	@echo "\nTesting mdz component..."
	@if [ -d "components/mdz" ]; then \
		(cd components/mdz && $(MAKE) test) || overall_exit_code=1; \
	fi
	
	@echo "\nTesting onboarding component..."
	@if [ -d "components/onboarding" ]; then \
		(cd components/onboarding && $(MAKE) test) || overall_exit_code=1; \
	fi
	
	@echo "\nTesting transaction component..."
	@if [ -d "components/transaction" ]; then \
		(cd components/transaction && $(MAKE) test) || overall_exit_code=1; \
	fi
	
	@end_time=$$(date +%s)
	@duration=$$((end_time - start_time))
	@echo "\nTest Summary:"
	@echo "----------------------------------------"
	@echo "Duration: $$(printf '%dm:%02ds' $$((duration / 60)) $$((duration % 60)))"
	@echo "----------------------------------------"
	
	@if [ "$$overall_exit_code" = "0" ]; then \
		echo "All tests passed successfully!"; \
		exit 0; \
	else \
		echo "Some tests failed. Please check the output above for details."; \
		exit 1; \
	fi

.PHONY: build
build:
	$(call title1,"Building all components")
	@for dir in $(COMPONENTS); do \
		echo "Building in $$dir..."; \
		(cd $$dir && $(MAKE) build) || exit 1; \
	done
	@echo "[ok] All components built successfully"

.PHONY: clean
clean:
	$(call title1,"Cleaning all build artifacts")
	@for dir in $(COMPONENTS); do \
		echo "Cleaning in $$dir..."; \
		(cd $$dir && $(MAKE) clean) || exit 1; \
		echo "Ensuring thorough cleanup in $$dir..."; \
		(cd $$dir && \
			for item in bin dist coverage.out coverage.html artifacts *.tmp node_modules; do \
				if [ -e "$$item" ]; then \
					echo "Removing $$dir/$$item"; \
					rm -rf "$$item"; \
				fi \
			done \
		) || true; \
	done
	@echo "Cleaning root-level build artifacts..."
	@for item in bin dist coverage.out coverage.html *.tmp node_modules; do \
		if [ -e "$$item" ]; then \
			echo "Removing $$item"; \
			rm -rf "$$item"; \
		fi \
	done
	@echo "Cleaning demo-data SDK..."
	@if [ -e "scripts/demo-data/sdk-source" ]; then \
		echo "Removing scripts/demo-data/sdk-source"; \
		rm -rf "scripts/demo-data/sdk-source"; \
	fi
	@if [ -e "scripts/demo-data/node_modules" ]; then \
		echo "Removing scripts/demo-data/node_modules"; \
		rm -rf "scripts/demo-data/node_modules"; \
	fi
	
	@echo "Deep cleaning project..."
	@echo "Finding and removing coverage.out files..."
	@find . -name "coverage.out" -type f -delete -print || true
	@echo "Finding and removing coverage.html files..."
	@find . -name "coverage.html" -type f -delete -print || true
	@echo "Finding and removing bin directories..."
	@find . -name "bin" -type d -exec rm -rf {} \; -prune -or -true | grep -v "Permission denied" || true
	@echo "Finding and removing dist directories..."
	@find . -name "dist" -type d -exec rm -rf {} \; -prune -or -true | grep -v "Permission denied" || true
	@echo "Finding and removing node_modules directories..."
	@find . -name "node_modules" -type d -exec rm -rf {} \; -prune -or -true | grep -v "Permission denied" || true
	@echo "Finding and removing .env files (preserving examples)..."
	@find . -name ".env" -o -name ".env.local" -o -name ".env.development" -o -name ".env.production" -o -name ".env.test" -type f -delete -print || true
	
	@echo "[ok] All artifacts cleaned successfully"

.PHONY: cover
cover:
	$(call title1,"Generating test coverage report")
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
	$(call title1,"Starting backend services")
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
	$(call title1,"Stopping backend services")
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
	$(call title1,"Restarting backend services")
	@make down-backend && make up-backend
	@echo "[ok] Backend services restarted successfully ✔️"

#-------------------------------------------------------
# Code Quality Commands
#-------------------------------------------------------

.PHONY: lint
lint:
	$(call title1,"Running linters on all components")
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
	$(call title1,"Formatting code in all components")
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
	$(call title1,"Cleaning dependencies in root directory")
	@echo "Tidying root go.mod..."
	@go mod tidy
	@echo "[ok] Dependencies cleaned successfully"

.PHONY: check-logs
check-logs:
	$(call title1,"Verifying error logging in usecases")
	@sh ./scripts/check-logs.sh
	@echo "[ok] Error logging verification completed"

.PHONY: check-tests
check-tests:
	$(call title1,"Verifying test coverage for components")
	@sh ./scripts/check-tests.sh
	@echo "[ok] Test coverage verification completed"

.PHONY: sec
sec:
	$(call title1,"Running security checks using gosec")
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
	$(call title1,"Installing and configuring git hooks")
	@sh ./scripts/setup-git-hooks.sh
	@echo "[ok] Git hooks installed successfully"

.PHONY: check-hooks
check-hooks:
	$(call title1,"Verifying git hooks installation status")
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
	$(call title1,"Checking if github hooks are installed and secret env files are not exposed")
	@sh ./scripts/check-envs.sh
	@echo "[ok] Environment check completed"

#-------------------------------------------------------
# Setup Commands
#-------------------------------------------------------

.PHONY: set-env
set-env:
	$(call title1,"Setting up environment files")
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
	$(call title1,"Starting all services with Docker Compose")
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
	$(call title1,"Stopping all services with Docker Compose")
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Stopping services in component: $$component_name"; \
			(cd $$dir && (docker compose -f docker-compose.yml down 2>/dev/null || docker-compose -f docker-compose.yml down)) || exit 1; \
		else \
			echo "No docker-compose.yml found in $$component_name, skipping"; \
		fi; \
	done
	@echo "[ok] All services stopped successfully"

.PHONY: start
start:
	$(call title1,"Starting all containers")
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
	$(call title1,"Cleaning all Docker resources")
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
	$(call title1,"Showing logs for all services")
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Logs for component: $$component_name"; \
			(cd $$dir && (docker compose -f docker-compose.yml logs --tail=50 2>/dev/null || docker-compose -f docker-compose.yml logs --tail=50)) || exit 1; \
			echo ""; \
		fi; \
	done

# Component-specific command execution
.PHONY: infra mdz onboarding transaction console all-components
infra:
	$(call title1,"Running command in infra component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(INFRA_DIR) && $(MAKE) $(COMMAND)

mdz:
	$(call title1,"Running command in mdz component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(MDZ_DIR) && $(MAKE) $(COMMAND)

onboarding:
	$(call title1,"Running command in onboarding component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(ONBOARDING_DIR) && $(MAKE) $(COMMAND)

transaction:
	$(call title1,"Running command in transaction component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(TRANSACTION_DIR) && $(MAKE) $(COMMAND)

console:
	$(call title1,"Running command in console component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd> to specify a command."; \
		exit 1; \
	fi
	@cd $(CONSOLE_DIR) && $(MAKE) $(COMMAND)

all-components:
	$(call title1,"Running command across all components")
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
# Demo Data Commands
#-------------------------------------------------------

.PHONY: demo-data demo-data-small demo-data-medium demo-data-large demo-data-test demo-data-test-watch demo-data-test-coverage

demo-data: demo-data-small

demo-data-small:
	$(call title1,"Generating demo data with small volume")
	@echo "Ensuring services are running..."
	@$(MAKE) up > /dev/null || (echo "Error starting services" && exit 1)
	@echo "Running demo data generator with small volume..."
	@cd scripts/demo-data && ./run-generator.sh small none
	@echo "[ok] Demo data generated successfully with small volume"

demo-data-medium:
	$(call title1,"Generating demo data with medium volume")
	@echo "Ensuring services are running..."
	@$(MAKE) up > /dev/null || (echo "Error starting services" && exit 1)
	@echo "Running demo data generator with medium volume..."
	@cd scripts/demo-data && ./run-generator.sh medium none
	@echo "[ok] Demo data generated successfully with medium volume"

demo-data-large:
	$(call title1,"Generating demo data with large volume")
	@echo "Ensuring services are running..."
	@$(MAKE) up > /dev/null || (echo "Error starting services" && exit 1)
	@echo "Running demo data generator with large volume..."
	@cd scripts/demo-data && ./run-generator.sh large none
	@echo "[ok] Demo data generated successfully with large volume"

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call title1,"Setting up development environment for all components")
	@echo "Setting up git hooks..."
	@$(MAKE) setup-git-hooks
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		echo "Setting up development environment for component: $$component_name"; \
		(cd $$dir && $(MAKE) dev-setup) || exit 1; \
		echo ""; \
	done
	@echo "[ok] Development environment set up successfully for all components"