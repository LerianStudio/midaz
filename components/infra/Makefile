# Infra Component Makefile

# Component-specific variables
SERVICE_NAME := infra-service
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts

# Docker command with fallback
DOCKER_CMD := $(shell command -v docker-compose || echo "docker compose")

# Ensure artifacts directory exists
$(shell mkdir -p $(ARTIFACTS_DIR))

# Define the root directory of the project
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)

# Include shared color definitions and utility functions
include $(MIDAZ_ROOT)/pkg/shell/makefile_colors.mk
include $(MIDAZ_ROOT)/pkg/shell/makefile_utils.mk

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)Infra Service Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "  make up                          - Start services with Docker Compose"
	@echo "  make down                        - Stop services with Docker Compose"
	@echo "  make start                       - Start existing containers"
	@echo "  make stop                        - Stop running containers"
	@echo "  make restart                     - Restart all containers"
	@echo "  make logs                        - Show logs for all services"
	@echo "  make ps                          - List container status"
	@echo "  make rebuild-up                  - Rebuild and restart services during development"
	@echo "  make clean-docker                - Clean all Docker resources (containers, networks, volumes)"
	@echo "  make destroy                     - Alias for clean-docker (maintained for compatibility)"
	@echo ""
	@echo "$(BOLD)Developer Helper Commands:$(NC)"
	@echo "  make dev-setup                   - Set up development environment"
	@echo ""

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call title1,"Building Infra component")
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Build completed successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Test Commands
#-------------------------------------------------------

.PHONY: test
test:
	$(call title1,"Running tests")
	@go test -v ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Tests completed successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Code Quality Commands
#-------------------------------------------------------

.PHONY: lint
lint:
	$(call title1,"Running linters")
	@if find . -name "*.go" -type f | grep -q .; then \
		if ! command -v golangci-lint >/dev/null 2>&1; then \
			echo "$(YELLOW)Installing golangci-lint...$(NC)"; \
			go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		fi; \
		golangci-lint run --fix ./... --verbose; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Linting completed successfully$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping linting$(NC)"; \
	fi

.PHONY: format
format:
	$(call title1,"Formatting code")
	@go fmt ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Formatting completed successfully$(GREEN) ✔️$(NC)"

.PHONY: tidy
tidy:
	$(call title1,"Cleaning dependencies")
	@go mod tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dependencies cleaned successfully$(GREEN) ✔️$(NC)"

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
		echo "$(GREEN)$(BOLD)[ok]$(NC) Security checks completed$(GREEN) ✔️$(NC)"; \
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
# Clean Commands
#-------------------------------------------------------

.PHONY: clean
clean:
	$(call title1,"Cleaning build artifacts")
	@rm -rf $(BIN_DIR)/* $(ARTIFACTS_DIR)/*
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Artifacts cleaned successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Docker Commands
#-------------------------------------------------------

.PHONY: build-docker
build-docker:
	$(call title1,"Building Docker images")
	@$(DOCKER_CMD) -f docker-compose.yml build $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Docker images built successfully$(GREEN) ✔️$(NC)"

.PHONY: up
up:
	$(call title1,"Starting all services in detached mode")
	@$(DOCKER_CMD) -f docker-compose.yml up $(c) -d
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services started successfully$(GREEN) ✔️$(NC)"

.PHONY: start
start:
	$(call title1,"Starting existing containers")
	@$(DOCKER_CMD) -f docker-compose.yml start $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Containers started successfully$(GREEN) ✔️$(NC)"

.PHONY: down
down:
	$(call title1,"Stopping and removing containers|networks|volumes")
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml down $(c); \
	else \
		echo "$(YELLOW)No docker-compose.yml file found. Skipping down command.$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services stopped successfully$(GREEN) ✔️$(NC)"

.PHONY: destroy
destroy: clean-docker
	@echo "$(YELLOW)Note: 'make destroy' is now an alias for 'make clean-docker'$(NC)"

.PHONY: stop
stop:
	$(call title1,"Stopping running containers")
	@$(DOCKER_CMD) -f docker-compose.yml stop $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Containers stopped successfully$(GREEN) ✔️$(NC)"

.PHONY: restart
restart:
	$(call title1,"Restarting all services")
	@make stop && make up
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services restarted successfully$(GREEN) ✔️$(NC)"

.PHONY: rebuild-up
rebuild-up:
	$(call title1,"Rebuilding and restarting services")
	@$(DOCKER_CMD) -f docker-compose.yml down
	@$(DOCKER_CMD) -f docker-compose.yml build
	@$(DOCKER_CMD) -f docker-compose.yml up -d
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services rebuilt and restarted successfully$(GREEN) ✔️$(NC)"

.PHONY: clean-docker
clean-docker:
	$(call title1,"Cleaning Docker resources")
	@echo "$(YELLOW)Stopping and removing containers, networks, and volumes...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml down -v --rmi all --remove-orphans
	@echo "$(YELLOW)Pruning unused Docker resources...$(NC)"
	@docker system prune -f
	@echo "$(YELLOW)Pruning unused volumes...$(NC)"
	@docker volume prune -f
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Docker resources cleaned successfully$(GREEN) ✔️$(NC)"

.PHONY: logs
logs:
	$(call title1,"Showing logs for all services")
	@if [ -f "docker-compose.yml" ]; then \
		echo "$(CYAN)Logs for component: $(BOLD)infra$(NC)"; \
		docker compose -f docker-compose.yml logs --tail=100 -f $(c) 2>/dev/null || docker-compose -f docker-compose.yml logs --tail=100 -f $(c); \
	else \
		echo "$(YELLOW)No docker-compose.yml file found. Skipping logs command.$(NC)"; \
	fi

.PHONY: ps
ps:
	$(call title1,"Listing container status")
	@$(DOCKER_CMD) -f docker-compose.yml ps

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
	@if ! command -v mockgen >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing mockgen...$(NC)"; \
		go install github.com/golang/mock/mockgen@latest; \
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
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully$(GREEN) ✔️$(NC)"
	@echo "$(CYAN)You're ready to start developing! Here are some useful commands:$(NC)"
	@echo "  make build         - Build the component"
	@echo "  make test          - Run tests"
	@echo "  make up            - Start services"
	@echo "  make rebuild-up    - Rebuild and restart services during development"
