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
include $(MIDAZ_ROOT)/pkg/shell/makefile_colors.mk
include $(MIDAZ_ROOT)/pkg/shell/makefile_utils.mk

# Shell utility functions
define print_logo
	@cat $(PWD)/pkg/shell/logo.txt
endef

# Check if a command exists
define check_command
	@if ! command -v $(1) >/dev/null 2>&1; then \
		echo "$(RED)Error: $(1) is not installed$(NC)"; \
		echo "$(MAGENTA)To install: $(2)$(NC)"; \
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
		echo "$(YELLOW)Environment files are missing. Running set-env command first...$(NC)"; \
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
	@echo "$(BOLD)Midaz Project Management Commands$(NC)"
	@echo ""
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make help                        - Display this help message"
	@echo "  make test                        - Run tests on all components"
	@echo "  make build                       - Build all components"
	@echo "  make clean                       - Clean all build artifacts"
	@echo "  make cover                       - Run test coverage"
	@echo ""
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run linting on all components"
	@echo "  make format                      - Format code in all components"
	@echo "  make tidy                        - Clean dependencies in root directory"
	@echo "  make check-logs                  - Verify error logging in usecases"
	@echo "  make check-tests                 - Verify test coverage for components"
	@echo "  make sec                         - Run security checks using gosec"
	@echo ""
	@echo ""
	@echo "$(BOLD)Git Hook Commands:$(NC)"
	@echo "  make setup-git-hooks             - Install and configure git hooks"
	@echo "  make check-hooks                 - Verify git hooks installation status"
	@echo "  make check-envs                  - Check if github hooks are installed and secret env files are not exposed"
	@echo ""
	@echo ""
	@echo "$(BOLD)Setup Commands:$(NC)"
	@echo "  make set-env                     - Copy .env.example to .env for all components"
	@echo "  make dev-setup                   - Set up development environment for all components (includes git hooks)"
	@echo ""
	@echo ""
	@echo "$(BOLD)Service Commands:$(NC)"
	@echo "  make up                          - Start all services with Docker Compose"
	@echo "  make down                        - Stop all services with Docker Compose"
	@echo "  make start                       - Start all containers"
	@echo "  make stop                        - Stop all containers"
	@echo "  make restart                     - Restart all containers"
	@echo "  make rebuild-up                  - Rebuild and restart all services"
	@echo "  make clean-docker                - Clean all Docker resources (containers, networks, volumes)"
	@echo "  make logs                        - Show logs for all services"
	@echo "  make infra COMMAND=<cmd>         - Run command in infra component"
	@echo "  make mdz COMMAND=<cmd>           - Run command in mdz component"
	@echo "  make onboarding COMMAND=<cmd>    - Run command in onboarding component"
	@echo "  make transaction COMMAND=<cmd>   - Run command in transaction component"
	@echo "  make all-components COMMAND=<cmd>- Run command across all components"
	@echo ""
	@echo ""
	@echo "$(BOLD)Development Commands:$(NC)"
	@echo "  make generate-docs-all           - Generate Swagger documentation for all services"
	@echo "  make sync-postman                - Sync Postman collection with OpenAPI documentation"
	@echo "  make verify-api-docs             - Verify API documentation coverage"
	@echo "  make validate-api-docs           - Validate API documentation structure and implementation"
	@echo "  make validate-onboarding         - Validate only the onboarding component"
	@echo "  make validate-transaction        - Validate only the transaction component"
	@echo "  make install-api-validation      - Install API validation dependencies"
	@echo "  make regenerate-mocks            - Regenerate mock files for all components"
	@echo "  make cleanup-mocks               - Remove all existing mock files"
	@echo "  make cleanup-regenerate-mocks    - Combine both operations and fix unused imports"
	@echo ""
	@echo ""

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: test
test:
	$(call title1,"Running tests on all components")
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@echo "$(CYAN)Starting tests at $$(date)$(NC)"
	@start_time=$$(date +%s); \
	test_output=$$(go test -v ./... 2>&1); \
	exit_code=$$?; \
	end_time=$$(date +%s); \
	duration=$$((end_time - start_time)); \
	echo "$$test_output"; \
	echo ""; \
	echo "$(BOLD)$(BLUE)Test Summary:$(NC)"; \
	echo "$(CYAN)----------------------------------------$(NC)"; \
	passed=$$(echo "$$test_output" | grep -c "PASS"); \
	failed=$$(echo "$$test_output" | grep -c "FAIL"); \
	skipped=$$(echo "$$test_output" | grep -c "\[no test"); \
	total=$$((passed + failed)); \
	echo "$(GREEN)✓ Passed:  $$passed tests$(NC)"; \
	if [ $$failed -gt 0 ]; then \
		echo "$(RED)✗ Failed:  $$failed tests$(NC)"; \
	else \
		echo "$(GREEN)✓ Failed:  $$failed tests$(NC)"; \
	fi; \
	echo "$(YELLOW)⚠ Skipped: $$skipped packages [no test files]$(NC)"; \
	echo "$(BLUE)⏱ Duration: $$(printf '%dm:%02ds' $$((duration / 60)) $$((duration % 60)))$(NC)"; \
	echo "$(CYAN)----------------------------------------$(NC)"; \
	if [ $$failed -eq 0 ]; then \
		echo "$(GREEN)$(BOLD)All tests passed successfully!$(NC)"; \
	else \
		echo "$(RED)$(BOLD)Some tests failed. Please check the output above for details.$(NC)"; \
	fi; \
	exit $$exit_code

