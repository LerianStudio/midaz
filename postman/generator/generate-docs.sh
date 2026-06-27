#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

# Clean documentation generation script
# Abstracts swag complexity and provides beautiful output

# Root directory of the repo (this script lives in postman/generator/)
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Generator tooling and published-spec directories
GENERATOR_DIR="${ROOT_DIR}/postman/generator"
SPECS_DIR="${ROOT_DIR}/postman/specs"

# Components to process (each must have a cmd/app/main.go entry point)
COMPONENTS=("ledger" "tracer")

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

# Resolve swag even when GOPATH/bin is not on PATH.
resolve_swag_bin() {
    if command -v swag >/dev/null 2>&1; then
        command -v swag
        return 0
    fi

    local go_bin=""
    go_bin="$(go env GOBIN 2>/dev/null || true)"

    if [ -z "${go_bin}" ]; then
        local go_path=""
        go_path="$(go env GOPATH 2>/dev/null || true)"
        if [ -n "${go_path}" ]; then
            go_bin="${go_path}/bin"
        fi
    fi

    if [ -n "${go_bin}" ] && [ -x "${go_bin}/swag" ]; then
        printf '%s\n' "${go_bin}/swag"
        return 0
    fi

    return 1
}

SWAG_BIN=""

# Generate OpenAPI specs for a component
generate_openapi_spec() {
    local component="$1"
    local component_dir="${ROOT_DIR}/components/${component}"
    local start_time=$(date +%s.%N)

    print_step "Generating ${component} OpenAPI spec" "PROCESSING"

    # Redirect all swag output to log files
    local out_log="${LOG_DIR}/${component}_swag.out"
    local err_log="${LOG_DIR}/${component}_swag.err"

    local swag_args=(init -g cmd/app/main.go -o api --parseDependency --parseInternal --outputTypes go,json,yaml)

    if (cd "${component_dir}" && "${SWAG_BIN}" "${swag_args[@]}" > "${out_log}" 2> "${err_log}"); then
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
    local swagger_file="swagger.json"

    if (cd "${component_dir}" && \
        docker run --rm -v ./:/workspace --user "$(id -u):$(id -g)" \
            openapitools/openapi-generator-cli:v7.10.0 generate \
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

# Copy generated spec artifacts into postman/specs/<component>/ for the hub
publish_specs() {
    local component="$1"
    local api_dir="${ROOT_DIR}/components/${component}/api"
    local dest_dir="${SPECS_DIR}/${component}"

    print_step "Publishing ${component} specs to postman/specs" "PROCESSING"

    if mkdir -p "${dest_dir}" && \
        cp "${api_dir}/swagger.json" "${api_dir}/swagger.yaml" "${api_dir}/openapi.yaml" "${dest_dir}/"; then
        print_step "Published ${component} specs to postman/specs" "SUCCESS"
        return 0
    else
        print_step "Publish ${component} specs to postman/specs" "FAILED"
        return 1
    fi
}

# Merge the three per-component openapi.yaml specs into one consolidated spec
# (postman/specs/midaz.openapi.{yaml,json}) via @redocly/cli join. Ledger is
# listed first so it acts as the "main" and takes precedence on shared metadata.
consolidate_openapi() {
    print_step "Consolidating OpenAPI specs" "PROCESSING"

    local out_log="${LOG_DIR}/consolidate.out"
    local err_log="${LOG_DIR}/consolidate.err"
    local start_time=$(date +%s.%N)

    local redocly_bin="${GENERATOR_DIR}/node_modules/.bin/redocly"
    local consolidated_yaml="${SPECS_DIR}/midaz.openapi.yaml"
    local consolidated_json="${SPECS_DIR}/midaz.openapi.json"

    if [ ! -x "${redocly_bin}" ]; then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        @redocly/cli not found at ${redocly_bin}; run install_npm_dependencies first."
        return 1
    fi

    # 1. Assert all three component specs declare the same openapi: version.
    local ref_version="" version=""
    for component in "${COMPONENTS[@]}"; do
        local spec="${ROOT_DIR}/components/${component}/api/openapi.yaml"
        if [ ! -f "${spec}" ]; then
            print_step "Consolidate OpenAPI specs" "FAILED"
            echo -e "      ${RED}Error details:${NC}"
            echo "        Missing component spec: ${spec}"
            return 1
        fi
        version="$(awk '/^openapi:/ {print $2; exit}' "${spec}" | tr -d '"'"'"\\r)"
        if [ -z "${ref_version}" ]; then
            ref_version="${version}"
        elif [ "${version}" != "${ref_version}" ]; then
            print_step "Consolidate OpenAPI specs" "FAILED"
            echo -e "      ${RED}Error details:${NC}"
            echo "        openapi version mismatch: ${component} is '${version}', expected '${ref_version}'."
            echo "        All component specs must share one openapi version before join."
            return 1
        fi
    done

    # 2. Join (ledger first => takes precedence). Run the locally-installed binary
    #    directly so the component paths stay relative to ROOT_DIR.
    if ! (cd "${ROOT_DIR}" && "${redocly_bin}" join \
            components/ledger/api/openapi.yaml \
            components/tracer/api/openapi.yaml \
            --prefix-tags-with-info-prop title \
            -o postman/specs/midaz.openapi.yaml > "${out_log}" 2> "${err_log}"); then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${err_log}" | sed 's/^/        /'
        return 1
    fi

    # 3. Produce a deterministic JSON twin from the YAML via the bundled js-yaml.
    if ! (cd "${ROOT_DIR}" && NODE_PATH="${GENERATOR_DIR}/node_modules" node -e '
        const yaml = require("js-yaml");
        const fs = require("fs");
        const doc = yaml.load(fs.readFileSync("postman/specs/midaz.openapi.yaml", "utf8"));
        fs.writeFileSync("postman/specs/midaz.openapi.json", JSON.stringify(doc, null, 2) + "\n");
    ' >> "${out_log}" 2>> "${err_log}"); then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${err_log}" | sed 's/^/        /'
        return 1
    fi

    # 4. Security post-validation against the JSON twin. redocly join's security
    #    merge is undocumented and root security may be dropped (known issue), so
    #    this guard catches a regression where a scheme goes missing or an
    #    operation references a scheme that is not defined.
    local missing
    missing="$(jq -r '
        ["BearerAuth","ApiKeyAuth"]
        - (.components.securitySchemes // {} | keys)
        | join(", ")
    ' "${consolidated_json}")"
    if [ -n "${missing}" ]; then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        Consolidated spec is missing required securityScheme(s): ${missing}."
        echo "        Expected both BearerAuth (ledger) and ApiKeyAuth (tracer)."
        return 1
    fi

    local orphans
    orphans="$(jq -r '
        (.components.securitySchemes // {} | keys) as $defined
        | [ .paths | to_entries[] | .value | to_entries[]
            | select(.key | test("^(get|post|put|patch|delete|head|options)$"))
            | (.value.security // [])[] | keys[] ]
        | unique
        | map(select(. as $s | ($defined | index($s)) | not))
        | join(", ")
    ' "${consolidated_json}")"
    if [ -n "${orphans}" ]; then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        Consolidated spec has operations referencing undefined securityScheme(s): ${orphans}."
        return 1
    fi

    local end_time=$(date +%s.%N)
    local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
    print_step "Consolidated OpenAPI specs (openapi ${ref_version})" "SUCCESS" "${elapsed}"
    return 0
}

# Install Node.js dependencies for Postman generation
install_npm_dependencies() {
    print_step "Installing Node.js dependencies" "PROCESSING"

    local npm_out="${LOG_DIR}/npm.out"
    local npm_err="${LOG_DIR}/npm.err"
    local start_time=$(date +%s.%N)
    local postman_dir="${GENERATOR_DIR}"
    
    # Check if node_modules exists and package.json hasn't changed
    if [ -d "${postman_dir}/node_modules" ] && [ "${postman_dir}/node_modules" -nt "${postman_dir}/package.json" ]; then
        print_step "Node.js dependencies already up to date" "SUCCESS" "0.0"
        return 0
    fi
    
    if [ -f "${postman_dir}/package-lock.json" ]; then
        if (cd "${postman_dir}" && npm ci --silent > "${npm_out}" 2> "${npm_err}"); then
            local end_time=$(date +%s.%N)
            local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
            print_step "Installed Node.js dependencies" "SUCCESS" "${elapsed}"
            return 0
        fi
    elif (cd "${postman_dir}" && npm install --silent > "${npm_out}" 2> "${npm_err}"); then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Installed Node.js dependencies" "SUCCESS" "${elapsed}"
        return 0
    fi

    print_step "Install Node.js dependencies" "FAILED"
    echo -e "      ${RED}Error details:${NC}"
    head -5 "${npm_err}" | sed 's/^/        /'
    return 1
}

# Convert to Postman collection
convert_to_postman() {
    print_step "Converting to Postman collection" "PROCESSING"
    
    local sync_out="${LOG_DIR}/sync.out"
    local sync_err="${LOG_DIR}/sync.err"
    local start_time=$(date +%s.%N)
    
    if "${GENERATOR_DIR}/sync-postman.sh" > "${sync_out}" 2> "${sync_err}"; then
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

    if ! SWAG_BIN="$(resolve_swag_bin)"; then
        print_step "Resolve swag executable" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        swag was not found on PATH or in Go's bin directory."
        echo "        Install it with: go install github.com/swaggo/swag/cmd/swag@v1.16.6"
        overall_success=false
    fi
    
    # Generate OpenAPI specs for each component
    if [ "$overall_success" = true ]; then
        for component in "${COMPONENTS[@]}"; do
            if ! generate_openapi_spec "$component"; then
                overall_success=false
                break
            fi
        done
    fi

    # Generate openapi.yaml for each component
    if [ "$overall_success" = true ]; then
        for component in "${COMPONENTS[@]}"; do
            if ! generate_openapi_yaml "$component"; then
                overall_success=false
                break
            fi
        done
    fi

    # Publish spec artifacts into postman/specs/<component>/ (copies; the api/
    # directory stays the swag output home that Go and the contract test import)
    if [ "$overall_success" = true ]; then
        for component in "${COMPONENTS[@]}"; do
            if ! publish_specs "$component"; then
                overall_success=false
                break
            fi
        done
    fi

    # If OpenAPI generation succeeded, install dependencies, consolidate the
    # per-component specs into one, then convert to Postman.
    if [ "$overall_success" = true ]; then
        if ! install_npm_dependencies; then
            overall_success=false
        elif ! consolidate_openapi; then
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
        echo -e "   📚 Consolidated spec: postman/specs/midaz.openapi.yaml"
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
