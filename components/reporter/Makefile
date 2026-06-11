# Reporter Component Makefile (unified API + worker binary)
#
# Thin component Makefile built from the shared monorepo template: it sets the
# component identity (SERVICE_NAME, MIDAZ_ROOT, scoped coverage) and includes the
# root mk/*.mk fragments rather than re-implementing docker/quality/security
# logic. Component-specific targets (static build flags, mocks) live here.
#
# ONE binary, all RUN_MODEs: the unified reporter binary serves the REST API
# (RUN_MODE=api), the RabbitMQ report-generation worker (RUN_MODE=worker), or
# both (RUN_MODE=all, dev-only). Production splits api/worker into two
# Deployments from the SAME image; the two component images
# (midaz-reporter-manager / midaz-reporter-worker) are built by the Dockerfiles
# under components/reporter-manager and components/reporter-worker, which compile
# THIS component's ./cmd/app/main.go and differ only by base image + default
# RUN_MODE. See those Dockerfiles for the api/worker split.

# Component identity
SERVICE_NAME := reporter
BIN_DIR := ./.bin
ARTIFACTS_DIR := ./artifacts

# Pinned tool versions — keep in sync with .github/workflows/go-combined-analysis.yml
GOLANGCI_LINT_VERSION := v2.12.2

# Ensure build/output directories exist
$(shell mkdir -p $(BIN_DIR) $(ARTIFACTS_DIR))

# Root of the monorepo (for shared mk fragments and scoped coverage)
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)
COVERAGE_PACKAGES := ./components/reporter/...

# Shared monorepo fragments — one template, included by every component.
# No database.mk: the reporter ships no SQL migrations. API docs are generated
# from the repo root (postman/generator/generate-docs.sh).
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
	@echo "$(BOLD)Reporter Service Commands (unified API + worker binary)$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make build                       - Build the unified component binary (.bin/reporter)"
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
	@echo "  make up / down / start / stop / restart / rebuild-up / logs / clean-docker"
	@echo ""
	@echo "$(BOLD)Reporter-Specific Commands:$(NC)"
	@echo "  (API docs are generated from the repo root: make generate-docs)"
	@echo "  make generate-mocks              - Generate gomock mocks (go generate)"
	@echo ""
	@echo "$(BOLD)Runtime modes (RUN_MODE env):$(NC)"
	@echo "  api    - REST API surface only (production api Deployment, :4005)"
	@echo "  worker - RabbitMQ consumer + health server only (production worker Deployment, :4006)"
	@echo "  all    - both surfaces in one process (DEV ONLY; default when unset)"
	@echo ""
	@echo "$(BOLD)Developer Helper Commands:$(NC)"
	@echo "  make dev-setup                   - Set up development environment"
	@echo ""

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

# ONE build: the unified binary supports every RUN_MODE at runtime. The two
# production images are produced from this binary by the manager/worker
# Dockerfiles, NOT by separate build targets.
.PHONY: build
build:
	$(call title1,"Building Reporter component")
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
# Reporter-Specific Commands
#-------------------------------------------------------

# API documentation is generated from the repo root: `make generate-docs`
# (postman/generator/generate-docs.sh covers ledger, tracer and reporter).

.PHONY: generate-mocks
generate-mocks:
	$(call title1,"Generating gomock mocks - Reporter")
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
	@if ! command -v mockgen >/dev/null 2>&1; then \
		go install go.uber.org/mock/mockgen@v0.6.0; \
	fi
	@$(MAKE) set-env
	@$(MAKE) tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully$(GREEN) ✔️$(NC)"
