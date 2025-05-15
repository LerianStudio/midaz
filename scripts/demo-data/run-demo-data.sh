#!/bin/bash
# Script to run the demo data generator from the Makefile

# Default volume size is small
VOLUME_SIZE="${VOLUME:-small}"
BASE_URL="${BASE_URL:-http://localhost}"
ONBOARDING_PORT="${ONBOARDING_PORT:-3000}"
TRANSACTION_PORT="${TRANSACTION_PORT:-3001}"
AUTH_TOKEN="${AUTH_TOKEN:-}"

# Colors for console output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Navigate to the demo-data directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}" || exit 1

echo -e "${BLUE}==============================================${NC}"
echo -e "${BLUE}   Midaz Demo Data Generator               ${NC}"
echo -e "${BLUE}==============================================${NC}"
echo

# Check if npm is installed
if ! command -v npm &> /dev/null; then
    echo -e "${RED}Error: npm is not installed${NC}"
    echo "Please install Node.js and npm to continue."
    exit 1
fi

# Install dependencies if they don't exist yet
if [ ! -d "node_modules" ]; then
    echo -e "${YELLOW}Installing dependencies...${NC}"
    npm install --no-fund --no-audit --loglevel=error
    echo
fi

# Check if auth token is provided
if [ -z "$AUTH_TOKEN" ]; then
    echo -e "${YELLOW}Warning: No AUTH_TOKEN provided. If your API requires authentication, this may fail.${NC}"
    echo "You can provide a token using the AUTH_TOKEN environment variable:"
    echo "make generate-demo-data AUTH_TOKEN=your_token"
    echo
fi

# Run the demo data generator
echo -e "${GREEN}Generating demo data (volume: ${VOLUME_SIZE})...${NC}"
echo "Base URL: ${BASE_URL}"
echo "Onboarding Port: ${ONBOARDING_PORT}"
echo "Transaction Port: ${TRANSACTION_PORT}"
echo

# Execute the Node.js application
node src/index.js \
  --volume "${VOLUME_SIZE}" \
  --base-url "${BASE_URL}" \
  --onboarding-port "${ONBOARDING_PORT}" \
  --transaction-port "${TRANSACTION_PORT}" \
  ${AUTH_TOKEN:+--auth-token "${AUTH_TOKEN}"}

# Check exit status
if [ $? -eq 0 ]; then
  echo
  echo -e "${GREEN}Demo data generation completed successfully!${NC}"
else
  echo
  echo -e "${RED}Demo data generation failed!${NC}"
  exit 1
fi