#!/bin/bash

# Set colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Check for error logging in usecases
echo "${CYAN}Checking for proper error logging in usecases...${NC}"

# Define directories to check
COMPONENTS=("./components/mdz" "./components/onboarding" "./components/transaction")

# Define patterns to search for
MISSING_LOG_PATTERN="return err"
PROPER_LOG_PATTERN="log.*Error.*return err"

# Count of issues found
ISSUES_FOUND=0

for component in "${COMPONENTS[@]}"; do
    echo "${YELLOW}Checking ${component}...${NC}"
    
    # Skip if no Go files
    if ! find "$component" -name "*.go" -type f | grep -q .; then
        echo "${YELLOW}No Go files found in $component, skipping check${NC}"
        echo ""
        continue
    fi
    
    # Find all service files that might contain usecases
    SERVICE_FILES=$(find "$component" -path "*/services/*" -name "*.go" | grep -v "_test.go")
    
    for file in $SERVICE_FILES; do
        # Check for potential missing error logging
        MISSING_LOGS=$(grep -n "$MISSING_LOG_PATTERN" "$file" | grep -v "$PROPER_LOG_PATTERN" | grep -v "log\." | grep -v "fmt\.")
        
        if [ ! -z "$MISSING_LOGS" ]; then
            echo "${RED}${BOLD}[ISSUE]${NC} Potential missing error logging in ${file}:${NC}"
            echo "$MISSING_LOGS" | while read -r line; do
                LINE_NUM=$(echo "$line" | cut -d':' -f1)
                CODE=$(echo "$line" | cut -d':' -f2-)
                echo "  ${RED}Line $LINE_NUM:${NC} $CODE"
            done
            echo ""
            ISSUES_FOUND=$((ISSUES_FOUND + 1))
        fi
    done
done

if [ $ISSUES_FOUND -eq 0 ]; then
    echo "${GREEN}${BOLD}[PASS]${NC} No issues found with error logging in usecases."
else
    echo "${RED}${BOLD}[WARNING]${NC} Found $ISSUES_FOUND potential issues with error logging."
    echo "${YELLOW}Consider adding proper error logging before returning errors.${NC}"
fi

exit 0
