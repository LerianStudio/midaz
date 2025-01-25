# Service configuration
service_name := mdz
GO := $(shell which go)

# Use tput if available, fallback to ANSI codes if not
BLUE := $(shell command -v tput >/dev/null 2>&1 && tput setaf 4 || echo '\033[0;34m')
NC := $(shell command -v tput >/dev/null 2>&1 && tput sgr0 || echo '\033[0m')
BOLD := $(shell command -v tput >/dev/null 2>&1 && tput bold || echo '\033[1m')
RED := $(shell command -v tput >/dev/null 2>&1 && tput setaf 1 || echo '\033[0;31m')
MAGENTA := $(shell command -v tput >/dev/null 2>&1 && tput setaf 5 || echo '\033[0;35m')

ifeq (, $(GO))
$(error "$(RED)No go binary found in your system, please install go version go1.23.2 before continuing$(NC)")
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
	@echo "$(BOLD)MDZ CLI Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Development Commands:$(NC)"
	@echo "- make build                        - Build the MDZ CLI"
	@echo "- make install-local                - Install MDZ CLI locally"
	@echo "- make test                         - Run tests"
	@echo "- make test_integration             - Run integration tests"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "- make lint                         - Run golangci-lint"
	@echo "- make govulncheck                  - Run vulnerability checks"
	@echo "- make gosec                        - Run security checks"
	@echo "- make perfsprint                   - Run performance checks"
	@echo ""

# Development Commands
.PHONY: build
build:
	@echo "$(BLUE)Building MDZ CLI...$(NC)"
	@go version
	@go build -ldflags "$(LDFLAGS)" -o ./bin/$(service_name) ./main.go

.PHONY: install-local
install-local: build
	@echo "$(BLUE)Installing MDZ CLI locally...$(NC)"
	@sudo cp -r bin/mdz /usr/local/bin

.PHONY: test
test:
	@echo "$(BLUE)Running tests...$(NC)"
	@go test ./...

.PHONY: test_integration
test_integration:
	@echo "$(BLUE)Running integration tests...$(NC)"
	@go test -v -tags=integration ./test/integration/...

# Code Quality Commands
.PHONY: get-lint-deps
get-lint-deps:
	@echo "$(BLUE)Installing golangci-lint...$(NC)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: lint
lint: get-lint-deps
	@echo "$(BLUE)Running golangci-lint...$(NC)"
	@golangci-lint run --fix ./... --verbose

.PHONY: get-govulncheck-deps
get-govulncheck-deps:
	@echo "$(BLUE)Installing govulncheck...$(NC)"
	@go install golang.org/x/vuln/cmd/govulncheck@latest

.PHONY: govulncheck
govulncheck: get-govulncheck-deps
	@echo "$(BLUE)Running vulnerability checks...$(NC)"
	@govulncheck ./...

.PHONY: get-gosec-deps
get-gosec-deps:
	@echo "$(BLUE)Installing gosec...$(NC)"
	@go install github.com/securego/gosec/v2/cmd/gosec@latest

.PHONY: gosec
gosec: get-gosec-deps
	@echo "$(BLUE)Running security checks...$(NC)"
	@gosec ./...

.PHONY: get-perfsprint-deps
get-perfsprint-deps:
	@echo "$(BLUE)Installing perfsprint...$(NC)"
	@go get github.com/catenacyber/perfsprint@latest

.PHONY: perfsprint
perfsprint: get-perfsprint-deps
	@echo "$(BLUE)Running performance checks...$(NC)"
	@perfsprint ./...
