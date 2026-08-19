#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
report_dir=${CI_REPORT_DIR:-$repo_root/reports/ci}
wall_timeout=${E2E_REQUIRED_WALL_TIMEOUT:-20m}
go_timeout=${E2E_REQUIRED_GO_TIMEOUT:-19m}
json_file="$report_dir/ledger-e2e.json"
junit_file="$report_dir/ledger-e2e.xml"

run_tests() {
  mkdir -p "$report_dir"
  cd "$repo_root"

  export E2E_REQUIRED=1
  export E2E_STREAMING=${E2E_STREAMING:-1}
  export E2E_LEDGER_WORKERS=${E2E_LEDGER_WORKERS:-4}
  export LEDGER_URL=${LEDGER_URL:-http://localhost:3002}
  export TRACER_URL=${TRACER_URL:-http://localhost:4020}

  go_args=(-tags=e2e -v -count=1 "-parallel=$E2E_LEDGER_WORKERS" "-timeout=$go_timeout")
  if [[ -n ${GO_TEST_LDFLAGS:-} ]]; then
    read -r -a extra_go_args <<< "$GO_TEST_LDFLAGS"
    go_args+=("${extra_go_args[@]}")
  fi
  go_args+=(./tests/e2e/...)

  test_status=0
  if command -v gotestsum >/dev/null 2>&1; then
    gotestsum --format testname --junitfile "$junit_file" --jsonfile "$json_file" -- "${go_args[@]}" || test_status=$?
  else
    go test -json "${go_args[@]}" | tee "$json_file" || test_status=${PIPESTATUS[0]}
  fi

  if [[ $test_status -ne 0 ]]; then
    return "$test_status"
  fi

  required_passes=(
    TestRequiredStackLane
    TestStreamingAccountCreatedEmitted
    TestStreamingHolderCreateEmitsRedacted
    TestReserveLifecycleTransactionTupleNoDoubleHold
  )
  for required_test in "${required_passes[@]}"; do
    if ! grep -Eq "\"Action\":\"pass\".*\"Test\":\"$required_test\"|\"Test\":\"$required_test\".*\"Action\":\"pass\"" "$json_file"; then
      echo "required E2E lane produced no passing $required_test event; selected capabilities may not skip or disappear" >&2
      return 1
    fi
  done
}

if [[ ${1:-} == --execute ]]; then
  run_tests
  exit $?
fi

CI_REPORT_DIR="$report_dir" "$repo_root/scripts/run-ci-lane.sh" \
  ledger-e2e "$wall_timeout" "$repo_root/scripts/run-required-e2e.sh" --execute
