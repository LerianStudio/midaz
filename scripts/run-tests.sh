#!/bin/bash

# Import shared utilities and colors
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Check required commands
check_command() {
    command -v $1 >/dev/null 2>&1 || { echo "Error: $1 is required but not installed. $2"; exit 1; }
}

check_command go "Install Go from https://golang.org/doc/install"
check_command npm "Install Node.js and npm from https://nodejs.org/"

# Print header
echo "------------------------------------------"
echo "   📝 Running tests on all components  "
echo "------------------------------------------"

# Start timing
echo "Starting tests at $(date)"
start_time=$(date +%s)
overall_exit_code=0

# Run core package tests
echo -e "\nRunning tests on pkg kernel..."
go test -v ./pkg || overall_exit_code=1

echo -e "\nRunning component tests..."

# Test console component
echo -e "\nTesting console component..."
if [ -d "components/console" ]; then
    (cd components/console && make test) || { 
        overall_exit_code=1
        echo "[error] Console component tests failed."
    }
fi

# Test mdz component
echo -e "\nTesting mdz component..."
if [ -d "components/mdz" ]; then
    (cd components/mdz && make test) || {
        overall_exit_code=1
        echo "[error] MDZ component tests failed."
    }
fi

# Test onboarding component
echo -e "\nTesting onboarding component..."
if [ -d "components/onboarding" ]; then
    (cd components/onboarding && make test) || {
        overall_exit_code=1
        echo "[error] Onboarding component tests failed."
    }
fi

# Test transaction component
echo -e "\nTesting transaction component..."
if [ -d "components/transaction" ]; then
    (cd components/transaction && make test) || {
        overall_exit_code=1
        echo "[error] Transaction component tests failed."
    }
fi

# Calculate duration and print summary
end_time=$(date +%s)
duration=$((end_time - start_time))
echo -e "\nTest Summary:"
echo "----------------------------------------"
echo "Duration: $(printf '%dm:%02ds' $((duration / 60)) $((duration % 60)))"
echo "----------------------------------------"

# Print final status and exit with appropriate code
if [ $overall_exit_code -eq 0 ]; then
    echo "[ok] All tests passed successfully ✔️"
else
    echo "[error] Some tests failed. Please check the output above for details."
fi

exit $overall_exit_code
