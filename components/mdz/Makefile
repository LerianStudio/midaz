# MDZ Component Makefile

# Component-specific variables
SERVICE_NAME := mdz-service
BIN_DIR := ./bin
ARTIFACTS_DIR := ./artifacts
NAME := mdz

# Ensure artifacts directory exists
$(shell mkdir -p $(ARTIFACTS_DIR))

# Define the root directory of the project
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)

# Include shared color definitions and utility functions
# Define local utility functions
define title1
	@echo ""
	@echo "-----------------------------"
	@echo "   📝 $(1)  "
	@echo "-----------------------------"
endef

# Check if Go is installed
GO := $(shell which go)
ifeq (, $(GO))
$(error "No go binary found in your system, please install go before continuing")
endif

# Load environment variables if .env exists
ifneq (,$(wildcard .env))
    include .env
endif

# MDZ-specific LDFLAGS
LDFLAGS=-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.ClientID=$(CLIENT_ID)'\
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.ClientSecret=$(CLIENT_SECRET)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.URLAPIAuth=$(URL_API_AUTH)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.URLAPILedger=$(URL_API_LEDGER)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.Version=$(VERSION)'

#-------------------------------------------------------
# Core Commands
#-------------------------------------------------------

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)MDZ Service Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Core Commands:$(NC)"
	@echo "  make help                        - Display this help message"
	@echo "  make build                       - Build the MDZ binary"
	@echo "  make test                        - Run tests"
	@echo "  make clean                       - Clean build artifacts"
	@echo "  make cover                       - Generate test coverage report"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint                        - Run linting tools"
	@echo "  make format                      - Format code"
	@echo "  make tidy                        - Clean dependencies"
	@echo "  make govulncheck                 - Check for vulnerabilities"
	@echo "  make gosec                       - Run security analysis"
	@echo "  make perfsprint                  - Run performance checks"
	@echo ""
	@echo "$(BOLD)MDZ-Specific Commands:$(NC)"
	@echo "  make test_integration            - Run integration tests"
	@echo "  make install-local               - Install MDZ binary to /usr/local/bin"
	@echo ""
	@echo "$(BOLD)Developer Helper Commands:$(NC)"
	@echo "  make dev-setup                   - Set up development environment"
	@echo ""
	@echo "$(BOLD)Code Quality Helper Commands:$(NC)"
	@echo "  make get-govulncheck-deps        - Install govulncheck"
	@echo "  make get-gosec-deps              - Install gosec"
	@echo "  make get-perfsprint-deps         - Install perfsprint"

#-------------------------------------------------------
# Build Commands
#-------------------------------------------------------

.PHONY: build
build:
	$(call title1,"Building MDZ binary")
	@go version
	@go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(NAME) ./main.go
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Binary built successfully at $(BIN_DIR)/$(NAME)$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Test Commands
#-------------------------------------------------------

.PHONY: test
test:
	$(call title1,"Running unit tests")
	@go test ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Tests completed successfully$(GREEN) ✔️$(NC)"

.PHONY: test_integration
test_integration:
	$(call title1,"Running integration tests")
	@go test -v -tags=integration ./test/integration/...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Integration tests completed successfully$(GREEN) ✔️$(NC)"

.PHONY: cover
cover:
	$(call title1,"Generating test coverage")
	@go test -coverprofile=$(ARTIFACTS_DIR)/coverage.out ./...
	@go tool cover -html=$(ARTIFACTS_DIR)/coverage.out -o $(ARTIFACTS_DIR)/coverage.html
	@echo "$(GREEN)Coverage report generated at $(ARTIFACTS_DIR)/coverage.html$(NC)"
	@echo ""
	@echo "$(CYAN)Coverage Summary:$(NC)"
	@echo "$(CYAN)----------------------------------------$(NC)"
	@go tool cover -func=$(ARTIFACTS_DIR)/coverage.out | grep total | awk '{print "Total coverage: " $$3}'
	@echo "$(CYAN)----------------------------------------$(NC)"
	@echo "$(YELLOW)Open $(ARTIFACTS_DIR)/coverage.html in your browser to view detailed coverage report$(NC)"

#-------------------------------------------------------
# Security Commands
#-------------------------------------------------------

.PHONY: sec
sec:
	$(call title1,"Running security checks using gosec")
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing gosec...$(NC)"; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@if find . -name "*.go" -type f | grep -q .; then \
		echo "$(CYAN)Running security checks...$(NC)"; \
		gosec ./...; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Security checks completed$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping security checks$(NC)"; \
	fi

#-------------------------------------------------------
# Test Coverage Commands
#-------------------------------------------------------

.PHONY: check-tests
check-tests:
	$(call title1,"Verifying test coverage")
	@if find . -name "*.go" -type f | grep -q .; then \
		echo "$(CYAN)Running test coverage check...$(NC)"; \
		go test -coverprofile=coverage.tmp ./... > /dev/null 2>&1; \
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
# Code Quality Commands
#-------------------------------------------------------

