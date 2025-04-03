# Midaz Go SDK Makefile

# Component-specific variables
SERVICE_NAME := Midaz Go SDK
BIN_DIR := ./bin
ARTIFACTS_DIR := ./artifacts
DOCS_DIR := ./docs/godoc
VERSION := 1.0.0

# Ensure artifacts directory exists
$(shell mkdir -p $(ARTIFACTS_DIR))
$(shell mkdir -p $(DOCS_DIR))

# Define the root directory of the project
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)

# Include shared color definitions and utility functions
include $(MIDAZ_ROOT)/pkg/shell/makefile_colors.mk
include $(MIDAZ_ROOT)/pkg/shell/makefile_utils.mk

# Check if Go is installed
GO := $(shell which go)
ifeq (, $(GO))
$(error "No go binary found in your system, please install go before continuing")
endif

# Go commands
GO := go
GOFMT := gofmt
GOLINT := golangci-lint
GOMOD := $(GO) mod
GOBUILD := $(GO) build
GOTEST := $(GO) test
GOTOOL := $(GO) tool
GOGET := $(GO) get
GOCLEAN := $(GO) clean

# Project variables
PROJECT_ROOT := $(shell pwd)
PROJECT_NAME := midaz-go-sdk
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

# API change detector variables
API_DETECTOR := $(PROJECT_ROOT)/utils/api_change_detector.go
ROOT_DIR := $(MIDAZ_ROOT)

# Environment variables
ENV_FILE := $(PROJECT_ROOT)/.env
ENV_EXAMPLE := $(PROJECT_ROOT)/.env.example

# Load environment variables if .env exists
ifneq (,$(wildcard .env))
    include .env
endif

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)$(SERVICE_NAME) Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make help                        - Display this help message"
	@echo "  make test                        - Run tests"
	@echo "  make clean                       - Clean build artifacts"
	@echo "  make coverage                    - Generate test coverage report"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run linting tools"
	@echo "  make fmt                         - Format code"
	@echo "  make tidy                        - Clean dependencies"
	@echo ""
	@echo "$(BOLD)Example Applications:$(NC)"
	@echo "  make example-create-setup        - Setup the create example with environment file"
	@echo "  make example-create-build        - Build the create example application"
	@echo "  make example-create-run          - Run the create example application"
	@echo ""
	@echo "$(BOLD)API Management Commands:$(NC)"
	@echo "  make check-api-changes           - Check for API changes"
	@echo "  make update-api-hashes           - Update API hashes"
	@echo ""
	@echo "$(BOLD)Documentation Commands:$(NC)"
	@echo "  make godoc                       - Generate Go documentation"
	@echo "  make godoc-static                - Generate static Go documentation"
	@echo ""

#-------------------------------------------------------
# Test Commands
#-------------------------------------------------------

.PHONY: test
test:
	$(call title1,"Running tests")
	@$(GOTEST) -v ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Tests completed successfully$(GREEN) ✔️$(NC)"

.PHONY: coverage
coverage:
	$(call title1,"Generating test coverage")
	@$(GOTEST) -coverprofile=$(ARTIFACTS_DIR)/coverage.out ./...
	@$(GOTOOL) cover -html=$(ARTIFACTS_DIR)/coverage.out -o $(ARTIFACTS_DIR)/coverage.html
	@echo "$(GREEN)Coverage report generated at $(ARTIFACTS_DIR)/coverage.html$(NC)"
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Coverage report generated successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Code Quality Commands
#-------------------------------------------------------

.PHONY: lint
lint:
	$(call title1,"Running linters")
	@if find . -name "*.go" -type f | grep -q .; then \
		if ! command -v $(GOLINT) > /dev/null; then \
			echo "$(YELLOW)Installing golangci-lint...$(NC)"; \
			go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		fi; \
		$(GOLINT) run; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Linting completed successfully$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping linting$(NC)"; \
	fi

.PHONY: fmt
fmt:
	$(call title1,"Formatting code")
	@$(GOFMT) -s -w .
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Formatting completed successfully$(GREEN) ✔️$(NC)"

