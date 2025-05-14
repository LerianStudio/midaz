#!/bin/bash

# Set colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Check if git hooks are installed
echo "${CYAN}Checking if git hooks are installed...${NC}"
HOOKS_INSTALLED=true

# List of hooks to check
HOOKS=("pre-commit" "pre-push" "commit-msg" "pre-receive")

for hook in "${HOOKS[@]}"; do
    if [ ! -f ".git/hooks/$hook" ]; then
        echo "${RED}${BOLD}[MISSING]${NC} $hook hook is not installed"
        HOOKS_INSTALLED=false
    else
        echo "${GREEN}${BOLD}[OK]${NC} $hook hook is installed"
    fi
done

# If hooks are not installed, suggest running setup-git-hooks
if [ "$HOOKS_INSTALLED" = false ]; then
    echo "${YELLOW}Run 'make setup-git-hooks' to install missing hooks${NC}"
    echo ""
fi

# Check for exposed .env files in git
echo "${CYAN}Checking for exposed .env files...${NC}"
EXPOSED_ENV_FILES=$(git ls-files | grep "\.env$" | grep -v "\.env\.example$" | grep -v "\.env\.sample$")

if [ -z "$EXPOSED_ENV_FILES" ]; then
    echo "${GREEN}${BOLD}[OK]${NC} No .env files are exposed in git"
else
    echo "${RED}${BOLD}[WARNING]${NC} The following .env files are tracked by git:"
    echo "$EXPOSED_ENV_FILES"
    echo "${YELLOW}Consider adding these files to .gitignore and removing them from git${NC}"
    echo "Run: git rm --cached <file> to untrack without deleting the file"
fi

# Check for .env files in all components
echo ""
echo "${CYAN}Checking for .env files in components...${NC}"

# Find all components
COMPONENTS=$(find ./components -maxdepth 1 -type d | grep -v "^./components$")
MISSING_ENV_FILES=false

for component in $COMPONENTS; do
    component_name=$(basename "$component")
    
    # Check if .env exists
    if [ -f "$component/.env" ]; then
        echo "${GREEN}${BOLD}[OK]${NC} $component_name has .env file"
    else
        # Check if .env.example exists
        if [ -f "$component/.env.example" ]; then
            echo "${YELLOW}${BOLD}[MISSING]${NC} $component_name is missing .env file (but has .env.example)"
            MISSING_ENV_FILES=true
        else
            echo "${CYAN}${BOLD}[INFO]${NC} $component_name does not have .env or .env.example files"
        fi
    fi
done

# Check if set-env target exists in Makefile
if grep -q "set-env:" Makefile; then
    if [ "$MISSING_ENV_FILES" = true ]; then
        echo "${YELLOW}Run 'make set-env' to create .env files from templates${NC}"
    fi
else
    if [ "$MISSING_ENV_FILES" = true ]; then
        echo "${YELLOW}Consider creating a 'set-env' target in your Makefile to automate .env file creation${NC}"
    fi
fi

echo ""
echo "${CYAN}Environment check completed.${NC}"

# Always exit with success code as this is an informational script
exit 0
