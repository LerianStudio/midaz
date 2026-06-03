# ------------------------------------------------------
# API documentation generation and validation commands
# ------------------------------------------------------
# This file contains all documentation-related operations including
# Swagger/OpenAPI generation, verification, and validation.
#
# Requirements:
#   - swag CLI (auto-installed if missing)
#   - Docker (for OpenAPI generator)
#   - ./scripts/verify-api-docs.sh script
#   - API annotations in Go code (@Summary, @Description, etc.)
#
# Variables used from main Makefile:
#   (none - runs from project root)
#
# Generated files:
#   - ./api/swagger.json - Swagger specification
#   - ./api/swagger.yaml - Swagger YAML format
#   - ./api/openapi.yaml - OpenAPI 3.0 specification
#
# Usage:
#   make generate-docs               # Generate Swagger documentation
#   make generate-docs-all           # Generate with coverage verification
#   make verify-api-docs             # Check documentation coverage
#   make validate-api-docs           # Validate OpenAPI spec
#
# Tools:
#   - swag: Swagger documentation generator for Go
#   - openapi-generator-cli: OpenAPI format converter/validator
# ------------------------------------------------------

#-------------------------------------------------------
# Commands (alphabetically ordered)
#-------------------------------------------------------

# Generate Swagger API documentation
# Scans Go code annotations and generates swagger.json and openapi.yaml
# Uses Docker to convert Swagger to OpenAPI 3.0 format
.PHONY: generate-docs
generate-docs:
	$(call title1,"Generating Swagger API documentation")
	@if ! command -v swag >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing swag...$(NC)"; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
	fi
	@swag init -g cmd/app/main.go -o api --parseDependency --parseInternal
	@docker run --rm -v ./:/local --user $(shell id -u):$(shell id -g) openapitools/openapi-generator-cli:v7.10.0 generate -i /local/api/swagger.json -g openapi-yaml -o /local/api
	@mv ./api/openapi/openapi.yaml ./api/openapi.yaml
	@rm -rf ./api/README.md ./api/.openapi-generator* ./api/openapi
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Swagger API documentation generated successfully$(GREEN) ✔️$(NC)"

# Generate Swagger documentation with coverage verification
# Aggregator command that verifies documentation coverage before generating
# Suppresses warning messages in output
.PHONY: generate-docs-all
generate-docs-all:
	$(call title1,"Generating Swagger documentation for all services")
	$(call check_command,swag,"go install github.com/swaggo/swag/cmd/swag@latest")
	@echo "$(CYAN)Verifying API documentation coverage...$(NC)"
	@sh ./scripts/verify-api-docs.sh || echo "$(YELLOW)Warning: Some API endpoints may not be properly documented. Continuing with documentation generation...$(NC)"
	@echo "$(CYAN)Generating documentation for plugin component...$(NC)"
	$(MAKE) generate-docs 2>&1 | grep -v "warning: "
	@echo "$(GREEN)$(BOLD)[ok]$(NC) Swagger documentation generated successfully$(GREEN) ✔️$(NC)"

# Validate generated OpenAPI specification
# Runs OpenAPI validator against generated swagger.json
# Requires: generate-docs to be run first (dependency)
.PHONY: validate-api-docs
validate-api-docs: generate-docs
	$(call title1,"Validating API documentation")
	@docker run --rm -v ./:/local openapitools/openapi-generator-cli:v7.10.0 validate -i /local/api/swagger.json
	@echo "$(GREEN)$(BOLD)[ok]$(NC) API documentation validation completed$(GREEN) ✔️$(NC)"

# Verify API documentation coverage
# Checks if all API endpoints have proper Swagger annotations
# Uses custom verification script
.PHONY: verify-api-docs
verify-api-docs:
	$(call title1,"Verifying API documentation coverage")
	@sh ./scripts/verify-api-docs.sh
	@echo "$(GREEN)$(BOLD)[ok]$(NC) API documentation verification completed$(GREEN) ✔️$(NC)"
