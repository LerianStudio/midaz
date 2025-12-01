#!/usr/bin/env bash
# shellcheck disable=SC2086  # Intentional word splitting for command arguments
# shellcheck disable=SC2155  # Declare and assign separately to avoid masking return values
set -euo pipefail

# Unified OpenAPI Spec Generator
# Converts Swagger 2.0 specs to OpenAPI 3.0 and merges them into a single spec
#
# Environment variables:
#   OPENAPI_GENERATOR_VERSION - OpenAPI Generator version (default: v7.10.0)

# Root directory of the midaz app
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Docker image version pinning
# Using tag-based version for easier updates. For maximum security and reproducibility,
# consider pinning to SHA256 digest:
# openapitools/openapi-generator-cli@sha256:<digest>
#
# To get digest: docker pull openapitools/openapi-generator-cli:v7.10.0
#                docker inspect --format='{{.RepoDigests}}' openapitools/openapi-generator-cli:v7.10.0
OPENAPI_GENERATOR_VERSION="${OPENAPI_GENERATOR_VERSION:-v7.10.0}"

# Component directories
ONBOARDING_API="${ROOT_DIR}/components/onboarding/api"
TRANSACTION_API="${ROOT_DIR}/components/transaction/api"
OUTPUT_DIR="${ROOT_DIR}/api"
SCRIPTS_DIR="${ROOT_DIR}/scripts/postman-coll-generation"

# Temporary log dir
LOG_DIR="${ROOT_DIR}/tmp"
mkdir -p "${LOG_DIR}"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print step with status
print_step() {
    local step_name="$1"
    local status="$2"
    local time_taken="${3:-}"

    if [ "$status" = "SUCCESS" ]; then
        echo -e "    ${GREEN}[ok] ${step_name}${time_taken:+ (${time_taken}s)}${NC}"
    elif [ "$status" = "FAILED" ]; then
        echo -e "    ${RED}[error] ${step_name} - FAILED${NC}"
    else
        echo -e "    ${YELLOW}[...] ${step_name}...${NC}"
    fi
}

# Check if required tools are available
check_dependencies() {
    local missing=false

    if ! command -v node >/dev/null 2>&1; then
        echo -e "${RED}Error: Node.js is required but not installed.${NC}"
        echo "Please install Node.js from https://nodejs.org/"
        missing=true
    fi

    if ! command -v docker >/dev/null 2>&1; then
        echo -e "${RED}Error: Docker is required for OpenAPI conversion.${NC}"
        echo "Please install Docker from https://docs.docker.com/get-docker/"
        missing=true
    fi

    if [ "$missing" = true ]; then
        exit 1
    fi
}

# Ensure output directory exists
ensure_output_dir() {
    mkdir -p "${OUTPUT_DIR}"
}

# Install npm dependencies if needed
install_dependencies() {
    print_step "Checking npm dependencies" "PROCESSING"

    local npm_out="${LOG_DIR}/npm_merge.out"
    local npm_err="${LOG_DIR}/npm_merge.err"
    local start_time=$(date +%s.%N)

    # Check if openapi-merge-cli is available
    if [ -d "${SCRIPTS_DIR}/node_modules/openapi-merge-cli" ]; then
        print_step "npm dependencies already installed" "SUCCESS" "0.0"
        return 0
    fi

    if (cd "${SCRIPTS_DIR}" && npm install --silent > "${npm_out}" 2> "${npm_err}"); then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Installed npm dependencies" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Install npm dependencies" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${npm_err}" 2>/dev/null | sed 's/^/        /' || true
        return 1
    fi
}

# Convert Swagger 2.0 to OpenAPI 3.0 using Docker
convert_swagger_to_openapi() {
    local component="$1"
    local input_file="$2"
    local output_file="$3"

    print_step "Converting ${component} Swagger 2.0 to OpenAPI 3.0" "PROCESSING"

    local convert_out="${LOG_DIR}/${component}_convert.out"
    local convert_err="${LOG_DIR}/${component}_convert.err"
    local start_time=$(date +%s.%N)

    # Check if input file exists
    if [ ! -f "${input_file}" ]; then
        print_step "Converting ${component} - input file not found" "FAILED"
        echo -e "      ${RED}File not found: ${input_file}${NC}"
        return 1
    fi

    # Use openapi-generator-cli Docker image to convert Swagger 2.0 to OpenAPI 3.0
    if docker run --rm \
        -v "${ROOT_DIR}:/local" \
        openapitools/openapi-generator-cli:"${OPENAPI_GENERATOR_VERSION}" generate \
        -i "/local/components/${component}/api/swagger.json" \
        -g openapi-yaml \
        -o "/local/components/${component}/api" \
        --skip-validate-spec \
        > "${convert_out}" 2> "${convert_err}"; then

        # Rename the generated file
        if [ -f "${ROOT_DIR}/components/${component}/api/openapi/openapi.yaml" ]; then
            mv "${ROOT_DIR}/components/${component}/api/openapi/openapi.yaml" "${output_file}"
            rm -rf "${ROOT_DIR}/components/${component}/api/openapi"
        fi

        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Converted ${component} to OpenAPI 3.0" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Converting ${component}" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -10 "${convert_err}" 2>/dev/null | sed 's/^/        /' || true
        return 1
    fi
}

