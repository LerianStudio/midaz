SERVICE_NAME := mdz-service
BIN_DIR := ./bin
ARTIFACTS_DIR := ./artifacts
NAME := mdz

$(shell mkdir -p $(ARTIFACTS_DIR))

# Define the root directory of the project (if not already defined)
MIDAZ_ROOT ?= $(shell cd ../.. && pwd)

# Include shared color definitions and utility functions
include $(MIDAZ_ROOT)/pkg/shell/makefile_colors.mk
include $(MIDAZ_ROOT)/pkg/shell/makefile_utils.mk

GO := $(shell which go)

ifeq (, $(GO))
$(error "No go binary found in your system, please install go version go1.23.2 before continuing")
endif

ifneq (,$(wildcard .env))
    include .env
endif

LDFLAGS=-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.ClientID=$(CLIENT_ID)'\
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.ClientSecret=$(CLIENT_SECRET)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.URLAPIAuth=$(URL_API_AUTH)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.URLAPILedger=$(URL_API_LEDGER)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.Version=$(VERSION)'

# Display available commands
.PHONY: info
info:
	@echo ""
	@echo "$(BOLD)MDZ Service Commands$(NC)"
	@echo ""
	@echo "To run a specific command inside the mdz container using make, you can execute:"
	@echo ""
	@echo "$(CYAN)make mdz COMMAND=\"any\"$(NC)"
	@echo ""
	@echo "This command will run the specified command inside the mdz container."
	@echo "Replace \"any\" with the desired command you want to execute."
	@echo ""
	@echo "$(BOLD)## Linting and Security commands:$(NC)"
	@echo ""
	@echo "  $(CYAN)COMMAND=\"lint\"$(NC)                                 Run code linting"
	@echo "  $(CYAN)COMMAND=\"govulncheck\"$(NC)                          Check for vulnerabilities"
	@echo "  $(CYAN)COMMAND=\"gosec\"$(NC)                                Run security analysis"
	@echo "  $(CYAN)COMMAND=\"perfsprint\"$(NC)                           Run performance checks"
	@echo ""
	@echo "$(BOLD)## Testing commands:$(NC)"
	@echo ""
	@echo "  $(CYAN)COMMAND=\"test\"$(NC)                                 Run unit tests"
	@echo "  $(CYAN)COMMAND=\"test_integration\"$(NC)                     Run integration tests"
	@echo ""
	@echo "$(BOLD)## Build commands:$(NC)"
	@echo ""
	@echo "  $(CYAN)COMMAND=\"build\"$(NC)                                Build mdz binary"
	@echo "  $(CYAN)COMMAND=\"install-local\"$(NC)                        Install mdz to /usr/local/bin"
	@echo ""

# Linting and Security Commands
.PHONY: get-lint-deps
get-lint-deps:
	@echo "$(BLUE)Installing golangci-lint...$(NC)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: lint
lint: get-lint-deps
	$(call title1,"Running code linting")
	@golangci-lint run --fix ./... --verbose
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Linting completed successfully$(GREEN) ✔️$(NC)"

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

# Testing Commands
.PHONY: test
test: 
	@echo "$(BLUE)Running unit tests...$(NC)"
	@go test ./...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Tests completed successfully$(GREEN) ✔️$(NC)"

.PHONY: test_integration
test_integration:
	$(call title1,"Running integration tests")
	@go test -v -tags=integration ./test/integration/...
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Integration tests completed successfully$(GREEN) ✔️$(NC)"

# Build Commands
.PHONY: build
build:
	$(call title1,"Building mdz binary")
	@go version
	@go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(NAME) ./main.go
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Binary built successfully at $(BIN_DIR)/$(NAME)$(GREEN) ✔️$(NC)"

.PHONY: install-local
install-local: build
	@echo "$(BLUE)Installing mdz binary to /usr/local/bin...$(NC)"
	@sudo cp -r $(BIN_DIR)/mdz /usr/local/bin
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Binary installed successfully to /usr/local/bin/mdz$(GREEN) ✔️$(NC)"