.PHONY: build
build:
	$(call title1,"Building all components")
	@for dir in $(COMPONENTS); do \
		echo "$(CYAN)Building in $$dir...$(NC)"; \
		(cd $$dir && $(MAKE) build) || exit 1; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All components built successfully$(GREEN) ✔️$(NC)"

.PHONY: clean
clean:
	$(call title1,"Cleaning all build artifacts")
	@for dir in $(COMPONENTS); do \
		echo "$(CYAN)Cleaning in $$dir...$(NC)"; \
		(cd $$dir && $(MAKE) clean) || exit 1; \
		echo "$(CYAN)Ensuring thorough cleanup in $$dir...$(NC)"; \
		(cd $$dir && \
			for item in bin dist coverage.out coverage.html artifacts *.tmp; do \
				if [ -e "$$item" ]; then \
					echo "$(YELLOW)Removing $$dir/$$item$(NC)"; \
					rm -rf "$$item"; \
				fi \
			done \
		) || true; \
	done
	@echo "$(CYAN)Cleaning root-level build artifacts...$(NC)"
	@for item in bin dist coverage.out coverage.html *.tmp; do \
		if [ -e "$$item" ]; then \
			echo "$(YELLOW)Removing $$item$(NC)"; \
			rm -rf "$$item"; \
		fi \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All artifacts cleaned successfully$(GREEN) ✔️$(NC)"

.PHONY: cover
cover:
	$(call title1,"Generating test coverage report")
	@echo "$(YELLOW)Note: PostgreSQL repository tests are excluded from coverage metrics.$(NC)"
	@echo "$(YELLOW)See coverage report for details on why and what is being tested.$(NC)"
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@sh ./scripts/coverage.sh
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated at coverage.html$(NC)"
	@echo ""
	@echo "$(CYAN)Coverage Summary:$(NC)"
	@echo "$(CYAN)----------------------------------------$(NC)"
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'
	@echo "$(CYAN)----------------------------------------$(NC)"
	@echo "$(YELLOW)Open coverage.html in your browser to view detailed coverage report$(NC)"
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Coverage report generated successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Code Quality Commands
#-------------------------------------------------------

.PHONY: lint
lint:
	$(call title1,"Running linters on all components")
	@for dir in $(COMPONENTS); do \
		echo "$(CYAN)Checking for Go files in $$dir...$(NC)"; \
		if find "$$dir" -name "*.go" -type f | grep -q .; then \
			echo "$(CYAN)Linting in $$dir...$(NC)"; \
			(cd $$dir && $(MAKE) lint) || exit 1; \
		else \
			echo "$(YELLOW)No Go files found in $$dir, skipping linting$(NC)"; \
		fi; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Linting completed successfully$(GREEN) ✔️$(NC)"

