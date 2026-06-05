# ------------------------------------------------------
# API documentation verification commands
# ------------------------------------------------------
# API documentation/OpenAPI generation is single-sourced at the repo root
# (`make generate-docs` -> postman/generator/generate-docs.sh, which regenerates
# ledger, tracer and reporter-manager specs and refreshes the Postman hub).
#
# This fragment keeps only the per-component annotation-coverage check, used by
# tracer's pre-merge gate. It depends on a component-local ./scripts/verify-api-docs.sh.
#
# Usage:
#   make verify-api-docs             # Check documentation coverage
# ------------------------------------------------------

# Verify API documentation coverage
# Checks if all API endpoints have proper Swagger annotations
# Uses custom verification script
.PHONY: verify-api-docs
verify-api-docs:
	$(call title1,"Verifying API documentation coverage")
	@sh ./scripts/verify-api-docs.sh
	@echo "$(GREEN)$(BOLD)[ok]$(NC) API documentation verification completed$(GREEN) ✔️$(NC)"
