# Reconciliation Component Makefile

# Component-specific variables
SERVICE_NAME := reconciliation-service
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts

# Optional service filter for Docker commands.
# Override on command line to target specific service(s), e.g.: make up c=reconciliation
# When empty (default), commands apply to all services in the compose file.
c ?=

# Ensure artifacts directory exists
$(shell mkdir -p $(ARTIFACTS_DIR))

# Define the root directory of the project
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)

# Terminal color definitions (guarded for non-TTY environments)
# Only set ANSI codes when stdout is a terminal to avoid garbled output in logs/CI
ifeq ($(shell [ -t 1 ] && echo yes),yes)
  BOLD   := $(shell printf '\033[1m')
  NC     := $(shell printf '\033[0m')
  GREEN  := $(shell printf '\033[32m')
  CYAN   := $(shell printf '\033[36m')
  YELLOW := $(shell printf '\033[33m')
else
  BOLD   :=
  NC     :=
  GREEN  :=
  CYAN   :=
  YELLOW :=
endif

# Docker command detection
DOCKER_CMD := $(shell command -v docker compose >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

# Define local utility functions
define title1
	@echo ""
	@echo "------------------------"
	@echo "   $(1)  "
	@echo "------------------------"
endef

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)Reconciliation Service Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make help                        - Display this help message"
	@echo "  make build                       - Build the component"
	@echo "  make test                        - Run tests"
	@echo "  make clean                       - Clean build artifacts"
	@echo "  make run                         - Run the application with .env config"
	@echo "  make cover-html                  - Generate HTML test coverage report"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run linting tools"
	@echo "  make format                      - Format code"
	@echo "  make tidy                        - Clean dependencies"
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "  make up                          - Start services with Docker Compose"
	@echo "  make down                        - Stop services with Docker Compose"
	@echo "  make start                       - Start existing containers"
	@echo "  make stop                        - Stop running containers"
	@echo "  make restart                     - Restart all containers"
	@echo "  make logs                        - Show logs for all services"
	@echo "  make logs-api                    - Show logs for reconciliation service"
	@echo "  make ps                          - List container status"
	@echo "  make rebuild-up                  - Rebuild and restart services during development"
	@echo "  make clean-docker                - Clean all Docker resources (containers, networks, volumes)"
	@echo "  make destroy                     - Alias for clean-docker (maintained for compatibility)"
	@echo ""
	@echo "  $(YELLOW)Tip: Use c=<service> to target specific services, e.g.: make up c=reconciliation$(NC)"
	@echo ""
	@echo "$(BOLD)Development (Hot-Reload) Commands:$(NC)"
	@echo "  make up-dev                      - Start services in dev mode with hot-reload"
	@echo "  make down-dev                    - Stop dev services"
	@echo "  make rebuild-up-dev              - Rebuild and restart dev services"
	@echo "  make restart-dev                 - Restart dev services"
	@echo "  make logs-dev                    - Show logs for dev services"
	@echo "  make ps-dev                      - List dev container status"
	@echo ""
	@echo "$(BOLD)Reconciliation-Specific Commands:$(NC)"
	@echo "  make status                      - Get reconciliation status"
	@echo "  make report                      - Get full reconciliation report"
	@echo "  make trigger                     - Trigger manual reconciliation run"
	@echo ""

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call title1,"Building Reconciliation component")
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/$(SERVICE_NAME) ./cmd/app/main.go
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Build completed successfully$(GREEN) $(NC)"

#-------------------------------------------------------
# Test Commands
#-------------------------------------------------------

.PHONY: test
test:
	$(call title1,"Running tests")
	@go test -v -race ./... && echo "$(GREEN)$(BOLD)[ok]$(NC) Tests completed successfully$(GREEN) $(NC)"

.PHONY: cover-html
cover-html:
	$(call title1,"Generating HTML test coverage report")
	@go test -coverprofile=$(ARTIFACTS_DIR)/coverage.out ./...
	@go tool cover -html=$(ARTIFACTS_DIR)/coverage.out -o $(ARTIFACTS_DIR)/coverage.html
	@echo "$(GREEN)Coverage report generated at $(ARTIFACTS_DIR)/coverage.html$(NC)"
	@echo ""
	@echo "$(CYAN)Coverage Summary:$(NC)"
	@echo "$(CYAN)----------------------------------------$(NC)"
	@go tool cover -func=$(ARTIFACTS_DIR)/coverage.out | grep total | awk '{print "Total coverage: " $$3}'
	@echo "$(CYAN)----------------------------------------$(NC)"
	@echo "$(YELLOW)Open $(ARTIFACTS_DIR)/coverage.html in your browser to view detailed coverage report$(NC)"

