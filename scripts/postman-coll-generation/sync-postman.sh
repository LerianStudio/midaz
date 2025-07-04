#!/bin/bash
set -e

# Script to sync Postman collection with OpenAPI documentation (OPTIMIZED)
# Performance improvements:
# 1. Parallel processing of components
# 2. Reduced jq invocations
# 3. Optimized JSON merging
# 4. Skip unnecessary dependency checks

# Define paths
MIDAZ_ROOT=$(pwd)
SCRIPTS_DIR="${MIDAZ_ROOT}/scripts/postman-coll-generation"
CONVERTER="${SCRIPTS_DIR}/convert-openapi.js"
POSTMAN_DIR="${MIDAZ_ROOT}/postman"
TEMP_DIR="${MIDAZ_ROOT}/postman/temp"
ONBOARDING_API="${MIDAZ_ROOT}/components/onboarding/api"
TRANSACTION_API="${MIDAZ_ROOT}/components/transaction/api"
POSTMAN_COLLECTION="${POSTMAN_DIR}/MIDAZ.postman_collection.json"
POSTMAN_ENVIRONMENT="${POSTMAN_DIR}/MIDAZ.postman_environment.json"
BACKUP_DIR="${POSTMAN_DIR}/backups"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Quick dependency check (no installation - that should be done in setup)
if ! command_exists node; then
    echo -e "${RED}Error: Node.js is required but not installed.${NC}"
    echo "Please install Node.js from https://nodejs.org/"
    exit 1
fi

if ! command_exists jq; then
    echo -e "${RED}Error: jq is required but not installed.${NC}"
    echo "Please install jq for your system"
    exit 1
fi

# Create necessary directories
mkdir -p "${TEMP_DIR}" "${BACKUP_DIR}"

# Backup existing files (if they exist)
if [ -f "${POSTMAN_COLLECTION}" ] || [ -f "${POSTMAN_ENVIRONMENT}" ]; then
    TIMESTAMP=$(date +"%Y%m%d%H%M%S")
    [ -f "${POSTMAN_COLLECTION}" ] && cp "${POSTMAN_COLLECTION}" "${BACKUP_DIR}/MIDAZ.postman_collection.${TIMESTAMP}.json"
    [ -f "${POSTMAN_ENVIRONMENT}" ] && cp "${POSTMAN_ENVIRONMENT}" "${BACKUP_DIR}/MIDAZ.postman_environment.${TIMESTAMP}.json"
fi

# Function to convert OpenAPI to Postman (runs in parallel)
convert_component() {
    local component=$1
    local input_file=$2
    local output_collection="${TEMP_DIR}/${component}.postman_collection.json"
    local output_env="${TEMP_DIR}/${component}.environment.json"
    local status_file="${TEMP_DIR}/${component}.status"
    
    {
        if [ -f "${input_file}" ]; then
            echo "Processing ${component}..." >&2
            if node "${CONVERTER}" "${input_file}" "${output_collection}" --env "${output_env}" 2>"${TEMP_DIR}/${component}.err"; then
                echo "SUCCESS" > "${status_file}"
            else
                echo "FAILED" > "${status_file}"
                cat "${TEMP_DIR}/${component}.err" >&2
            fi
        else
            echo "${component} API spec not found. Skipping..." >&2
            echo "SKIPPED" > "${status_file}"
        fi
    } &
}

echo "Converting OpenAPI specs to Postman collections..."

# Process components in parallel
convert_component "onboarding" "${ONBOARDING_API}/swagger.json"
ONBOARDING_PID=$!

convert_component "transaction" "${TRANSACTION_API}/swagger.json"
TRANSACTION_PID=$!

# Wait for both conversions to complete
wait $ONBOARDING_PID $TRANSACTION_PID

# Check conversion results
ONBOARDING_STATUS=$(cat "${TEMP_DIR}/onboarding.status" 2>/dev/null || echo "FAILED")
TRANSACTION_STATUS=$(cat "${TEMP_DIR}/transaction.status" 2>/dev/null || echo "FAILED")