# Merge OpenAPI specs using openapi-merge-cli
merge_openapi_specs() {
    print_step "Merging OpenAPI specifications" "PROCESSING"

    local merge_out="${LOG_DIR}/merge.out"
    local merge_err="${LOG_DIR}/merge.err"
    local start_time=$(date +%s.%N)

    # Run openapi-merge-cli
    if (cd "${ROOT_DIR}" && npx openapi-merge-cli --config openapi-merge.json > "${merge_out}" 2> "${merge_err}"); then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Merged OpenAPI specifications" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Merging OpenAPI specifications" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -10 "${merge_err}" 2>/dev/null | sed 's/^/        /' || true
        return 1
    fi
}

# Post-process the unified spec to add metadata
postprocess_unified_spec() {
    print_step "Post-processing unified specification" "PROCESSING"

    local unified_spec="${OUTPUT_DIR}/midaz-unified.yaml"
    local temp_spec="${OUTPUT_DIR}/midaz-unified.tmp.yaml"
    local start_time=$(date +%s.%N)

    if [ ! -f "${unified_spec}" ]; then
        print_step "Post-processing - unified spec not found" "FAILED"
        return 1
    fi

    # Create a node script to post-process the YAML
    local postprocess_script="${SCRIPTS_DIR}/postprocess-openapi.js"
    cat > "${postprocess_script}" << 'POSTPROCESS_EOF'
const fs = require('fs');
const yaml = require('js-yaml');

const inputFile = process.argv[2];
const outputFile = process.argv[3];
const serverUrl = process.env.MIDAZ_API_URL || 'http://localhost:3000';

try {
    const content = fs.readFileSync(inputFile, 'utf8');
    const spec = yaml.load(content);

    // Update info section
    spec.info = {
        title: 'Midaz API',
        description: 'Unified API documentation for Midaz - a financial ledger platform. This specification combines the Onboarding API (Organizations, Ledgers, Assets, Portfolios, Segments, Accounts) and Transaction API (Transactions, Operations, Balances, Asset Rates).',
        version: spec.info?.version || 'v1.48.0',
        termsOfService: 'http://swagger.io/terms/',
        contact: {
            name: 'Discord community',
            url: 'https://discord.gg/DnhqKwkGv3'
        },
        license: {
            name: 'Apache 2.0',
            url: 'http://www.apache.org/licenses/LICENSE-2.0.html'
        }
    };

    // Update servers section
    spec.servers = [
        {
            url: serverUrl,
            description: 'Midaz API Server'
        }
    ];

    // Ensure security schemes are properly defined
    if (!spec.components) {
        spec.components = {};
    }
    if (!spec.components.securitySchemes) {
        spec.components.securitySchemes = {
            BearerAuth: {
                type: 'http',
                scheme: 'bearer',
                bearerFormat: 'JWT',
                description: 'JWT Authorization header using the Bearer scheme. Example: "Authorization: Bearer {token}"'
            }
        };
    }

    // Add global security requirement
    spec.security = [{ BearerAuth: [] }];

    // Sort tags for better organization
    if (spec.tags) {
        const tagOrder = [
            'Organizations', 'Ledgers', 'Assets', 'Portfolios', 'Segments',
            'Accounts', 'Account Types', 'Transactions', 'Operations',
            'Balances', 'Asset Rates', 'Operation Route', 'Transaction Route'
        ];
        spec.tags.sort((a, b) => {
            const aIndex = tagOrder.indexOf(a.name);
            const bIndex = tagOrder.indexOf(b.name);
            if (aIndex === -1 && bIndex === -1) return 0;
            if (aIndex === -1) return 1;
            if (bIndex === -1) return -1;
            return aIndex - bIndex;
        });
    }

    const output = yaml.dump(spec, {
        indent: 2,
        lineWidth: -1,
        noRefs: true,
        sortKeys: false
    });

    fs.writeFileSync(outputFile, output);
    console.log('Post-processing completed successfully');
} catch (error) {
    console.error('Error during post-processing:', error.message);
    process.exit(1);
}
POSTPROCESS_EOF

    # Run post-processing from the scripts directory where node_modules exists
    if (cd "${SCRIPTS_DIR}" && node postprocess-openapi.js "${unified_spec}" "${temp_spec}" 2>"${LOG_DIR}/postprocess.err"); then
        mv "${temp_spec}" "${unified_spec}"
        rm -f "${postprocess_script}"  # Clean up the temp script
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Post-processed unified specification" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Post-processing unified specification" "FAILED"
        head -5 "${LOG_DIR}/postprocess.err" 2>/dev/null | sed 's/^/        /' || true
        rm -f "${postprocess_script}"  # Clean up the temp script
        return 1
    fi
}