#-------------------------------------------------------
# Code Quality Commands
#-------------------------------------------------------

.PHONY: lint
lint:
	$(call title1,"Running linters")
	@if find . -name "*.go" -type f | grep -q .; then \
		if ! command -v golangci-lint >/dev/null 2>&1; then \
			echo "$(YELLOW)golangci-lint not found, installing...$(NC)"; \
				go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		else \
			echo "$(GREEN)golangci-lint already installed $(NC)"; \
		fi; \
		golangci-lint run --fix ./...; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Linting completed successfully$(GREEN) $(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping linting$(NC)"; \
	fi

.PHONY: format
format:
	$(call title1,"Formatting code")
	@go fmt ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Formatting completed successfully$(GREEN) $(NC)"

.PHONY: tidy
tidy:
	$(call title1,"Cleaning dependencies")
	@go mod tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dependencies cleaned successfully$(GREEN) $(NC)"

#-------------------------------------------------------
# Clean Commands
#-------------------------------------------------------

.PHONY: clean
clean:
	$(call title1,"Cleaning build artifacts")
	@rm -rf $(BIN_DIR)/* $(ARTIFACTS_DIR)/*
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Artifacts cleaned successfully$(GREEN) $(NC)"

#-------------------------------------------------------
# Docker Commands
#-------------------------------------------------------

.PHONY: build-docker
build-docker:
	$(call title1,"Building Docker images")
	@$(DOCKER_CMD) -f docker-compose.yml build $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Docker images built successfully$(GREEN) $(NC)"

.PHONY: up
up:
	$(call title1,"Starting all services in detached mode")
	@$(DOCKER_CMD) -f docker-compose.yml up $(c) -d
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services started successfully$(GREEN) $(NC)"

.PHONY: start
start:
	$(call title1,"Starting existing containers")
	@$(DOCKER_CMD) -f docker-compose.yml start $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Containers started successfully$(GREEN) $(NC)"

.PHONY: down
down:
	$(call title1,"Stopping and removing containers|networks|volumes")
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml down $(c); \
	else \
		echo "$(YELLOW)No docker-compose.yml file found. Skipping down command.$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services stopped successfully$(GREEN) $(NC)"

.PHONY: destroy
destroy: clean-docker
	@echo "$(YELLOW)Note: 'make destroy' is now an alias for 'make clean-docker'$(NC)"

.PHONY: stop
stop:
	$(call title1,"Stopping running containers")
	@$(DOCKER_CMD) -f docker-compose.yml stop $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Containers stopped successfully$(GREEN) $(NC)"

.PHONY: restart
restart:
	$(call title1,"Restarting all services")
	@make stop && make up
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services restarted successfully$(GREEN) $(NC)"

.PHONY: rebuild-up
rebuild-up:
	$(call title1,"Rebuilding and restarting services")
	@$(DOCKER_CMD) -f docker-compose.yml down
	@$(DOCKER_CMD) -f docker-compose.yml build
	@$(DOCKER_CMD) -f docker-compose.yml up -d
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services rebuilt and restarted successfully$(GREEN) $(NC)"

#-------------------------------------------------------
# Development (Hot-Reload) Commands
#-------------------------------------------------------

.PHONY: up-dev
up-dev:
	$(call title1,"Starting services in DEV mode with hot-reload")
	@$(DOCKER_CMD) -f docker-compose.dev.yml up $(c) -d
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dev services started with hot-reload$(GREEN) $(NC)"

.PHONY: down-dev
down-dev:
	$(call title1,"Stopping DEV services")
	@if [ -f "docker-compose.dev.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.dev.yml down $(c); \
	else \
		echo "$(YELLOW)No docker-compose.dev.yml file found. Skipping down command.$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dev services stopped successfully$(GREEN) $(NC)"

.PHONY: rebuild-up-dev
rebuild-up-dev:
	$(call title1,"Rebuilding and restarting DEV services")
	@$(DOCKER_CMD) -f docker-compose.dev.yml down
	@$(DOCKER_CMD) -f docker-compose.dev.yml build --no-cache
	@$(DOCKER_CMD) -f docker-compose.dev.yml up -d
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dev services rebuilt and restarted$(GREEN) $(NC)"

.PHONY: restart-dev
restart-dev:
	$(call title1,"Restarting DEV services")
	@$(DOCKER_CMD) -f docker-compose.dev.yml restart $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dev services restarted$(GREEN) $(NC)"

.PHONY: logs-dev
logs-dev:
	$(call title1,"Showing logs for DEV services")
	@if [ -f "docker-compose.dev.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.dev.yml logs --tail=100 -f $(c); \
	else \
		echo "$(YELLOW)No docker-compose.dev.yml file found.$(NC)"; \
	fi

.PHONY: ps-dev
ps-dev:
	$(call title1,"Listing DEV container status")
	@$(DOCKER_CMD) -f docker-compose.dev.yml ps

.PHONY: clean-docker
clean-docker:
	$(call title1,"Cleaning Docker resources")
	@echo "$(YELLOW)Stopping and removing containers, networks, and volumes...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml down -v
	@echo "$(YELLOW)Pruning unused Docker resources...$(NC)"
	@docker system prune -f
	@echo "$(YELLOW)Pruning unused volumes...$(NC)"
	@docker volume prune -f
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Docker resources cleaned successfully$(GREEN) $(NC)"

.PHONY: logs
logs:
	$(call title1,"Showing logs for all services")
	@if [ -f "docker-compose.yml" ]; then \
		echo "$(CYAN)Logs for component: $(BOLD)reconciliation$(NC)"; \
		$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f $(c); \
	else \
		echo "$(YELLOW)No docker-compose.yml file found. Skipping logs command.$(NC)"; \
	fi

.PHONY: logs-api
logs-api:
	$(call title1,"Showing logs for reconciliation service")
	@$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f reconciliation

.PHONY: ps
ps:
	$(call title1,"Listing container status")
	@$(DOCKER_CMD) -f docker-compose.yml ps

#-------------------------------------------------------
# Security Commands
#-------------------------------------------------------

.PHONY: sec
sec:
	$(call title1,"Running security checks using gosec")
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing gosec...$(NC)"; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@if find . -name "*.go" -type f | grep -q .; then \
		echo "$(CYAN)Running security checks...$(NC)"; \
		gosec ./...; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Security checks completed$(GREEN) $(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping security checks$(NC)"; \
	fi

#-------------------------------------------------------
# Test Coverage Commands
#-------------------------------------------------------

.PHONY: check-tests
check-tests:
	$(call title1,"Verifying test coverage")
	@if find . -name "*.go" -type f | grep -q .; then \
		echo "$(CYAN)Running test coverage check...$(NC)"; \
		go test -coverprofile=coverage.tmp ./... > /dev/null 2>&1; \
		if [ -f coverage.tmp ]; then \
			coverage=$$(go tool cover -func=coverage.tmp | grep total | awk '{print $$3}'); \
			echo "$(CYAN)Test coverage: $(GREEN)$$coverage$(NC)"; \
			rm coverage.tmp; \
		else \
			echo "$(YELLOW)No coverage data generated$(NC)"; \
		fi; \
	else \
		echo "$(YELLOW)No Go files found, skipping test coverage check$(NC)"; \
	fi

#-------------------------------------------------------
# Reconciliation-Specific Commands
#-------------------------------------------------------

.PHONY: run
run:
	$(call title1,"Running the application with .env config")
	@go run cmd/app/main.go .env
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Application started successfully$(GREEN) $(NC)"

.PHONY: status
status:
	$(call title1,"Getting reconciliation status")
	@curl -s http://localhost:3005/reconciliation/status | jq .

.PHONY: report
report:
	$(call title1,"Getting full reconciliation report")
	@curl -s http://localhost:3005/reconciliation/report | jq .

.PHONY: trigger
trigger:
	$(call title1,"Triggering manual reconciliation run")
	@curl -s -X POST http://localhost:3005/reconciliation/run | jq .

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call title1,"Setting up development environment")
	@echo "$(CYAN)Installing development tools...$(NC)"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing golangci-lint...$(NC)"; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing gosec...$(NC)"; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@echo "$(CYAN)Setting up environment...$(NC)"
	@if [ -f .env.example ] && [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "$(GREEN)Created .env file from template$(NC)"; \
	fi
	@make tidy
	@make check-tests
	@make sec
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully$(GREEN) $(NC)"
	@echo "$(CYAN)You're ready to start developing! Here are some useful commands:$(NC)"
	@echo "  make build         - Build the component"
	@echo "  make test          - Run tests"
	@echo "  make up            - Start services"
	@echo "  make rebuild-up    - Rebuild and restart services during development"
