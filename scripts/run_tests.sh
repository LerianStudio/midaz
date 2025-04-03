#!/bin/bash

# Script to run all tests in the repository and provide a comprehensive summary


# Initialize counters
start_time=$(date +%s)
total_passed=0
total_failed=0
total_skipped=0
exit_code=0

# Get the root directory
ROOT_DIR=$(pwd)
SDK_GO_DIR="${ROOT_DIR}/sdks/go"

# Run main project tests
echo "Running main project tests..."
test_output=$(go test -v ./... 2>&1)
main_exit_code=$?

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
echo "Comprehensive Test Summary:"
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
if [ $exit_code -eq 0 ]; then
    echo "All tests across the repository passed successfully!"
else
    echo "Some tests failed. Please check the output above for details."
fi

exit $exit_code
