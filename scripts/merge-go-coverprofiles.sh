#!/usr/bin/env bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

if (( $# < 2 )); then
  echo "usage: $0 <output-coverprofile> <input-coverprofile>..." >&2
  exit 2
fi

output_profile=$1
shift

mode=

for input_profile in "$@"; do
  [[ -s "$input_profile" ]] || {
    echo "[error] coverprofile is missing or empty: $input_profile" >&2
    exit 1
  }

  IFS= read -r input_mode <"$input_profile" || true
  [[ "$input_mode" == mode:* ]] || {
    echo "[error] invalid coverprofile header in $input_profile" >&2
    exit 1
  }

  if [[ -z "$mode" ]]; then
    mode=$input_mode
  elif [[ "$input_mode" != "$mode" ]]; then
    echo "[error] coverprofile mode mismatch: '$mode' versus '$input_mode'" >&2
    exit 1
  fi

done

body_file=$(mktemp)
trap 'rm -f "$body_file"' EXIT

awk '
  FNR == 1 { next }
  NF != 3 || $2 !~ /^[0-9]+$/ || $3 !~ /^[0-9]+$/ {
    print "[error] malformed coverprofile row in " FILENAME ": " $0 > "/dev/stderr"
    invalid = 1
    next
  }
  {
    key = $1 SUBSEP $2
    counts[key] += $3
  }
  END {
    if (invalid) {
      exit 1
    }
    for (key in counts) {
      split(key, fields, SUBSEP)
      print fields[1], fields[2], counts[key]
    }
  }
' "$@" | LC_ALL=C sort >"$body_file"

{
  printf '%s\n' "$mode"
  cat "$body_file"
} >"$output_profile"
