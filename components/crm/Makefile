# Plugin CRM Makefile

ifneq ("$(wildcard .env.local)","")
	include .env.local
	export
endif

# Component-specific variables
SERVICE_NAME := plugin-crm
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts

# Ensure artifacts directory exists
$(shell mkdir -p $(ARTIFACTS_DIR))

# Define the root directory of the project
ROOT_DIR := $(shell pwd)

# Component directories
BACKEND_DIR := .
FRONTEND_DIR := ./frontend

# Define component groups for easier management
COMPONENTS := $(BACKEND_DIR) $(FRONTEND_DIR)
GO_COMPONENTS := $(BACKEND_DIR)
NODE_COMPONENTS := $(FRONTEND_DIR)

# Docker command detection
DOCKER_CMD := $(shell \
	if [ "$(shell printf '%s\n' "$(DOCKER_MIN_VERSION)" "$(DOCKER_VERSION)" | sort -V | head -n1)" = "$(DOCKER_MIN_VERSION)" ]; then \
		echo "docker compose"; \
	else \
		echo "docker-compose"; \
	fi \
)

# Include shared color definitions and utility functions
#include $(ROOT_DIR)/pkg/shell/makefile_colors.mk
#include $(ROOT_DIR)/pkg/shell/makefile_utils.mk

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)CRM Plugin Project Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Multi-Component Commands:$(NC)"
	@echo "  make help                        - Display this help message"
	@echo "  make build                       - Build all components (backend + frontend)"
	@echo "  make test                        - Run tests on all components"
	@echo "  make clean                       - Clean all build artifacts"
	@echo "  make dev-setup                   - Set up development environment for all components"
	@echo ""
	@echo "$(BOLD)Component-Specific Commands:$(NC)"
	@echo "  make backend COMMAND=<cmd>       - Run command in backend component"
	@echo "  make frontend COMMAND=<cmd>      - Run command in frontend component"
	@echo "  make all-components COMMAND=<cmd> - Run command across all components"
	@echo ""
	@echo "$(BOLD)Backend-Specific Commands:$(NC)"
	@echo "  make run                         - Run the backend application with .env config"
	@echo "  make cover-html                  - Generate HTML test coverage report"
	@echo "  make lint                        - Run Go linting tools"
	@echo "  make format                      - Format Go code"
	@echo "  make tidy                        - Clean Go dependencies"
	@echo "  make sec                         - Run security checks using gosec"
	@echo "  make generate-docs               - Generate Swagger API documentation"
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "  make up                          - Start all services with Docker Compose"
	@echo "  make down                        - Stop all services with Docker Compose"
	@echo "  make start                       - Start existing containers"
	@echo "  make stop                        - Stop running containers"
	@echo "  make restart                     - Restart all containers"
	@echo "  make logs                        - Show logs for all services"
	@echo "  make logs-backend                - Show logs for backend service"
	@echo "  make logs-frontend               - Show logs for frontend service"
	@echo "  make ps                          - List container status"
	@echo "  make rebuild-up                  - Rebuild and restart all services"
	@echo ""
	@echo "$(BOLD)Development Commands:$(NC)"
	@echo "  make dev-all                     - Start development mode (backend + frontend)"
	@echo "  make status                      - Show project status"
	@echo ""
	@echo "$(BOLD)Environment Commands:$(NC)"
	@echo "  make set-env                     - Set up environment files for all components"
	@echo "  make check-envs                  - Check for exposed .env files"
	@echo ""
	@echo "$(BOLD)Git Hook Commands:$(NC)"
	@echo "  make setup-git-hooks             - Install git hooks"
	@echo "  make check-hooks                 - Check git hooks status"
	@echo ""

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
	$(call title1,"Setting up environment files for all components")
	@echo "$(CYAN)Setting up backend environment...$(NC)"
	@if [ -f ".env.example" ] && [ ! -f ".env" ]; then \
		echo "  Creating .env from .env.example"; \
		cp ".env.example" ".env"; \
		echo "$(GREEN)  ✓ Backend .env created$(NC)"; \
	elif [ ! -f ".env.example" ]; then \
		echo "$(YELLOW)  ! No .env.example found in backend$(NC)"; \
	else \
		echo "$(GREEN)  ✓ Backend .env already exists$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)Setting up frontend environment...$(NC)"
	@if [ -f "$(FRONTEND_DIR)/.env.example" ] && [ ! -f "$(FRONTEND_DIR)/.env" ]; then \
		echo "  Creating .env from .env.example"; \
		cp "$(FRONTEND_DIR)/.env.example" "$(FRONTEND_DIR)/.env"; \
		echo "$(GREEN)  ✓ Frontend .env created$(NC)"; \
	elif [ ! -f "$(FRONTEND_DIR)/.env.example" ]; then \
		if [ -f "$(FRONTEND_DIR)/Makefile" ]; then \
			echo "  Running frontend set-env command..."; \
			cd $(FRONTEND_DIR) && $(MAKE) set-env; \
		else \
			echo "$(YELLOW)  ! No .env.example found in frontend$(NC)"; \
		fi \
	else \
		echo "$(GREEN)  ✓ Frontend .env already exists$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Environment files set up successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call title1,"Building all components")
	@echo "$(CYAN)Building backend...$(NC)"
	@if find . -maxdepth 1 -name "*.go" -type f | grep -q . || [ -f "go.mod" ]; then \
		if [ -f "cmd/app/main.go" ]; then \
			go build -o bin/plugin-crm cmd/app/main.go; \
			echo "$(GREEN)✓ Backend built successfully$(NC)"; \
		else \
			echo "$(YELLOW)No backend main.go found, skipping...$(NC)"; \
		fi; \
	else \
		echo "$(YELLOW)No Go files found in backend, skipping...$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)Building frontend...$(NC)"
	@if [ -f "$(FRONTEND_DIR)/package.json" ]; then \
		cd $(FRONTEND_DIR) && $(MAKE) build; \
		echo "$(GREEN)✓ Frontend built successfully$(NC)"; \
	else \
		echo "$(YELLOW)No frontend found, skipping...$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All components built successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Test Commands
