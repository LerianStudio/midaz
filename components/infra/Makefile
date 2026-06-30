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
# mockgen has no shared pin yet, so dev-setup pins it here.
MOCKGEN_VERSION ?= v0.6.0

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
	@echo "$(BOLD)Vault Transit Commands:$(NC)"
	@echo "  make vault-transit-single                    - Enable the single-tenant transit engine (transit-st)"
	@echo "  make vault-transit-multi                     - Enable the multi-tenant shared transit engine (transit-mt)"
	@echo "  make vault-transit-setup                     - Pick transit-st/transit-mt from MULTI_TENANT_ENABLED in ledger/.env"
	@echo "  make vault-transit-list                      - List transit mounts"
	@echo "  make vault-transit-disable MOUNT=<path>      - Disable a transit mount"
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
	@# Reinstall only when missing OR the installed version does not match the
	@# pin, so an older preinstalled binary is upgraded (existence alone is not
	@# enough) without a slow blind reinstall on every dev-setup. Versions are
	@# matched as substrings of each tool's --version output, with the leading
	@# "v" stripped from the pin to match tools that report a bare semver.
	@if ! command -v golangci-lint >/dev/null 2>&1 || \
		! golangci-lint version 2>&1 | grep -q "$(GOLANGCI_LINT_VERSION:v%=%)"; then \
		echo "$(YELLOW)Installing golangci-lint $(GOLANGCI_LINT_VERSION)...$(NC)"; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	@if ! command -v mockgen >/dev/null 2>&1 || \
		! mockgen --version 2>&1 | grep -q "$(MOCKGEN_VERSION:v%=%)"; then \
		echo "$(YELLOW)Installing mockgen $(MOCKGEN_VERSION)...$(NC)"; \
		go install go.uber.org/mock/mockgen@$(MOCKGEN_VERSION); \
	fi
	@if ! command -v gosec >/dev/null 2>&1 || \
		! gosec --version 2>&1 | grep -q "$(GOSEC_VERSION:v%=%)"; then \
		echo "$(YELLOW)Installing gosec $(GOSEC_VERSION)...$(NC)"; \
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
# Provision the single shared Vault Transit secrets engine for envelope encryption.
#   Single-tenant: one engine at $(TRANSIT_MOUNT_ST).
#   Multi-tenant : one engine at $(TRANSIT_MOUNT_MT).
# There are no per-tenant mounts. Tenant isolation is handled by the app via the
# transit key name ({tenantID}/org-<uuid>) inside the single engine.

VAULT_CONTAINER ?= midaz-hc-vault
VAULT_DEV_TOKEN ?= root
TRANSIT_MOUNT_ST ?= transit-st
TRANSIT_MOUNT_MT ?= transit-mt
VAULT_SH = docker exec $(VAULT_CONTAINER) sh -c

define vault_running
	@docker ps --format '{{.Names}}' | grep -q '^$(VAULT_CONTAINER)$$' || { echo "$(YELLOW)Vault container '$(VAULT_CONTAINER)' is not running. Start it first (make up).$(NC)"; exit 1; }
endef

.PHONY: vault-transit-single
vault-transit-single:
	$(call title1,"Enabling single-tenant Vault Transit engine $(TRANSIT_MOUNT_ST)")
	$(call vault_running)
	@$(VAULT_SH) 'export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=$(VAULT_DEV_TOKEN); \
		if vault secrets list -format=json | grep -q "\"$(TRANSIT_MOUNT_ST)/\""; then \
			echo "transit already enabled at $(TRANSIT_MOUNT_ST)/"; \
		else \
			vault secrets enable -path=$(TRANSIT_MOUNT_ST) transit && echo "enabled transit at $(TRANSIT_MOUNT_ST)/"; \
		fi'
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Single-tenant transit ready$(GREEN) ✔️$(NC)"

.PHONY: vault-transit-multi
vault-transit-multi:
	$(call title1,"Enabling multi-tenant shared Vault Transit engine $(TRANSIT_MOUNT_MT)")
	$(call vault_running)
	@$(VAULT_SH) 'export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=$(VAULT_DEV_TOKEN); \
		if vault secrets list -format=json | grep -q "\"$(TRANSIT_MOUNT_MT)/\""; then \
			echo "transit already enabled at $(TRANSIT_MOUNT_MT)/"; \
		else \
			vault secrets enable -path=$(TRANSIT_MOUNT_MT) transit && echo "enabled transit at $(TRANSIT_MOUNT_MT)/"; \
		fi'
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Multi-tenant transit ready$(GREEN) ✔️$(NC)"

.PHONY: vault-transit-setup
vault-transit-setup:
	$(call title1,"Provisioning Vault Transit from MULTI_TENANT_ENABLED")
	@MT=$$(grep -E "^MULTI_TENANT_ENABLED=" $(MIDAZ_ROOT)/components/ledger/.env 2>/dev/null | cut -d= -f2 | tr -d '[:space:]"'); \
	if [ "$${MT:-false}" = "true" ]; then \
		echo "$(CYAN)MULTI_TENANT_ENABLED=true -> $(TRANSIT_MOUNT_MT) engine$(NC)"; \
		$(MAKE) vault-transit-multi; \
	else \
		echo "$(CYAN)MULTI_TENANT_ENABLED!=true -> $(TRANSIT_MOUNT_ST) engine$(NC)"; \
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
	@[ -n "$(MOUNT)" ] || { echo "$(YELLOW)Provide MOUNT=<path> (e.g. transit-st or transit-mt)$(NC)"; exit 1; }
	$(call vault_running)
	@$(VAULT_SH) 'export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=$(VAULT_DEV_TOKEN); vault secrets disable $(MOUNT) && echo "disabled $(MOUNT)"'
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Mount disabled$(GREEN) ✔️$(NC)"
