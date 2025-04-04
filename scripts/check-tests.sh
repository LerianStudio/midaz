#!/bin/bash

# Set colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Minimum required test coverage percentage
MIN_COVERAGE=80

# Find all Go packages in the project
echo "Checking test coverage for all components..."
echo ""

# Check each component directory
for component in ./components/*; do
    if [ -d "$component" ]; then
        component_name=$(basename "$component")
        echo "${YELLOW}Checking $component_name component...${NC}"
        
        # Skip if no Go files
        if ! find "$component" -name "*.go" -type f | grep -q .; then
            echo "${YELLOW}No Go files found in $component_name, skipping coverage check${NC}"
            echo ""
            continue
        fi
        
        # Run coverage test
        (cd "$component" && go test -coverprofile=coverage.tmp ./... > /dev/null 2>&1)
        
        if [ $? -eq 0 ]; then
            # Extract coverage percentage
            if [ -f "$component/coverage.tmp" ]; then
                coverage=$(go tool cover -func="$component/coverage.tmp" | grep total | awk '{print $3}' | sed 's/%//')
                
                # Compare with minimum required
                if (( $(echo "$coverage < $MIN_COVERAGE" | bc -l) )); then
                    echo "${RED}${BOLD}[FAIL]${NC} $component_name coverage is ${RED}$coverage%${NC} (minimum required: ${GREEN}$MIN_COVERAGE%${NC})"
                else
                    echo "${GREEN}${BOLD}[PASS]${NC} $component_name coverage is ${GREEN}$coverage%${NC}"
                fi
                
                # Clean up temporary file
                rm "$component/coverage.tmp"
            else
                echo "${YELLOW}No coverage data generated for $component_name${NC}"
            fi
        else
            echo "${RED}${BOLD}[ERROR]${NC} Failed to run tests for $component_name"
        fi
        
        echo ""
    fi
done

echo "Coverage check completed."
