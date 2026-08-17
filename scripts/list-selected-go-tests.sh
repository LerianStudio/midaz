#!/usr/bin/env bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

if (( $# < 3 )); then
  echo "usage: $0 <go-build-tags> <run-pattern> <package>..." >&2
  exit 2
fi

go_build_tags=$1
run_pattern=$2
shift 2

selected_count=0

for package in "$@"; do
  package_listing=$(GOFLAGS="-buildvcs=false ${GOFLAGS:-}" go test \
    -tags="$go_build_tags" -list "$run_pattern" "$package")

  runnable_tests=$(printf '%s\n' "$package_listing" \
    | grep -E '^(Test|Fuzz|Example)[[:alnum:]_]*$') && grep_status=0 || grep_status=$?

  if (( grep_status > 1 )); then
    exit "$grep_status"
  fi

  if (( grep_status == 0 )); then
    while IFS= read -r test_name; do
      [[ -n "$test_name" ]] || continue
      printf '%s %s\n' "$package" "$test_name"
      selected_count=$((selected_count + 1))
    done <<<"$runnable_tests"
  fi
done

if (( selected_count == 0 )); then
  echo "[error] build tags \"$go_build_tags\" and run pattern \"$run_pattern\" selected zero runnable tests" >&2
  exit 1
fi
