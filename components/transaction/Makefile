service_name := transaction-service
bin_dir := ./.bin
artifacts_dir := ./artifacts
ANTLR4_VERSION := 4.13.1

$(shell mkdir -p $(artifacts_dir))

DOCKER_VERSION := $(shell docker version --format '{{.Server.Version}}')
DOCKER_MIN_VERSION := 20.10.13

DOCKER_CMD := $(shell \
	if [ "$(shell printf '%s\n' "$(DOCKER_MIN_VERSION)" "$(DOCKER_VERSION)" | sort -V | head -n1)" = "$(DOCKER_MIN_VERSION)" ]; then \
		echo "docker compose"; \
	else \
		echo "docker-compose"; \
	fi \
)

# Display available commands
.PHONY: info
info:
	@echo "                                                                                                                                       "
	@echo "                                                                                                                                       "
	@echo "To run a specific command inside the transaction container using make, you can execute:                                                     "
	@echo "                                                                                                                                       "
	@echo "make transaction COMMAND=\"any\"                                                                                                            "
	@echo "                                                                                                                                       "
	@echo "This command will run the specified command inside the transaction container. Replace \"any\" with the desired command you want to execute. "
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
	@echo "                                                                                                                                       "
	@echo "                                                                                                                                       "


# Docker Compose Commands
.PHONY: build
build:
	@$(DOCKER_CMD) -f docker-compose.yml build $(c)

.PHONY: up
up:
	@$(DOCKER_CMD) -f docker-compose.yml up $(c) -d

.PHONY: start
start:
	@docker compose -f docker-compose.yml start $(c)

.PHONY: down
down:
	@$(DOCKER_CMD) -f docker-compose.yml down $(c)

.PHONY: destroy
destroy:
	@$(DOCKER_CMD) -f docker-compose.yml down -v $(c)

.PHONY: stop
stop:
	@$(DOCKER_CMD) -f docker-compose.yml stop $(c)

.PHONY: restart
restart:
	make stop && \
    make up

.PHONY: logs
logs:
	@$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f $(c)

.PHONY: logs-api
logs-api:
	@docker compose -f docker-compose.yml logs --tail=100 -f transaction

.PHONY: ps
ps:
	@$(DOCKER_CMD) -f docker-compose.yml ps

# ANTLR
.PHONY: antlr
antlr:
	cd ../../pkg/gold && \
	curl --continue-at - https://www.antlr.org/download/antlr-$(ANTLR4_VERSION)-complete.jar -O && \
	java -Xmx500m -cp antlr-$(ANTLR4_VERSION)-complete.jar org.antlr.v4.Tool -Dlanguage=Go -o parser Transaction.g4 -visitor && \
	find . | sed -e "s/[^-][^\/]*\// |/g" -e "s/|\([^ ]\)/|-\1/" && \
	rm -f antlr-$(ANTLR4_VERSION)-complete.jar

# App Commands
.PHONY: run
run:
	@go run cmd/app/main.go .env

.PHONY: test
test:
	@go test -v ./...

.PHONY: cover-html
cover-html:
	@go tool cover -html=$(artifacts_dir)/coverage.out -o $(artifacts_dir)/coverage.html

.PHONY: tidy
tidy:
	@go mod tidy

.PHONY: generate-docs
generate-docs:
	@swag init -g ../../../cmd/app/main.go -d ./internal/adapters/http -o ./api --parseDependency --parseInternal
	@docker run --rm -v ./:/transaction --user $(shell id -u):$(shell id -g) openapitools/openapi-generator-cli:v5.1.1 generate -i ./transaction/api/swagger.json -g openapi-yaml -o ./transaction/api
	@mv ./api/openapi/openapi.yaml ./api/openapi.yaml
	@rm -rf ./api/README.md ./api/.openapi-generator* ./api/openapi