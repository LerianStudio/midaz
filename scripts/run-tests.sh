#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

# Import shared utilities and colors
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Check required commands
check_command() {
    command -v $1 >/dev/null 2>&1 || { echo "Error: $1 is required but not installed. $2"; exit 1; }
}

check_command go "Install Go from https://golang.org/doc/install"

# Print header
echo "------------------------------------------"
echo "   📝 Running tests on all components  "
echo "------------------------------------------"

# Start timing
echo "Starting tests at $(date)"
start_time=$(date +%s)
overall_exit_code=0

# Midaz is a single Go module rooted at PROJECT_ROOT, so `go test ./...`
# covers ledger + tracer + reporter + pkg in one pass. Flags mirror
# mk/tests.mk test-unit (-race -count=1) so behaviour stays consistent.
echo -e "\nRunning go test ./... from $PROJECT_ROOT ..."
(cd "$PROJECT_ROOT" && go test -race -count=1 ./...) || overall_exit_code=$?

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
    echo "[error] Some tests failed (go test exited $overall_exit_code). Please check the output above for details."
fi

exit $overall_exit_code
