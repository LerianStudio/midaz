#!/bin/bash

# Script to verify API documentation consistency
# This script ensures that:
# 1. OpenAPI documentation is valid
# 2. Documentation matches implementation
# 3. All endpoints and error codes are properly documented

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Ensure that js-yaml is available in all component directories
ensure_dependencies() {
  local component_dir="$1"
  
  if [ -d "$component_dir/scripts" ]; then
    if [ ! -d "$component_dir/node_modules/js-yaml" ]; then
      echo "Installing js-yaml dependency in $component_dir..."
      (cd "$component_dir" && npm install js-yaml glob commander axios)
    fi
  fi
}

echo "Verifying API documentation and implementations..."

# List of components to verify
COMPONENTS=(
  "components/onboarding"
  "components/transaction"
)

# Verify each component
for component in "${COMPONENTS[@]}"; do
  component_dir="$PROJECT_ROOT/$component"
  
  if [ ! -d "$component_dir" ]; then
    echo "Component directory not found: $component_dir"
    continue
  fi
  
  echo ""
  echo "Verifying $component..."
  
  # Ensure dependencies are installed in the component directory
  ensure_dependencies "$component_dir"
  
  # Use single make target for validation if available
  if grep -q "validate-api-docs:" "$component_dir/Makefile"; then
    echo "Using unified validation approach..."
    if cd "$component_dir" && make validate-api-docs; then
      echo "Documentation validated successfully"
    else
      echo "Documentation validation failed"
      exit 1
    fi
  else
    # Fallback to the original step-by-step approach
    echo "Using step-by-step validation approach..."
    
    # Step 1: Generate OpenAPI documentation
    echo "Generating OpenAPI documentation..."
    if cd "$component_dir" && make generate-docs; then
      echo "Documentation generated successfully"
    else
      echo "Failed to generate documentation"
      exit 1
    fi
    
    # Step 2: Validate OpenAPI structure
    echo ""
    echo "Validating OpenAPI structure..."
    if [ -f "$component_dir/scripts/validate-api-docs.js" ]; then
      if cd "$SCRIPT_DIR" && node "$component_dir/scripts/validate-api-docs.js"; then
        echo "OpenAPI structure is valid"
      else
        echo "OpenAPI structure validation failed"
        exit 1
      fi
    else
      echo "! Validation script not found, skipping structure validation"
    fi
    
    # Step 3: Validate API implementations against documentation
    echo ""
    echo "Validating API implementations..."
    if [ -f "$component_dir/scripts/validate-api-implementations.js" ]; then
      if cd "$SCRIPT_DIR" && node "$component_dir/scripts/validate-api-implementations.js"; then
        echo "API implementations match documentation"
      else
        echo "Found discrepancies between implementations and documentation"
        echo "Run with --fix to automatically add missing response codes:"
        echo "node $component_dir/scripts/validate-api-implementations.js --fix"
        exit 1
      fi
    else
      echo "! Validation script not found, skipping implementation validation"
    fi
  fi
done

echo ""
echo "API documentation verification completed successfully"
exit 0