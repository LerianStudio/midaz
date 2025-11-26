#!/bin/bash

# Script to verify API documentation consistency for plugin-crm
# This script ensures that:
# 1. OpenAPI documentation is valid
# 2. Documentation matches implementation
# 3. All endpoints and error codes are properly documented

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPONENT_DIR="$PROJECT_ROOT"

echo -e "${CYAN}------------------------------------------------------${NC}"
echo -e "${CYAN}   Verifying API documentation for plugin-crm         ${NC}"
echo -e "${CYAN}------------------------------------------------------${NC}"

# Ensure dependencies
ensure_dependencies() {
  if [ -d "$COMPONENT_DIR/scripts" ]; then
    if [ ! -d "$COMPONENT_DIR/scripts/node_modules/js-yaml" ]; then
      echo -e "${YELLOW}Installing dependencies in plugin-crm/scripts...${NC}"
      (cd "$COMPONENT_DIR/scripts" && npm install js-yaml glob commander axios)
    fi
  fi
}

# Check if component dir exists
if [ ! -d "$COMPONENT_DIR" ]; then
  echo -e "${RED}Directory not found: $COMPONENT_DIR${NC}"
  exit 1
fi

ensure_dependencies

# Try unified Makefile validation
if grep -q "validate-api-docs:" "$COMPONENT_DIR/Makefile"; then
  echo -e "${CYAN}Using unified Makefile target: validate-api-docs...${NC}"
  if cd "$COMPONENT_DIR" && make validate-api-docs; then
    echo -e "${GREEN}[ok]${NC} Documentation validated successfully"
  else
    echo -e "${RED}[error]${NC} Documentation validation failed"
    exit 1
  fi
else
  echo -e "${CYAN}Fallback: manual step-by-step validation${NC}"

  # Step 1: Generate docs
  echo -e "${CYAN}Generating OpenAPI documentation...${NC}"
  if cd "$COMPONENT_DIR" && make generate-docs; then
    echo -e "${GREEN}[ok]${NC} Documentation generated"
  else
    echo -e "${RED}[error]${NC} Failed to generate documentation"
    exit 1
  fi

  # Step 2: Validate OpenAPI structure
  if [ -f "$COMPONENT_DIR/scripts/validate-api-docs.js" ]; then
    echo -e "${CYAN}Validating OpenAPI structure...${NC}"
    if cd "$SCRIPT_DIR" && node "$COMPONENT_DIR/scripts/validate-api-docs.js"; then
      echo -e "${GREEN}[ok]${NC} Structure is valid"
    else
      echo -e "${RED}[error]${NC} Structure validation failed"
      exit 1
    fi
  else
    echo -e "${YELLOW}! Skipping structure validation – script not found${NC}"
  fi

  # Step 3: Validate implementation vs docs
  if [ -f "$COMPONENT_DIR/scripts/validate-api-implementations.js" ]; then
    echo -e "${CYAN}Validating implementation vs documentation...${NC}"
    if cd "$SCRIPT_DIR" && node "$COMPONENT_DIR/scripts/validate-api-implementations.js"; then
      echo -e "${GREEN}[ok]${NC} Implementation matches documentation"
    else
      echo -e "${RED}[warning]${NC} Discrepancies found"
      echo "To fix automatically:"
      echo "  node $COMPONENT_DIR/scripts/validate-api-implementations.js --fix"
      exit 1
    fi
  else
    echo -e "${YELLOW}! Skipping implementation validation – script not found${NC}"
  fi
fi

echo -e "${GREEN}[✔]${NC} API documentation verification completed successfully"
exit 0