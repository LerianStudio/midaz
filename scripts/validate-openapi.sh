#!/usr/bin/env bash
# shellcheck disable=SC2086  # Intentional word splitting for command arguments
# shellcheck disable=SC2155  # Declare and assign separately to avoid masking return values
set -euo pipefail

# OpenAPI Spec Validation Script
# Validates OpenAPI specifications using Spectral and additional checks.
#
# Usage:
#   ./scripts/validate-openapi.sh [--install-deps]
#
# Options:
#   --install-deps    Install Spectral CLI if not present
#
# Exit codes:
#   0 - All validations passed
#   1 - Validation errors found

# Root directory of the repo
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
ERRORS=0
WARNINGS=0

# Components to validate
COMPONENTS=("onboarding" "transaction")

# Print section header
print_header() {
    echo ""
    echo -e "${BLUE}=================================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}=================================================${NC}"
    echo ""
}

# Print step result
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
    elif [ "$status" = "WARN" ]; then
        echo -e "  ${YELLOW}[WARN]${NC} $step_name"
        if [ -n "$details" ]; then
            echo -e "       $details"
        fi
    elif [ "$status" = "SKIP" ]; then
        echo -e "  ${YELLOW}[SKIP]${NC} $step_name"
        if [ -n "$details" ]; then
            echo -e "       $details"
        fi
    fi
}

# Check and install dependencies
check_dependencies() {
    print_header "Checking Dependencies"

    # Check for spectral
    if command -v spectral &> /dev/null; then
        SPECTRAL_VERSION=$(spectral --version 2>/dev/null || echo "unknown")
        print_step "Spectral CLI installed ($SPECTRAL_VERSION)" "SUCCESS"
    else
        if [ "${1:-}" = "--install-deps" ]; then
            echo "  Installing @stoplight/spectral-cli..."
            if command -v npm &> /dev/null; then
                npm install -g @stoplight/spectral-cli 2>/dev/null || {
                    print_step "Failed to install Spectral" "FAILED"
                    echo "  Please install manually: npm install -g @stoplight/spectral-cli"
                    return 1
                }
                print_step "Spectral CLI installed" "SUCCESS"
            else
                print_step "npm not found, cannot install Spectral" "FAILED"
                return 1
            fi
        else
            print_step "Spectral CLI not installed" "WARN" "Run with --install-deps to install, or: npm install -g @stoplight/spectral-cli"
            return 1
        fi
    fi

    # Check for jq (useful for JSON manipulation)
    if command -v jq &> /dev/null; then
        print_step "jq installed" "SUCCESS"
    else
        print_step "jq not installed" "WARN" "Optional: Install jq for better JSON handling"
    fi

    return 0
}

# Validate JSON syntax
validate_json_syntax() {
    local file="$1"
    local component="$2"

    if [ ! -f "$file" ]; then
        print_step "$component: swagger.json not found" "SKIP" "Run 'make generate-docs' first"
        return 1
    fi

    if command -v jq &> /dev/null; then
        if jq empty "$file" 2>/dev/null; then
            print_step "$component: Valid JSON syntax" "SUCCESS"
            return 0
        else
            print_step "$component: Invalid JSON syntax" "FAILED"
            ((ERRORS++))
            return 1
        fi
    else
        # Fallback to python if jq not available
        if command -v python3 &> /dev/null; then
            if python3 -c "import json; json.load(open('$file'))" 2>/dev/null; then
                print_step "$component: Valid JSON syntax" "SUCCESS"
                return 0
            else
                print_step "$component: Invalid JSON syntax" "FAILED"
                ((ERRORS++))
                return 1
            fi
        fi
    fi

    print_step "$component: Could not validate JSON (no jq or python3)" "WARN"
    return 0
}

# Validate OpenAPI version
validate_openapi_version() {
    local file="$1"
    local component="$2"

    if [ ! -f "$file" ]; then
        return 1
    fi

    local version=""
    if command -v jq &> /dev/null; then
        version=$(jq -r '.swagger // .openapi // "unknown"' "$file" 2>/dev/null)
    fi

    if [ -n "$version" ] && [ "$version" != "unknown" ] && [ "$version" != "null" ]; then
        print_step "$component: OpenAPI/Swagger version $version" "SUCCESS"
    else
        print_step "$component: Could not determine OpenAPI version" "WARN"
    fi
}

