service_name := audit-service
bin_dir := ./.bin
artifacts_dir := ./artifacts

$(shell mkdir -p $(artifacts_dir))

.PHONY: info gen run test cover-html tidy help build up start down destroy stop restart logs logs-api ps generate-docs

# Display available commands
info:
	@echo "                                                                                                                                       "
	@echo "                                                                                                                                       "
	@echo "To run a specific command inside the auth container using make, you can execute:                                                     "
	@echo "                                                                                                                                       "
	@echo "make auth COMMAND=\"any\"                                                                                                            "
	@echo "                                                                                                                                       "
	@echo "This command will run the specified command inside the auth container. Replace \"any\" with the desired command you want to execute. "
	@echo "                                                                                                                         "
	@echo "## App commands:"
	@echo "                                                                                                                         "
	@echo "  COMMAND=\"generate-docs\" 						  Generates Swagger API documentation using swag."
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
	@echo "  COMMAND=\"logs-api\"                             Shows the last 100 lines of logs and follows live log output for the auth service defined in docker-compose.yml."
	@echo "  COMMAND=\"ps\"                                   Lists the status of containers defined in docker-compose.yml."

# Docker Compose Commands
build:
	@docker-compose -f docker-compose.yml build $(c)

up:
	@docker-compose -f docker-compose.yml up $(c) -d

start:
	@docker-compose -f docker-compose.yml start $(c)

down:
	@docker-compose -f docker-compose.yml down $(c)

destroy:
	@docker-compose -f docker-compose.yml down -v $(c)

stop:
	@docker-compose -f docker-compose.yml stop $(c)

restart:
	docker-compose -f docker-compose.yml down $(c) && \
	docker-compose -f docker-compose.yml up -d $(c)

logs:
	@docker-compose -f docker-compose.yml logs --tail=100 -f $(c)

logs-api:
	@docker-compose -f docker-compose.yml logs --tail=100 -f auth

ps:
	@docker-compose -f docker-compose.yml ps

generate-docs:
	@swag init -g ../../../cmd/app/main.go -d ./internal/adapters/http -o ./api --parseDependency --parseInternal