# Infra Component Makefile
#
# Thin component Makefile built from the shared monorepo template: it sets the
# component identity (SERVICE_NAME, MIDAZ_ROOT) and includes the root mk/*.mk
# fragments rather than re-implementing docker/quality/security logic.
# Component-specific targets (no-op build, clean, coverage check, dev-setup)
# live here.

# Component identity
SERVICE_NAME := infra-service
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts

# Tool versions (GOLANGCI_LINT_VERSION, GOSEC_VERSION, ...) come from the shared
# mk/quality.mk + mk/security.mk fragments included below — single source of truth.

# Ensure artifacts directory exists
$(shell mkdir -p $(ARTIFACTS_DIR))

# Root of the monorepo (for shared mk fragments)
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)

# Shared monorepo fragments — one template, included by every component.
include $(MIDAZ_ROOT)/mk/colors.mk
include $(MIDAZ_ROOT)/mk/utils.mk
include $(MIDAZ_ROOT)/mk/docker.mk
include $(MIDAZ_ROOT)/mk/quality.mk
include $(MIDAZ_ROOT)/mk/security.mk

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)Infra Service Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make build                       - Build the component (no-op)"
	@echo "  make clean                       - Clean build artifacts"
	@echo "  make coverage-summary            - Verify test coverage"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run golangci-lint (read-only gate)"
	@echo "  make lint-fix                    - Apply lint autofixes (mutates source)"
	@echo "  make format                      - Format code with go fmt"
	@echo "  make tidy                        - Clean unused dependencies"
	@echo "  make sec                         - Run security checks (gosec + govulncheck)"
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "  make up / down / start / stop / restart / rebuild-up / clean-docker / logs"
	@echo ""
	@echo "$(BOLD)Developer Helper Commands:$(NC)"
	@echo "  make dev-setup                   - Set up development environment"
	@echo ""

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call title1,"Building Infra component")
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
# Test Coverage Commands
#-------------------------------------------------------

.PHONY: coverage-summary
coverage-summary:
	$(call title1,"Verifying test coverage")
	@if find . -name "*.go" -type f | grep -q .; then \
		echo "$(CYAN)Running test coverage check...$(NC)"; \
		go test -coverprofile=coverage.tmp ./...; \
		test_exit=$$?; \
		if [ $$test_exit -ne 0 ]; then \
			echo "$(RED)$(BOLD)[error]$(NC) Tests failed (exit code $$test_exit). Run 'make test' for details.$(RED) ❌$(NC)"; \
			rm -f coverage.tmp; \
			exit 1; \
		fi; \
		if [ -f coverage.tmp ]; then \
			coverage=$$(go tool cover -func=coverage.tmp | grep total | awk '{print $$3}'); \
			echo "$(CYAN)Test coverage: $(GREEN)$$coverage$(NC)"; \
			rm coverage.tmp; \
		else \
			echo "$(YELLOW)No coverage data generated$(NC)"; \
		fi; \
	else \
		echo "$(YELLOW)No Go files found, skipping test coverage check$(NC)"; \
	fi

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
	@$(MAKE) tidy
	@$(MAKE) coverage-summary
	@$(MAKE) sec
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully$(GREEN) ✔️$(NC)"
	@echo "$(CYAN)You're ready to start developing! Here are some useful commands:$(NC)"
	@echo "  make build         - Build the component"
	@echo "  make test          - Run tests"
	@echo "  make up            - Start services"
	@echo "  make rebuild-up    - Rebuild and restart services during development"

#-------------------------------------------------------
# Vault Transit Commands
#-------------------------------------------------------
# Provision the Vault Transit secrets engine for envelope encryption.
#   Single-tenant: one flat mount at $(TRANSIT_MOUNT).
#   Multi-tenant : one mount per tenant at $(TRANSIT_MOUNT)/<tenantID>.
# A flat mount and per-tenant mounts cannot coexist (they collide on the
# "$(TRANSIT_MOUNT)/" prefix), so the multi-tenant target refuses to run while a
# flat mount exists unless FORCE=1 is passed to disable it first.

VAULT_CONTAINER ?= midaz-hc-vault
VAULT_DEV_TOKEN ?= root
TRANSIT_MOUNT ?= transit
VAULT_SH = docker exec $(VAULT_CONTAINER) sh -c

