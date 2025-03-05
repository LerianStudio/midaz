SERVICE_NAME := audit-service
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
	@echo "$(BOLD)Audit Service Commands$(NC)"
	@echo ""
	@echo "To run a specific command inside the audit container using make, you can execute:"
	@echo ""
	@echo "$(CYAN)make audit COMMAND=\"any\"$(NC)"
	@echo ""
	@echo "This command will run the specified command inside the audit container."
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
	@echo "  $(CYAN)COMMAND=\"logs-api\"$(NC)                             Show and follow logs for audit service"
	@echo "  $(CYAN)COMMAND=\"ps\"$(NC)                                   List container status"
	@echo ""
	@echo "$(BOLD)## App commands:$(NC)"
	@echo ""
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
	@echo "$(BLUE)Showing logs for audit service...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f midaz-audit

.PHONY: ps
ps:
	@echo "$(BLUE)Listing container status...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml ps

.PHONY: generate-docs
generate-docs:
	$(call title1,"Generating Swagger API documentation")
	@swag init -g ../../../cmd/app/main.go -d ./internal/adapters/http -o ./api --parseDependency --parseInternal
	@docker run --rm -v ./:/audit --user $(shell id -u):$(shell id -g) openapitools/openapi-generator-cli:v5.1.1 generate -i ./audit/api/swagger.json -g openapi-yaml -o ./audit/api
	@mv ./api/openapi/openapi.yaml ./api/openapi.yaml
	@rm -rf ./api/README.md ./api/.openapi-generator* ./api/openapi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Swagger API documentation generated successfully$(GREEN) ✔️$(NC)"