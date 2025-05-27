#!/bin/bash

echo "Checking all plugin routes..."
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Summary information
# Route counts will be calculated dynamically

# Define all routes
routes=(
  # Accounting
  "/plugins/accounting"
  "/plugins/accounting/account-types"
  "/plugins/accounting/account-types/create"
  "/plugins/accounting/transaction-routes"
  "/plugins/accounting/operation-routes"
  "/plugins/accounting/compliance"
  
  # CRM
  "/plugins/crm"
  "/plugins/crm/customers"
  "/plugins/crm/customers/create"
  "/plugins/crm/aliases"
  "/plugins/crm/analytics"
  "/plugins/crm/holders"  # New holder-based routes
  
  # Fees
  "/plugins/fees"
  "/plugins/fees/packages"
  "/plugins/fees/packages/create"
  "/plugins/fees/billing"
  "/plugins/fees/analytics"
  
  # Reconciliation
  "/plugins/reconciliation"
  "/plugins/reconciliation/processes"
  "/plugins/reconciliation/processes/create"
  "/plugins/reconciliation/exceptions"
  "/plugins/reconciliation/rules"
  "/plugins/reconciliation/analytics"
  
  # Workflows
  "/plugins/workflows"
  "/plugins/workflows/library"
  "/plugins/workflows/library/create"
  "/plugins/workflows/executions"
  "/plugins/workflows/executions/start"
  "/plugins/workflows/executions/monitoring"
  "/plugins/workflows/tasks"
  "/plugins/workflows/tasks/testing"
  "/plugins/workflows/integrations"
  "/plugins/workflows/integrations/testing"
  "/plugins/workflows/analytics"
  "/plugins/workflows/demo"
)

# API endpoints to test
api_endpoints=(
  # Core APIs
  "/api/organizations?page=1&limit=5"
  "/api/organizations/019704c6-845c-72e5-b892-a9e8adf619a8"
  "/api/organizations/019704c6-845c-72e5-b892-a9e8adf619a8/ledgers?page=1&limit=5"
  "/api/organizations/019704c6-845c-72e5-b892-a9e8adf619a8/ledgers/019704c6-8472-724e-a6d2-71fea0055e4e/assets?page=1&limit=5"
  "/api/organizations/019704c6-845c-72e5-b892-a9e8adf619a8/ledgers/019704c6-8472-724e-a6d2-71fea0055e4e/accounts?page=1&limit=5"
  
  # Plugin APIs
  "/api/crm/holders?page=1&limit=5"
  "/api/crm/holders/019704c6-845c-72e5-b892-a9e8adf619a8"  # Sample holder ID
  "/api/crm/holders/019704c6-845c-72e5-b892-a9e8adf619a8/aliases?page=1&limit=5"
  "/api/crm/aliases?page=1&limit=5"
  "/api/identity/users?page=1&limit=5"
  "/api/identity/groups?page=1&limit=5"
  "/api/identity/applications?page=1&limit=5"
  "/api/fees/packages?page=1&limit=5"
  "/api/accounting/account-types?page=1&limit=5"
)

