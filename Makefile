# Define the root directory of the project
MIDAZ_ROOT := $(shell pwd)

INFRA_DIR := ./components/infra
MDZ_DIR := ./components/mdz
ONBOARDING_DIR := ./components/onboarding
TRANSACTION_DIR := ./components/transaction

# Define a list of all component directories for easier iteration
COMPONENTS := $(INFRA_DIR) $(ONBOARDING_DIR) $(TRANSACTION_DIR) $(MDZ_DIR)

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

# Core Commands
.PHONY: help
help:
	@echo ""
	@echo ""
	@echo "$(BOLD)Midaz Project Management Commands$(NC)"
	@echo ""
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make help                        - Display this help message"
	@echo "  make test                        - Run tests on all projects"
	@echo "  make cover                       - Run test coverage"
	@echo ""
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run golangci-lint and performance checks"
	@echo "  make format                      - Format Go code using gofmt"
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
	@echo "  make build                       - Build all project services"
	@echo "  make clean                       - Clean artifacts and files matching .gitignore patterns"
	@echo ""
	@echo ""
	@echo "$(BOLD)Service Commands:$(NC)"
	@echo "  make up                          - Start all services with Docker Compose"
	@echo "  make down                        - Stop all services with Docker Compose"
	@echo "  make start                       - Start all containers (or build and start if images don't exist)"
	@echo "  make stop                        - Stop all containers"
	@echo "  make restart                     - Restart all containers (or build and start if images don't exist)"
	@echo "  make rebuild-up                  - Rebuild and restart all services"
	@echo "  make infra COMMAND=<cmd>         - Run command in infra service"
	@echo "  make onboarding COMMAND=<cmd>    - Run command in onboarding service"
	@echo "  make transaction COMMAND=<cmd>   - Run command in transaction service"
	@echo "  make all-services COMMAND=<cmd>  - Run command across all services"
	@echo ""
	@echo ""
	@echo "$(BOLD)Development Commands:$(NC)"
	@echo "  make tidy                        - Run go mod tidy"
	@echo "  make goreleaser                  - Create a release snapshot"
	@echo "  make generate-docs-all           - Generate Swagger documentation for all services"
	@echo ""
	@echo ""

# Core Commands
.PHONY: test
test:
	@echo "$(BLUE)Running tests on all projects...$(NC)"
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	go test -v ./... ./...

.PHONY: cover
cover:
	@echo "$(BLUE)Generating test coverage report...$(NC)"
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@sh ./scripts/coverage.sh
	@go tool cover -html=coverage.out -o coverage.html

# Code Quality Commands
.PHONY: lint
lint:
	@echo "$(BLUE)Running linting and performance checks...$(NC)"
	$(call title1,"STARTING LINT")
	$(call check_command,golangci-lint,"go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest")
	@out=$$(golangci-lint run --fix ./... 2>&1); \
	out_err=$$?; \
	perf_out=$$(perfsprint ./... 2>&1); \
	perf_err=$$?; \
	echo "$$out"; \
	echo "$$perf_out"; \
	if [ $$out_err -ne 0 ]; then \
		echo -e "\n$(BOLD)$(RED)An error has occurred during the lint process: \n $$out\n"; \
		exit 1; \
	fi; \
	if [ $$perf_err -ne 0 ]; then \
		echo -e "\n$(BOLD)$(RED)An error has occurred during the performance check: \n $$perf_out\n"; \
		exit 1; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Lint and performance checks passed successfully$(GREEN) ✔️$(NC)"

.PHONY: format
format:
	@echo "$(BLUE)Formatting Go code using gofmt...$(NC)"
	$(call title1,"Formatting all golang source code")
	$(call check_command,gofmt,"Install Go from https://golang.org/doc/install")
	@gofmt -w ./
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All go files formatted$(GREEN) ✔️$(NC)"

.PHONY: check-logs
check-logs:
	@echo "$(BLUE)Verifying error logging in usecases...$(NC)"
	$(call title1,"STARTING LOGS ANALYZER")
	@find . -type f -path '*usecase*/*' -name '*.go' > /tmp/midaz_go_files.txt; \
	err=0; \
	while IFS= read -r path; do \
		if grep -q 'err != nil' "$$path" && ! grep -qE '(logger\.Error|log\.Error)' "$$path" && [[ "$$path" != *"_test"* ]]; then \
			err=1; \
			echo "$$path"; \
		fi; \
	done < /tmp/midaz_go_files.txt; \
	rm /tmp/midaz_go_files.txt; \
	if [ $$err -eq 1 ]; then \
		echo -e "\n$(RED)You need to log all errors inside usecases after they are handled. $(BOLD)[WARNING]$(NC)\n"; \
		exit 1; \
	else \
		echo "$(GREEN)$(BOLD)[ok]$(NC) All good$(GREEN) ✔️$(NC)"; \
	fi

