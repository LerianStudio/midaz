#!/bin/bash

# Script to run all tests in the repository and provide a comprehensive summary


# Initialize counters
start_time=$(date +%s)
total_passed=0
total_failed=0
total_skipped=0
exit_code=0
no_test_packages=()

# Get the root directory
ROOT_DIR=$(pwd)
SDK_GO_DIR="${ROOT_DIR}/sdks/go-sdk"

# Load ignore list if it exists
IGNORE_LIST=()
if [ -f "${ROOT_DIR}/scripts/coverage_ignore.txt" ]; then
    while IFS= read -r line; do
        # Skip empty lines and comments
        if [[ -n "$line" && ! "$line" =~ ^# ]]; then
            IGNORE_LIST+=("$line")
        fi
    done < "${ROOT_DIR}/scripts/coverage_ignore.txt"
fi

# Function to check if a package should be ignored
should_ignore() {
    local pkg="$1"
    for ignore_pkg in "${IGNORE_LIST[@]}"; do
        if [[ "$pkg" == "$ignore_pkg" ]]; then
            return 0 # True, should ignore
        fi
    done
    return 1 # False, should not ignore
}

# Run main project tests
echo "Running main project tests..."
test_output=$(go test -v ./... 2>&1)
main_exit_code=$?

# Collect packages with no test files
main_no_test_packages=$(echo "$test_output" | grep "\[no test" | sed 's/^?[[:space:]]*\(.*\)[[:space:]]*\[no test.*$/\1/' || true)
for pkg in $main_no_test_packages; do
    # Skip packages that should be ignored
    if should_ignore "$pkg"; then
        continue
    fi
    rel_path=$(echo "$pkg" | sed "s|github.com/LerianStudio/midaz/||")
    no_test_packages+=("$rel_path")
done

# Count test results with proper error handling
passed=$(echo "$test_output" | grep -c "PASS" || true)
failed=$(echo "$test_output" | grep -c "FAIL" || true)
skipped=$(echo "$test_output" | grep -c "\[no test" || true)

# Update totals
total_passed=$((total_passed + passed))
total_failed=$((total_failed + failed))
total_skipped=$((total_skipped + skipped))

# Display output
echo "$test_output"
echo ""

# Run Go SDK tests
echo "Running Go SDK tests..."
cd "$SDK_GO_DIR" 
sdk_test_output=$(make test 2>&1)
sdk_exit_code=$?
cd "$ROOT_DIR"

# Display output
echo "$sdk_test_output"

# Collect SDK packages with no test files
sdk_no_test_packages=$(echo "$sdk_test_output" | grep "\[no test" | sed 's/^?[[:space:]]*\(.*\)[[:space:]]*\[no test.*$/\1/' || true)
for pkg in $sdk_no_test_packages; do
    # Skip packages that should be ignored
    if should_ignore "$pkg"; then
        continue
    fi
    rel_path=$(echo "$pkg" | sed "s|github.com/LerianStudio/midaz/||")
    no_test_packages+=("$rel_path")
done

# Count SDK test results with proper error handling
sdk_passed=$(echo "$sdk_test_output" | grep -c "PASS" || true)
sdk_failed=$(echo "$sdk_test_output" | grep -c "FAIL" || true)
sdk_skipped=$(echo "$sdk_test_output" | grep -c "\[no test" || true)

# Update totals
total_passed=$((total_passed + sdk_passed))
total_failed=$((total_failed + sdk_failed))
total_skipped=$((total_skipped + sdk_skipped))

echo ""

# Calculate overall exit code
if [ $main_exit_code -ne 0 ] || [ $sdk_exit_code -ne 0 ]; then
    exit_code=1
fi

# Display comprehensive summary
end_time=$(date +%s)
duration=$((end_time - start_time))
echo "Comprehensive Test Summary (considering coverage_ignore.txt):"
echo "----------------------------------------"
echo "✓ Passed:  $total_passed tests"
if [ $total_failed -gt 0 ]; then
    echo "✗ Failed:  $total_failed tests"
else
    echo "✓ Failed:  $total_failed tests"
fi
echo "⚠ Skipped: $total_skipped packages [no test files]"
echo "⏱ Duration: $(printf "%dm:%02ds" $((duration / 60)) $((duration % 60)))"
echo "----------------------------------------"

# List packages with no test files (excluding ignored packages)
if [ ${#no_test_packages[@]} -gt 0 ]; then
    echo "Packages with no test files (excluding ignored packages):"
    echo "----------------------------------------"
    for pkg in "${no_test_packages[@]}"; do
        echo "  - $pkg"
    done
    echo "----------------------------------------"
fi

if [ $exit_code -eq 0 ]; then
    echo "All tests across the repository passed successfully!"
else
    echo "Some tests failed. Please check the output above for details."
fi

exit $exit_code
