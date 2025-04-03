# Midaz Project Root Makefile
# Coordinates all component Makefiles and provides centralized commands

# Define the root directory of the project
MIDAZ_ROOT := $(shell pwd)

# Component directories
INFRA_DIR := ./components/infra
MDZ_DIR := ./components/mdz
ONBOARDING_DIR := ./components/onboarding
TRANSACTION_DIR := ./components/transaction

# Define a list of all component directories for easier iteration
COMPONENTS := $(INFRA_DIR) $(MDZ_DIR) $(ONBOARDING_DIR) $(TRANSACTION_DIR)

# Include shared color definitions and utility functions
include $(MIDAZ_ROOT)/pkg/shell/makefile_utils.mk

# Shell utility functions
define print_logo
	@cat $(PWD)/pkg/shell/logo.txt
endef

# Check if a command exists
define check_command
	@if ! command -v $(1) >/dev/null 2>&1; then \
		echo "Error: $(1) not installed"; \
		echo "Install: $(2)"; \
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
		echo "Missing env files. Running set-env..."; \
		$(MAKE) set-env; \
	fi
endef

#-------------------------------------------------------
# Help Command
#-------------------------------------------------------

.PHONY: help
help:
	@chmod +x ./scripts/show_help.sh
	@./scripts/show_help.sh

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: test
test:
	$(call title1,"Running tests on all components")
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@chmod +x ./scripts/run_tests.sh
	@./scripts/run_tests.sh

.PHONY: build
build:
	$(call title1,"Building all components")
	@for dir in $(COMPONENTS); do \
		echo "Building $$dir..."; \
		(cd $$dir && $(MAKE) build) || exit 1; \
	done
	@echo "[ok] Build complete"

.PHONY: clean
clean:
	$(call title1,"Cleaning all build artifacts")
	@for dir in $(COMPONENTS); do \
		echo "Cleaning $$dir..."; \
		(cd $$dir && $(MAKE) clean) || exit 1; \
		echo "Deep cleaning $$dir..."; \
		(cd $$dir && \
			for item in bin dist coverage.out coverage.html artifacts *.tmp; do \
				if [ -e "$$item" ]; then \
					echo "Removing $$dir/$$item"; \
					rm -rf "$$item"; \
				fi \
			done \
		) || true; \
	done
	@echo "Cleaning root artifacts..."
	@for item in bin dist coverage.out coverage.html *.tmp; do \
		if [ -e "$$item" ]; then \
			echo "Removing $$item"; \
			rm -rf "$$item"; \
		fi \
	done
	@echo "[ok] Cleanup complete"

.PHONY: cover
cover:
	$(call title1,"Generating test coverage report")
	@echo "Note: PostgreSQL tests excluded from metrics"
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@sh ./scripts/coverage.sh
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Report: coverage.html"
	@echo ""
	@echo "Coverage Summary:"
	@echo "----------------------------------------"
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'
	@echo "----------------------------------------"
	@echo "Open coverage.html to view details"
	@echo "[ok] Coverage report ready"

#-------------------------------------------------------
# Code Quality Commands
#-------------------------------------------------------

.PHONY: lint
lint:
	$(call title1,"Running linters on all components")
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@chmod +x ./scripts/run_lint.sh
	@./scripts/run_lint.sh
	@echo "[ok] Lint complete"

.PHONY: format
format:
	$(call title1,"Formatting code in all components")
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@chmod +x ./scripts/run_format.sh
	@./scripts/run_format.sh
	@echo "[ok] Format complete"

.PHONY: tidy
tidy:
	$(call title1,"Cleaning dependencies in root directory")
	@echo "Tidying root go.mod..."
	@go mod tidy
	@echo "[ok] Dependencies cleaned"

.PHONY: check-logs
check-logs:
	$(call title1,"Verifying error logging in usecases")
	@sh ./scripts/check-logs.sh
	@echo "[ok] Error logging verified"

