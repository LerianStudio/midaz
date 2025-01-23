# Service configuration
service_name := transaction-service
bin_dir := ./.bin
artifacts_dir := ./artifacts
ANTLR4_VERSION := 4.13.1

# Use tput if available, fallback to ANSI codes if not
BLUE := $(shell command -v tput >/dev/null 2>&1 && tput setaf 4 || echo '\033[0;34m')
NC := $(shell command -v tput >/dev/null 2>&1 && tput sgr0 || echo '\033[0m')
BOLD := $(shell command -v tput >/dev/null 2>&1 && tput bold || echo '\033[1m')
RED := $(shell command -v tput >/dev/null 2>&1 && tput setaf 1 || echo '\033[0;31m')
MAGENTA := $(shell command -v tput >/dev/null 2>&1 && tput setaf 5 || echo '\033[0;35m')

$(shell mkdir -p $(artifacts_dir))

# Docker configuration
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
	@echo "$(BOLD)Transaction Service Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Usage:$(NC)"
	@echo "make transaction COMMAND=\"<command>\""
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "- make build                         - Build all Docker images"
	@echo "- make up                           - Start all services in detached mode"
	@echo "- make start                        - Start existing containers"
	@echo "- make stop                         - Stop running containers"
	@echo "- make down                         - Stop and remove containers"
	@echo "- make destroy                      - Stop and remove containers including volumes"
	@echo "- make restart                      - Restart all services"
	@echo "- make logs                         - Show and follow all container logs"
	@echo "- make logs-api                     - Show and follow transaction service logs"
	@echo "- make ps                           - List container status"
	@echo ""
	@echo "$(BOLD)ANTLR Commands:$(NC)"
	@echo "- make antlr                        - Generate ANTLR parser (version ${ANTLR4_VERSION})"
	@echo ""
	@echo "$(BOLD)App Commands:$(NC)"
	@echo "- make run                          - Run the application"
	@echo "- make test                         - Run tests"
	@echo "- make cover-html                   - Generate coverage report"
	@echo "- make tidy                         - Run go mod tidy"
	@echo "- make generate-docs                - Generate API documentation"
	@echo ""

# Docker Compose Commands
.PHONY: build
build:
	@echo "$(BLUE)Building Docker images...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml build $(c)

.PHONY: up
up:
	@echo "$(BLUE)Starting services...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml up $(c) -d

.PHONY: start
start:
	@echo "$(BLUE)Starting existing containers...$(NC)"
	@docker compose -f docker-compose.yml start $(c)

.PHONY: down
down:
	@echo "$(BLUE)Stopping and removing containers...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml down $(c)

.PHONY: destroy
destroy:
	@echo "$(BLUE)Stopping and removing containers with volumes...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml down -v $(c)

.PHONY: stop
stop:
	@echo "$(BLUE)Stopping containers...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml stop $(c)

.PHONY: restart
restart:
	@echo "$(BLUE)Restarting services...$(NC)"
	make stop && make up

.PHONY: logs
logs:
	@echo "$(BLUE)Showing container logs...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml logs --tail=100 -f $(c)

.PHONY: logs-api
logs-api:
	@echo "$(BLUE)Showing transaction service logs...$(NC)"
	@docker compose -f docker-compose.yml logs --tail=100 -f transaction

.PHONY: ps
ps:
	@echo "$(BLUE)Listing container status...$(NC)"
	@$(DOCKER_CMD) -f docker-compose.yml ps

# ANTLR Commands
.PHONY: antlr
antlr:
	@echo "$(BLUE)Generating ANTLR parser...$(NC)"
	cd ../../pkg/gold && \
	curl --continue-at - https://www.antlr.org/download/antlr-$(ANTLR4_VERSION)-complete.jar -O && \
	java -Xmx500m -cp antlr-$(ANTLR4_VERSION)-complete.jar org.antlr.v4.Tool -Dlanguage=Go -o parser Transaction.g4 -visitor && \
	find . | sed -e "s/[^-][^\/]*\// |/g" -e "s/|\([^ ]\)/|-\1/" && \
	rm -f antlr-$(ANTLR4_VERSION)-complete.jar

# App Commands
.PHONY: run
run:
	@echo "$(BLUE)Running application...$(NC)"
	@go run cmd/app/main.go .env

.PHONY: test
test:
	@echo "$(BLUE)Running tests...$(NC)"
	@go test -v ./...

.PHONY: cover-html
cover-html:
	@echo "$(BLUE)Generating coverage report...$(NC)"
	@go tool cover -html=$(artifacts_dir)/coverage.out -o $(artifacts_dir)/coverage.html

.PHONY: tidy
tidy:
	@echo "$(BLUE)Running go mod tidy...$(NC)"
	@go mod tidy

.PHONY: generate-docs
generate-docs:
	@echo "$(BLUE)Generating API documentation...$(NC)"
	@swag init -g ../../../cmd/app/main.go -d ./internal/adapters/http -o ./api --parseDependency --parseInternal
	@docker run --rm -v ./:/transaction --user $(shell id -u):$(shell id -g) openapitools/openapi-generator-cli:v5.1.1 generate -i ./transaction/api/swagger.json -g openapi-yaml -o ./transaction/api
	@mv ./api/openapi/openapi.yaml ./api/openapi.yaml
	@rm -rf ./api/README.md ./api/.openapi-generator* ./api/openapi