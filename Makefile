# Directories
AUDIT_DIR := ./components/audit
AUTH_DIR := ./components/auth
INFRA_DIR := ./components/infra
MDZ_DIR := ./components/mdz
LEDGER_DIR := ./components/ledger
TRANSACTION_DIR := ./components/transaction

# Use tput if available, fallback to ANSI codes if not
BLUE := $(shell command -v tput >/dev/null 2>&1 && tput setaf 4 || echo '\033[0;34m')
NC := $(shell command -v tput >/dev/null 2>&1 && tput sgr0 || echo '\033[0m')
BOLD := $(shell command -v tput >/dev/null 2>&1 && tput bold || echo '\033[1m')
RED := $(shell command -v tput >/dev/null 2>&1 && tput setaf 1 || echo '\033[0;31m')
MAGENTA := $(shell command -v tput >/dev/null 2>&1 && tput setaf 5 || echo '\033[0;35m')
GREEN := $(shell command -v tput >/dev/null 2>&1 && tput setaf 2 || echo '\033[0;32m')
YELLOW := $(shell command -v tput >/dev/null 2>&1 && tput setaf 3 || echo '\033[0;33m')

DOCKER_VERSION := $(shell docker version --format '{{.Server.Version}}')
DOCKER_MIN_VERSION := 20.10.13

DOCKER_CMD := $(shell \
	if [ "$(shell printf '%s\n' "$(DOCKER_MIN_VERSION)" "$(DOCKER_VERSION)" | sort -V | head -n1)" = "$(DOCKER_MIN_VERSION)" ]; then \
		echo "docker compose"; \
	else \
		echo "docker-compose"; \
	fi \
)

# Variables for git hooks
GITHOOKS_PATH := ./.githooks
GIT_HOOKS_PATH := ./.git/hooks

# Check for required dependencies
DOCKER_AVAILABLE := $(shell command -v docker >/dev/null 2>&1 && echo 1 || echo 0)
GO_AVAILABLE := $(shell command -v go >/dev/null 2>&1 && echo 1 || echo 0)

# If Go is available, check if GOPATH is in PATH
ifeq ($(GO_AVAILABLE),1)
	GOPATH := $(shell go env GOPATH)
	PATH_WITH_GOPATH := $(PATH):$(GOPATH)/bin
	GOLANGCI_LINT_AVAILABLE := $(shell PATH="$(PATH_WITH_GOPATH)" command -v golangci-lint >/dev/null 2>&1 && echo 1 || echo 0)
	GORELEASER_AVAILABLE := $(shell PATH="$(PATH_WITH_GOPATH)" command -v goreleaser >/dev/null 2>&1 && echo 1 || echo 0)
	GOSEC_AVAILABLE := $(shell PATH="$(PATH_WITH_GOPATH)" command -v gosec >/dev/null 2>&1 && echo 1 || echo 0)
	PERFSPRINT_AVAILABLE := $(shell PATH="$(PATH_WITH_GOPATH)" command -v perfsprint >/dev/null 2>&1 && echo 1 || echo 0)
else
	GOLANGCI_LINT_AVAILABLE := 0
	GORELEASER_AVAILABLE := 0
	GOSEC_AVAILABLE := 0
	PERFSPRINT_AVAILABLE := 0
endif

# Installation instructions
DOCKER_INSTALL_MSG := "Docker is not installed. Visit https://docs.docker.com/get-docker/ to install"
GO_INSTALL_MSG := "Go is not installed. Visit https://golang.org/doc/install to install"
GOLANGCI_LINT_INSTALL_MSG := "golangci-lint is not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
GORELEASER_INSTALL_MSG := "goreleaser is not installed. Visit https://goreleaser.com/install/ to install"
GOSEC_INSTALL_MSG := "gosec is not installed. Run: go install github.com/securego/gosec/v2/cmd/gosec@latest"
PERFSPRINT_INSTALL_MSG := "perfsprint is not installed. Run: go install github.com/sagikazarmark/perfsprint@latest"