# Count operations in the unified spec
count_operations() {
    local unified_spec="${OUTPUT_DIR}/midaz-unified.yaml"

    if [ -f "${unified_spec}" ]; then
        # Use node to count operations since we have js-yaml available
        local count_script="${SCRIPTS_DIR}/count-operations.js"
        cat > "${count_script}" << 'COUNT_EOF'
const fs = require('fs');
const yaml = require('js-yaml');

const inputFile = process.argv[2];
const content = fs.readFileSync(inputFile, 'utf8');
const spec = yaml.load(content);

let pathCount = 0;
let operationCount = 0;
const methods = ['get', 'post', 'put', 'patch', 'delete', 'options', 'head'];

if (spec.paths) {
    pathCount = Object.keys(spec.paths).length;
    for (const path of Object.values(spec.paths)) {
        for (const method of methods) {
            if (path[method]) {
                operationCount++;
            }
        }
    }
}

console.log(JSON.stringify({ paths: pathCount, operations: operationCount }));
COUNT_EOF

        local result=$(cd "${SCRIPTS_DIR}" && node count-operations.js "${unified_spec}" 2>/dev/null || echo '{"paths":0,"operations":0}')
        rm -f "${count_script}"  # Clean up
        echo "${result}"
    else
        echo '{"paths":0,"operations":0}'
    fi
}

# Main execution
main() {
    echo ""
    echo -e "${BLUE}=================================================${NC}"
    echo -e "${BLUE}  Unified OpenAPI Spec Generator${NC}"
    echo -e "${BLUE}=================================================${NC}"
    echo ""

    local overall_success=true

    # Check dependencies
    check_dependencies

    # Ensure output directory exists
    ensure_output_dir

    # Install npm dependencies
    if ! install_dependencies; then
        overall_success=false
    fi

    # Convert Swagger specs to OpenAPI 3.0 (if not already converted)
    if [ "$overall_success" = true ]; then
        # Check if openapi.yaml files already exist and are newer than swagger.json
        local need_convert_onboarding=true
        local need_convert_transaction=true

        if [ -f "${ONBOARDING_API}/openapi.yaml" ] && [ "${ONBOARDING_API}/openapi.yaml" -nt "${ONBOARDING_API}/swagger.json" ]; then
            print_step "Onboarding OpenAPI 3.0 spec is up-to-date" "SUCCESS" "0.0"
            need_convert_onboarding=false
        fi

        if [ -f "${TRANSACTION_API}/openapi.yaml" ] && [ "${TRANSACTION_API}/openapi.yaml" -nt "${TRANSACTION_API}/swagger.json" ]; then
            print_step "Transaction OpenAPI 3.0 spec is up-to-date" "SUCCESS" "0.0"
            need_convert_transaction=false
        fi

        # Convert if needed
        if [ "$need_convert_onboarding" = true ]; then
            if ! convert_swagger_to_openapi "onboarding" "${ONBOARDING_API}/swagger.json" "${ONBOARDING_API}/openapi.yaml"; then
                overall_success=false
            fi
        fi

        if [ "$need_convert_transaction" = true ] && [ "$overall_success" = true ]; then
            if ! convert_swagger_to_openapi "transaction" "${TRANSACTION_API}/swagger.json" "${TRANSACTION_API}/openapi.yaml"; then
                overall_success=false
            fi
        fi
    fi

    # Merge OpenAPI specs
    if [ "$overall_success" = true ]; then
        if ! merge_openapi_specs; then
            overall_success=false
        fi
    fi

    # Post-process the unified spec
    if [ "$overall_success" = true ]; then
        if ! postprocess_unified_spec; then
            overall_success=false
        fi
    fi

    # Final status
    echo ""
    if [ "$overall_success" = true ]; then
        local stats=$(count_operations)
        local path_count=$(echo "${stats}" | jq -r '.paths' 2>/dev/null || echo "?")
        local op_count=$(echo "${stats}" | jq -r '.operations' 2>/dev/null || echo "?")

        echo -e "${GREEN}Unified OpenAPI spec generated successfully!${NC}"
        echo -e "   Output: ${OUTPUT_DIR}/midaz-unified.yaml"
        echo -e "   Paths: ${path_count}"
        echo -e "   Operations: ${op_count}"

        # Clean up temporary logs on success
        rm -rf "${LOG_DIR}"
    else
        echo -e "${RED}Unified OpenAPI spec generation failed.${NC}"
        echo -e "   Check logs in: ${LOG_DIR}/"
        exit 1
    fi

    echo ""
}

# Run main function
main "$@"
