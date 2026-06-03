#!/bin/bash

# Script to verify API documentation consistency for tracer
# Uses Docker-based openapi-generator-cli for validation (no Node.js required)

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo -e "${CYAN}--------------------------------------------------------------------${NC}"
echo -e "${CYAN}   Verifying API documentation for tracer                           ${NC}"
echo -e "${CYAN}--------------------------------------------------------------------${NC}"

# Check if swagger.json exists
if [ ! -f "$PROJECT_ROOT/api/swagger.json" ]; then
  echo -e "${YELLOW}swagger.json not found. Generating documentation first...${NC}"
  cd "$PROJECT_ROOT" && make generate-docs
fi

# Validate OpenAPI spec using openapi-generator-cli
echo -e "${CYAN}Validating OpenAPI specification...${NC}"
if docker run --rm -v "$PROJECT_ROOT:/local" openapitools/openapi-generator-cli:v7.10.0 validate -i /local/api/swagger.json; then
  echo -e "${GREEN}[ok]${NC} OpenAPI specification is valid"
else
  echo -e "${RED}[error]${NC} OpenAPI specification validation failed"
  exit 1
fi

echo -e "${GREEN}[ok]${NC} API documentation verification completed successfully"
exit 0