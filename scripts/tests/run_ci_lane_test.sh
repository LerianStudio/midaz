#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

CI_REPORT_DIR="$test_dir" "$repo_root/scripts/run-ci-lane.sh" success 5s bash -c 'printf "lane ran\n"'
grep -q '"status":"passed"' "$test_dir/success-timing.json"
grep -q 'lane ran' "$test_dir/success.log"

status=0
CI_REPORT_DIR="$test_dir" "$repo_root/scripts/run-ci-lane.sh" failure 5s bash -c 'exit 7' || status=$?
if [[ $status -ne 7 ]]; then
  echo "failure lane returned $status, want 7" >&2
  exit 1
fi
grep -q '"status":"failed"' "$test_dir/failure-timing.json"
grep -q '"exit_code":7' "$test_dir/failure-timing.json"

status=0
CI_REPORT_DIR="$test_dir" "$repo_root/scripts/run-ci-lane.sh" bounded 1s bash -c 'sleep 5' || status=$?
if [[ $status -ne 124 ]]; then
  echo "bounded lane returned $status, want timeout exit 124" >&2
  exit 1
fi
grep -q '"status":"timed_out"' "$test_dir/bounded-timing.json"

echo "run-ci-lane tests passed"
