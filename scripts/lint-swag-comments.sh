#!/usr/bin/env bash
# shellcheck disable=SC2086  # Intentional word splitting for command arguments
# shellcheck disable=SC2155  # Declare and assign separately to avoid masking return values
# shellcheck disable=SC2207  # Prefer mapfile for array assignment
set -euo pipefail

# Swag Comment Linting Script
# Validates swag annotations in HTTP handler files for common issues.
#
# Usage:
#   ./scripts/lint-swag-comments.sh [--fix]
#
# Options:
#   --fix    Attempt to auto-fix simple issues (not implemented yet)
#
# Exit codes:
#   0 - All checks passed
#   1 - Errors found

# Root directory of the repo
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Counters
ERRORS=0
WARNINGS=0

# Handler file patterns
HANDLER_DIRS=(
    "${ROOT_DIR}/components/onboarding/internal/adapters/http/in"
    "${ROOT_DIR}/components/transaction/internal/adapters/http/in"
)

# Print section header
print_section() {
    echo -e "\n${YELLOW}=== $1 ===${NC}"
}

# Print check result
print_check() {
    local check_name="$1"
    local status="$2"
    local details="${3:-}"

    if [ "$status" = "PASS" ]; then
        echo -e "  ${GREEN}[PASS]${NC} $check_name"
    elif [ "$status" = "FAIL" ]; then
        echo -e "  ${RED}[FAIL]${NC} $check_name"
        if [ -n "$details" ]; then
            echo -e "         $details"
        fi
    elif [ "$status" = "WARN" ]; then
        echo -e "  ${YELLOW}[WARN]${NC} $check_name"
        if [ -n "$details" ]; then
            echo -e "         $details"
        fi
    fi
}

# Get handler Go files
get_handler_files() {
    local files=()
    for dir in "${HANDLER_DIRS[@]}"; do
        if [ -d "$dir" ]; then
            while IFS= read -r -d '' file; do
                # Exclude routes.go and swagger.go
                basename=$(basename "$file")
                if [[ "$basename" != "routes.go" && "$basename" != "swagger.go" ]]; then
                    files+=("$file")
                fi
            done < <(find "$dir" -name "*.go" -type f -print0)
        fi
    done
    echo "${files[@]}"
}

echo -e "${GREEN}Linting Swag Comments${NC}"
echo "=================================================="

# Get all handler files
HANDLER_FILES=($(get_handler_files))