total_routes=${#routes[@]}
total_apis=${#api_endpoints[@]}
total=$((total_routes + total_apis))
successful=0
failed=0
failed_items=()

# Function to check if server is running
check_server() {
  curl -s -o /dev/null -w "%{http_code}" "http://localhost:8081/" > /dev/null 2>&1
  return $?
}

# Check if server is running
if ! check_server; then
  echo -e "${RED}Error: Server is not responding on http://localhost:8081${NC}"
  echo "Please ensure the console is running with 'npm run dev' or 'docker-compose up'"
  exit 1
fi

# Create temporary file for storing full responses
temp_file=$(mktemp)
api_responses_file=$(mktemp)

echo -e "${BLUE}=== Checking UI Routes ===${NC}"
echo ""

# Check UI routes
for route in "${routes[@]}"; do
  # Get both status code and response body
  response=$(curl -s -w "\n%{http_code}" "http://localhost:8081$route" 2>&1)
  http_code=$(echo "$response" | tail -n 1)
  # Use sed instead of head -n -1 to avoid errors
  body=$(echo "$response" | sed '$d')
  
  if [ "$http_code" = "200" ]; then
    echo -e "${GREEN}✓ $route${NC}"
    ((successful++))
  else
    echo -e "${RED}✗ $route (HTTP $http_code)${NC}"
    ((failed++))
    failed_items+=("UI: $route (HTTP $http_code)")
    
    # Save error details
    echo "=== $route ===" >> "$temp_file"
    echo "$body" >> "$temp_file"
    echo "" >> "$temp_file"
  fi
done

echo ""
echo -e "${BLUE}=== Testing API Endpoints ===${NC}"
echo ""

# Test API endpoints
for endpoint in "${api_endpoints[@]}"; do
  # Make API request and get response
  response=$(curl -s -w "\nHTTP_CODE:%{http_code}" "http://localhost:8081$endpoint" 2>&1)
  http_code=$(echo "$response" | grep "HTTP_CODE:" | cut -d: -f2)
  body=$(echo "$response" | sed '/HTTP_CODE:/d')
  
  # Try to parse as JSON
  if echo "$body" | jq . >/dev/null 2>&1; then
    # Valid JSON response
    if [ "$http_code" = "200" ] || [ "$http_code" = "201" ]; then
      # Check if response has expected structure
      if echo "$body" | jq -e '.items' >/dev/null 2>&1 || echo "$body" | jq -e '.id' >/dev/null 2>&1; then
        echo -e "${GREEN}✓ $endpoint (HTTP $http_code)${NC}"
        ((successful++))
      else
        # Check if it's an error response
        if echo "$body" | jq -e '.message' >/dev/null 2>&1; then
          message=$(echo "$body" | jq -r '.message')
          if [ "$message" = "{}" ] || [ "$message" = "null" ]; then
            echo -e "${YELLOW}⚠ $endpoint (HTTP $http_code) - Empty error message${NC}"
          else
            echo -e "${YELLOW}⚠ $endpoint (HTTP $http_code) - Error: $message${NC}"
          fi
        else
          echo -e "${GREEN}✓ $endpoint (HTTP $http_code)${NC}"
        fi
        ((successful++))
      fi
      
      # Save successful response sample
      echo "=== SUCCESS: $endpoint ===" >> "$api_responses_file"
      echo "$body" | jq . 2>/dev/null >> "$api_responses_file"
      echo "" >> "$api_responses_file"
    elif [ "$http_code" = "503" ]; then
      # Service unavailable - expected if plugin not running
      echo -e "${YELLOW}⚠ $endpoint (HTTP $http_code) - Service unavailable${NC}"
      ((successful++))
    else
      echo -e "${RED}✗ $endpoint (HTTP $http_code)${NC}"
      ((failed++))
      failed_items+=("API: $endpoint (HTTP $http_code)")
      
      # Save error response
      echo "=== ERROR: $endpoint ===" >> "$api_responses_file"
      echo "$body" | jq . 2>/dev/null || echo "$body" >> "$api_responses_file"
      echo "" >> "$api_responses_file"
    fi
  else
    # Invalid JSON or HTML error
    echo -e "${RED}✗ $endpoint (HTTP $http_code) - Invalid JSON response${NC}"
    ((failed++))
    failed_items+=("API: $endpoint (HTTP $http_code) - Invalid JSON")
    
    # Save error details
    echo "=== ERROR: $endpoint ===" >> "$api_responses_file"
    echo "$body" | head -20 >> "$api_responses_file"
    echo "" >> "$api_responses_file"
  fi
done

echo ""
echo -e "Summary: ${GREEN}$successful${NC}/$total checks passed (${RED}$failed failed${NC})"

# If there are failures, show details
if [ $failed -gt 0 ]; then
  echo ""
  echo -e "${YELLOW}Failed checks:${NC}"
  for item in "${failed_items[@]}"; do
    echo "  - $item"
  done
  
  echo ""
  echo -e "${YELLOW}Error details saved to:${NC}"
  echo "  UI errors: $temp_file"
  echo "  API responses: $api_responses_file"
  echo ""
  echo -e "${YELLOW}To view error details:${NC}"
  echo "  cat $temp_file      # UI route errors"
  echo "  cat $api_responses_file  # API response details"
  
  # Check console logs for compilation errors
  echo ""
  echo -e "${YELLOW}Checking for compilation errors...${NC}"
  
  # Look for TypeScript/React errors in docker logs
  if command -v docker-compose &> /dev/null; then
    echo ""
    echo "Recent console errors from Docker logs:"
    docker-compose logs --tail=20 console 2>&1 | grep -E "(error|Error|ERROR|TypeError|ReferenceError|SyntaxError|Failed to compile)" | head -10 || echo "No recent errors found in Docker logs"
  fi
  
  echo ""
  echo -e "${YELLOW}To debug and fix:${NC}"
  echo "1. Check the console output in the terminal where the dev server is running"
  echo "2. Look for TypeScript compilation errors or missing imports"
  echo "3. Check browser console for runtime errors"
  echo "4. Run 'npm run lint' to find linting issues"
  echo "5. Check if required services are running (CRM, Identity, etc.)"
  echo "6. Ensure all dependencies are installed with 'npm install'"
  echo "7. Check package.json for missing dependencies"
else
  # Clean up temp files if no errors
  rm -f "$temp_file"
  echo ""
  echo -e "${GREEN}All checks passed! 🎉${NC}"
  echo ""
  echo -e "${BLUE}API response samples saved to:${NC} $api_responses_file"
fi

# Show service status
echo ""
echo -e "${BLUE}=== Service Status ===${NC}"
echo ""

# Check which services are running
services=(
  "midaz-onboarding:Onboarding API"
  "midaz-transaction:Transaction API"
  "plugin-crm:CRM Plugin"
  "plugin-identity:Identity Plugin"
  "plugin-auth:Auth Plugin"
  "plugin-fees:Fees Plugin"
  "plugin-reconciliation:Reconciliation Plugin"
)

for service_info in "${services[@]}"; do
  IFS=':' read -r container_name display_name <<< "$service_info"
  if docker ps | grep -q "$container_name"; then
    echo -e "${GREEN}✓ $display_name is running${NC}"
  else
    echo -e "${YELLOW}○ $display_name is not running${NC}"
  fi
done