.PHONY: format
format:
	$(call title1,"Formatting code in all components")
	@for dir in $(COMPONENTS); do \
		echo "$(CYAN)Checking for Go files in $$dir...$(NC)"; \
		if find "$$dir" -name "*.go" -type f | grep -q .; then \
			echo "$(CYAN)Formatting in $$dir...$(NC)"; \
			(cd $$dir && $(MAKE) format) || exit 1; \
		else \
			echo "$(YELLOW)No Go files found in $$dir, skipping formatting$(NC)"; \
		fi; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Formatting completed successfully$(GREEN) ✔️$(NC)"

.PHONY: tidy
tidy:
	$(call title1,"Cleaning dependencies in root directory")
	@echo "$(CYAN)Tidying root go.mod...$(NC)"
	@go mod tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dependencies cleaned successfully$(GREEN) ✔️$(NC)"

.PHONY: check-logs
check-logs:
	$(call title1,"Verifying error logging in usecases")
	@sh ./scripts/check-logs.sh
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Error logging verification completed$(GREEN) ✔️$(NC)"

.PHONY: check-tests
check-tests:
	$(call title1,"Verifying test coverage for components")
	@sh ./scripts/check-tests.sh
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Test coverage verification completed$(GREEN) ✔️$(NC)"

.PHONY: sec
sec:
	$(call title1,"Running security checks using gosec")
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing gosec...$(NC)"; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@if find ./components ./pkg -name "*.go" -type f | grep -q .; then \
		echo "$(CYAN)Running security checks on components/ and pkg/ folders...$(NC)"; \
		gosec ./components/... ./pkg/...; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Security checks completed$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping security checks$(NC)"; \
	fi

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
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/.env.example" ] && [ ! -f "$$dir/.env" ]; then \
			echo "$(CYAN)Creating .env in $$dir from .env.example$(NC)"; \
			cp "$$dir/.env.example" "$$dir/.env"; \
		elif [ ! -f "$$dir/.env.example" ]; then \
			echo "$(YELLOW)Warning: No .env.example found in $$dir$(NC)"; \
		else \
			echo "$(GREEN).env already exists in $$dir$(NC)"; \
		fi; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Environment files set up successfully$(GREEN) ✔️$(NC)"

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
			echo "$(CYAN)Starting services in $$dir...$(NC)"; \
			(cd $$dir && $(MAKE) up) || exit 1; \
		fi; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All services started successfully$(GREEN) ✔️$(NC)"

.PHONY: down
down:
	$(call title1,"Stopping all services with Docker Compose")
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "$(CYAN)Stopping services in component: $(BOLD)$$component_name$(NC)"; \
			(cd $$dir && (docker compose -f docker-compose.yml down 2>/dev/null || docker-compose -f docker-compose.yml down)) || exit 1; \
		else \
			echo "$(YELLOW)No docker-compose.yml found in $$component_name, skipping$(NC)"; \
		fi; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All services stopped successfully$(GREEN) ✔️$(NC)"

.PHONY: start
start:
	$(call title1,"Starting all containers")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "$(CYAN)Starting containers in $$dir...$(NC)"; \
			(cd $$dir && $(MAKE) start) || exit 1; \
		fi; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All containers started successfully$(GREEN) ✔️$(NC)"

.PHONY: stop
stop:
	$(call title1,"Stopping all containers")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "$(CYAN)Stopping containers in $$dir...$(NC)"; \
			(cd $$dir && $(MAKE) stop) || exit 1; \
		fi; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All containers stopped successfully$(GREEN) ✔️$(NC)"

.PHONY: restart
restart:
	$(call title1,"Restarting all containers")
	@make stop && make start
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All containers restarted successfully$(GREEN) ✔️$(NC)"

.PHONY: rebuild-up
rebuild-up:
	$(call title1,"Rebuilding and restarting all services")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "$(CYAN)Rebuilding and restarting services in $$dir...$(NC)"; \
			(cd $$dir && $(MAKE) rebuild-up) || exit 1; \
		fi; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All services rebuilt and restarted successfully$(GREEN) ✔️$(NC)"

