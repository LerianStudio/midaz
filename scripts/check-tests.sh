#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

# Set colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Flag to track if any component failed the coverage check
ANY_FAILED=false

# Default minimum test coverage percentage.
# Applied to any component not listed in MIN_COVERAGE_OVERRIDE below.
MIN_COVERAGE_DEFAULT=80

# Per-component coverage floors for components that hit an architectural
# ceiling in unit-test mode. See docs/adrs/0001-per-component-coverage-floors.md
# for the full rationale — both components have composition-root bootstraps
# (InitServersWithOptions, bootstrap.Run) wiring 12+ concrete subsystems
# that require testcontainers-backed integration tests (Gate 6) to exercise.
# Headroom is intentionally tight so regressions are still caught.
declare -A MIN_COVERAGE_OVERRIDE=(
    ["authorizer"]=70
    ["transaction"]=65
)

# Find all Go packages in the project
echo "Checking test coverage for all components..."
echo ""

# Check each component directory
for component in ./components/*; do
    if [ -d "$component" ]; then
        component_name=$(basename "$component")
        echo "${YELLOW}Checking $component_name component...${NC}"
        
        # Skip if no Go files
        if ! find "$component" -name "*.go" -type f | grep -q .; then
            echo "${YELLOW}No Go files found in $component_name, skipping coverage check${NC}"
            echo ""
            continue
        fi
        
        # Run coverage test (unit mode: -short skips tests gated on testing.Short()).
        # Capture combined output so we can surface failures instead of silently
        # reporting "No coverage data generated".
        test_log=$(mktemp)
        (cd "$component" && go test -short -coverprofile=coverage.tmp ./... > "$test_log" 2>&1)
        test_exit=$?

        if [ $test_exit -eq 0 ]; then
            # Extract coverage percentage
            if [ -f "$component/coverage.tmp" ]; then
                # Exclude generated files from coverage calculation.
                # Generated code (mockgen mocks, swaggo docs, protobuf stubs)
                # cannot and should not be tested; counting them in the
                # denominator penalizes components for legitimate codegen.
                # Patterns: *_mock.go (mockgen), *_docs.go / api/docs.go (swaggo),
                # *.pb.go (protobuf).
                grep -Ev '(_mock\.go|_docs\.go|/docs\.go|\.pb\.go):' \
                    "$component/coverage.tmp" > "$component/coverage.filtered"
                mv "$component/coverage.filtered" "$component/coverage.tmp"

                coverage=$(go tool cover -func="$component/coverage.tmp" | grep total | awk '{print $3}' | sed 's/%//')

                # Resolve the floor for this component (override or default).
                floor=${MIN_COVERAGE_OVERRIDE[$component_name]:-$MIN_COVERAGE_DEFAULT}

                # Compare with minimum required
                if (( $(echo "$coverage < $floor" | bc -l) )); then
                    echo "${RED}${BOLD}[FAIL]${NC} $component_name coverage is ${RED}$coverage%${NC} (minimum required: ${GREEN}$floor%${NC})"
                    ANY_FAILED=true
                else
                    echo "${GREEN}${BOLD}[PASS]${NC} $component_name coverage is ${GREEN}$coverage%${NC} (floor: ${GREEN}$floor%${NC})"
                fi

                # Clean up temporary file
                rm "$component/coverage.tmp"
            else
                echo "${YELLOW}No coverage data generated for $component_name${NC}"
                echo "${YELLOW}--- go test output ---${NC}"
                cat "$test_log"
                echo "${YELLOW}--- end output ---${NC}"
                ANY_FAILED=true
            fi
        else
            echo "${RED}${BOLD}[ERROR]${NC} Failed to run tests for $component_name (exit $test_exit)"
            echo "${RED}--- go test output ---${NC}"
            cat "$test_log"
            echo "${RED}--- end output ---${NC}"
            ANY_FAILED=true
        fi

        rm -f "$test_log"
        
        echo ""
    fi
done

echo "Coverage check completed."

# Exit with appropriate status code
if [ "$ANY_FAILED" = true ]; then
    echo "${RED}${BOLD}[ERROR]${NC} Some components failed the coverage check"
    exit 1
fi

exit 0
