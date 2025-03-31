#!/bin/bash

# Set colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'


# Display header
echo -e "${CYAN}----------------------------------------------${NC}"
echo -e "${CYAN}   Verifying error logging in usecases  ${NC}"
echo -e "${CYAN}----------------------------------------------${NC}"

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
        # Use awk to analyze the file more thoroughly
        MISSING_LOGS=$(awk '
            BEGIN { issues = 0; line_num = 0; in_func = 0; has_log = 0; }
            
            # Track line numbers
            { line_num++ }
            
            # Check for function start
            /func/ { in_func = 1; has_log = 0; }
            
            # Check for error logging statements
            /logger\.Error/ || /log\.Error/ || /logger\.Errorf/ || /log\.Errorf/ { 
                has_log = 1; 
                last_log_line = line_num;
            }
            
            # Check for return err statements
            /return err/ { 
                # If we have not seen a logging statement in the last 5 lines, flag it
                if (has_log == 0 || (line_num - last_log_line > 5)) {
                    printf("%d:		%s\n", line_num, $0);
                    issues++;
                }
            }
            
            # Reset when we leave a function
            /^}/ { 
                if (in_func == 1) {
                    in_func = 0; 
                    has_log = 0;
                }
            }
            
            END { exit issues }
        ' "$file")
        
        EXIT_CODE=$?
        
        if [ $EXIT_CODE -ne 0 ]; then
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

echo "${GREEN}[ok]${NC} Error logging verification completed ${GREEN}✔️${NC}"

exit 0