#-------------------------------------------------------

.PHONY: test
test:
	$(call title1,"Running tests on all components")
	@echo "$(CYAN)Testing backend...$(NC)"
	@if find . -maxdepth 1 -name "*.go" -type f | grep -q . || [ -f "go.mod" ]; then \
		go test -v ./...; \
		echo "$(GREEN)✓ Backend tests passed$(NC)"; \
	else \
		echo "$(YELLOW)No Go files found in backend, skipping tests...$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)Testing frontend...$(NC)"
	@if [ -f "$(FRONTEND_DIR)/package.json" ]; then \
		cd $(FRONTEND_DIR) && $(MAKE) test; \
		echo "$(GREEN)✓ Frontend tests passed$(NC)"; \
	else \
		echo "$(YELLOW)No frontend found, skipping tests...$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All tests completed successfully$(GREEN) ✔️$(NC)"

.PHONY: cover-html
cover-html:
	$(call title1,"Generating HTML test coverage report")
	@PACKAGES=$$(go list ./... | grep -v -f ./scripts/coverage_ignore.txt); \
	go test -coverprofile=$(ARTIFACTS_DIR)/coverage.out $$PACKAGES
	@go tool cover -html=$(ARTIFACTS_DIR)/coverage.out -o $(ARTIFACTS_DIR)/coverage.html
	@echo "$(GREEN)Coverage report generated at $(ARTIFACTS_DIR)/coverage.html$(NC)"
	@echo ""
	@echo "$(CYAN)Coverage Summary:$(NC)"
	@echo "$(CYAN)----------------------------------------$(NC)"
	@go tool cover -func=$(ARTIFACTS_DIR)/coverage.out | grep total | awk '{print "Total coverage: " $$3}'
	@echo "$(CYAN)----------------------------------------$(NC)"
	@echo "$(YELLOW)Open $(ARTIFACTS_DIR)/coverage.html in your browser to view detailed coverage report$(NC)"

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
	$(call title1,"Update and Cleaning dependencies")
	@go get -u ./...
	@go mod tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dependencies updated and cleaned successfully$(GREEN) ✔️$(NC)"

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
		gosec -quiet ./...; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Security checks completed$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping security checks$(NC)"; \
	fi

