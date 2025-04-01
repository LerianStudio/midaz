#!/bin/bash

# Script to verify that all API endpoints are documented in the OpenAPI specification
# This helps ensure that the Postman collection will include all endpoints

# Include color definitions
source "$(dirname "$0")/../pkg/shell/colors.sh"

# Define paths
MIDAZ_ROOT=$(pwd)
ONBOARDING_ROUTES="${MIDAZ_ROOT}/components/onboarding/internal/adapters/http/in/routes.go"
TRANSACTION_ROUTES="${MIDAZ_ROOT}/components/transaction/internal/adapters/http/in/routes.go"
ONBOARDING_SWAGGER="${MIDAZ_ROOT}/components/onboarding/api/swagger.json"
TRANSACTION_SWAGGER="${MIDAZ_ROOT}/components/transaction/api/swagger.json"

# Function to extract routes from Go code
extract_routes() {
    grep -E 'f\.(Get|Post|Put|Patch|Delete)' "$1" | 
    sed -E 's/.*f\.(Get|Post|Put|Patch|Delete)\("([^"]+)".*/\1 \2/g' |
    grep -v "/health\|/version\|/swagger" |
    sed 's/:organization_id/{organization_id}/g' |
    sed 's/:ledger_id/{ledger_id}/g' |
    sed 's/:account_id/{account_id}/g' |
    sed 's/:transaction_id/{transaction_id}/g' |
    sed 's/:operation_id/{operation_id}/g' |
    sed 's/:balance_id/{balance_id}/g' |
    sed 's/:external_id/{external_id}/g' |
    sed 's/:asset_code/{asset_code}/g' |
    sed 's/:id/{id}/g' |
    sed 's/:alias/{alias}/g' |
    sort
}

# Function to extract paths from Swagger JSON
extract_swagger_paths() {
    jq -r '.paths | keys[]' "$1" | sort
}

# Function to check if a route is documented
check_route_documented() {
    local route="$1"
    local method="$2"
    local swagger_file="$3"
    
    # Convert method to lowercase for JSON comparison
    local method_lower=$(echo "$method" | tr '[:upper:]' '[:lower:]')
    
    # Check if the route exists in the Swagger file
    if jq -e ".paths[\"$route\"] | has(\"$method_lower\")" "$swagger_file" > /dev/null; then
        return 0 # Route is documented
    else
        return 1 # Route is not documented
    fi
}

# Main function to verify API documentation
verify_api_docs() {
    local component="$1"
    local routes_file="$2"
    local swagger_file="$3"
    
    echo "${CYAN}Verifying API documentation for ${component} component...${NC}"
    
    # Check if files exist
    if [ ! -f "$routes_file" ]; then
        echo "${RED}Routes file not found: $routes_file${NC}"
        return 1
    fi
    
    if [ ! -f "$swagger_file" ]; then
        echo "${RED}Swagger file not found: $swagger_file${NC}"
        return 1
    fi
    
    # Extract routes from Go code
    local routes=$(extract_routes "$routes_file")
    local missing_routes=0
    
    # Check each route
    while IFS= read -r line; do
        if [ -n "$line" ]; then
            method=$(echo "$line" | cut -d' ' -f1)
            route=$(echo "$line" | cut -d' ' -f2-)
            
            if ! check_route_documented "$route" "$method" "$swagger_file"; then
                echo "${YELLOW}Missing documentation for: $method $route${NC}"
                missing_routes=$((missing_routes + 1))
                
                # Suggest how to document the route
                echo "${MAGENTA}Suggestion: Add @Router $route [$method] annotation to the handler function${NC}"
            fi
        fi
    done <<< "$routes"
    
    if [ $missing_routes -eq 0 ]; then
        echo "${GREEN}All routes in $component are properly documented!${NC}"
        return 0
    else
        echo "${YELLOW}Found $missing_routes undocumented routes in $component component.${NC}"
        echo "${YELLOW}Please add proper annotations to ensure all endpoints are documented.${NC}"
        return 1
    fi
}

# Main execution
echo "${CYAN}----------------------------------------------${NC}"
echo "${CYAN}   Verifying API Documentation Coverage   ${NC}"
echo "${CYAN}----------------------------------------------${NC}"

# Verify onboarding component
verify_api_docs "onboarding" "$ONBOARDING_ROUTES" "$ONBOARDING_SWAGGER"
onboarding_status=$?

# Verify transaction component
verify_api_docs "transaction" "$TRANSACTION_ROUTES" "$TRANSACTION_SWAGGER"
transaction_status=$?

# Summary
echo "${CYAN}----------------------------------------------${NC}"
if [ $onboarding_status -eq 0 ] && [ $transaction_status -eq 0 ]; then
    echo "${GREEN}${BOLD}[ok]${NC} All API endpoints are properly documented!${GREEN} ✔️${NC}"
else
    echo "${YELLOW}Some API endpoints are not properly documented.${NC}"
    echo "${YELLOW}Please add the missing documentation to ensure complete Postman collection sync.${NC}"
    echo ""
    echo "${CYAN}How to document an endpoint:${NC}"
    echo "1. Find the handler function for the undocumented endpoint"
    echo "2. Add proper annotations before the function, for example:"
    echo ""
    echo "   // @Summary Create a transaction using JSON"
    echo "   // @Description Create a Transaction with the input JSON payload"
    echo "   // @Tags Transactions"
    echo "   // @Accept json"
    echo "   // @Produce json"
    echo "   // @Param Authorization header string true \"Authorization Bearer Token\""
    echo "   // @Param X-Request-Id header string false \"Request ID\""
    echo "   // @Param organization_id path string true \"Organization ID\""
    echo "   // @Param ledger_id path string true \"Ledger ID\""
    echo "   // @Param transaction body transaction.CreateTransactionInput true \"Transaction data\""
    echo "   // @Success 201 {object} mmodel.Transaction"
    echo "   // @Failure 400 {object} mmodel.Error"
    echo "   // @Failure 401 {object} mmodel.Error"
    echo "   // @Failure 500 {object} mmodel.Error"
    echo "   // @Router /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json [post]"
    echo ""
    echo "3. Run 'make generate-docs-all' to update the documentation"
fi
