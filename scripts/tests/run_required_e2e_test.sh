#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

mkdir -p "$test_dir/bin"
ln -s "$repo_root/scripts/tests/fixtures/fake-gotestsum" "$test_dir/bin/gotestsum"

PATH="$test_dir/bin:$PATH" CI_REPORT_DIR="$test_dir/pass" FAKE_SENTINEL=1 \
  E2E_REQUIRED_WALL_TIMEOUT=5s "$repo_root/scripts/run-required-e2e.sh"
grep -q '"status":"passed"' "$test_dir/pass/ledger-e2e-timing.json"
grep -q 'TestRequiredStackLane' "$test_dir/pass/ledger-e2e.json"
test -s "$test_dir/pass/ledger-e2e.xml"

status=0
PATH="$test_dir/bin:$PATH" CI_REPORT_DIR="$test_dir/streaming-skip" FAKE_SENTINEL=1 FAKE_STREAMING_SKIP=1 \
  E2E_REQUIRED_WALL_TIMEOUT=5s "$repo_root/scripts/run-required-e2e.sh" || status=$?
if [[ $status -eq 0 ]]; then
  echo "required E2E accepted skipped tests for its selected streaming capability" >&2
  exit 1
fi
grep -q '"status":"failed"' "$test_dir/streaming-skip/ledger-e2e-timing.json"

status=0
PATH="$test_dir/bin:$PATH" CI_REPORT_DIR="$test_dir/reservation-skip" FAKE_SENTINEL=1 FAKE_RESERVATION_SKIP=1 \
  E2E_REQUIRED_WALL_TIMEOUT=5s "$repo_root/scripts/run-required-e2e.sh" || status=$?
if [[ $status -eq 0 ]]; then
  echo "required E2E accepted a skipped reservation tuple-idempotency capability" >&2
  exit 1
fi
grep -q '"status":"failed"' "$test_dir/reservation-skip/ledger-e2e-timing.json"

status=0
PATH="$test_dir/bin:$PATH" CI_REPORT_DIR="$test_dir/empty" FAKE_SENTINEL=0 \
  E2E_REQUIRED_WALL_TIMEOUT=5s "$repo_root/scripts/run-required-e2e.sh" || status=$?
if [[ $status -eq 0 ]]; then
  echo "required E2E accepted a lane without its stack sentinel" >&2
  exit 1
fi
grep -q '"status":"failed"' "$test_dir/empty/ledger-e2e-timing.json"

echo "run-required-e2e tests passed"
