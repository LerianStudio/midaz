#!/bin/bash

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "${BLUE}Generating test coverage report...${NC}"

PACKAGES=$(go list ./... | grep -v -f ./scripts/coverage_ignore.txt)

echo "${BLUE}Running tests on packages:${NC}"
echo "$PACKAGES"

go test -cover $PACKAGES -coverprofile=coverage.out

printf "\n${GREEN}Coverage Summary:${NC}\n"
go tool cover -func=coverage.out

printf "\n${BLUE}Generating HTML coverage report...${NC}\n"
go tool cover -html=coverage.out -o coverage.html
echo "${GREEN}HTML coverage report generated at: ${NC}coverage.html"
