#!/bin/bash

# Git hooks directory
HOOKS_DIR=".git/hooks"
PROJECT_ROOT=$(git rev-parse --show-toplevel)

echo "Setting up git hooks for Midaz project..."

# Check if .git directory exists
if [ ! -d ".git" ]; then
    echo "No .git directory found. Are you in the root of the repository?"
    exit 1
fi

# Create hooks directory if it doesn't exist
mkdir -p "$HOOKS_DIR"

# Check if .githooks directory exists
if [ -d ".githooks" ]; then
    echo "Found .githooks directory, checking for hook files..."
    
    # Get all hook directories in .githooks
    HOOK_DIRS=$(find .githooks -maxdepth 1 -type d | grep -v "^.githooks$")
    
    for hook_dir in $HOOK_DIRS; do
        hook_name=$(basename "$hook_dir")
        echo "Processing $hook_name hook..."
        
        # Check if it's a directory
        if [ -d "$hook_dir" ]; then
            echo "Found $hook_name directory, using template instead"
            create_template_hook=true
        elif [ -f "$hook_dir" ]; then
            echo "Installing $hook_name hook from .githooks"
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
                    echo "Creating template pre-commit hook"
                    cat > "$HOOKS_DIR/pre-commit" << 'EOF'
#!/bin/bash

echo "Running pre-commit checks..."

# Check for .env files
if git diff --cached --name-only | grep -q "\.env$"; then
    echo "[ERROR] Attempting to commit .env file. Please remove it from staging."
    exit 1
fi

# Check for large files (>5MB)
LARGE_FILES=$(git diff --cached --name-only | xargs ls -l 2>/dev/null | awk '$5 > 5000000 {print $9}')
if [ ! -z "$LARGE_FILES" ]; then
    echo "[ERROR] Attempting to commit large files (>5MB):"
    echo "$LARGE_FILES"
    echo "Please remove them from staging or use Git LFS."
    exit 1
fi

# Run gofmt on staged Go files
STAGED_GO_FILES=$(git diff --cached --name-only | grep "\.go$")
if [ ! -z "$STAGED_GO_FILES" ]; then
    echo "Checking Go formatting..."
    for file in $STAGED_GO_FILES; do
        if [ -f "$file" ]; then
            gofmt -l -w "$file"
            git add "$file"
        fi
    done
fi

# Run golangci-lint if available
if command -v golangci-lint >/dev/null 2>&1; then
    echo "Running quick lint check..."
    golangci-lint run ./... || {
        echo "[ERROR] Linting failed. Please fix the issues before committing."
        echo "You can run 'make lint' for more details."
        exit 1
    }
fi

echo "[PASS] Pre-commit checks completed successfully."
exit 0
EOF
                    chmod +x "$HOOKS_DIR/pre-commit"
                    ;;
                    
                pre-push)
                    echo "Creating template pre-push hook"
                    cat > "$HOOKS_DIR/pre-push" << 'EOF'
#!/bin/bash

echo "Running pre-push checks..."

# Run tests
echo "Running tests..."
go test ./... -short || {
    echo "[ERROR] Tests failed. Please fix the failing tests before pushing."
    exit 1
}

echo "[PASS] Pre-push checks completed successfully."
exit 0
EOF
                    chmod +x "$HOOKS_DIR/pre-push"
                    ;;
                    
                commit-msg)
                    echo "Creating template commit-msg hook"
                    cat > "$HOOKS_DIR/commit-msg" << 'EOF'
#!/bin/bash

echo "Checking commit message format..."

# Get the commit message from the file
commit_msg_file=$1
commit_msg=$(cat "$commit_msg_file")

# Get the first line of the commit message
first_line=$(echo "$commit_msg" | head -n 1)

# For debugging
echo "Commit message first line: '$first_line'"

# Define the conventional commit format regex
# Format: type(scope): description
# Where type is one of: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
conventional_format='^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-zA-Z0-9_.-]*\))?: .+'

if ! [[ "$first_line" =~ $conventional_format ]]; then
    echo "[ERROR] Commit message does not follow conventional format."
    echo "Format should be: type(scope): description"
    echo "Where type is one of: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert"
    echo "Example: feat(auth): add login functionality"
    exit 1
fi

