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
    
    # Get all hook directories in .githooks
    HOOK_DIRS=$(find .githooks -maxdepth 1 -type d | grep -v "^.githooks$")
    
    for hook_dir in $HOOK_DIRS; do
        hook_name=$(basename "$hook_dir")
        echo "${CYAN}Processing $hook_name hook...${NC}"
        
        # Check if it's a directory
        if [ -d "$hook_dir" ]; then
            echo "${YELLOW}Found $hook_name directory, using template instead${NC}"
            create_template_hook=true
        elif [ -f "$hook_dir" ]; then
            echo "${CYAN}Installing $hook_name hook from .githooks${NC}"
            cp "$hook_dir" "$HOOKS_DIR/$hook_name"
            chmod +x "$HOOKS_DIR/$hook_name"
            create_template_hook=false
        else
            create_template_hook=true
        fi
        
        # Create template hook if needed
        if [ "$create_template_hook" = true ]; then
            case "$hook_name" in
                pre-commit)
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
                    ;;
                    
                pre-push)
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
                    ;;
                    
                commit-msg)
                    echo "${CYAN}Creating template commit-msg hook${NC}"
                    cat > "$HOOKS_DIR/commit-msg" << 'EOF'
#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color
BOLD='\033[1m'

echo "${YELLOW}Checking commit message format...${NC}"

# Get the commit message from the file
commit_msg_file=$1
commit_msg=$(cat "$commit_msg_file")

# Define the conventional commit format regex
# Format: type(scope): description
# Where type is one of: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
conventional_format="^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9-]+\))?: .+"

if ! [[ "$commit_msg" =~ $conventional_format ]]; then
    echo "${RED}${BOLD}[ERROR]${NC} Commit message does not follow conventional format."
    echo "${YELLOW}Format should be: type(scope): description${NC}"
    echo "${YELLOW}Where type is one of: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert${NC}"
    echo "${YELLOW}Example: feat(auth): add login functionality${NC}"
    exit 1
fi

echo "${GREEN}${BOLD}[PASS]${NC} Commit message format is valid."
exit 0
EOF
                    chmod +x "$HOOKS_DIR/commit-msg"
                    ;;
                    
                pre-receive)
                    echo "${CYAN}Creating template pre-receive hook${NC}"
                    cat > "$HOOKS_DIR/pre-receive" << 'EOF'
#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color
BOLD='\033[1m'

echo "${YELLOW}Running pre-receive checks...${NC}"

# This hook runs on the server side
# Add server-side validation logic here if needed

echo "${GREEN}${BOLD}[PASS]${NC} Pre-receive checks completed successfully."
exit 0
EOF
                    chmod +x "$HOOKS_DIR/pre-receive"
                    ;;
                    
                *)
                    echo "${YELLOW}No template available for $hook_name, creating a basic hook${NC}"
                    cat > "$HOOKS_DIR/$hook_name" << EOF
#!/bin/bash

# This is a basic template for the $hook_name hook
# Add your custom logic here

exit 0
EOF
                    chmod +x "$HOOKS_DIR/$hook_name"
                    ;;
            esac
        fi
    done
else
    echo "${YELLOW}No .githooks directory found, using template hooks${NC}"
    
    # Create pre-commit hook
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
    
    # Create pre-push hook
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

# List installed hooks in a portable way
echo "${GREEN}${BOLD}[ok]${NC} Git hooks installed successfully."
echo "${CYAN}Installed hooks:${NC}"
for hook in $(ls -1 "$HOOKS_DIR" | grep -v "\.sample$"); do
    if [ -x "$HOOKS_DIR/$hook" ]; then
        echo "  - $hook"
    fi
done
