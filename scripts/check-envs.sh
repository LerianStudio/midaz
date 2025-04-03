#!/bin/bash

# Script to check environment setup and configuration

# ===== Git Hooks Check =====
echo "===== Git Hooks Check ====="
HOOKS_INSTALLED=true

# List of hooks to check
HOOKS=("pre-commit" "pre-push" "commit-msg" "pre-receive")

for hook in "${HOOKS[@]}"; do
    if [ ! -f ".git/hooks/$hook" ]; then
        echo "❌ $hook hook is missing"
        HOOKS_INSTALLED=false
    else
        echo "✅ $hook hook is installed"
    fi
done

# If hooks are not installed, suggest running setup-git-hooks
if [ "$HOOKS_INSTALLED" = false ]; then
    echo "➡️ Run 'make setup-git-hooks' to install missing hooks"
fi

# ===== Git Security Check =====
echo ""
echo "===== Git Security Check ====="
EXPOSED_ENV_FILES=$(git ls-files | grep "\.env$" | grep -v "\.env\.example$" | grep -v "\.env\.sample$")

if [ -z "$EXPOSED_ENV_FILES" ]; then
    echo "✅ No .env files exposed in git"
else
    echo "⚠️ The following .env files are tracked by git:"
    echo "$EXPOSED_ENV_FILES"
    echo "➡️ Run: git rm --cached <file> to untrack without deleting"
fi

# ===== Environment Files Check =====
echo ""
echo "===== Environment Files Check ====="

# ----- Components -----
echo "Components:"
COMPONENTS=$(find ./components -maxdepth 1 -type d | grep -v "^./components$")
MISSING_ENV_FILES=false

for component in $COMPONENTS; do
    component_name=$(basename "$component")
    
    # Check if .env exists
    if [ -f "$component/.env" ]; then
        echo "✅ $component_name"
    else
        # Check if .env.example exists
        if [ -f "$component/.env.example" ]; then
            echo "❌ $component_name (template exists)"
            MISSING_ENV_FILES=true
        else
            echo "ℹ️ $component_name (no env files)"
        fi
    fi
done

# ----- Go SDK -----
echo ""
echo "Go SDK:"
SDK_GO_DIR="./sdks/go"
SDK_MISSING_ENV=false

# Check if Go SDK directory exists
if [ -d "$SDK_GO_DIR" ]; then
    # Check if .env exists
    if [ -f "$SDK_GO_DIR/.env" ]; then
        echo "✅ Main SDK"
    else
        # Check if .env.example exists
        if [ -f "$SDK_GO_DIR/.env.example" ]; then
            echo "❌ Main SDK (template exists)"
            SDK_MISSING_ENV=true
        else
            echo "ℹ️ Main SDK (no env files)"
        fi
    fi
    
    # Check for .env files in SDK examples
    if [ -d "$SDK_GO_DIR/examples" ]; then
        echo ""
        echo "SDK Examples:"
        
        # Find all example directories
        SDK_EXAMPLES=$(find "$SDK_GO_DIR/examples" -maxdepth 1 -type d | grep -v "^$SDK_GO_DIR/examples$")
        
        for example in $SDK_EXAMPLES; do
            example_name=$(basename "$example")
            
            # Check if .env exists
            if [ -f "$example/.env" ]; then
                echo "✅ $example_name"
            else
                # Check if .env.example exists
                if [ -f "$example/.env.example" ]; then
                    echo "❌ $example_name (template exists)"
                    SDK_MISSING_ENV=true
                else
                    # Only show info if the example might need env files (has Go files)
                    if [ -n "$(find "$example" -name "*.go" -type f -print -quit)" ]; then
                        echo "ℹ️ $example_name (no env files)"
                    fi
                fi
            fi
        done
    fi
else
    echo "ℹ️ SDK directory not found"
fi

# ===== Action Recommendations =====
echo ""
echo "===== Action Recommendations ====="
if [ "$MISSING_ENV_FILES" = true ]; then
    echo "➡️ Run 'make set-env' to create component .env files from templates"
fi

if [ "$SDK_MISSING_ENV" = true ]; then
    echo "➡️ Run 'make sdk-go-env-setup' to create SDK .env files from templates"
fi

if [ "$HOOKS_INSTALLED" = false ]; then
    echo "➡️ Run 'make setup-git-hooks' to install git hooks"
fi

if [ -n "$EXPOSED_ENV_FILES" ]; then
    echo "➡️ Remove .env files from git tracking to protect sensitive information"
fi

echo ""
echo "✅ Environment check completed"
