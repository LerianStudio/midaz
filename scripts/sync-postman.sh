#!/bin/bash

# Script to sync Postman collection with OpenAPI documentation
# This script uses openapi-to-postman to convert OpenAPI specs to Postman collections

# Include color definitions
source "$(dirname "$0")/../pkg/shell/colors.sh"

# Check if openapi-to-postman is installed
if ! command -v openapi-to-postman &> /dev/null; then
    echo -e "${YELLOW}openapi-to-postman is not installed. Installing...${NC}"
    npm install -g openapi-to-postman
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to install openapi-to-postman. Please install it manually:${NC}"
        echo -e "${MAGENTA}npm install -g openapi-to-postman${NC}"
        exit 1
    fi
fi

# Define paths
MIDAZ_ROOT=$(pwd)
POSTMAN_DIR="${MIDAZ_ROOT}/postman"
TEMP_DIR="${MIDAZ_ROOT}/postman/temp"
ONBOARDING_API="${MIDAZ_ROOT}/components/onboarding/api"
TRANSACTION_API="${MIDAZ_ROOT}/components/transaction/api"
POSTMAN_COLLECTION="${POSTMAN_DIR}/MIDAZ.postman_collection.json"
BACKUP_DIR="${POSTMAN_DIR}/backups"

# Create necessary directories
mkdir -p "${TEMP_DIR}"
mkdir -p "${BACKUP_DIR}"

# Backup existing Postman collection
TIMESTAMP=$(date +"%Y%m%d%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/MIDAZ.postman_collection.${TIMESTAMP}.json"
if [ -f "${POSTMAN_COLLECTION}" ]; then
    echo -e "${CYAN}Backing up existing Postman collection to ${BACKUP_FILE}...${NC}"
    cp "${POSTMAN_COLLECTION}" "${BACKUP_FILE}"
fi

# Convert OpenAPI specs to Postman collections
echo -e "${CYAN}Converting OpenAPI specs to Postman collections...${NC}"

# Process onboarding component
if [ -f "${ONBOARDING_API}/swagger.json" ]; then
    echo -e "${CYAN}Processing onboarding component...${NC}"
    openapi-to-postman -s "${ONBOARDING_API}/swagger.json" -o "${TEMP_DIR}/onboarding.postman_collection.json" -p -t json
else
    echo -e "${YELLOW}Onboarding API spec not found. Skipping...${NC}"
fi

# Process transaction component
if [ -f "${TRANSACTION_API}/swagger.json" ]; then
    echo -e "${CYAN}Processing transaction component...${NC}"
    openapi-to-postman -s "${TRANSACTION_API}/swagger.json" -o "${TEMP_DIR}/transaction.postman_collection.json" -p -t json
else
    echo -e "${YELLOW}Transaction API spec not found. Skipping...${NC}"
fi

# Merge collections if needed
if [ -f "${TEMP_DIR}/onboarding.postman_collection.json" ] && [ -f "${TEMP_DIR}/transaction.postman_collection.json" ]; then
    echo -e "${CYAN}Merging collections...${NC}"
    
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
    echo -e "${CYAN}Only onboarding component found. Using it as the main collection...${NC}"
    jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' \
        "${TEMP_DIR}/onboarding.postman_collection.json" > "${POSTMAN_COLLECTION}"
    
elif [ -f "${TEMP_DIR}/transaction.postman_collection.json" ]; then
    echo -e "${CYAN}Only transaction component found. Using it as the main collection...${NC}"
    jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' \
        "${TEMP_DIR}/transaction.postman_collection.json" > "${POSTMAN_COLLECTION}"
    
else
    echo -e "${RED}No OpenAPI specs found. Make sure to generate the documentation first using 'make generate-docs-all'.${NC}"
    exit 1
fi

# Clean up temporary files
echo -e "${CYAN}Cleaning up temporary files...${NC}"
rm -rf "${TEMP_DIR}"

echo -e "${GREEN}${BOLD}[ok]${NC} Postman collection synced successfully with OpenAPI documentation${GREEN} ✔️${NC}"
echo -e "${YELLOW}Note: The synced collection is available at ${POSTMAN_COLLECTION}${NC}"
echo -e "${YELLOW}A backup of the previous collection is available at ${BACKUP_FILE}${NC}"
