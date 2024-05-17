service_name := ledger-service
bin_dir := ./.bin
artifacts_dir := ./artifacts

$(shell mkdir -p $(artifacts_dir))

.PHONY: info gen run test cover-html tidy help build up start down destroy stop restart logs logs-api ps login-timescale login-api db-shell

# Display available commands
info:
	@echo "Available commands:"
	@echo "  make wire"
	@echo "  make run"
	@echo "  make test"

gen:
	@go generate ./...

run:
	@go run internal/main.go .env

test:
	@go test -v ./...

cover-html:
	@go tool cover -html=$(artifacts_dir)/coverage.out -o $(artifacts_dir)/coverage.html

tidy:
	@go mod tidy

# Help command to list other commands
help:
	@make -pRrq -f $(lastword $(MAKEFILE_LIST)) : 2>/dev/null | \
	awk -v RS= -F: '/^# File/,/^# Finished Make data base/ {if ($$1 !~ "^[#.]") {print $$1}}' | \
	sort | egrep -v -e '^[^[:alnum:]]' -e '^$@$$'

# Docker Compose Commands
build:
	@docker-compose -f docker-compose.yml build $(c)

up:
	@docker-compose -f docker-compose.yml up $(c)

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
	@docker-compose -f docker-compose.yml logs --tail=100 -f ledger

ps:
	@docker-compose -f docker-compose.yml ps

ledger-api:
	@docker-compose -f docker-compose.yml exec ledger /bin/bash

db-shell:
	@docker-compose -f docker-compose.yml exec ledger psql -Upostgres
