#!/usr/bin/env bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

if (( $# < 3 )); then
  echo "usage: $0 <required-build-tag> <go-build-tags> <package-pattern>..." >&2
  exit 2
fi

required_tag=$1
go_build_tags=$2
shift 2

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
selected_files=$("$script_dir/discover-tagged-test-packages.sh" \
  --files "$required_tag" "$go_build_tags" "$@")

selected_count=0

while IFS=$'\t' read -r import_path test_file; do
  [[ -n "$test_file" ]] || continue

  test_functions=$(sed -nE \
    's/^func[[:space:]]+(Test[[:alnum:]_]+)[[:space:]]*\(.*/\1/p' \
    "$test_file")

  while IFS= read -r test_name; do
    [[ -n "$test_name" && "$test_name" != "TestMain" ]] || continue
    printf '%s %s\n' "$import_path" "$test_name"
    selected_count=$((selected_count + 1))
  done <<<"$test_functions"
done <<<"$selected_files"

if (( selected_count == 0 )); then
  echo "[error] build tag \"$required_tag\" selected zero top-level Test functions" >&2
  exit 1
fi
