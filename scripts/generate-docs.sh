#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

# Clean documentation generation script
# Abstracts swag complexity and provides beautiful output

# Root directory of the repo
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Components to process
COMPONENTS=("onboarding" "transaction" "crm")

# Temporary log dir
LOG_DIR="${ROOT_DIR}/tmp"
mkdir -p "${LOG_DIR}"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print a nice header
print_header() {
    echo ""
    echo -e "${BLUE}=================================================${NC}"
    echo -e "${BLUE}  📝 $1${NC}"
    echo -e "${BLUE}=================================================${NC}"
    echo ""
}

# Print step with status
print_step() {
    local step_name="$1"
    local status="$2"
    local time_taken="${3:-}"
    
    if [ "$status" = "SUCCESS" ]; then
        echo -e "    ${GREEN}✅ ${step_name}${time_taken:+ (${time_taken}s)}${NC}"
    elif [ "$status" = "FAILED" ]; then
        echo -e "    ${RED}❌ ${step_name} - FAILED${NC}"
    else
        echo -e "    ${YELLOW}⏳ ${step_name}...${NC}"
    fi
}

# Generate OpenAPI specs for a component
generate_openapi_spec() {
    local component="$1"
    local component_dir="${ROOT_DIR}/components/${component}"
    local start_time=$(date +%s.%N)

    print_step "Generating ${component} OpenAPI spec" "PROCESSING"

    # Redirect all swag output to log files
    local out_log="${LOG_DIR}/${component}_swag.out"
    local err_log="${LOG_DIR}/${component}_swag.err"

    if (cd "${component_dir}" && swag init -g cmd/app/main.go -o api --parseDependency --parseInternal --instanceName "${component}" > "${out_log}" 2> "${err_log}"); then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Generated ${component} OpenAPI spec" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Generate ${component} OpenAPI spec" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${err_log}" | sed 's/^/        /'
        return 1
    fi
}

# Generate openapi.yaml from swagger JSON using Docker
generate_openapi_yaml() {
    local component="$1"
    local component_dir="${ROOT_DIR}/components/${component}"
    local start_time=$(date +%s.%N)

    print_step "Generating ${component} openapi.yaml" "PROCESSING"

    local out_log="${LOG_DIR}/${component}_openapi.out"
    local err_log="${LOG_DIR}/${component}_openapi.err"
    local swagger_file="${component}_swagger.json"

    if (cd "${component_dir}" && \
        docker run --rm -v ./:/workspace --user "$(id -u):$(id -g)" \
            openapitools/openapi-generator-cli:v5.1.1 generate \
            -i "/workspace/api/${swagger_file}" \
            -g openapi-yaml \
            -o /workspace/api > "${out_log}" 2> "${err_log}" && \
        mv api/openapi/openapi.yaml api/openapi.yaml && \
        rm -rf api/README.md api/.openapi-generator* api/openapi); then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Generated ${component} openapi.yaml" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Generate ${component} openapi.yaml" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${err_log}" | sed 's/^/        /'
        return 1
    fi
}

# Install Node.js dependencies for Postman generation
install_npm_dependencies() {
    print_step "Installing Node.js dependencies" "PROCESSING"
    
    local npm_out="${LOG_DIR}/npm.out"
    local npm_err="${LOG_DIR}/npm.err"
    local start_time=$(date +%s.%N)
    local postman_dir="${ROOT_DIR}/scripts/postman-coll-generation"
    
    # Check if node_modules exists and package.json hasn't changed
    if [ -d "${postman_dir}/node_modules" ] && [ "${postman_dir}/node_modules" -nt "${postman_dir}/package.json" ]; then
        print_step "Node.js dependencies already up to date" "SUCCESS" "0.0"
        return 0
    fi
    
    if (cd "${postman_dir}" && npm install --silent > "${npm_out}" 2> "${npm_err}"); then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Installed Node.js dependencies" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Install Node.js dependencies" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${npm_err}" | sed 's/^/        /'
        return 1
    fi
}

