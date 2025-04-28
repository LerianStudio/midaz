#!/bin/bash

# Script to run Postman tests using Newman
# This script runs the Postman collection tests for the Midaz API

# Define paths
if [[ $PWD == */scripts ]]; then
  # Running from scripts directory
  MIDAZ_ROOT=$(cd .. && pwd)
else
  # Running from another directory
  MIDAZ_ROOT=$(pwd)
fi
POSTMAN_DIR="${MIDAZ_ROOT}/postman"
POSTMAN_COLLECTION="${POSTMAN_DIR}/MIDAZ.postman_collection.json"
POSTMAN_ENVIRONMENT="${POSTMAN_DIR}/MIDAZ.postman_environment.json"

# Verify files exist
if [ ! -f "${POSTMAN_COLLECTION}" ]; then
  echo "Error: Postman collection not found at ${POSTMAN_COLLECTION}"
  echo "Please run 'make sync-postman' first to generate the collection."
  exit 1
fi

if [ ! -f "${POSTMAN_ENVIRONMENT}" ]; then
  echo "Error: Postman environment not found at ${POSTMAN_ENVIRONMENT}"
  echo "Please run 'make sync-postman' first to generate the environment."
  exit 1
fi

# Check if folder is specified
if [ -n "$1" ]; then
  FOLDER="--folder \"$1\""
  echo "Running tests for folder: $1"
else
  FOLDER=""
  echo "Running all tests in the collection"
fi

# Install newman if not already installed
if ! command -v npx &> /dev/null; then
  echo "Error: npx is not available. Please install Node.js and npm first."
  exit 1
fi

# Run the tests
echo "Starting Postman tests..."
eval "npx newman run \"${POSTMAN_COLLECTION}\" -e \"${POSTMAN_ENVIRONMENT}\" ${FOLDER}"

# Check the result
EXIT_CODE=$?
if [ ${EXIT_CODE} -eq 0 ]; then
  echo "✅ Postman tests completed successfully!"
else
  echo "❌ Postman tests failed with exit code: ${EXIT_CODE}"
fi

exit ${EXIT_CODE}