.PHONY: help
help:
	@echo "$(BOLD)$(YELLOW)MIDAZ PROJECT MANAGEMENT COMMANDS$(NC)"
	@echo "$(YELLOW)* midaz is licensed under the apache 2.0 license$(NC)"
	@echo "$(YELLOW)* visit: https://github.com/lerianstudio/midaz$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@if [ $(GO_AVAILABLE) -eq 1 ]; then \
		echo "- make test                          - Run tests on all projects"; \
		echo "- make cover                         - Run test coverage"; \
	else \
		echo "$(RED)Go commands unavailable - install Go first$(NC)"; \
	fi
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@if [ $(GOLANGCI_LINT_AVAILABLE) -eq 1 ] && [ $(PERFSPRINT_AVAILABLE) -eq 1 ]; then \
		echo "- make lint                          - Run golangci-lint and performance checks"; \
	fi
	@if [ $(GO_AVAILABLE) -eq 1 ]; then \
		echo "- make format                        - Format Go code using gofmt"; \
		echo "- make check-logs                    - Verify error logging in usecases"; \
		echo "- make check-tests                   - Verify test coverage for components"; \
	fi
	@if [ $(GOSEC_AVAILABLE) -eq 1 ]; then \
		echo "- make sec                           - Run security checks using gosec"; \
	fi
	@echo ""
	@echo "$(BOLD)Git Hook Commands:$(NC)"
	@echo "- make setup-git-hooks               - Install and configure git hooks"
	@echo "- make check-hooks                   - Verify git hooks installation status"
	@echo ""
	@echo "$(BOLD)Setup Commands:$(NC)"
	@echo "- make set-env                       - Copy .env.example to .env for all components"
	@echo "- make check-dependencies            - Check status of required dependencies"
	@echo ""
	@echo "$(BOLD)Service Commands:$(NC)"
	@if [ $(DOCKER_AVAILABLE) -eq 1 ]; then \
		echo "- make up                            - Start all services"; \
		echo "- make stop                          - Stop all services containers"; \
		echo "- make down                          - Remove all services containers"; \
		echo "- make status                        - Show status of running containers"; \
		echo "- make mdz-build                     - Build mdz (Midaz CLI) and install it locally"; \
		echo ""; \
		echo "- make auth COMMAND=<cmd>            - Run command in auth service"; \
		echo "- make infra COMMAND=<cmd>           - Run command in infra service"; \
		echo "- make ledger COMMAND=<cmd>          - Run command in ledger service"; \
		echo "- make transaction COMMAND=<cmd>     - Run command in transaction service"; \
		echo "- make audit COMMAND=<cmd>           - Run command in audit service"; \
		echo "- make mdz COMMAND=<cmd>             - Run command in mdz service"; \
		echo "- make all-services COMMAND=<cmd>    - Run command across all services"; \
		echo "- make clean-docker                  - Run command to clean docker"; \
	else \
		echo "$(RED)Docker commands unavailable - install Docker first$(NC)"; \
	fi
	@echo ""
	@echo "$(BOLD)Development Commands:$(NC)"
	@if [ $(GO_AVAILABLE) -eq 1 ]; then \
		echo "- make tidy                          - Run go mod tidy"; \
	fi
	@if [ $(GORELEASER_AVAILABLE) -eq 1 ]; then \
		echo "- make goreleaser                    - Create a release snapshot"; \
	fi
	@echo ""

