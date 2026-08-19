#!/usr/bin/env bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

output_mode=packages
if [[ "${1:-}" == "--files" ]]; then
  output_mode=files
  shift
fi

if (( $# < 3 )); then
  echo "usage: $0 [--files] <required-build-tag> <go-build-tags> <package-pattern>..." >&2
  exit 2
fi

required_tag=$1
go_build_tags=$2
shift 2

if [[ ! "$required_tag" =~ ^[[:alnum:]_]+$ ]]; then
  echo "[error] invalid Go build tag: $required_tag" >&2
  exit 2
fi

# go list decides which files the requested build-tag set actually selects.
# The semantic filter then evaluates each selected file's parsed build
# constraint with the lane tag enabled and disabled. A file belongs to the lane
# only when that tag changes the constraint from false to true.
selected_files=$(GOFLAGS="-buildvcs=false ${GOFLAGS:-}" go list \
  -tags="$go_build_tags" \
  -f '{{range .TestGoFiles}}{{$.Dir}}{{"\t"}}{{$.ImportPath}}{{"\t"}}{{.}}{{"\n"}}{{end}}{{range .XTestGoFiles}}{{$.Dir}}{{"\t"}}{{$.ImportPath}}{{"\t"}}{{.}}{{"\n"}}{{end}}' \
  "$@")

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
candidate_files=$(printf '%s\n' "$selected_files" \
  | awk -F '\t' 'NF == 3 { print $2 "\t" $1 "/" $3 }')
lane_files=$(printf '%s\n' "$candidate_files" \
  | GOFLAGS="-buildvcs=false ${GOFLAGS:-}" go run "$script_dir/test_selection/main.go" \
      filter-files "$required_tag" "$go_build_tags")

if [[ -z "$lane_files" ]]; then
  echo "[error] no Go packages contain tests selected by build tag \"$required_tag\"" >&2
  exit 1
fi

if [[ "$output_mode" == "files" ]]; then
  printf '%s\n' "$lane_files"
else
  printf '%s\n' "$lane_files" | awk -F '\t' '!seen[$1]++ { print $1 }'
fi