#-------------------------------------------------------
# Clean Commands
#-------------------------------------------------------

.PHONY: clean
clean:
	$(call title1,"Cleaning build artifacts for all components")
	@echo "$(CYAN)Cleaning backend artifacts...$(NC)"
	@rm -rf $(BIN_DIR) $(ARTIFACTS_DIR) coverage.tmp coverage.out coverage.html
	@rm -rf bin/
	@echo "$(GREEN)✓ Backend cleaned$(NC)"
	@echo ""
	@echo "$(CYAN)Cleaning frontend artifacts...$(NC)"
	@if [ -f "$(FRONTEND_DIR)/Makefile" ]; then \
		cd $(FRONTEND_DIR) && $(MAKE) clean; \
		echo "$(GREEN)✓ Frontend cleaned$(NC)"; \
	else \
		echo "$(YELLOW)No frontend Makefile found, skipping...$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All artifacts cleaned successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Docker Commands
#-------------------------------------------------------

.PHONY: run
run:
	$(call title1,"Running the application with .env config")
	@go run cmd/app/main.go .env
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Application started successfully$(GREEN) ✔️$(NC)"

.PHONY: build-docker
build-docker:
	$(call title1,"Building Docker images")
	@$(DOCKER_CMD) -f docker-compose.yml build $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Docker images built successfully$(GREEN) ✔️$(NC)"

.PHONY: up
up:
	$(call title1,"Starting all services in detached mode")
	@echo "$(CYAN)Starting backend services...$(NC)"
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml up $(c) -d; \
		echo "$(GREEN)✓ Backend services started$(NC)"; \
	else \
		echo "$(YELLOW)No backend docker-compose.yml found$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)Starting frontend services...$(NC)"
	@if [ -f "$(FRONTEND_DIR)/docker-compose.yml" ]; then \
		cd $(FRONTEND_DIR) && $(MAKE) up; \
		echo "$(GREEN)✓ Frontend services started$(NC)"; \
	else \
		echo "$(YELLOW)No frontend docker-compose.yml found$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All services started successfully$(GREEN) ✔️$(NC)"

.PHONY: start
start:
	$(call title1,"Starting existing containers")
	@$(DOCKER_CMD) -f docker-compose.yml start $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Containers started successfully$(GREEN) ✔️$(NC)"

.PHONY: down
down:
	$(call title1,"Stopping and removing containers|networks|volumes")
	@echo "$(CYAN)Stopping frontend services...$(NC)"
	@if [ -f "$(FRONTEND_DIR)/docker-compose.yml" ]; then \
		cd $(FRONTEND_DIR) && $(MAKE) down; \
		echo "$(GREEN)✓ Frontend services stopped$(NC)"; \
	else \
		echo "$(YELLOW)No frontend docker-compose.yml found$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)Stopping backend services...$(NC)"
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml down $(c); \
		echo "$(GREEN)✓ Backend services stopped$(NC)"; \
	else \
		echo "$(YELLOW)No backend docker-compose.yml found$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All services stopped successfully$(GREEN) ✔️$(NC)"

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
	$(call title1,"Rebuilding and restarting all services")
	@echo "$(CYAN)Rebuilding backend...$(NC)"
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml down; \
		$(DOCKER_CMD) -f docker-compose.yml build; \
		$(DOCKER_CMD) -f docker-compose.yml up -d; \
		echo "$(GREEN)✓ Backend rebuilt and started$(NC)"; \
	else \
		echo "$(YELLOW)No backend docker-compose.yml found$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)Rebuilding frontend...$(NC)"
	@if [ -f "$(FRONTEND_DIR)/docker-compose.yml" ]; then \
		cd $(FRONTEND_DIR) && $(MAKE) rebuild-up; \
		echo "$(GREEN)✓ Frontend rebuilt and started$(NC)"; \
	else \
		echo "$(YELLOW)No frontend docker-compose.yml found$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All services rebuilt and restarted successfully$(GREEN) ✔️$(NC)"

