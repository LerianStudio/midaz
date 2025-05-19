# Console Component Makefile

# Component-specific variables
SERVICE_NAME := console
BUILD_DIR := ./dist
ARTIFACTS_DIR := ./artifacts

# Ensure artifacts directory exists
$(shell mkdir -p $(ARTIFACTS_DIR))

# Define the root directory of the project
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)

# Define local utility functions
define title1
	@echo ""
	@echo "------------------------"
	@echo "   📝 $(1)  "
	@echo "------------------------"
endef

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)Console Component Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Development Commands:$(NC)"
	@echo "  make install            - Install dependencies"
	@echo "  make dev                - Start development server"
	@echo "  make build              - Build the application"
	@echo "  make start              - Start the application"
	@echo "  make test               - Run tests"
	@echo "  make test:e2e           - Run end-to-end tests"
	@echo "  make lint               - Run linting"
	@echo "  make format             - Format code"
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "  make up                 - Start Docker container"
	@echo "  make down               - Stop Docker container"
	@echo "  make restart            - Restart Docker container"
	@echo "  make rebuild-up         - Rebuild and restart services"
	@echo "  make set-env            - Copy .env.example to .env"
	@echo ""
	@echo "$(BOLD)Storybook Commands:$(NC)"
	@echo "  make storybook          - Start Storybook server"
	@echo "  make build-storybook    - Build Storybook static site"
	@echo ""
	@echo "$(BOLD)i18n Commands:$(NC)"
	@echo "  make extract-i18n       - Extract i18n messages"
	@echo "  make compile-i18n       - Compile i18n messages"
	@echo "  make i18n               - Run both extract and compile i18n"
	@echo ""

#-------------------------------------------------------
# Development Commands
#-------------------------------------------------------

.PHONY: dev
dev:
	$(call title1,"Starting development server")
	npm run dev

.PHONY: install
install:
	$(call title1,"Installing dependencies")
	npm install

.PHONY: build
build:
	$(call title1,"Building console")
	$(MAKE) install
	npm run build

.PHONY: start
start:
	$(call title1,"Starting the application")
	npm run start

.PHONY: test
test:
	$(call title1,"Running tests")
	npm install && npm run test

.PHONY: test\:e2e
test\:e2e:
	$(call title1,"Running end-to-end tests")
	npm install && npm run test:e2e

.PHONY: lint
lint:
	$(call title1,"Running linting")
	npm run lint

.PHONY: format
format:
	$(call title1,"Formatting code")
	npm run format

#-------------------------------------------------------
# Docker Commands
#-------------------------------------------------------

.PHONY: up
up:
	$(call title1,"Starting all services in detached mode")
	@$(DOCKER_CMD) -f docker-compose.yml up $(c) -d
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services started successfully$(GREEN) ✔️$(NC)"

.PHONY: down
down:
	$(call title1,"Stopping and removing containers|networks|volumes")
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml down $(c); \
	else \
		echo "$(YELLOW)No docker-compose.yml file found. Skipping down command.$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services stopped successfully$(GREEN) ✔️$(NC)"

.PHONY: restart
restart:
	$(call title1,"Restarting Docker container")
	$(MAKE) down
	$(MAKE) up

.PHONY: rebuild-up
rebuild-up:
	$(call title1,"Rebuilding and restarting services")
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml up $(c) -d --build; \
	else \
		echo "No docker-compose.yml file found. Skipping rebuild-up command."; \
	fi
	@echo "[ok] Services rebuilt and restarted successfully ✔️"

.PHONY: set-env
set-env:
	$(call title1,"Setting up environment file")
	@if [ -f ".env.example" ] && [ ! -f ".env" ]; then \
		echo "$(CYAN)Creating .env from .env.example$(NC)"; \
		cp .env.example .env; \
	elif [ ! -f ".env.example" ]; then \
		echo "$(YELLOW)Warning: No .env.example found$(NC)"; \
	else \
		echo "$(GREEN).env already exists$(NC)"; \
	fi

#-------------------------------------------------------
# Storybook Commands
#-------------------------------------------------------

.PHONY: storybook
storybook:
	$(call title1,"Starting Storybook server")
	npm run storybook

.PHONY: build-storybook
build-storybook:
	$(call title1,"Building Storybook static site")
	npm run build-storybook

#-------------------------------------------------------
# i18n Commands
#-------------------------------------------------------

.PHONY: extract-i18n
extract-i18n:
	$(call title1,"Extracting i18n messages")
	npm run extract:i18n

.PHONY: compile-i18n
compile-i18n:
	$(call title1,"Compiling i18n messages")
	npm run compile:i18n

.PHONY: i18n
i18n:
	$(call title1,"Running i18n tasks")
	$(MAKE) extract-i18n
	$(MAKE) compile-i18n

#-------------------------------------------------------
# Cleanup Commands
#-------------------------------------------------------

clean:
	$(call title1,"Cleaning build artifacts")
	@echo "Cleaning $(SERVICE_NAME) artifacts..."
	@rm -rf $(BUILD_DIR) $(ARTIFACTS_DIR) coverage coverage.out coverage.html *.tmp
	@if [ -d "node_modules" ]; then \
		echo "Removing node_modules..."; \
		rm -rf node_modules; \
	fi
	@if [ -d ".next" ]; then \
		echo "Removing .next..."; \
		rm -rf .next; \
	fi
	@echo "$(OK_COLOR)[ok] Artifacts cleaned successfully ✔️$(NO_COLOR)"