# Check Dependencies
.PHONY: check-dependencies
check-dependencies:
	@echo "$(BOLD)Required Dependencies Status:$(NC)"
	@if [ $(DOCKER_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)✗ Docker: $(DOCKER_INSTALL_MSG)$(NC)"; \
	else \
		echo "$(BLUE)✓ Docker$(NC)"; \
	fi
	@if [ $(GO_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)✗ Go: $(GO_INSTALL_MSG)$(NC)"; \
	else \
		echo "$(BLUE)✓ Go$(NC)"; \
		if ! echo "$(PATH)" | grep -q "$(shell go env GOPATH)/bin"; then \
			echo "$(RED)⚠ Warning: GOPATH is not in your PATH. Here's how to add it:$(NC)"; \
			echo "$(RED)For Linux/macOS:$(NC)"; \
			echo "$(RED)1. Edit ~/.bashrc (bash) or ~/.zshrc (zsh)$(NC)"; \
			echo "$(RED)2. Add these lines:$(NC)"; \
			echo "$(RED)   export GOPATH=\$$HOME/go$(NC)"; \
			echo "$(RED)   export PATH=\$$PATH:\$$GOPATH/bin$(NC)"; \
			echo "$(RED)3. Run: source ~/.bashrc (or ~/.zshrc)$(NC)"; \
			echo "$(RED)For Windows:$(NC)"; \
			echo "$(RED)1. Open System Properties > Advanced > Environment Variables$(NC)"; \
			echo "$(RED)2. Add GOPATH=%%USERPROFILE%%\\go$(NC)"; \
			echo "$(RED)3. Add %%GOPATH%%\\bin to PATH$(NC)"; \
		fi; \
	fi
	@if [ $(GOLANGCI_LINT_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)✗ golangci-lint: $(GOLANGCI_LINT_INSTALL_MSG)$(NC)"; \
	else \
		echo "$(BLUE)✓ golangci-lint$(NC)"; \
	fi
	@if [ $(GORELEASER_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)✗ goreleaser: $(GORELEASER_INSTALL_MSG)$(NC)"; \
	else \
		echo "$(BLUE)✓ goreleaser$(NC)"; \
	fi
	@if [ $(GOSEC_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)✗ gosec: $(GOSEC_INSTALL_MSG)$(NC)"; \
	else \
		echo "$(BLUE)✓ gosec$(NC)"; \
	fi
	@if [ $(PERFSPRINT_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)✗ perfsprint: $(PERFSPRINT_INSTALL_MSG)$(NC)"; \
	else \
		echo "$(BLUE)✓ perfsprint$(NC)"; \
	fi

# Core Commands
.PHONY: test
test:
	@if [ $(GO_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)Error: $(GO_INSTALL_MSG)$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Running tests...$(NC)"
	go test -v ./... ./...

.PHONY: cover
cover:
	@echo -e "$(BLUE)Generating test coverage...$(NC)"
	@if ! command -v go >/dev/null 2>&1; then \
		echo "$(RED)Error: go is not installed$(NC)"; \
		exit 1; \
	fi
	@sh ./scripts/coverage.sh
	@go tool cover -html=coverage.out -o coverage.html

# Code Quality Commands
.PHONY: lint
lint:
	@if [ $(GOLANGCI_LINT_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)Error: $(GOLANGCI_LINT_INSTALL_MSG)$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Running linter and performance checks...$(NC)"
	@out=$$(golangci-lint run --fix ./... 2>&1); \
	out_err=$$?; \
	perf_out=$$(perfsprint ./... 2>&1); \
	perf_err=$$?; \
	echo "$$out"; \
	echo "$$perf_out"; \
	if [ $$out_err -ne 0 ]; then \
		echo "$(RED)$(BOLD)An error occurred during lint: $$out$(NC)"; \
		exit 1; \
	fi; \
	if [ $$perf_err -ne 0 ]; then \
		echo "$(RED)$(BOLD)An error occurred during performance check: $$perf_out$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Lint and performance checks passed successfully$(NC)"

.PHONY: format
format:
	@echo "$(BLUE)Formatting Go code...$(NC)"
	@if ! command -v gofmt >/dev/null 2>&1; then \
		echo "$(RED)Error: gofmt is not installed. Please install Go first.$(NC)"; \
		exit 1; \
	fi
	@gofmt -w ./
	@echo "$(BLUE)All go files formatted$(NC)"a

.PHONY: check-logs
check-logs:
	@echo "$(BLUE)Checking error logging in usecases...$(NC)"
	@err=0; \
	while IFS= read -r path; do \
		if grep -q 'err != nil' "$$path" && ! grep -qE '(logger\.Error|log\.Error)' "$$path" && [[ "$$path" != *"_test"* ]]; then \
			err=1; \
			echo "$$path"; \
		fi \
	done < <(find . -type f -path '*usecase*/*' -name '*.go'); \
	if [ $$err -eq 1 ]; then \
		echo "$(RED)You need to log all errors inside usecases after they are handled. $(BOLD)[WARNING]$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)All good$(NC)"

.PHONY: check-tests
check-tests:
	@echo "$(BLUE)Verifying test coverage...$(NC)"
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
		done < <(find "$$subdir" -type f -name "*.go"); \
	done; \
	if [ "$$err" = true ]; then \
		echo "$(RED)There are files without corresponding test files.$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)All tests are in place$(NC)"

.PHONY: sec
sec:
	@echo "$(BLUE)Running security checks...$(NC)"
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(RED)Error: gosec is not installed$(NC)"; \
		echo "$(MAGENTA)To install: go install github.com/securego/gosec/v2/cmd/gosec@latest$(NC)"; \
		exit 1; \
	fi
	gosec ./...

# Git Hook Commands
.PHONY: setup-git-hooks
setup-git-hooks:
	@echo "$(BLUE)Setting up git hooks...$(NC)"
	@for hook_dir in "$(GITHOOKS_PATH)"/*; do \
		if [ -d "$$hook_dir" ]; then \
			hook_name="$$(basename -- $$hook_dir)"; \
			hook_file="$$hook_dir/$$hook_name"; \
			if [ -f "$$hook_file" ]; then \
				cp "$$hook_file" "$(GIT_HOOKS_PATH)/$$hook_name"; \
				chmod +x "$(GIT_HOOKS_PATH)/$$hook_name"; \
				echo "Installed $$hook_name hook"; \
			fi; \
		fi; \
	done
	@echo "$(BLUE)All hooks installed and updated$(NC)"

.PHONY: check-hooks
check-hooks:
	@echo "$(BLUE)Checking git hooks status...$(NC)"
	@err=0; \
	for hook_dir in "$(GITHOOKS_PATH)"/*; do \
		if [ -d "$$hook_dir" ]; then \
			hook_name="$$(basename -- $$hook_dir)"; \
			hook_file="$$hook_dir/$$hook_name"; \
			target_file="$(GIT_HOOKS_PATH)/$$hook_name"; \
			if [ -f "$$hook_file" ]; then \
				if [ -f "$$target_file" ]; then \
					if cmp -s "$$hook_file" "$$target_file"; then \
						echo "$(BLUE)Hook file $$hook_name installed and updated$(NC)"; \
					else \
						echo "$(RED)Hook file $$hook_name installed but out-of-date [OUT-OF-DATE]$(NC)"; \
						err=1; \
					fi; \
				else \
					echo "$(RED)Hook file $$hook_name not installed [NOT INSTALLED]$(NC)"; \
					err=1; \
				fi; \
			fi; \
		fi; \
	done; \
	if [ $$err -ne 0 ]; then \
		echo "Run 'make setup-git-hooks' to setup your development environment, then try again."; \
		exit 1; \
	fi
	@echo "$(BLUE)All good$(NC)"

# Setup Commands
.PHONY: set-env
set-env:
	@echo "$(BLUE)Setting up environment files...$(NC)"
	cp -r $(AUTH_DIR)/.env.example $(AUTH_DIR)/.env
	cp -r $(INFRA_DIR)/.env.example $(INFRA_DIR)/.env
	cp -r $(LEDGER_DIR)/.env.example $(LEDGER_DIR)/.env
	cp -r $(TRANSACTION_DIR)/.env.example $(TRANSACTION_DIR)/.env
	cp -r $(MDZ_DIR)/.env.example $(MDZ_DIR)/.env
	cp -r $(AUDIT_DIR)/.env.example $(AUDIT_DIR)/.env
	@echo "$(BLUE)Environment files created successfully$(NC)"

# Root Commands
.PHONY: build
build:
	@echo "$(BLUE)Building all project services...$(NC)"
	$(MAKE) all-services COMMAND=build

.PHONY: clean
clean:
	@echo "$(BLUE)Cleaning produced artifacts...$(NC)"
	@find . -name "coverage.out" -type f -delete
	@find . -name "coverage.html" -type f -delete
	@find . -name "dist" -type d -exec rm -rf {} +
	@go clean -cache -testcache

.PHONY: check-envs
check-envs:
	@echo "$(BLUE)Checking environment files and git hooks...$(NC)"
	./make.sh "checkEnvs"

.PHONY: generate-docs-all
generate-docs-all:
	@echo "$(BLUE)Executing command to generate swagger...$(NC)"
	$(MAKE) -C $(LEDGER_DIR) generate-docs && \
	$(MAKE) -C $(TRANSACTION_DIR) generate-docs && \
	$(MAKE) -C $(AUDIT_DIR) generate-docs

# Service Commands
.PHONY: up
up:
	@if [ $(DOCKER_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)Error: $(DOCKER_INSTALL_MSG)$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Checking environment files...$(NC)"
	@if [ ! -f "$(AUTH_DIR)/.env" ] || [ ! -f "$(INFRA_DIR)/.env" ] || [ ! -f "$(LEDGER_DIR)/.env" ] || [ ! -f "$(TRANSACTION_DIR)/.env" ] || [ ! -f "$(AUDIT_DIR)/.env" ]; then \
		echo "$(RED)Error: Environment files missing. Run 'make set-env' first$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Starting all services containers...$(NC)"
	@$(DOCKER_CMD) -f $(AUTH_DIR)/docker-compose.yml up --build -d
	@$(DOCKER_CMD) -f $(INFRA_DIR)/docker-compose.yml up --build -d
	@$(DOCKER_CMD) -f $(LEDGER_DIR)/docker-compose.yml up --build -d
	@$(DOCKER_CMD) -f $(TRANSACTION_DIR)/docker-compose.yml up --build -d
	@$(DOCKER_CMD) -f $(AUDIT_DIR)/docker-compose.yml up --build -d
	@echo "$(BLUE)All services started successfully$(NC)"

.PHONY: status
status:
	@if [ $(DOCKER_AVAILABLE) -eq 0 ]; then \
		echo "$(RED)Error: $(DOCKER_INSTALL_MSG)$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Checking environment files...$(NC)"
	@if [ ! -f "$(AUTH_DIR)/.env" ] || [ ! -f "$(INFRA_DIR)/.env" ] || [ ! -f "$(LEDGER_DIR)/.env" ] || [ ! -f "$(TRANSACTION_DIR)/.env" ] || [ ! -f "$(AUDIT_DIR)/.env" ]; then \
		echo "$(RED)Error: Environment files missing. Run 'make set-env' first$(NC)"; \
		exit 1; \
	fi
	@echo "$(BOLD)Midaz Services Status:$(NC)"
	@echo ""
	@echo "$(BOLD)Auth Service:$(NC)"
	@$(DOCKER_CMD) -f $(AUTH_DIR)/docker-compose.yml ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || true
	@echo ""
	@echo "$(BOLD)Infra Service:$(NC)"
	@$(DOCKER_CMD) -f $(INFRA_DIR)/docker-compose.yml ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || true
	@echo ""
	@echo "$(BOLD)Ledger Service:$(NC)"
	@$(DOCKER_CMD) -f $(LEDGER_DIR)/docker-compose.yml ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || true
	@echo ""
	@echo "$(BOLD)Transaction Service:$(NC)"
	@$(DOCKER_CMD) -f $(TRANSACTION_DIR)/docker-compose.yml ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || true
	@echo ""
	@echo "$(BOLD)Audit Service:$(NC)"
	@$(DOCKER_CMD) -f $(AUDIT_DIR)/docker-compose.yml ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || true

.PHONY: stop
stop:
	@echo "$(BLUE)Stopping all services containers...$(NC)"
	@$(DOCKER_CMD) -f $(AUTH_DIR)/docker-compose.yml stop
	@$(DOCKER_CMD) -f $(INFRA_DIR)/docker-compose.yml stop
	@$(DOCKER_CMD) -f $(LEDGER_DIR)/docker-compose.yml stop
	@$(DOCKER_CMD) -f $(TRANSACTION_DIR)/docker-compose.yml stop
	@$(DOCKER_CMD) -f $(AUDIT_DIR)/docker-compose.yml stop
	@echo "$(BLUE)All services containers stopped successfully$(NC)"

.PHONY: down
down:
	@echo "$(BLUE)Removing all services containers...$(NC)"
	@$(DOCKER_CMD) -f $(AUTH_DIR)/docker-compose.yml down
	@$(DOCKER_CMD) -f $(INFRA_DIR)/docker-compose.yml down
	@$(DOCKER_CMD) -f $(LEDGER_DIR)/docker-compose.yml down
	@$(DOCKER_CMD) -f $(TRANSACTION_DIR)/docker-compose.yml down
	@$(DOCKER_CMD) -f $(AUDIT_DIR)/docker-compose.yml down
	@echo "$(BLUE)All services containers removed successfully$(NC)"

.PHONY: auth
auth:
	@echo "$(BLUE)Executing command in auth service...$(NC)"
	$(MAKE) -C $(AUTH_DIR) $(COMMAND)

.PHONY: infra
infra:
	@echo "$(BLUE)Executing command in infra service...$(NC)"
	$(MAKE) -C $(INFRA_DIR) $(COMMAND)

.PHONY: ledger
ledger:
	@echo "$(BLUE)Executing command in ledger service...$(NC)"
	$(MAKE) -C $(LEDGER_DIR) $(COMMAND)

.PHONY: transaction
transaction:
	@echo "$(BLUE)Executing command in transaction service...$(NC)"
	$(MAKE) -C $(TRANSACTION_DIR) $(COMMAND)

.PHONY: audit
audit:
	@echo "$(BLUE)Executing command in audit service...$(NC)"
	$(MAKE) -C $(AUDIT_DIR) $(COMMAND)

.PHONY: mdz
mdz:
	@echo "$(BLUE)Executing command in mdz service...$(NC)"
	$(MAKE) -C $(MDZ_DIR) $(COMMAND)

.PHONY: mdz-build
mdz-build:
	@if [ -f "$(MDZ_DIR)/bin/mdz" ]; then \
		echo "$(BLUE)MDZ binary already exists. Do you want to rebuild it? [y/N]$(NC)"; \
		read -r response; \
		if [[ $$response =~ ^[Yy]$$ ]]; then \
			echo "$(BLUE)Rebuilding mdz (Midaz CLI)...$(NC)"; \
			$(MAKE) -C $(MDZ_DIR) build; \
		fi; \
	else \
		echo "$(BLUE)Building mdz (Midaz CLI)...$(NC)"; \
		$(MAKE) -C $(MDZ_DIR) build; \
	fi
	@echo "$(BLUE)Would you like to install MDZ CLI locally? This will require elevated privileges. [y/N]$(NC)"
	@read -r response; \
	if [[ $$response =~ ^[Yy]$$ ]]; then \
		echo "$(BLUE)Installing mdz (Midaz CLI) locally...$(NC)"; \
		echo "$(BLUE)We need to run this command as root, please enter your password.$(NC)"; \
		$(MAKE) -C $(MDZ_DIR) install-local; \
	else \
		echo "$(YELLOW)Skipping local installation. You can find the binary at: $(MDZ_DIR)/bin/mdz$(NC)"; \
	fi

.PHONY: all-services
all-services:
	@echo "$(BLUE)Executing command across all services...$(NC)"
	$(MAKE) -C $(AUTH_DIR) $(COMMAND) && \
	$(MAKE) -C $(INFRA_DIR) $(COMMAND) && \
	$(MAKE) -C $(LEDGER_DIR) $(COMMAND) && \
	$(MAKE) -C $(TRANSACTION_DIR) $(COMMAND) && \
	$(MAKE) -C $(AUDIT_DIR) $(COMMAND) && \
	$(MAKE) -C $(MDZ_DIR) $(COMMAND)

.PHONY: clean-docker
clean-docker:
	docker system prune -a -f && docker volume prune -a -f

# Development Commands
.PHONY: tidy
tidy:
	@echo "$(BLUE)Running go mod tidy...$(NC)"
	go mod tidy

.PHONY: goreleaser
goreleaser:
	@echo "$(BLUE)Creating release snapshot...$(NC)"
	goreleaser release --snapshot --clean

# Additional Commands (not in help)
.PHONY: test_integration_cli
test_integration_cli:
	go test -v -tags=integration ./components/mdz/test/integration/...
