#!/usr/bin/env bash
# shellcheck disable=SC2086  # Intentional word splitting for command arguments
# shellcheck disable=SC2155  # Declare and assign separately to avoid masking return values
set -euo pipefail

# SDK Sample Generation Script
# Generates SDK client code from OpenAPI specifications using openapi-generator.
#
# Usage:
#   ./scripts/generate-sdk-samples.sh [--language go|typescript|python] [--output-dir DIR]
#
# Options:
#   --language    Target language for SDK generation (default: go)
#   --output-dir  Custom output directory (default: docs/sdk-samples/<language>)
#   --all         Generate SDKs for all supported languages
#
# Environment variables:
#   OPENAPI_GENERATOR_VERSION - OpenAPI Generator version (default: v7.10.0)
#
# Prerequisites:
#   - Docker (for openapi-generator-cli)
#   - Generated OpenAPI specs (run 'make generate-docs' first)
#
# Exit codes:
#   0 - Generation successful
#   1 - Error occurred

# Root directory of the repo
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Use versioned image, allow override via environment variable
OPENAPI_GENERATOR_VERSION="${OPENAPI_GENERATOR_VERSION:-v7.10.0}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
LANGUAGE="go"
OUTPUT_DIR=""
GENERATE_ALL=false

# Supported languages
SUPPORTED_LANGUAGES=("go" "typescript" "python" "java")

# Components to generate SDKs for
COMPONENTS=("onboarding" "transaction")

# Print usage
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --language LANG    Target language: go, typescript, python, java (default: go)"
    echo "  --output-dir DIR   Custom output directory"
    echo "  --all              Generate SDKs for all supported languages"
    echo "  --help             Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                           # Generate Go SDK"
    echo "  $0 --language typescript     # Generate TypeScript SDK"
    echo "  $0 --all                     # Generate SDKs for all languages"
}

# Print header
print_header() {
    echo ""
    echo -e "${BLUE}=================================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}=================================================${NC}"
    echo ""
}

# Print step
print_step() {
    local step_name="$1"
    local status="$2"
    local details="${3:-}"

    if [ "$status" = "SUCCESS" ]; then
        echo -e "  ${GREEN}[OK]${NC} $step_name"
    elif [ "$status" = "FAILED" ]; then
        echo -e "  ${RED}[FAIL]${NC} $step_name"
        if [ -n "$details" ]; then
            echo -e "       $details"
        fi
    elif [ "$status" = "SKIP" ]; then
        echo -e "  ${YELLOW}[SKIP]${NC} $step_name"
        if [ -n "$details" ]; then
            echo -e "       $details"
        fi
    else
        echo -e "  ${YELLOW}[...]${NC} $step_name"
    fi
}

# Check prerequisites
check_prerequisites() {
    print_header "Checking Prerequisites"

    # Check for Docker
    if command -v docker &> /dev/null; then
        print_step "Docker installed" "SUCCESS"
    else
        print_step "Docker not installed" "FAILED" "Please install Docker: https://docs.docker.com/get-docker/"
        exit 1
    fi

    # Check for OpenAPI specs
    local specs_found=0
    for component in "${COMPONENTS[@]}"; do
        local spec_file="${ROOT_DIR}/components/${component}/api/swagger.json"
        if [ -f "$spec_file" ]; then
            print_step "Found $component OpenAPI spec" "SUCCESS"
            ((specs_found++))
        else
            print_step "Missing $component OpenAPI spec" "SKIP" "Run 'make generate-docs' first"
        fi
    done

    if [ $specs_found -eq 0 ]; then
        echo ""
        echo -e "${RED}No OpenAPI specs found. Run 'make generate-docs' first.${NC}"
        exit 1
    fi
}