# Function to merge collections efficiently
merge_collections() {
    local collection1=$1
    local collection2=$2
    local env1=$3
    local env2=$4
    
    # Merge collections and environments in a single jq invocation
    if [ -f "$collection1" ] && [ -f "$collection2" ]; then
        echo "Merging collections..."
        
        # Create merged collection with optimized jq script
        jq -s '
            # Start with first collection as base
            .[0] as $base |
            .[1] as $addition |
            
            # Merge and update in one pass
            $base |
            .item = ([$base.item[], $addition.item[]] | map(select(.name != "E2E Flow"))) |
            .variable = (
                if ($base.variable != null and $addition.variable != null) then
                    ($base.variable + $addition.variable | unique_by(.key))
                elif ($addition.variable != null) then
                    $addition.variable
                else
                    $base.variable
                end
            ) |
            .info.name = "MIDAZ" |
            .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"
        ' "$collection1" "$collection2" > "${POSTMAN_COLLECTION}"
        
        # Merge environments if both exist
        if [ -f "$env1" ] && [ -f "$env2" ]; then
            echo "Merging environment templates..."
            jq -s '
                # Start with first environment as base
                .[0] as $base |
                .[1] as $addition |
                
                $base |
                .name = "MIDAZ Environment" |
                .id = "midaz-environment-id" |
                .values = ($base.values + $addition.values | unique_by(.key))
            ' "$env1" "$env2" > "${POSTMAN_ENVIRONMENT}"
        elif [ -f "$env1" ]; then
            jq '.name = "MIDAZ Environment" | .id = "midaz-environment-id"' "$env1" > "${POSTMAN_ENVIRONMENT}"
        elif [ -f "$env2" ]; then
            jq '.name = "MIDAZ Environment" | .id = "midaz-environment-id"' "$env2" > "${POSTMAN_ENVIRONMENT}"
        fi
        
        return 0
    fi
    
    return 1
}

# Process based on what was successfully converted
ONBOARDING_COLLECTION="${TEMP_DIR}/onboarding.postman_collection.json"
TRANSACTION_COLLECTION="${TEMP_DIR}/transaction.postman_collection.json"
ONBOARDING_ENV="${TEMP_DIR}/onboarding.environment.json"
TRANSACTION_ENV="${TEMP_DIR}/transaction.environment.json"

if [ "$ONBOARDING_STATUS" = "SUCCESS" ] && [ "$TRANSACTION_STATUS" = "SUCCESS" ]; then
    merge_collections "$ONBOARDING_COLLECTION" "$TRANSACTION_COLLECTION" "$ONBOARDING_ENV" "$TRANSACTION_ENV"
elif [ "$ONBOARDING_STATUS" = "SUCCESS" ]; then
    echo "Only onboarding component found. Using it as the main collection..."
    jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' \
        "$ONBOARDING_COLLECTION" > "${POSTMAN_COLLECTION}"
    [ -f "$ONBOARDING_ENV" ] && \
        jq '.name = "MIDAZ Environment" | .id = "midaz-environment-id"' "$ONBOARDING_ENV" > "${POSTMAN_ENVIRONMENT}"
elif [ "$TRANSACTION_STATUS" = "SUCCESS" ]; then
    echo "Only transaction component found. Using it as the main collection..."
    jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' \
        "$TRANSACTION_COLLECTION" > "${POSTMAN_COLLECTION}"
    [ -f "$TRANSACTION_ENV" ] && \
        jq '.name = "MIDAZ Environment" | .id = "midaz-environment-id"' "$TRANSACTION_ENV" > "${POSTMAN_ENVIRONMENT}"
else
    echo -e "${RED}No OpenAPI specs were successfully converted.${NC}"
    rm -rf "${TEMP_DIR}"
    exit 1
fi

# Add workflow sequence (optimized to check dependencies first)
if [ -f "${POSTMAN_COLLECTION}" ] && [ -f "${MIDAZ_ROOT}/postman/WORKFLOW.md" ]; then
    echo "Adding workflow sequence to Postman collection..."
    
    # Check if uuid is available in node_modules (should be from npm install)
    if [ -d "${SCRIPTS_DIR}/node_modules/uuid" ]; then
        if node "${SCRIPTS_DIR}/create-workflow.js" \
            "${POSTMAN_COLLECTION}" \
            "${MIDAZ_ROOT}/postman/WORKFLOW.md" \
            "${POSTMAN_COLLECTION}" 2>"${TEMP_DIR}/workflow.err"; then
            echo -e "${GREEN}✓ Workflow sequence added${NC}"
        else
            echo -e "${YELLOW}⚠ Failed to add workflow sequence${NC}"
            [ -s "${TEMP_DIR}/workflow.err" ] && cat "${TEMP_DIR}/workflow.err" >&2
        fi
    else
        echo -e "${YELLOW}⚠ uuid package not found. Skipping workflow addition.${NC}"
    fi
fi

# Clean up
echo "Cleaning up temporary files..."
rm -rf "${TEMP_DIR}"

echo -e "${GREEN}✓ Postman collection sync completed successfully${NC}"
echo "Collection: ${POSTMAN_COLLECTION}"
[ -f "${POSTMAN_ENVIRONMENT}" ] && echo "Environment: ${POSTMAN_ENVIRONMENT}"

exit 0