.PHONY: tidy
tidy:
	$(call title1,"Cleaning dependencies")
	@$(GOMOD) tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dependencies cleaned successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Clean Commands
#-------------------------------------------------------

.PHONY: clean
clean:
	$(call title1,"Cleaning build artifacts")
	@echo "$(CYAN)Cleaning build artifacts...$(NC)"
	@$(GOCLEAN)
	@rm -rf $(BIN_DIR)/ $(ARTIFACTS_DIR)/coverage.out $(ARTIFACTS_DIR)/coverage.html
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Artifacts cleaned successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Example Applications
#-------------------------------------------------------

.PHONY: example-create-setup
example-create-setup:
	$(call title1,"Setting up create example")
	@echo "$(CYAN)Copying environment file...$(NC)"
	@cp $(ENV_EXAMPLE) examples/create/.env
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Environment file copied to examples/create/.env$(GREEN) ✔️$(NC)"

.PHONY: example-create-build
example-create-build: example-create-setup
	$(call title1,"Building create example")
	@echo "$(CYAN)Building create example...$(NC)"
	@cd examples/create && $(GOBUILD) -o $(BIN_DIR)/create
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Create example built successfully at $(BIN_DIR)/create$(GREEN) ✔️$(NC)"

.PHONY: example-create-run
example-create-run: example-create-build
	$(call title1,"Running create example")
	@echo "$(CYAN)Running create example...$(NC)"
	@cd examples/create && $(BIN_DIR)/create
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Create example executed$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# API Management Commands
#-------------------------------------------------------

.PHONY: check-api-changes update-api-hashes
check-api-changes:
	$(call title1,"Checking for API changes")
	@echo "$(CYAN)Checking for API changes...$(NC)"
	@$(GO) run $(API_DETECTOR) -root $(ROOT_DIR)
	@echo "$(GREEN)$(BOLD)[ok]$(NC) API change check completed$(GREEN) ✔️$(NC)"

update-api-hashes:
	$(call title1,"Updating API hashes")
	@echo "$(CYAN)Updating API hashes...$(NC)"
	@$(GO) run $(API_DETECTOR) -root $(ROOT_DIR) -update
	@echo "$(GREEN)$(BOLD)[ok]$(NC) API hashes updated successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Documentation Commands
#-------------------------------------------------------

.PHONY: godoc
godoc:
	$(call title1,"Generating Go documentation")
	@echo "$(CYAN)Generating godoc documentation...$(NC)"
	@if ! command -v godoc > /dev/null; then \
		echo "$(YELLOW)Installing godoc...$(NC)"; \
		go install golang.org/x/tools/cmd/godoc@latest; \
	fi
	@echo "$(CYAN)Starting godoc server on http://localhost:6060/pkg/github.com/LerianStudio/midaz/sdks/go-sdk/$(NC)"
	@echo "$(CYAN)Press Ctrl+C to stop the server$(NC)"
	@godoc -http=:6060
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Documentation server stopped$(GREEN) ✔️$(NC)"

.PHONY: godoc-static
godoc-static:
	$(call title1,"Generating static Go documentation")
	@echo "$(CYAN)Generating static godoc documentation...$(NC)"
	@mkdir -p $(DOCS_DIR)
	@if ! command -v go > /dev/null; then \
		echo "$(RED)Error: go command not found$(NC)"; \
		exit 1; \
	fi
	@echo "$(CYAN)Generating documentation for all packages...$(NC)"
	@go doc -all github.com/LerianStudio/midaz/sdks/go-sdk > $(DOCS_DIR)/index.txt
	@for pkg in abstractions builders entities models; do \
		mkdir -p $(DOCS_DIR)/$$pkg; \
		go doc -all github.com/LerianStudio/midaz/sdks/go-sdk/$$pkg > $(DOCS_DIR)/$$pkg/index.txt; \
		echo "Generated documentation for $$pkg"; \
	done
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Static documentation generated at $(DOCS_DIR)/$(GREEN) ✔️$(NC)"
	@echo "$(CYAN)You can browse the documentation by opening the text files in your editor$(NC)"