# Get generator options for language
get_generator_options() {
    local lang="$1"
    local package_name="midaz"

    case $lang in
        go)
            echo "--additional-properties=packageName=${package_name},enumClassPrefix=true,generateInterfaces=true"
            ;;
        typescript)
            echo "--additional-properties=npmName=@midaz/client,supportsES6=true,withNodeImports=true"
            ;;
        python)
            echo "--additional-properties=packageName=${package_name},projectName=midaz-client"
            ;;
        java)
            echo "--additional-properties=groupId=io.midaz,artifactId=midaz-client,invokerPackage=io.midaz.client"
            ;;
        *)
            echo ""
            ;;
    esac
}

# Generate SDK for a specific component and language
generate_sdk() {
    local component="$1"
    local lang="$2"
    local output_base="$3"

    local spec_file="${ROOT_DIR}/components/${component}/api/swagger.json"
    local output_dir="${output_base}/${lang}/${component}"

    if [ ! -f "$spec_file" ]; then
        print_step "Skipping $component (no spec file)" "SKIP"
        return 0
    fi

    echo "  Generating $lang SDK for $component..."

    # Create output directory
    mkdir -p "$output_dir"

    # Get generator options
    local generator_opts=$(get_generator_options "$lang")

    # Run openapi-generator via Docker
    if docker run --rm \
        -v "${ROOT_DIR}:/local" \
        openapitools/openapi-generator-cli:"${OPENAPI_GENERATOR_VERSION}" generate \
        -i "/local/components/${component}/api/swagger.json" \
        -g "$lang" \
        -o "/local/docs/sdk-samples/${lang}/${component}" \
        $generator_opts \
        2>/dev/null; then
        print_step "Generated $lang SDK for $component" "SUCCESS"
        return 0
    else
        print_step "Failed to generate $lang SDK for $component" "FAILED"
        return 1
    fi
}

# Generate SDKs for a single language
generate_for_language() {
    local lang="$1"
    local output_base="${OUTPUT_DIR:-${ROOT_DIR}/docs/sdk-samples}"

    print_header "Generating $lang SDK"

    local success=0
    local failed=0

    for component in "${COMPONENTS[@]}"; do
        if generate_sdk "$component" "$lang" "$output_base"; then
            ((success++))
        else
            ((failed++))
        fi
    done

    echo ""
    echo "  Components generated: $success"
    echo "  Components failed: $failed"

    return $failed
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --language)
            LANGUAGE="$2"
            shift 2
            ;;
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --all)
            GENERATE_ALL=true
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Validate language
if [ "$GENERATE_ALL" = false ]; then
    valid_lang=false
    for supported in "${SUPPORTED_LANGUAGES[@]}"; do
        if [ "$LANGUAGE" = "$supported" ]; then
            valid_lang=true
            break
        fi
    done

    if [ "$valid_lang" = false ]; then
        echo "Unsupported language: $LANGUAGE"
        echo "Supported languages: ${SUPPORTED_LANGUAGES[*]}"
        exit 1
    fi
fi

# Main execution
echo -e "${GREEN}SDK Sample Generation${NC}"
echo "=================================================="

check_prerequisites

total_errors=0

if [ "$GENERATE_ALL" = true ]; then
    for lang in "${SUPPORTED_LANGUAGES[@]}"; do
        if ! generate_for_language "$lang"; then
            ((total_errors++))
        fi
    done
else
    if ! generate_for_language "$LANGUAGE"; then
        ((total_errors++))
    fi
fi

# Summary
print_header "Summary"

OUTPUT_BASE="${OUTPUT_DIR:-${ROOT_DIR}/docs/sdk-samples}"
echo "Output directory: $OUTPUT_BASE"

if [ "$GENERATE_ALL" = true ]; then
    echo "Languages generated: ${SUPPORTED_LANGUAGES[*]}"
else
    echo "Language generated: $LANGUAGE"
fi

echo ""

if [ $total_errors -eq 0 ]; then
    echo -e "${GREEN}SDK generation completed successfully!${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Review generated code in: $OUTPUT_BASE"
    echo "  2. Copy relevant examples to your SDK project"
    echo "  3. Customize as needed for your use case"
    exit 0
else
    echo -e "${YELLOW}SDK generation completed with some failures.${NC}"
    exit 1
fi
