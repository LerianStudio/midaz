# Reporter Manager Component Makefile (REST API, no Chromium)
#
# Thin component Makefile built from the shared monorepo template: it sets the
# component identity (SERVICE_NAME, MIDAZ_ROOT, scoped coverage) and includes the
# root mk/*.mk fragments rather than re-implementing docker/quality/security
# logic. Component-specific targets (build flags, docs, mocks) live here.

# Component identity
SERVICE_NAME := reporter-manager
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts

# Pinned tool versions — keep in sync with .github/workflows/go-combined-analysis.yml
GOLANGCI_LINT_VERSION := v2.4.0

# Ensure build/output directories exist
$(shell mkdir -p $(BIN_DIR) $(ARTIFACTS_DIR))

# Root of the monorepo (for shared mk fragments and scoped coverage)
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)
COVERAGE_PACKAGES := ./components/reporter-manager/...

# Shared monorepo fragments — one template, included by every component.
# No database.mk/docs.mk: reporter-manager ships no SQL migrations and uses a
# bespoke swag invocation below (no docker OpenAPI conversion step).
include $(MIDAZ_ROOT)/mk/colors.mk
include $(MIDAZ_ROOT)/mk/utils.mk
include $(MIDAZ_ROOT)/mk/docker.mk
include $(MIDAZ_ROOT)/mk/quality.mk
include $(MIDAZ_ROOT)/mk/security.mk
include $(MIDAZ_ROOT)/mk/coverage-unit.mk

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)Reporter Manager Service Commands (REST API)$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make build                       - Build the component"
	@echo "  make test                        - Run tests"
	@echo "  make coverage-unit               - Run unit tests with coverage report"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run linting tools"
	@echo "  make format                      - Format code"
	@echo "  make tidy                        - Clean dependencies"
	@echo "  make sec                         - Run security checks (gosec + govulncheck)"
	@echo ""
	@echo "$(BOLD)Docker Commands:$(NC)"
	@echo "  make up / down / start / stop / restart / rebuild-up / logs / ps / clean-docker"
	@echo ""
	@echo "$(BOLD)Reporter-Manager-Specific Commands:$(NC)"
	@echo "  make generate-docs               - Generate Swagger API documentation"
	@echo "  make generate-mocks              - Generate gomock mocks (go generate)"
	@echo ""
	@echo "$(BOLD)Developer Helper Commands:$(NC)"
	@echo "  make dev-setup                   - Set up development environment"
	@echo ""

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call title1,"Building Reporter Manager component")
	@CGO_ENABLED=0 go build -tags netgo -ldflags '-s -w -extldflags "-static"' -o $(BIN_DIR)/$(SERVICE_NAME) ./cmd/app/main.go
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
# Reporter-Manager-Specific Commands
#-------------------------------------------------------

.PHONY: generate-docs
generate-docs:
	$(call title1,"Generating Swagger API documentation - Reporter Manager")
	@SWAG_BIN=$$(command -v swag 2>/dev/null || true); \
	if [ -z "$$SWAG_BIN" ]; then \
		echo "$(YELLOW)Installing swag...$(NC)"; \
		go install github.com/swaggo/swag/cmd/swag@v1.16.6; \
		GO_BIN=$$(go env GOBIN); \
		if [ -z "$$GO_BIN" ]; then GO_BIN="$$(go env GOPATH)/bin"; fi; \
		SWAG_BIN="$$GO_BIN/swag"; \
	fi; \
	cd $(MIDAZ_ROOT)/components/reporter-manager && "$$SWAG_BIN" init -g cmd/app/main.go -o api --parseDependency --parseInternal --outputTypes go,json,yaml
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Swagger documentation generated$(GREEN) ✔️$(NC)"

.PHONY: generate-mocks
generate-mocks:
	$(call title1,"Generating gomock mocks - Reporter Manager")
	@if ! command -v mockgen >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing mockgen...$(NC)"; \
		go install go.uber.org/mock/mockgen@v0.6.0; \
	fi
	@export PATH="$$(go env GOPATH)/bin:$$PATH"; go generate ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Mocks generated$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Setup / Developer Helper Commands
#-------------------------------------------------------

.PHONY: set-env
set-env:
	$(call title1,"Setting up environment files")
	@if [ -f ".env.example" ] && [ ! -f ".env" ]; then \
		echo "$(CYAN)Creating .env from .env.example$(NC)"; \
		cp ".env.example" ".env"; \
	elif [ ! -f ".env.example" ]; then \
		echo "$(YELLOW)Warning: No .env.example found$(NC)"; \
	else \
		echo "$(GREEN).env already exists$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Environment files set up successfully$(GREEN) ✔️$(NC)"

.PHONY: dev-setup
dev-setup:
	$(call title1,"Setting up development environment")
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	@if ! command -v swag >/dev/null 2>&1; then \
		go install github.com/swaggo/swag/cmd/swag@latest; \
	fi
	@$(MAKE) set-env
	@$(MAKE) tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully$(GREEN) ✔️$(NC)"