echo "[PASS] Commit message format is valid."
exit 0
EOF
                    chmod +x "$HOOKS_DIR/commit-msg"
                    ;;
                    
                pre-receive)
                    echo "Creating template pre-receive hook"
                    cat > "$HOOKS_DIR/pre-receive" << 'EOF'
#!/bin/bash

echo "Running pre-receive checks..."

# This hook runs on the server side
# Add server-side validation logic here if needed

echo "[PASS] Pre-receive checks completed successfully."
exit 0
EOF
                    chmod +x "$HOOKS_DIR/pre-receive"
                    ;;
                    
                *)
                    echo "No template available for $hook_name, creating a basic hook"
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
    echo "No .githooks directory found, using template hooks"
    
    # Create pre-commit hook
    echo "Creating template pre-commit hook"
    cat > "$HOOKS_DIR/pre-commit" << 'EOF'
#!/bin/bash

echo "Running pre-commit checks..."

# Check for .env files
if git diff --cached --name-only | grep -q "\.env$"; then
    echo "[ERROR] Attempting to commit .env file. Please remove it from staging."
    exit 1
fi

# Check for large files (>5MB)
LARGE_FILES=$(git diff --cached --name-only | xargs ls -l 2>/dev/null | awk '$5 > 5000000 {print $9}')
if [ ! -z "$LARGE_FILES" ]; then
    echo "[ERROR] Attempting to commit large files (>5MB):"
    echo "$LARGE_FILES"
    echo "Please remove them from staging or use Git LFS."
    exit 1
fi

# Run gofmt on staged Go files
STAGED_GO_FILES=$(git diff --cached --name-only | grep "\.go$")
if [ ! -z "$STAGED_GO_FILES" ]; then
    echo "Checking Go formatting..."
    for file in $STAGED_GO_FILES; do
        if [ -f "$file" ]; then
            gofmt -l -w "$file"
            git add "$file"
        fi
    done
fi

# Run golangci-lint if available
if command -v golangci-lint >/dev/null 2>&1; then
    echo "Running quick lint check..."
    golangci-lint run ./... || {
        echo "[ERROR] Linting failed. Please fix the issues before committing."
        echo "You can run 'make lint' for more details."
        exit 1
    }
fi

echo "[PASS] Pre-commit checks completed successfully."
exit 0
EOF
    chmod +x "$HOOKS_DIR/pre-commit"
    
    # Create pre-push hook
    echo "Creating template pre-push hook"
    cat > "$HOOKS_DIR/pre-push" << 'EOF'
#!/bin/bash

echo "Running pre-push checks..."

# Run tests
echo "Running tests..."
go test ./... -short || {
    echo "[ERROR] Tests failed. Please fix the failing tests before pushing."
    exit 1
}

echo "[PASS] Pre-push checks completed successfully."
exit 0
EOF
    chmod +x "$HOOKS_DIR/pre-push"
    
    # Create commit-msg hook
    echo "Creating template commit-msg hook"
    cat > "$HOOKS_DIR/commit-msg" << 'EOF'
#!/bin/bash

echo "Checking commit message format..."

# Get the commit message from the file
commit_msg_file=$1
commit_msg=$(cat "$commit_msg_file")

# Get the first line of the commit message
first_line=$(echo "$commit_msg" | head -n 1)

# For debugging
echo "Commit message first line: '$first_line'"

# Define the conventional commit format regex
# Format: type(scope): description
# Where type is one of: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
conventional_format='^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-zA-Z0-9_.-]*\))?: .+'

if ! [[ "$first_line" =~ $conventional_format ]]; then
    echo "[ERROR] Commit message does not follow conventional format."
    echo "Format should be: type(scope): description"
    echo "Where type is one of: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert"
    echo "Example: feat(auth): add login functionality"
    exit 1
fi

echo "[PASS] Commit message format is valid."
exit 0
EOF
    chmod +x "$HOOKS_DIR/commit-msg"
fi

# List installed hooks in a portable way
echo "[ok] Git hooks installed successfully."
echo "Installed hooks:"
for hook in $(ls -1 "$HOOKS_DIR" | grep -v "\.sample$"); do
    if [ -x "$HOOKS_DIR/$hook" ]; then
        echo "  - $hook"
    fi
done
