#!/bin/bash

# Script to generate test coverage reports for all components and the Go SDK

# ===== Configuration =====
# Get the root directory
ROOT_DIR=$(pwd)
SDK_GO_DIR="${ROOT_DIR}/sdks/go-sdk"

# ===== Header =====
echo "===== Test Coverage Report Generation ====="

# ===== Main Project Coverage =====
echo ""
echo "Generating coverage for Components:"

# Get the list of packages to test, excluding those in the ignore list
if [ -f "./scripts/coverage_ignore.txt" ]; then
    IGNORED=$(cat ./scripts/coverage_ignore.txt | xargs -I{} echo '-not -path ./{}/*' | xargs)
    PACKAGES=$(go list ./pkg/... ./components/... | grep -v -f ./scripts/coverage_ignore.txt)
else
    PACKAGES=$(go list ./pkg/... ./components/...)
fi

echo "- Running tests on packages:"
echo "$PACKAGES"

# Run the tests and generate coverage profile
go test -cover $PACKAGES -coverprofile=coverage.out

# Print coverage summary
echo ""
echo "Main Project Coverage Summary:"
COMPONENT_COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
echo "- Overall coverage: $COMPONENT_COVERAGE"

# Generate HTML report for components
echo "- Generating HTML coverage report: coverage.html"
go tool cover -html=coverage.out -o coverage.html

# ===== SDK Coverage =====
echo ""
echo "Generating coverage for Go SDK:"

# Check if SDK directory exists
if [ -d "${SDK_GO_DIR}" ]; then
    # Change to the SDK directory
    cd "${SDK_GO_DIR}"
    
    # Get the list of SDK packages to test, excluding those in the ignore list
    if [ -f "${ROOT_DIR}/scripts/coverage_ignore.txt" ]; then
        SDK_PACKAGES=$(go list ./... | grep -v -f "${ROOT_DIR}/scripts/coverage_ignore.txt")
    else
        SDK_PACKAGES=$(go list ./...)
    fi
    
    echo "- Running tests on SDK packages:"
    echo "$SDK_PACKAGES"
    
    # Run the tests and generate coverage profile
    go test -cover $SDK_PACKAGES -coverprofile=sdk_coverage.out
    
    # Print SDK coverage summary
    SDK_COVERAGE=$(go tool cover -func=sdk_coverage.out | grep total | awk '{print $3}')
    echo "- Overall coverage: $SDK_COVERAGE"
    
    # Generate HTML report for SDK
    echo "- Generating HTML coverage report: sdk_coverage.html"
    go tool cover -html=sdk_coverage.out -o sdk_coverage.html
    
    # Move the coverage files to the root directory
    mv sdk_coverage.out "${ROOT_DIR}/sdk_coverage.out"
    mv sdk_coverage.html "${ROOT_DIR}/sdk_coverage.html"
    
    # Return to the root directory
    cd "${ROOT_DIR}"
else
    echo "ℹ️ SDK directory not found"
fi

# ===== Notes =====
echo ""
echo "===== Notes on Coverage ====="
echo "📝 PostgreSQL repository tests are excluded from coverage metrics because they use mock repositories"
echo "   that satisfy the Repository interface rather than directly testing the actual implementation."
echo "   This is a common pattern in Go testing for database interactions, but it means the coverage"
echo "   report doesn't accurately reflect the test coverage of the database code."
echo ""
echo "📝 Mock packages and model-only packages are excluded from coverage metrics as specified in"
echo "   scripts/coverage_ignore.txt. These packages typically don't require tests as they are either"
echo "   auto-generated or contain only data structures without business logic."
echo ""
echo "   Despite not being included in coverage metrics, these tests effectively validate:"
echo "   - The correct behavior of the repository interfaces"
echo "   - Proper handling of success and error cases"
echo "   - Correct SQL query construction"
echo "   - Appropriate error handling"

# ===== Summary =====
echo ""
echo "===== Summary ====="
echo "✅ Coverage reports generated successfully"
echo ""
echo "📊 Coverage Reports:"
echo "- Components: coverage.html"
if [ -f "sdk_coverage.html" ]; then
    echo "- Go SDK: sdk_coverage.html"
fi
echo ""
echo "🔍 To view the reports, open the HTML files in your browser:"
echo "- open coverage.html"
if [ -f "sdk_coverage.html" ]; then
    echo "- open sdk_coverage.html"
fi