.PHONY: check-tests
check-tests:
	$(call title1,"Verifying test coverage for components")
	@sh ./scripts/check-tests.sh
	@echo "[ok] Test coverage verified"

.PHONY: sec
sec:
	$(call title1,"Running security checks using gosec")
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "Installing gosec..."; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@if find ./components ./pkg -name "*.go" -type f | grep -q .; then \
		echo "Running security checks..."; \
		gosec ./components/... ./pkg/...; \
		echo "[ok] Security checks complete"; \
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
	@echo "[ok] Git hooks installed"

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
		echo "[ok] All git hooks installed"; \
	else \
		echo "[error] Missing git hooks. Run 'make setup-git-hooks'"; \
		exit 1; \
	fi

.PHONY: check-envs
check-envs:
	$(call title1,"Checking git hooks and env files")
	@sh ./scripts/check-envs.sh
	@echo "[ok] Environment check complete"

#-------------------------------------------------------
# Setup Commands
#-------------------------------------------------------

.PHONY: set-env
set-env:
	$(call title1,"Setting up environment files")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/.env.example" ] && [ ! -f "$$dir/.env" ]; then \
			echo "Creating .env in $$dir"; \
			cp "$$dir/.env.example" "$$dir/.env"; \
		elif [ ! -f "$$dir/.env.example" ]; then \
			echo "Warning: No .env.example in $$dir"; \
		else \
			echo ".env exists in $$dir"; \
		fi; \
	done
	@echo "[ok] Environment files ready"

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
	@echo "[ok] All services started"

.PHONY: down
down:
	$(call title1,"Stopping all services with Docker Compose")
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Stopping: $$component_name"; \
			(cd $$dir && (docker compose -f docker-compose.yml down 2>/dev/null || docker-compose -f docker-compose.yml down)) || exit 1; \
		else \
			echo "No docker-compose.yml in $$component_name"; \
		fi; \
	done
	@echo "[ok] All services stopped"

.PHONY: start
start:
	$(call title1,"Starting all containers")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Starting containers in $$dir..."; \
			(cd $$dir && $(MAKE) start) || exit 1; \
		fi; \
	done
	@echo "[ok] All containers started"

.PHONY: stop
stop:
	$(call title1,"Stopping all containers")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Stopping containers in $$dir..."; \
			(cd $$dir && $(MAKE) stop) || exit 1; \
		fi; \
	done
	@echo "[ok] All containers stopped"

.PHONY: restart
restart:
	$(call title1,"Restarting all containers")
	@make stop && make start
	@echo "[ok] All containers restarted"

.PHONY: rebuild-up
rebuild-up:
	$(call title1,"Rebuilding and restarting all services")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Rebuilding $$dir..."; \
			(cd $$dir && $(MAKE) rebuild-up) || exit 1; \
		fi; \
	done
	@echo "[ok] All services rebuilt and restarted"

.PHONY: clean-docker
clean-docker:
	$(call title1,"Cleaning all Docker resources")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "Cleaning Docker in $$dir..."; \
			(cd $$dir && $(MAKE) clean-docker) || exit 1; \
		fi; \
	done
	@echo "Pruning Docker containers and networks..."
	@docker container prune -f
	@docker network prune -f
	@echo "Pruning Docker volumes..."
	@docker volume prune -f
	@echo "[ok] Docker resources cleaned (images preserved)"

#-------------------------------------------------------
# Component-specific command execution
#-------------------------------------------------------