.PHONY: logs
logs:
	$(call title1,"Showing logs for all services")
	@echo "$(CYAN)Use Ctrl+C to stop following logs$(NC)"
	@echo ""
	@if command -v tmux >/dev/null 2>&1; then \
		echo "$(CYAN)Opening logs in tmux split panes...$(NC)"; \
		tmux new-session -d -s crm-logs '$(MAKE) logs-backend' \; \
			split-window -h '$(MAKE) logs-frontend' \; \
			select-pane -t 0 \; \
			attach-session -t crm-logs || \
		echo "$(YELLOW)Tmux session 'crm-logs' already exists. Attach with: tmux attach -t crm-logs$(NC)"; \
	else \
		echo "$(YELLOW)tmux not found. Showing logs sequentially...$(NC)"; \
		echo "$(CYAN)Backend logs (last 50 lines):$(NC)"; \
		if [ -f "docker-compose.yml" ]; then \
			$(DOCKER_CMD) -f docker-compose.yml logs --tail=50 2>/dev/null || echo "$(YELLOW)No backend containers running$(NC)"; \
		fi; \
		echo ""; \
		echo "$(CYAN)Frontend logs (last 50 lines):$(NC)"; \
		if [ -f "$(FRONTEND_DIR)/docker-compose.yml" ]; then \
			cd $(FRONTEND_DIR) && $(DOCKER_CMD) -f docker-compose.yml logs --tail=50 2>/dev/null || echo "$(YELLOW)No frontend containers running$(NC)"; \
		fi \
	fi

.PHONY: logs-backend
logs-backend:
	$(call title1,"Showing logs for backend service")
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f $(c); \
	else \
		echo "$(YELLOW)No backend docker-compose.yml found$(NC)"; \
	fi

.PHONY: logs-frontend
logs-frontend:
	$(call title1,"Showing logs for frontend service")
	@if [ -f "$(FRONTEND_DIR)/docker-compose.yml" ]; then \
		cd $(FRONTEND_DIR) && $(MAKE) logs; \
	else \
		echo "$(YELLOW)No frontend docker-compose.yml found$(NC)"; \
	fi

.PHONY: ps
ps:
	$(call title1,"Listing container status")
	@$(DOCKER_CMD) -f docker-compose.yml ps

#-------------------------------------------------------
# Docs Commands
#-------------------------------------------------------

.PHONY: generate-docs-all
generate-docs-all:
	$(call title1,"Generating Swagger documentation for all services")
	$(call check_command,swag,"go install github.com/swaggo/swag/cmd/swag@latest")
	@echo "$(CYAN)Verifying API documentation coverage...$(NC)"
	@sh ./scripts/verify-api-docs.sh 2>/dev/null || echo "$(YELLOW)Warning: Some API endpoints may not be properly documented. Continuing with documentation generation...$(NC)"
	@echo "$(CYAN)Generating documentation for plugin component...$(NC)"
	$(MAKE) generate-docs 2>&1 | grep -v "warning: "
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Swagger documentation generated successfully$(GREEN) ✔️$(NC)"


.PHONY: verify-api-docs
verify-api-docs:
	$(call title1,"Verifying API documentation coverage")
	@if [ -f "./scripts/package.json" ]; then \
		echo "$(CYAN)Installing npm dependencies...$(NC)"; \
		cd ./scripts && npm install; \
	fi
	@sh ./scripts/verify-api-docs.sh
	@echo "$(GREEN)$(BOLD)[ok]$(NC) API documentation verification completed$(GREEN) ✔️$(NC)"

.PHONY: validate-plugin
validate-plugin:
	$(call title1,"Validating pluginAPI documentation")
	@if [ -f "./scripts/package.json" ]; then \
		echo "$(CYAN)Installing npm dependencies...$(NC)"; \
		cd ./scripts && npm install; \
	fi
	make validate-api-docs
	@echo "$(GREEN)$(BOLD)[ok]$(NC) plugin API validation completed$(GREEN) ✔️$(NC)"


