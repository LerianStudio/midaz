#!/bin/bash
set -euo pipefail

# Setup script for documentation generation dependencies
# This should be run once during CI setup phase

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Setting up documentation dependencies..."

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check all required tools are available
check_system_dependencies() {
    echo "Checking system dependencies..."
    
    local tools=("node" "npm" "jq" "go")
    local missing_tools=()
    
    for tool in "${tools[@]}"; do
        if ! command_exists "$tool"; then
            missing_tools+=("$tool")
        fi
    done
    
    if [ ${#missing_tools[@]} -gt 0 ]; then
        echo "❌ Missing required tools: ${missing_tools[*]}"
        echo "Please install the missing tools and run this script again."
        exit 1
    fi
    
    echo "  ✅ System dependencies OK"
}

# Install swag tool globally if not present
install_swag() {
    echo "Checking swag tool..."
    
    if ! command_exists swag; then
        echo "  Installing swag tool..."
        go install github.com/swaggo/swag/cmd/swag@latest
        echo "  ✅ swag installed"
    else
        echo "  ✅ swag available"
    fi
}

# Install Node.js dependencies for postman collection generation
install_node_dependencies() {
    echo "Installing Node.js dependencies..."
    
    local postman_dir="${ROOT_DIR}/scripts/postman-coll-generation"
    
    if [ ! -f "$postman_dir/package.json" ]; then
        echo "❌ package.json not found in $postman_dir"
        exit 1
    fi
    
    cd "$postman_dir"
    
    # Use npm ci for faster, reliable, reproducible builds
    if [ -f "package-lock.json" ]; then
        # Using npm ci
        npm ci --silent --prefer-offline
    else
        # Using npm install
        npm install --silent
    fi
    
    # Verify critical packages are installed
    local required_packages=("js-yaml" "uuid")
    for package in "${required_packages[@]}"; do
        if [ ! -d "node_modules/$package" ]; then
            echo "❌ Required package '$package' not installed"
            exit 1
        fi
    done
    
    echo "  ✅ Node.js dependencies OK"
}

# Create necessary directories
create_directories() {
    echo "Creating necessary directories..."
    
    local dirs=(
        "${ROOT_DIR}/tmp"
        "${ROOT_DIR}/postman"
        "${ROOT_DIR}/postman/backups"
        "${ROOT_DIR}/postman/temp"
    )
    
    for dir in "${dirs[@]}"; do
        if [ ! -d "$dir" ]; then
            mkdir -p "$dir"
        fi
    done
}

# Verify component structure
verify_component_structure() {
    echo "Verifying component structure..."
    
    local components=("onboarding" "transaction")
    
    for component in "${components[@]}"; do
        local main_file="${ROOT_DIR}/components/${component}/cmd/app/main.go"
        local api_dir="${ROOT_DIR}/components/${component}/api"
        
        if [ ! -f "$main_file" ]; then
            echo "❌ Missing main.go for component: $component"
            echo "    Expected: $main_file"
            exit 1
        fi
        
        if [ ! -d "$api_dir" ]; then
            mkdir -p "$api_dir"
        fi
    done
}

# Check disk space
check_disk_space() {
    echo "Checking disk space..."
    
    local available_space
    available_space=$(df . | awk 'NR==2 {print $4}')
    
    if [ "$available_space" -lt 500000 ]; then  # Less than 500MB
        echo "⚠️ Low disk space: ${available_space}KB available"
        echo "  Consider freeing up disk space before running documentation generation"
    fi
}

# Main execution
main() {
    # Ensure tmp directory exists first
    mkdir -p "${ROOT_DIR}/tmp"
    
    # Create a lock file to prevent concurrent setups
    local setup_lock_file="${ROOT_DIR}/tmp/setup-docs.lock"
    
    # Check if setup is already running
    if [ -f "$setup_lock_file" ]; then
        echo "⚠️ Setup already in progress (lock file exists)"
        echo "If this is incorrect, remove: $setup_lock_file"
        exit 1
    fi
    
    # Create lock file
    touch "$setup_lock_file"
    
    # Ensure lock file is removed on exit
    trap "rm -f '$setup_lock_file'" EXIT
    
    # Starting dependency setup
    
    check_system_dependencies
    echo ""
    
    install_swag
    echo ""
    
    create_directories
    echo ""
    
    verify_component_structure
    echo ""
    
    install_node_dependencies
    echo ""
    
    check_disk_space
    echo ""
    
    echo "✅ Dependencies setup completed"
    echo "You can now run 'make generate-docs' reliably."
}

# Run main function
main "$@"