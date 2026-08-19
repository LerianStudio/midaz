#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
report_dir=${CI_REPORT_DIR:-$repo_root/reports/ci}
wall_timeout=${E2E_TRACER_V2_WALL_TIMEOUT:-10m}
go_timeout=${E2E_TRACER_V2_GO_TIMEOUT:-9m}
json_file="$report_dir/ledger-tracer-v2-e2e.json"
junit_file="$report_dir/ledger-tracer-v2-e2e.xml"

require_ledger_v2_mode() {
  local readiness
  if ! readiness=$(curl --fail --silent --show-error --connect-timeout 5 --max-time 30 "$LEDGER_URL/readyz"); then
    echo "Tracer V2 E2E preflight could not read Ledger readiness at $LEDGER_URL/readyz" >&2
    return 1
  fi

  if ! grep -Eq '"tracer_outcome_mode"[[:space:]]*:[[:space:]]*"ledger_outcome_v2"' <<<"$readiness"; then
    echo "Tracer V2 E2E requires the running Ledger to report tracer_outcome_mode=ledger_outcome_v2" >&2
    echo "Ledger readiness: $readiness" >&2
    return 1
  fi
}

run_tests() {
  mkdir -p "$report_dir"
  cd "$repo_root"

  export E2E_REQUIRED=1
  export E2E_TRACER_OUTCOME_V2=1
  export E2E_LEDGER_WORKERS=1
  export LEDGER_URL=${LEDGER_URL:-http://localhost:3002}
  export TRACER_URL=${TRACER_URL:-http://localhost:4020}

  require_ledger_v2_mode

  go_args=(-tags=e2e -v -count=1 -parallel=1 "-timeout=$go_timeout" -run '^TestTracerOutcomeV2LedgerToTracerAndLostACK$' ./tests/e2e/...)
  test_status=0
  if command -v gotestsum >/dev/null 2>&1; then
    gotestsum --format testname --junitfile "$junit_file" --jsonfile "$json_file" -- "${go_args[@]}" || test_status=$?
  else
    go test -json "${go_args[@]}" | tee "$json_file" || test_status=${PIPESTATUS[0]}
  fi
  if [[ $test_status -ne 0 ]]; then
    return "$test_status"
  fi

  if ! grep -Eq '"Action":"pass".*"Test":"TestTracerOutcomeV2LedgerToTracerAndLostACK"|"Test":"TestTracerOutcomeV2LedgerToTracerAndLostACK".*"Action":"pass"' "$json_file"; then
    echo "required Tracer V2 E2E produced no passing durable outcome event" >&2
    return 1
  fi
}

if [[ ${1:-} == --execute ]]; then
  run_tests
  exit $?
fi

CI_REPORT_DIR="$report_dir" "$repo_root/scripts/run-ci-lane.sh" \
  ledger-tracer-v2-e2e "$wall_timeout" "$repo_root/scripts/run-required-tracer-v2-e2e.sh" --execute
