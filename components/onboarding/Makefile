SERVICE_NAME := onboarding-service
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts

$(shell mkdir -p $(ARTIFACTS_DIR))

# Define the root directory of the project (if not already defined)
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)

# Include shared color definitions and utility functions
include $(MIDAZ_ROOT)/pkg/shell/makefile_colors.mk
include $(MIDAZ_ROOT)/pkg/shell/makefile_utils.mk

# Display available commands
.PHONY: info
info:
	@echo ""
	@echo "$(BOLD)Onboarding Service Commands$(NC)"
	@echo ""
	@echo "To run a specific command inside the onboarding container using make, you can execute:"
	@echo ""
	@echo "$(CYAN)make onboarding COMMAND=\"any\"$(NC)"
	@echo ""
	@echo "This command will run the specified command inside the onboarding container."
	@echo "Replace \"any\" with the desired command you want to execute."
	@echo ""
	@echo "$(BOLD)## Docker commands:$(NC)"
	@echo ""
	@echo "  $(CYAN)COMMAND=\"build\"$(NC)                                Build Docker images"
	@echo "  $(CYAN)COMMAND=\"up\"$(NC)                                   Start all services in detached mode"
	@echo "  $(CYAN)COMMAND=\"start\"$(NC)                                Start existing containers"
	@echo "  $(CYAN)COMMAND=\"stop\"$(NC)                                 Stop running containers"
	@echo "  $(CYAN)COMMAND=\"down\"$(NC)                                 Stop and remove containers, networks, and volumes"
	@echo "  $(CYAN)COMMAND=\"destroy\"$(NC)                              Stop and remove containers, networks, and all volumes"
	@echo "  $(CYAN)COMMAND=\"restart\"$(NC)                              Restart all services"
	@echo "  $(CYAN)COMMAND=\"logs\"$(NC)                                 Show and follow logs for all services"
	@echo "  $(CYAN)COMMAND=\"logs-api\"$(NC)                             Show and follow logs for onboarding service"
	@echo "  $(CYAN)COMMAND=\"ps\"$(NC)                                   List container status"
	@echo ""
	@echo "$(BOLD)## App commands:$(NC)"
	@echo ""
	@echo "  $(CYAN)COMMAND=\"run\"$(NC)                                  Run the application with .env config"
	@echo "  $(CYAN)COMMAND=\"test\"$(NC)                                 Run all tests"
	@echo "  $(CYAN)COMMAND=\"cover-html\"$(NC)                           Generate HTML test coverage report"
	@echo "  $(CYAN)COMMAND=\"tidy\"$(NC)                                 Clean and update Go dependencies"
	@echo "  $(CYAN)COMMAND=\"generate-docs\"$(NC)                        Generate Swagger API documentation"
	@echo ""

# Docker Compose Commands
.PHONY: build
build:
	@echo "$(BLUE)Building Docker images...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml build $(c)

.PHONY: up
up:
	@echo "$(BLUE)Starting all services in detached mode...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml up $(c) -d

.PHONY: start
start:
	@echo "$(BLUE)Starting existing containers...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml start $(c)

.PHONY: down
down:
	@echo "$(BLUE)Stopping and removing containers, networks, and volumes...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml down $(c)

.PHONY: destroy
destroy:
	@echo "$(BLUE)Stopping and removing containers, networks, and all volumes...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml down -v $(c)

.PHONY: stop
stop:
	@echo "$(BLUE)Stopping running containers...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml stop $(c)

.PHONY: restart
restart:
	@echo "$(BLUE)Restarting all services...$(NC)"
	@make stop && \
    make up

.PHONY: logs
logs:
	@echo "$(BLUE)Showing logs for all services...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f $(c)

.PHONY: logs-api
logs-api:
	@echo "$(BLUE)Showing logs for onboarding service...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f onboarding

.PHONY: ps
ps:
	@echo "$(BLUE)Listing container status...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml ps

# App Commands
.PHONY: run
run:
	@echo "$(BLUE)Running the application with .env config...$(NC)"
	@go run cmd/app/main.go .env

.PHONY: test
test:
	@echo "$(BLUE)Running all tests...$(NC)"
	@go test -v ./...

.PHONY: cover-html
cover-html:
	@echo "$(BLUE)Generating HTML test coverage report...$(NC)"
	@go tool cover -html=$(ARTIFACTS_DIR)/coverage.out -o $(ARTIFACTS_DIR)/coverage.html
	@echo "$(GREEN)Coverage report generated at $(ARTIFACTS_DIR)/coverage.html$(NC)"

.PHONY: tidy
tidy:
	@echo "$(BLUE)Cleaning and updating Go dependencies...$(NC)"
	@go mod tidy

.PHONY: generate-docs
generate-docs:
	$(call title1,"Generating Swagger API documentation")
	@cd $(MIDAZ_ROOT)/components/onboarding && swag init -g cmd/app/main.go -o api --parseDependency --parseInternal
	@docker run --rm -v ./:/onboarding --user $(shell id -u):$(shell id -g) openapitools/openapi-generator-cli:v5.1.1 generate -i ./onboarding/api/swagger.json -g openapi-yaml -o ./onboarding/api
	@mv ./api/openapi/openapi.yaml ./api/openapi.yaml
	@rm -rf ./api/README.md ./api/.openapi-generator* ./api/openapi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Swagger API documentation generated successfully$(GREEN) ✔️$(NC)"