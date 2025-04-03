#!/bin/bash

# Script to sync Postman collection with OpenAPI documentation
# This script uses a custom Node.js converter to convert OpenAPI specs to Postman collections

# Check if Node.js is installed
if ! command -v node &> /dev/null; then
    echo "Error: Node.js is not installed. Please install Node.js to use this script."
    echo "Visit https://nodejs.org/ to download and install Node.js"
    exit 1
fi

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    echo "Error: jq is not installed. Please install jq to use this script."
    echo "Run: brew install jq"
    exit 1
fi

# Define paths
MIDAZ_ROOT=$(pwd)
POSTMAN_DIR="${MIDAZ_ROOT}/postman"
TEMP_DIR="${MIDAZ_ROOT}/postman/temp"
ONBOARDING_API="${MIDAZ_ROOT}/components/onboarding/api"
TRANSACTION_API="${MIDAZ_ROOT}/components/transaction/api"
POSTMAN_COLLECTION="${POSTMAN_DIR}/MIDAZ.postman_collection.json"
BACKUP_DIR="${POSTMAN_DIR}/backups"
CONVERTER_SCRIPT="${MIDAZ_ROOT}/scripts/convert-openapi.js"

# Create necessary directories
mkdir -p "${TEMP_DIR}"
mkdir -p "${BACKUP_DIR}"

# Backup existing Postman collection
TIMESTAMP=$(date +"%Y%m%d%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/MIDAZ.postman_collection..json"
if [ -f "${POSTMAN_COLLECTION}" ]; then
    echo "Backing up existing Postman collection to ${BACKUP_FILE}..."
    cp "${POSTMAN_COLLECTION}" "${BACKUP_FILE}"
fi

# Convert OpenAPI specs to Postman collections
echo "Converting OpenAPI specs to Postman collections..."

# Process onboarding component
if [ -f "${ONBOARDING_API}/swagger.json" ]; then
    echo "Processing onboarding component..."
    node "${CONVERTER_SCRIPT}" "${ONBOARDING_API}/swagger.json" "${TEMP_DIR}/onboarding.postman_collection.json"
    if [ $? -ne 0 ]; then
        echo "Failed to convert onboarding API spec to Postman collection."
        echo "Continuing with other components..."
    fi
else
    echo "Onboarding API spec not found. Skipping..."
fi

# Process transaction component
if [ -f "${TRANSACTION_API}/swagger.json" ]; then
    echo "Processing transaction component..."
    node "${CONVERTER_SCRIPT}" "${TRANSACTION_API}/swagger.json" "${TEMP_DIR}/transaction.postman_collection.json"
    if [ $? -ne 0 ]; then
        echo "Failed to convert transaction API spec to Postman collection."
        echo "Continuing with other components..."
    fi
else
    echo "Transaction API spec not found. Skipping..."
fi

# Merge collections if needed
if [ -f "${TEMP_DIR}/onboarding.postman_collection.json" ] && [ -f "${TEMP_DIR}/transaction.postman_collection.json" ]; then
    echo "Merging collections..."
    
    # Create a new merged collection
    # This is a simple approach - for more complex merging, consider using a dedicated tool like postman-collection-merger
    jq -s '.[0].item = (.[0].item + .[1].item) | .[0]' \
        "${TEMP_DIR}/onboarding.postman_collection.json" \
        "${TEMP_DIR}/transaction.postman_collection.json" > "${TEMP_DIR}/merged.postman_collection.json"
    
    # Update the collection name and description
    jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' \
        "${TEMP_DIR}/merged.postman_collection.json" > "${TEMP_DIR}/MIDAZ.postman_collection.json"
    
    # Copy the merged collection to the final location
    cp "${TEMP_DIR}/MIDAZ.postman_collection.json" "${POSTMAN_COLLECTION}"
    
elif [ -f "${TEMP_DIR}/onboarding.postman_collection.json" ]; then
    echo "Only onboarding component found. Using it as the main collection..."
    jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' \
        "${TEMP_DIR}/onboarding.postman_collection.json" > "${POSTMAN_COLLECTION}"
    
elif [ -f "${TEMP_DIR}/transaction.postman_collection.json" ]; then
    echo "Only transaction component found. Using it as the main collection..."
    jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' \
        "${TEMP_DIR}/transaction.postman_collection.json" > "${POSTMAN_COLLECTION}"
    
else
    echo "No OpenAPI specs found. Make sure to generate the documentation first using 'make generate-docs-all'."
    exit 1
fi

# Clean up temporary files
echo "Cleaning up temporary files..."
rm -rf "${TEMP_DIR}"

echo "[ok] Postman collection synced successfully with OpenAPI documentation ✔️"
echo "Note: The synced collection is available at ${POSTMAN_COLLECTION}"
echo "A backup of the previous collection is available at ${BACKUP_FILE}"
