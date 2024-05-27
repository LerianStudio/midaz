service_name := ledger-service
bin_dir := ./.bin
artifacts_dir := ./artifacts

$(shell mkdir -p $(artifacts_dir))

.PHONY: info gen run test cover-html tidy help build up start down destroy stop restart logs logs-api ps login-timescale login-api db-shell

# Display available commands
info:
	@echo "                                                                                                                                       "
	@echo "                                                                                                                                       "
	@echo "To run a specific command inside the ledger container using make, you can execute:                                                     "
	@echo "                                                                                                                                       "
	@echo "make ledger COMMAND=\"any\"                                                                                                            "
	@echo "                                                                                                                                       "
	@echo "This command will run the specified command inside the ledger container. Replace \"any\" with the desired command you want to execute. "
	@echo "                                                                                                                                       "
	@echo "## App commands:"
	@echo "                                                                                                                         "
	@echo "  COMMAND=\"wire\"                                 Generates all project code to connect components using Wire."
	@echo "  COMMAND=\"run\"                                  Executes the Go program in internal/main.go using .env configuration."
	@echo "  COMMAND=\"test\"                                 Runs all Go tests in the project with verbose output."
	@echo "  COMMAND=\"cover-html\"                           Generates an HTML report from Go test coverage data."
	@echo "  COMMAND=\"tidy\"                                 Cleans and updates Go dependencies in the go.mod file."
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
	@echo "  COMMAND=\"logs-api\"                             Shows the last 100 lines of logs and follows live log output for the ledger service defined in docker-compose.yml."
	@echo "  COMMAND=\"ps\"                                   Lists the status of containers defined in docker-compose.yml."
	@echo "  COMMAND=\"ledger-api\"                           Starts an interactive Bash shell session inside the running ledger container defined in docker-compose.yml."
	@echo "  COMMAND=\"db-shell\"                             Starts a PostgreSQL interactive terminal inside the ledger container, logging in as the postgres user."



wire:
	@go generate ./...

run:
	@go run internal/main.go .env

test:
	@go test -v ./...

cover-html:
	@go tool cover -html=$(artifacts_dir)/coverage.out -o $(artifacts_dir)/coverage.html

tidy:
	@go mod tidy

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
	@docker-compose -f docker-compose.yml logs --tail=100 -f ledger

ps:
	@docker-compose -f docker-compose.yml ps

ledger-api:
	@docker-compose -f docker-compose.yml exec ledger /bin/bash

db-shell:
	@docker-compose -f docker-compose.yml exec ledger psql -Upostgres