.PHONY: generate-docs
generate-docs:
	$(call title1,"Generating Swagger API documentation")
	@if ! command -v swag >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing swag...$(NC)"; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
	fi
	@cd $(ROOT_DIR) && swag init -g cmd/app/main.go -o api --parseDependency --parseInternal
	@docker run --rm -v ./:/plugin --user $(shell id -u):$(shell id -g) openapitools/openapi-generator-cli:v5.1.1 generate -i ./plugin/api/swagger.json -g openapi-yaml -o ./plugin/api
	@mv ./api/openapi/openapi.yaml ./api/openapi.yaml
	@rm -rf ./api/README.md ./api/.openapi-generator* ./api/openapi
	@if [ -f "$(ROOT_DIR)/scripts/package.json" ]; then \
		echo "$(YELLOW)Installing npm dependencies for validation...$(NC)"; \
		cd $(ROOT_DIR)/scripts && npm install > /dev/null; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Swagger API documentation generated successfully$(GREEN) ✔️$(NC)"

.PHONY: validate-api-docs
validate-api-docs: generate-docs
	$(call title1,"Validating API documentation")
	@if [ -f "scripts/validate-api-docs.js" ] && [ -f "$(ROOT_DIR)/scripts/package.json" ]; then \
		echo "$(YELLOW)Validating API documentation structure...$(NC)"; \
		cd $(ROOT_DIR)/scripts && node $(ROOT_DIR)/scripts/validate-api-docs.js; \
		echo "$(YELLOW)Validating API implementations...$(NC)"; \
		cd $(ROOT_DIR)/scripts && node $(ROOT_DIR)/scripts/validate-api-implementations.js; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) API documentation validation completed$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(YELLOW)Validation scripts not found. Skipping validation.$(NC)"; \
	fi

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call title1,"Setting up development environment for all components")
	@echo "$(CYAN)=== Backend Setup ===$(NC)"
	@echo "$(CYAN)Installing Go development tools...$(NC)"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "  Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@if ! command -v swag >/dev/null 2>&1; then \
		echo "  Installing swag..."; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
	fi
	@if ! command -v mockgen >/dev/null 2>&1; then \
		echo "  Installing mockgen..."; \
		go install github.com/golang/mock/mockgen@latest; \
	fi
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "  Installing gosec..."; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@echo "$(CYAN)Setting up backend environment...$(NC)"
	@$(MAKE) set-env
	@$(MAKE) tidy
	@echo "$(GREEN)✓ Backend setup complete$(NC)"
	@echo ""
	@echo "$(CYAN)=== Frontend Setup ===$(NC)"
	@if [ -f "$(FRONTEND_DIR)/Makefile" ]; then \
		cd $(FRONTEND_DIR) && $(MAKE) dev-setup; \
		echo "$(GREEN)✓ Frontend setup complete$(NC)"; \
	else \
		echo "$(YELLOW)! No frontend Makefile found, skipping frontend setup$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)=== Git Hooks Setup ===$(NC)"
	@$(MAKE) setup-git-hooks
	@echo ""
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment ready!$(GREEN) ✔️$(NC)"
	@echo ""
	@echo "$(CYAN)Quick start commands:$(NC)"
	@echo "  make dev-all      - Start backend + frontend development"
	@echo "  make test         - Run all tests"
	@echo "  make build        - Build all components"
	@echo "  make up           - Start all services with Docker"
	@echo "  make status       - Check project status"
	@echo ""
	@echo "$(CYAN)Component-specific commands:$(NC)"
	@echo "  make backend COMMAND=<cmd>  - Run command in backend"
	@echo "  make frontend COMMAND=<cmd> - Run command in frontend"
	@echo ""
	@echo "$(CYAN)Happy coding! 🚀$(NC)"

#-------------------------------------------------------
# Component-specific command execution
#-------------------------------------------------------

.PHONY: backend
backend:
	$(call title1,"Starting backend services in detached mode")
	@echo "$(CYAN)Starting backend services...$(NC)"
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml up $(c) -d; \
		echo "$(GREEN)✓ Backend services started$(NC)"; \
	else \
		echo "$(YELLOW)No backend docker-compose.yml found$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All services started successfully$(GREEN) ✔️$(NC)"

