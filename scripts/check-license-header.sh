#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

# Check if staged files have the required license header
# Returns 0 if all files have headers, 1 otherwise

REPO_ROOT=$(git rev-parse --show-toplevel)
source "$REPO_ROOT"/pkg/shell/colors.sh 2>/dev/null || true

# Get staged files by type (excluding generated files)
# Excludes: *.pb.go (protobuf), mock_*.go, *_mock.go, *_mocks.go (mockgen)
STAGED_FILES=$(git diff --cached --name-only --diff-filter=d | grep -E '\.(go|ts|js|sh|proto)$' | grep -v '\.pb\.go$' | grep -v -E '(^|/)mock_.*\.go$' | grep -v -E '_mocks?\.go$' || true)

if [ -z "$STAGED_FILES" ]; then
    exit 0
fi

MISSING_HEADER=""

for file in $STAGED_FILES; do
    # Read STAGED content (not working directory) using git show
    FIRST_LINES=$(git show ":$file" 2>/dev/null | head -10)
    if [ -n "$FIRST_LINES" ]; then
        # Check if line STARTS with comment + Copyright (regex anchored to line start)
        # This avoids matching patterns inside string literals
        if ! echo "$FIRST_LINES" | grep -qE '^(//|#) Copyright \(c\) 2026 Lerian Studio'; then
            MISSING_HEADER="${MISSING_HEADER}${file}\n"
        fi
    fi
done

if [ -n "$MISSING_HEADER" ]; then
    echo "${red:-}❌ Missing license header in files:${normal:-}"
    echo -e "$MISSING_HEADER"
    echo ""
    echo "Add this header to the top of each file:"
    echo ""
    echo "  // Copyright (c) 2026 Lerian Studio. All rights reserved."
    echo "  // Use of this source code is governed by the Elastic License 2.0"
    echo "  // that can be found in the LICENSE file."
    echo ""
    echo "For shell scripts, use # instead of //"
    exit 1
fi

exit 0
