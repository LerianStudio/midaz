service_name := infra-service
bin_dir := ./.bin
artifacts_dir := ./artifacts

$(shell mkdir -p $(artifacts_dir))

.PHONY: help build up start down destroy stop restart logs logs-api ps login-timescale login-api db-shell

# Display available commands
info:
	@echo "                                                                                                                                       "
	@echo "                                                                                                                                       "
	@echo "To run a specific command inside the infra container using make, you can execute:                                                     "
	@echo "                                                                                                                                       "
	@echo "make infra COMMAND=\"any\"                                                                                                            "
	@echo "                                                                                                                                       "
	@echo "This command will run the specified command inside the infra container. Replace \"any\" with the desired command you want to execute. "
	@echo "                                                                                                                                       "
	@echo "                                                                                                                         "
	@echo "## Docker commands:"
	@echo "                                                                                                                         "
	@echo "  COMMAND=\"build\"                                Builds all Docker images defined in docker-compose.yml."
	@echo "  COMMAND=\"up\"                                   Starts and runs all services defined in docker-compose.yml."
	@echo "  COMMAND=\"start\"                                Starts existing containers defined in docker-compose.yml without creating them."
	@echo "  COMMAND=\"stop\"                                 Stops running containers defined in docker-compose.yml without removing them."
	@echo "  COMMAND=\"down\"                                 Stops and removes containers, networks, and volumes defined in docker-compose.yml."
	@echo "  COMMAND=\"destroy\"                              Stops and removes containers, networks, and volumes (including named volumes) defined in docker-compose.yml."
	@echo "  COMMAND=\"restart\"                              Stops and removes containers, networks, and volumes, then starts all services in detached mode."
	@echo "  COMMAND=\"logs\"                                 Shows the last 100 lines of logs and follows live log output for services defined in docker-compose.yml."
	@echo "  COMMAND=\"infra-api\"                             Shows the last 100 lines of logs and follows live log output for the infra service defined in docker-compose.yml."
	@echo "  COMMAND=\"ps\"                                   Lists the status of containers defined in docker-compose.yml."
	@echo "  COMMAND=\"db-shell\"                             Starts a PostgreSQL interactive terminal inside the infra container, logging in as the postgres user."


# Docker Compose Commands
build:
	@docker-compose -f docker-compose.yml build

up:
	@docker-compose -f docker-compose.yml up -d

start:
	@docker-compose -f docker-compose.yml start

down:
	@docker-compose -f docker-compose.yml down

destroy:
	@docker-compose -f docker-compose.yml down -v

stop:
	@docker-compose -f docker-compose.yml stop

restart:
	docker-compose -f docker-compose.yml down && \
    docker-compose -f docker-compose.yml up -d

logs:
	@docker-compose -f docker-compose.yml logs --tail=100 -f

logs-api:
	@docker-compose -f docker-compose.yml logs --tail=100 -f infra

ps:
	@docker-compose -f docker-compose.yml ps

infra-api:
	@docker-compose -f docker-compose.yml exec infra /bin/bash

db-shell:
	@docker-compose -f docker-compose.yml exec infra psql -Upostgres
