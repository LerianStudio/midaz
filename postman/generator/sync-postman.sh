#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -e

# Script to sync Postman collection with OpenAPI documentation (OPTIMIZED)
# Performance improvements:
# 1. Parallel processing of components
# 2. Reduced jq invocations
# 3. Optimized JSON merging
# 4. Skip unnecessary dependency checks

# Define paths (this script lives in postman/generator/)
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIDAZ_ROOT="$(cd "${SCRIPTS_DIR}/../.." && pwd)"
CONVERTER="${SCRIPTS_DIR}/convert-openapi.js"
POSTMAN_DIR="${MIDAZ_ROOT}/postman"
TEMP_DIR="${POSTMAN_DIR}/temp"
SPECS_DIR="${POSTMAN_DIR}/specs"
POSTMAN_COLLECTION="${POSTMAN_DIR}/MIDAZ.postman_collection.json"
POSTMAN_ENVIRONMENT="${POSTMAN_DIR}/MIDAZ.postman_environment.json"
BACKUP_DIR="${POSTMAN_DIR}/backups"

# Components merged into the unified MIDAZ collection (ledger is primary)
COMPONENTS=("ledger" "tracer" "reporter")

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
            if node "${CONVERTER}" "${input_file}" "${output_collection}" --env "${output_env}" --component "${component}" 2>"${TEMP_DIR}/${component}.err"; then
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

# Process all components in parallel from their published specs
declare -a CONVERT_PIDS=()
for component in "${COMPONENTS[@]}"; do
    convert_component "${component}" "${SPECS_DIR}/${component}/swagger.json"
    CONVERT_PIDS+=("$!")
done

# Wait for all conversions to complete
set +e
for pid in "${CONVERT_PIDS[@]}"; do
    wait "${pid}"
done
set -e

# Check ledger conversion result (ledger is the required primary collection)
LEDGER_STATUS=$(cat "${TEMP_DIR}/ledger.status" 2>/dev/null || echo "FAILED")

# Function to merge multiple collections efficiently
merge_all_collections() {
    local -a collections=()
    local -a environments=()

    # Collect all successful collections and environments
    for component in "${COMPONENTS[@]}"; do
        local status=$(cat "${TEMP_DIR}/${component}.status" 2>/dev/null || echo "FAILED")
        if [ "$status" != "SUCCESS" ]; then
            echo "Skipping ${component}: conversion status is ${status}"
            continue
        fi

        local coll="${TEMP_DIR}/${component}.postman_collection.json"
        local env="${TEMP_DIR}/${component}.environment.json"
        if [ -f "$coll" ]; then
            collections+=("$coll")
        fi
        if [ -f "$env" ]; then
            environments+=("$env")
        fi
    done

    local num_collections=${#collections[@]}

    if [ "$num_collections" -eq 0 ]; then
        return 1
    elif [ "$num_collections" -eq 1 ]; then
        echo "Single collection found. Using it as the main collection..."
        jq '.info.name = "MIDAZ" | .info._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"' \
            "${collections[0]}" > "${POSTMAN_COLLECTION}"
    else
        echo "Merging ${num_collections} collections..."
        # Merge all collections using jq slurp
        jq -s '
            # Combine all items from all collections, excluding E2E Flow
            reduce .[] as $coll (
                {
                    info: (.[0].info | .name = "MIDAZ" | ._postman_id = "00b3869d-895d-49b2-a6b5-68b193471560"),
                    item: [],
                    variable: []
                };
                .item += ($coll.item // [] | map(select(.name != "E2E Flow"))) |
                .variable += ($coll.variable // [])
            ) |
            .variable = (.variable | unique_by(.key))
        ' "${collections[@]}" > "${POSTMAN_COLLECTION}"
    fi

    # Merge environments
    local num_environments=${#environments[@]}
    if [ "$num_environments" -eq 1 ]; then
        jq '.name = "MIDAZ Environment" | .id = "midaz-environment-id"' \
            "${environments[0]}" > "${POSTMAN_ENVIRONMENT}"
    elif [ "$num_environments" -gt 1 ]; then
        echo "Merging ${num_environments} environment templates..."
        jq -s '
            reduce .[] as $env (
                {
                    name: "MIDAZ Environment",
                    id: "midaz-environment-id",
                    values: []
                };
                .values += ($env.values // [])
            ) |
            .values = (.values | unique_by(.key))
        ' "${environments[@]}" > "${POSTMAN_ENVIRONMENT}"
    fi

    return 0
}

# Process based on what was successfully converted
# Require the ledger conversion to succeed
if [ "$LEDGER_STATUS" != "SUCCESS" ]; then
	echo -e "${RED}OpenAPI conversion failed (ledger=${LEDGER_STATUS}).${NC}"
	rm -rf "${TEMP_DIR}"
	exit 1
fi

# Merge all successful collections
if ! merge_all_collections; then
    echo -e "${RED}Failed to merge collections.${NC}"
    rm -rf "${TEMP_DIR}"
    exit 1
fi

# Add workflow sequence (optimized to check dependencies first)
if [ -f "${POSTMAN_COLLECTION}" ] && [ -f "${MIDAZ_ROOT}/postman/WORKFLOW.md" ]; then
    echo "Adding workflow sequence to Postman collection..."
    
    # Check if uuid is available in node_modules (should be from npm install)
    if [ -d "${SCRIPTS_DIR}/node_modules/uuid" ]; then
        # Use the new workflow generator
        WORKFLOW_SCRIPT="${SCRIPTS_DIR}/create-workflow.js"
        echo "Adding workflow sequence with new generator..."
        
        if node "${WORKFLOW_SCRIPT}" \
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
