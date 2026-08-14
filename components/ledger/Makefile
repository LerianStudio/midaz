# Ledger Component Makefile (Unified Onboarding + Transaction)
#
# Thin component Makefile built from the shared monorepo template: it sets the
# component identity (SERVICE_NAME, MIDAZ_ROOT, scoped coverage) and includes the
# root mk/*.mk fragments rather than re-implementing docker/quality/security
# logic. Component-specific targets (build flags, crypto-key injection, run,
# dev-setup) live here.

# Component identity
SERVICE_NAME := ledger-service
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts

# Tool versions (GOLANGCI_LINT_VERSION, GOSEC_VERSION, ...) come from the shared
# mk/quality.mk + mk/security.mk fragments included below — single source of truth.

# Ensure build/output directories exist
$(shell mkdir -p $(BIN_DIR) $(ARTIFACTS_DIR))

# Root of the monorepo (for shared mk fragments and scoped coverage)
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)
COVERAGE_PACKAGES := ./components/ledger/...

# Shared monorepo fragments — one template, included by every component.
include $(MIDAZ_ROOT)/mk/colors.mk
include $(MIDAZ_ROOT)/mk/utils.mk
include $(MIDAZ_ROOT)/mk/docker.mk
include $(MIDAZ_ROOT)/mk/quality.mk
include $(MIDAZ_ROOT)/mk/security.mk
# Database migrations: provides ensure-migrate, MIGRATE_BIN, MIGRATE_VERSION and
# the single-DB migrate/migrate-down targets. The ledger serves TWO schemas, so
# the two-DB section below overrides migrate/migrate-down (last definition wins).
include $(MIDAZ_ROOT)/mk/database.mk
# Unit test coverage (scoped to ledger via COVERAGE_PACKAGES above).
include $(MIDAZ_ROOT)/mk/coverage-unit.mk

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)Ledger Service Commands (Unified Onboarding + Transaction)$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make help                        - Display this help message"
	@echo "  make build                       - Build the binary to .bin/ledger"
	@echo "  make test                        - Run tests"
	@echo "  make clean                       - Clean build artifacts"
	@echo "  make run                         - Run the application with .env config"
	@echo "  make coverage-unit               - Run unit tests with coverage report"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run golangci-lint (read-only gate)"
	@echo "  make lint-fix                    - Apply lint autofixes (mutates source)"
	@echo "  make format                      - Format code with go fmt"
	@echo "  make generate                    - Generate code (mocks, etc.)"
	@echo "  make tidy                        - Clean unused dependencies"
	@echo "  make sec                         - Run security checks (gosec + govulncheck)"
	@echo "  make sec SARIF=1                 - Run security checks with SARIF output"
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "  make up / down / start / stop / restart / rebuild-up / clean-docker / logs"
	@echo ""
	@echo "$(BOLD)Migration Commands (two-DB: onboarding + transaction):$(NC)"
	@echo "  make migrate                     - Apply migrations to BOTH schemas"
	@echo "  make migrate-onboarding          - Apply onboarding migrations only"
	@echo "  make migrate-transaction         - Apply transaction migrations only"
	@echo "  make migrate-down                - Roll back last migration on BOTH schemas"
	@echo "  make migrate-down-onboarding     - Roll back last onboarding migration"
	@echo "  make migrate-down-transaction    - Roll back last transaction migration"
	@echo ""
	@echo "$(BOLD)Ledger-Specific Commands:$(NC)"
	@echo "  make generate-keys               - Generate crypto keys for local testing"
	@echo "  (API docs are generated from the repo root: make generate-docs)"
	@echo ""
	@echo "$(BOLD)Developer Helper Commands:$(NC)"
	@echo "  make dev-setup                   - Set up development environment"
	@echo ""

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call title1,"Building Ledger component")
	@go build -o $(BIN_DIR)/ledger ./cmd/app/main.go
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Build completed successfully$(GREEN) ✔️$(NC)"

