AUDIT_DIR := ./components/audit
AUTH_DIR := ./components/auth
INFRA_DIR := ./components/infra
MDZ_DIR := ./components/mdz
ONBOARDING_DIR := ./components/onboarding
TRANSACTION_DIR := ./components/transaction

# Define a list of all component directories for easier iteration
COMPONENTS := $(AUTH_DIR) $(INFRA_DIR) $(ONBOARDING_DIR) $(TRANSACTION_DIR) $(MDZ_DIR) $(AUDIT_DIR)

BLUE := \033[36m
NC := \033[0m
BOLD := \033[1m
RED := \033[31m
MAGENTA := \033[35m
YELLOW := \033[33m
GREEN := \033[32m
CYAN := \033[36m
WHITE := \033[37m

DOCKER_VERSION := $(shell docker version --format '{{.Server.Version}}' 2>/dev/null || echo "0.0.0")
DOCKER_MIN_VERSION := 20.10.13

# Use docker compose if version is >= 20.10.13, otherwise use docker-compose
DOCKER_CMD := $(shell \
	if command -v sort -V >/dev/null 2>&1 && [ "$$(printf '%s\n%s\n' "$(DOCKER_MIN_VERSION)" "$(DOCKER_VERSION)" | sort -V | head -n1)" = "$(DOCKER_MIN_VERSION)" ]; then \
		echo "docker compose"; \
	else \
		echo "docker-compose"; \
	fi \
)

# Shell utility functions
define print_logo
	@cat $(PWD)/pkg/shell/logo.txt
endef

define border
	@echo ""; \
	len=$$(echo "$(1)" | wc -c); \
	for i in $$(seq 1 $$((len + 4))); do \
		printf "-"; \
	done; \
	echo ""; \
	echo "  $(1)  "; \
	for i in $$(seq 1 $$((len + 4))); do \
		printf "-"; \
	done; \
	echo ""
endef

define title1
	@$(call border, "📝 $(1)")
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
	$(call print_logo)
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
	@echo "  make rebuild-up                  - Rebuild and restart all services"
	@echo "  make auth COMMAND=<cmd>          - Run command in auth service"
	@echo "  make infra COMMAND=<cmd>         - Run command in infra service"
	@echo "  make onboarding COMMAND=<cmd>    - Run command in onboarding service"
	@echo "  make transaction COMMAND=<cmd>   - Run command in transaction service"
	@echo "  make audit COMMAND=<cmd>         - Run command in audit service"
	@echo "  make all-services COMMAND=<cmd>  - Run command across all services"
	@echo "  make clean-docker                - Run command to clean docker"
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
	@err=0; \
	while IFS= read -r path; do \
		if grep -q 'err != nil' "$$path" && ! grep -qE '(logger\.Error|log\.Error)' "$$path" && [[ "$$path" != *"_test"* ]]; then \
			err=1; \
			echo "$$path"; \
		fi; \
	done < <(find . -type f -path '*usecase*/*' -name '*.go'); \
	if [ $$err -eq 1 ]; then \
		echo -e "\n$(RED)You need to log all errors inside usecases after they are handled. $(BOLD)[WARNING]$(NC)\n"; \
		exit 1; \
	else \
		echo "$(GREEN)$(BOLD)[ok]$(NC) All good$(GREEN) ✔️$(NC)"; \
	fi

.PHONY: check-tests
check-tests:
	@echo "$(BLUE)Verifying test coverage for components...$(NC)"
	$(call title1,"STARTING TESTS ANALYZER")
	@err=false; \
	subdirs="components/*/internal/services/query components/*/internal/services/command"; \
	for subdir in $$subdirs; do \
		while IFS= read -r file; do \
			if [[ "$$file" != *"_test.go" ]]; then \
				test_file="$${file%.go}_test.go"; \
				if [ ! -f "$$test_file" ]; then \
					echo "Error: There is no test for the file $$file"; \
					err=true; \
				fi; \
			fi; \
		done < <(find "$$subdir" -type f -name "*.go" 2>/dev/null || echo ""); \
	done; \
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
	@for dir in $(COMPONENTS); do \
		if [ -f "$$dir/.env.example" ]; then \
			cp -r $$dir/.env.example $$dir/.env; \
			echo "$(GREEN)Created $$dir/.env from example$(NC)"; \
		else \
			echo "$(YELLOW)Warning: $$dir/.env.example not found, creating empty .env file$(NC)"; \
			touch $$dir/.env; \
		fi; \
	done
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
		$(DOCKER_CMD) -f $$dir/docker-compose.yml up --build -d; \
	done
	@echo "$(GREEN)All services started successfully$(NC)"

.PHONY: down
down:
	@echo "$(BLUE)Stopping all services with Docker Compose...$(NC)"
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	@for dir in $(COMPONENTS); do \
		$(DOCKER_CMD) -f $$dir/docker-compose.yml down; \
	done
	@echo "$(GREEN)All services stopped successfully$(NC)"

.PHONY: rebuild-up
rebuild-up:
	@echo "$(BLUE)Rebuilding and restarting all services...$(NC)"
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	$(call check_env_files)
	@for dir in $(COMPONENTS); do \
		$(DOCKER_CMD) -f $$dir/docker-compose.yml up --build --force-recreate -d; \
	done
	@echo "$(GREEN)All services rebuilt and restarted successfully$(NC)"

.PHONY: auth
auth:
	@echo "$(BLUE)Executing command in auth service...$(NC)"
	$(MAKE) -C $(AUTH_DIR) $(COMMAND)

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

.PHONY: audit
audit:
	@echo "$(BLUE)Executing command in audit service...$(NC)"
	$(MAKE) -C $(AUDIT_DIR) $(COMMAND)

.PHONY: all-services
all-services:
	@echo "$(BLUE)Executing command across all services...$(NC)"
	@for dir in $(COMPONENTS); do \
		$(MAKE) -C $$dir $(COMMAND) || exit 1; \
	done

.PHONY: clean-docker
clean-docker:
	@echo "$(BLUE)Cleaning Docker system and volumes...$(NC)"
	$(call check_command,docker,"Install Docker from https://docs.docker.com/get-docker/")
	docker system prune -a -f && docker volume prune -a -f

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
	@echo "$(BLUE)Generating Swagger documentation for all services...$(NC)"
	@for dir in $(ONBOARDING_DIR) $(TRANSACTION_DIR) $(AUDIT_DIR); do \
		$(MAKE) -C $$dir generate-docs || exit 1; \
	done
