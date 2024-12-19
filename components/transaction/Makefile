service_name := transaction-service
bin_dir := ./.bin
artifacts_dir := ./artifacts
ANTLR4_VERSION := 4.13.1

$(shell mkdir -p $(artifacts_dir))

.PHONY: info gen run test cover-html tidy help build up start down destroy stop restart logs logs-api ps login-timescale login-api db-shell

# Display available commands
info:
	@echo "                                                                                                                                       "
	@echo "                                                                                                                                       "
	@echo "To run a specific command inside the transaction container using make, you can execute:                                                     "
	@echo "                                                                                                                                       "
	@echo "make transaction COMMAND=\"any\"                                                                                                            "
	@echo "                                                                                                                                       "
	@echo "This command will run the specified command inside the transaction container. Replace \"any\" with the desired command you want to execute. "
	@echo "                                                                                                                                       "
	@echo "## ANTLR Commands:"
	@echo "  COMMAND=\"antlr\"                                Run ANTLER version ${ANTLR4_VERSION} jar to read a Transaction.g4 file and to produced parse and visitor artifacts of Gold language."
	@echo "                                                                                                                                       "
	@echo "## App commands:"
	@echo "                                                                                                                         "
	@echo "  COMMAND=\"wire\"                                 Generates all project code to connect components using Wire."
	@echo "  COMMAND=\"run\"                                  Executes the Go program in internal/main.go using .env configuration."
	@echo "  COMMAND=\"test\"                                 Runs all Go tests in the project with verbose output."
	@echo "  COMMAND=\"cover-html\"                           Generates an HTML report from Go test coverage data."
	@echo "  COMMAND=\"tidy\"                                 Cleans and updates Go dependencies in the go.mod file."
	@echo "  COMMAND=\"generate-docs\" 						  Generates Swagger API documentation and an OpenAPI Specification."
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
	@echo "  COMMAND=\"logs-api\"                             Shows the last 100 lines of logs and follows live log output for the transaction service defined in docker-compose.yml."
	@echo "  COMMAND=\"ps\"                                   Lists the status of containers defined in docker-compose.yml."
	@echo "  COMMAND=\"transaction-api\"                      Starts an interactive Bash shell session inside the running transaction container defined in docker-compose.yml."



# ANTLR
antlr:
	cd ../../pkg/gold && \
	curl --continue-at - https://www.antlr.org/download/antlr-$(ANTLR4_VERSION)-complete.jar -O && \
	java -Xmx500m -cp antlr-$(ANTLR4_VERSION)-complete.jar org.antlr.v4.Tool -Dlanguage=Go -o parser Transaction.g4 -visitor && \
	find . | sed -e "s/[^-][^\/]*\// |/g" -e "s/|\([^ ]\)/|-\1/" && \
	rm -f antlr-$(ANTLR4_VERSION)-complete.jar

# COMMONS
run:
	@go run cmd/app/main.go .env

test:
	@go test -v ./...

cover-html:
	@go tool cover -html=$(artifacts_dir)/coverage.out -o $(artifacts_dir)/coverage.html

tidy:
	@go mod tidy

generate-docs:
	@swag init -g ../../../cmd/app/main.go -d ./internal/adapters/http -o ./api --parseDependency --parseInternal
	@docker run --rm -v ./:/transaction --user $(shell id -u):$(shell id -g) openapitools/openapi-generator-cli:v5.1.1 generate -i ./transaction/api/swagger.json -g openapi-yaml -o ./transaction/api
	@mv ./api/openapi/openapi.yaml ./api/openapi.yaml
	@rm -rf ./api/README.md ./api/.openapi-generator* ./api/openapi

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
	@docker-compose -f docker-compose.yml logs --tail=100 -f transaction

ps:
	@docker-compose -f docker-compose.yml ps

transaction-api:
	@docker-compose -f docker-compose.yml exec transaction /bin/bash