define vault_running
	@docker ps --format '{{.Names}}' | grep -q '^$(VAULT_CONTAINER)$$' || { echo "$(YELLOW)Vault container '$(VAULT_CONTAINER)' is not running. Start it first (make up).$(NC)"; exit 1; }
endef

.PHONY: vault-transit-single
vault-transit-single:
	$(call title1,"Enabling Vault Transit single-tenant mount $(TRANSIT_MOUNT)")
	$(call vault_running)
	@$(VAULT_SH) 'export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=$(VAULT_DEV_TOKEN); \
		if vault secrets list -format=json | grep -q "\"$(TRANSIT_MOUNT)/\""; then \
			echo "transit already enabled at $(TRANSIT_MOUNT)/"; \
		else \
			vault secrets enable -path=$(TRANSIT_MOUNT) transit && echo "enabled transit at $(TRANSIT_MOUNT)/"; \
		fi'
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Single-tenant transit ready$(GREEN) ✔️$(NC)"

.PHONY: vault-transit-multi
vault-transit-multi:
	$(call title1,"Enabling Vault Transit multi-tenant mounts")
	@[ -n "$(TENANTS)" ] || { echo "$(YELLOW)Provide TENANTS=\"<id> [<id> ...]\"$(NC)"; exit 1; }
	$(call vault_running)
	@$(VAULT_SH) 'export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=$(VAULT_DEV_TOKEN); \
		if vault secrets list -format=json | grep -q "\"$(TRANSIT_MOUNT)/\""; then \
			if [ "$(FORCE)" = "1" ]; then \
				echo "disabling flat $(TRANSIT_MOUNT)/ mount (FORCE=1)"; vault secrets disable $(TRANSIT_MOUNT); \
			else \
				echo "ERROR: flat \"$(TRANSIT_MOUNT)/\" mount exists and conflicts with per-tenant mounts."; \
				echo "Re-run with FORCE=1 to disable it first (this deletes its keys)."; exit 1; \
			fi; \
		fi; \
		for t in $(TENANTS); do \
			if vault secrets list -format=json | grep -q "\"$(TRANSIT_MOUNT)/$$t/\""; then \
				echo "transit already enabled at $(TRANSIT_MOUNT)/$$t/"; \
			else \
				vault secrets enable -path=$(TRANSIT_MOUNT)/$$t transit && echo "enabled transit at $(TRANSIT_MOUNT)/$$t/"; \
			fi; \
		done'
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Multi-tenant transit ready$(GREEN) ✔️$(NC)"

.PHONY: vault-transit-setup
vault-transit-setup:
	$(call title1,"Provisioning Vault Transit from MULTI_TENANT_ENABLED")
	@MT=$$(grep -E "^MULTI_TENANT_ENABLED=" $(MIDAZ_ROOT)/components/ledger/.env 2>/dev/null | cut -d= -f2 | tr -d '[:space:]"'); \
	if [ "$${MT:-false}" = "true" ]; then \
		echo "$(CYAN)MULTI_TENANT_ENABLED=true -> multi-tenant transit$(NC)"; \
		if [ -z "$(TENANTS)" ]; then echo "$(YELLOW)Provide TENANTS=\"<id> [<id> ...]\"$(NC)"; exit 1; fi; \
		$(MAKE) vault-transit-multi TENANTS="$(TENANTS)" FORCE="$(FORCE)"; \
	else \
		echo "$(CYAN)MULTI_TENANT_ENABLED!=true -> single-tenant transit$(NC)"; \
		$(MAKE) vault-transit-single; \
	fi

.PHONY: vault-transit-list
vault-transit-list:
	$(call title1,"Vault Transit mounts")
	$(call vault_running)
	@$(VAULT_SH) 'export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=$(VAULT_DEV_TOKEN); vault secrets list' | grep -E "Path|transit" || echo "(no transit mounts)"

.PHONY: vault-transit-disable
vault-transit-disable:
	$(call title1,"Disabling Vault Transit mount: $(MOUNT)")
	@[ -n "$(MOUNT)" ] || { echo "$(YELLOW)Provide MOUNT=<path> (e.g. transit or transit/<tenantID>)$(NC)"; exit 1; }
	$(call vault_running)
	@$(VAULT_SH) 'export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=$(VAULT_DEV_TOKEN); vault secrets disable $(MOUNT) && echo "disabled $(MOUNT)"'
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Mount disabled$(GREEN) ✔️$(NC)"