if [ ${#HANDLER_FILES[@]} -eq 0 ]; then
    echo -e "${YELLOW}No handler files found to lint${NC}"
    exit 0
fi

echo "Found ${#HANDLER_FILES[@]} handler files to check"

# -----------------------------------------------------------------------------
# Check 1: PascalCase HTTP Methods
# The HTTP method in @Router should be lowercase (get, post, put, patch, delete)
# -----------------------------------------------------------------------------
print_section "Checking for PascalCase HTTP methods"

# Check for PascalCase HTTP methods (case-insensitive check for capital letters)
echo "  Checking for PascalCase HTTP methods..."
PASCAL_CASE_FOUND=false
for file in "${HANDLER_FILES[@]}"; do
    # Match any HTTP method with capital letters: [Get], [POST], [Patch], etc.
    # We look for methods that are NOT all lowercase
    if grep -E '@Router.*\[(GET|Get|POST|Post|PUT|Put|PATCH|Patch|DELETE|Delete|HEAD|Head)\]' "$file" 2>/dev/null | grep -v '\[get\]\|\[post\]\|\[put\]\|\[patch\]\|\[delete\]\|\[head\]' > /dev/null; then
        echo "    Found PascalCase HTTP method in: $file"
        grep -E '@Router.*\[(GET|Get|POST|Post|PUT|Put|PATCH|Patch|DELETE|Delete|HEAD|Head)\]' "$file" 2>/dev/null | grep -v '\[get\]\|\[post\]\|\[put\]\|\[patch\]\|\[delete\]\|\[head\]' | while IFS= read -r line; do
            echo -e "         ${RED}$line${NC}"
        done
        PASCAL_CASE_FOUND=true
    fi
done

if [ "$PASCAL_CASE_FOUND" = true ]; then
    print_check "HTTP methods should be lowercase" "FAIL" "Use lowercase HTTP methods: [get], [post], [patch], [delete], [put], [head]"
    ((ERRORS++))
else
    print_check "HTTP methods are lowercase" "PASS"
fi

# -----------------------------------------------------------------------------
# Check 2: Missing @Tags annotation
# Every handler with @Router should have a @Tags annotation
# -----------------------------------------------------------------------------
print_section "Checking for missing @Tags annotations"

MISSING_TAGS=0
for file in "${HANDLER_FILES[@]}"; do
    # Count @Router and @Tags in the file
    ROUTER_COUNT=$(grep -c "@Router" "$file" 2>/dev/null || echo "0")
    TAGS_COUNT=$(grep -c "@Tags" "$file" 2>/dev/null || echo "0")

    if [ "$ROUTER_COUNT" -gt "$TAGS_COUNT" ]; then
        MISSING_TAGS=$((MISSING_TAGS + ROUTER_COUNT - TAGS_COUNT))
        print_check "$(basename "$file"): $ROUTER_COUNT routes but only $TAGS_COUNT @Tags" "WARN"
        ((WARNINGS++))
    fi
done

if [ "$MISSING_TAGS" -eq 0 ]; then
    print_check "All routes have @Tags annotations" "PASS"
fi

# -----------------------------------------------------------------------------
# Check 3: Missing @Summary annotation
# Every handler with @Router should have a @Summary annotation
# -----------------------------------------------------------------------------
print_section "Checking for missing @Summary annotations"

MISSING_SUMMARY=0
for file in "${HANDLER_FILES[@]}"; do
    ROUTER_COUNT=$(grep -c "@Router" "$file" 2>/dev/null || echo "0")
    SUMMARY_COUNT=$(grep -c "@Summary" "$file" 2>/dev/null || echo "0")

    if [ "$ROUTER_COUNT" -gt "$SUMMARY_COUNT" ]; then
        MISSING_SUMMARY=$((MISSING_SUMMARY + ROUTER_COUNT - SUMMARY_COUNT))
        print_check "$(basename "$file"): $ROUTER_COUNT routes but only $SUMMARY_COUNT @Summary" "WARN"
        ((WARNINGS++))
    fi
done

if [ "$MISSING_SUMMARY" -eq 0 ]; then
    print_check "All routes have @Summary annotations" "PASS"
fi

# -----------------------------------------------------------------------------
# Check 4: Missing @Description annotation
# Every handler with @Router should have a @Description annotation
# -----------------------------------------------------------------------------
print_section "Checking for missing @Description annotations"

MISSING_DESC=0
for file in "${HANDLER_FILES[@]}"; do
    ROUTER_COUNT=$(grep -c "@Router" "$file" 2>/dev/null || echo "0")
    DESC_COUNT=$(grep -c "@Description" "$file" 2>/dev/null || echo "0")

    if [ "$ROUTER_COUNT" -gt "$DESC_COUNT" ]; then
        MISSING_DESC=$((MISSING_DESC + ROUTER_COUNT - DESC_COUNT))
        print_check "$(basename "$file"): $ROUTER_COUNT routes but only $DESC_COUNT @Description" "WARN"
        ((WARNINGS++))
    fi
done

if [ "$MISSING_DESC" -eq 0 ]; then
    print_check "All routes have @Description annotations" "PASS"
fi

# -----------------------------------------------------------------------------
# Check 5: Missing @Produce annotation for non-DELETE routes
# Routes that return data should have @Produce annotation
# -----------------------------------------------------------------------------
print_section "Checking for missing @Produce annotations"

MISSING_PRODUCE=0
for file in "${HANDLER_FILES[@]}"; do
    # Count routes that should produce output (excluding DELETE which may return 204)
    ROUTER_COUNT=$(grep -c "@Router" "$file" 2>/dev/null || echo "0")
    PRODUCE_COUNT=$(grep -c "@Produce" "$file" 2>/dev/null || echo "0")

    if [ "$ROUTER_COUNT" -gt "$PRODUCE_COUNT" ]; then
        MISSING_PRODUCE=$((MISSING_PRODUCE + ROUTER_COUNT - PRODUCE_COUNT))
        print_check "$(basename "$file"): $ROUTER_COUNT routes but only $PRODUCE_COUNT @Produce" "WARN"
        ((WARNINGS++))
    fi
done

if [ "$MISSING_PRODUCE" -eq 0 ]; then
    print_check "All routes have @Produce annotations" "PASS"
fi

# -----------------------------------------------------------------------------
# Check 6: Missing @Accept annotation for POST/PUT/PATCH routes
# Routes that accept body should have @Accept annotation
# -----------------------------------------------------------------------------
print_section "Checking for missing @Accept annotations on POST/PUT/PATCH routes"

MISSING_ACCEPT=0
for file in "${HANDLER_FILES[@]}"; do
    # Count routes that should accept input
    BODY_ROUTES=$(grep -c "@Router.*\[post\]\|@Router.*\[put\]\|@Router.*\[patch\]" "$file" 2>/dev/null || echo "0")
    ACCEPT_COUNT=$(grep -c "@Accept" "$file" 2>/dev/null || echo "0")

    if [ "$BODY_ROUTES" -gt "$ACCEPT_COUNT" ]; then
        MISSING_ACCEPT=$((MISSING_ACCEPT + BODY_ROUTES - ACCEPT_COUNT))
        print_check "$(basename "$file"): $BODY_ROUTES body routes but only $ACCEPT_COUNT @Accept" "WARN"
        ((WARNINGS++))
    fi
done

if [ "$MISSING_ACCEPT" -eq 0 ]; then
    print_check "All body routes have @Accept annotations" "PASS"
fi

# -----------------------------------------------------------------------------
# Check 7: Missing @Success annotation
# Every handler should have at least one @Success annotation
# -----------------------------------------------------------------------------
print_section "Checking for missing @Success annotations"

MISSING_SUCCESS=0
for file in "${HANDLER_FILES[@]}"; do
    ROUTER_COUNT=$(grep -c "@Router" "$file" 2>/dev/null || echo "0")
    SUCCESS_COUNT=$(grep -c "@Success" "$file" 2>/dev/null || echo "0")

    if [ "$ROUTER_COUNT" -gt "$SUCCESS_COUNT" ]; then
        MISSING_SUCCESS=$((MISSING_SUCCESS + ROUTER_COUNT - SUCCESS_COUNT))
        print_check "$(basename "$file"): $ROUTER_COUNT routes but only $SUCCESS_COUNT @Success" "FAIL"
        ((ERRORS++))
    fi
done

if [ "$MISSING_SUCCESS" -eq 0 ]; then
    print_check "All routes have @Success annotations" "PASS"
fi

# -----------------------------------------------------------------------------
# Check 8: Missing @Failure annotations
# Every handler should have @Failure annotations for common error codes
# -----------------------------------------------------------------------------
print_section "Checking for missing @Failure annotations"

MISSING_FAILURE=0
for file in "${HANDLER_FILES[@]}"; do
    ROUTER_COUNT=$(grep -c "@Router" "$file" 2>/dev/null || echo "0")
    FAILURE_COUNT=$(grep -c "@Failure" "$file" 2>/dev/null || echo "0")

    # Each route should have at least 2 failure codes (400/500 at minimum)
    MIN_FAILURES=$((ROUTER_COUNT * 2))
    if [ "$FAILURE_COUNT" -lt "$MIN_FAILURES" ]; then
        print_check "$(basename "$file"): $ROUTER_COUNT routes but only $FAILURE_COUNT @Failure (expected at least $MIN_FAILURES)" "WARN"
        ((WARNINGS++))
    fi
done

if [ "$MISSING_FAILURE" -eq 0 ]; then
    print_check "Routes have sufficient @Failure annotations" "PASS"
fi

# -----------------------------------------------------------------------------
# Check 9: Uppercase Enum values (should be lowercase in swag)
# Enum values in swag annotations should use lowercase 'Enums(' or 'enum:'
# -----------------------------------------------------------------------------
print_section "Checking for enum casing consistency"

UPPERCASE_ENUM=$(grep -r "Enums(" "${HANDLER_FILES[@]}" 2>/dev/null || true)

if [ -n "$UPPERCASE_ENUM" ]; then
    # Enums() is actually valid swag syntax, just note it
    ENUM_COUNT=$(echo "$UPPERCASE_ENUM" | wc -l | tr -d ' ')
    print_check "Found $ENUM_COUNT uses of 'Enums(' - this is valid swag syntax" "PASS"
fi

# -----------------------------------------------------------------------------
# Check 10: Authorization header documentation
# Routes should document the Authorization header
# -----------------------------------------------------------------------------
print_section "Checking for Authorization header documentation"

MISSING_AUTH_HEADER=0
for file in "${HANDLER_FILES[@]}"; do
    ROUTER_COUNT=$(grep -c "@Router" "$file" 2>/dev/null || echo "0")
    AUTH_HEADER_COUNT=$(grep -c "@Param.*Authorization.*header" "$file" 2>/dev/null || echo "0")

    if [ "$ROUTER_COUNT" -gt "$AUTH_HEADER_COUNT" ]; then
        MISSING_AUTH_HEADER=$((MISSING_AUTH_HEADER + ROUTER_COUNT - AUTH_HEADER_COUNT))
        print_check "$(basename "$file"): $ROUTER_COUNT routes but only $AUTH_HEADER_COUNT Authorization headers" "WARN"
        ((WARNINGS++))
    fi
done

if [ "$MISSING_AUTH_HEADER" -eq 0 ]; then
    print_check "All routes document Authorization header" "PASS"
fi

# -----------------------------------------------------------------------------
# Summary
# -----------------------------------------------------------------------------
print_section "Summary"

echo ""
echo "Files checked:  ${#HANDLER_FILES[@]}"
echo -e "Errors:         ${RED}$ERRORS${NC}"
echo -e "Warnings:       ${YELLOW}$WARNINGS${NC}"
echo ""

if [ $ERRORS -gt 0 ]; then
    echo -e "${RED}Swag linting failed with $ERRORS error(s)${NC}"
    echo "Fix the errors above before proceeding."
    exit 1
elif [ $WARNINGS -gt 0 ]; then
    echo -e "${YELLOW}Swag linting passed with $WARNINGS warning(s)${NC}"
    echo "Consider addressing the warnings above."
    exit 0
else
    echo -e "${GREEN}All swag comment checks passed!${NC}"
    exit 0
fi
