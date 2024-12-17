AUTH_DIR := ./components/auth
INFRA_DIR := ./components/infra
LEDGER_DIR := ./components/ledger
TRANSACTION_DIR := ./components/transaction
MDZ_DIR := ./components/mdz
AUDIT_DIR := ./components/audit

.PHONY: help test cover lint format check-logs check-tests \
        setup-git-hooks check-hooks goreleaser tidy sec set-env up auth infra ledger \
        transaction audit all-services

BLUE := \033[36m
NC := \033[0m
BOLD := \033[1m
RED := \033[31m
MAGENTA := \033[35m

help:
	@echo "$(BOLD)Midaz Project Management Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make test               - Run tests on all projects"
	@echo "  make cover              - Run test coverage"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint               - Run golangci-lint and performance checks"
	@echo "  make format             - Format Go code using gofmt"
	@echo "  make check-logs         - Verify error logging in usecases"
	@echo "  make check-tests        - Verify test coverage for components"
	@echo "  make sec                - Run security checks using gosec"
	@echo ""
	@echo "$(BOLD)Git Hook Commands:$(NC)"
	@echo "  make setup-git-hooks    - Install and configure git hooks"
	@echo "  make check-hooks        - Verify git hooks installation status"
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
	@echo "    make ledger                              Run a command inside the ledger app in the components directory to see available commands."
	@echo "    make transaction                         Run a command inside the transaction app in the components directory to see available commands."
	@echo "    make audit                         		Run a command inside the audit app in the components directory to see available commands."
	@echo "    make set-env                             Run a command to copy all .env.example to .env into respective folders."
	@echo "    make all-services                        Run a command to all services passing any individual container command."
	@echo "    make generate-docs-all                   Run a command to inside the ledger and transaction app to generate swagger docs."
	@echo ""
	@echo "$(BOLD)Service Commands:$(NC)"
	@echo "  make up                 - Start all services with Docker Compose"
	@echo "  make auth COMMAND=<cmd> - Run command in auth service"
	@echo "  make infra COMMAND=<cmd> - Run command in infra service"
	@echo "  make ledger COMMAND=<cmd> - Run command in ledger service"
	@echo "  make transaction COMMAND=<cmd> - Run command in transaction service"
	@echo "  make audit COMMAND=<cmd> - Run command in audit service"
	@echo "  make all-services COMMAND=<cmd> - Run command across all services"
	@echo ""
	@echo "$(BOLD)Development Commands:$(NC)"
	@echo "  make tidy               - Run go mod tidy"
	@echo "  make goreleaser         - Create a release snapshot"

test:
	@echo "$(BLUE)Running tests...$(NC)"
		@if ! command -v go >/dev/null 2>&1; then \
		echo "$(RED)Error: go is not installed$(NC)"; \
		exit 1; \
	fi
	go test -v ./... ./...

cover:
	@echo -e "$(BLUE)Generating test coverage...$(NC)"
	@if ! command -v go >/dev/null 2>&1; then \
		echo "$(RED)Error: go is not installed$(NC)"; \
		exit 1; \
	fi
	@sh ./scripts/coverage.sh
	@go tool cover -html=coverage.out -o coverage.html

lint:
	@echo "$(BLUE)Running linter and performance checks...$(NC)"
	./make.sh "lint"

format:
	@echo "$(BLUE)Formatting Go code...$(NC)"
	./make.sh "format"

check-logs:
	@echo "$(BLUE)Checking error logging in usecases...$(NC)"
	./make.sh "checkLogs"

check-tests:
	@echo "$(BLUE)Verifying test coverage...$(NC)"
	./make.sh "checkTests"

sec:
	@echo "$(BLUE)Running security checks...$(NC)"
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(RED)Error: gosec is not installed$(NC)"; \
		echo "$(MAGENTA)To install: go install github.com/securego/gosec/v2/cmd/gosec@latest$(NC)"; \
		exit 1; \
	fi
	gosec ./...

setup-git-hooks:
	@echo "$(BLUE)Setting up git hooks...$(NC)"
	./make.sh "setupGitHooks"

check-hooks:
	@echo "$(BLUE)Checking git hooks status...$(NC)"
	./make.sh "checkHooks"

set-env:
	@echo "$(BLUE)Setting up environment files...$(NC)"
	cp -r $(AUTH_DIR)/.env.example $(AUTH_DIR)/.env
	cp -r $(INFRA_DIR)/.env.example $(INFRA_DIR)/.env
	cp -r $(LEDGER_DIR)/.env.example $(LEDGER_DIR)/.env
	cp -r $(TRANSACTION_DIR)/.env.example $(TRANSACTION_DIR)/.env
	cp -r $(MDZ_DIR)/.env.example $(MDZ_DIR)/.env
	cp -r $(AUDIT_DIR)/.env.example $(AUDIT_DIR)/.env
	@echo "$(BLUE)Environment files created successfully$(NC)"

up: 
	@echo "$(BLUE)Starting all services...$(NC)"
	docker-compose -f $(AUTH_DIR)/docker-compose.yml up --build -d && \
	docker-compose -f $(INFRA_DIR)/docker-compose.yml up --build -d && \
	docker-compose -f $(LEDGER_DIR)/docker-compose.yml up --build -d && \
	docker-compose -f $(TRANSACTION_DIR)/docker-compose.yml up --build -d 
	@echo "$(BLUE)All services started successfully$(NC)"

auth:
	@echo "$(BLUE)Executing command in auth service...$(NC)"
	$(MAKE) -C $(AUTH_DIR) $(COMMAND)

infra:
	@echo "$(BLUE)Executing command in infra service...$(NC)"
	$(MAKE) -C $(INFRA_DIR) $(COMMAND)

ledger:
	@echo "$(BLUE)Executing command in ledger service...$(NC)"
	$(MAKE) -C $(LEDGER_DIR) $(COMMAND)

transaction:
	@echo "$(BLUE)Executing command in transaction service...$(NC)"
	$(MAKE) -C $(TRANSACTION_DIR) $(COMMAND)

audit:
	@echo "$(BLUE)Executing command in audit service...$(NC)"
	$(MAKE) -C $(AUDIT_DIR) $(COMMAND)

all-services:
	@echo "$(BLUE)Executing command across all services...$(NC)"
	$(MAKE) -C $(AUTH_DIR) $(COMMAND) && \
	$(MAKE) -C $(INFRA_DIR) $(COMMAND) && \
	$(MAKE) -C $(LEDGER_DIR) $(COMMAND) && \
	$(MAKE) -C $(TRANSACTION_DIR) $(COMMAND) && \
	$(MAKE) -C $(AUDIT_DIR) $(COMMAND)

test_integration_cli:
	go test -v -tags=integration ./components/mdz/test/integration/...

goreleaser:
	@echo "$(BLUE)Creating release snapshot...$(NC)"
	goreleaser release --snapshot --skip-publish --rm-dist

tidy:
	@echo "$(BLUE)Running go mod tidy...$(NC)"
	go mod tidy

generate-docs-all:
	@echo "$(BLUE)Executing command to generate swagger...$(NC)"
	$(MAKE) -C $(LEDGER_DIR) generate-docs && \
	$(MAKE) -C $(TRANSACTION_DIR) generate-docs && \
	$(MAKE) -C $(AUDIT_DIR) generate-docs
