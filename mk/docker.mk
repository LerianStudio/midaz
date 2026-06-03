# ------------------------------------------------------
# Docker and container management commands
# ------------------------------------------------------
# This file contains all Docker Compose operations for
# managing containers, services, and their lifecycle.
#
# Requirements:
#   - docker or docker-compose installed
#   - docker-compose.yml in the component directory
#
# Variables used from the including Makefile:
#   - DOCKER_CMD: docker compose or docker-compose (from mk/utils.mk)
#   - SERVICE_NAME: name of the service
#
# Usage:
#   make up                  # Start all services
#   make down                # Stop and remove containers
#   make logs                # View logs
#   make rebuild-up          # Full rebuild and restart
# ------------------------------------------------------

#-------------------------------------------------------
# Commands (alphabetically ordered)
#-------------------------------------------------------

# Build Docker images
.PHONY: build-docker
build-docker:
	$(call title1,"Building Docker images")
	@$(DOCKER_CMD) -f docker-compose.yml build $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Docker images built successfully$(GREEN) ✔️$(NC)"

# Stop and remove all Docker resources for this component
.PHONY: clean-docker
clean-docker:
	$(call title1,"Cleaning Docker resources")
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml down -v; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Docker resources cleaned successfully$(GREEN) ✔️$(NC)"

# Stop and remove containers, networks, and volumes
.PHONY: down
down:
	$(call title1,"Stopping and removing containers|networks|volumes")
	@if [ -f "docker-compose.yml" ]; then \
		$(DOCKER_CMD) -f docker-compose.yml down $(c); \
	else \
		echo "$(YELLOW)No docker-compose.yml file found. Skipping down command.$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services stopped successfully$(GREEN) ✔️$(NC)"

# Show logs for all services
.PHONY: logs
logs:
	$(call title1,"Showing logs for all services")
	@if [ -f "docker-compose.yml" ]; then \
		echo "$(CYAN)Logs for component: $(BOLD)$(SERVICE_NAME)$(NC)"; \
		$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f $(c); \
	else \
		echo "$(YELLOW)No docker-compose.yml file found. Skipping logs command.$(NC)"; \
	fi

# Show logs for the component's own service only
.PHONY: logs-api
logs-api:
	$(call title1,"Showing logs for $(SERVICE_NAME) service")
	@$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f $(SERVICE_NAME)

# List container status
.PHONY: ps
ps:
	$(call title1,"Listing container status")
	@$(DOCKER_CMD) -f docker-compose.yml ps

# Rebuild and restart services (development workflow)
.PHONY: rebuild-up
rebuild-up:
	$(call title1,"Rebuilding and restarting services")
	@$(DOCKER_CMD) -f docker-compose.yml down
	@$(DOCKER_CMD) -f docker-compose.yml build
	@$(DOCKER_CMD) -f docker-compose.yml up -d
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services rebuilt and restarted successfully$(GREEN) ✔️$(NC)"

# Restart all services (stop + start)
.PHONY: restart
restart:
	$(call title1,"Restarting all services")
	@$(MAKE) stop && $(MAKE) up
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services restarted successfully$(GREEN) ✔️$(NC)"

# Run the application locally with .env config (without Docker)
.PHONY: run
run:
	$(call title1,"Running the application with .env config")
	@go run cmd/app/main.go .env
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Application started successfully$(GREEN) ✔️$(NC)"

# Start existing containers
.PHONY: start
start:
	$(call title1,"Starting existing containers")
	@$(DOCKER_CMD) -f docker-compose.yml start $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Containers started successfully$(GREEN) ✔️$(NC)"

# Stop running containers
.PHONY: stop
stop:
	$(call title1,"Stopping running containers")
	@$(DOCKER_CMD) -f docker-compose.yml stop $(c)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Containers stopped successfully$(GREEN) ✔️$(NC)"

# Start all services in detached mode
.PHONY: up
up:
	$(call title1,"Starting all services in detached mode")
	@$(DOCKER_CMD) -f docker-compose.yml up $(c) -d
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services started successfully$(GREEN) ✔️$(NC)"