# Validate required fields
validate_required_fields() {
    local file="$1"
    local component="$2"

    if [ ! -f "$file" ] || ! command -v jq &> /dev/null; then
        return 0
    fi

    local missing_fields=()

    # Check info section
    local info_title=$(jq -r '.info.title // ""' "$file" 2>/dev/null)
    if [ -z "$info_title" ] || [ "$info_title" = "null" ]; then
        missing_fields+=("info.title")
    fi

    local info_version=$(jq -r '.info.version // ""' "$file" 2>/dev/null)
    if [ -z "$info_version" ] || [ "$info_version" = "null" ]; then
        missing_fields+=("info.version")
    fi

    local info_desc=$(jq -r '.info.description // ""' "$file" 2>/dev/null)
    if [ -z "$info_desc" ] || [ "$info_desc" = "null" ]; then
        missing_fields+=("info.description")
    fi

    if [ ${#missing_fields[@]} -gt 0 ]; then
        print_step "$component: Missing required fields: ${missing_fields[*]}" "WARN"
        ((WARNINGS++))
    else
        print_step "$component: All required fields present" "SUCCESS"
    fi
}

# Check for operations without operationId
validate_operation_ids() {
    local file="$1"
    local component="$2"

    if [ ! -f "$file" ] || ! command -v jq &> /dev/null; then
        return 0
    fi

    # Count total operations and operations with operationId
    local total_ops=$(jq '[.paths | to_entries[] | .value | to_entries[] | select(.key != "parameters")] | length' "$file" 2>/dev/null || echo "0")
    local ops_with_id=$(jq '[.paths | to_entries[] | .value | to_entries[] | select(.key != "parameters" and .value.operationId != null)] | length' "$file" 2>/dev/null || echo "0")

    if [ "$total_ops" = "$ops_with_id" ]; then
        print_step "$component: All $total_ops operations have operationId" "SUCCESS"
    else
        local missing=$((total_ops - ops_with_id))
        print_step "$component: $missing of $total_ops operations missing operationId" "WARN"
        ((WARNINGS++))
    fi
}

# Check for operations without tags
validate_tags() {
    local file="$1"
    local component="$2"

    if [ ! -f "$file" ] || ! command -v jq &> /dev/null; then
        return 0
    fi

    local total_ops=$(jq '[.paths | to_entries[] | .value | to_entries[] | select(.key != "parameters")] | length' "$file" 2>/dev/null || echo "0")
    local ops_with_tags=$(jq '[.paths | to_entries[] | .value | to_entries[] | select(.key != "parameters" and .value.tags != null and (.value.tags | length) > 0)] | length' "$file" 2>/dev/null || echo "0")

    if [ "$total_ops" = "$ops_with_tags" ]; then
        print_step "$component: All $total_ops operations have tags" "SUCCESS"
    else
        local missing=$((total_ops - ops_with_tags))
        print_step "$component: $missing of $total_ops operations missing tags" "WARN"
        ((WARNINGS++))
    fi
}

# Run Spectral validation
run_spectral_validation() {
    local file="$1"
    local component="$2"
    # Use SCRIPT_DIR for reliable path resolution regardless of execution directory
    local ruleset="${SCRIPT_DIR}/../.spectral.yaml"

    if [ ! -f "$file" ]; then
        return 0
    fi

    if ! command -v spectral &> /dev/null; then
        print_step "$component: Spectral validation skipped (not installed)" "SKIP"
        return 0
    fi

    echo "  Running Spectral validation for $component..."

    local spectral_args="--format stylish"
    if [ -f "$ruleset" ]; then
        spectral_args="$spectral_args --ruleset $ruleset"
    fi

    # Run spectral and capture output
    local output
    local exit_code=0
    output=$(spectral lint "$file" $spectral_args 2>&1) || exit_code=$?

    if [ $exit_code -eq 0 ]; then
        print_step "$component: Spectral validation passed" "SUCCESS"
    elif [ $exit_code -eq 1 ]; then
        # Warnings only
        print_step "$component: Spectral found warnings" "WARN"
        echo "$output" | head -20 | sed 's/^/       /'
        ((WARNINGS++))
    else
        # Errors
        print_step "$component: Spectral validation failed" "FAILED"
        echo "$output" | head -20 | sed 's/^/       /'
        ((ERRORS++))
    fi
}

# Count operations and paths
count_operations() {
    local file="$1"
    local component="$2"

    if [ ! -f "$file" ] || ! command -v jq &> /dev/null; then
        return 0
    fi

    local paths_count=$(jq '.paths | length' "$file" 2>/dev/null || echo "0")
    local ops_count=$(jq '[.paths | to_entries[] | .value | to_entries[] | select(.key != "parameters")] | length' "$file" 2>/dev/null || echo "0")
    local schemas_count=$(jq '(.definitions // .components.schemas // {}) | length' "$file" 2>/dev/null || echo "0")

    print_step "$component: $paths_count paths, $ops_count operations, $schemas_count schemas" "SUCCESS"
}

# Main validation
main() {
    local install_deps=false

    # Parse arguments
    for arg in "$@"; do
        case $arg in
            --install-deps)
                install_deps=true
                ;;
        esac
    done

    echo -e "${GREEN}OpenAPI Specification Validation${NC}"
    echo "=================================================="

    # Check dependencies
    if [ "$install_deps" = true ]; then
        check_dependencies "--install-deps" || true
    else
        check_dependencies || true
    fi

    # Validate each component
    for component in "${COMPONENTS[@]}"; do
        local spec_file="${ROOT_DIR}/components/${component}/api/swagger.json"

        print_header "Validating $component API Spec"

        # Check if file exists
        if [ ! -f "$spec_file" ]; then
            print_step "$component: swagger.json not found" "SKIP" "Run 'make generate-docs' first"
            continue
        fi

        # Run validations
        validate_json_syntax "$spec_file" "$component"
        validate_openapi_version "$spec_file" "$component"
        validate_required_fields "$spec_file" "$component"
        validate_operation_ids "$spec_file" "$component"
        validate_tags "$spec_file" "$component"
        count_operations "$spec_file" "$component"
        run_spectral_validation "$spec_file" "$component"
    done

    # Check for unified spec
    local unified_spec="${ROOT_DIR}/api/midaz-unified.yaml"
    if [ -f "$unified_spec" ]; then
        print_header "Validating Unified API Spec"
        run_spectral_validation "$unified_spec" "unified"
    fi

    # Summary
    print_header "Summary"

    echo "Components validated: ${#COMPONENTS[@]}"
    echo -e "Errors:              ${RED}$ERRORS${NC}"
    echo -e "Warnings:            ${YELLOW}$WARNINGS${NC}"
    echo ""

    if [ $ERRORS -gt 0 ]; then
        echo -e "${RED}OpenAPI validation failed with $ERRORS error(s)${NC}"
        exit 1
    elif [ $WARNINGS -gt 0 ]; then
        echo -e "${YELLOW}OpenAPI validation passed with $WARNINGS warning(s)${NC}"
        exit 0
    else
        echo -e "${GREEN}All OpenAPI validations passed!${NC}"
        exit 0
    fi
}

main "$@"