.PHONY: lint
lint:
	$(call title1,"Running linters")
	@if find . -name "*.go" -type f | grep -q .; then \
		if ! command -v golangci-lint >/dev/null 2>&1; then \
			echo "$(YELLOW)golangci-lint not found, installing...$(NC)"; \
			@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		else \
			echo "$(GREEN)golangci-lint already installed ✔️$(NC)"; \
		fi; \
		golangci-lint run --fix ./... --verbose; \
		echo "$(GREEN)$(BOLD)[ok]$(NC) Linting completed successfully$(GREEN) ✔️$(NC)"; \
	else \
		echo "$(YELLOW)No Go files found, skipping linting$(NC)"; \
	fi

.PHONY: format
format:
	$(call title1,"Formatting code")
	@go fmt ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Formatting completed successfully$(GREEN) ✔️$(NC)"

.PHONY: tidy
tidy:
	$(call title1,"Cleaning dependencies")
	@go mod tidy
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Dependencies cleaned successfully$(GREEN) ✔️$(NC)"

.PHONY: get-govulncheck-deps
get-govulncheck-deps:
	@echo "$(BLUE)Installing govulncheck...$(NC)"
	@go install golang.org/x/vuln/cmd/govulncheck@latest

.PHONY: govulncheck
govulncheck: get-govulncheck-deps
	$(call title1,"Checking for vulnerabilities")
	@govulncheck ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Vulnerability check completed$(GREEN) ✔️$(NC)"

.PHONY: get-gosec-deps
get-gosec-deps:
	@echo "$(BLUE)Installing gosec...$(NC)"
	@go install github.com/securego/gosec/v2/cmd/gosec@latest

.PHONY: gosec
gosec: get-gosec-deps
	$(call title1,"Running security analysis")
	@gosec ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Security analysis completed$(GREEN) ✔️$(NC)"

.PHONY: get-perfsprint-deps
get-perfsprint-deps:
	@echo "$(BLUE)Installing perfsprint...$(NC)"
	@go get github.com/catenacyber/perfsprint@latest

.PHONY: perfsprint
perfsprint: get-perfsprint-deps
	$(call title1,"Running performance checks")
	@perfsprint ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Performance checks completed$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Clean Commands
#-------------------------------------------------------

.PHONY: clean
clean:
	$(call title1,"Cleaning build artifacts")
	@rm -rf $(BIN_DIR)/* $(ARTIFACTS_DIR)/*
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Artifacts cleaned successfully$(GREEN) ✔️$(NC)"

#-------------------------------------------------------
# Developer Helper Commands
#-------------------------------------------------------

.PHONY: dev-setup
dev-setup:
	$(call title1,"Setting up development environment")
	@echo "$(CYAN)Installing development tools...$(NC)"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing golangci-lint...$(NC)"; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@if ! command -v mockgen >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing mockgen...$(NC)"; \
		go install github.com/golang/mock/mockgen@latest; \
	fi
	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing gosec...$(NC)"; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi
	@echo "$(CYAN)Setting up environment...$(NC)"
	@if [ -f .env.example ] && [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "$(GREEN)Created .env file from template$(NC)"; \
	fi
	@make tidy
	@make check-tests
	@make sec
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Development environment set up successfully$(GREEN) ✔️$(NC)"
	@echo "$(CYAN)You're ready to start developing! Here are some useful commands:$(NC)"
	@echo "  make build         - Build the MDZ binary"
	@echo "  make test          - Run tests"
	@echo "  make install-local - Install MDZ binary to /usr/local/bin"

#-------------------------------------------------------
# MDZ-Specific Commands
#-------------------------------------------------------

.PHONY: install-local
install-local: build
	$(call title1,"Installing MDZ binary to /usr/local/bin")
	@sudo cp -r $(BIN_DIR)/mdz /usr/local/bin
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Binary installed successfully to /usr/local/bin/mdz$(GREEN) ✔️$(NC)"

.PHONY: logs
logs:
	$(call title1,"Showing logs for all services")
	@if [ -f "docker-compose.yml" ]; then \
		echo "$(CYAN)Logs for component: $(BOLD)mdz$(NC)"; \
		docker compose -f docker-compose.yml logs --tail=100 -f $(c) 2>/dev/null || docker-compose -f docker-compose.yml logs --tail=100 -f $(c); \
	else \
		echo "$(YELLOW)No docker-compose.yml file found. Skipping logs command.$(NC)"; \
	fi

.PHONY: down
down:
	$(call title1,"Stopping and removing containers, networks, and volumes")
	@if [ -f "docker-compose.yml" ]; then \
		docker compose -f docker-compose.yml down 2>/dev/null || docker-compose -f docker-compose.yml down; \
	else \
		echo "$(YELLOW)No docker-compose.yml file found. Skipping down command.$(NC)"; \
	fi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Services stopped successfully$(GREEN) ✔️$(NC)"
