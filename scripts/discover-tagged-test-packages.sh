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
# Grep then identifies which of those selected test files explicitly owns the
# required lane tag. This excludes ordinary co-located tests and constraints
# such as `!integration` without reimplementing Go's constraint evaluator.
selected_files=$(GOFLAGS="-buildvcs=false ${GOFLAGS:-}" go list \
  -tags="$go_build_tags" \
  -f '{{range .TestGoFiles}}{{$.Dir}}{{"\t"}}{{$.ImportPath}}{{"\t"}}{{.}}{{"\n"}}{{end}}{{range .XTestGoFiles}}{{$.Dir}}{{"\t"}}{{$.ImportPath}}{{"\t"}}{{.}}{{"\n"}}{{end}}' \
  "$@")

packages_file=$(mktemp)
trap 'rm -f "$packages_file"' EXIT
selected_file_count=0

while IFS=$'\t' read -r package_dir import_path test_file; do
  [[ -n "$test_file" ]] || continue

  build_constraint=$(grep -E '^//go:build[[:space:]]+' "$package_dir/$test_file") \
    && constraint_status=0 || constraint_status=$?

  if (( constraint_status > 1 )); then
    exit "$constraint_status"
  fi

  if (( constraint_status == 0 )) \
    && printf '%s\n' "$build_constraint" \
      | grep -Eq "(^|[^[:alnum:]_])${required_tag}([^[:alnum:]_]|$)"; then
    selected_file_count=$((selected_file_count + 1))

    if [[ "$output_mode" == "files" ]]; then
      printf '%s\t%s\n' "$import_path" "$package_dir/$test_file"
    else
      printf '%s\n' "$import_path" >>"$packages_file"
    fi
  else
    tag_status=$?
    if (( constraint_status == 0 && tag_status > 1 )); then
      exit "$tag_status"
    fi
  fi
done <<<"$selected_files"

if (( selected_file_count == 0 )); then
  echo "[error] no Go packages contain tests selected by build tag \"$required_tag\"" >&2
  exit 1
fi

if [[ "$output_mode" == "packages" ]]; then
  awk '!seen[$0]++' "$packages_file"
fi
