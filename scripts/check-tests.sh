#!/bin/bash

# Script to verify test coverage for all components and SDK

# ===== Configuration =====
# Minimum required test coverage percentage
MIN_COVERAGE=80

# ===== Header =====
echo "===== Test Coverage Verification ====="

# ===== Component Check =====
echo ""
echo "Checking Components:"
COMPONENT_ISSUES=0

# Check each component directory
for component in ./components/*; do
    if [ -d "$component" ]; then
        component_name=$(basename "$component")
        echo "- $component_name"
        
        # Skip if no Go files
        if ! find "$component" -name "*.go" -type f | grep -q .; then
            echo "  ℹ️ No Go files found, skipping"
            continue
        fi
        
        # Run coverage test
        (cd "$component" && go test -coverprofile=coverage.tmp ./... > /dev/null 2>&1)
        
        if [ $? -eq 0 ]; then
            # Extract coverage percentage
            if [ -f "$component/coverage.tmp" ]; then
                coverage=$(go tool cover -func="$component/coverage.tmp" | grep total | awk '{print $3}' | sed 's/%//')
                
                # Compare with minimum required
                if (( $(echo "$coverage < $MIN_COVERAGE" | bc -l) )); then
                    echo "  ❌ Coverage: $coverage% (minimum: $MIN_COVERAGE%)"
                    COMPONENT_ISSUES=$((COMPONENT_ISSUES + 1))
                else
                    echo "  ✅ Coverage: $coverage%"
                fi
                
                # Clean up temporary file
                rm "$component/coverage.tmp"
            else
                echo "  ⚠️ No coverage data generated"
            fi
        else
            echo "  ❌ Failed to run tests"
            COMPONENT_ISSUES=$((COMPONENT_ISSUES + 1))
        fi
    fi
done

# ===== SDK Check =====
echo ""
echo "Checking Go SDK:"
SDK_ISSUES=0

# Check the Go SDK
SDK_DIR="./sdks/go"
if [ -d "$SDK_DIR" ]; then
    echo "  ℹ️ Using SDK's own coverage check"
    
    # Run the SDK's coverage check
    SDK_OUTPUT=$(cd "$SDK_DIR" && make coverage 2>&1)
    SDK_EXIT_CODE=$?
    
    # Extract coverage percentage from the output
    SDK_COVERAGE=$(echo "$SDK_OUTPUT" | grep -o 'coverage: [0-9]*\.[0-9]*%' | head -1 | sed 's/coverage: //' | sed 's/%//')
    
    if [ -n "$SDK_COVERAGE" ]; then
        # Compare with minimum required
        if (( $(echo "$SDK_COVERAGE < $MIN_COVERAGE" | bc -l) )); then
            echo "  ❌ Coverage: $SDK_COVERAGE% (minimum: $MIN_COVERAGE%)"
            SDK_ISSUES=$((SDK_ISSUES + 1))
        else
            echo "  ✅ Coverage: $SDK_COVERAGE%"
        fi
    else
        if [ $SDK_EXIT_CODE -eq 0 ]; then
            echo "  ⚠️ Could not determine coverage"
        else
            echo "  ❌ Failed to run tests"
        fi
        SDK_ISSUES=$((SDK_ISSUES + 1))
    fi
else
    echo "  ℹ️ SDK directory not found"
fi

# ===== Summary =====
echo ""
echo "===== Summary ====="
TOTAL_ISSUES=$((COMPONENT_ISSUES + SDK_ISSUES))

if [ $TOTAL_ISSUES -eq 0 ]; then
    echo "✅ All components and SDK meet the minimum coverage requirement of $MIN_COVERAGE%"
else
    echo "⚠️ Found $TOTAL_ISSUES coverage issues:"
    echo "  - Components: $COMPONENT_ISSUES issues"
    echo "  - Go SDK: $SDK_ISSUES issues"
    echo ""
    echo "➡️ Consider improving test coverage to meet the minimum requirement of $MIN_COVERAGE%"
fi

exit 0