# TODO: add output files for tests to comply with the test coverage
# TODO: add test coverage for missing components
# ----------------------------------
#    📝 STARTING TESTS ANALYZER  
# ----------------------------------
# Error: There is no test for the file components/onboarding/internal/services/query/query.go
# Error: There is no test for the file components/transaction/internal/services/query/query.go
# Error: There is no test for the file components/transaction/internal/services/query/get-balances.go
# Error: There is no test for the file components/onboarding/internal/services/command/command.go
# Error: There is no test for the file components/onboarding/internal/services/command/send-account-queue-transaction.go
# Error: There is no test for the file components/transaction/internal/services/command/command.go
# Error: There is no test for the file components/transaction/internal/services/command/create-balance-transaction-operations-async.go
# Error: There is no test for the file components/transaction/internal/services/command/send-bto-execute-async.go
# Error: There is no test for the file components/transaction/internal/services/command/create-idempotency-key.go
# Error: There is no test for the file components/transaction/internal/services/command/send-log-transaction-audit-queue.go
# Error: There is no test for the file components/transaction/internal/services/command/create-balance.go

.PHONY: check-tests
check-tests:
	@echo "$(BLUE)Verifying test coverage for components...$(NC)"
	$(call title1,"STARTING TESTS ANALYZER")
	@err=false; \
	subdirs="components/*/internal/services/query components/*/internal/services/command"; \
	for subdir in $$subdirs; do \
		find "$$subdir" -type f -name "*.go" 2>/dev/null > /tmp/midaz_test_files.txt || echo "" > /tmp/midaz_test_files.txt; \
		while IFS= read -r file; do \
			if [[ "$$file" != *"_test.go" ]]; then \
				test_file="$${file%.go}_test.go"; \
				if [ ! -f "$$test_file" ]; then \
					echo "Error: There is no test for the file $$file"; \
					err=true; \
				fi; \
			fi; \
		done < /tmp/midaz_test_files.txt; \
	done; \
	rm -f /tmp/midaz_test_files.txt; \
	if [ "$$err" = true ]; then \
		echo -e "\n$(RED)There are files without corresponding test files.$(NC)\n"; \
		exit 1; \
	else \
		echo "$(GREEN)$(BOLD)[ok]$(NC) All tests are in place$(GREEN) ✔️$(NC)"; \
	fi

.PHONY: sec
sec:
	@echo "$(BLUE)Running security checks using gosec...$(NC)"
	$(call check_command,gosec,"go install github.com/securego/gosec/v2/cmd/gosec@latest")
	gosec ./...

