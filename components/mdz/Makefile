GO := $(shell which go)
NAME := mdz

# Colors for terminal output
BLUE := \033[36m
NC := \033[0m
BOLD := \033[1m
RED := \033[31m
MAGENTA := \033[35m
YELLOW := \033[33m
GREEN := \033[32m
CYAN := \033[36m
WHITE := \033[37m

ifeq (, $(GO))
$(error "No go binary found in your system, please install go version go1.23.2 before continuing")
endif

ifneq (,$(wildcard .env))
    include .env
endif

LDFLAGS=-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.ClientID=$(CLIENT_ID)'\
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.ClientSecret=$(CLIENT_SECRET)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.URLAPIAuth=$(URL_API_AUTH)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.URLAPIOnboarding=$(URL_API_ONBOARDING)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.URLAPITransaction=$(URL_API_TRANSACTION)' \
	-X 'github.com/LerianStudio/midaz/components/mdz/pkg/environment.Version=$(VERSION)'

.PHONY: help
help:
	@echo ""
	@echo "$(BOLD)MDZ CLI Management Commands$(NC)"
	@echo ""
	@echo "$(BOLD)Build Commands:$(NC)"
	@echo "  make build              - Build MDZ CLI binary"
	@echo "  make install-local      - Build and install MDZ CLI locally"
	@echo ""
	@echo "$(BOLD)Testing Commands:$(NC)"
	@echo "  make test               - Run unit tests"
	@echo "  make test_integration   - Run integration tests"
	@echo ""
	@echo "$(BOLD)Code Quality Commands:$(NC)"
	@echo "  make lint               - Run golangci-lint and fix issues"
	@echo "  make govulncheck        - Check for vulnerabilities using govulncheck"
	@echo "  make gosec              - Run security checks using gosec"
	@echo "  make perfsprint         - Run performance analysis"
	@echo ""
	@echo "$(BOLD)Dependency Commands:$(NC)"
	@echo "  make get-lint-deps      - Install golangci-lint tool"
	@echo "  make get-gosec-deps     - Install gosec tool"
	@echo "  make get-govulncheck-deps - Install govulncheck tool"
	@echo "  make get-perfsprint-deps  - Install perfsprint tool"
	@echo ""

.PHONY: get-lint-deps
get-lint-deps:
	@echo "$(BLUE)Installing golangci-lint...$(NC)"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: lint
lint: get-lint-deps
	@echo "$(BLUE)Running linter...$(NC)"
	golangci-lint run --fix ./... --verbose

.PHONY: get-govulncheck-deps
get-govulncheck-deps:
	@echo "$(BLUE)Installing govulncheck...$(NC)"
	go install golang.org/x/vuln/cmd/govulncheck@latest

.PHONY: govulncheck
govulncheck: get-govulncheck-deps
	@echo "$(BLUE)Checking for vulnerabilities...$(NC)"
	govulncheck ./...

.PHONY: get-gosec-deps
get-gosec-deps:
	@echo "$(BLUE)Installing gosec...$(NC)"
	go install github.com/securego/gosec/v2/cmd/gosec@latest

.PHONY: gosec
gosec: get-gosec-deps
	@echo "$(BLUE)Running security checks...$(NC)"
	gosec ./...

.PHONY: get-perfsprint-deps
get-perfsprint-deps:
	@echo "$(BLUE)Installing perfsprint...$(NC)"
	go get github.com/catenacyber/perfsprint@latest

.PHONY: perfsprint
perfsprint: get-perfsprint-deps
	@echo "$(BLUE)Running performance analysis...$(NC)"
	perfsprint ./...

.PHONY: test
test:
	@echo "$(BLUE)Running unit tests...$(NC)"
	go test ./...

.PHONY: test_integration
test_integration:
	@echo "$(BLUE)Running integration tests...$(NC)"
	go test -v -tags=integration ./test/integration/...

.PHONY: build
build:
	@echo "$(BLUE)Building MDZ CLI binary...$(NC)"
	go version
	go build -ldflags "$(LDFLAGS)" -o ./bin/$(NAME) ./main.go

.PHONY: install-local
install-local: build
	@echo "$(BLUE)Installing MDZ CLI locally...$(NC)"
	sudo cp -r bin/mdz /usr/local/bin
	@echo "$(GREEN)MDZ CLI installed successfully at /usr/local/bin/mdz$(NC)"

.PHONY: cover
cover:
	@echo "$(BLUE)Generating test coverage...$(NC)"
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated at coverage.html$(NC)"

.PHONY: tidy
tidy:
	@echo "$(BLUE)Running go mod tidy...$(NC)"
	go mod tidy

.PHONY: format
format:
	@echo "$(BLUE)Formatting Go code...$(NC)"
	go fmt ./...