.PHONY: frontend
frontend:
	$(call title1,"Running command in frontend component")
	@if [ -z "$(COMMAND)" ]; then \
		echo "$(RED)Error: No command specified. Use COMMAND=<cmd> to specify a command.$(NC)"; \
		exit 1; \
	fi
	@if [ -f "$(FRONTEND_DIR)/Makefile" ]; then \
		echo "$(CYAN)Executing 'make $(COMMAND)' in frontend...$(NC)"; \
		cd $(FRONTEND_DIR) && $(MAKE) $(COMMAND); \
	else \
		echo "$(RED)Error: No Makefile found in frontend directory$(NC)"; \
		exit 1; \
	fi

.PHONY: all-components
all-components:
	$(call title1,"Running command across all components")
	@if [ -z "$(COMMAND)" ]; then \
		echo "$(RED)Error: No command specified. Use COMMAND=<cmd> to specify a command.$(NC)"; \
		exit 1; \
	fi
	@echo "$(CYAN)Running '$(COMMAND)' in backend...$(NC)"
	@if $(MAKE) $(COMMAND) 2>/dev/null; then \
		echo "$(GREEN)✓ Backend command completed$(NC)"; \
	else \
		echo "$(YELLOW)! Backend command failed or not found$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)Running '$(COMMAND)' in frontend...$(NC)"
	@if [ -f "$(FRONTEND_DIR)/Makefile" ]; then \
		if cd $(FRONTEND_DIR) && $(MAKE) $(COMMAND) 2>/dev/null; then \
			echo "$(GREEN)✓ Frontend command completed$(NC)"; \
		else \
			echo "$(YELLOW)! Frontend command failed or not found$(NC)"; \
		fi \
	else \
		echo "$(YELLOW)No frontend Makefile found, skipping...$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Command '$(COMMAND)' executed across all components$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Development Commands
#-------------------------------------------------------

.PHONY: dev-all
dev-all:
	$(call title1,"Starting development mode for all components")
	@if command -v tmux >/dev/null 2>&1; then \
		echo "$(CYAN)Starting tmux session with backend and frontend...$(NC)"; \
		tmux new-session -d -s crm-dev 'make run' \; \
			split-window -h 'cd $(FRONTEND_DIR) && make dev' \; \
			select-pane -t 0 \; \
			attach-session -t crm-dev || \
		echo "$(YELLOW)Tmux session 'crm-dev' already exists. Attach with: tmux attach -t crm-dev$(NC)"; \
	else \
		echo "$(YELLOW)tmux not found. Please run these commands in separate terminals:$(NC)"; \
		echo "  Terminal 1: make run"; \
		echo "  Terminal 2: cd $(FRONTEND_DIR) && make dev"; \
	fi

.PHONY: status
status:
	$(call title1,"Project Status")
	@echo "$(CYAN)Backend Status:$(NC)"
	@if pgrep -f "plugin-crm" > /dev/null 2>&1; then \
		echo "  $(GREEN)✓ Backend is running$(NC)"; \
	else \
		echo "  $(YELLOW)✗ Backend is not running$(NC)"; \
	fi
	@if [ -f ".env" ]; then \
		echo "  $(GREEN)✓ Backend .env exists$(NC)"; \
	else \
		echo "  $(YELLOW)✗ Backend .env missing$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)Frontend Status:$(NC)"
	@if lsof -i :8082 > /dev/null 2>&1; then \
		echo "  $(GREEN)✓ Frontend is running on port 8082$(NC)"; \
	else \
		echo "  $(YELLOW)✗ Frontend is not running$(NC)"; \
	fi
	@if [ -f "$(FRONTEND_DIR)/.env" ]; then \
		echo "  $(GREEN)✓ Frontend .env exists$(NC)"; \
	else \
		echo "  $(YELLOW)✗ Frontend .env missing$(NC)"; \
	fi
	@echo ""
	@echo "$(CYAN)Docker Status:$(NC)"
	@if docker ps | grep -q "plugin-crm" 2>/dev/null; then \
		echo "  $(GREEN)✓ Backend container is running$(NC)"; \
	else \
		echo "  $(YELLOW)✗ Backend container is not running$(NC)"; \
	fi
	@if docker ps | grep -q "plugin-crm-ui" 2>/dev/null; then \
		echo "  $(GREEN)✓ Frontend container is running$(NC)"; \
	else \
		echo "  $(YELLOW)✗ Frontend container is not running$(NC)"; \
	fi