# Git Hook Commands
.PHONY: setup-git-hooks
setup-git-hooks:
	@echo "$(BLUE)Installing and configuring git hooks...$(NC)"
	$(call title1,"Setting up git hooks...")
	@find .githooks -type f -exec cp {} .git/hooks \;
	@chmod +x .git/hooks/*
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All hooks installed and updated$(GREEN) ✔️$(NC)"

.PHONY: check-hooks
check-hooks:
	@echo "$(BLUE)Verifying git hooks installation status...$(NC)"
	$(call title1,"Checking git hooks status...")
	@err=0; \
	for hook_dir in .githooks/*; do \
		if [ -d "$$hook_dir" ]; then \
			for FILE in "$$hook_dir"/*; do \
				if [ -f "$$FILE" ]; then \
					f=$$(basename -- $$hook_dir)/$$(basename -- $$FILE); \
					hook_name=$$(basename -- $$FILE); \
					FILE2=.git/hooks/$$hook_name; \
					if [ -f "$$FILE2" ]; then \
						if cmp -s "$$FILE" "$$FILE2"; then \
							echo "$(GREEN)$(BOLD)[ok]$(NC) Hook file $$f installed and updated$(GREEN) ✔️$(NC)"; \
						else \
							echo "$(RED)Hook file $$f installed but out-of-date [OUT-OF-DATE] ✗$(NC)"; \
							err=1; \
						fi; \
					else \
						echo "$(RED)Hook file $$f not installed [NOT INSTALLED] ✗$(NC)"; \
						err=1; \
					fi; \
				fi; \
			done; \
		fi; \
	done; \
	if [ $$err -ne 0 ]; then \
		echo -e "\nRun $(BOLD)make setup-git-hooks$(NC) to setup your development environment, then try again.\n"; \
		exit 1; \
	else \
		echo "$(GREEN)$(BOLD)[ok]$(NC) All hooks are properly installed$(GREEN) ✔️$(NC)"; \
	fi

.PHONY: check-envs
check-envs:
	@echo "$(BLUE)Checking git hooks and environment files for security issues...$(NC)"
	$(MAKE) check-hooks
	@echo "$(BLUE)Checking for exposed secrets in environment files...$(NC)"
	@if grep -r "SECRET.*=" --include=".env" .; then \
		echo "$(RED)Warning: Secrets found in environment files. Make sure these are not committed to the repository.$(NC)"; \
		exit 1; \
	else \
		echo "$(GREEN)No exposed secrets found in environment files$(GREEN) ✔️$(NC)"; \
	fi

# Setup Commands
.PHONY: set-env
set-env:
	@echo "$(BLUE)Setting up environment files for all components...$(NC)"
	@echo "$(YELLOW)WARNING:$(NC)"
	@echo "$(YELLOW)Customize .env variables to fit your environment. Default values are for initial setup and may not be secure for production. Protect sensitive info and avoid exposing .env files in public repositories.$(NC)"
	@echo "$(BLUE)Setting up environment files...$(NC)"
	@cp -r $(INFRA_DIR)/.env.example $(INFRA_DIR)/.env
	@cp -r $(ONBOARDING_DIR)/.env.example $(ONBOARDING_DIR)/.env
	@cp -r $(TRANSACTION_DIR)/.env.example $(TRANSACTION_DIR)/.env
	@cp -r $(MDZ_DIR)/.env.example $(MDZ_DIR)/.env
	@echo "$(BLUE)Environment files created successfully$(NC)"

.PHONY: build
build:
	@echo "$(BLUE)Building all project services...$(NC)"
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	@for dir in $(COMPONENTS); do \
		echo "$(BLUE)Building $$(basename $$dir) service...$(NC)"; \
		$(MAKE) -C $$dir build || exit 1; \
	done
	@echo "$(GREEN)All services built successfully$(NC)"

.PHONY: clean
clean:
	@echo "$(BLUE)Cleaning project artifacts and temporary files...$(NC)"
	
	@echo "$(BLUE)Cleaning files based on .gitignore patterns...$(NC)"
	# Dynamically read patterns from .gitignore file
	# This ensures that when you update .gitignore, the clean command automatically adapts
	@if [ -f .gitignore ]; then \
		echo "$(CYAN)Reading patterns from .gitignore...$(NC)"; \
		patterns=$$(grep -v '^#' .gitignore | grep -v '^$$'); \
		if [ -n "$$patterns" ]; then \
			echo "$(CYAN)Processing .gitignore patterns...$(NC)"; \
			echo "$$patterns" | xargs -I{} sh -c 'echo "$(CYAN)Processing pattern: {}" && find . -name "{}" -not -path "*/\.git/*" -exec rm -rf {} \; 2>/dev/null || true'; \
		fi; \
	else \
		echo "$(YELLOW)No .gitignore file found. Skipping gitignore-based cleaning.$(NC)"; \
	fi
	
	@echo "$(BLUE)Cleaning common build artifacts...$(NC)"
	@find . -name "*.o" -o -name "*.a" -o -name "*.so" -o -name "*.test" -o -name "*.out" -o -name "coverage.html" -o -name "__debug_bin*" -type f -delete
	@find . -path "*/dist/*" -o -path "*/.idea/*" -o -path "*/.vscode/*" -o -path "*/.run/*" -not -path "*/\.git/*" -exec rm -rf {} \; 2>/dev/null || true
	
	@echo "$(GREEN)All artifacts cleaned successfully$(NC)"

# Service Commands
.PHONY: up
up: 
	@echo "$(BLUE)Starting all services with Docker Compose...$(NC)"
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			ENV_NAME=development $(DOCKER_CMD) -f $$dir/docker-compose.yml up --build -d; \
		else \
			echo "$(YELLOW)Skipping $$dir: No docker-compose.yml file found$(NC)"; \
		fi; \
	done
	@echo "$(GREEN)All services started successfully$(NC)"

.PHONY: down
down:
	@echo "$(BLUE)Stopping all services with Docker Compose...$(NC)"
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			ENV_NAME=development $(DOCKER_CMD) -f $$dir/docker-compose.yml down; \
		else \
			echo "$(YELLOW)Skipping $$dir: No docker-compose.yml file found$(NC)"; \
		fi; \
	done
	@echo "$(GREEN)All services stopped successfully$(NC)"

.PHONY: start
start:
	@echo "$(BLUE)Starting all services...$(NC)"
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@containers_exist=true; \
	for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			service_name=$$(basename $$dir); \
			if ! docker ps -a --format '{{.Names}}' | grep -q "$$service_name"; then \
				containers_exist=false; \
				break; \
			fi; \
		fi; \
	done; \
	if [ "$$containers_exist" = "false" ]; then \
		echo "$(YELLOW)Some containers don't exist. Running 'up' to build and start them...$(NC)"; \
		$(MAKE) up; \
	else \
		for dir in $(COMPONENTS); do \
			if [ -f "$$dir/docker-compose.yml" ]; then \
				ENV_NAME=development $(DOCKER_CMD) -f $$dir/docker-compose.yml start; \
			else \
				echo "$(YELLOW)Skipping $$dir: No docker-compose.yml file found$(NC)"; \
			fi; \
		done; \
		echo "$(GREEN)All services started successfully$(NC)"; \
	fi

.PHONY: stop
stop:
	@echo "$(BLUE)Stopping all services...$(NC)"
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			ENV_NAME=development $(DOCKER_CMD) -f $$dir/docker-compose.yml stop; \
		else \
			echo "$(YELLOW)Skipping $$dir: No docker-compose.yml file found$(NC)"; \
		fi; \
	done
	@echo "$(GREEN)All services stopped successfully$(NC)"

.PHONY: restart
restart:
	@echo "$(BLUE)Restarting all services...$(NC)"
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@containers_exist=true; \
	for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			service_name=$$(basename $$dir); \
			if ! docker ps -a --format '{{.Names}}' | grep -q "$$service_name"; then \
				containers_exist=false; \
				break; \
			fi; \
		fi; \
	done; \
	if [ "$$containers_exist" = "false" ]; then \
		echo "$(YELLOW)Some containers don't exist. Running 'up' to build and start them...$(NC)"; \
		$(MAKE) up; \
	else \
		for dir in $(COMPONENTS); do \
			if [ -f "$$dir/docker-compose.yml" ]; then \
				ENV_NAME=development $(DOCKER_CMD) -f $$dir/docker-compose.yml restart; \
			else \
				echo "$(YELLOW)Skipping $$dir: No docker-compose.yml file found$(NC)"; \
			fi; \
		done; \
		echo "$(GREEN)All services restarted successfully$(NC)"; \
	fi

.PHONY: rebuild-up
rebuild-up:
	@echo "$(BLUE)Rebuilding and restarting all services...$(NC)"
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/docker-compose.yml" ]; then \
			ENV_NAME=development $(DOCKER_CMD) -f $$dir/docker-compose.yml up --build --force-recreate -d; \
		else \
			echo "$(YELLOW)Skipping $$dir: No docker-compose.yml file found$(NC)"; \
		fi; \
	done
	@echo "$(GREEN)All services rebuilt and restarted successfully$(NC)"

.PHONY: infra
infra:
	@echo "$(BLUE)Executing command in infra service...$(NC)"
	$(MAKE) -C $(INFRA_DIR) $(COMMAND)

.PHONY: onboarding
onboarding:
	@echo "$(BLUE)Executing command in onboarding service...$(NC)"
	$(MAKE) -C $(ONBOARDING_DIR) $(COMMAND)

.PHONY: transaction
transaction:
	@echo "$(BLUE)Executing command in transaction service...$(NC)"
	$(MAKE) -C $(TRANSACTION_DIR) $(COMMAND)

.PHONY: all-services
all-services:
	@echo "$(BLUE)Executing command across all services...$(NC)"
	$(MAKE) -C $(INFRA_DIR) $(COMMAND) && \
	$(MAKE) -C $(ONBOARDING_DIR) $(COMMAND) && \
	$(MAKE) -C $(TRANSACTION_DIR) $(COMMAND)

# Development Commands
.PHONY: tidy
tidy:
	@echo "$(BLUE)Running go mod tidy to clean up dependencies...$(NC)"
	$(call check_command,go,"Install Go from https://golang.org/doc/install")
	go mod tidy

.PHONY: goreleaser
goreleaser:
	@echo "$(BLUE)Creating release snapshot with goreleaser...$(NC)"
	$(call check_command,goreleaser,"go install github.com/goreleaser/goreleaser@latest")
	goreleaser release --snapshot --skip-publish --rm-dist

.PHONY: generate-docs-all
generate-docs-all:
	@echo "$(BLUE)Executing command to generate swagger...$(NC)"
	$(MAKE) -C $(ONBOARDING_DIR) generate-docs && \
	$(MAKE) -C $(TRANSACTION_DIR) generate-docs
