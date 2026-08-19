#!/usr/bin/env bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

if (( $# != 2 )); then
  echo "usage: $0 <run-pattern> <runtime-patterns-file> < tagged-test-inventory" >&2
  exit 2
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
GOFLAGS="-buildvcs=false ${GOFLAGS:-}" \
  go run "$script_dir/test_selection/main.go" prepare-run "$1" "$2"