.PHONY: infra mdz onboarding transaction all-components
infra:
	$(call title1,"Running command in infra component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd>"; \
		exit 1; \
	fi
	@cd $(INFRA_DIR) && $(MAKE) $(COMMAND)

mdz:
	$(call title1,"Running command in mdz component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd>"; \
		exit 1; \
	fi
	@cd $(MDZ_DIR) && $(MAKE) $(COMMAND)

onboarding:
	$(call title1,"Running command in onboarding component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd>"; \
		exit 1; \
	fi
	@cd $(ONBOARDING_DIR) && $(MAKE) $(COMMAND)

transaction:
	$(call title1,"Running command in transaction component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd>"; \
		exit 1; \
	fi
	@cd $(TRANSACTION_DIR) && $(MAKE) $(COMMAND)

all-components:
	$(call title1,"Running command across all components")
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: No command specified. Use COMMAND=<cmd>"; \
		exit 1; \
	fi
	@for dir in $(COMPONENTS); do \
		echo "Running '$(COMMAND)' in $$dir..."; \
		(cd $$dir && $(MAKE) $(COMMAND)) || exit 1; \
	done
	@echo "[ok] Command '$(COMMAND)' executed across all components"

#-------------------------------------------------------
# Development Commands
#-------------------------------------------------------

.PHONY: generate-docs-all
generate-docs-all:
	$(call title1,"Generating Swagger documentation for all services")
	$(call check_command,swag,"go install github.com/swaggo/swag/cmd/swag@latest")
	@echo "$(CYAN)Verifying API documentation coverage...$(NC)"
	@sh ./scripts/verify-api-docs.sh 2>/dev/null || echo "$(YELLOW)Warning: Some API endpoints may not be properly documented. Continuing with documentation generation...$(NC)"
	@echo "$(CYAN)Generating documentation for onboarding component...$(NC)"
	@cd $(ONBOARDING_DIR) && $(MAKE) generate-docs 2>&1 | grep -v "warning: "
	@echo "$(CYAN)Generating documentation for transaction component...$(NC)"
	@cd $(TRANSACTION_DIR) && $(MAKE) generate-docs 2>&1 | grep -v "warning: "
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Swagger documentation generated successfully for all services$(GREEN) ✔️$(NC)"
	@echo "$(CYAN)Syncing Postman collection with the generated OpenAPI documentation...$(NC)"
	@if command -v jq >/dev/null 2>&1; then \
		sh ./scripts/sync-postman.sh; \
	else \
		echo "Warning: jq not installed. Skipping Postman sync"; \
		echo "Install: brew install jq"; \
		echo "Then run: make sync-postman"; \
	fi

.PHONY: sync-postman
sync-postman:
	$(call title1,"Syncing Postman collection with OpenAPI docs")
	$(call check_command,jq,"brew install jq")
	@sh ./scripts/sync-postman.sh
	@echo "[ok] Postman collection synced"

.PHONY: verify-api-docs
verify-api-docs:
	$(call title1,"Verifying API documentation coverage")
	@if [ -f "./scripts/package.json" ]; then \
		echo "$(CYAN)Installing npm dependencies...$(NC)"; \
		cd ./scripts && npm install; \
	fi
	@sh ./scripts/verify-api-docs.sh
	@echo "$(GREEN)$(BOLD)[ok]$(NC) API documentation verification completed$(GREEN) ✔️$(NC)"

.PHONY: validate-api-docs
validate-api-docs:
	$(call title1,"Validating API documentation structure and implementation")
	@if [ -f "./scripts/package.json" ]; then \
		echo "$(CYAN)Using npm to run validation...$(NC)"; \
		cd ./scripts && npm run validate-all; \
	else \
		echo "$(YELLOW)No package.json found in scripts directory. Running traditional validation...$(NC)"; \
		$(MAKE) verify-api-docs; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) API documentation validation completed$(GREEN) ✔️$(NC)"

.PHONY: validate-onboarding
validate-onboarding:
	$(call title1,"Validating onboarding component API documentation")
	@if [ -f "./scripts/package.json" ]; then \
		echo "$(CYAN)Installing npm dependencies...$(NC)"; \
		cd ./scripts && npm install; \
	fi
	@cd ./components/onboarding && make validate-api-docs
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Onboarding API validation completed$(GREEN) ✔️$(NC)"

.PHONY: validate-transaction
validate-transaction:
	$(call title1,"Validating transaction component API documentation")
	@if [ -f "./scripts/package.json" ]; then \
		echo "$(CYAN)Installing npm dependencies...$(NC)"; \
		cd ./scripts && npm install; \
	fi
	@cd ./components/transaction && make validate-api-docs
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Transaction API validation completed$(GREEN) ✔️$(NC)"

.PHONY: install-api-validation
install-api-validation:
	$(call title1,"Installing API validation dependencies")
	@mkdir -p ./scripts
	@if [ ! -f "./scripts/package.json" ]; then \
		echo "$(CYAN)Creating package.json in scripts directory...$(NC)"; \
		echo '{"name":"midaz-scripts","version":"1.0.0","description":"Midaz API documentation validation scripts","scripts":{"verify-api":"bash ./verify-api-docs.sh","validate-onboarding":"cd ../components/onboarding && make validate-api-docs","validate-transaction":"cd ../components/transaction && make validate-api-docs","validate-all":"npm run validate-onboarding && npm run validate-transaction"},"dependencies":{"axios":"^1.8.4","commander":"^9.4.1","glob":"^8.0.3","js-yaml":"^4.1.0"}}' > ./scripts/package.json; \
	fi
	@cd ./scripts && npm install
	@echo "$(GREEN)$(BOLD)[ok]$(NC) API validation dependencies installed$(GREEN) ✔️$(NC)"

.PHONY: mdz-goreleaser
mdz-goreleaser:
	$(call title1,"Releasing MDZ CLI using goreleaser")
	$(call check_command,goreleaser,"go install github.com/goreleaser/goreleaser@latest")
	@if [ ! -f .goreleaser.yml ] && [ ! -f .goreleaser.yaml ]; then \
		echo "$(YELLOW)No goreleaser configuration found in root directory. Creating a default configuration...$(NC)"; \
		goreleaser init; \
	fi
	@if [ -z "$$GITHUB_TOKEN" ]; then \
		echo "$(RED)Error: GITHUB_TOKEN environment variable is required for releases.$(NC)"; \
		echo "$(YELLOW)Please set it using: export GITHUB_TOKEN=your_github_token$(NC)"; \
		echo "$(YELLOW)You can create a token at: https://github.com/settings/tokens$(NC)"; \
		exit 1; \
	fi
	@echo "$(CYAN)Building and releasing MDZ CLI...$(NC)"
	@goreleaser release --clean
	@echo "$(GREEN)$(BOLD)[ok]$(NC) MDZ CLI released successfully$(GREEN) ✔️$(NC)"

.PHONY: mdz-goreleaser-snapshot
mdz-goreleaser-snapshot:
	$(call title1,"Creating snapshot release of MDZ CLI")
	$(call check_command,goreleaser,"go install github.com/goreleaser/goreleaser@latest")
	@if [ ! -f .goreleaser.yml ] && [ ! -f .goreleaser.yaml ]; then \
		echo "No goreleaser config found. Creating default..."; \
		goreleaser init; \
	fi
	@echo "Building MDZ CLI snapshot..."
	@goreleaser release --snapshot --clean
	@echo "[ok] MDZ CLI snapshot created"

.PHONY: goreleaser
goreleaser:
	$(call title1,"Running goreleaser (CI/CD compatible)")
	$(call check_command,goreleaser,"go install github.com/goreleaser/goreleaser@latest")
	@if [ -z "$$GITHUB_TOKEN" ]; then \
		echo "Error: GITHUB_TOKEN required for releases"; \
		echo "Set with: export GITHUB_TOKEN=your_github_token"; \
		echo "Create token at: https://github.com/settings/tokens"; \
		exit 1; \
	fi
	@echo "Running goreleaser..."
	@goreleaser release --clean
	@echo "[ok] Release complete"

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
		echo "Setting up dev env for: $$component_name"; \
		(cd $$dir && $(MAKE) dev-setup) || exit 1; \
		echo ""; \
	done
	@echo "[ok] Dev environment ready"