.PHONY: clean
clean:
	$(call title1,"Cleaning build artifacts")
	@rm -rf $(BIN_DIR)/* $(ARTIFACTS_DIR)/*
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Artifacts cleaned successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Test Commands
#-------------------------------------------------------

.PHONY: test
test:
	$(call title1,"Running tests")
	@go test -v ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Tests completed successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Ledger-Specific Commands
#-------------------------------------------------------

# `run` comes from mk/docker.mk (go run cmd/app/main.go .env).

# API documentation is generated from the repo root: `make generate-docs`
# (postman/generator/generate-docs.sh covers ledger and tracer).

#-------------------------------------------------------
# Database Migrations (two-DB: onboarding + transaction)
#-------------------------------------------------------
# The ledger binary serves two independent PostgreSQL schemas (onboarding and
# transaction), so it overrides the single-DB `migrate`/`migrate-down` targets
# from mk/database.mk with per-DB aggregates. The shared single-DB semantics
# are left intact for tracer, which includes the same fragment.
#
# The canonical migration path in local dev is the compose `ledger-migrate`
# service (run automatically by `make up` via the completed-successfully gate).
# These host targets are a thin convenience for running the golang-migrate CLI
# from the host against the infra Postgres.
#
# Connection assumption (host-run): infra maps midaz-postgres-primary as
# ${DB_PORT}:${DB_PORT} (5701:5701 in .env.example), so the host-exposed port
# equals the in-network port. Host targets therefore reach Postgres at
# localhost:$(DB_ONBOARDING_PORT) / localhost:$(DB_TRANSACTION_PORT). Values are
# read from .env at invocation time; missing values fall back to safe defaults.

# Per-DB migration file locations (baked into the migrations-image the same way).
ONBOARDING_MIGRATIONS_PATH := ./migrations/onboarding
TRANSACTION_MIGRATIONS_PATH := ./migrations/transaction

# read_env,KEY,DEFAULT — echo the value of KEY from ./.env, or DEFAULT if unset.
read_env = $(shell v=$$(grep -E "^$(1)=" .env 2>/dev/null | head -1 | cut -d'=' -f2-); echo "$${v:-$(2)}")

# Build a per-DB DSN from the DB_<PREFIX>_* .env values. Host-run targets use
# localhost (the in-network host name is not resolvable from the host).
define ledger_db_url
postgres://$(call read_env,DB_$(1)_USER,midaz):$(call read_env,DB_$(1)_PASSWORD,lerian)@localhost:$(call read_env,DB_$(1)_PORT,5701)/$(call read_env,DB_$(1)_NAME,$(2))?sslmode=$(call read_env,DB_$(1)_SSLMODE,disable)
endef

.PHONY: migrate-onboarding
migrate-onboarding: ensure-migrate
	$(call title1,"Applying onboarding migrations")
	@$(MIGRATE_BIN) -database "$(call ledger_db_url,ONBOARDING,onboarding)" -path $(ONBOARDING_MIGRATIONS_PATH) up
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Onboarding migrations applied successfully$(GREEN) ✔️$(NC)"

.PHONY: migrate-transaction
migrate-transaction: ensure-migrate
	$(call title1,"Applying transaction migrations")
	@$(MIGRATE_BIN) -database "$(call ledger_db_url,TRANSACTION,transaction)" -path $(TRANSACTION_MIGRATIONS_PATH) up
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Transaction migrations applied successfully$(GREEN) ✔️$(NC)"

# Aggregate override: apply BOTH schemas (shadows the single-DB mk/database.mk target).
.PHONY: migrate
migrate: migrate-onboarding migrate-transaction
	@echo "$(GREEN)$(BOLD)[ok]$(NC) All ledger migrations applied successfully$(GREEN) ✔️$(NC)"

.PHONY: migrate-down-onboarding
migrate-down-onboarding: ensure-migrate
	$(call title1,"Rolling back last onboarding migration")
	@$(MIGRATE_BIN) -database "$(call ledger_db_url,ONBOARDING,onboarding)" -path $(ONBOARDING_MIGRATIONS_PATH) down 1
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Last onboarding migration rolled back successfully$(GREEN) ✔️$(NC)"

.PHONY: migrate-down-transaction
migrate-down-transaction: ensure-migrate
	$(call title1,"Rolling back last transaction migration")
	@$(MIGRATE_BIN) -database "$(call ledger_db_url,TRANSACTION,transaction)" -path $(TRANSACTION_MIGRATIONS_PATH) down 1
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Last transaction migration rolled back successfully$(GREEN) ✔️$(NC)"

# Aggregate override: roll back BOTH schemas (shadows the single-DB mk/database.mk target).
.PHONY: migrate-down
migrate-down: migrate-down-onboarding migrate-down-transaction
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Last ledger migration rolled back on both schemas$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Setup Commands
#-------------------------------------------------------

.PHONY: set-env
set-env:
	$(call title1,"Setting up environment file")
	@if [ -f ".env.example" ] && [ ! -f ".env" ]; then \
		echo "Creating .env from .env.example"; \
		cp ".env.example" ".env"; \
	elif [ ! -f ".env.example" ]; then \
		echo "$(YELLOW)Warning: No .env.example found$(NC)"; \
	else \
		echo ".env already exists"; \
	fi
	@$(MAKE) generate-keys
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Environment file set up successfully$(GREEN) ✔️$(NC)"

# Generate and inject a crypto key into .env if it is unset or still the
# placeholder. The collapsed CRM code reads the bare LCRYPTO_* env names.
# Usage: $(call inject_crypto_key,KEY_NAME,PLACEHOLDER)
define inject_crypto_key
	@CURRENT_VAL=$$(grep "^$(1)=" .env 2>/dev/null | cut -d'=' -f2); \
	if [ -z "$$CURRENT_VAL" ] || [ "$$CURRENT_VAL" = "$(2)" ]; then \
		echo "  Generating $(1)..."; \
		NEW_KEY=$$(docker run --rm alpine/openssl rand -hex 32 2>/dev/null); \
		if [ -z "$$NEW_KEY" ] || [ $${#NEW_KEY} -ne 64 ]; then \
			echo "$(RED)  ✗ Failed to generate key. Ensure Docker is running.$(NC)"; \
			exit 1; \
		fi; \
		if grep -q "^$(1)=" ".env"; then \
			sed -i.bak "s|^$(1)=.*|$(1)=$$NEW_KEY|" ".env" && rm -f ".env.bak"; \
		else \
			echo "$(1)=$$NEW_KEY" >> ".env"; \
		fi; \
		echo "$(GREEN)  ✓ $(1) generated$(NC)"; \
	else \
		echo "$(GREEN)  ✓ $(1) already set$(NC)"; \
	fi
endef

.PHONY: generate-keys
generate-keys:
	$(call title1,"Generating crypto keys for local testing")
	@if [ ! -f ".env" ]; then \
		echo "$(YELLOW)  ! No .env file found. Run 'make set-env' first.$(NC)"; \
		exit 0; \
	fi
	$(call inject_crypto_key,LCRYPTO_HASH_SECRET_KEY,my-hash-secret-key)
	$(call inject_crypto_key,LCRYPTO_ENCRYPT_SECRET_KEY,my-encrypt-secret-key)
	@echo ""
	@echo "$(YELLOW)  ⚠️  WARNING: Generated keys are for LOCAL/TEST use only.$(NC)"
	@echo "$(YELLOW)     For production, generate keys securely and store in a secrets manager.$(NC)"

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call title1,"Setting up development environment")
	@echo "$(CYAN)Installing development tools...$(NC)"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing golangci-lint...$(NC)"; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	@if ! command -v mockgen >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing mockgen...$(NC)"; \
		go install go.uber.org/mock/mockgen@v0.6.0; \
	fi
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing gosec...$(NC)"; \
		go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION); \
	fi
	@echo "$(CYAN)Setting up environment...$(NC)"
	@if [ -f .env.example ] && [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "$(GREEN)Created .env file from template$(NC)"; \
	fi
	@$(MAKE) generate-keys
	@$(MAKE) tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully$(GREEN) ✔️$(NC)"
	@echo "$(CYAN)You're ready to start developing! Here are some useful commands:$(NC)"
	@echo "  make build         - Build the component"
	@echo "  make test          - Run tests"
	@echo "  make up            - Start services"
	@echo "  make rebuild-up    - Rebuild and restart services during development"
