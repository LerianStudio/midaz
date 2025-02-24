AUTH_DIR := ./components/auth
INFRA_DIR := ./components/infra
MDZ_DIR := ./components/mdz
ONBOARDING_DIR := ./components/onboarding
TRANSACTION_DIR := ./components/transaction

BLUE := \033[36m
NC := \033[0m
BOLD := \033[1m
RED := \033[31m
MAGENTA := \033[35m
YELLOW := \033[33m
GREEN := \033[32m
CYAN := \033[36m
WHITE := \033[37m

DOCKER_VERSION := $(shell docker version --format '{{.Server.Version}}')
DOCKER_MIN_VERSION := 20.10.13

DOCKER_CMD := $(shell \
	if [ "$(shell printf '%s\n' "$(DOCKER_MIN_VERSION)" "$(DOCKER_VERSION)" | sort -V | head -n1)" = "$(DOCKER_MIN_VERSION)" ]; then \
		echo "docker compose"; \
	else \
		echo "docker-compose"; \
	fi \
)

.PHONY: help
help:
	@echo ""
	@echo ""
	@echo "$(BOLD)Midaz Project Management Commands$(NC)"
	@echo ""
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make test               - Run tests on all projects"
	@echo "  make cover              - Run test coverage"
	@echo ""
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint               - Run golangci-lint and performance checks"
	@echo "  make format             - Format Go code using gofmt"
	@echo "  make check-logs         - Verify error logging in usecases"
	@echo "  make check-tests        - Verify test coverage for components"
	@echo "  make sec                - Run security checks using gosec"
	@echo ""
	@echo ""
	@echo "$(BOLD)Git Hook Commands:$(NC)"
	@echo "  make setup-git-hooks    - Install and configure git hooks"
	@echo "  make check-hooks        - Verify git hooks installation status"
	@echo ""
	@echo ""
	@echo "$(BOLD)Setup Commands:$(NC)"
	@echo "  make set-env            - Copy .env.example to .env for all components"
	@echo "Usage:"
	@echo "  ## Root Commands"
	@echo "    make build                               Build all project services."
	@echo "    make test                                Run tests on all projects."
	@echo "    make clean                               Clean the directory tree of produced artifacts."
	@echo "    make lint                                Run static code analysis (lint)."
	@echo "    make format                              Run code formatter."
	@echo "    make checkEnvs                           Check if github hooks are installed and secret env on files are not exposed."
	@echo "    make auth                                Run a command inside the auth app in the components directory to see available commands."
	@echo "    make infra                               Run a command inside the infra app in the components directory to see available commands."
	@echo "    make onboarding                              Run a command inside the onboarding app in the components directory to see available commands."
	@echo "    make transaction                         Run a command inside the transaction app in the components directory to see available commands."
	@echo "    make audit                         		Run a command inside the audit app in the components directory to see available commands."
	@echo "    make set-env                             Run a command to copy all .env.example to .env into respective folders."
	@echo "    make all-services                        Run a command to all services passing any individual container command."
	@echo "    make generate-docs-all                   Run a command to inside the onboarding and transaction app to generate swagger docs."
	@echo ""
	@echo ""
	@echo "$(BOLD)Service Commands:$(NC)"
	@echo "  make up                 - Start all services with Docker Compose"
	@echo "  make auth COMMAND=<cmd> - Run command in auth service"
	@echo "  make infra COMMAND=<cmd> - Run command in infra service"
	@echo "  make onboarding COMMAND=<cmd> - Run command in onboarding service"
	@echo "  make transaction COMMAND=<cmd> - Run command in transaction service"
	@echo "  make audit COMMAND=<cmd> - Run command in audit service"
	@echo "  make all-services COMMAND=<cmd> - Run command across all services"
	@echo "  make clean-docker - Run command to clean docker"
	@echo ""
	@echo ""
	@echo "$(BOLD)Development Commands:$(NC)"
	@echo "  make tidy               - Run go mod tidy"
	@echo "  make goreleaser         - Create a release snapshot"
	@echo ""
	@echo ""

.PHONY: test
test:
	@echo "$(BLUE)Running tests...$(NC)"
		@if ! command -v go >/dev/null 2>&1; then \
		echo "$(RED)Error: go is not installed$(NC)"; \
		exit 1; \
	fi
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

.PHONY: lint
lint:
	@echo "$(BLUE)Running linter and performance checks...$(NC)"
	./make.sh "lint"

.PHONY: format
format:
	@echo "$(BLUE)Formatting Go code...$(NC)"
	./make.sh "format"

.PHONY: check-logs
check-logs:
	@echo "$(BLUE)Checking error logging in usecases...$(NC)"
	./make.sh "checkLogs"

.PHONY: check-tests
check-tests:
	@echo "$(BLUE)Verifying test coverage...$(NC)"
	./make.sh "checkTests"

.PHONY: sec
sec:
	@echo "$(BLUE)Running security checks...$(NC)"
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(RED)Error: gosec is not installed$(NC)"; \
		echo "$(MAGENTA)To install: go install github.com/securego/gosec/v2/cmd/gosec@latest$(NC)"; \
		exit 1; \
	fi
	gosec ./...

.PHONY: setup-git-hooks
setup-git-hooks:
	@echo "$(BLUE)Setting up git hooks...$(NC)"
	./make.sh "setupGitHooks"

.PHONY: check-hooks
check-hooks:
	@echo "$(BLUE)Checking git hooks status...$(NC)"
	./make.sh "checkHooks"

.PHONY: set-env
set-env:
	@echo "$(YELLOW)WARNING:$(NC)"
	@echo "$(YELLOW)Customize .env variables to fit your environment. Default values are for initial setup and may not be secure for production. Protect sensitive info and avoid exposing .env files in public repositories.$(NC)"
	@echo "$(BLUE)Setting up environment files...$(NC)"
	cp -r $(AUTH_DIR)/.env.example $(AUTH_DIR)/.env
	cp -r $(INFRA_DIR)/.env.example $(INFRA_DIR)/.env
	cp -r $(ONBOARDING_DIR)/.env.example $(ONBOARDING_DIR)/.env
	cp -r $(TRANSACTION_DIR)/.env.example $(TRANSACTION_DIR)/.env
	cp -r $(MDZ_DIR)/.env.example $(MDZ_DIR)/.env
	@echo "$(BLUE)Environment files created successfully$(NC)"

.PHONY: up
up: 
	@echo "$(BLUE)Starting all services...$(NC)"
	@$(DOCKER_CMD) -f $(AUTH_DIR)/docker-compose.yml up --build -d
	@$(DOCKER_CMD) -f $(INFRA_DIR)/docker-compose.yml up --build -d
	@$(DOCKER_CMD) -f $(ONBOARDING_DIR)/docker-compose.yml up --build -d
	@$(DOCKER_CMD) -f $(TRANSACTION_DIR)/docker-compose.yml up --build -d
	@echo "$(BLUE)All services started successfully$(NC)"

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

.PHONY: all-services
all-services:
	@echo "$(BLUE)Executing command across all services...$(NC)"
	$(MAKE) -C $(AUTH_DIR) $(COMMAND) && \
	$(MAKE) -C $(INFRA_DIR) $(COMMAND) && \
	$(MAKE) -C $(ONBOARDING_DIR) $(COMMAND) && \
	$(MAKE) -C $(TRANSACTION_DIR) $(COMMAND)

.PHONY: clean-docker
clean-docker:
	docker system prune -a -f && docker volume prune -a -f

.PHONY: test_integration_cli
test_integration_cli:
	go test -v -tags=integration ./components/mdz/test/integration/...

.PHONY: goreleaser
goreleaser:
	@echo "$(BLUE)Creating release snapshot...$(NC)"
	goreleaser release --snapshot --skip-publish --rm-dist

.PHONY: tidy
tidy:
	@echo "$(BLUE)Running go mod tidy...$(NC)"
	go mod tidy

.PHONY: generate-docs-all
generate-docs-all:
	@echo "$(BLUE)Executing command to generate swagger...$(NC)"
	$(MAKE) -C $(ONBOARDING_DIR) generate-docs && \
	$(MAKE) -C $(TRANSACTION_DIR) generate-docs