# Convert to Postman collection
convert_to_postman() {
    print_step "Converting to Postman collection" "PROCESSING"
    
    local sync_out="${LOG_DIR}/sync.out"
    local sync_err="${LOG_DIR}/sync.err"
    local start_time=$(date +%s.%N)
    
    if "${ROOT_DIR}/scripts/postman-coll-generation/sync-postman.sh" > "${sync_out}" 2> "${sync_err}"; then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Converted to Postman collection" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Convert to Postman collection" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${sync_err}" | sed 's/^/        /'
        return 1
    fi
}

# Verify outputs
verify_outputs() {
    print_step "Verifying generated files" "PROCESSING"
    
    local collection_file="${ROOT_DIR}/postman/MIDAZ.postman_collection.json"
    local environment_file="${ROOT_DIR}/postman/MIDAZ.postman_environment.json"
    
    if [ -f "${collection_file}" ] && [ -f "${environment_file}" ]; then
        # Check if collection has content
        local request_count=$(jq '.item | length' "${collection_file}" 2>/dev/null || echo "0")
        local env_vars_count=$(jq '.values | length' "${environment_file}" 2>/dev/null || echo "0")
        
        print_step "Generated collection with ${request_count} folders and ${env_vars_count} environment variables" "SUCCESS"
        return 0
    else
        print_step "Verify generated files" "FAILED"
        return 1
    fi
}

# Main execution
main() {
    print_header "Generating Swagger API Documentation"
    
    # Track overall success
    local overall_success=true
    
    # Generate OpenAPI specs for each component
    for component in "${COMPONENTS[@]}"; do
        if ! generate_openapi_spec "$component"; then
            overall_success=false
            break
        fi
    done

    # Generate openapi.yaml for each component
    if [ "$overall_success" = true ]; then
        for component in "${COMPONENTS[@]}"; do
            if ! generate_openapi_yaml "$component"; then
                overall_success=false
                break
            fi
        done
    fi

    # Generate ledger unified swagger (merges onboarding + transaction + ledger settings)
    if [ "$overall_success" = true ]; then
        if [ -f "${ROOT_DIR}/components/ledger/scripts/merge-swagger.sh" ]; then
            print_step "Generating ledger unified swagger" "PROCESSING"
            local ledger_out="${LOG_DIR}/ledger_merge.out"
            local ledger_err="${LOG_DIR}/ledger_merge.err"
            local start_time=$(date +%s.%N)

            if "${ROOT_DIR}/components/ledger/scripts/merge-swagger.sh" > "${ledger_out}" 2> "${ledger_err}"; then
                local end_time=$(date +%s.%N)
                local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
                print_step "Generated ledger unified swagger" "SUCCESS" "${elapsed}"
            else
                print_step "Generate ledger unified swagger" "FAILED"
                echo -e "      ${RED}Error details:${NC}"
                head -5 "${ledger_err}" | sed 's/^/        /'
                overall_success=false
            fi
        fi
    fi

    # If OpenAPI generation succeeded, install dependencies and convert to Postman
    if [ "$overall_success" = true ]; then
        if ! install_npm_dependencies; then
            overall_success=false
        elif ! convert_to_postman; then
            overall_success=false
        fi
    fi
    
    # Verify outputs
    if [ "$overall_success" = true ]; then
        if ! verify_outputs; then
            overall_success=false
        fi
    fi
    
    # Final status
    echo ""
    if [ "$overall_success" = true ]; then
        echo -e "${GREEN}🎉 Documentation generation completed successfully!${NC}"
        echo -e "   📄 Collection: postman/MIDAZ.postman_collection.json"
        echo -e "   🌍 Environment: postman/MIDAZ.postman_environment.json"
    else
        echo -e "${RED}❌ Documentation generation failed.${NC}"
        echo -e "   📋 Check logs in: ${LOG_DIR}/"
        exit 1
    fi
    
    # Clean up temporary logs on success
    if [ "$overall_success" = true ]; then
        rm -rf "${LOG_DIR}"
    fi
    
    echo ""
}

# Run main function
main "$@"