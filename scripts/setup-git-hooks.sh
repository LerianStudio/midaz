#!/bin/bash

# Set colors for output
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Git hooks directory
HOOKS_DIR=".git/hooks"
PROJECT_ROOT=$(git rev-parse --show-toplevel)

echo "${CYAN}Setting up git hooks for Midaz project...${NC}"

# Check if .git directory exists
if [ ! -d ".git" ]; then
    echo "${YELLOW}No .git directory found. Are you in the root of the repository?${NC}"
    exit 1
fi

# Create hooks directory if it doesn't exist
mkdir -p "$HOOKS_DIR"

# Check if .githooks directory exists
if [ -d ".githooks" ]; then
    echo "${CYAN}Found .githooks directory, checking for hook files...${NC}"
    
    # Check for pre-commit hook
    if [ -d ".githooks/pre-commit" ]; then
        echo "${YELLOW}Found pre-commit directory, using template instead${NC}"
        USE_TEMPLATE_PRECOMMIT=true
    elif [ -f ".githooks/pre-commit" ]; then
        echo "${CYAN}Installing pre-commit hook from .githooks${NC}"
        cp ".githooks/pre-commit" "$HOOKS_DIR/pre-commit"
        chmod +x "$HOOKS_DIR/pre-commit"
        USE_TEMPLATE_PRECOMMIT=false
    else
        USE_TEMPLATE_PRECOMMIT=true
    fi
    
    # Check for pre-push hook
    if [ -d ".githooks/pre-push" ]; then
        echo "${YELLOW}Found pre-push directory, using template instead${NC}"
        USE_TEMPLATE_PREPUSH=true
    elif [ -f ".githooks/pre-push" ]; then
        echo "${CYAN}Installing pre-push hook from .githooks${NC}"
        cp ".githooks/pre-push" "$HOOKS_DIR/pre-push"
        chmod +x "$HOOKS_DIR/pre-push"
        USE_TEMPLATE_PREPUSH=false
    else
        USE_TEMPLATE_PREPUSH=true
    fi
else
    echo "${YELLOW}No .githooks directory found, using template hooks${NC}"
    USE_TEMPLATE_PRECOMMIT=true
    USE_TEMPLATE_PREPUSH=true
fi

# Create pre-commit hook if needed
if [ "$USE_TEMPLATE_PRECOMMIT" = true ]; then
    echo "${CYAN}Creating template pre-commit hook${NC}"
    cat > "$HOOKS_DIR/pre-commit" << 'EOF'
#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color
BOLD='\033[1m'

echo "${YELLOW}Running pre-commit checks...${NC}"

# Check for .env files
if git diff --cached --name-only | grep -q "\.env$"; then
    echo "${RED}${BOLD}[ERROR]${NC} Attempting to commit .env file. Please remove it from staging."
    exit 1
fi

# Check for large files (>5MB)
LARGE_FILES=$(git diff --cached --name-only | xargs ls -l 2>/dev/null | awk '$5 > 5000000 {print $9}')
if [ ! -z "$LARGE_FILES" ]; then
    echo "${RED}${BOLD}[ERROR]${NC} Attempting to commit large files (>5MB):"
    echo "$LARGE_FILES"
    echo "Please remove them from staging or use Git LFS."
    exit 1
fi

# Run gofmt on staged Go files
STAGED_GO_FILES=$(git diff --cached --name-only | grep "\.go$")
if [ ! -z "$STAGED_GO_FILES" ]; then
    echo "${YELLOW}Checking Go formatting...${NC}"
    for file in $STAGED_GO_FILES; do
        if [ -f "$file" ]; then
            gofmt -l -w "$file"
            git add "$file"
        fi
    done
fi

# Run golangci-lint if available
if command -v golangci-lint >/dev/null 2>&1; then
    echo "${YELLOW}Running quick lint check...${NC}"
    golangci-lint run --fast ./... || {
        echo "${RED}${BOLD}[ERROR]${NC} Linting failed. Please fix the issues before committing."
        echo "${YELLOW}You can run 'make lint' for more details.${NC}"
        exit 1
    }
fi

echo "${GREEN}${BOLD}[PASS]${NC} Pre-commit checks completed successfully."
exit 0
EOF
    chmod +x "$HOOKS_DIR/pre-commit"
fi

# Create pre-push hook if needed
if [ "$USE_TEMPLATE_PREPUSH" = true ]; then
    echo "${CYAN}Creating template pre-push hook${NC}"
    cat > "$HOOKS_DIR/pre-push" << 'EOF'
#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color
BOLD='\033[1m'

echo "${YELLOW}Running pre-push checks...${NC}"

# Run tests
echo "${YELLOW}Running tests...${NC}"
go test ./... -short || {
    echo "${RED}${BOLD}[ERROR]${NC} Tests failed. Please fix the failing tests before pushing."
    exit 1
}

echo "${GREEN}${BOLD}[PASS]${NC} Pre-push checks completed successfully."
exit 0
EOF
    chmod +x "$HOOKS_DIR/pre-push"
fi

echo "${GREEN}${BOLD}[ok]${NC} Git hooks installed successfully."
echo "${CYAN}Installed hooks:${NC}"
echo "  - pre-commit: Checks formatting, linting, and prevents committing .env files"
echo "  - pre-push: Runs tests before pushing"