.PHONY: clean-docker
clean-docker:
	$(call title1,"Cleaning all Docker resources")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "$(CYAN)Cleaning Docker resources in $$dir...$(NC)"; \
			(cd $$dir && $(MAKE) clean-docker) || exit 1; \
		fi; \
	done
	@echo "$(YELLOW)Pruning system-wide Docker resources...$(NC)"
	@docker system prune -f
	@echo "$(YELLOW)Pruning system-wide Docker volumes...$(NC)"
	@docker volume prune -f
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All Docker resources cleaned successfully$(GREEN) ✔️$(NC)"

.PHONY: logs
logs:
	$(call title1,"Showing logs for all services")
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			echo "$(CYAN)Logs for component: $(BOLD)$$component_name$(NC)"; \
			(cd $$dir && (docker compose -f docker-compose.yml logs --tail=50 2>/dev/null || docker-compose -f docker-compose.yml logs --tail=50)) || exit 1; \
			echo ""; \
		fi; \
	done

# Component-specific command execution
.PHONY: infra mdz onboarding transaction all-components
infra:
	$(call title1,"Running command in infra component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "$(RED)Error: No command specified. Use COMMAND=<cmd> to specify a command.$(NC)"; \
		exit 1; \
	fi
	@cd $(INFRA_DIR) && $(MAKE) $(COMMAND)

mdz:
	$(call title1,"Running command in mdz component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "$(RED)Error: No command specified. Use COMMAND=<cmd> to specify a command.$(NC)"; \
		exit 1; \
	fi
	@cd $(MDZ_DIR) && $(MAKE) $(COMMAND)

onboarding:
	$(call title1,"Running command in onboarding component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "$(RED)Error: No command specified. Use COMMAND=<cmd> to specify a command.$(NC)"; \
		exit 1; \
	fi
	@cd $(ONBOARDING_DIR) && $(MAKE) $(COMMAND)

transaction:
	$(call title1,"Running command in transaction component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "$(RED)Error: No command specified. Use COMMAND=<cmd> to specify a command.$(NC)"; \
		exit 1; \
	fi
	@cd $(TRANSACTION_DIR) && $(MAKE) $(COMMAND)

all-components:
	$(call title1,"Running command across all components")
	@if [ -z "$(COMMAND)" ]; then \
		echo "$(RED)Error: No command specified. Use COMMAND=<cmd> to specify a command.$(NC)"; \
		exit 1; \
	fi
	@for dir in $(COMPONENTS); do \
		echo "$(CYAN)Running '$(COMMAND)' in $$dir...$(NC)"; \
		(cd $$dir && $(MAKE) $(COMMAND)) || exit 1; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Command '$(COMMAND)' executed successfully across all components$(GREEN) ✔️$(NC)"

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
		echo "$(YELLOW)Warning: jq is not installed. Skipping Postman collection sync.$(NC)"; \
		echo "$(YELLOW)To install jq: brew install jq$(NC)"; \
		echo "$(YELLOW)Then run: make sync-postman$(NC)"; \
	fi

.PHONY: sync-postman
sync-postman:
	$(call title1,"Syncing Postman collection with OpenAPI documentation")
	$(call check_command,jq,"brew install jq")
	@sh ./scripts/sync-postman.sh
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Postman collection synced successfully with OpenAPI documentation$(GREEN) ✔️$(NC)"

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
		echo "$(YELLOW)No goreleaser configuration found in root directory. Creating a default configuration...$(NC)"; \
		goreleaser init; \
	fi
	@echo "$(CYAN)Building snapshot release of MDZ CLI...$(NC)"
	@goreleaser release --snapshot --clean
	@echo "$(GREEN)$(BOLD)[ok]$(NC) MDZ CLI snapshot created successfully$(GREEN) ✔️$(NC)"

.PHONY: goreleaser
goreleaser:
	$(call title1,"Running goreleaser (CI/CD compatible)")
	$(call check_command,goreleaser,"go install github.com/goreleaser/goreleaser@latest")
	@if [ -z "$$GITHUB_TOKEN" ]; then \
		echo "$(RED)Error: GITHUB_TOKEN environment variable is required for releases.$(NC)"; \
		echo "$(YELLOW)Please set it using: export GITHUB_TOKEN=your_github_token$(NC)"; \
		echo "$(YELLOW)You can create a token at: https://github.com/settings/tokens$(NC)"; \
		exit 1; \
	fi
	@echo "$(CYAN)Running goreleaser...$(NC)"
	@goreleaser release --clean
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Release completed successfully$(GREEN) ✔️$(NC)"

.PHONY: regenerate-mocks
regenerate-mocks:
	$(call title1,"Regenerating mocks for all components")
	$(call check_command,mockgen,"go install github.com/golang/mock/mockgen@latest")
	@MODULE_NAME=$$(go list -m); \
	for component in $$(find ./components -maxdepth 1 -mindepth 1 -type d); do \
		echo "$(CYAN)Scanning directory: $$component$(NC)"; \
		for file in $$(find "$$component" -name "*.go" -not -name "*_mock.go" -not -path "*/vendor/*"); do \
			if grep -q "type.*interface" "$$file"; then \
				pkg_path=$$(dirname "$$file"); \
				pkg_name=$$(basename "$$pkg_path"); \
				file_name=$$(basename "$$file" .go); \
				rel_path=$${pkg_path#./}; \
				full_import_path="$$MODULE_NAME/$$rel_path"; \
				echo "$(GREEN)Generating mock for: $$file (package: $$full_import_path)$(NC)"; \
				mockgen -source="$$file" -destination="$${file%.*}_mock.go" -package="$$pkg_name" || { \
					interfaces=$$(grep -E "type[[:space:]]+[A-Z][a-zA-Z0-9_]*[[:space:]]+interface" "$$file" | awk '{print $$2}'); \
					for interface in $$interfaces; do \
						echo "$(YELLOW)Trying package mode for interface: $$interface$(NC)"; \
						mockgen -destination="$${file%.*}_mock.go" -package="$$pkg_name" "$$full_import_path" "$$interface"; \
					done; \
				}; \
			fi; \
		done; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Mock regeneration completed$(GREEN) ✔️$(NC)"

.PHONY: cleanup-mocks
cleanup-mocks:
	$(call title1,"Cleaning up duplicate mock files")
	@for component in $$(find ./components -maxdepth 1 -mindepth 1 -type d); do \
		echo "$(CYAN)Cleaning directory: $$component$(NC)"; \
		find "$$component" -name "*_mock.go" -o -name "*mock.go" | while read -r mock_file; do \
			echo "$(YELLOW)Removing $$mock_file$(NC)"; \
			rm "$$mock_file"; \
		done; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Mock cleanup completed$(GREEN) ✔️$(NC)"

.PHONY: cleanup-regenerate-mocks
cleanup-regenerate-mocks: cleanup-mocks regenerate-mocks
	$(call title1,"Fixing any unused imports in test files")
	@if grep -q "github.com/stretchr/testify/assert.*and not used" ./components/onboarding/internal/services/command/send-account-queue-transaction_test.go 2>/dev/null; then \
		echo "$(YELLOW)Fixing unused import in send-account-queue-transaction_test.go$(NC)"; \
		sed -i '' 's/^import (/import (\n\/\/ testify\/assert is used in commented out code\n/' ./components/onboarding/internal/services/command/send-account-queue-transaction_test.go; \
	fi
	@if grep -q "github.com/stretchr/testify/assert.*and not used" ./components/transaction/internal/services/command/send-bto-execute-async_test.go 2>/dev/null; then \
		echo "$(YELLOW)Fixing unused import in send-bto-execute-async_test.go$(NC)"; \
		sed -i '' 's/^import (/import (\n\/\/ testify\/assert is used in commented out code\n/' ./components/transaction/internal/services/command/send-bto-execute-async_test.go; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Mock cleanup and regeneration completed$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call title1,"Setting up development environment for all components")
	@echo "$(CYAN)Setting up git hooks...$(NC)"
	@$(MAKE) setup-git-hooks
	@for dir in $(COMPONENTS); do \
		component_name=$$(basename $$dir); \
		echo "$(CYAN)Setting up development environment for component: $(BOLD)$$component_name$(NC)"; \
		(cd $$dir && $(MAKE) dev-setup) || exit 1; \
		echo ""; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully for all components$(GREEN) ✔️$(NC)"