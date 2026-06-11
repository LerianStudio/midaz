# ------------------------------------------------------
# Database migration and seed management commands
# ------------------------------------------------------
# This file contains all database operations including
# migrations, rollbacks, versioning, and seed data management.
#
# Requirements:
#   - golang-migrate/migrate CLI (auto-installed if missing)
#   - PostgreSQL database accessible via DATABASE_URL
#   - Docker Compose for seed operations (uses POSTGRES_SERVICE)
#
# Variables (override per component via `?=`):
#   - DATABASE_URL: Connection string
#   - POSTGRES_USER: PostgreSQL username for seed operations
#   - POSTGRES_DB: PostgreSQL database name for seed operations
#   - MIGRATIONS_PATH: Path to migration files (default: ./migrations)
#   - POSTGRES_SERVICE: Docker service name (the shared infra Postgres)
#   - DOCKER_CMD: Docker compose command (from mk/utils.mk)
#
# Usage:
#   make migrate                     # Apply all pending migrations
#   make migrate-down                # Rollback last migration
#   make seed                        # Load development seed data
# ------------------------------------------------------

# Database configuration (component Makefiles override as needed)
DATABASE_URL ?= postgres://midaz:lerian@localhost:5432/$(SERVICE_NAME)?sslmode=disable
POSTGRES_USER ?= midaz
POSTGRES_DB ?= $(SERVICE_NAME)
MIGRATIONS_PATH ?= ./migrations
POSTGRES_SERVICE ?= midaz-postgres-primary

# Pinned golang-migrate CLI version — keep the gate deterministic.
MIGRATE_VERSION ?= v4.19.1

# Path to the installed migrate binary
MIGRATE_BIN := $(or $(GOBIN),$(shell go env GOPATH)/bin)/migrate

#-------------------------------------------------------
# Commands (alphabetically ordered)
#-------------------------------------------------------

# Ensure golang-migrate CLI is installed
.PHONY: ensure-migrate
ensure-migrate:
	@if [ ! -x "$(MIGRATE_BIN)" ]; then \
		echo "$(YELLOW)Installing golang-migrate...$(NC)"; \
		go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION); \
	fi

# Apply all pending migrations
.PHONY: migrate
migrate: ensure-migrate
	$(call title1,"Applying database migrations")
	@$(MIGRATE_BIN) -database "$(DATABASE_URL)" -path $(MIGRATIONS_PATH) up
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Migrations applied successfully$(GREEN) ✔️$(NC)"

# Rollback the last migration
.PHONY: migrate-down
migrate-down: ensure-migrate
	$(call title1,"Rolling back last migration")
	@$(MIGRATE_BIN) -database "$(DATABASE_URL)" -path $(MIGRATIONS_PATH) down 1
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Last migration rolled back successfully$(GREEN) ✔️$(NC)"

# Load development seed data into database
# Requires: ./migrations/seeds/001_dev_data.sql
# Uses Docker Compose to execute SQL against the PostgreSQL service
.PHONY: seed
seed:
	$(call title1,"Loading development seed data")
	@if [ ! -f "$(MIGRATIONS_PATH)/seeds/001_dev_data.sql" ]; then \
		echo "$(YELLOW)No seed file found at $(MIGRATIONS_PATH)/seeds/001_dev_data.sql$(NC)"; \
		echo "$(YELLOW)Skipping seed data loading$(NC)"; \
	else \
		$(DOCKER_CMD) -f docker-compose.yml exec -T $(POSTGRES_SERVICE) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) < $(MIGRATIONS_PATH)/seeds/001_dev_data.sql; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Seed data loaded successfully$(GREEN) ✔️$(NC)"; \
	fi
